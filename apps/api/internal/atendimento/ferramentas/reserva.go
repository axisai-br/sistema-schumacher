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
		Descricao: "Cria a reserva de CADA trecho (viagem) escolhido que ainda nao tem reserva, depois que a viagem foi escolhida, os passageiros estao completos e o cliente decidiu como pagar: " +
			"'integral' (valor total) ou 'sinal' (valor de entrada por passageiro pagante, resto no embarque). " +
			"Ida e volta geram uma reserva por trecho, com os mesmos passageiros e a mesma forma de pagamento. " +
			"Nao use antes de ter viagem e passageiros validos. Chamar de novo nao duplica: devolve as reservas ja criadas e tenta so os trechos que faltam. " +
			"Devolve a lista por trecho, o total geral e o valor a pagar agora somado; se um trecho falhar, os outros continuam criados e a saida diz quais. Depois use gerar_pix.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"pagamento":{"type":"string","enum":["integral","sinal"],"description":"Forma de pagamento escolhida pelo cliente (vale para todos os trechos)."}},
  "required":["pagamento"],
  "additionalProperties":false
}`),
	}
}

// itemTrecho identifica o trecho nas saidas por trecho.
func itemTrecho(i int, tr conversa.Trecho) map[string]any {
	return map[string]any{"trecho": i + 1, "rota": tr.Rota(), "data": tr.Viagem.Data, "horario": tr.Viagem.Horario}
}

// itemFalha converte uma Saida de falha em item por trecho.
func itemFalha(item map[string]any, s Saida) map[string]any {
	item["ok"] = false
	item["motivo"] = s.Motivo
	if d, ok := s.Dados.(map[string]any); ok {
		if m, ok := d["mensagem"]; ok {
			item["mensagem"] = m
		}
	}
	return item
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

	if len(e.Trechos) == 0 {
		return falhaDados("dados_incompletos", map[string]any{
			"pendencias": pendenciasParaReserva(*e),
			"mensagem":   "Ainda faltam dados antes de criar a reserva.",
		})
	}
	if !e.TodosReservados() {
		if pend := pendenciasParaReserva(*e); len(pend) > 0 {
			return falhaDados("dados_incompletos", map[string]any{
				"pendencias": pend,
				"mensagem":   "Ainda faltam dados antes de criar a reserva.",
			})
		}
	}
	// A forma de pagamento vale para todos os trechos. Enquanto nenhum PIX foi
	// gerado o cliente pode trocar integral x sinal; depois do primeiro PIX a
	// forma ja cobrada e mantida.
	pixGerado := false
	for _, tr := range e.Trechos {
		if tr.PagamentoID != "" {
			pixGerado = true
		}
	}
	if !pixGerado || e.Pagamento == "" {
		e.Pagamento = pg
	}

	nPagantes := e.Pagantes()
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

	var itens []map[string]any
	var totalGeral, aPagarAgora float64
	var falhas []Saida
	var reservados []int
	for i := range e.Trechos {
		tr := &e.Trechos[i]
		item := itemTrecho(i, *tr)
		if tr.ReservaID != "" { // idempotente
			item["ok"], item["ja_existente"], item["reserva_id"] = true, true, tr.ReservaID
			if d, err := t.r.Get(ctx, tr.ReservaID); err == nil {
				preencherReserva(item, d, e.Pagamento, nPagantes, t.cfg.SinalPorPagante)
			}
			reservados = append(reservados, i+1)
		} else {
			cot, err := cotar(ctx, t.q, tr.Viagem)
			if err != nil {
				s := falha("erro_cotacao", "Nao consegui confirmar o preco agora. Tente de novo; se repetir, transfira para um atendente.")
				falhas = append(falhas, s)
				itens = append(itens, itemFalha(item, s))
				continue
			}
			if diferePreco(cot, tr.Viagem.Preco) {
				item["preco_anterior"] = tr.Viagem.Preco
				item["preco_atualizado"] = cot
				tr.Viagem.Preco = cot
			}
			total := arredondar(cot * float64(nPagantes))
			deposito, resto := total, 0.0
			if e.Pagamento == pagamentoSinal {
				deposito = valorSinal(total, nPagantes, t.cfg.SinalPorPagante)
				resto = arredondar(total - deposito)
			}
			det, err := t.r.Create(ctx, bookings.CreateBookingInput{
				TripID:          tr.Viagem.TripID,
				BoardStopID:     tr.Viagem.BoardStopID,
				AlightStopID:    tr.Viagem.AlightStopID,
				Passengers:      pax,
				IdempotencyKey:  hashReserva(c.Conversa.ID, tr.Viagem, e.Passageiros),
				Source:          &origem,
				TotalAmount:     total,
				DepositAmount:   deposito,
				RemainderAmount: resto,
			})
			var s *Saida
			switch {
			case err != nil:
				x := mapearErroReserva(err)
				s = &x
			case det.Booking.ID == "":
				x := falha("erro_ao_criar_reserva", "A reserva nao retornou identificador. Transfira para um atendente.")
				s = &x
			}
			if s != nil {
				falhas = append(falhas, *s)
				itens = append(itens, itemFalha(item, *s))
				continue
			}
			tr.ReservaID = det.Booking.ID
			item["ok"], item["ja_existente"], item["reserva_id"] = true, false, det.Booking.ID
			preencherReserva(item, det, e.Pagamento, nPagantes, t.cfg.SinalPorPagante)
			reservados = append(reservados, i+1)
		}
		if v, ok := item["total"].(float64); ok {
			totalGeral += v
		}
		if v, ok := item["valor_a_pagar_agora"].(float64); ok {
			aPagarAgora += v
		}
		itens = append(itens, item)
	}

	dados := map[string]any{"pagamento": e.Pagamento, "trechos": itens}
	if totalGeral > 0 {
		dados["total_geral"] = arredondar(totalGeral)
		dados["valor_a_pagar_agora"] = arredondar(aPagarAgora)
		dados["restante_no_embarque_total"] = arredondar(totalGeral - aPagarAgora)
	}
	if len(falhas) == 0 {
		dados["proximo_passo"] = "gerar_pix"
		return sucesso(dados)
	}
	// Erro em um trecho nao desfaz os outros: a saida diz quais deram certo.
	dados["trechos_reservados"] = reservados
	motivo := falhas[0].Motivo
	if len(reservados) > 0 {
		motivo = "reserva_parcial"
		dados["mensagem"] = "Alguns trechos foram reservados e outros falharam (veja ok/motivo em trechos). Informe o cliente com clareza; nao diga que tudo foi reservado. Gere o PIX dos trechos reservados e tente criar_reserva de novo para os que falharam, ou transfira para um atendente."
	} else if d, okd := falhas[0].Dados.(map[string]any); okd {
		dados["mensagem"] = d["mensagem"]
	}
	return falhaDados(motivo, dados)
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
		agora := valorAPagar(pagamento, b.TotalAmount, pagantes, sinal)
		dados["valor_a_pagar_agora"] = agora
		dados["restante_no_embarque"] = arredondar(b.TotalAmount - agora)
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
