package ferramentas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/bookings"
)

const (
	pagamentoIntegral = "integral"
	pagamentoSinal    = "sinal"
	origemReserva     = "WHATSAPP_V2"
)

// valorSinal: sinal por pagante (limitado ao total). O chat antigo usa R$ 250
// por pagante; bookings.Checkout exige 30% do total (divergencia de negocio).
func valorSinal(total float64, pagantes int, porPagante float64) float64 {
	return arredondar(math.Min(total, porPagante*float64(max(pagantes, 1))))
}

func valorAPagar(pagamento string, total float64, pagantes int, sinal float64) float64 {
	if pagamento == pagamentoSinal {
		return valorSinal(total, pagantes, sinal)
	}
	return arredondar(total)
}

func hashReserva(conversaID string, v conversa.Opcao, pax []conversa.Passageiro) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|", v.TripID, v.BoardStopID, v.AlightStopID)
	for _, p := range pax {
		fmt.Fprintf(h, "%s|%s|%s|%t;", strings.ToLower(p.Nome), p.Documento, p.TipoDocumento, p.CriancaAte5)
	}
	return "atdv2:" + conversaID + ":" + hex.EncodeToString(h.Sum(nil))[:16]
}

type criarReserva struct {
	r   Reservas
	q   Cotador
	cfg Config
}

func (t *criarReserva) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "criar_reserva",
		Descricao: "Cria a reserva depois que a viagem foi escolhida, os passageiros estao completos e o cliente decidiu como pagar: " +
			"'integral' (valor total) ou 'sinal' (valor de entrada por passageiro pagante, resto no embarque). " +
			"Nao use antes de ter viagem e passageiros validos. Chamar de novo devolve a mesma reserva. Depois use gerar_pix.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"pagamento":{"type":"string","enum":["integral","sinal"],"description":"Forma de pagamento escolhida pelo cliente."}},
  "required":["pagamento"],
  "additionalProperties":false
}`),
	}
}

func (t *criarReserva) Executar(ctx context.Context, c *Contexto, raw json.RawMessage) Saida {
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

	if e.ReservaID != "" { // idempotente
		if e.PagamentoID == "" {
			e.Pagamento = pg // ainda sem PIX: o cliente pode trocar integral x sinal
		}
		dados := map[string]any{"ja_existente": true, "reserva_id": e.ReservaID, "pagamento": e.Pagamento}
		if d, err := t.r.Get(ctx, e.ReservaID); err == nil {
			preencherReserva(dados, d, e.Pagamento, e.Pagantes(), t.cfg.SinalPorPagante)
		}
		return sucesso(dados)
	}

	if pend := pendenciasParaReserva(*e); len(pend) > 0 {
		return falhaDados("dados_incompletos", map[string]any{
			"pendencias": pend,
			"mensagem":   "Ainda faltam dados antes de criar a reserva.",
		})
	}
	v := e.Viagem
	pagantes := e.Pagantes()
	cot, err := cotar(ctx, t.q, *v)
	if err != nil {
		return falha("erro_cotacao", "Nao consegui confirmar o preco agora. Tente de novo; se repetir, transfira para um atendente.")
	}
	avisos := map[string]any{}
	if diferePreco(cot, v.Preco) {
		avisos["preco_anterior"] = v.Preco
		avisos["preco_atualizado"] = cot
		v.Preco = cot
	}
	total := arredondar(cot * float64(pagantes))
	deposito, resto := total, 0.0
	if pg == pagamentoSinal {
		deposito = valorSinal(total, pagantes, t.cfg.SinalPorPagante)
		resto = arredondar(total - deposito)
	}
	telefone := apenasDigitos(c.Conversa.Telefone)
	pax := make([]bookings.PassengerInput, 0, len(e.Passageiros))
	for _, p := range e.Passageiros {
		pax = append(pax, bookings.PassengerInput{
			Name:         p.Nome,
			Document:     p.Documento,
			DocumentType: p.TipoDocumento,
			Phone:        telefone,
			IsLapChild:   p.CriancaAte5,
		})
	}
	origem := origemReserva
	det, err := t.r.Create(ctx, bookings.CreateBookingInput{
		TripID:          v.TripID,
		BoardStopID:     v.BoardStopID,
		AlightStopID:    v.AlightStopID,
		Passengers:      pax,
		IdempotencyKey:  hashReserva(c.Conversa.ID, *v, e.Passageiros),
		Source:          &origem,
		TotalAmount:     total,
		DepositAmount:   deposito,
		RemainderAmount: resto,
	})
	if err != nil {
		return mapearErroReserva(err)
	}
	if det.Booking.ID == "" {
		return falha("erro_ao_criar_reserva", "A reserva nao retornou identificador. Transfira para um atendente.")
	}
	e.ReservaID = det.Booking.ID
	e.Pagamento = pg
	dados := map[string]any{"ja_existente": false, "reserva_id": det.Booking.ID, "pagamento": pg}
	preencherReserva(dados, det, pg, pagantes, t.cfg.SinalPorPagante)
	dados["proximo_passo"] = "gerar_pix"
	for k, val := range avisos {
		dados[k] = val
	}
	return sucesso(dados)
}

func preencherReserva(dados map[string]any, d bookings.BookingDetails, pagamento string, pagantes int, sinal float64) {
	b := d.Booking
	if b.ReservationCode != "" {
		dados["codigo_reserva"] = b.ReservationCode
	}
	if b.Status != "" {
		dados["status"] = b.Status
	}
	if b.TotalAmount > 0 {
		dados["total"] = b.TotalAmount
		dados["valor_a_pagar_agora"] = valorAPagar(pagamento, b.TotalAmount, pagantes, sinal)
	}
	if b.ExpiresAt != nil {
		dados["reservada_ate"] = b.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
}

// mapearErroReserva traduz erros de bookings em motivos legiveis (como
// chat/booking_create_tool.go).
func mapearErroReserva(err error) Saida {
	switch {
	case errors.Is(err, bookings.ErrNoSeatsAvailable):
		return falha("sem_vagas", "Nao ha mais vagas suficientes nessa viagem. Informe o cliente e rode buscar_viagens para oferecer outra opcao.")
	case errors.Is(err, bookings.ErrSeatNotInTrip):
		return falha("opcao_inconsistente", "A viagem escolhida ficou inconsistente. Rode buscar_viagens e peca nova escolha.")
	case errors.Is(err, bookings.ErrPassengerNameRequired):
		return falha("nome_passageiro_obrigatorio", "Todo passageiro precisa de nome. Corrija com registrar_passageiros.")
	case errors.Is(err, bookings.ErrPassengerDocumentType):
		return falha("tipo_documento_invalido", "Tipo de documento invalido (use CPF, RG ou CNH). Corrija com registrar_passageiros.")
	case errors.Is(err, bookings.ErrMissingFields), errors.Is(err, bookings.ErrMissingStops):
		return falha("dados_incompletos", "Faltam dados da viagem ou dos passageiros. Refaca a escolha da viagem e confirme os passageiros.")
	case errors.Is(err, bookings.ErrSeatRequiresSinglePassenger),
		errors.Is(err, bookings.ErrNegativeAmounts),
		errors.Is(err, bookings.ErrInvalidAmounts):
		return falha("valores_invalidos", "Os valores da reserva nao fecharam. Nao tente de novo sem corrigir; transfira para um atendente se persistir.")
	default:
		return falha("erro_ao_criar_reserva", "Nao consegui criar a reserva agora. Nao confirme nada ao cliente; tente uma vez mais ou transfira para um atendente.")
	}
}
