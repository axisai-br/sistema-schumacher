package agente

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
)

// TimeoutRoteadorPadrao limita a chamada ao Jev: acima disso o turno segue no LLM.
const TimeoutRoteadorPadrao = 3 * time.Second

type roteadorJev struct {
	c       *clienteJev
	timeout time.Duration
}

// NovoRoteadorJev cria um Roteador que faz UMA requisicao ao Jev por turno, com
// todas as perguntas independentes em paralelo (speculative fan-out). As
// instrucoes ficam em ingles (idioma principal do Jev); o estado (mensagens,
// cidades) segue em portugues. http nil usa um cliente sem timeout proprio (o
// limite e o timeout do roteador, 3s).
func NovoRoteadorJev(apiKey string, hc *http.Client) Roteador {
	if hc == nil {
		hc = &http.Client{}
	}
	return &roteadorJev{c: novoClienteJev(apiKey, hc), timeout: TimeoutRoteadorPadrao}
}

// autorJev traduz o autor da mensagem para o rotulo (em ingles) do estado do Jev.
func autorJev(a conversa.Autor) string {
	switch a {
	case conversa.AutorCliente:
		return "customer"
	case conversa.AutorHumano:
		return "human_agent"
	default:
		return "assistant"
	}
}

// cidadeJev e uma opcao das perguntas origem/destino.
type cidadeJev struct {
	chave string // chave da opcao (nome da cidade; "Nome/UF" se houver homonimas)
	desc  string
}

func opcoesCidades(cidades []ferramentas.Cidade) []cidadeJev {
	cont := map[string]int{}
	for _, c := range cidades {
		cont[c.Nome]++
	}
	var out []cidadeJev
	vistos := map[string]bool{}
	for _, c := range cidades {
		chave := c.Nome
		if cont[c.Nome] > 1 {
			chave = c.Nome + "/" + c.UF
		}
		if strings.TrimSpace(chave) == "" || vistos[chave] {
			continue
		}
		vistos[chave] = true
		desc := c.Nome
		if c.UF != "" {
			desc += " (" + c.UF + ")"
		}
		out = append(out, cidadeJev{chave: chave, desc: desc})
	}
	return out
}

// estadoJev monta o estado enviado ao Jev.
func estadoJev(e EntradaRota) map[string]any {
	msgs := e.Mensagens
	if len(msgs) > 6 {
		msgs = msgs[len(msgs)-6:]
	}
	conv := make([]map[string]string, 0, len(msgs))
	var ultimas []string // mensagens do cliente no fim da conversa
	for _, m := range msgs {
		t := strings.TrimSpace(m.Texto)
		if t == "" {
			continue
		}
		conv = append(conv, map[string]string{"author": autorJev(m.Autor), "text": t})
		if m.Autor == conversa.AutorCliente {
			ultimas = append(ultimas, t)
		} else {
			ultimas = nil
		}
	}
	st := map[string]any{
		"conversation":            conv,
		"latest_customer_message": strings.Join(ultimas, "\n"),
		"booking_summary":         e.Estado.Resumo(),
	}
	if len(e.Estado.Opcoes) > 0 {
		ops := make([]map[string]any, 0, len(e.Estado.Opcoes))
		for _, o := range e.Estado.Opcoes {
			ops = append(ops, map[string]any{
				"number": o.Numero, "date": o.Data, "time": o.Horario,
				"origin": o.Origem, "destination": o.Destino,
			})
		}
		st["current_options"] = ops
	}
	if len(e.Cidades) > 0 {
		nomes := make([]string, 0, len(e.Cidades))
		for _, c := range e.Cidades {
			nomes = append(nomes, c.Nome)
		}
		st["served_cities"] = nomes
	}
	return st
}

