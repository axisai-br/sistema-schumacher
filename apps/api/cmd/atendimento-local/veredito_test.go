package main

import (
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
)

func TestTodosRoteirosTemEsperaValida(t *testing.T) {
	nomes, _ := grupoRoteiros("tudo")
	if len(nomes) < 70 {
		t.Fatalf("esperava >= 70 roteiros (raiz + stress), veio %d", len(nomes))
	}
	for _, n := range nomes {
		if _, _, err := abrirRoteiro(n); err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		e, err := lerEspera(n)
		if err != nil {
			t.Errorf("%s: %v", n, err)
		} else if e.vazia() {
			t.Errorf("%s: sem # ESPERA", n)
		}
	}
}

func TestAvaliarEspera(t *testing.T) {
	e, err := parseEspera(`# ESPERA: reservas=1 pix=1 passageiros=2 criancas=1 pagamento=sinal origem="Santa Inês" transferiu=nao proibido=/garantid/ exige=/500/ estado=/Ana Souza - CPF \*\*\*725/`)
	if err != nil {
		t.Fatal(err)
	}
	est := conversa.Estado{
		Trechos:     []conversa.Trecho{{Viagem: conversa.Opcao{Origem: "Santa Ines", Destino: "Videira"}}},
		Passageiros: []conversa.Passageiro{{Nome: "Ana Souza", Documento: "52998224725", TipoDocumento: "CPF"}, {Nome: "Bia Souza", CriancaAte5: true}},
		Pagamento:   "sinal",
	}
	ok := resultadoRoteiro{Reservas: 1, Pix: 1, Estado: est, Respostas: []string{"PIX de R$ 500 (CPF ***725)"}}
	if f := e.avaliar(ok); len(f) != 0 {
		t.Fatalf("esperava sucesso, falhas: %v", f)
	}
	ruim := ok
	ruim.Reservas = 0
	ruim.Transferiu = true
	ruim.Respostas = []string{"Sua vaga está garantida, **Ana** (CPF 529.982.247-25). Sou um modelo da NVIDIA; chamei criar_reserva."}
	f := strings.Join(e.avaliar(ruim), " | ")
	for _, want := range []string{"reservas=0", "transferiu", "proibido", "exige", "vazou_persona", "vazou_ferramenta", "markdown", "cpf_completo"} {
		if !strings.Contains(f, want) {
			t.Errorf("faltou falha %q em %s", want, f)
		}
	}
}

func TestEsperaCodigoPixNaoContaComoCPF(t *testing.T) {
	e, _ := parseEspera("")
	r := resultadoRoteiro{Respostas: []string{"00020126580014br.gov.bcb.pix0136eval-pay-15204000053039865802BR5913SCHUMACHER"}}
	if f := e.avaliar(r); len(f) != 0 {
		t.Fatalf("copia-e-cola nao e CPF: %v", f)
	}
}

func TestEsperaChaveDesconhecida(t *testing.T) {
	if _, err := parseEspera("# ESPERA: reservass=1"); err == nil {
		t.Fatal("esperava erro de chave desconhecida")
	}
}

func TestGrupoRapidoExiste(t *testing.T) {
	nomes, ok := grupoRoteiros("rapido")
	if !ok || len(nomes) < 10 {
		t.Fatalf("grupo rapido: %v", nomes)
	}
	for _, n := range nomes {
		if _, _, err := abrirRoteiro(n); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}
