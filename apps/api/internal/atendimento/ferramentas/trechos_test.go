package ferramentas

import (
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/bookings"
)

const paxTres = `{"passageiros":[
	{"nome":"Maria Silva","documento":"` + cpfA + `"},
	{"nome":"João Souza","documento":"12.345.678-9"},
	{"nome":"Pedrinho Silva","crianca_ate_5":true}]}`

// escolherIdaEVolta escolhe t1 (Chapeco -> Santa Ines) e t4 (Santa Ines -> Chapeco).
func escolherIdaEVolta(t *testing.T, a *ambiente) {
	t.Helper()
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	if s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`); !s.OK {
		t.Fatalf("ida: %+v", m)
	}
	a.exec(t, "buscar_viagens", `{"origem":"Santa Inês","destino":"Chapecó"}`)
	if s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`); !s.OK {
		t.Fatalf("volta: %+v", m)
	}
}

func TestDoisTrechosDuasReservasDoisPix(t *testing.T) {
	a := novoAmbiente(t)
	escolherIdaEVolta(t, a)
	if n := len(a.ctx.Estado.Trechos); n != 2 {
		t.Fatalf("trechos: %+v", a.ctx.Estado.Trechos)
	}
	if s, m := a.exec(t, "registrar_passageiros", paxTres); !s.OK {
		t.Fatalf("%+v", m)
	}
	s, m := a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	if len(a.r.criadas) != 2 || a.r.criadas[0].TripID != "t1" || a.r.criadas[1].TripID != "t4" ||
		a.r.criadas[0].IdempotencyKey == a.r.criadas[1].IdempotencyKey {
		t.Fatalf("reservas: %+v", a.r.criadas)
	}
	d := dadosDe(m)
	// 2 pagantes x 1100 por trecho = 2200; sinal 500 por trecho
	if d["total_geral"] != float64(4400) || d["valor_a_pagar_agora"] != float64(1000) || d["proximo_passo"] != "gerar_pix" {
		t.Fatalf("dados: %+v", d)
	}
	if item(m, "trechos", 0)["codigo_reserva"] != "SCH0001" || item(m, "trechos", 1)["codigo_reserva"] != "SCH0002" || item(m, "trechos", 1)["trecho"] != float64(2) {
		t.Fatalf("trechos: %+v", d["trechos"])
	}
	// idempotente
	if s, m = a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`); !s.OK || len(a.r.criadas) != 2 || item(m, "trechos", 1)["ja_existente"] != true {
		t.Fatalf("idempotencia: criadas=%d %+v", len(a.r.criadas), m)
	}

	s, m = a.exec(t, "gerar_pix", `{}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	if len(a.p.criados) != 2 || a.p.criados[0].BookingID != "bk-1" || a.p.criados[1].BookingID != "bk-2" ||
		a.p.criados[0].Amount != 500 || a.p.criados[1].Amount != 500 {
		t.Fatalf("pagamentos: %+v", a.p.criados)
	}
	d = dadosDe(m)
	p0, p1 := item(m, "pix", 0), item(m, "pix", 1)
	if d["total"] != float64(1000) || p0["pix_copia_e_cola"] != "000201PIXpay-1" || p1["pix_copia_e_cola"] != "000201PIXpay-2" ||
		p0["trecho"] != float64(1) || p1["trecho"] != float64(2) || p1["rota"] != "Santa Inês para Chapecó" || p1["data"] != "2026-11-25" || p1["expira_em"] == nil {
		t.Fatalf("pix: %+v", d)
	}
	if e := a.ctx.Estado; e.Trechos[0].PagamentoID != "pay-1" || e.Trechos[1].PagamentoID != "pay-2" || len(pendenciasDados(*e)) != 0 {
		t.Fatalf("estado: %+v pend=%v", e.Trechos, pendenciasDados(*e))
	}
	// de novo: reaproveita os dois, sem cobranca nova
	s, m = a.exec(t, "gerar_pix", `{}`)
	if !s.OK || len(a.p.criados) != 2 || item(m, "pix", 0)["reaproveitado"] != true || item(m, "pix", 1)["reaproveitado"] != true {
		t.Fatalf("reuso: criados=%d %+v", len(a.p.criados), m)
	}
}