// perguntasJev monta as perguntas (instructions/criteria em ingles). Origem e
// destino so existem com cidades; opcao_escolhida so com opcoes no estado.
func perguntasJev(e EntradaRota) map[string]any {
	q := map[string]any{
		"pede_humano": map[string]any{
			"type":         "noul",
			"instructions": "Is the customer, in `latest_customer_message`, asking to talk to a human agent, a real person or the support staff, or asking for a person to help them?",
			"criteria": map[string]any{
				"true":  "Explicitly asks for a human attendant, a real person or human support (e.g. 'quero falar com um atendente', 'tem alguém aí?').",
				"false": "Does not ask for a human. Asking the assistant for information, prices or trips is not a request for a human.",
			},
		},
		"irritacao": map[string]any{
			"type":         "score",
			"instructions": "How irritated or angry is the customer in `latest_customer_message`, considering the tone of the `conversation`?",
			"criteria": []string{
				"Calm or neutral: a polite or plain message, no complaint.",
				"Annoyed or impatient: mild frustration, a complaint about waiting, or repeating something already said.",
				"Angry: insults, shouting, strong complaints or threats to complain or never buy again.",
			},
		},
		"intencao": map[string]any{
			"type":         "choice",
			"instructions": "What is the customer trying to do in `latest_customer_message`? Use the earlier `conversation`, `booking_summary` and `current_options` only to interpret it. Choose the single best option.",
			"criteria": map[string]any{
				IntencaoSaudacao:           "The customer only greets (e.g. 'oi', 'bom dia', 'boa tarde') and says nothing else: no city, no question, no request.",
				IntencaoCidadesAtendidas:   "The customer asks which cities, routes or destinations the company serves or has trips to, without naming a specific city to travel from or to.",
				IntencaoBuscarViagens:      "The customer wants to travel, or asks about trips, schedules, availability or prices, and names an origin or destination city (in this message or earlier in the conversation), and is not picking one of the `current_options`.",
				IntencaoEscolherOpcao:      "The customer picks one of the `current_options` (by number, date or time) or confirms the one just offered.",
				IntencaoInformarPassageiro: "The customer gives the number of passengers or passenger data: names, ID documents (CPF, RG, CNH) or ages.",
				IntencaoFormaPagamento:     "The customer talks about how to pay: paying in full or a deposit ('sinal'), PIX, or chooses a payment option.",
				IntencaoConsultarReserva:   "The customer asks about a booking or ticket they already have, or about a payment they already made.",
				IntencaoDuvidaInformativa:  "The customer asks a general question about luggage, boarding, children, pets, payment rules or other company policies.",
				IntencaoForaDoAssunto:      "The message has no relation to bus travel, tickets or bookings.",
				IntencaoOutro:              "None of the above, or unclear: thanks, acknowledgements, corrections, or several requests mixed together.",
			},
		},
		"menciona_data_ou_pessoas": map[string]any{
			"type":         "noul",
			"instructions": "Does the customer, in `latest_customer_message` or in earlier customer messages of the `conversation`, mention a specific travel date, a day of the week, a time of day, a date range, or how many people are traveling?",
			"criteria": map[string]any{
				"true":  "A date, weekday, period, time of day or number of travelers was mentioned.",
				"false": "None of these was mentioned.",
			},
		},
	}
	if cs := opcoesCidades(e.Cidades); len(cs) > 0 {
		lado := func(pergunta, nomeLado string) map[string]any {
			crit := map[string]any{}
			for _, c := range cs {
				crit[c.chave] = nomeLado + ": " + c.desc
			}
			crit[CidadeNaoInformada] = "The customer has not said the " + nomeLado + " city, and it cannot be inferred from the conversation."
			crit[CidadeNaoAtendida] = "The customer named a " + nomeLado + " city that is not in `served_cities`."
			return map[string]any{"type": "choice", "instructions": pergunta, "criteria": crit}
		}
		q["origem"] = lado("From which city does the customer want to depart? Consider `latest_customer_message` together with the earlier `conversation`: the latest message may give only one side of the route or change one side, and the other side then comes from the context. The answer must be one of `served_cities`.", "departure")
		q["destino"] = lado("To which city does the customer want to travel? Consider `latest_customer_message` together with the earlier `conversation`: the latest message may give only one side of the route or change one side, and the other side then comes from the context. The answer must be one of `served_cities`.", "destination")
	}
	if len(e.Estado.Opcoes) > 0 {
		crit := map[string]any{OpcaoNenhuma: "The customer is not clearly picking one of the `current_options` in `latest_customer_message`."}
		for _, o := range e.Estado.Opcoes {
			crit[strconv.Itoa(o.Numero)] = fmt.Sprintf("Option %d: %s at %s, %s to %s.", o.Numero, o.Data, o.Horario, o.Origem, o.Destino)
		}
		q["opcao_escolhida"] = map[string]any{
			"type":         "choice",
			"instructions": "Which of the `current_options` is the customer choosing in `latest_customer_message`: by option number, date, time or description? Choose 'nenhuma' unless the choice is clear.",
			"criteria":     crit,
		}
	}
	return q
}

func (r *roteadorJev) Rotear(ctx context.Context, e EntradaRota) (Rota, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	resp, err := r.c.avaliar(ctx, estadoJev(e), perguntasJev(e))
	if err != nil {
		return Rota{}, err
	}
	return rotaDeRespostas(resp, e), nil
}

// rotaDeRespostas converte as respostas do Jev em Rota. Respostas de cidade
// fora das opcoes enviadas viram "nao informado" com confianca 0.
func rotaDeRespostas(resp map[string]respostaJev, e EntradaRota) Rota {
	var rt Rota
	conf := func(a respostaJev) float64 {
		if a.Confidence == nil {
			return 0
		}
		return limitar01(*a.Confidence)
	}
	if a, ok := resp["pede_humano"]; ok && a.Noul != nil {
		rt.PedeHumano = limitar01(*a.Noul)
	}
	if a, ok := resp["irritacao"]; ok && a.Score != nil {
		rt.Irritacao = limitar01(*a.Score / 2)
	}
	if a, ok := resp["menciona_data_ou_pessoas"]; ok && a.Noul != nil {
		rt.DetalhesExtras = limitar01(*a.Noul)
	}
	if a, ok := resp["intencao"]; ok {
		rt.Intencao, rt.ConfIntencao = a.Choice, conf(a)
	}
	cidade := func(a respostaJev) string {
		if a.Choice == CidadeNaoInformada || a.Choice == CidadeNaoAtendida {
			return a.Choice
		}
		for _, c := range opcoesCidades(e.Cidades) {
			if c.chave == a.Choice {
				return a.Choice
			}
		}
		return ""
	}
	if a, ok := resp["origem"]; ok {
		if rt.Origem = cidade(a); rt.Origem != "" {
			rt.ConfOrigem = conf(a)
		}
	}
	if a, ok := resp["destino"]; ok {
		if rt.Destino = cidade(a); rt.Destino != "" {
			rt.ConfDestino = conf(a)
		}
	}
	if a, ok := resp["opcao_escolhida"]; ok {
		rt.Opcao, rt.ConfOpcao = a.Choice, conf(a)
	}
	return rt
}
