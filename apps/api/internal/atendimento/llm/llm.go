// Package llm define o contrato do modelo de linguagem usado pelo agente.
package llm

import (
	"context"
	"encoding/json"
)

type Papel string

const (
	PapelSistema    Papel = "system"
	PapelUsuario    Papel = "user"
	PapelAssistente Papel = "assistant"
	PapelFerramenta Papel = "tool"
)

type ChamadaFerramenta struct {
	ID         string
	Nome       string
	Argumentos json.RawMessage
}

type Mensagem struct {
	Papel     Papel
	Texto     string
	Chamadas  []ChamadaFerramenta // so assistente
	ChamadaID string              // so ferramenta
}

type DefFerramenta struct {
	Nome, Descricao string
	Parametros      json.RawMessage // JSON Schema object
}

type Pedido struct {
	Modelo      string
	Instrucoes  string
	Mensagens   []Mensagem
	Ferramentas []DefFerramenta
	SaidaJSON   json.RawMessage // opcional: JSON schema para saida estruturada
	MaxTokens   int
}

type Resposta struct {
	Texto                      string
	Chamadas                   []ChamadaFerramenta
	TokensEntrada, TokensSaida int
	Modelo                     string
}

type Modelo interface {
	Gerar(ctx context.Context, p Pedido) (Resposta, error)
}
