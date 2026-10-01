package ferramentas

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/llm"
)

// interpretarData converte uma expressao de data do cliente em datas exatas.
type interpretarData struct {
	fuso *time.Location
}

func (t *interpretarData) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "interpretar_data",
		Descricao: "Converte o que o cliente disse sobre QUANDO viajar em datas exatas (AAAA-MM-DD), calculadas a partir de hoje: " +
			"'amanha', 'daqui 15 dias', 'semana que vem', 'mes que vem', 'quinta que vem', 'fim de outubro', 'dia 12', '15/10'. " +
			"Use antes de citar uma data calculada ou para escolher_viagem por data. Para buscar viagens prefira buscar_viagens com quando.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"expressao":{"type":"string","description":"O trecho do cliente sobre a data, como ele escreveu."}},
  "required":["expressao"],
  "additionalProperties":false
}`),
	}
}

func (t *interpretarData) Executar(_ context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Expressao string `json:"expressao"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	p, ok := ResolverQuando(strings.TrimSpace(a.Expressao), dataLocal(c.Agora, t.fuso))
	if !ok {
		return falha("data_nao_entendida", "Nao consegui entender essa data. Pergunte ao cliente o dia (ex.: 'dia 15' ou '15/10').")
	}
	d := map[string]any{
		"data_de":   p.De.Format("2006-01-02"),
		"data_ate":  p.Ate.Format("2006-01-02"),
		"descricao": p.Descricao,
	}
	if p.De.Equal(p.Ate) {
		d["dia_semana"] = diaSemana(p.De.Format("2006-01-02"))
	}
	return sucesso(d)
}
