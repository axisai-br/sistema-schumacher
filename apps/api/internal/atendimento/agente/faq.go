package agente

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/politica"
)

// Perguntas soltas no motor por comandos: o assunto sai de palavras-chave
// (ou do extrator) e a resposta e o texto fixo de politica/faq.md. Assunto
// sem texto vira TextoNaoSei: nada e inventado.

// TextoNaoSei e a resposta para assunto que a politica nao cobre.
const TextoNaoSei = "Essa informação eu não tenho aqui. O suporte confirma pra você: +55 49 9886-2222."

// TextoDesistir encerra sem insistir quando o cliente desiste.
const TextoDesistir = "Tudo bem! Se mudar de ideia, é só me chamar por aqui. 😊"

// assuntosPorPalavra: ordem importa (o primeiro que casar vence). Texto sem
// acento e minusculo.
var assuntosPorPalavra = []struct {
	chave string
	re    *regexp.Regexp
}{
	{"precos", regexp.MustCompile(`mais barat|mais car[ao]|mesmo preco|mesmo valor|quanto custa|qual (o |e o )?preco|preco da passagem|valor da passagem|quanto (e|ta|esta|sai) a passagem|tabela de preco|precos`)},
	{"valores", regexp.MustCompile(`quanto (fica|e|da|sai|vai ficar|vou pagar)|qual (o |e o )?(valor|total)|valor (do sinal|total|da entrada)|quanto (e|fica) o sinal|restante|quanto falta|fica quanto`)},
	{"garantia_vaga", regexp.MustCompile(`garantid|garante (a|minha) vaga|vaga (fica|esta|ta) (garantida|reservada)|segura (a|minha) vaga`)},
	{"formas_pagamento", regexp.MustCompile(`cart[ao]o|credito|debito|parcel|boleto|\bmetade\b|pagar depois|pago depois|pagar no dia|dinheiro`)},
	{"bagagem", regexp.MustCompile(`bagagem|\bmala|malas\b|mochila|quantos? quilos|\bkg\b|volume`)},
	{"animais", regexp.MustCompile(`cachorr|gato|animal|animais|\bpet\b|bicho`)},
	{"duracao", regexp.MustCompile(`quanto tempo|quantas horas|duracao|demora (quanto|muito)|horas de viagem|chega (que|qual) (horas|dia)`)},
	{"comodidades", regexp.MustCompile(`banheiro|wi-?fi|internet|tomada|ar condicionado|leito|poltrona|lanche|refei`)},
	{"descontos", regexp.MustCompile(`desconto|idoso|estudante|meia passagem|promoc`)},
	{"comprovante", regexp.MustCompile(`comprovante|nota fiscal|\bnota\b|recibo`)},
	{"embarque", regexp.MustCompile(`onde (e o |fica o )?embarque|local (de|do) embarque|ponto de embarque|onde (eu )?embarc|onde (o onibus|ele) para|rodoviaria`)},
	{"documentos", regexp.MustCompile(`que documento|quais documentos|precisa de documento|serve (rg|cnh)|aceita (rg|cnh)`)},
	{"criancas", regexp.MustCompile(`crianca paga|crianca precisa|bebe paga|idade (da|pra) crianca|menor de (idade|5)`)},
}

var (
	rePersonaPergunta = regexp.MustCompile(`voce e (um |uma )?(robo|bot|ia|humano|pessoa|maquina|inteligencia)|qual (modelo|ia|inteligencia)|(chat)?gpt|claude|llama|gemini|quem (te )?(criou|fez|programou)`)
	reInjecao         = regexp.MustCompile(`ignore (suas|as|todas)|esqueca (suas|as)|(mostre|mostra|revele|me (de|da|passa)) (o |seu )?(prompt|instruc)|lista de ferramentas|system ?:|modo (desenvolvedor|admin)|de graca|gratis`)
	reDesistir        = regexp.MustCompile(`(deixa|deixe) (pra|para) la|nao quero mais|desisti|vou desistir|nao vou mais viajar`)
	reMidiaAudio      = regexp.MustCompile(`\[audio nao compreendido\]`)
	reMidiaImagem     = regexp.MustCompile(`\[(foto|imagem)(:| )`)
	// reMidiaGeral tira marcadores de midia do texto original (com acento).
	reMidiaGeral = regexp.MustCompile(`\[(?:[áa]udio n[ãa]o compreendido|foto[^\]]*|imagem[^\]]*)\]`)
)

