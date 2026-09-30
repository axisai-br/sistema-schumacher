package agente

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/politica"
)

var diasSemana = [...]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}

// instrucoes monta o prompt de sistema: politica + catalogo + estado + contexto.
func (a *Agente) instrucoes(tc *turno, catalogo string, agora time.Time) string {
	est, _ := json.Marshal(tc.estado)
	pend := tc.estado.Pendencias()
	pendTxt := "nenhuma"
	if len(pend) > 0 {
		pendTxt = strings.Join(pend, "; ")
	}
	nome := strings.TrimSpace(tc.c.Nome)
	if nome == "" {
		nome = "(desconhecido)"
	}
	var b strings.Builder
	b.WriteString(politica.Texto())
	b.WriteString("\n\n# CATÁLOGO\n")
	b.WriteString(catalogo)
	b.WriteString("\n\n# ESTADO DA RESERVA\n")
	b.Write(est)
	b.WriteString("\nPendências: ")
	b.WriteString(pendTxt)
	fmt.Fprintf(&b, "\n\n# CONTEXTO\nData e hora atuais: %s, %s (America/Sao_Paulo)\nNome do cliente: %s\n",
		diasSemana[agora.Weekday()], agora.Format("02/01/2006 15:04"), nome)
	return b.String()
}

// mapearHistorico converte o historico em mensagens do modelo.
func mapearHistorico(hist []conversa.Mensagem) []llm.Mensagem {
	out := make([]llm.Mensagem, 0, len(hist))
	for _, m := range hist {
		if envioFalhou(m) {
			continue // saida que nunca chegou ao cliente
		}
		texto := strings.TrimSpace(m.Texto)
		switch m.Autor {
		case conversa.AutorCliente:
			if texto == "" {
				texto = "[mensagem sem texto]"
			}
			out = append(out, llm.Mensagem{Papel: llm.PapelUsuario, Texto: texto})
		case conversa.AutorHumano:
			if texto == "" {
				continue
			}
			out = append(out, llm.Mensagem{Papel: llm.PapelAssistente, Texto: "[atendente humano] " + texto})
		default:
			if texto == "" {
				continue
			}
			out = append(out, llm.Mensagem{Papel: llm.PapelAssistente, Texto: texto})
		}
	}
	return out
}

func envioFalhou(m conversa.Mensagem) bool {
	return m.Midia != nil && m.Midia["envio_status"] == "FALHOU"
}
