package agente

import (
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
)

var cidadesTeste = []string{"Fraiburgo", "Monção", "Chapecó", "Santa Inês"}

func TestRotasSemBuscaVoltaInventada(t *testing.T) {
	conhecidas := map[string]bool{chaveRota("Fraiburgo", "Monção"): true}
	texto := "Para a volta (Monção → Fraiburgo), aqui estão as opções:\n1. 08/10, 16:30, R$ 950"
	got := rotasSemBusca(texto, cidadesTeste, conhecidas)
	if len(got) != 1 || got[0] != "Monção → Fraiburgo" {
		t.Fatalf("got %v", got)
	}
	// A rota buscada passa.
	if got := rotasSemBusca("Fraiburgo → Monção, 08/10 às 16:30", cidadesTeste, conhecidas); len(got) != 0 {
		t.Fatalf("rota conhecida marcada: %v", got)
	}
	// "de X para Y" tambem conta.
	if got := rotasSemBusca("Saindo de Monção para Fraiburgo dia 12/10 às 08:40", cidadesTeste, conhecidas); len(got) != 1 {
		t.Fatalf("de X para Y: %v", got)
	}
}

func TestRotasSemBuscaSemHorarioNaoMarca(t *testing.T) {
	// Oferecer a busca da volta, sem citar datas, e permitido.
	got := rotasSemBusca("Quer que eu busque a volta Monção → Fraiburgo?", cidadesTeste, map[string]bool{})
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestOpcoesDoTurnoETextoOpcoes(t *testing.T) {
	res := []string{
		`{"ok":false,"motivo":"x"}`,
		`{"ok":true,"dados":{"opcoes":[{"numero":1,"origem":"Monção","destino":"Fraiburgo","data":"2026-10-12","horario":"08:40","preco":950},{"numero":2,"origem":"Monção","destino":"Fraiburgo","data":"2026-10-19","horario":"08:40","preco":1100}]}}`,
	}
	bs := opcoesDoTurno(res)
	if len(bs) != 1 || len(bs[0]) != 2 {
		t.Fatalf("opcoes: %+v", bs)
	}
	txt := textoOpcoes(bs[0])
	for _, s := range []string{"Opções de Monção → Fraiburgo:", "1. seg 12/10 às 08:40, R$ 950", "2. seg 19/10 às 08:40, R$ 1.100", "Qual delas você prefere?"} {
		if !strings.Contains(txt, s) {
			t.Errorf("faltou %q em:\n%s", s, txt)
		}
	}
}

func TestRotasConhecidasIncluiTrechos(t *testing.T) {
	tc := &turno{estado: conversa.Estado{Trechos: []conversa.Trecho{{Viagem: conversa.Opcao{Origem: "Monção", Destino: "Fraiburgo"}}}}}
	if !rotasConhecidas(tc)[chaveRota("Monção", "Fraiburgo")] {
		t.Fatal("trecho escolhido deveria contar como rota conhecida")
	}
}

func TestPixDoTurnoETextoPix(t *testing.T) {
	res := []string{`{"ok":true,"dados":{"pix":[{"trecho":1,"rota":"Fraiburgo → Monção","data":"2026-10-08","valor":250,"pix_copia_e_cola":"AAA"},{"trecho":2,"rota":"Monção → Fraiburgo","data":"2026-10-12","valor":250,"pix_copia_e_cola":"BBB"}],"total":500}}`}
	px := pixDoTurno(res)
	if len(px) != 2 {
		t.Fatalf("pix: %+v", px)
	}
	txt := textoPix(px)
	for _, s := range []string{"Fraiburgo → Monção, 08/10: R$ 250\nAAA", "Monção → Fraiburgo, 12/10: R$ 250\nBBB", "Total a pagar agora: R$ 500."} {
		if !strings.Contains(txt, s) {
			t.Errorf("faltou %q em:\n%s", s, txt)
		}
	}
}