func TestAdicionarVoltaDepoisDaIdaReservadaEComPix(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.exec(t, "gerar_pix", `{}`)
	a.p.status["pay-1"] = "PAID"

	a.exec(t, "buscar_viagens", `{"origem":"Santa Inês","destino":"Chapecó"}`)
	s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`)
	if !s.OK || dadosDe(m)["trecho"] != float64(2) {
		t.Fatalf("volta: %+v", m)
	}
	e := a.ctx.Estado
	if len(e.Trechos) != 2 || e.Trechos[0].ReservaID != "bk-1" || e.Trechos[0].PagamentoID != "pay-1" || e.Trechos[1].ReservaID != "" {
		t.Fatalf("trechos: %+v", e.Trechos)
	}
	pend := pendenciasDados(*e)
	if len(pend) != 2 || !strings.HasPrefix(pend[0], "criar reserva do trecho 2") || !strings.HasPrefix(pend[1], "gerar PIX do trecho 2") {
		t.Fatalf("pendencias: %v", pend)
	}
	// os mesmos passageiros valem; outra lista nao
	if s, m := a.exec(t, "registrar_passageiros", paxTres); !s.OK {
		t.Fatalf("mesma lista: %+v", m)
	}
	if s, _ := a.exec(t, "registrar_passageiros", `{"passageiros":[{"nome":"Maria Silva","documento":"`+cpfA+`"}]}`); s.Motivo != "reserva_ja_criada" {
		t.Fatalf("lista diferente deveria bloquear: %+v", s)
	}
	// cria so a reserva da volta
	s, m = a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	if !s.OK || len(a.r.criadas) != 2 || a.r.criadas[1].TripID != "t4" {
		t.Fatalf("criar: criadas=%d %+v", len(a.r.criadas), m)
	}
	if item(m, "trechos", 0)["ja_existente"] != true || item(m, "trechos", 1)["ja_existente"] != false {
		t.Fatalf("trechos: %+v", dadosDe(m)["trechos"])
	}
	// PIX: a ida ja esta paga (informada, sem nova cobranca); gera so o da volta
	s, m = a.exec(t, "gerar_pix", `{}`)
	if !s.OK || len(a.p.criados) != 2 || a.p.criados[1].BookingID != "bk-2" {
		t.Fatalf("pix: criados=%d %+v", len(a.p.criados), m)
	}
	if item(m, "pix", 0)["status"] != "pago" || item(m, "pix", 1)["pix_copia_e_cola"] != "000201PIXpay-2" || dadosDe(m)["total"] != float64(500) {
		t.Fatalf("saida: %+v", dadosDe(m))
	}
	if len(pendenciasDados(*a.ctx.Estado)) != 0 {
		t.Fatalf("pendencias: %v", pendenciasDados(*a.ctx.Estado))
	}
}

func TestCriarReservaParcialNaoDesfazOsOutros(t *testing.T) {
	a := novoAmbiente(t)
	escolherIdaEVolta(t, a)
	a.exec(t, "registrar_passageiros", paxTres)
	a.r.errPorTrip = map[string]error{"t4": bookings.ErrNoSeatsAvailable}
	s, m := a.exec(t, "criar_reserva", `{"pagamento":"integral"}`)
	if s.OK || s.Motivo != "reserva_parcial" {
		t.Fatalf("%+v", m)
	}
	e := a.ctx.Estado
	if e.Trechos[0].ReservaID == "" || e.Trechos[1].ReservaID != "" {
		t.Fatalf("a ida deveria ficar reservada: %+v", e.Trechos)
	}
	if item(m, "trechos", 0)["ok"] != true || item(m, "trechos", 1)["ok"] != false || item(m, "trechos", 1)["motivo"] != "sem_vagas" {
		t.Fatalf("trechos: %+v", dadosDe(m)["trechos"])
	}
	// o PIX da ida sai; a volta fica sem reserva e e sinalizada
	s, m = a.exec(t, "gerar_pix", `{}`)
	if !s.OK || len(a.p.criados) != 1 || len(dadosDe(m)["trechos_sem_reserva"].([]any)) != 1 {
		t.Fatalf("pix: %+v", m)
	}
	// falhou nos dois: motivo do primeiro erro
	b := novoAmbiente(t)
	escolherIdaEVolta(t, b)
	b.exec(t, "registrar_passageiros", paxTres)
	b.r.errCreate = errBoom
	if s, _ := b.exec(t, "criar_reserva", `{"pagamento":"integral"}`); s.OK || s.Motivo != "erro_ao_criar_reserva" || b.ctx.Estado.AlgumReservado() {
		t.Fatalf("%+v", s)
	}
}

func TestGerarPixParcial(t *testing.T) {
	a := novoAmbiente(t)
	escolherIdaEVolta(t, a)
	a.exec(t, "registrar_passageiros", paxTres)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	// a volta foi cancelada: a ida ainda gera PIX
	a.r.porChave[a.r.criadas[1].IdempotencyKey] = bookings.BookingDetails{Booking: bookings.Booking{ID: "bk-2", Status: "CANCELLED", TotalAmount: 2200}}
	s, m := a.exec(t, "gerar_pix", `{}`)
	if s.OK || s.Motivo != "pix_parcial" || len(a.p.criados) != 1 {
		t.Fatalf("%+v", m)
	}
	if item(m, "pix", 0)["pix_copia_e_cola"] != "000201PIXpay-1" || item(m, "pix", 1)["motivo"] != "reserva_cancelada_ou_expirada" {
		t.Fatalf("itens: %+v", dadosDe(m)["pix"])
	}
}

func TestTrechosDuplicadoLimiteSubstituirERemover(t *testing.T) {
	a := novoAmbiente(t)
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`) // t1 (12 vagas), t2 (1 vaga)
	a.exec(t, "escolher_viagem", `{"opcao":1}`)
	if s, m := a.exec(t, "escolher_viagem", `{"opcao":1}`); s.OK || s.Motivo != "trecho_duplicado" {
		t.Fatalf("duplicado: %+v", m)
	}
	// substituir o trecho 1 pela opcao 2 (t2)
	s, m := a.exec(t, "escolher_viagem", `{"opcao":2,"substituir_trecho":1}`)
	if !s.OK || dadosDe(m)["substituiu"] != true {
		t.Fatalf("substituir: %+v", m)
	}
	if tr := a.ctx.Estado.Trechos; len(tr) != 1 || tr[0].Viagem.TripID != "t2" {
		t.Fatalf("trechos: %+v", a.ctx.Estado.Trechos)
	}
	if s, _ := a.exec(t, "escolher_viagem", `{"opcao":2,"substituir_trecho":3}`); s.Motivo != "trecho_inexistente" {
		t.Fatalf("%+v", s)
	}
	// remover
	s, m = a.exec(t, "remover_trecho", `{"trecho":1}`)
	if !s.OK || len(a.ctx.Estado.Trechos) != 0 || dadosDe(m)["removido"] != "Chapecó para Santa Inês" {
		t.Fatalf("remover: %+v", m)
	}
	if s, _ := a.exec(t, "remover_trecho", `{"trecho":1}`); s.Motivo != "trecho_inexistente" {
		t.Fatalf("%+v", s)
	}
	// limite de 4 trechos
	b := novoAmbiente(t)
	b.ctx.Estado.Trechos = make([]conversa.Trecho, 4)
	b.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	if s, _ := b.exec(t, "escolher_viagem", `{"opcao":1}`); s.Motivo != "limite_de_trechos" {
		t.Fatalf("%+v", s)
	}
}

