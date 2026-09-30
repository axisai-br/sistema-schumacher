//go:build eval

package evals

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/atendimento/agente"
	"schumacher-tur/api/internal/atendimento/conversa"
)

// Rotulos das frases do juiz.
const (
	rotuloPedeHumano = "pede_humano"
	rotuloIrritado   = "irritado"
	rotuloNormal     = "normal"
)

type fraseJuiz struct {
	Rotulo   string
	Frase    string
	Contexto string // fala anterior do bot (opcional)
}

// 30 frases de WhatsApp em PT-BR: 10 pedem humano, 10 irritadas, 10 normais.
var frasesJuiz = []fraseJuiz{
	{rotuloPedeHumano, "quero falar com um atendente", ""},
	{rotuloPedeHumano, "me passa pra uma pessoa de verdade por favor", ""},
	{rotuloPedeHumano, "tem algum humano ai?", "Qual a cidade de saída?"},
	{rotuloPedeHumano, "Preciso de ajuda", "A passagem é para você ou vai mais alguém junto?"},
	{rotuloPedeHumano, "chama alguem pra me atender pfv", ""},
	{rotuloPedeHumano, "prefiro falar com a atendente, como faço?", "Me passa o nome completo e o CPF."},
	{rotuloPedeHumano, "qual o numero do suporte? quero falar com uma pessoa", ""},
	{rotuloPedeHumano, "atendente pfv", "Não entendi, pode repetir?"},
	{rotuloPedeHumano, "será que alguem da equipe pode me ligar? é sobre uma reserva que já fiz", ""},
	{rotuloPedeHumano, "nao quero falar com robo, cade o atendente", ""},

	{rotuloIrritado, "vcs demoram demais pra responder, to cansado de esperar", ""},
	{rotuloIrritado, "que absurdo, já faz 2 horas que to esperando resposta!!", ""},
	{rotuloIrritado, "isso é uma palhaçada, ninguém resolve nada", "Qual a cidade de saída?"},
	{rotuloIrritado, "já falei isso 3 vezes!!! vcs não entendem??", "A passagem é só para você ou vai mais alguém junto?"},
	{rotuloIrritado, "péssimo atendimento, vou reclamar no procon", ""},
	{rotuloIrritado, "to ficando P da vida com esse bot burro", "Não entendi, pode repetir?"},
	{rotuloIrritado, "ninguem responde nada, que descaso com o cliente", ""},
	{rotuloIrritado, "ja perdi meu tempo demais com isso, ridiculo", "Me passa o nome completo e o CPF."},
	{rotuloIrritado, "VOCES SAO UM LIXO, NUNCA MAIS COMPRO AI", ""},
	{rotuloIrritado, "de novo a mesma pergunta?? que saco", "A passagem é só para você ou vai mais alguém junto?"},

	{rotuloNormal, "oi, boa tarde", ""},
	{rotuloNormal, "tem viagem pra chapecó?", ""},
	{rotuloNormal, "somos em 3 pessoas", "A passagem é só para você ou vai mais alguém junto?"},
	{rotuloNormal, "quanto custa de videira pra moncao?", ""},
	{rotuloNormal, "pode ser dia 12 mesmo", "Tenho viagens em 12/10 e 19/10. Qual prefere?"},
	{rotuloNormal, "meu cpf é 123.456.789-09", "Me passa o nome completo e o CPF."},
	{rotuloNormal, "qual o horario de saida em santa ines?", ""},
	{rotuloNormal, "obrigado, vou ver com a familia e te aviso", "Fica R$ 950 por pessoa."},
	{rotuloNormal, "tenho duas malas, pode levar?", ""},
	{rotuloNormal, "a crianca tem 4 anos, ela paga?", ""},
}

// classifica converte a avaliacao do juiz em rotulo, com os mesmos limiares
// padrao do agente.
func classifica(av agente.Avaliacao) string {
	const limH, limI = 0.7, 0.75 // padroes de agente.Config
	switch {
	case av.PedeHumano >= limH:
		return rotuloPedeHumano
	case av.Irritacao >= limI:
		return rotuloIrritado
	}
	return rotuloNormal
}

func transferir(rotulo string) bool { return rotulo != rotuloNormal }

type resultadoJuiz struct {
	nome           string
	exato, binario int
	erros          int
	total          int
	latenciaTotal  time.Duration
	divergencias   []string
}

func avaliarJuiz(t *testing.T, nome string, j agente.Juiz) resultadoJuiz {
	r := resultadoJuiz{nome: nome, total: len(frasesJuiz)}
	for _, f := range frasesJuiz {
		var hist []conversa.Mensagem
		if f.Contexto != "" {
			hist = append(hist, conversa.Mensagem{Autor: conversa.AutorBot, Direcao: conversa.DirecaoSaida, Texto: f.Contexto})
		}
		hist = append(hist, conversa.Mensagem{Autor: conversa.AutorCliente, Direcao: conversa.DirecaoEntrada, Texto: f.Frase})
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t0 := time.Now()
		av, err := j.Avaliar(ctx, hist)
		r.latenciaTotal += time.Since(t0)
		cancel()
		if err != nil {
			r.erros++
			r.divergencias = append(r.divergencias, "ERRO em "+f.Frase+": "+err.Error())
			continue
		}
		got := classifica(av)
		if got == f.Rotulo {
			r.exato++
		}
		if transferir(got) == transferir(f.Rotulo) {
			r.binario++
		}
		if got != f.Rotulo {
			r.divergencias = append(r.divergencias,
				"esperado "+f.Rotulo+", veio "+got+" (pede="+fmtF(av.PedeHumano)+" irrit="+fmtF(av.Irritacao)+"): "+f.Frase)
		}
	}
	return r
}
func fmtF(v float64) string { return fmt.Sprintf("%.2f", v) }

func TestJuizLLMvsJev(t *testing.T) {
	real := novoOpenAI(t)
	modelo := envOu("EVAL_MODELO_JUIZ", envOu("EVAL_MODELO_AGENTE", "gpt-4.1-mini"))

	juizes := []struct {
		nome string
		j    agente.Juiz
	}{{"LLM (" + modelo + ")", agente.NovoJuizLLM(real, modelo)}}
	if k := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")); k != "" {
		juizes = append(juizes, struct {
			nome string
			j    agente.Juiz
		}{"Jev (TypeSafe)", agente.NovoJuizJev(k, nil)})
	} else {
		t.Log("TYPESAFE_API_KEY ausente: juiz Jev não será avaliado")
	}

	for _, it := range juizes {
		r := avaliarJuiz(t, it.nome, it.j)
		ok := max(r.total-r.erros, 1)
		t.Logf("%s: acurácia 3 classes = %d/%d (%.0f%%); acurácia 'transferir?' = %d/%d (%.0f%%); erros = %d; latência média = %s",
			r.nome, r.exato, r.total, 100*float64(r.exato)/float64(r.total),
			r.binario, r.total, 100*float64(r.binario)/float64(r.total), r.erros,
			(r.latenciaTotal / time.Duration(ok)).Round(time.Millisecond))
		for _, d := range r.divergencias {
			t.Logf("  - %s", d)
		}
		if r.erros == r.total {
			t.Errorf("%s: todas as chamadas falharam", r.nome)
		}
	}
}
