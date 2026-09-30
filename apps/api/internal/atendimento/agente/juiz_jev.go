package agente

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

	"schumacher-tur/api/internal/atendimento/conversa"
)

const urlJev = "https://api.typesafe.ai/v1/systemone"

type juizJev struct {
	apiKey string
	http   *http.Client
	url    string
	espera time.Duration
}

// NovoJuizJev cria um Juiz que usa o Jev (TypeSafe). http nil usa um cliente
// com timeout de 10s.
func NovoJuizJev(apiKey string, hc *http.Client) Juiz {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &juizJev{apiKey: strings.TrimSpace(apiKey), http: hc, url: urlJev, espera: 500 * time.Millisecond}
}

func (j *juizJev) Avaliar(ctx context.Context, ultimas []conversa.Mensagem) (Avaliacao, error) {
	if j.apiKey == "" {
		return Avaliacao{}, errors.New("jev: chave de API nao configurada")
	}
	if len(ultimas) > 6 {
		ultimas = ultimas[len(ultimas)-6:]
	}
	corpo, err := json.Marshal(map[string]any{
		"state": transcricao(ultimas),
		"model": "jev-latest",
		"questions": map[string]any{
			"pede_humano": map[string]any{
				"type":         "noul",
				"instructions": "O cliente está pedindo para falar com um atendente humano ou pedindo ajuda de uma pessoa?",
			},
			"irritacao": map[string]any{
				"type":         "score",
				"instructions": "Quão irritado está o cliente na última mensagem?",
				"criteria":     []string{"Calmo", "Incomodado", "Irritado"},
			},
			"fora_do_assunto": map[string]any{
				"type":         "noul",
				"instructions": "A última mensagem do cliente é sobre um assunto que não tem relação com viagens de ônibus, passagens ou reservas?",
			},
		},
	})
	if err != nil {
		return Avaliacao{}, err
	}
	var dados []byte
	for tentativa := 0; tentativa < 2; tentativa++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.url, bytes.NewReader(corpo))
		if err != nil {
			return Avaliacao{}, err
		}
		req.Header.Set("Authorization", "Bearer "+j.apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := j.http.Do(req)
		if err != nil {
			return Avaliacao{}, errors.New("jev: falha de rede")
		}
		dados, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return Avaliacao{}, errors.New("jev: falha ao ler resposta")
		}
		if (resp.StatusCode == 429 || resp.StatusCode == 529) && tentativa == 0 {
			select {
			case <-ctx.Done():
				return Avaliacao{}, ctx.Err()
			case <-time.After(j.espera):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return Avaliacao{}, fmt.Errorf("jev: status %d", resp.StatusCode)
		}
		break
	}
	var r struct {
		Answers map[string]struct {
			Noul  *float64 `json:"noul"`
			Score *float64 `json:"score"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(dados, &r); err != nil {
		return Avaliacao{}, fmt.Errorf("jev: resposta invalida: %w", err)
	}
	var av Avaliacao
	if a, ok := r.Answers["pede_humano"]; ok && a.Noul != nil {
		av.PedeHumano = limitar01(*a.Noul)
	}
	if a, ok := r.Answers["fora_do_assunto"]; ok && a.Noul != nil {
		av.ForaDoAssunto = limitar01(*a.Noul)
	}
	if a, ok := r.Answers["irritacao"]; ok && a.Score != nil {
		av.Irritacao = limitar01(*a.Score / 2)
	}
	return av, nil
}
