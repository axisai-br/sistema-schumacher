package conversa

import (
	"reflect"
	"strings"
	"testing"
)

func TestPendencias(t *testing.T) {
	viagem := &Opcao{TripID: "t1", Origem: "Videira", Destino: "Monção", Data: "2026-10-10", Horario: "19:00", Preco: 950}
	ok := Passageiro{Nome: "Ana Silva", Documento: "12345678901", TipoDocumento: "CPF"}
	casos := []struct {
		nome string
		e    Estado
		want []string
	}{
		{"vazio", Estado{}, []string{"escolher viagem", "informar passageiros (nome e documento)", "escolher pagamento: integral ou sinal", "criar reserva", "gerar PIX"}},
		{"so viagem", Estado{Viagem: viagem}, []string{"informar passageiros (nome e documento)", "escolher pagamento: integral ou sinal", "criar reserva", "gerar PIX"}},
		{"passageiro sem documento", Estado{Viagem: viagem, Passageiros: []Passageiro{ok, {Nome: "Beto"}}},
			[]string{"completar dados de 1 passageiro(s)", "escolher pagamento: integral ou sinal", "criar reserva", "gerar PIX"}},
		{"pessoas informadas a mais", Estado{Viagem: viagem, PessoasInformadas: 3, Passageiros: []Passageiro{ok}, Pagamento: "sinal"},
			[]string{"completar dados de 2 passageiro(s)", "criar reserva", "gerar PIX"}},
		{"reserva sem pix", Estado{Viagem: viagem, Passageiros: []Passageiro{ok}, Pagamento: "integral", ReservaID: "r1"}, []string{"gerar PIX"}},
		{"completo", Estado{Viagem: viagem, Passageiros: []Passageiro{ok}, Pagamento: "sinal", ReservaID: "r1", PagamentoID: "p1"}, nil},
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

func TestResumoMascaraDocumento(t *testing.T) {
	e := Estado{
		Origem:      &Parada{Nome: "Videira", UF: "SC"},
		Destino:     &Parada{Nome: "Monção", UF: "MA"},
		Viagem:      &Opcao{Origem: "Videira", Destino: "Monção", Data: "2026-10-10", Horario: "19:00", Preco: 950},
		Passageiros: []Passageiro{{Nome: "Ana Silva", Documento: "123.456.789-01", TipoDocumento: "CPF"}},
		Pagamento:   "sinal", MotivoHumano: "pediu atendente",
	}
	r := e.Resumo()
	for _, proibido := range []string{"12345678901", "123.456", "456.789"} {
		if strings.Contains(r, proibido) {
			t.Fatalf("documento vazou no resumo (%q): %s", proibido, r)
		}
	}
	for _, want := range []string{"Videira/SC para Monção/MA", "10/10/2026", "***901", "sinal", "criar reserva", "pediu atendente"} {
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