func TestPassageirosAcimaDasVagasDeQualquerTrecho(t *testing.T) {
	a := novoAmbiente(t)
	a.exec(t, "buscar_viagens", `{"origem":"Chapecó","destino":"Santa Inês"}`)
	a.exec(t, "escolher_viagem", `{"opcao":1}`) // t1: 12 vagas
	a.exec(t, "buscar_viagens", `{"origem":"Videira","destino":"Monção"}`)
	a.exec(t, "escolher_viagem", `{"opcao":1}`) // t3: 3 vagas
	s, m := a.exec(t, "registrar_passageiros", `{"passageiros":[
		{"nome":"Maria Silva","documento":"`+cpfA+`"},{"nome":"Ana Lima","documento":"`+cpfB+`"},
		{"nome":"João Souza","documento":"12.345.678-9"},{"nome":"Rita Souza","documento":"98.765.432-1"}]}`)
	if s.OK || s.Motivo != "passageiros_acima_das_vagas" || dadosDe(m)["trecho"] != float64(2) {
		t.Fatalf("%+v", m)
	}
}

func TestConsultarReservaTodosOsTrechos(t *testing.T) {
	a := novoAmbiente(t)
	escolherIdaEVolta(t, a)
	a.exec(t, "registrar_passageiros", paxTres)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.exec(t, "gerar_pix", `{}`)
	a.p.status["pay-2"] = "PAID"
	s, m := a.exec(t, "consultar_reserva", `{}`)
	if !s.OK || dadosDe(m)["total_reservas"] != float64(2) {
		t.Fatalf("%+v", m)
	}
	r0, r1 := item(m, "reservas", 0), item(m, "reservas", 1)
	if r0["codigo_reserva"] != "SCH0001" || r1["codigo_reserva"] != "SCH0002" || r1["trecho"] != float64(2) {
		t.Fatalf("reservas: %+v", dadosDe(m)["reservas"])
	}
	if r0["pagamento"].(map[string]any)["status_legivel"] != "pendente" || r1["pagamento"].(map[string]any)["status_legivel"] != "pago" {
		t.Fatalf("pagamentos: %+v / %+v", r0["pagamento"], r1["pagamento"])
	}
}

