package ferramentas

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/payments"
)

const validadePix = time.Hour // payments.Service cria o PIX com expires_in=3600

type gerarPix struct {
	r   Reservas
	p   Pagamentos
	cfg Config
}

func (t *gerarPix) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "gerar_pix",
		Descricao: "Gera o PIX copia-e-cola da reserva ja criada (valor integral ou sinal, conforme escolhido em criar_reserva). " +
			"Use logo apos criar_reserva ou quando o cliente pedir o PIX de novo: se ja houver PIX pendente, devolve o mesmo codigo sem criar outra cobranca. " +
			"O PIX exige CPF do pagador: se nenhum passageiro adulto informou CPF, peca o CPF de quem vai pagar e envie em cpf_pagador.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"cpf_pagador":{"type":"string","description":"CPF de quem paga, so se nenhum adulto informou CPF."}},
  "additionalProperties":false
}`),
	}
}

func statusPendente(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PENDING", "WAITING", "WAITING_PAYMENT", "PROCESSING", "CREATED", "OPEN":
		return true
	}
	return false
}

func statusPago(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PAID", "CONFIRMED", "CAPTURED":
		return true
	}
	return false
}

// extrairPix le o copia-e-cola e a expiracao do retorno do provedor, tanto do
// JSON bruto da criacao quanto do metadata {"order": ...} gravado no pagamento.
func extrairPix(raw json.RawMessage) (codigo string, expira *time.Time) {
	_, codigo = payments.ExtractCheckoutAndPix(raw)
	if codigo == "" {
		var w struct {
			Order json.RawMessage `json:"order"`
		}
		if json.Unmarshal(raw, &w) == nil && len(w.Order) > 0 {
			_, codigo = payments.ExtractCheckoutAndPix(w.Order)
		}
	}
	var any1 any
	if json.Unmarshal(raw, &any1) == nil {
		expira = buscarExpiracao(any1, 0)
	}
	return strings.TrimSpace(codigo), expira
}

func buscarExpiracao(v any, prof int) *time.Time {
	if prof > 8 {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x["expires_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return &t
			}
		}
		for _, k := range []string{"order", "charges", "last_transaction", "data"} {
			if sub, ok := x[k]; ok {
				if t := buscarExpiracao(sub, prof+1); t != nil {
					return t
				}
			}
		}
	case []any:
		for _, it := range x {
			if t := buscarExpiracao(it, prof+1); t != nil {
				return t
			}
		}
	}
	return nil
}

func dadosPix(valor float64, codigo string, expira *time.Time, t0 time.Time) map[string]any {
	d := map[string]any{"valor": valor, "pix_copia_e_cola": codigo}
	if expira != nil {
		d["expira_em"] = expira.Format("2006-01-02T15:04:05Z07:00")
	} else if !t0.IsZero() {
		d["expira_em"] = t0.Add(validadePix).Format("2006-01-02T15:04:05Z07:00")
	}
	return d
}

func (t *gerarPix) Executar(ctx context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		CPFPagador string `json:"cpf_pagador"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	e := c.Estado
	if e.ReservaID == "" {
		return falha("reserva_nao_criada", "Crie a reserva com criar_reserva antes de gerar o PIX.")
	}
	det, err := t.r.Get(ctx, e.ReservaID)
	if err != nil {
		return falha("erro_consultar_reserva", "Nao consegui consultar a reserva agora. Tente de novo ou transfira para um atendente.")
	}
	switch strings.ToUpper(strings.TrimSpace(det.Booking.Status)) {
	case "CANCELLED", "EXPIRED":
		return falha("reserva_cancelada_ou_expirada", "Essa reserva esta cancelada ou expirada; nao gere PIX. Transfira para um atendente.")
	}
	pagamento := e.Pagamento
	if pagamento == "" {
		pagamento = pagamentoSinal
	}
	total := arredondar(det.Booking.TotalAmount)
	valor := valorAPagar(pagamento, total, e.Pagantes(), t.cfg.SinalPorPagante)

	// Reaproveita cobranca pendente.
	if e.PagamentoID != "" {
		st, err := t.p.GetStatus(ctx, e.PagamentoID)
		if err != nil {
			return falha("erro_consultar_pagamento", "Nao consegui consultar o pagamento agora. Nao gere outra cobranca; tente de novo ou transfira para um atendente.")
		}
		switch {
		case statusPago(st.Status):
			return sucesso(map[string]any{"status": "pago", "mensagem": "Esse pagamento ja foi confirmado. Nao gere outro PIX."})
		case statusPendente(st.Status):
			codigo, expira := extrairPix(st.Metadata)
			var criado time.Time
			if lista, err := t.p.List(ctx, payments.PaymentListFilter{BookingID: e.ReservaID, Limit: 20}); err == nil {
				for _, p := range lista {
					if p.ID == e.PagamentoID {
						criado = p.CreatedAt
					}
				}
			}
			if expira == nil && !criado.IsZero() {
				x := criado.Add(validadePix)
				expira = &x
			}
			vigente := expira == nil || c.Agora.Before(*expira)
			if codigo == "" && vigente {
				return falha("pix_indisponivel", "O PIX existe mas o codigo nao esta disponivel. Nao invente codigo; transfira para um atendente.")
			}
			if vigente {
				d := dadosPix(st.Amount, codigo, expira, time.Time{})
				d["reaproveitado"] = true
				return sucesso(d)
			}
			// PIX pendente vencido: gera novo abaixo.
		}
	}

	pagador, falhaPagador := escolherPagador(e.Passageiros, a.CPFPagador)
	if falhaPagador != nil {
		return *falhaPagador
	}
	telefone := apenasDigitos(c.Conversa.Telefone)
	if len(telefone) < 10 {
		return falha("telefone_pagador_necessario", "Preciso de um telefone com DDD do pagador para gerar o PIX. Peca ao cliente.")
	}
	rotulo := firstNonEmpty(det.Booking.ReservationCode, det.Booking.ID)
	pay, rawPix, err := t.p.Create(ctx, payments.CreatePaymentInput{
		BookingID:   e.ReservaID,
		Amount:      valor,
		Method:      "PIX",
		Description: "Pagamento " + pagamento + " reserva " + rotulo,
		Customer:    &payments.CustomerInput{Name: pagador.nome, Phone: telefone, Document: pagador.cpf},
	})
	if err != nil {
		return falha("erro_gerar_pix", "Nao consegui gerar o PIX agora. Nao invente codigo; tente uma vez mais ou transfira para um atendente.")
	}
	e.PagamentoID = pay.ID
	codigo, expira := extrairPix(rawPix)
	if codigo == "" {
		return falha("pix_indisponivel", "A cobranca foi criada mas o codigo PIX nao veio. Nao invente codigo; transfira para um atendente.")
	}
	criado := pay.CreatedAt
	if criado.IsZero() {
		criado = c.Agora
	}
	return sucesso(dadosPix(valor, codigo, expira, criado))
}

