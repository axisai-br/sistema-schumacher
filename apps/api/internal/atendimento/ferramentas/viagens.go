package ferramentas

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	e.Opcoes = abertas
	if e.Viagem != nil && e.ReservaID == "" {
		mantem := false
		for _, o := range abertas {
			if mesmaViagem(o, *e.Viagem) {
				mantem = true
				break
			}
		}
		if !mantem {
			e.Viagem = nil
		}
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
		Descricao: "Registra a viagem que o cliente escolheu entre as opcoes numeradas devolvidas por buscar_viagens. " +
			"Use quando o cliente disser qual opcao quer (numero, data ou horario). Revalida as vagas antes de gravar.",
		Parametros: defJSON(`{
  "type":"object",
  "properties":{"opcao":{"type":"integer","minimum":1,"description":"Numero da opcao conforme buscar_viagens."}},
  "required":["opcao"],
  "additionalProperties":false
}`),
	}
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
		Opcao inteiro `json:"opcao"`
	}
	if s := lerArgs(raw, &a); s != nil {
		return *s
	}
	e := c.Estado
	if e.ReservaID != "" {
		return falha("reserva_ja_criada", "Ja existe reserva criada nesta conversa; para trocar de viagem transfira para um atendente.")
	}
	var op *conversa.Opcao
	var validas []int
	for i := range e.Opcoes {
		validas = append(validas, e.Opcoes[i].Numero)
		if e.Opcoes[i].Numero == int(a.Opcao) {
			op = &e.Opcoes[i]
		}
	}
	if op == nil {
		return falhaDados("opcao_inexistente", map[string]any{
			"opcoes_validas": validas,
			"mensagem":       "Essa opcao nao existe. Use um numero de opcoes_validas ou rode buscar_viagens de novo.",
		})
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
	e.Viagem = &escolhida
	dados["viagem"] = viewDe(escolhida)
	dados["pendencias"] = pendenciasDados(*e)
	return sucesso(dados)
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
