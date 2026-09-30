package ferramentas

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/bookings"
)

var errBoom = errors.New("boom")

const (
	cpfA = "529.982.247-25"
	cpfB = "111.444.777-35"
)

func TestValidarCPF(t *testing.T) {
	if !ValidarCPF(cpfA) || !ValidarCPF(cpfB) {
		t.Fatal("CPFs de teste deveriam ser validos")
	}
	for _, ruim := range []string{"529.982.247-24", "111.111.111-11", "123", ""} {
		if ValidarCPF(ruim) {
			t.Errorf("%q deveria ser invalido", ruim)
		}
	}
}

func TestClassificarDocumento(t *testing.T) {
	casos := []struct {
		doc, tipo string
		want      TipoDocumento
		norm      string
		codigo    string
	}{
		{cpfA, "", DocCPF, "52998224725", ""},
		{"52998224724", "", "", "", "cpf_invalido"},
		{"12.345.678-9", "", DocRG, "123456789", ""},
		{"MG-12.345.678", "", DocRG, "MG12345678", ""},
		{"12345678900", "CNH", DocCNH, "12345678900", ""},
		{"1234", "", "", "", "documento_invalido"},
		{"123456789012345", "", "", "", "documento_invalido"},
		{"12345", "", DocRG, "12345", ""},
		{"ab#123", "", "", "", "documento_invalido"},
		{"123", "XYZ", "", "", "tipo_documento_invalido"},
	}
	for _, c := range casos {
		tipo, norm, codigo, _ := ClassificarDocumento(c.doc, c.tipo)
		if tipo != c.want || norm != c.norm || codigo != c.codigo {
			t.Errorf("%q/%q: got (%s,%s,%s) want (%s,%s,%s)", c.doc, c.tipo, tipo, norm, codigo, c.want, c.norm, c.codigo)
		}
	}
}

func TestRegistrarPassageiros(t *testing.T) {
	a := novoAmbiente(t)
	// CPF invalido
	s, m := a.exec(t, "registrar_passageiros", `{"passageiros":[{"nome":"Maria Silva","documento":"529.982.247-24"}]}`)
	if s.OK || s.Motivo != "dados_invalidos" {
		t.Fatalf("%+v", m)
	}
	erros := dadosDe(m)["erros"].([]any)
	if erros[0].(map[string]any)["motivo"] != "cpf_invalido" {
		t.Fatalf("erros: %+v", erros)
	}
	if len(a.ctx.Estado.Passageiros) != 0 {
		t.Fatal("nao deveria gravar nada com erro")
	}
	// CPF valido + RG + crianca sem documento
	s, m = a.exec(t, "registrar_passageiros", `{"passageiros":[
		{"nome":"  Maria   Silva ","documento":"`+cpfA+`"},
		{"nome":"João Souza","documento":"12.345.678-9"},
		{"nome":"Pedrinho Silva","crianca_ate_5":true}]}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	ps := a.ctx.Estado.Passageiros
	if len(ps) != 3 || ps[0].Nome != "Maria Silva" || ps[0].TipoDocumento != "CPF" || ps[0].Documento != "52998224725" ||
		ps[1].TipoDocumento != "RG" || ps[2].Documento != "" || !ps[2].CriancaAte5 {
		t.Fatalf("passageiros: %+v", ps)
	}
	d := dadosDe(m)
	if d["pagantes"] != float64(2) || d["total"] != float64(3) {
		t.Fatalf("dados: %+v", d)
	}
	lista := d["passageiros"].([]any)
	if lista[0].(map[string]any)["documento"] != "***725" {
		t.Fatalf("documento deveria vir mascarado: %+v", lista[0])
	}
	if strings.Contains(fmt.Sprint(m), "52998224725") {
		t.Fatal("documento completo vazou na saida")
	}
	for _, p := range d["pendencias"].([]any) {
		if strings.HasPrefix(p.(string), "completar dados") {
			t.Fatalf("crianca sem documento nao deveria gerar pendencia: %+v", d["pendencias"])
		}
	}
}

func TestRegistrarPassageirosErros(t *testing.T) {
	a := novoAmbiente(t)
	casos := map[string]string{
		`{"passageiros":[{"nome":"Maria","documento":"` + cpfA + `"}]}`:                                                      "nome_invalido",
		`{"passageiros":[{"nome":"Maria Silva"}]}`:                                                                           "documento_obrigatorio",
		`{"passageiros":[{"nome":"Maria Silva","documento":"12"}]}`:                                                          "documento_invalido",
		`{"passageiros":[{"nome":"Maria Silva","documento":"` + cpfA + `"},{"nome":"Ana Lima","documento":"` + cpfA + `"}]}`: "documento_duplicado",
		`{"passageiros":[{"nome":"Maria Silva","documento":"12345678900","tipo_documento":"CNH"}]}`:                          "",
	}
	for args, quer := range casos {
		s, m := a.exec(t, "registrar_passageiros", args)
		if quer == "" {
			if !s.OK {
				t.Errorf("CNH deveria passar: %+v", m)
			}
			continue
		}
		erros, _ := dadosDe(m)["erros"].([]any)
		if s.OK || len(erros) == 0 || erros[len(erros)-1].(map[string]any)["motivo"] != quer {
			t.Errorf("%s: got %+v", quer, m)
		}
	}
	if s, _ := a.exec(t, "registrar_passageiros", `{"passageiros":[]}`); s.Motivo != "lista_vazia" {
		t.Fatalf("%+v", s)
	}
}

func TestRegistrarPassageirosAcimaDasVagas(t *testing.T) {
	a := novoAmbiente(t)
	a.ctx.Estado.Viagem = &conversa.Opcao{TripID: "t3", Vagas: 1}
	s, m := a.exec(t, "registrar_passageiros", `{"passageiros":[{"nome":"Maria Silva","documento":"`+cpfA+`"},{"nome":"Ana Lima","documento":"`+cpfB+`"}]}`)
	if s.OK || s.Motivo != "passageiros_acima_das_vagas" {
		t.Fatalf("%+v", m)
	}
}

// ---- reserva ----

func prepararReserva(t *testing.T, a *ambiente) {
	t.Helper()
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	if s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`); !s.OK {
		t.Fatalf("%+v", m)
	}
	if s, m := a.exec(t, "registrar_passageiros", `{"passageiros":[
		{"nome":"Maria Silva","documento":"`+cpfA+`"},
		{"nome":"João Souza","documento":"12.345.678-9"},
		{"nome":"Pedrinho Silva","crianca_ate_5":true}]}`); !s.OK {
		t.Fatalf("%+v", m)
	}
}

