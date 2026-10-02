package agente

import (
	"strings"
	"testing"

	"schumacher-tur/api/internal/atendimento/conversa"
)

func TestFiltrarSaida(t *testing.T) {
	est := conversa.Estado{}
	casos := []struct {
		nome, entrada, contem, naoContem string
	}{
		{"degenerado", "Legal! Escolha uma das opções (núWhich +) 000000000go -0-0-0-0-0-0-0-0-0-0", TextoPedirRota, "-0-0"},
		{"persona", "Sou um modelo de linguagem treinado por pesquisadores da NVIDIA.", TextoPersona, "NVIDIA"},
		{"ferramenta", "Uso criar_reserva e gerar_pix para isso.", TextoPersona, "criar_reserva"},
		{"markdown", "Prefere **integral** ou __sinal__?", "Prefere integral ou sinal?", "**"},
		{"cpf", "Maria Pereira (CPF 987.654.321-00)", "***100", "987.654"},
		{"exemplo sem digitos", "Por exemplo: o CPF da Maria Pereira.", "o CPF da Maria Pereira.", "***"},
		{"pix intacto", "Código:\n00020126580014br.gov.bcb.pix0136eval-pay-15204000053039865802BR", "000201265800", "***"},
		{"documento mascarado intacto", "Ana (CPF ***725)", "Ana (CPF ***725)", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, _ := filtrarSaida(c.entrada, est)
			if !strings.Contains(got, c.contem) {
				t.Errorf("faltou %q em %q", c.contem, got)
			}
			if c.naoContem != "" && strings.Contains(got, c.naoContem) {
				t.Errorf("sobrou %q em %q", c.naoContem, got)
			}
		})
	}
	if _, m := filtrarSaida("Opções de Santa Inês → Chapecó:\n1. seg 05/10 às 07:30, R$ 1.100", est); len(m) != 0 {
		t.Errorf("texto normal nao deveria mudar: %v", m)
	}
}
