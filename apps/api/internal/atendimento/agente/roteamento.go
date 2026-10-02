package agente

import (
	"context"
	"encoding/json"
	"fmt"
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
// temQuando: o cliente citou data/periodo (a mesma rota vale buscar de novo).
func (a *Agente) argsPreBusca(rt Rota, est conversa.Estado, temQuando bool) (map[string]string, bool) {
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
		if !temQuando && est.Origem != nil && est.Destino != nil && len(est.Opcoes) > 0 &&
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
	saida["so_isso"] = rt.SoIsso
	tc.rota = &rt
	// direto: a mensagem so avanca o fluxo; passos obvios respondem por
	// template, sem LLM.
	direto := rt.SoIsso >= limiarSoIsso

	if res, dec, ok := a.posReserva(ctx, tc, rt); ok {
		saida["pos_reserva"], saida["conf_pos_reserva"] = rt.PosReserva, rt.ConfPosReserva
		finalizar(dec)
		return res
	}
	saida["nega"], saida["corrige_passageiro"] = rt.Nega, rt.CorrigePassageiro
	if est := tc.estado; len(est.Trechos) > 0 && !est.TodosReservados() {
		// Correcao de dado ja dado: nada de template nem fechamento; o LLM
		// corrige com registrar_passageiros vendo a lista atual.
		if rt.CorrigePassageiro >= 0.8 {
			finalizar("llm_correcao")
			return resultadoRota{}
		}
		// "Nao" ao pedido de fechamento: pergunta o que ajustar.
		if rt.Nega >= limiarFechar && rt.Confirma < 0.5 && ultimoBotPedeFechamento(hist) {
			finalizar("template_nao_fechar")
			return resultadoRota{resposta: TextoNaoFechar}
		}
	}

	// Escolha clara de uma das opcoes mostradas ("a primeira", "dia 8"): o
	// codigo registra a escolha antes do LLM, que so confirma ao cliente.
	if n, ok := a.opcaoClara(rt, tc.estado); ok {
		if pre, ok := a.preExecutar(ctx, tc, "escolher_viagem", map[string]any{"opcao": n}); ok {
			if direto {
				finalizar("template_escolha")
				return resultadoRota{resposta: textoEscolhido(tc.estado)}
			}
			finalizar("pre_escolha")
			return resultadoRota{pre: pre}
		}
		saida["pre_escolha"] = "falhou"
	}

	// "ida dia 8 e volta dia 12": o codigo escolhe os dois trechos pela rota e
	// pelas datas.
	if res, dec, ok := a.preIdaVolta(ctx, tc, rt, hist, direto); ok {
		finalizar(dec)
		return res
	}

	novaQuantidade := aplicarQuantidade(rt, &tc.estado)
	if novaQuantidade {
		saida["quantidade"] = map[string]int{"pessoas": tc.estado.PessoasInformadas, "criancas_ate_5": tc.estado.CriancasInformadas}
	}

	// Foto de documento: os dados ja vem estruturados da leitura de imagem; o
	// codigo registra os passageiros e o LLM so confirma com o cliente.
	novos, avisos := passageirosDeFotos([]string{textoRecenteCliente(hist)}, a.d.Agora().In(a.loc))
	if len(novos) > 0 {
		if pre, ok := a.registrarEmCodigo(ctx, tc, novos); ok {
			if len(avisos) > 0 {
				finalizar("template_registro")
				return resultadoRota{resposta: textoRegistrados(tc.estado) + "\n\n" + strings.Join(avisos, "\n")}
			}
			if direto {
				finalizar("template_registro")
				return resultadoRota{resposta: textoConfirmarRegistro(tc.estado)}
			}
			finalizar("pre_registro")
			return resultadoRota{pre: pre}
		}
	}
	if len(avisos) > 0 {
		// Certidao de crianca acima de 5 anos sem CPF/RG: pede o documento.
		finalizar("template_certidao")
		return resultadoRota{resposta: strings.Join(avisos, "\n")}
	}

	// Passageiros escritos ("ana souza cpf 529...", "bebê Sofia Reis"): o
	// codigo registra; se a mensagem so tem isso, responde por template.
	if !tc.estado.AlgumReservado() {
		if novosT, completo := extrairPassageirosTexto(textoRecenteCliente(hist), tc.estado.CriancasInformadas); len(novosT) > 0 {
			if pre, ok := a.registrarEmCodigo(ctx, tc, novosT); ok {
				if completo {
					finalizar("template_registro_texto")
					return resultadoRota{resposta: textoConfirmarRegistro(tc.estado)}
				}
				finalizar("pre_registro_texto")
				return resultadoRota{pre: pre}
			}
		}
	}

	// Pagamento claro (ou "sim" para fechar) com tudo pronto: o codigo cria a
	// reserva e o PIX e responde sem o LLM.
	if res, dec, ok := a.preFechar(ctx, tc, rt, hist); ok {
		saida["pagamento"], saida["conf_pagamento"], saida["confirma"] = rt.Pagamento, rt.ConfPagamento, rt.Confirma
		finalizar(dec)
		return res
	}

	quando, temQuando := ferramentas.ResolverQuando(textoRecenteCliente(hist), a.d.Agora().In(a.loc))
	args, ok := a.argsPreBusca(rt, tc.estado, temQuando)
	decisao := "pre_busca"
	if o, d := tc.estado.Origem, tc.estado.Destino; rt.PedeVolta >= limiarVolta && o != nil && d != nil {
		// Volta: busca o sentido oposto da rota ja buscada.
		args, ok, decisao = map[string]string{"origem": d.Nome, "destino": o.Nome}, true, "pre_busca_volta"
	}
	if !ok {
		// So a quantidade mudou ("somos 3, 1 criança") com viagem escolhida e
		// sem passageiros: pede os dados de cada um por template.
		if direto && novaQuantidade && len(tc.estado.Trechos) > 0 && len(tc.estado.Passageiros) == 0 {
			finalizar("template_quantidade")
			return resultadoRota{resposta: textoProximoPasso(tc.estado)}
		}
		finalizar("llm")
		return resultadoRota{}
	}
	argsAny := map[string]any{}
	for k, v := range args {
		argsAny[k] = v
	}
	// Data/periodo citado pelo cliente ("daqui 15 dias", "mes que vem") entra na
	// busca, interpretado em codigo.
	if temQuando {
		argsAny["quando"] = quando.Expressao
		saida["quando"] = quando.Descricao
	}
	pre, ok := a.preExecutar(ctx, tc, "buscar_viagens", argsAny)
	if !ok {
		finalizar("pre_busca_falhou")
		return resultadoRota{}
	}
	// Busca limpa (opcoes, sem aviso da ferramenta) e mensagem so com a rota:
	// a lista vai por template.
	if direto {
		if ops, limpa := opcoesSemAviso(tc.resultados); limpa {
			finalizar("template_opcoes")
			return resultadoRota{resposta: textoOpcoes(ops)}
		}
	}
	finalizar(decisao)
	return resultadoRota{pre: pre}
}

// pagamentoEscolhido diz se o cliente ja escolheu integral/sinal (neste turno,
// pelo Roteador, ou antes, no estado). Sem Roteador a escolha e liberada.
func pagamentoEscolhido(tc *turno) bool {
	if tc.rota == nil || tc.estado.Pagamento != "" {
		return true
	}
	rt := tc.rota
	return (rt.Pagamento == PagamentoIntegral || rt.Pagamento == PagamentoSinal) && rt.ConfPagamento >= 0.7
}

// rotaDoTurno: origem e destino ditos agora (Roteador com confianca) ou, na
// falta, os do estado.
func (a *Agente) rotaDoTurno(rt Rota, est conversa.Estado) (string, string) {
	o, d := "", ""
	if cidadeValida(rt.Origem) && rt.ConfOrigem >= a.cfg.LimiarRota {
		o = rt.Origem
	} else if est.Origem != nil {
		o = est.Origem.Nome
	}
	if cidadeValida(rt.Destino) && rt.ConfDestino >= a.cfg.LimiarRota {
		d = rt.Destino
	} else if est.Destino != nil {
		d = est.Destino.Nome
	}
	return o, d
}

// preIdaVolta escolhe ida e volta numa mensagem so ("ida dia 8 e volta dia
// 12"), quando a rota e conhecida e ainda nao ha trechos. Se a volta falhar,
// a ida escolhida segue para o LLM.
func (a *Agente) preIdaVolta(ctx context.Context, tc *turno, rt Rota, hist []conversa.Mensagem, direto bool) (resultadoRota, string, bool) {
	if len(tc.estado.Trechos) > 0 {
		return resultadoRota{}, "", false
	}
	ida, volta, ok := ferramentas.ResolverIdaVolta(textoRecenteCliente(hist), a.d.Agora().In(a.loc))
	if !ok {
		return resultadoRota{}, "", false
	}
	o, d := a.rotaDoTurno(rt, tc.estado)
	if o == "" || d == "" || chaveRota(o, d) == chaveRota(d, o) {
		return resultadoRota{}, "", false
	}
	pre, ok := a.preExecutar(ctx, tc, "escolher_viagem", map[string]any{"origem": o, "destino": d, "data": ida.De.Format("2006-01-02")})
	if !ok {
		return resultadoRota{}, "pre_ida_volta_falhou", true
	}
	pre2, ok := a.preExecutar(ctx, tc, "escolher_viagem", map[string]any{"origem": d, "destino": o, "data": volta.De.Format("2006-01-02")})
	if !ok {
		return resultadoRota{pre: pre}, "pre_ida", true
	}
	if direto {
		return resultadoRota{resposta: textoEscolhido(tc.estado)}, "template_ida_volta", true
	}
	return resultadoRota{pre: append(pre, pre2...)}, "pre_ida_volta", true
}

// escolhaPermitida diz se o cliente deu algum sinal de escolha neste turno
// (opcao, intencao de escolher, dia exato). Sem Roteador nao ha como saber e
// a escolha e liberada.
func escolhaPermitida(tc *turno) bool {
	rt := tc.rota
	if rt == nil || tc.diaEspecifico {
		return true
	}
	if rt.Intencao == IntencaoEscolherOpcao && rt.ConfIntencao >= 0.7 {
		return true
	}
	if rt.Opcao != "" && rt.Opcao != OpcaoNenhuma && rt.ConfOpcao >= 0.7 {
		return true
	}
	return rt.PedeVolta >= limiarVolta
}

// TextoNaoFechar responde a um "nao" ao pedido de fechar a compra.
const TextoNaoFechar = "Sem problema! 😊 O que você quer ajustar: a viagem, os passageiros ou a forma de pagamento?"

// limiarSoIsso: confianca minima de que a mensagem so avanca o fluxo para
// responder por template.
const limiarSoIsso = 0.85

// opcoesSemAviso devolve as opcoes da ultima busca deste turno quando ela veio
// sem "mensagem"/"sugestao" (nada que o LLM precise explicar).
func opcoesSemAviso(resultados []string) ([]conversa.Opcao, bool) {
	if len(resultados) == 0 {
		return nil, false
	}
	var s struct {
		OK    bool `json:"ok"`
		Dados struct {
			Opcoes   []conversa.Opcao `json:"opcoes"`
			Mensagem string           `json:"mensagem"`
			Sugestao string           `json:"sugestao"`
			SemVaga  int              `json:"sem_vaga_para_pessoas_count"`
		} `json:"dados"`
	}
	if json.Unmarshal([]byte(resultados[len(resultados)-1]), &s) != nil || !s.OK {
		return nil, false
	}
	d := s.Dados
	return d.Opcoes, len(d.Opcoes) > 0 && d.Mensagem == "" && d.Sugestao == "" && d.SemVaga == 0
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
	tc.nPre++
	id := idPreBusca
	if tc.nPre > 1 {
		id = fmt.Sprintf("pre_%d", tc.nPre)
	}
	return []llm.Mensagem{
		{Papel: llm.PapelAssistente, Chamadas: []llm.ChamadaFerramenta{{ID: id, Nome: nome, Argumentos: argsJSON}}},
		{Papel: llm.PapelFerramenta, ChamadaID: id, Texto: string(js)},
	}, true
}

// limiarQuantidade: confianca minima para gravar a quantidade de pessoas dita
// pelo cliente (so orienta as pendencias; os nomes continuam com o LLM).
const limiarQuantidade = 0.85

// aplicarQuantidade grava no estado quantas pessoas (e criancas ate 5 anos) o
// cliente disse que vao, quando o Roteador tem certeza dos dois numeros e ainda
// nao ha passageiros. Devolve se mudou o estado.
func aplicarQuantidade(rt Rota, est *conversa.Estado) bool {
	if len(est.Passageiros) > 0 || rt.ConfAdultos < limiarQuantidade || rt.ConfCriancas < limiarQuantidade {
		return false
	}
	ad, err1 := strconv.Atoi(rt.Adultos)
	cr, err2 := strconv.Atoi(rt.Criancas)
	if err1 != nil || err2 != nil || ad < 1 || cr < 0 {
		return false
	}
	if est.PessoasInformadas == ad+cr && est.CriancasInformadas == cr {
		return false
	}
	est.PessoasInformadas, est.CriancasInformadas = ad+cr, cr
	return true
}

// limiarFechar: confianca minima para o codigo criar reserva e PIX sozinho.
const limiarFechar = 0.9

// pagamentoDecidido devolve a forma de pagamento quando o cliente acabou de
// escolher (com certeza) ou disse "sim" a um pedido de fechamento com o
// pagamento ja escolhido antes.
func pagamentoDecidido(rt Rota, est conversa.Estado, hist []conversa.Mensagem) string {
	if (rt.Pagamento == PagamentoIntegral || rt.Pagamento == PagamentoSinal) && rt.ConfPagamento >= limiarFechar {
		return rt.Pagamento
	}
	if est.Pagamento != "" && rt.Confirma >= limiarFechar && ultimoBotPedeFechamento(hist) {
		return est.Pagamento
	}
	return ""
}

// ultimoBotPedeFechamento diz se a ultima mensagem do bot fala em reserva, PIX
// ou fechar/confirmar (um "sim" depois dela e para fechar a compra).
func ultimoBotPedeFechamento(hist []conversa.Mensagem) bool {
	for i := len(hist) - 1; i >= 0; i-- {
		m := hist[i]
		if m.Autor == conversa.AutorCliente || envioFalhou(m) {
			continue
		}
		t := semAcento(strings.ToLower(m.Texto))
		for _, k := range []string{"reserva", "pix", "fechar", "confirm"} {
			if strings.Contains(t, k) {
				return true
			}
		}
		return false
	}
	return false
}

// preFechar cria a reserva e o PIX antes do LLM quando a viagem e os
// passageiros estao completos e o pagamento ficou claro. ok=false: nada a fazer
// aqui. Com PIX gerado a resposta e montada em codigo; se so a reserva deu
// certo, o LLM recebe as chamadas e trata o PIX (ex.: pedir CPF do pagador).
func (a *Agente) preFechar(ctx context.Context, tc *turno, rt Rota, hist []conversa.Mensagem) (resultadoRota, string, bool) {
	est := tc.estado
	if len(est.Trechos) == 0 || len(ferramentas.FaltaParaReserva(est)) > 0 {
		return resultadoRota{}, "", false
	}
	semPix := false
	for _, t := range est.Trechos {
		if t.ReservaID != "" && t.PagamentoID == "" {
			semPix = true
		}
	}
	if est.TodosReservados() && !semPix {
		return resultadoRota{}, "", false
	}
	pg := pagamentoDecidido(rt, est, hist)
	if pg == "" {
		return resultadoRota{}, "", false
	}
	if est.TodosReservados() {
		// Reserva criada sem PIX (ex.: o modelo reservou e nao gerou): gera o
		// PIX na forma escolhida; se mudou, troca antes (ajusta o sinal da reserva).
		nome, args := "gerar_pix", map[string]any{}
		if pg != est.Pagamento {
			nome, args = "trocar_pagamento", map[string]any{"pagamento": pg}
		}
		pre, ok := a.preExecutar(ctx, tc, nome, args)
		if !ok {
			return resultadoRota{}, "pre_pix_falhou", true
		}
		if px := pixDoTurno(tc.resultados); len(px) > 0 {
			return resultadoRota{resposta: textoFechamento(px, tc.resultados, pg)}, "pre_pix", true
		}
		return resultadoRota{pre: pre}, "pre_pix", true
	}
	pre, ok := a.preExecutar(ctx, tc, "criar_reserva", map[string]any{"pagamento": pg})
	if !ok {
		return resultadoRota{}, "pre_fechamento_falhou", true
	}
	pix, ok := a.preExecutar(ctx, tc, "gerar_pix", map[string]any{})
	if !ok {
		return resultadoRota{pre: pre}, "pre_reserva", true
	}
	if px := pixDoTurno(tc.resultados); len(px) > 0 {
		return resultadoRota{resposta: textoFechamento(px, tc.resultados, pg)}, "pre_fechamento", true
	}
	return resultadoRota{pre: append(pre, pix...)}, "pre_reserva", true
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
