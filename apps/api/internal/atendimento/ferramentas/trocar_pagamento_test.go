package ferramentas

import "testing"

func TestTrocarPagamentoIntegralParaSinal(t *testing.T) {
	a := novoAmbiente(t)
	if s, _ := a.exec(t, "trocar_pagamento", `{"pagamento":"sinal"}`); s.OK || s.Motivo != "reserva_nao_criada" {
		t.Fatalf("sem reserva: %+v", s)
	}
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"integral"}`)
	if s, _ := a.exec(t, "gerar_pix", `{}`); !s.OK {
		t.Fatalf("pix: %+v", s)
	}
	if s, _ := a.exec(t, "trocar_pagamento", `{"pagamento":"integral"}`); s.OK || s.Motivo != "mesma_forma" {
		t.Fatalf("mesma forma: %+v", s)
	}
	s, m := a.exec(t, "trocar_pagamento", `{"pagamento":"sinal"}`)
	if !s.OK {
		t.Fatalf("%+v", m)
	}
	if len(a.p.cancelados) != 1 || a.p.cancelados[0] != "pay-1" || a.p.status["pay-1"] != "CANCELLED" {
		t.Fatalf("cancelados=%v status=%v", a.p.cancelados, a.p.status)
	}
	if a.p.sinais["bk-1"] != 500 || a.ctx.Estado.Pagamento != "sinal" || a.ctx.Estado.Trechos[0].PagamentoID != "pay-2" {
		t.Fatalf("sinais=%v estado=%+v", a.p.sinais, a.ctx.Estado)
	}
	d := dadosDe(m)
	if item(m, "pix", 0)["valor"] != float64(500) || d["pagamento_anterior"] != "integral" || d["restante_no_embarque_total"] != float64(1700) {
		t.Fatalf("dados: %+v", d)
	}
}

func TestTrocarPagamentoJaPagoNaoTroca(t *testing.T) {
	a := novoAmbiente(t)
	prepararReserva(t, a)
	a.exec(t, "criar_reserva", `{"pagamento":"sinal"}`)
	a.exec(t, "gerar_pix", `{}`)
	a.p.status["pay-1"] = "PAID"
	s, _ := a.exec(t, "trocar_pagamento", `{"pagamento":"integral"}`)
	if s.OK || s.Motivo != "ja_pago" || len(a.p.cancelados) != 0 || a.ctx.Estado.Pagamento != "sinal" || len(a.p.criados) != 1 {
		t.Fatalf("%+v cancelados=%v estado=%+v", s, a.p.cancelados, a.ctx.Estado)
	}
}