func TestCriarReservaIncompleta(t *testing.T) {
	a := novoAmbiente(t)
	s, m := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if s.OK || s.Motivo != "dados_incompletos" {
		t.Fatalf("%+v", m)
	}
	pend := dadosDe(m)["pendencias"].([]any)
	if len(pend) != 2 || pend[0] != "escolher viagem" {
		t.Fatalf("pendencias: %+v", pend)
	}
	if len(a.r.criadas) != 0 {
		t.Fatal("nao deveria chamar Create")
	}
	if s, _ := a.exec(t, "criar_reserva", `{"pagamento":"boleto"}`); s.Motivo != "pagamento_invalido" {
		t.Fatalf("%+v", s)
	}
	// so criancas
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	a.exec(t, "escolher_viagem", `{"opcao":1}`)
	a.exec(t, "registrar_passageiros", `{"passageiros":[{"nome":"Pedrinho Silva","crianca_ate_5":true}]}`)
	if s, m := a.exec(t, "criar_reserva", `{"pagamento":"integral"}`); s.OK {
		t.Fatalf("so crianca nao deveria reservar: %+v", m)
	}
}

func TestCriarReservaSinalEIdempotente(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	s, m := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	if len(a.r.criadas) != 1 {
		t.Fatal("deveria criar 1")
	}
	in := a.r.criadas[0]
	if in.TripID != "t1" || in.BoardStopID == "" || in.AlightStopID == "" || in.Source == nil || *in.Source != "WHATSAPP_V2" {
		t.Fatalf("input: %+v", in)
	}
	if !strings.HasPrefix(in.IdempotencyKey, "atdv2:conv-1:") {
		t.Fatalf("chave: %s", in.IdempotencyKey)
	}
	// 2 pagantes x 1100 = 2200; sinal 2 x 250 = 500
	if in.TotalAmount != 2200 || in.DepositAmount != 500 || in.RemainderAmount != 1700 {
		t.Fatalf("valores: %+v", in)
	}
	if len(in.Passengers) != 3 || in.Passengers[0].DocumentType != "CPF" || in.Passengers[2].DocumentType != "" || !in.Passengers[2].IsLapChild || in.Passengers[0].Phone != "5549999887766" {
		t.Fatalf("passageiros: %+v", in.Passengers)
	}
	e := a.ctx.Estado
	if e.ReservaID != "bk-1" || e.Pagamento != "sinal" {
		t.Fatalf("estado: %+v", e)
	}
	d := dadosDe(m)
	if d["codigo_reserva"] != "SCH0001" || d["valor_a_pagar_agora"] != float64(500) || d["proximo_passo"] != "gerar_pix" {
		t.Fatalf("dados: %+v", d)
	}
	// segunda chamada: devolve a existente
	s, m = a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if !s.OK || len(a.r.criadas) != 1 || dadosDe(m)["ja_existente"] != true || dadosDe(m)["codigo_reserva"] != "SCH0001" {
		t.Fatalf("idempotencia: criadas=%d %+v", len(a.r.criadas), m)
	}
}

