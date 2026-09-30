package ferramentas

import (
	"context"
	"encoding/json"

	"schumacher-tur/api/internal/atendimento/llm"
)

type listarRotas struct{ cat *Catalogo }

func (t *listarRotas) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "listar_rotas",
		Descricao: "Lista as cidades atendidas pela Schumacher Tur, por estado (MA e SC), com o preco base (menor preco ativo) de cada uma. " +
			"Use quando o cliente perguntar quais cidades/rotas existem ou quanto custa 'a partir de'. " +
			"Nao use para saber datas ou vagas: para isso use buscar_viagens.",
		Parametros: defJSON(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t *listarRotas) Executar(ctx context.Context, c *Contexto, _ json.RawMessage) Saida {
	cidades, err := t.cat.Cidades(ctx)
	if err != nil {
		return falha("catalogo_indisponivel", "Nao consegui carregar as cidades agora. Tente de novo ou transfira para um atendente.")
	}
	ordem, grupos := gruposPorUF(cidades)
	estados := make([]map[string]any, 0, len(ordem))
	for _, uf := range ordem {
		lista := make([]map[string]any, 0, len(grupos[uf]))
		for _, ci := range grupos[uf] {
			item := map[string]any{"nome": ci.Nome}
			if ci.PrecoBase > 0 {
				item["preco_base"] = ci.PrecoBase
			}
			lista = append(lista, item)
		}
		estados = append(estados, map[string]any{"uf": uf, "estado": nomeUF(uf), "cidades": lista})
	}
	return sucesso(map[string]any{
		"estados":  estados,
		"mensagem": "Viagens nos dois sentidos entre as cidades de MA e SC. Preco base e 'a partir de'; o valor final vem de buscar_viagens.",
	})
}
