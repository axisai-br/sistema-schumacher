package midia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"schumacher-tur/api/internal/atendimento/llm/chatcompat"
)

const nvidiaBaseURLPadrao = "https://integrate.api.nvidia.com/v1"

func esquemaLeituraImagem() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"e_documento": map[string]any{"type": "boolean"},
			"nome":        map[string]any{"type": "string"},
			"cpf":         map[string]any{"type": "string"},
			"rg":          map[string]any{"type": "string"},
			"tipo":        map[string]any{"type": "string"},
			"nascimento":  map[string]any{"type": "string"},
			"descricao":   map[string]any{"type": "string"},
		},
		"required":             []string{"e_documento", "nome", "cpf", "rg", "tipo", "nascimento", "descricao"},
		"additionalProperties": false,
	}
}

// lerImagemChat le a imagem por Chat Completions OpenAI-compatible (NVIDIA NIM):
// content em partes (texto + image_url com data URL) e o mesmo JSON estruturado
// pedido via nvext.guided_json. Como nem todo modelo respeita o guided_json, o
// prompt tambem pede JSON e a resposta passa por limpeza (<think>, cercas).
func (p *Preparador) lerImagemChat(ctx context.Context, dataURL string) (leituraImagem, error) {
	schema := esquemaLeituraImagem()
	schemaJSON, _ := json.Marshal(schema)
	payload := map[string]any{
		"model": p.visao.Modelo,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": promptVisao + "\n\nResponda APENAS com um JSON válido seguindo este schema: " + string(schemaJSON)},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
			},
		}},
		"max_tokens":  1024,
		"temperature": 0.1,
		"stream":      false,
		"nvext":       map[string]any{"guided_json": schema},
	}
	if e := strings.TrimSpace(p.visao.EsforcoRaciocinio); e != "" {
		payload["reasoning_effort"] = strings.ToLower(e)
		payload["max_tokens"] = 4096
	}
	corpo, err := json.Marshal(payload)
	if err != nil {
		return leituraImagem{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.visao.BaseURL+"/chat/completions", bytes.NewReader(corpo))
	if err != nil {
		return leituraImagem{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.visao.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	raw, status, err := p.do(req)
	if err != nil {
		return leituraImagem{}, errors.New("visao: " + p.redigir(err.Error()))
	}
	if status < 200 || status >= 300 {
		return leituraImagem{}, fmt.Errorf("visao status=%d body=%s", status, resumir(p.redigir(string(raw)), 300))
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return leituraImagem{}, fmt.Errorf("visao: resposta invalida: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == nil {
		return leituraImagem{}, errors.New("visao: resposta sem texto")
	}
	texto := chatcompat.RemoverPensamento(*resp.Choices[0].Message.Content)
	obj, ok := chatcompat.ExtrairObjetoJSON(texto)
	if !ok {
		return leituraImagem{}, errors.New("visao: resposta sem json")
	}
	var l leituraImagem
	if err := json.Unmarshal([]byte(obj), &l); err != nil {
		return leituraImagem{}, fmt.Errorf("visao: json estruturado invalido: %w", err)
	}
	return l, nil
}

func (p *Preparador) redigir(s string) string {
	if p.visao.APIKey != "" {
		s = strings.ReplaceAll(s, p.visao.APIKey, "[redigido]")
	}
	return s
}
