package agente

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// TextoSaudacao e a resposta fixa para um cumprimento no inicio da conversa.
const TextoSaudacao = "Olá! 😊 Aqui é o Shabas, da Schumacher Tur.\n\nPra onde você quer viajar?"

// A busca previa roda mesmo quando o cliente citou data ou pessoas: sem esses
// filtros ela ainda mostra ao LLM as datas reais (evita inventar uma viagem
// "amanha"); o LLM refina com outra busca se precisar.

const idPreBusca = "pre_1"

// limiarVolta: acima disso o cliente pergunta pela volta e a busca previa usa
// a rota invertida.
const limiarVolta = 0.7

// resultadoRota e a decisao do codigo a partir da Rota.
type resultadoRota struct {
	transf   *transf        // transferir para humano
	resposta string         // resposta por template (sem LLM)
	pre      []llm.Mensagem // chamada + resultado de buscar_viagens ja executada
}

func cidadeValida(c string) bool {
	return c != "" && c != CidadeNaoInformada && c != CidadeNaoAtendida
}

// jaHouveResposta diz se alguem (bot ou humano) ja respondeu ao cliente.
func jaHouveResposta(hist []conversa.Mensagem) bool {
	for _, m := range hist {
		if envioFalhou(m) {
			continue
		}
		if m.Autor == conversa.AutorBot || m.Autor == conversa.AutorHumano {
			return true
		}
	}
	return false
}

// argsPreBusca decide se da para executar buscar_viagens antes do LLM.
func (a *Agente) argsPreBusca(rt Rota, est conversa.Estado) (map[string]string, bool) {
	lim := a.cfg.LimiarRota
	if rt.Intencao != IntencaoBuscarViagens || rt.ConfIntencao < lim {
		return nil, false
	}
	if (rt.Origem == CidadeNaoAtendida && rt.ConfOrigem >= lim) || (rt.Destino == CidadeNaoAtendida && rt.ConfDestino >= lim) {
		return nil, false // o LLM explica que a cidade nao e atendida
	}
	args := map[string]string{}
	if cidadeValida(rt.Origem) && rt.ConfOrigem >= lim {
		args["origem"] = rt.Origem
	}
	if cidadeValida(rt.Destino) && rt.ConfDestino >= lim {
		args["destino"] = rt.Destino
	}
	if len(args) == 0 || (len(args) == 2 && args["origem"] == args["destino"]) {
		return nil, false
	}
	// Com rota ja no estado, uma busca de um lado so apagaria o outro lado, e a
	// mesma rota com opcoes nao traz nada novo: fica para o LLM.
	if est.Origem != nil || est.Destino != nil {
		if len(args) < 2 {
			return nil, false
		}
		if est.Origem != nil && est.Destino != nil && len(est.Opcoes) > 0 &&
			chaveRota(est.Origem.Nome, est.Destino.Nome) == chaveRota(args["origem"], args["destino"]) {
			return nil, false
		}
	}
	return args, true
}