type pagadorPix struct{ nome, cpf string }

// escolherPagador: primeiro adulto com CPF; senao o CPF informado pelo cliente.
func escolherPagador(pax []conversa.Passageiro, cpfInformado string) (pagadorPix, *Saida) {
	for _, p := range pax {
		if !p.CriancaAte5 && p.TipoDocumento == string(DocCPF) && ValidarCPF(p.Documento) {
			return pagadorPix{p.Nome, apenasDigitos(p.Documento)}, nil
		}
	}
	nome := ""
	for _, p := range pax {
		if !p.CriancaAte5 {
			nome = p.Nome
			break
		}
	}
	if nome == "" && len(pax) > 0 {
		nome = pax[0].Nome
	}
	if ValidarCPF(cpfInformado) {
		return pagadorPix{firstNonEmpty(nome, "Cliente Schumacher"), apenasDigitos(cpfInformado)}, nil
	}
	msg := "O PIX exige CPF do pagador e nenhum passageiro adulto informou CPF. Peca o CPF de quem vai pagar e chame gerar_pix com cpf_pagador."
	if strings.TrimSpace(cpfInformado) != "" {
		msg = "O cpf_pagador informado e invalido. Peca o CPF de novo (11 digitos)."
	}
	s := falha("cpf_do_pagador_necessario", msg)
	return pagadorPix{}, &s
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