func TestEscolherPorRotaEDataSemNumeroDaBusca(t *testing.T) {
	a := novoAmbiente(t)
	// Ultima busca e a volta; a ida e escolhida por rota e data.
	a.exec(t, "buscar_viagens", `{"origem":"Santa Inês","destino":"Chapecó"}`)
	if s, m := a.exec(t, "escolher_viagem", `{"origem":"Chapecó","destino":"Santa Inês","data":"2026-10-10"}`); !s.OK {
		t.Fatalf("ida por data: %+v", m)
	}
	if s, m := a.exec(t, "escolher_viagem", `{"origem":"Santa Inês","destino":"Chapecó","data":"2026-11-25","horario":"07:00"}`); !s.OK {
		t.Fatalf("volta por data: %+v", m)
	}
	tr := a.ctx.Estado.Trechos
	if len(tr) != 2 || tr[0].Viagem.TripID != "t1" || tr[1].Viagem.TripID != "t4" {
		t.Fatalf("trechos: %+v", tr)
	}
	if s, _ := a.exec(t, "escolher_viagem", `{"origem":"Chapecó","destino":"Santa Inês","data":"2026-10-11"}`); s.OK || s.Motivo != "sem_viagem_na_data" {
		t.Fatalf("data sem viagem: %+v", s)
	}
	if s, _ := a.exec(t, "escolher_viagem", `{"origem":"Chapecó"}`); s.OK || s.Motivo != "viagem_nao_identificada" {
		t.Fatalf("sem dados: %+v", s)
	}
}