// rotear consulta o Roteador (falha ou timeout: ignora e segue no LLM) e decide:
// transferir, responder por template, pre-executar a busca ou seguir no LLM.
func (a *Agente) rotear(ctx context.Context, tc *turno, hist []conversa.Mensagem) resultadoRota {
	var cidades []ferramentas.Cidade
	if a.d.Cidades != nil {
		var err error
		if cidades, err = a.d.Cidades.Cidades(ctx); err != nil {
			a.d.Log.Printf("agente: cidades para o roteador: %v", err)
			cidades = nil
		}
	}
	ultimas := hist
	if len(ultimas) > 6 {
		ultimas = ultimas[len(ultimas)-6:]
	}
	t0 := a.d.Agora()
	rt, err := a.d.Roteador.Rotear(ctx, EntradaRota{Mensagens: ultimas, Estado: tc.estado, Cidades: cidades})
	saida := map[string]any{"decisao": "llm"}
	p := conversa.Passo{Tipo: "checagem", Nome: "roteador", DuracaoMS: a.d.Agora().Sub(t0).Milliseconds()}
	idx := len(tc.passos)
	finalizar := func(decisao string) {
		saida["decisao"] = decisao
		p.Saida = saida
		tc.passos[idx] = p
	}
	tc.passos = append(tc.passos, p)
	if err != nil {
		p.Erro = err.Error()
		a.d.Log.Printf("agente: roteador falhou (ignorado): %v", err)
		finalizar("llm")
		return resultadoRota{}
	}
	saida["intencao"], saida["confianca"] = rt.Intencao, rt.ConfIntencao
	saida["origem"], saida["destino"] = rt.Origem, rt.Destino
	saida["conf_origem"], saida["conf_destino"] = rt.ConfOrigem, rt.ConfDestino
	saida["opcao"], saida["pede_humano"], saida["irritacao"] = rt.Opcao, rt.PedeHumano, rt.Irritacao
	saida["detalhes_extras"] = rt.DetalhesExtras

	if rt.PedeHumano >= a.cfg.LimiarHumano {
		finalizar("transfere_humano")
		return resultadoRota{transf: &transf{motivo: "cliente pediu atendente (roteador)"}}
	}
	if rt.Irritacao >= a.cfg.LimiarIrritacao {
		finalizar("transfere_irritacao")
		return resultadoRota{transf: &transf{motivo: "cliente irritado (roteador)"}}
	}
	if rt.ConfIntencao >= a.cfg.LimiarRota {
		switch rt.Intencao {
		case IntencaoSaudacao:
			if !jaHouveResposta(hist) {
				finalizar("template_saudacao")
				return resultadoRota{resposta: TextoSaudacao}
			}
		case IntencaoCidadesAtendidas:
			if len(cidades) > 0 {
				finalizar("template_cidades")
				return resultadoRota{resposta: textoCidadesAtendidas(cidades)}
			}
		}
	}
	saida["pede_volta"] = rt.PedeVolta

	// Escolha clara de uma das opcoes mostradas ("a primeira", "dia 8"): o
	// codigo registra a escolha antes do LLM, que so confirma ao cliente.
	if n, ok := a.opcaoClara(rt, tc.estado); ok {
		if pre, ok := a.preExecutar(ctx, tc, "escolher_viagem", map[string]any{"opcao": n}); ok {
			finalizar("pre_escolha")
			return resultadoRota{pre: pre}
		}
		saida["pre_escolha"] = "falhou"
	}

	args, ok := a.argsPreBusca(rt, tc.estado)
	decisao := "pre_busca"
	if o, d := tc.estado.Origem, tc.estado.Destino; rt.PedeVolta >= limiarVolta && o != nil && d != nil {
		// Volta: busca o sentido oposto da rota ja buscada.
		args, ok, decisao = map[string]string{"origem": d.Nome, "destino": o.Nome}, true, "pre_busca_volta"
	}
	if !ok {
		finalizar("llm")
		return resultadoRota{}
	}
	argsAny := map[string]any{}
	for k, v := range args {
		argsAny[k] = v
	}
	// Data/periodo citado pelo cliente ("daqui 15 dias", "mes que vem") entra na
	// busca, interpretado em codigo.
	if p, ok := ferramentas.ResolverQuando(textoRecenteCliente(hist), a.d.Agora().In(a.loc)); ok {
		argsAny["quando"] = p.Expressao
		saida["quando"] = p.Descricao
	}
	pre, ok := a.preExecutar(ctx, tc, "buscar_viagens", argsAny)
	if !ok {
		finalizar("pre_busca_falhou")
		return resultadoRota{}
	}
	finalizar(decisao)
	return resultadoRota{pre: pre}
}

// limiarOpcao: confianca minima (intencao e opcao) para o codigo escolher a
// viagem sozinho; errar aqui custa uma troca, entao e mais alto que LimiarRota.
const limiarOpcao = 0.9

// opcaoClara devolve o numero da opcao quando o Roteador tem certeza de que o
// cliente escolheu UMA das opcoes atuais (e nao esta falando da volta).
func (a *Agente) opcaoClara(rt Rota, est conversa.Estado) (int, bool) {
	if rt.Intencao != IntencaoEscolherOpcao || rt.ConfIntencao < limiarOpcao || rt.ConfOpcao < limiarOpcao || rt.PedeVolta >= 0.5 {
		return 0, false
	}
	n, err := strconv.Atoi(rt.Opcao)
	if err != nil {
		return 0, false
	}
	for _, o := range est.Opcoes {
		if o.Numero == n {
			for _, t := range est.Trechos {
				if t.Viagem.TripID == o.TripID {
					return 0, false // ja escolhida: nada a fazer
				}
			}
			return n, true
		}
	}
	return 0, false
}

// preExecutar roda uma ferramenta antes do LLM (sobre um clone do estado, que
// so vale se der certo) e devolve a chamada + resultado para o historico.
func (a *Agente) preExecutar(ctx context.Context, tc *turno, nome string, args map[string]any) ([]llm.Mensagem, bool) {
	argsJSON, _ := json.Marshal(args)
	t1 := a.d.Agora()
	est := clonarEstado(tc.estado)
	s := a.d.Ferramentas.Executar(ctx, &ferramentas.Contexto{Conversa: tc.c, Estado: &est, Agora: a.d.Agora()}, nome, argsJSON)
	tc.passos = append(tc.passos, conversa.Passo{
		Tipo: "ferramenta", Nome: nome, Entrada: json.RawMessage(argsJSON), Saida: s,
		DuracaoMS: a.d.Agora().Sub(t1).Milliseconds(),
	})
	if !s.OK || s.Transferir {
		return nil, false
	}
	js, err := json.Marshal(s)
	if err != nil {
		return nil, false
	}
	tc.estado = est
	tc.resultados = append(tc.resultados, string(js))
	return []llm.Mensagem{
		{Papel: llm.PapelAssistente, Chamadas: []llm.ChamadaFerramenta{{ID: idPreBusca, Nome: nome, Argumentos: argsJSON}}},
		{Papel: llm.PapelFerramenta, ChamadaID: idPreBusca, Texto: string(js)},
	}, true
}

// textoRecenteCliente junta as mensagens do cliente desde a ultima resposta.
func textoRecenteCliente(hist []conversa.Mensagem) string {
	var partes []string
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Autor != conversa.AutorCliente {
			break
		}
		partes = append([]string{hist[i].Texto}, partes...)
	}
	return strings.Join(partes, " ")
}
