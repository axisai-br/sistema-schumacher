// Package politica guarda as instrucoes do agente (politica.md embutida).
package politica

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed politica.md
var texto string

// Texto devolve a politica completa.
func Texto() string { return texto }

// Versao devolve um hash curto do conteudo, para logar em cada turno.
func Versao() string {
	h := sha256.Sum256([]byte(texto))
	return hex.EncodeToString(h[:])[:8]
}