func TestCriarReservaIntegral(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	if s, m := a.exec(t, "criar_reserva", `{"pagamento":"integral"}`); !s.OK {
		t.Fatalf("%+v", m)
	}
	in := a.r.criadas[0]
	if in.TotalAmount != 2200 || in.DepositAmount != 2200 || in.RemainderAmount != 0 {
		t.Fatalf("valores: %+v", in)
	}
}

func TestCriarReservaSinalConfiguravel(t *testing.T) {
	a := novoAmbiente(t)
	a.reg = Padrao(a.cat, a.b, a.q, a.r, a.p, Config{SinalPorPagante: 300})
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if a.r.criadas[0].DepositAmount != 600 {
		t.Fatalf("sinal: %+v", a.r.criadas[0])
	}
}

func TestCriarReservaMapeiaErros(t *testing.T) {
	casos := map[error]string{
		bookings.ErrNoSeatsAvailable:                         "sem_vagas",
		bookings.ErrPassengerNameRequired:                    "nome_passageiro_obrigatorio",
		bookings.ErrPassengerDocumentType:                    "tipo_documento_invalido",
		bookings.ErrMissingFields:                            "dados_incompletos",
		bookings.ErrInvalidAmounts:                           "valores_invalidos",
		bookings.ErrSeatNotInTrip:                            "opcao_inconsistente",
		fmt.Errorf("wrap: %w", bookings.ErrNoSeatsAvailable): "sem_vagas",
		errBoom: "erro_ao_criar_reserva",
	}
	for err, quer := range casos {
		a := novoAmbiente(t)
		prepararReserva(t, a)
		a.r.errCreate = err
		s, m := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
		if s.OK || s.Motivo != quer {
			t.Errorf("%v: got %+v want %s", err, m, quer)
		}
		if a.ctx.Estado.ReservaID != "" {
			t.Errorf("%v: nao deveria gravar ReservaID", err)
		}
		if strings.Contains(fmt.Sprint(m), "boom") {
			t.Error("erro interno vazou ao modelo")
		}
	}
}

func TestReservaBloqueiaAlteracoesDepoisDeCriada(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if s, _ := a.exec(t, "registrar_passageiros", `{"passageiros":[{"nome":"Maria Silva","documento":"`+cpfA+`"}]}`); s.Motivo != "reserva_ja_criada" {
		t.Fatalf("%+v", s)
	}
	if s, _ := a.exec(t, "escolher_viagem", `{"opcao":1}`); s.Motivo != "reserva_ja_criada" {
		t.Fatalf("%+v", s)
	}
}

// ---- pix ----

func TestGerarPixNaoDuplica(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "gerar_pix", `{}`); s.OK || s.Motivo != "reserva_nao_criada" {
		t.Fatalf("%+v", s)
	}
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	s, m := a.exec(t, "gerar_pix", `{}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	if d["pix_copia_e_cola"] != "000201PIXpay-1" || d["valor"] != float64(500) || d["expira_em"] != "2026-09-30T16:00:00Z" {
		t.Fatalf("dados: %+v", d)
	}
	if len(a.p.criados) != 1 {
		t.Fatalf("criados: %d", len(a.p.criados))
	}
	in := a.p.criados[0]
	if in.Method != "PIX" || in.BookingID != "bk-1" || in.Amount != 500 || in.Customer == nil ||
		in.Customer.Name != "Maria Silva" || in.Customer.Document != "52998224725" || in.Customer.Phone != "5549999887766" {
		t.Fatalf("pagamento: %+v %+v", in, in.Customer)
	}
	if a.ctx.Estado.PagamentoID != "pay-1" {
		t.Fatalf("estado: %+v", a.ctx.Estado)
	}
	// chamada repetida reaproveita
	s, m = a.exec(t, "gerar_pix", `{}`)
	if !s.OK || len(a.p.criados) != 1 || dadosDe(m)["pix_copia_e_cola"] != "000201PIXpay-1" || dadosDe(m)["reaproveitado"] != true {
		t.Fatalf("reuso: criados=%d %+v", len(a.p.criados), m)
	}
	// pago: nao gera outro
	a.p.status["pay-1"] = "PAID"
	s, m = a.exec(t, "gerar_pix", `{}`)
	if !s.OK || len(a.p.criados) != 1 || dadosDe(m)["status"] != "pago" {
		t.Fatalf("pago: %+v", m)
	}
	// cancelado: gera novo
	a.p.status["pay-1"] = "FAILED"
	if s, m = a.exec(t, "gerar_pix", `{}`); !s.OK || len(a.p.criados) != 2 || a.ctx.Estado.PagamentoID != "pay-2" {
		t.Fatalf("falho: %+v", m)
	}
}

func TestGerarPixIntegralValorTotal(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"integral"}`)
	s, m := a.exec(t, "gerar_pix", `{}`)
	if !s.OK || dadosDe(m)["valor"] != float64(2200) {
		t.Fatalf("%+v", m)
	}
}

