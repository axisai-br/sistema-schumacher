package agente

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
)

const esquemaJuiz = `{"type":"object","properties":{"pede_humano":{"type":"number"},"irritacao":{"type":"number"},"fora_do_assunto":{"type":"number"}},"required":["pede_humano","irritacao","fora_do_assunto"],"additionalProperties":false}`

const instrucoesJuiz = `Você avalia a última mensagem do CLIENTE em uma conversa de atendimento de uma empresa de ônibus (viagens entre Maranhão e Santa Catarina). Responda só com JSON, com números de 0 a 1:
- pede_humano: o cliente pede atendente, ajuda de uma pessoa, suporte humano (1 = pede claramente).
- irritacao: quão irritado ou frustrado o cliente está (0 = calmo, 1 = muito irritado).
- fora_do_assunto: a última mensagem não tem relação com viagens de ônibus, passagens ou reservas (1 = totalmente fora).`

type juizLLM struct {
	m      llm.Modelo
	modelo string
}

// NovoJuizLLM cria um Juiz que faz uma chamada curta ao modelo com saida JSON.
func NovoJuizLLM(m llm.Modelo, modelo string) Juiz { return &juizLLM{m: m, modelo: modelo} }

func transcricao(msgs []conversa.Mensagem) string {
	var b strings.Builder
	for _, m := range msgs {
		t := strings.TrimSpace(m.Texto)
		if t == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", m.Autor, t)
	}
	return strings.TrimSpace(b.String())
}

func limitar01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (j *juizLLM) Avaliar(ctx context.Context, ultimas []conversa.Mensagem) (Avaliacao, error) {
	if len(ultimas) > 6 {
		ultimas = ultimas[len(ultimas)-6:]
	}
	resp, err := j.m.Gerar(ctx, llm.Pedido{
		Modelo:     j.modelo,
		Instrucoes: instrucoesJuiz,
		Mensagens:  []llm.Mensagem{{Papel: llm.PapelUsuario, Texto: transcricao(ultimas)}},
		SaidaJSON:  json.RawMessage(esquemaJuiz),
		MaxTokens:  100,
	})
	if err != nil {
		return Avaliacao{}, err
	}
	var out struct {
		PedeHumano    float64 `json:"pede_humano"`
		Irritacao     float64 `json:"irritacao"`
		ForaDoAssunto float64 `json:"fora_do_assunto"`
	}
	if err := json.Unmarshal([]byte(resp.Texto), &out); err != nil {
		return Avaliacao{}, fmt.Errorf("juiz llm: saida invalida: %w", err)
	}
	return Avaliacao{
		PedeHumano:    limitar01(out.PedeHumano),
		Irritacao:     limitar01(out.Irritacao),
		ForaDoAssunto: limitar01(out.ForaDoAssunto),
	}, nil
}
