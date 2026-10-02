package agente

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
)

// Escolha de opcao lida em codigo, para quando o cliente nao diz o numero:
// ordinal ("a segunda opção"), numero solto ("2"), data ("dia 12", "19/10"),
// dia da semana ("a de quinta"), "a mais cedo", "a mais barata" e "essa"
// (so com uma opcao na lista). So devolve quando ha exatamente uma opcao que
// atende; ambiguo fica sem escolha (o bot pergunta o numero).

var ordinais = map[string]int{"primeira": 1, "primeiro": 1, "segunda": 2, "segundo": 2, "terceira": 3, "terceiro": 3,
	"quarta": 4, "quarto": 4, "quinta": 5, "quinto": 5, "sexta": 6, "sexto": 6}

var (
	// "a segunda opção", "terceira da lista", "opção 2", "número 3", "a 2".
	reOrdinalOpcao = regexp.MustCompile(`\b(primeir[ao]|segund[ao]|terceir[ao]|quart[ao]|quint[ao]|sext[ao])\s+(?:opcao|da lista|viagem|horario)\b`)
	reOpcaoN       = regexp.MustCompile(`\b(?:opcao|numero|n[oº°]|a|o)\s*(\d{1,2})\b`)
	// "a primeira", "o primeiro", "primeira" (primeira nao e dia da semana).
	rePrimeira = regexp.MustCompile(`\b(?:a |o )?primeir[ao]\b`)
	reUltima   = regexp.MustCompile(`\b(?:a |o )?ultim[ao]\b`)
	reSoNumero = regexp.MustCompile(`^\D{0,12}?(\d{1,2})\D{0,20}$`)
	reMaisCedo = regexp.MustCompile(`mais cedo|mais proxim[ao]|o quanto antes|mais rapido|primeira que tiver|primeiro horario`)
	reBarata   = regexp.MustCompile(`mais barat[ao]|mais em conta|menor preco|mais economic`)
	reEssa     = regexp.MustCompile(`\b(essa|esta|esse|este|essa ai|pode ser|fechado|beleza|isso)\b`)
	reAbrevQue = regexp.MustCompile(`(?i)\s+q\s+`)
	reDiaSem   = regexp.MustCompile(`\b(?:a|o|na|no)\s+(?:de\s+|da\s+|do\s+)?(segunda|terca|quarta|quinta|sexta|sabado|domingo)\b`)
)

var diaDaSemana = map[string]time.Weekday{"domingo": time.Sunday, "segunda": time.Monday, "terca": time.Tuesday, "quarta": time.Wednesday,
	"quinta": time.Thursday, "sexta": time.Friday, "sabado": time.Saturday}

// opcaoDoTexto devolve o numero da opcao escolhida e como foi lida.
func opcaoDoTexto(texto string, est conversa.Estado, hoje time.Time) (int, string) {
	ops := est.Opcoes
	if len(ops) == 0 {
		return 0, ""
	}
	t := semAcento(strings.ToLower(texto))
	if reCPF.MatchString(texto) {
		return 0, "" // mensagem com documento nao e escolha de opcao
	}
	existe := func(n int) bool {
		for _, o := range ops {
			if o.Numero == n {
				return true
			}
		}
		return false
	}
	unica := func(filtro func(o conversa.Opcao) bool) int {
		achou := 0
		for _, o := range ops {
			if filtro(o) {
				if achou != 0 {
					return 0
				}
				achou = o.Numero
			}
		}
		return achou
	}
	if m := reOrdinalOpcao.FindStringSubmatch(t); m != nil && existe(ordinais[m[1]]) {
		return ordinais[m[1]], "ordinal"
	}
	_, ehQuantidade := quantidadeDoTexto(texto)
	if m := reOpcaoN.FindStringSubmatch(t); m != nil && !ehQuantidade {
		if n, _ := strconv.Atoi(m[1]); existe(n) && !strings.Contains(t, "/") && !strings.Contains(t, "dia ") {
			return n, "numero"
		}
	}
	if m := reSoNumero.FindStringSubmatch(strings.TrimSpace(t)); m != nil && !ehQuantidade && !strings.Contains(t, "/") && !strings.Contains(t, "dia") && !strings.Contains(t, "pessoa") {
		if n, _ := strconv.Atoi(m[1]); existe(n) {
			return n, "numero"
		}
	}
	if rePrimeira.MatchString(t) && existe(1) {
		return 1, "primeira"
	}
	if reUltima.MatchString(t) {
		max := 0
		for _, o := range ops {
			if o.Numero > max {
				max = o.Numero
			}
		}
		return max, "ultima"
	}
	// Data especifica ("dia 12", "19/10", "segunda que vem").
	if p, ok := ferramentas.ResolverQuando(reAbrevQue.ReplaceAllString(texto, " que "), hoje); ok && p.De.Equal(p.Ate) {
		dia := p.De.Format("2006-01-02")
		if n := unica(func(o conversa.Opcao) bool { return o.Data == dia }); n > 0 {
			return n, "data"
		}
	}
	// "a de quinta": a unica opcao nesse dia da semana.
	if m := reDiaSem.FindStringSubmatch(t); m != nil {
		wd := diaDaSemana[m[1]]
		if n := unica(func(o conversa.Opcao) bool {
			d, err := time.Parse("2006-01-02", o.Data)
			return err == nil && d.Weekday() == wd
		}); n > 0 {
			return n, "dia_semana"
		}
		// Todas no mesmo dia da semana: a mais proxima.
		if mesmoDia(ops, wd) {
			return maisCedo(ops), "dia_semana_proxima"
		}
	}
	if reMaisCedo.MatchString(t) {
		return maisCedo(ops), "mais_cedo"
	}
	if reBarata.MatchString(t) {
		menor, n, empate := 0.0, 0, false
		for _, o := range ops {
			switch {
			case n == 0 || o.Preco < menor:
				menor, n, empate = o.Preco, o.Numero, false
			case o.Preco == menor:
				empate = true
			}
		}
		if !empate {
			return n, "mais_barata"
		}
		return maisCedo(filtrarPreco(ops, menor)), "mais_barata_cedo"
	}
	if len(ops) == 1 && reEssa.MatchString(t) {
		return ops[0].Numero, "essa"
	}
	return 0, ""
}

func mesmoDia(ops []conversa.Opcao, wd time.Weekday) bool {
	for _, o := range ops {
		d, err := time.Parse("2006-01-02", o.Data)
		if err != nil || d.Weekday() != wd {
			return false
		}
	}
	return true
}

// maisCedo: a opcao de data/horario mais cedo.
func maisCedo(ops []conversa.Opcao) int {
	n, chave := 0, ""
	for _, o := range ops {
		k := o.Data + " " + o.Horario
		if n == 0 || k < chave {
			n, chave = o.Numero, k
		}
	}
	return n
}

func filtrarPreco(ops []conversa.Opcao, preco float64) []conversa.Opcao {
	var out []conversa.Opcao
	for _, o := range ops {
		if o.Preco == preco {
			out = append(out, o)
		}
	}
	return out
}