func TestGerarPixExigeCPFDoPagador(t *testing.T) {
	a := novoAmbiente(t)
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	a.exec(t, "escolher_viagem", `{"opcao":1}`)
	a.exec(t, "registrar_passageiros", `{"passageiros":[{"nome":"João Souza","documento":"12.345.678-9"}]}`)
	a.exec(t, "criar_reserva", `{"pagamento":"integral"}`)
	s, _ := a.exec(t, "gerar_pix", `{}`)
	if s.OK || s.Motivo != "cpf_do_pagador_necessario" || len(a.p.criados) != 0 {
		t.Fatalf("%+v", s)
	}
	if s, _ := a.exec(t, "gerar_pix", `{"cpf_pagador":"111"}`); s.OK {
		t.Fatal("cpf invalido nao deveria passar")
	}
	if s, m := a.exec(t, "gerar_pix", `{"cpf_pagador":"`+cpfB+`"}`); !s.OK || a.p.criados[0].Customer.Document != "11144477735" {
		t.Fatalf("%+v", m)
	}
}

func TestGerarPixFalhas(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.p.errCreate = errBoom
	if s, m := a.exec(t, "gerar_pix", `{}`); s.OK || s.Motivo != "erro_gerar_pix" || strings.Contains(fmt.Sprint(m), "boom") {
		t.Fatalf("%+v", m)
	}
	a.p.errCreate = nil
	a.p.semPix = true
	if s, _ := a.exec(t, "gerar_pix", `{}`); s.OK || s.Motivo != "pix_indisponivel" {
		t.Fatalf("%+v", s)
	}
	if a.ctx.Estado.PagamentoID == "" {
		t.Fatal("cobranca criada deve ser gravada para nao duplicar")
	}
	// reserva cancelada
	b := novoAmbiente(t)
	prepararReserva(t, b)
	b.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	b.r.status = "CANCELLED"
	if s, _ := b.exec(t, "gerar_pix", `{}`); s.Motivo != "reserva_cancelada_ou_expirada" {
		t.Fatalf("%+v", s)
	}
	// telefone ausente
	c := novoAmbiente(t)
	prepararReserva(t, c)
	c.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	c.ctx.Conversa.Telefone = ""
	if s, _ := c.exec(t, "gerar_pix", `{}`); s.Motivo != "telefone_pagador_necessario" {
		t.Fatalf("%+v", s)
	}
}

func TestGerarPixVencidoGeraNovo(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.exec(t, "gerar_pix", `{}`)
	a.ctx.Agora = agoraFixa.Add(2 * time.Hour) // passou de 1h
	if s, m := a.exec(t, "gerar_pix", `{}`); !s.OK || len(a.p.criados) != 2 {
		t.Fatalf("criados=%d %+v", len(a.p.criados), m)
	}
}

// ---- consultar e humano ----

