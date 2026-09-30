// Package ferramentas define as ferramentas tipadas que o agente pode chamar.
package ferramentas

import (
	"context"
	"encoding/json"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
)

type Contexto struct {
	Conversa conversa.Conversa
	Estado   *conversa.Estado
	Agora    time.Time
}

type Saida struct {
	OK         bool   `json:"ok"`
	Dados      any    `json:"dados,omitempty"`
	Motivo     string `json:"motivo,omitempty"`
	Transferir bool   `json:"-"`
}

type Ferramenta interface {
	Def() llm.DefFerramenta
	Executar(ctx context.Context, c *Contexto, args json.RawMessage) Saida
}

type Registro struct {
	itens map[string]Ferramenta
	ordem []string
}

// NovoRegistro monta o registro na ordem dada. Nome repetido substitui a
// ferramenta anterior mantendo a posicao original.
func NovoRegistro(fs ...Ferramenta) *Registro {
	r := &Registro{itens: make(map[string]Ferramenta, len(fs))}
	for _, f := range fs {
		nome := f.Def().Nome
		if _, ok := r.itens[nome]; !ok {
			r.ordem = append(r.ordem, nome)
		}
		r.itens[nome] = f
	}
	return r
}

func (r *Registro) Defs() []llm.DefFerramenta {
	defs := make([]llm.DefFerramenta, 0, len(r.ordem))
	for _, nome := range r.ordem {
		defs = append(defs, r.itens[nome].Def())
	}
	return defs
}

func (r *Registro) Executar(ctx context.Context, c *Contexto, nome string, args json.RawMessage) Saida {
	f, ok := r.itens[nome]
	if !ok {
		return Saida{OK: false, Motivo: "ferramenta_desconhecida"}
	}
	return f.Executar(ctx, c, args)
}
