package agente

import (
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/ferramentas"
)

func TestTextoPrecos(t *testing.T) {
	cidades := []ferramentas.Cidade{
		{Nome: "Santa Inês", UF: "MA", PrecoBase: 950}, {Nome: "Monção", UF: "MA", PrecoBase: 950},
		{Nome: "Fraiburgo", UF: "SC", PrecoBase: 950}, {Nome: "Videira", UF: "SC", PrecoBase: 950},
		{Nome: "Campos Novos", UF: "SC", PrecoBase: 1000}, {Nome: "Chapecó", UF: "SC", PrecoBase: 1100},
	}
	got := textoPrecos(cidades)
	for _, s := range []string{"R$ 950 para Fraiburgo e Videira", "R$ 1.000 para Campos Novos", "R$ 1.100 para Chapecó", "As mais baratas são Fraiburgo e Videira", "Santa Inês e Monção"} {
		if !strings.Contains(got, s) {
			t.Errorf("faltou %q em %q", s, got)
		}
	}
	if ass := assuntoDoTexto("qual a mais barata de sc para ma?"); ass != "precos" {
		t.Errorf("assunto=%q", ass)
	}
	if ass := assuntoDoTexto("todas são o mesmo preço?"); ass != "precos" {
		t.Errorf("assunto=%q", ass)
	}
}
