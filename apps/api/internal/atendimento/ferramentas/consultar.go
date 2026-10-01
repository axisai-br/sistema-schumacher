package ferramentas

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
)

type consultarReserva struct {
	r Reservas
	p Pagamentos
}

func (t *consultarReserva) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "consultar_reserva",
		Descricao: "Consulta o status das reservas e dos pagamentos. Use quando o cliente perguntar se a reserva/pagamento esta confirmado ou pedir informacoes de uma reserva. " +
			"Sem codigo, consulta TODAS as reservas desta conversa (uma por trecho: ida, volta...); para outra reserva, peca o codigo da reserva.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"codigo":{"type":"string","description":"Codigo da reserva (opcional)."}},
  "additionalProperties":false
}`),
	}
}

func statusReservaPT(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CONFIRMED":
		return "confirmada"
	case "PENDING":
		return "aguardando pagamento"
	case "CANCELLED":
		return "cancelada"
	case "EXPIRED":
		return "expirada"
	}
	return strings.ToLower(s)
}

func statusPagamentoPT(s string) string {
	switch {
	case statusPago(s):
		return "pago"
	case statusPendente(s):
		return "pendente"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func (t *consultarReserva) Executar(ctx context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Codigo string `json:"codigo"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	e := c.Estado
	codigo := strings.ToUpper(strings.TrimSpace(a.Codigo))

	if codigo == "" {
		// Sem codigo: todas as reservas dos trechos desta conversa.
		var itens []map[string]any
		for i, tr := range e.Trechos {
			if tr.ReservaID == "" {
				continue
			}
			d, s := t.consultarUma(ctx, tr.ReservaID, true, tr.PagamentoID)
			if s != nil {
				return *s
			}
			item := itemTrecho(i, tr)
			for k, v := range d {
				item[k] = v
			}
			itens = append(itens, item)
		}
		if len(itens) == 0 {
			// bookings.ListFilter nao filtra por telefone: sem codigo nao ha como localizar.
			return falha("informe_o_codigo", "Nao ha reserva nesta conversa. Peca o codigo da reserva ao cliente.")
		}
		return sucesso(map[string]any{"reservas": itens, "total_reservas": len(itens)})
	}

	itensLista, err := t.r.List(ctx, bookings.ListFilter{ReservationCode: codigo, Limit: 1})
	if err != nil {
		return falha("erro_consulta", "Nao consegui consultar agora. Tente de novo ou transfira para um atendente.")
	}
	if len(itensLista) == 0 {
		return falha("reserva_nao_encontrada", "Nao achei reserva com esse codigo. Peca para o cliente conferir o codigo.")
	}
	id := itensLista[0].ID
	pagID := ""
	proprio := false
	for _, tr := range e.Trechos {
		if tr.ReservaID == id {
			proprio, pagID = true, tr.PagamentoID
		}
	}
	d, s := t.consultarUma(ctx, id, proprio, pagID)
	if s != nil {
		return *s
	}
	return sucesso(d)
}

// consultarUma devolve os dados de uma reserva e do pagamento dela. proprio
// indica reserva desta conversa (so ela mostra os nomes dos passageiros);
// pagamentoID e o PIX gravado no trecho, quando houver.
func (t *consultarReserva) consultarUma(ctx context.Context, id string, proprio bool, pagamentoID string) (map[string]any, *Saida) {
	det, err := t.r.Get(ctx, id)
	if err != nil {
		s := falha("erro_consulta", "Nao consegui consultar agora. Tente de novo ou transfira para um atendente.")
		if errors.Is(err, pgx.ErrNoRows) {
			s = falha("reserva_nao_encontrada", "Nao achei essa reserva.")
		}
		return nil, &s
	}
	b := det.Booking
	dados := map[string]any{
		"codigo_reserva":         b.ReservationCode,
		"status":                 b.Status,
		"status_legivel":         statusReservaPT(b.Status),
		"total":                  b.TotalAmount,
		"valor_pago":             b.DepositAmount,
		"valor_restante":         b.RemainderAmount,
		"quantidade_passageiros": len(det.Passengers),
	}
	if b.ExpiresAt != nil {
		dados["reservada_ate"] = b.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if proprio {
		nomes := make([]string, 0, len(det.Passengers))
		for _, p := range det.Passengers {
			nomes = append(nomes, p.Name)
		}
		dados["passageiros"] = nomes
	}

	var pag *payments.PaymentStatusResponse
	if proprio && pagamentoID != "" {
		if st, err := t.p.GetStatus(ctx, pagamentoID); err == nil {
			pag = &st
		}
	}
	if pag == nil {
		if lista, err := t.p.List(ctx, payments.PaymentListFilter{BookingID: id, Limit: 20}); err == nil && len(lista) > 0 {
			escolhido := lista[0]
			for _, p := range lista {
				if statusPago(p.Status) {
					escolhido = p
					break
				}
			}
			pag = &payments.PaymentStatusResponse{ID: escolhido.ID, Status: escolhido.Status, Amount: escolhido.Amount}
		}
	}
	if pag != nil {
		dados["pagamento"] = map[string]any{
			"status":         pag.Status,
			"status_legivel": statusPagamentoPT(pag.Status),
			"valor":          pag.Amount,
		}
	} else {
		dados["pagamento"] = map[string]any{"status_legivel": "nenhum pagamento registrado"}
	}
	return dados, nil
}
