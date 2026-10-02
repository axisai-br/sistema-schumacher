package ferramentas

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/payments"
)

// trocarPagamento troca integral <-> sinal depois da reserva: confere que
// nenhum PIX foi pago, cancela os pendentes, ajusta o valor que confirma cada
// reserva e gera os PIX novos.
type trocarPagamento struct {
	r   Reservas
	p   Pagamentos
	pix *gerarPix
	cfg Config
}

func (t *trocarPagamento) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "trocar_pagamento",
		Descricao: "Troca a forma de pagamento (integral <-> sinal) DEPOIS que a reserva foi criada. Confere que nenhum PIX foi pago, " +
			"cancela o PIX pendente e gera o novo com o valor da nova forma (devolve os PIX como gerar_pix). " +
			"Se algum PIX ja foi pago, nao troca: transfira para um atendente. Antes da reserva use criar_reserva com a nova forma.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"pagamento":{"type":"string","enum":["integral","sinal"],"description":"Nova forma de pagamento."}},
  "required":["pagamento"],
  "additionalProperties":false
}`),
	}
}

func (t *trocarPagamento) Executar(ctx context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Pagamento string `json:"pagamento"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	pg := strings.ToLower(strings.TrimSpace(a.Pagamento))
	if pg != pagamentoIntegral && pg != pagamentoSinal {
		return falha("pagamento_invalido", "Use pagamento 'integral' ou 'sinal'.")
	}
	e := c.Estado
	if !e.AlgumReservado() {
		return falha("reserva_nao_criada", "Ainda nao ha reserva: use criar_reserva com a nova forma de pagamento.")
	}
	if pg == e.Pagamento {
		return falha("mesma_forma", "O pagamento ja esta nessa forma. Se o cliente quer o PIX de novo, use gerar_pix.")
	}
	jaPago := func(i int) Saida {
		return falhaDados("ja_pago", map[string]any{
			"trecho":   i + 1,
			"mensagem": "Esse PIX ja foi pago, entao a forma de pagamento nao pode ser trocada por aqui. Explique e transfira para um atendente.",
		})
	}
	// 1) Nenhum PIX pago (conferido antes de cancelar qualquer um).
	for i, tr := range e.Trechos {
		if tr.PagamentoID == "" {
			continue
		}
		st, err := t.p.GetStatus(ctx, tr.PagamentoID)
		if err != nil {
			return falha("erro_consultar_pagamento", "Nao consegui consultar o pagamento agora. Nao troque nada; tente de novo ou transfira para um atendente.")
		}
		if statusPago(st.Status) {
			return jaPago(i)
		}
	}
	// 2) Cancela os pendentes (o servico confere de novo no provedor).
	anterior := e.Pagamento
	for i := range e.Trechos {
		tr := &e.Trechos[i]
		if tr.PagamentoID == "" {
			continue
		}
		if err := t.p.CancelarPendente(ctx, tr.PagamentoID); err != nil {
			if errors.Is(err, payments.ErrPagamentoJaPago) {
				return jaPago(i)
			}
			return falha("erro_cancelar_pix", "Nao consegui cancelar o PIX anterior agora. Nao gere outro; tente de novo uma vez ou transfira para um atendente.")
		}
		tr.PagamentoID = ""
	}
	// 3) Valor que confirma cada reserva (sinal ou total).
	e.Pagamento = pg
	for _, tr := range e.Trechos {
		if tr.ReservaID == "" {
			continue
		}
		det, err := t.r.Get(ctx, tr.ReservaID)
		if err != nil {
			return falha("erro_consultar_reserva", "Nao consegui consultar a reserva agora. Tente de novo ou transfira para um atendente.")
		}
		valor := valorAPagar(pg, arredondar(det.Booking.TotalAmount), e.Pagantes(), t.cfg.SinalPorPagante)
		if err := t.p.DefinirSinalReserva(ctx, tr.ReservaID, valor); err != nil {
			return falha("erro_ajustar_reserva", "Nao consegui ajustar o valor da reserva agora. Transfira para um atendente.")
		}
	}
	// 4) PIX novos.
	s := t.pix.Executar(ctx, c, json.RawMessage(`{}`))
	if d, ok := s.Dados.(map[string]any); ok {
		d["pagamento"], d["pagamento_anterior"] = pg, anterior
		if s.OK {
			total := 0.0
			for _, tr := range e.Trechos {
				if det, err := t.r.Get(ctx, tr.ReservaID); err == nil {
					total += det.Booking.TotalAmount
				}
			}
			if v, ok := d["total"].(float64); ok && total > v {
				d["restante_no_embarque_total"] = arredondar(total - v)
			}
		}
	}
	return s
}
