// Package openai implementa llm.Modelo sobre a Responses API da OpenAI, via net/http.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
)

type Config struct {
	APIKey  string
	BaseURL string // padrao https://api.openai.com/v1
	Modelo  string
	Timeout time.Duration // padrao 45s
	HTTP    *http.Client
}

type Cliente struct {
	apiKey  string
	baseURL string
	modelo  string
	http    *http.Client
	backoff []time.Duration
}

var _ llm.Modelo = (*Cliente)(nil)

func Novo(cfg Config) *Cliente {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	hc := cfg.HTTP
	if hc == nil {
		t := cfg.Timeout
		if t <= 0 {
			t = 45 * time.Second
		}
		hc = &http.Client{Timeout: t}
	}
	return &Cliente{
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: base,
		modelo:  strings.TrimSpace(cfg.Modelo),
		http:    hc,
		backoff: []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond},
	}
}

// erroHTTP e um erro de status nao-2xx; o corpo e truncado e a chave nunca aparece.
type erroHTTP struct {
	status int
	corpo  string
}

func (e *erroHTTP) Error() string { return fmt.Sprintf("openai: status %d: %s", e.status, e.corpo) }

func (e *erroHTTP) retentavel() bool { return e.status == 429 || e.status >= 500 }

func (c *Cliente) montarPayload(p llm.Pedido) map[string]any {
	modelo := strings.TrimSpace(p.Modelo)
	if modelo == "" {
		modelo = c.modelo
	}
	input := make([]any, 0, len(p.Mensagens))
	for _, m := range p.Mensagens {
		switch m.Papel {
		case llm.PapelFerramenta:
			input = append(input, map[string]any{
				"type": "function_call_output", "call_id": m.ChamadaID, "output": m.Texto,
			})
		case llm.PapelAssistente:
			if strings.TrimSpace(m.Texto) != "" {
				input = append(input, map[string]any{"role": "assistant", "content": m.Texto})
			}
			for _, ch := range m.Chamadas {
				args := string(ch.Argumentos)
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				input = append(input, map[string]any{
					"type": "function_call", "call_id": ch.ID, "name": ch.Nome, "arguments": args,
				})
			}
		case llm.PapelSistema:
			input = append(input, map[string]any{"role": "system", "content": m.Texto})
		default:
			input = append(input, map[string]any{"role": "user", "content": m.Texto})
		}
	}
	payload := map[string]any{
		"model": modelo,
		"input": input,
		"store": false,
	}
	if strings.TrimSpace(p.Instrucoes) != "" {
		payload["instructions"] = p.Instrucoes
	}
	if len(p.Ferramentas) > 0 {
		tools := make([]any, 0, len(p.Ferramentas))
		for _, f := range p.Ferramentas {
			params := f.Parametros
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			tools = append(tools, map[string]any{
				"type": "function", "name": f.Nome, "description": f.Descricao,
				"parameters": params, "strict": false,
			})
		}
		payload["tools"] = tools
	}
	if len(p.SaidaJSON) > 0 {
		payload["text"] = map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "saida", "schema": p.SaidaJSON, "strict": true,
		}}
	}
	if p.MaxTokens > 0 {
		payload["max_output_tokens"] = p.MaxTokens
	}
	return payload
}

type respostaAPI struct {
	Model  string `json:"model"`
	Output []struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (c *Cliente) Gerar(ctx context.Context, p llm.Pedido) (llm.Resposta, error) {
	if c.apiKey == "" {
		return llm.Resposta{}, errors.New("openai: chave de API nao configurada")
	}
	corpo, err := json.Marshal(c.montarPayload(p))
	if err != nil {
		return llm.Resposta{}, err
	}
	var ultimo error
	for tentativa := 0; tentativa <= len(c.backoff); tentativa++ {
		if tentativa > 0 {
			select {
			case <-ctx.Done():
				return llm.Resposta{}, ctx.Err()
			case <-time.After(c.backoff[tentativa-1]):
			}
		}
		resp, err := c.uma(ctx, corpo)
		if err == nil {
			return resp, nil
		}
		ultimo = err
		if ctx.Err() != nil {
			return llm.Resposta{}, err
		}
		var eh *erroHTTP
		if errors.As(err, &eh) && !eh.retentavel() {
			return llm.Resposta{}, err
		}
		// erro de rede/timeout/5xx/429: tenta de novo
	}
	return llm.Resposta{}, ultimo
}

func (c *Cliente) uma(ctx context.Context, corpo []byte) (llm.Resposta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(corpo))
	if err != nil {
		return llm.Resposta{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return llm.Resposta{}, errors.New("openai: falha de rede: " + c.limpar(err.Error()))
	}
	defer resp.Body.Close()
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return llm.Resposta{}, errors.New("openai: falha ao ler resposta: " + c.limpar(err.Error()))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		txt := c.limpar(string(dados))
		if len(txt) > 500 {
			txt = txt[:500]
		}
		return llm.Resposta{}, &erroHTTP{status: resp.StatusCode, corpo: txt}
	}
	var r respostaAPI
	if err := json.Unmarshal(dados, &r); err != nil {
		return llm.Resposta{}, fmt.Errorf("openai: resposta invalida: %w", err)
	}
	out := llm.Resposta{Modelo: r.Model, TokensEntrada: r.Usage.InputTokens, TokensSaida: r.Usage.OutputTokens}
	var texto strings.Builder
	for _, it := range r.Output {
		switch it.Type {
		case "message":
			for _, ct := range it.Content {
				if ct.Type == "output_text" {
					texto.WriteString(ct.Text)
				}
			}
		case "function_call":
			args := json.RawMessage(it.Arguments)
			if len(bytes.TrimSpace(args)) == 0 {
				args = json.RawMessage("{}")
			}
			out.Chamadas = append(out.Chamadas, llm.ChamadaFerramenta{ID: it.CallID, Nome: it.Name, Argumentos: args})
		}
	}
	out.Texto = strings.TrimSpace(texto.String())
	return out, nil
}

func (c *Cliente) limpar(s string) string {
	if c.apiKey != "" {
		s = strings.ReplaceAll(s, c.apiKey, "[redigido]")
	}
	return s
}
