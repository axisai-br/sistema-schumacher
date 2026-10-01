package conversa

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPendencias(t *testing.T) {
	viagem := Opcao{TripID: "t1", Origem: "Videira", Destino: "Monção", Data: "2026-10-10", Horario: "19:00", Preco: 950}
	volta := Opcao{TripID: "t2", Origem: "Monção", Destino: "Videira", Data: "2026-10-14", Horario: "07:00", Preco: 950}
	tr := func(r, p string) []Trecho { return []Trecho{{Viagem: viagem, ReservaID: r, PagamentoID: p}} }
	ok := Passageiro{Nome: "Ana Silva", Documento: "12345678901", TipoDocumento: "CPF"}
	casos := []struct {
		nome string
		e    Estado
		want []string
	}{
		{"vazio", Estado{}, []string{"escolher viagem", "informar passageiros (nome e documento)", "escolher pagamento: integral ou sinal"}},
		{"so viagem", Estado{Trechos: tr("", "")}, []string{"informar passageiros (nome e documento)", "escolher pagamento: integral ou sinal", "criar reserva", "gerar PIX"}},
		{"passageiro sem documento", Estado{Trechos: tr("", ""), Passageiros: []Passageiro{ok, {Nome: "Beto"}}},
			[]string{"completar dados de 1 passageiro(s)", "escolher pagamento: integral ou sinal", "criar reserva", "gerar PIX"}},
		{"pessoas informadas a mais", Estado{Trechos: tr("", ""), PessoasInformadas: 3, Passageiros: []Passageiro{ok}, Pagamento: "sinal"},
			[]string{"completar dados de 2 passageiro(s)", "criar reserva", "gerar PIX"}},
		{"reserva sem pix", Estado{Trechos: tr("r1", ""), Passageiros: []Passageiro{ok}, Pagamento: "integral"}, []string{"gerar PIX"}},
		{"completo", Estado{Trechos: tr("r1", "p1"), Passageiros: []Passageiro{ok}, Pagamento: "sinal"}, nil},
		{"ida paga, volta sem reserva",
			Estado{Trechos: []Trecho{{Viagem: viagem, ReservaID: "r1", PagamentoID: "p1"}, {Viagem: volta}}, Passageiros: []Passageiro{ok}, Pagamento: "sinal"},
			[]string{"criar reserva do trecho 2 (Monção para Videira)", "gerar PIX do trecho 2 (Monção para Videira)"}},
		{"duas reservas sem pix na volta",
			Estado{Trechos: []Trecho{{Viagem: viagem, ReservaID: "r1", PagamentoID: "p1"}, {Viagem: volta, ReservaID: "r2"}}, Passageiros: []Passageiro{ok}, Pagamento: "sinal"},
			[]string{"gerar PIX do trecho 2 (Monção para Videira)"}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := c.e.Pendencias()
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestPagantes(t *testing.T) {
	e := Estado{Passageiros: []Passageiro{{Nome: "A"}, {Nome: "B", CriancaAte5: true}, {Nome: "C"}}}
	if got := e.Pagantes(); got != 2 {
		t.Fatalf("Pagantes = %d, want 2", got)
	}
}

func TestReservados(t *testing.T) {
	e := Estado{}
	if e.AlgumReservado() || e.TodosReservados() {
		t.Fatal("estado vazio nao tem reservas")
	}
	e.Trechos = []Trecho{{ReservaID: "r1"}, {}}
	if !e.AlgumReservado() || e.TodosReservados() {
		t.Fatal("um de dois reservado")
	}
	e.Trechos[1].ReservaID = "r2"
	if !e.TodosReservados() {
		t.Fatal("todos reservados")
	}
}

func TestResumoMascaraDocumento(t *testing.T) {
	e := Estado{
		Origem:  &Parada{Nome: "Videira", UF: "SC"},
		Destino: &Parada{Nome: "Monção", UF: "MA"},
		Trechos: []Trecho{
			{Viagem: Opcao{Origem: "Videira", Destino: "Monção", Data: "2026-10-10", Horario: "19:00", Preco: 950}, ReservaID: "bk-9", PagamentoID: "pay-9"},
			{Viagem: Opcao{Origem: "Monção", Destino: "Videira", Data: "2026-10-14", Horario: "07:00", Preco: 950}},
		},
		Passageiros: []Passageiro{{Nome: "Ana Silva", Documento: "123.456.789-01", TipoDocumento: "CPF"}},
		Pagamento:   "sinal", MotivoHumano: "pediu atendente",
	}
	r := e.Resumo()
	for _, proibido := range []string{"12345678901", "123.456", "456.789"} {
		if strings.Contains(r, proibido) {
			t.Fatalf("documento vazou no resumo (%q): %s", proibido, r)
		}
	}
	for _, want := range []string{"Videira/SC para Monção/MA", "Trecho 1: Videira para Monção em 10/10/2026 as 19:00", "reserva: bk-9; PIX: gerado (pay-9)",
		"Trecho 2: Monção para Videira em 14/10/2026 as 07:00", "reserva: nao criada; PIX: nao gerado", "***901", "sinal", "criar reserva do trecho 2", "pediu atendente"} {
		if !strings.Contains(r, want) {
			t.Errorf("resumo sem %q:\n%s", want, r)
		}
	}
}

func TestMascararDocumento(t *testing.T) {
	if got := MascararDocumento("123.456.789-01"); got != "***901" {
		t.Fatalf("got %q", got)
	}
	if got := MascararDocumento("12"); got != "***" {
		t.Fatalf("got %q", got)
	}
}

func TestEstadoJSONFormatoAntigo(t *testing.T) {
	antigo := `{"origem":{"stop_id":"a","nome":"Fraiburgo","uf":"SC"},
		"viagem":{"numero":1,"trip_id":"t1","origem":"Fraiburgo","destino":"Monção","data":"2026-10-08","horario":"19:00","preco":950,"vagas":10},
		"passageiros":[{"nome":"Ana Silva","documento":"52998224725","tipo_documento":"CPF","crianca_ate_5":false}],
		"pagamento":"sinal","reserva_id":"bk-1","pagamento_id":"pay-1","falhas":1}`
	var e Estado
	if err := json.Unmarshal([]byte(antigo), &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Trechos) != 1 || e.Trechos[0].Viagem.TripID != "t1" || e.Trechos[0].ReservaID != "bk-1" || e.Trechos[0].PagamentoID != "pay-1" {
		t.Fatalf("trechos: %+v", e.Trechos)
	}
	if e.Origem == nil || e.Origem.Nome != "Fraiburgo" || e.Pagamento != "sinal" || e.Falhas != 1 || len(e.Passageiros) != 1 {
		t.Fatalf("estado: %+v", e)
	}
	// regravar usa so o formato novo
	b, _ := json.Marshal(e)
	var raiz map[string]json.RawMessage
	_ = json.Unmarshal(b, &raiz)
	for _, velho := range []string{"viagem", "reserva_id", "pagamento_id"} {
		if _, ok := raiz[velho]; ok {
			t.Errorf("campo antigo %q regravado na raiz: %s", velho, b)
		}
	}
	if _, ok := raiz["trechos"]; !ok {
		t.Errorf("sem trechos: %s", b)
	}

	// somente viagem escolhida (sem reserva)
	var so Estado
	if err := json.Unmarshal([]byte(`{"viagem":{"trip_id":"t9"},"falhas":0}`), &so); err != nil || len(so.Trechos) != 1 || so.Trechos[0].ReservaID != "" {
		t.Fatalf("so viagem: %v %+v", err, so)
	}
	// vazio e novo formato
	var vazio Estado
	if err := json.Unmarshal([]byte(`{"falhas":2}`), &vazio); err != nil || len(vazio.Trechos) != 0 || vazio.Falhas != 2 {
		t.Fatalf("vazio: %v %+v", err, vazio)
	}
	novo := Estado{Trechos: []Trecho{{Viagem: Opcao{TripID: "a"}, ReservaID: "r1"}, {Viagem: Opcao{TripID: "b"}}}, Pagamento: "integral"}
	b, _ = json.Marshal(novo)
	var volta Estado
	if err := json.Unmarshal(b, &volta); err != nil || !reflect.DeepEqual(novo, volta) {
		t.Fatalf("ida e volta do JSON: %v\n%+v\n%+v", err, novo, volta)
	}
}