// assuntoDoTexto devolve a chave do assunto perguntado ("" = nenhum).
func assuntoDoTexto(texto string) string {
	t := semAcento(strings.ToLower(texto))
	for _, a := range assuntosPorPalavra {
		if a.re.MatchString(t) {
			return a.chave
		}
	}
	return ""
}

// respostaFAQ responde o assunto: "valores" e calculado do estado; os outros
// vem de faq.md (vazio = TextoNaoSei).
func respostaFAQ(chave string, e conversa.Estado, sinal float64) string {
	if chave == "valores" {
		return textoValores(e, sinal)
	}
	if txt, ok := politica.FAQ()[chave]; ok && txt != "" {
		return txt
	}
	return TextoNaoSei
}

// textoValores: integral, sinal e restante com os passageiros/quantidade que
// ja existem; sem viagem escolhida, so a regra do sinal.
func textoValores(e conversa.Estado, sinal float64) string {
	pag := e.Pagantes()
	if pag == 0 {
		pag = max(e.PessoasInformadas-e.CriancasInformadas, 0)
	}
	if len(e.Trechos) == 0 || pag == 0 {
		return fmt.Sprintf("O sinal é %s por passageiro pagante (maior de 5 anos) em cada trecho; o restante é pago no embarque. Assim que você escolher a viagem e me disser quantas pessoas vão, eu te passo o total certinho.", reais(sinal))
	}
	var total, s float64
	for _, t := range e.Trechos {
		tt := t.Viagem.Preco * float64(pag)
		total += tt
		s += min(tt, sinal*float64(pag))
	}
	pessoas := "1 passageiro pagante"
	if pag > 1 {
		pessoas = fmt.Sprintf("%d passageiros pagantes", pag)
	}
	if e.AlgumReservado() && e.Pagamento == "sinal" {
		return fmt.Sprintf("Para %s: total %s. Com o sinal, você paga %s agora no PIX e %s no embarque.", pessoas, reais(total), reais(s), reais(total-s))
	}
	return fmt.Sprintf("Para %s: integral %s, ou sinal de %s agora (%s por pagante) e %s no embarque.", pessoas, reais(total), reais(s), reais(sinal), reais(total-s))
}

// respostaAssunto: respostaFAQ mais os assuntos calculados com o catalogo
// ("precos": tabela por cidade de SC, sem precisar do LLM). Com viagem ja
// escolhida, pergunta de preco vira os valores da compra.
func (a *Agente) respostaAssunto(ctx context.Context, chave string, e conversa.Estado) string {
	if chave == "precos" && len(e.Trechos) == 0 && a.d.Cidades != nil {
		if cidades, err := a.d.Cidades.Cidades(ctx); err == nil {
			if t := textoPrecos(cidades); t != "" {
				return t
			}
		}
	}
	if chave == "precos" {
		chave = "valores"
	}
	return respostaFAQ(chave, e, a.cfg.SinalPorPagante)
}

// textoPrecos agrupa as cidades de Santa Catarina por preco (o mesmo nos dois
// sentidos, de/para qualquer cidade do Maranhao).
func textoPrecos(cidades []ferramentas.Cidade) string {
	porPreco := map[float64][]string{}
	var precos []float64
	var ma []string
	for _, c := range cidades {
		if c.UF == "MA" {
			ma = append(ma, c.Nome)
			continue
		}
		if c.PrecoBase <= 0 {
			continue
		}
		if _, ok := porPreco[c.PrecoBase]; !ok {
			precos = append(precos, c.PrecoBase)
		}
		porPreco[c.PrecoBase] = append(porPreco[c.PrecoBase], c.Nome)
	}
	if len(precos) == 0 {
		return ""
	}
	sort.Float64s(precos)
	var partes []string
	for _, p := range precos {
		partes = append(partes, fmt.Sprintf("%s para %s", reais(p), juntarNomes(porPreco[p])))
	}
	t := "O preço por pessoa, entre o Maranhão e Santa Catarina (nos dois sentidos), é a partir de " + strings.Join(partes, "; ") + "."
	if len(precos) > 1 {
		t += fmt.Sprintf(" As mais baratas são %s.", juntarNomes(porPreco[precos[0]]))
	}
	if len(ma) > 0 {
		t += fmt.Sprintf(" Do lado do Maranhão (%s) o preço não muda.", juntarNomes(ma))
	}
	return t + " O valor exato aparece quando eu buscar as datas da sua rota."
}
