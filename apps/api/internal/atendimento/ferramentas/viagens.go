package ferramentas

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/availability"
	"schumacher-tur/api/internal/pricing"
)

const (
	janelaPadraoDias = 60
	limiteOpcoes     = 10
)

type opcaoView struct {
	Numero    int     `json:"numero"`
	Origem    string  `json:"origem"`
	Destino   string  `json:"destino"`
	Data      string  `json:"data"`
	DiaSemana string  `json:"dia_semana,omitempty"`
	Horario   string  `json:"horario"`
	Preco     float64 `json:"preco"`
	Vagas     int     `json:"vagas"`
	Pacote    string  `json:"pacote,omitempty"`
}

func viewDe(o conversa.Opcao) opcaoView {
	return opcaoView{Numero: o.Numero, Origem: o.Origem, Destino: o.Destino, Data: o.Data,
		DiaSemana: diaSemana(o.Data), Horario: o.Horario, Preco: o.Preco, Vagas: o.Vagas, Pacote: o.Pacote}
}

func nomesCidades(cs []Cidade) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Nome)
	}
	return out
}

func parseData(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("data invalida: %q", s)
}

// dataLocal devolve a meia-noite (UTC) do dia de t no fuso dado.
func dataLocal(t time.Time, fuso *time.Location) time.Time {
	l := t.In(fuso)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

func nomeSemUF(display string) string {
	n, _ := separarNomeUF(display)
	return n
}

func opcaoDeResultado(r availability.SearchResult) conversa.Opcao {
	return conversa.Opcao{
		TripID:       r.TripID,
		BoardStopID:  r.BoardStopID,
		AlightStopID: r.AlightStopID,
		Origem:       nomeSemUF(r.OriginDisplayName),
		Destino:      nomeSemUF(r.DestinationDisplayName),
		Data:         r.TripDate,
		Horario:      r.OriginDepartTime,
		Preco:        r.Price,
		Vagas:        r.SeatsAvailable,
		Pacote:       r.PackageName,
	}
}

func mesmaViagem(a, b conversa.Opcao) bool {
	return a.TripID == b.TripID && a.BoardStopID == b.BoardStopID && a.AlightStopID == b.AlightStopID
}

// ---- buscar_viagens ----

type buscarViagens struct {
	cat  *Catalogo
	b    Buscador
	fuso *time.Location
}

func (t *buscarViagens) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "buscar_viagens",
		Descricao: "Busca viagens disponiveis (data, horario, preco, vagas). Use assim que o cliente disser a origem OU o destino, sem pedir data nem quantidade antes: " +
			"mostre as opcoes e refine depois. Precisa de pelo menos origem ou destino (nome da cidade como o cliente disse). " +
			"Para a VOLTA (ou outra viagem) busque de novo com origem e destino invertidos: isso nao apaga os trechos ja escolhidos. " +
			"Sem datas busca de hoje ate 60 dias. Se pessoas for informado, so devolve viagens com vagas suficientes. " +
			"Resultado vem numerado (opcao 1..n); depois o cliente escolhe e voce chama escolher_viagem. Nunca cite viagem, data ou preco que nao venham daqui.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{
    "origem":{"type":"string","description":"Cidade de origem como o cliente disse, ex: 'Videira' ou 'Santa Ines'. Omita se nao informada."},
    "destino":{"type":"string","description":"Cidade de destino como o cliente disse. Omita se nao informada."},
    "data_de":{"type":"string","description":"Data inicial AAAA-MM-DD. Sozinha, busca so esse dia."},
    "data_ate":{"type":"string","description":"Data final AAAA-MM-DD."},
    "pessoas":{"type":"integer","minimum":1,"description":"Quantidade de passageiros, se o cliente ja disse."}
  },
  "additionalProperties":false
}`),
	}
}

type argsBuscar struct {
	Origem  string  `json:"origem"`
	Destino string  `json:"destino"`
	DataDe  string  `json:"data_de"`
	DataAte string  `json:"data_ate"`
	Pessoas inteiro `json:"pessoas"`
}

// resolverLado resolve o texto de origem/destino. Retorna a parada ou uma Saida de falha.
func resolverLado(ctx context.Context, cat *Catalogo, texto string) (*conversa.Parada, *Saida) {
	if uf, ok := cat.EstadoDoTexto(texto); ok {
		cs, _ := cat.CidadesDaUF(ctx, uf)
		s := falhaDados("informe_a_cidade", map[string]any{
			"estado_informado": nomeUF(uf),
			"cidades":          nomesCidades(cs),
			"mensagem":         "O cliente falou so o estado. Pergunte qual cidade, oferecendo as cidades listadas.",
		})
		return nil, &s
	}
	p, ok := cat.ResolverCidade(ctx, texto)
	if !ok {
		todas, _ := cat.Cidades(ctx)
		s := falhaDados("cidade_nao_atendida", map[string]any{
			"cidade_informada":  texto,
			"cidades_atendidas": nomesCidades(todas),
			"mensagem":          "Essa cidade nao e atendida. Diga isso uma vez, liste as cidades atendidas e ofereca o suporte humano.",
		})
		return nil, &s
	}
	return &p, nil
}

func (t *buscarViagens) Executar(ctx context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a argsBuscar
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	origemTxt, destinoTxt := strings.TrimSpace(a.Origem), strings.TrimSpace(a.Destino)
	if origemTxt == "" && destinoTxt == "" {
		return falha("informe_origem_ou_destino", "Preciso de pelo menos a cidade de origem ou a de destino para buscar.")
	}
	if _, err := t.cat.Cidades(ctx); err != nil {
		return falha("catalogo_indisponivel", "Nao consegui carregar as cidades agora. Tente de novo ou transfira para um atendente.")
	}
	var origem, destino *conversa.Parada
	if origemTxt != "" {
		p, s := resolverLado(ctx, t.cat, origemTxt)
		if s != nil {
			return *s
		}
		origem = p
	}
	if destinoTxt != "" {
		p, s := resolverLado(ctx, t.cat, destinoTxt)
		if s != nil {
			return *s
		}
		destino = p
	}
	if origem != nil && destino != nil && origem.StopID == destino.StopID {
		return falha("origem_igual_destino", "Origem e destino sao a mesma cidade. Pergunte de onde para onde o cliente quer ir.")
	}
	// So ha viagens entre MA e SC: dentro do mesmo estado nao existe em
	// nenhuma data (nao sugira outra data).
	if origem != nil && destino != nil && origem.UF != "" && origem.UF == destino.UF {
		return falhaDados("rota_mesmo_estado", map[string]any{
			"estado":   nomeUF(origem.UF),
			"mensagem": "Nao existe viagem entre duas cidades do mesmo estado, em nenhuma data. Diga isso uma vez, explique que as viagens sao entre Maranhao e Santa Catarina e ofereca o suporte humano. Nao sugira outra data.",
		})
	}

	hoje := dataLocal(c.Agora, t.fuso)
	de, ate := hoje, hoje.AddDate(0, 0, janelaPadraoDias)
	temDatas := strings.TrimSpace(a.DataDe) != "" || strings.TrimSpace(a.DataAte) != ""
	if strings.TrimSpace(a.DataDe) != "" {
		d, err := parseData(a.DataDe)
		if err != nil {
			return falha("data_invalida", "Use datas no formato AAAA-MM-DD.")
		}
		de = d
		ate = d
	}
	if strings.TrimSpace(a.DataAte) != "" {
		d, err := parseData(a.DataAte)
		if err != nil {
			return falha("data_invalida", "Use datas no formato AAAA-MM-DD.")
		}
		ate = d
	}
	if ate.Before(hoje) {
		return falha("data_no_passado", "Essa data ja passou. Pergunte outra data ou busque sem data.")
	}
	if de.Before(hoje) {
		de = hoje
	}
	if ate.Before(de) {
		return falha("periodo_invalido", "A data final e anterior a inicial.")
	}

	pessoas := int(a.Pessoas)
	if pessoas < 0 {
		pessoas = 0
	}
	base := availability.SearchFilter{OnlyActive: true}
	if origem != nil {
		base.OriginStopID = origem.StopID
	}
	if destino != nil {
		base.DestinationStopID = destino.StopID
	}
	f := base
	f.DateFrom, f.DateTo, f.Limit = &de, &ate, limiteOpcoes
	res, err := t.b.Search(ctx, f)
	if err != nil {
		return falha("erro_busca", "A busca falhou agora. Tente de novo uma vez; se repetir, transfira para um atendente.")
	}

	e := c.Estado
	e.Origem, e.Destino = origem, destino
	if pessoas > 0 {
		e.PessoasInformadas = pessoas
	}

	precisa := max(pessoas, 1)
	var abertas []conversa.Opcao
	var lotadas []map[string]any
	for _, r := range res {
		o := opcaoDeResultado(r)
		if o.Vagas >= precisa {
			o.Numero = len(abertas) + 1
			abertas = append(abertas, o)
		} else {
			lotadas = append(lotadas, map[string]any{"data": o.Data, "horario": o.Horario, "vagas": o.Vagas})
		}
	}
	// Os trechos ja escolhidos (e.Trechos) nao dependem desta busca: buscar a
	// volta nao pode apagar a ida. So escolher_viagem/remover_trecho mexem neles.
	e.Opcoes = abertas
	if origem != nil && destino != nil && len(abertas) > 0 {
		e.RegistrarRotaBuscada(resumoRota(origem.Nome, destino.Nome, abertas))
	}

	if len(abertas) > 0 {
		views := make([]opcaoView, len(abertas))
		for i, o := range abertas {
			views[i] = viewDe(o)
		}
		dados := map[string]any{"opcoes": views, "total": len(views)}
		if pessoas > 0 {
			dados["pessoas"] = pessoas
		}
		if len(lotadas) > 0 {
			dados["sem_vaga_para_pessoas_count"] = len(lotadas)
		}
		return sucesso(dados)
	}

	if len(res) > 0 {
		return sucesso(map[string]any{
			"opcoes":                []opcaoView{},
			"sem_vaga_para_pessoas": true,
			"pessoas":               precisa,
			"viagens_lotadas":       lotadas,
			"mensagem":              fmt.Sprintf("Ha viagens nesse periodo, mas nenhuma com %d vaga(s). Ofereca outra data ou fale com um atendente.", precisa),
		})
	}

	dados := map[string]any{"opcoes": []opcaoView{}, "mensagem": "sem viagens nesse período"}
	// Sugestao: proxima data disponivel, sem limite de data final.
	g := base
	g.DateFrom, g.Limit = &hoje, 1
	if prox, err := t.b.Search(ctx, g); err == nil && len(prox) > 0 {
		o := opcaoDeResultado(prox[0])
		dados["proxima_data_disponivel"] = map[string]any{
			"origem": o.Origem, "destino": o.Destino, "data": o.Data, "dia_semana": diaSemana(o.Data), "horario": o.Horario,
		}
		if temDatas {
			dados["sugestao"] = "Nao ha viagens nas datas pedidas; informe ao cliente a proxima data disponivel (proxima_data_disponivel)."
		} else {
			dados["sugestao"] = "Nao ha viagens nos proximos 60 dias; informe a proxima data disponivel (proxima_data_disponivel)."
		}
	}
	return sucesso(dados)
}

// ---- escolher_viagem ----

type escolherViagem struct {
	cat  *Catalogo
	b    Buscador
	q    Cotador
	fuso *time.Location
}

func (t *escolherViagem) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "escolher_viagem",
		Descricao: "ADICIONA um trecho (viagem) a compra. Identifique a viagem por opcao (numero da ULTIMA busca) OU por origem + destino + data (+ horario se houver mais de uma no dia); " +
			"prefira origem/destino/data quando o cliente escolhe por data ou quando a viagem e de uma busca anterior (ex.: ida e volta: duas chamadas, uma por trecho, sem precisar buscar de novo). " +
			"Use quando o cliente disser qual opcao quer (numero, data ou horario). Ida e volta (ou varias viagens, ate 4) sao trechos da mesma compra: " +
			"chame uma vez por trecho, mesmo que um trecho anterior ja tenha reserva ou PIX. A mesma viagem nao entra duas vezes. " +
			"Para TROCAR um trecho que ainda nao tem reserva por outra viagem, informe substituir_trecho (numero do trecho, 1..n). " +
			"Revalida as vagas antes de gravar.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{
    "opcao":{"type":"integer","minimum":1,"description":"Numero da opcao conforme a ULTIMA buscar_viagens."},
    "origem":{"type":"string","description":"Cidade de partida (use com destino e data, no lugar de opcao)."},
    "destino":{"type":"string","description":"Cidade de chegada."},
    "data":{"type":"string","description":"Data da viagem AAAA-MM-DD."},
    "horario":{"type":"string","description":"Opcional: HH:MM, se houver mais de uma viagem no dia."},
    "substituir_trecho":{"type":"integer","minimum":1,"description":"Opcional: numero do trecho (sem reserva) a trocar por esta opcao, em vez de adicionar."}
  },
  "additionalProperties":false
}`),
	}
}

