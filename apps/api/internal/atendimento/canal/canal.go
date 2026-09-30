// Package canal define o contrato dos canais de mensagem (ex.: WhatsApp via Evolution).
package canal

import (
	"context"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

// Entrada e um evento normalizado vindo do canal.
type Entrada struct {
	Contato, Telefone, Nome string
	DoProprioNumero         bool // mensagem enviada pelo celular da empresa (fromMe)
	Tipo                    conversa.Tipo
	Texto                   string
	Midia                   map[string]any
	ProvedorID              string
	RecebidaEm              time.Time
}

type Canal interface {
	Nome() string // "WHATSAPP"
	// ok=false quando o evento deve ser ignorado (status, presenca, grupo, etc).
	Normalizar(ctx context.Context, corpo []byte) (e Entrada, ok bool, err error)
	Enviar(ctx context.Context, contato string, texto string) (provedorID string, err error)
	// Baixa midia (audio/imagem/doc) como data URL, a partir de Entrada.Midia.
	BaixarMidia(ctx context.Context, e conversa.Mensagem) (dataURL string, mime string, err error)
}
