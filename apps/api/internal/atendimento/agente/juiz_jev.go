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

// respostaJev e a resposta de uma pergunta do Jev (noul, score ou choice).
type respostaJev struct {
	Tipo          string             `json:"type"`
	Noul          *float64           `json:"noul"`
	Score         *float64           `json:"score"`
	Choice        string             `json:"choice"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// clienteJev e o cliente HTTP do TypeSafe System One (compartilhado pelo juiz e
// pelo roteador): uma requisicao com state + questions, com 1 retry em 429/529.
type clienteJev struct {
	apiKey string
	http   *http.Client
	url    string
	espera time.Duration
}

func novoClienteJev(apiKey string, hc *http.Client) *clienteJev {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &clienteJev{apiKey: strings.TrimSpace(apiKey), http: hc, url: urlJev, espera: 500 * time.Millisecond}
}

// avaliar envia state e questions ao Jev e devolve as respostas por id.
func (c *clienteJev) avaliar(ctx context.Context, state any, questions map[string]any) (map[string]respostaJev, error) {
	if c.apiKey == "" {
		return nil, errors.New("jev: chave de API nao configurada")
	}
	corpo, err := json.Marshal(map[string]any{
		"state":     state,
		"model":     "jev-latest",
		"questions": questions,
	})
	if err != nil {
		return nil, err
	}
	var dados []byte
	for tentativa := 0; tentativa < 2; tentativa++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(corpo))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("jev: falha de rede")
		}
		dados, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return nil, errors.New("jev: falha ao ler resposta")
		}
		if (resp.StatusCode == 429 || resp.StatusCode == 529) && tentativa == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.espera):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("jev: status %d", resp.StatusCode)
		}
		break
	}
	var r struct {
		Answers map[string]respostaJev `json:"answers"`
	}
	if err := json.Unmarshal(dados, &r); err != nil {
		return nil, fmt.Errorf("jev: resposta invalida: %w", err)
	}
	return r.Answers, nil
}

type juizJev struct{ c *clienteJev }

// NovoJuizJev cria um Juiz que usa o Jev (TypeSafe). http nil usa um cliente
// com timeout de 10s.
func NovoJuizJev(apiKey string, hc *http.Client) Juiz {
	return &juizJev{c: novoClienteJev(apiKey, hc)}
}

func (j *juizJev) Avaliar(ctx context.Context, ultimas []conversa.Mensagem) (Avaliacao, error) {
	if len(ultimas) > 6 {
		ultimas = ultimas[len(ultimas)-6:]
	}
	resp, err := j.c.avaliar(ctx, transcricao(ultimas), map[string]any{
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
	})
	if err != nil {
		return Avaliacao{}, err
	}
	var av Avaliacao
	if a, ok := resp["pede_humano"]; ok && a.Noul != nil {
		av.PedeHumano = limitar01(*a.Noul)
	}
	if a, ok := resp["fora_do_assunto"]; ok && a.Noul != nil {
		av.ForaDoAssunto = limitar01(*a.Noul)
	}
	if a, ok := resp["irritacao"]; ok && a.Score != nil {
		av.Irritacao = limitar01(*a.Score / 2)
	}
	return av, nil
}