// porRotaEData acha a viagem pela rota e data (sem depender do numero da
// ultima busca). Varias no dia sem horario: devolve as candidatas.
func (t *escolherViagem) porRotaEData(ctx context.Context, origemTxt, destinoTxt, dataTxt, horario string) (conversa.Opcao, *Saida) {
	if strings.TrimSpace(origemTxt) == "" || strings.TrimSpace(destinoTxt) == "" || strings.TrimSpace(dataTxt) == "" {
		s := falha("viagem_nao_identificada", "Informe opcao (numero da ultima busca) ou origem, destino e data (AAAA-MM-DD).")
		return conversa.Opcao{}, &s
	}
	origem, s := resolverLado(ctx, t.cat, origemTxt)
	if s != nil {
		return conversa.Opcao{}, s
	}
	destino, s := resolverLado(ctx, t.cat, destinoTxt)
	if s != nil {
		return conversa.Opcao{}, s
	}
	d, err := parseData(dataTxt)
	if err != nil {
		x := falha("data_invalida", "Use a data no formato AAAA-MM-DD.")
		return conversa.Opcao{}, &x
	}
	res, err := t.b.Search(ctx, availability.SearchFilter{
		OriginStopID: origem.StopID, DestinationStopID: destino.StopID,
		DateFrom: &d, DateTo: &d, OnlyActive: true, Limit: 50,
	})
	if err != nil {
		x := falha("erro_busca", "A busca falhou agora. Tente de novo; se repetir, transfira para um atendente.")
		return conversa.Opcao{}, &x
	}
	var cands []conversa.Opcao
	for _, r := range res {
		o := opcaoDeResultado(r)
		if h := strings.TrimSpace(horario); h != "" && !strings.HasPrefix(o.Horario, h) {
			continue
		}
		cands = append(cands, o)
	}
	switch len(cands) {
	case 0:
		x := falha("sem_viagem_na_data", "Nao ha viagem dessa rota nessa data. Rode buscar_viagens para mostrar as datas disponiveis.")
		return conversa.Opcao{}, &x
	case 1:
		return cands[0], nil
	}
	vs := make([]opcaoView, 0, len(cands))
	for _, o := range cands {
		vs = append(vs, viewDe(o))
	}
	x := falhaDados("varias_viagens_no_dia", map[string]any{"viagens": vs, "mensagem": "Ha mais de uma viagem nesse dia. Pergunte o horario ao cliente e chame de novo com horario."})
	return conversa.Opcao{}, &x
}

