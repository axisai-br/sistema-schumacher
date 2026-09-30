package ferramentas

import (
	"context"
	"encoding/json"
	"strings"

	"schumacher-tur/api/internal/atendimento/llm"
)

type transferirParaHumano struct{}

func (t *transferirParaHumano) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "transferir_para_humano",
		Descricao: "Transfere a conversa para um atendente humano. Use quando o cliente pedir ajuda/atendente/humano, estiver irritado, insistir em assunto fora de viagens, " +
			"tiver uma duvida que as ferramentas nao resolvem, ou quando uma ferramenta falhar repetidamente. Informe o motivo em uma frase.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"motivo":{"type":"string","description":"Motivo da transferencia, em uma frase."}},
  "required":["motivo"],
  "additionalProperties":false
}`),
	}
}

func (t *transferirParaHumano) Executar(_ context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Motivo string `json:"motivo"`
	}
	if s := lerArgs(raw, &a); s != nil {
		a.Motivo = ""
	}
	motivo := strings.TrimSpace(a.Motivo)
	if motivo == "" {
		motivo = "transferencia solicitada"
	}
	if r := []rune(motivo); len(r) > 300 {
		motivo = string(r[:300])
	}
	c.Estado.MotivoHumano = motivo
	return Saida{OK: true, Transferir: true, Dados: map[string]any{"mensagem": "Conversa transferida para um atendente humano."}}
}
