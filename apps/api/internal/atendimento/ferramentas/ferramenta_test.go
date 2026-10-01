package ferramentas

import (
	"context"
	"encoding/json"
	"testing"

	"schumacher-tur/api/internal/atendimento/llm"
)

type falsa struct {
	nome string
	out  Saida
}

func (f falsa) Def() llm.DefFerramenta {
	return llm.DefFerramenta{Nome: f.nome, Parametros: json.RawMessage(`{"type":"object"}`)}
}

func (f falsa) Executar(context.Context, *Contexto, json.RawMessage) Saida { return f.out }

func TestRegistroExecutaEListaNaOrdem(t *testing.T) {
	r := NovoRegistro(falsa{"b", Saida{OK: true, Dados: 1}}, falsa{"a", Saida{OK: false, Motivo: "x"}})
	defs := r.Defs()
	if len(defs) != 2 || defs[0].Nome != "b" || defs[1].Nome != "a" {
		t.Fatalf("ordem inesperada: %+v", defs)
	}
	if s := r.Executar(context.Background(), &Contexto{}, "b", nil); !s.OK {
		t.Fatalf("b deveria ser OK: %+v", s)
	}
	if s := r.Executar(context.Background(), &Contexto{}, "a", nil); s.OK || s.Motivo != "x" {
		t.Fatalf("a inesperado: %+v", s)
	}
}

func TestRegistroFerramentaDesconhecida(t *testing.T) {
	s := NovoRegistro().Executar(context.Background(), &Contexto{}, "nada", nil)
	if s.OK || s.Motivo != "ferramenta_desconhecida" {
		t.Fatalf("got %+v", s)
	}
}

func TestRegistroNomeRepetidoSubstitui(t *testing.T) {
	r := NovoRegistro(falsa{"a", Saida{Motivo: "1"}}, falsa{"a", Saida{Motivo: "2"}})
	if len(r.Defs()) != 1 {
		t.Fatal("esperava 1 def")
	}
	if s := r.Executar(context.Background(), &Contexto{}, "a", nil); s.Motivo != "2" {
		t.Fatalf("got %+v", s)
	}
}