func (t *escolherViagem) stopID(ctx context.Context, nome string, cands ...*conversa.Parada) string {
	n := normalizar(nome)
	for _, p := range cands {
		if p != nil && normalizar(p.Nome) == n {
			return p.StopID
		}
	}
	if p, ok := t.cat.ResolverCidade(ctx, nome); ok {
		return p.StopID
	}
	return ""
}

func (t *escolherViagem) Executar(ctx context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Opcao            inteiro `json:"opcao"`
		SubstituirTrecho inteiro `json:"substituir_trecho"`
		Origem           string  `json:"origem"`
		Destino          string  `json:"destino"`
		Data             string  `json:"data"`
		Horario          string  `json:"horario"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	e := c.Estado
	subst := -1 // indice do trecho a substituir
	if a.SubstituirTrecho != 0 {
		i := int(a.SubstituirTrecho) - 1
		if i < 0 || i >= len(e.Trechos) {
			return falhaDados("trecho_inexistente", map[string]any{
				"trechos":  trechosView(*e),
				"mensagem": "Esse trecho nao existe. Use o numero de um trecho de trechos.",
			})
		}
		if e.Trechos[i].ReservaID != "" {
			return falha("trecho_ja_reservado", "Esse trecho ja tem reserva criada; para trocar a viagem transfira para um atendente.")
		}
		subst = i
	} else if len(e.Trechos) >= conversa.MaxTrechos {
		return falha("limite_de_trechos", fmt.Sprintf("O limite e de %d trechos por conversa. Para mais viagens, transfira para um atendente.", conversa.MaxTrechos))
	}
	var op *conversa.Opcao
	var validas []int
	// Rota + data identificam a viagem sem ambiguidade e tem prioridade sobre o
	// numero (que so vale para a ultima busca).
	porData := strings.TrimSpace(a.Origem) != "" && strings.TrimSpace(a.Destino) != "" && strings.TrimSpace(a.Data) != ""
	if porData || a.Opcao == 0 {
		o, s := t.porRotaEData(ctx, a.Origem, a.Destino, a.Data, a.Horario)
		if s != nil {
			return *s
		}
		op = &o
	}
	for i := range e.Opcoes {
		validas = append(validas, e.Opcoes[i].Numero)
		if !porData && a.Opcao != 0 && e.Opcoes[i].Numero == int(a.Opcao) {
			op = &e.Opcoes[i]
		}
	}
	if op == nil {
		return falhaDados("opcao_inexistente", map[string]any{
			"opcoes_validas": validas,
			"mensagem":       "Essa opcao nao existe. Use um numero de opcoes_validas ou rode buscar_viagens de novo.",
		})
	}
	for i, tr := range e.Trechos {
		if i != subst && mesmaViagem(tr.Viagem, *op) {
			return falhaDados("trecho_duplicado", map[string]any{
				"trecho":   i + 1,
				"mensagem": "Essa viagem ja esta na compra. Se o cliente quer outra, mostre as opcoes; se e a volta, busque com origem e destino invertidos.",
			})
		}
	}

	origemID := t.stopID(ctx, op.Origem, e.Origem)
	destinoID := t.stopID(ctx, op.Destino, e.Destino)
	if origemID == "" || destinoID == "" {
		return falha("viagem_indisponivel", "Nao consegui revalidar essa viagem. Rode buscar_viagens de novo.")
	}
	d, err := parseData(op.Data)
	if err != nil {
		return falha("viagem_indisponivel", "Data da opcao invalida. Rode buscar_viagens de novo.")
	}
	res, err := t.b.Search(ctx, availability.SearchFilter{
		OriginStopID: origemID, DestinationStopID: destinoID,
		DateFrom: &d, DateTo: &d, OnlyActive: true, Limit: 50,
	})
	if err != nil {
		return falha("erro_busca", "A revalidacao falhou agora. Tente de novo; se repetir, transfira para um atendente.")
	}
	var atual *conversa.Opcao
	for _, r := range res {
		o := opcaoDeResultado(r)
		if mesmaViagem(o, *op) {
			o.Numero = op.Numero
			atual = &o
			break
		}
	}
	if atual == nil {
		return falha("viagem_indisponivel", "Essa viagem nao esta mais disponivel. Rode buscar_viagens de novo e mostre as opcoes atualizadas.")
	}
	precisa := max(e.PessoasInformadas, len(e.Passageiros), 1)
	if atual.Vagas < precisa {
		*op = *atual
		return falhaDados("sem_vagas_suficientes", map[string]any{
			"vagas":    atual.Vagas,
			"precisa":  precisa,
			"mensagem": "Restam menos vagas do que o necessario. Informe ao cliente e ofereca outra opcao.",
		})
	}
	cot, err := cotar(ctx, t.q, *atual)
	if err != nil {
		return falha("erro_cotacao", "Nao consegui confirmar o preco agora. Tente de novo; se repetir, transfira para um atendente.")
	}
	dados := map[string]any{}
	if diferePreco(cot, atual.Preco) {
		dados["preco_anterior"] = atual.Preco
		dados["preco_atualizado"] = cot
		atual.Preco = cot
	}
	*op = *atual
	escolhida := *atual
	n := subst + 1
	if subst >= 0 {
		e.Trechos[subst] = conversa.Trecho{Viagem: escolhida}
		dados["substituiu"] = true
	} else {
		e.Trechos = append(e.Trechos, conversa.Trecho{Viagem: escolhida})
	}
	// Trechos em ordem cronologica: o 1o e sempre a ida.
	sort.SliceStable(e.Trechos, func(i, j int) bool {
		a, b := e.Trechos[i].Viagem, e.Trechos[j].Viagem
		return a.Data+a.Horario < b.Data+b.Horario
	})
	for i, tr := range e.Trechos {
		if mesmaViagem(tr.Viagem, escolhida) {
			n = i + 1
		}
	}
	dados["trecho"] = n
	dados["viagem"] = viewDe(escolhida)
	dados["trechos"] = trechosView(*e)
	dados["pendencias"] = pendenciasDados(*e)
	return sucesso(dados)
}

// trechoView e o trecho como o modelo o ve (sem IDs internos de viagem).
type trechoView struct {
	Trecho  int     `json:"trecho"`
	Rota    string  `json:"rota"`
	Data    string  `json:"data"`
	Horario string  `json:"horario"`
	Preco   float64 `json:"preco"`
	Reserva string  `json:"reserva,omitempty"` // "criada" quando ha reserva
	PIX     string  `json:"pix,omitempty"`     // "gerado" quando ha PIX
}

func trechosView(e conversa.Estado) []trechoView {
	out := make([]trechoView, len(e.Trechos))
	for i, t := range e.Trechos {
		v := trechoView{Trecho: i + 1, Rota: t.Rota(), Data: t.Viagem.Data, Horario: t.Viagem.Horario, Preco: t.Viagem.Preco}
		if t.ReservaID != "" {
			v.Reserva = "criada"
		}
		if t.PagamentoID != "" {
			v.PIX = "gerado"
		}
		out[i] = v
	}
	return out
}

// ---- remover_trecho ----

type removerTrecho struct{}

func (t *removerTrecho) Def() llm.DefFerramenta {
	return llm.DefFerramenta{
		Nome: "remover_trecho",
		Descricao: "Remove da compra um trecho (viagem) que o cliente desistiu e que ainda NAO tem reserva. " +
			"Trecho ja reservado nao pode ser removido aqui: transfira para um atendente.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"trecho":{"type":"integer","minimum":1,"description":"Numero do trecho (1..n) conforme trechos do estado."}},
  "required":["trecho"],
  "additionalProperties":false
}`),
	}
}