func TestConsultarReserva(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "consultar_reserva", `{}`); s.OK || s.Motivo != "informe_o_codigo" {
		t.Fatalf("%+v", s)
	}
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.exec(t, "gerar_pix", `{}`)
	s, m := a.exec(t, "consultar_reserva", `{}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	if d["codigo_reserva"] != "SCH0001" || d["status_legivel"] != "aguardando pagamento" || len(d["passageiros"].([]any)) != 3 {
		t.Fatalf("dados: %+v", d)
	}
	if pg := d["pagamento"].(map[string]any); pg["status_legivel"] != "pendente" || pg["valor"] != float64(500) {
		t.Fatalf("pagamento: %+v", pg)
	}
	a.p.status["pay-1"] = "PAID"
	_, m = a.exec(t, "consultar_reserva", `{}`)
	if dadosDe(m)["pagamento"].(map[string]any)["status_legivel"] != "pago" {
		t.Fatalf("%+v", m)
	}
}

func TestConsultarReservaPorCodigo(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.r.itens = []bookings.BookingListItem{{ID: "bk-1", ReservationCode: "SCH0001"}}
	// outra conversa (sem ReservaID) consultando por codigo: sem nomes
	b := novoAmbiente(t)
	b.r, b.p, b.q = a.r, a.p, a.q
	b.reg = Padrao(b.cat, b.b, b.q, b.r, b.p, Config{})
	s, m := b.exec(t, "consultar_reserva", `{"codigo":"sch0001"}`)
	if !s.OK || dadosDe(m)["codigo_reserva"] != "SCH0001" || dadosDe(m)["passageiros"] != nil {
		t.Fatalf("%+v", m)
	}
	if s, _ := b.exec(t, "consultar_reserva", `{"codigo":"NAOEXISTE"}`); s.OK || s.Motivo != "reserva_nao_encontrada" {
		t.Fatalf("%+v", s)
	}
}

func TestTransferirParaHumano(t *testing.T) {
	a := novoAmbiente(t)
	s, _ := a.exec(t, "transferir_para_humano", `{"motivo":"cliente pediu atendente"}`)
	if !s.OK || !s.Transferir || a.ctx.Estado.MotivoHumano != "cliente pediu atendente" {
		t.Fatalf("%+v / %+v", s, a.ctx.Estado)
	}
	b := novoAmbiente(t)
	if s, _ := b.exec(t, "transferir_para_humano", `{}`); !s.OK || !s.Transferir || b.ctx.Estado.MotivoHumano == "" {
		t.Fatalf("sem motivo tambem deve transferir: %+v", s)
	}
}

func TestArgumentosInvalidos(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "buscar_viagens", `{"pessoas":"muitas","destino":"Chapecó"}`); s.Motivo != "argumentos_invalidos" {
		t.Fatalf("%+v", s)
	}
	if s, _ := a.exec(t, "escolher_viagem", `not json`); s.Motivo != "argumentos_invalidos" {
		t.Fatalf("%+v", s)
	}
}

func TestCotacaoIgualNaoAvisa(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	_, m := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if dadosDe(m)["preco_atualizado"] != nil {
		t.Fatalf("nao deveria avisar: %+v", m)
	}
	if len(a.q.ins) < 2 || a.q.ins[0].FareMode != "AUTO" || a.q.ins[0].TripID != "t1" {
		t.Fatalf("cotacoes: %+v", a.q.ins)
	}
}

func TestEscolherViagemCotaPrecoDivergente(t *testing.T) {
	a := novoAmbiente(t)
	a.q.ajuste = map[string]float64{"t1": 1050}
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	d := dadosDe(m)
	if d["preco_atualizado"] != float64(1050) || d["preco_anterior"] != float64(1100) || a.ctx.Estado.Viagem.Preco != 1050 {
		t.Fatalf("dados: %+v preco=%v", d, a.ctx.Estado.Viagem.Preco)
	}
	if d["viagem"].(map[string]any)["preco"] != float64(1050) {
		t.Fatalf("viagem: %+v", d["viagem"])
	}
}

func TestCriarReservaUsaPrecoCotado(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.q.ajuste = map[string]float64{"t1": 1000} // mudou depois da escolha
	s, m := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	in := a.r.criadas[0]
	if in.TotalAmount != 2000 || in.DepositAmount != 500 || in.RemainderAmount != 1500 {
		t.Fatalf("valores: %+v", in)
	}
	d := dadosDe(m)
	if d["preco_atualizado"] != float64(1000) || d["preco_anterior"] != float64(1100) || a.ctx.Estado.Viagem.Preco != 1000 {
		t.Fatalf("dados: %+v", d)
	}
}

func TestErroDeCotacao(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.q.err = errBoom
	if s, _ := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`); s.OK || s.Motivo != "erro_cotacao" || len(a.r.criadas) != 0 {
		t.Fatalf("%+v", s)
	}
	if s, _ := a.exec(t, "escolher_viagem", `{"opcao":1}`); s.Motivo != "erro_cotacao" {
		t.Fatalf("%+v", s)
	}
}