func (t *removerTrecho) Executar(_ context.Context, c *Contexto, raw json.RawMessage) Saida {
	var a struct {
		Trecho inteiro `json:"trecho"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	e := c.Estado
	i := int(a.Trecho) - 1
	if i < 0 || i >= len(e.Trechos) {
		return falhaDados("trecho_inexistente", map[string]any{
			"trechos":  trechosView(*e),
			"mensagem": "Esse trecho nao existe. Use o numero de um trecho de trechos.",
		})
	}
	if e.Trechos[i].ReservaID != "" {
		return falha("trecho_ja_reservado", "Esse trecho ja tem reserva criada; para remover transfira para um atendente.")
	}
	removido := e.Trechos[i]
	e.Trechos = append(e.Trechos[:i:i], e.Trechos[i+1:]...)
	return sucesso(map[string]any{
		"removido":   removido.Rota(),
		"trechos":    trechosView(*e),
		"pendencias": pendenciasDados(*e),
	})
}

// cotar devolve o preco unitario real (pricing, FareMode AUTO), como bookings.Create.
func cotar(ctx context.Context, q Cotador, o conversa.Opcao) (float64, error) {
	r, err := q.Quote(ctx, pricing.QuoteInput{TripID: o.TripID, BoardStopID: o.BoardStopID, AlightStopID: o.AlightStopID, FareMode: "AUTO"})
	if err != nil {
		return 0, err
	}
	return r.FinalAmount, nil
}

func diferePreco(a, b float64) bool { return math.Abs(a-b) > 0.01 }

// resumoRota descreve a rota buscada com os dias da semana e horarios das
// opcoes (ex.: "Fraiburgo → Monção: quinta-feira 16:30"), para o modelo
// responder perguntas gerais sem confundir ida e volta.
func resumoRota(origem, destino string, os []conversa.Opcao) string {
	var partes []string
	visto := map[string]bool{}
	for _, o := range os {
		k := diaSemana(o.Data) + " " + o.Horario
		if !visto[k] {
			visto[k] = true
			partes = append(partes, k)
		}
	}
	return origem + " → " + destino + ": " + strings.Join(partes, ", ")
}
