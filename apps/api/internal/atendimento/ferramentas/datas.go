package ferramentas

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Periodo e um intervalo de datas (inclusive) interpretado de uma expressao do
// cliente, ex.: "daqui 15 dias", "mes que vem", "quinta que vem".
type Periodo struct {
	De, Ate   time.Time
	Expressao string // trecho reconhecido, como o cliente escreveu (normalizado)
	Descricao string // ex.: "mes que vem (01/11 a 30/11)"
}

var mesesPT = map[string]time.Month{
	"janeiro": 1, "jan": 1, "fevereiro": 2, "fev": 2, "marco": 3, "mar": 3, "abril": 4, "abr": 4,
	"maio": 5, "mai": 5, "junho": 6, "jun": 6, "julho": 7, "jul": 7, "agosto": 8, "ago": 8,
	"setembro": 9, "set": 9, "outubro": 10, "out": 10, "novembro": 11, "nov": 11, "dezembro": 12, "dez": 12,
}

var nomesMes = [...]string{"", "janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}

var diasPT = map[string]time.Weekday{
	"domingo": time.Sunday, "segunda": time.Monday, "terca": time.Tuesday, "quarta": time.Wednesday,
	"quinta": time.Thursday, "sexta": time.Friday, "sabado": time.Saturday,
}

var numerosPT = map[string]int{
	"um": 1, "uma": 1, "dois": 2, "duas": 2, "tres": 3, "quatro": 4, "cinco": 5, "seis": 6, "sete": 7,
	"oito": 8, "nove": 9, "dez": 10, "onze": 11, "doze": 12, "treze": 13, "quatorze": 14, "catorze": 14,
	"quinze": 15, "vinte": 20, "trinta": 30, "quarenta": 40, "sessenta": 60,
}

const (
	reNum  = `(\d{1,3}|um|uma|dois|duas|tres|quatro|cinco|seis|sete|oito|nove|dez|onze|doze|treze|quatorze|catorze|quinze|vinte|trinta|quarenta|sessenta)`
	reMes  = `(janeiro|fevereiro|marco|abril|maio|junho|julho|agosto|setembro|outubro|novembro|dezembro|jan|fev|mar|abr|mai|jun|jul|ago|set|out|nov|dez)`
	reDia  = `(domingo|segunda|terca|quarta|quinta|sexta|sabado)`
	reFase = `(inicio|comeco|meados|meio|fim|final)`
)

// Regras em ordem de prioridade (as mais especificas primeiro).
var (
	rxDepoisAmanha = regexp.MustCompile(`\bdepois de amanha\b`)
	rxAmanha       = regexp.MustCompile(`\bamanha\b`)
	rxHoje         = regexp.MustCompile(`\bhoje\b`)
	rxDaqui        = regexp.MustCompile(`\b(?:daqui|dqui|daki|em|dentro de|apos|depois de)\s+(?:a\s+|uns\s+|umas\s+)?` + reNum + `\s+(dias?|semanas?|mes|meses)\b`)
	rxDataBarra    = regexp.MustCompile(`\b(\d{1,2})\s*/\s*(\d{1,2})(?:\s*/\s*(\d{2,4}))?\b`)
	rxDiaDeMes     = regexp.MustCompile(`\b(?:dia\s+)?(\d{1,2})\s+(?:de\s+)?` + reMes + `\b`)
	rxFaseMes      = regexp.MustCompile(`\b` + reFase + `\s+(?:de\s+|do\s+mes\s+de\s+)?` + reMes + `\b`)
	rxFaseMesQVem  = regexp.MustCompile(`\b` + reFase + `\s+do\s+(?:mes\s+que\s+vem|proximo\s+mes)\b`)
	rxFaseEsteMes  = regexp.MustCompile(`\b` + reFase + `\s+(?:do|deste|desse)\s+mes\b`)
	rxMesQueVem    = regexp.MustCompile(`\b(?:mes\s+que\s+vem|proximo\s+mes)\b`)
	rxEsteMes      = regexp.MustCompile(`\b(?:este|esse|neste|nesse)\s+mes\b`)
	rxSemanaQVem   = regexp.MustCompile(`\b(?:semana\s+que\s+vem|proxima\s+semana)\b`)
	rxEstaSemana   = regexp.MustCompile(`\b(?:esta|essa|nesta|nessa)\s+semana\b`)
	rxFimDeSemana  = regexp.MustCompile(`\bfi(?:m|nal)\s+de\s+semana\b`)
	rxDiaSemana    = regexp.MustCompile(`\b(?:(proxima|proximo|essa|esta|nessa|nesta|na|numa|toda)\s+)?` + reDia + `(\s*-?\s*feira)?(\s+(?:que\s+vem|da\s+semana\s+que\s+vem))?\b`)
	rxSoMes        = regexp.MustCompile(`\b(?:em|pra|para|no\s+mes\s+de|mes\s+de|de)\s+` + reMes + `\b`)
	rxSoDia        = regexp.MustCompile(`\bdia\s+(\d{1,2})\b`)
)

func numeroPT(s string) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	n, ok := numerosPT[s]
	return n, ok
}

func dia0(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func fimDoMes(ano int, m time.Month, loc *time.Location) time.Time {
	return time.Date(ano, m+1, 0, 0, 0, 0, 0, loc)
}

// mesFuturo devolve o ano em que o mes m acontece a partir de hoje.
func mesFuturo(hoje time.Time, m time.Month) int {
	if m < hoje.Month() {
		return hoje.Year() + 1
	}
	return hoje.Year()
}

func faseDoMes(fase string, ano int, m time.Month, loc *time.Location) (time.Time, time.Time) {
	fim := fimDoMes(ano, m, loc)
	switch fase {
	case "inicio", "comeco":
		return time.Date(ano, m, 1, 0, 0, 0, 0, loc), time.Date(ano, m, 10, 0, 0, 0, 0, loc)
	case "meados", "meio":
		return time.Date(ano, m, 11, 0, 0, 0, 0, loc), time.Date(ano, m, 20, 0, 0, 0, 0, loc)
	default: // fim, final
		return time.Date(ano, m, 21, 0, 0, 0, 0, loc), fim
	}
}

func nomeFase(f string) string {
	switch f {
	case "comeco":
		return "começo"
	case "inicio":
		return "início"
	}
	return f
}

// ResolverQuando procura no texto uma expressao de data/periodo e devolve o
// intervalo correspondente a partir de hoje. Datas no passado sao levadas
// para o futuro (proximo mes/ano) ou cortadas em hoje.
func ResolverQuando(texto string, hoje time.Time) (Periodo, bool) {
	t := " " + strings.Join(strings.Fields(substAcentos.Replace(strings.ToLower(texto))), " ") + " "
	h := dia0(hoje)
	loc := h.Location()
	mk := func(de, ate time.Time, expr, desc string) (Periodo, bool) {
		if de.Before(h) {
			de = h
		}
		if ate.Before(de) {
			return Periodo{}, false
		}
		if de.Equal(ate) {
			desc = fmt.Sprintf("%s (%s, %s)", desc, de.Format("02/01"), diaSemana(de.Format("2006-01-02")))
		} else {
			desc = fmt.Sprintf("%s (%s a %s)", desc, de.Format("02/01"), ate.Format("02/01"))
		}
		return Periodo{De: de, Ate: ate, Expressao: strings.TrimSpace(expr), Descricao: desc}, true
	}

	if m := rxDepoisAmanha.FindString(t); m != "" {
		d := h.AddDate(0, 0, 2)
		return mk(d, d, m, "depois de amanhã")
	}
	if m := rxAmanha.FindString(t); m != "" {
		d := h.AddDate(0, 0, 1)
		return mk(d, d, m, "amanhã")
	}
	if g := rxDaqui.FindStringSubmatch(t); g != nil {
		if n, ok := numeroPT(g[1]); ok && n > 0 && n <= 400 {
			var alvo time.Time
			var antes, depois int // janela em dias em volta do alvo
			switch {
			case strings.HasPrefix(g[2], "dia"):
				alvo, antes, depois = h.AddDate(0, 0, n), 3, 3
			case strings.HasPrefix(g[2], "semana"):
				alvo, antes, depois = h.AddDate(0, 0, 7*n), 3, 3
			default:
				alvo, antes, depois = h.AddDate(0, n, 0), 7, 7
			}
			return mk(alvo.AddDate(0, 0, -antes), alvo.AddDate(0, 0, depois), g[0], "por volta de "+strings.TrimSpace(g[0]))
		}
	}
	if g := rxDataBarra.FindStringSubmatch(t); g != nil {
		d, _ := strconv.Atoi(g[1])
		mo, _ := strconv.Atoi(g[2])
		if d >= 1 && d <= 31 && mo >= 1 && mo <= 12 {
			ano := h.Year()
			if g[3] != "" {
				ano, _ = strconv.Atoi(g[3])
				if ano < 100 {
					ano += 2000
				}
			}
			x := time.Date(ano, time.Month(mo), d, 0, 0, 0, 0, loc)
			if g[3] == "" && x.Before(h) {
				x = x.AddDate(1, 0, 0)
			}
			if x.Day() == d {
				return mk(x, x, g[0], "dia "+x.Format("02/01"))
			}
		}
	}
	if g := rxDiaDeMes.FindStringSubmatch(t); g != nil {
		d, _ := strconv.Atoi(g[1])
		m := mesesPT[g[2]]
		ano := mesFuturo(h, m)
		x := time.Date(ano, m, d, 0, 0, 0, 0, loc)
		if x.Before(h) {
			x = x.AddDate(1, 0, 0)
		}
		if d >= 1 && x.Day() == d {
			return mk(x, x, g[0], fmt.Sprintf("%d de %s", d, nomesMes[m]))
		}
	}
	if g := rxFaseMesQVem.FindStringSubmatch(t); g != nil {
		p := h.AddDate(0, 1, 1-h.Day())
		de, ate := faseDoMes(g[1], p.Year(), p.Month(), loc)
		return mk(de, ate, g[0], nomeFase(g[1])+" de "+nomesMes[p.Month()])
	}
	if g := rxFaseMes.FindStringSubmatch(t); g != nil {
		m := mesesPT[g[2]]
		de, ate := faseDoMes(g[1], mesFuturo(h, m), m, loc)
		return mk(de, ate, g[0], nomeFase(g[1])+" de "+nomesMes[m])
	}
	if g := rxFaseEsteMes.FindStringSubmatch(t); g != nil {
		de, ate := faseDoMes(g[1], h.Year(), h.Month(), loc)
		if ate.Sub(h) < 3*24*time.Hour { // ja passou ou quase: mes seguinte
			p := h.AddDate(0, 1, 1-h.Day())
			de, ate = faseDoMes(g[1], p.Year(), p.Month(), loc)
		}
		return mk(de, ate, g[0], nomeFase(g[1])+" do mês")
	}
	if m := rxMesQueVem.FindString(t); m != "" {
		p := h.AddDate(0, 1, 1-h.Day())
		return mk(p, fimDoMes(p.Year(), p.Month(), loc), m, "mês que vem, "+nomesMes[p.Month()])
	}
	if m := rxEsteMes.FindString(t); m != "" {
		return mk(h, fimDoMes(h.Year(), h.Month(), loc), m, "este mês")
	}
	for _, g := range rxDiaSemana.FindAllStringSubmatch(t, -1) {
		// "segunda" sozinha pode ser ordinal ("segunda opcao"): exige contexto.
		if g[2] == "segunda" && g[1] == "" && g[3] == "" && g[4] == "" {
			continue
		}
		wd := diasPT[g[2]]
		dif := (int(wd) - int(h.Weekday()) + 7) % 7
		if dif == 0 {
			dif = 7 // "quinta" dito numa quinta: a proxima
		}
		x := h.AddDate(0, 0, dif)
		// "da semana que vem": o dia na semana seguinte (segunda a domingo).
		if strings.Contains(g[4], "semana") {
			seg := h.AddDate(0, 0, (8-int(h.Weekday()))%7)
			if seg.Equal(h) {
				seg = seg.AddDate(0, 0, 7)
			}
			x = seg.AddDate(0, 0, (int(wd)+6)%7)
			return mk(x, x, g[0], g[2]+" da semana que vem")
		}
		// "quinta que vem" / "proxima quinta" e ambiguo quando a proxima quinta
		// cai nesta semana (pode ser a da semana seguinte): busca as duas.
		if (strings.Contains(g[4], "que vem") || strings.HasPrefix(g[1], "proxim")) && x.Before(proximaSegunda(h)) {
			return mk(x, x.AddDate(0, 0, 7), g[0], strings.TrimSpace(g[0])+", esta ou a da semana seguinte")
		}
		return mk(x, x, g[0], "próxima "+strings.TrimSpace(g[2]))
	}
	if m := rxSemanaQVem.FindString(t); m != "" {
		seg := h.AddDate(0, 0, (8-int(h.Weekday()))%7)
		if seg.Equal(h) {
			seg = seg.AddDate(0, 0, 7)
		}
		return mk(seg, seg.AddDate(0, 0, 6), m, "semana que vem")
	}
	if m := rxEstaSemana.FindString(t); m != "" {
		dom := h.AddDate(0, 0, (7-int(h.Weekday()))%7)
		return mk(h, dom, m, "esta semana")
	}
	if m := rxFimDeSemana.FindString(t); m != "" {
		sab := h.AddDate(0, 0, (int(time.Saturday)-int(h.Weekday())+7)%7)
		return mk(sab, sab.AddDate(0, 0, 1), m, "fim de semana")
	}
	if g := rxSoMes.FindStringSubmatch(t); g != nil {
		m := mesesPT[g[1]]
		ano := mesFuturo(h, m)
		return mk(time.Date(ano, m, 1, 0, 0, 0, 0, loc), fimDoMes(ano, m, loc), g[0], nomesMes[m])
	}
	if g := rxSoDia.FindStringSubmatch(t); g != nil {
		d, _ := strconv.Atoi(g[1])
		if d >= 1 && d <= 31 {
			x := time.Date(h.Year(), h.Month(), d, 0, 0, 0, 0, loc)
			if x.Before(h) || x.Day() != d {
				x = time.Date(h.Year(), h.Month()+1, d, 0, 0, 0, 0, loc)
			}
			if x.Day() == d {
				return mk(x, x, g[0], "dia "+x.Format("02/01"))
			}
		}
	}
	if m := rxHoje.FindString(t); m != "" {
		return mk(h, h, m, "hoje")
	}
	return Periodo{}, false
}

// proximaSegunda e a segunda-feira seguinte a hoje (inicio da semana que vem).
func proximaSegunda(h time.Time) time.Time {
	seg := h.AddDate(0, 0, (8-int(h.Weekday()))%7)
	if seg.Equal(h) {
		seg = seg.AddDate(0, 0, 7)
	}
	return seg
}

// rxMarcaVolta separa o pedido de volta ("e a volta dia 12", "retorno 15/10").
var rxMarcaVolta = regexp.MustCompile(`\b(volta|voltar|voltando|retorno|retornar|regresso)\b`)

// ResolverIdaVolta acha, numa mesma mensagem, o dia da ida e o dia da volta
// ("ida dia 8 e volta dia 12"). So reconhece dias exatos e volta depois da ida;
// "volta dia 3" depois de "ida dia 28" vira o dia 3 do mes seguinte.
func ResolverIdaVolta(texto string, hoje time.Time) (ida, volta Periodo, ok bool) {
	t := substAcentos.Replace(strings.ToLower(texto))
	loc := rxMarcaVolta.FindStringIndex(t)
	if loc == nil {
		return Periodo{}, Periodo{}, false
	}
	ida, ok1 := ResolverQuando(t[:loc[0]], hoje)
	volta, ok2 := ResolverQuando(t[loc[0]:], hoje)
	if !ok1 || !ok2 || !ida.De.Equal(ida.Ate) || !volta.De.Equal(volta.Ate) {
		return Periodo{}, Periodo{}, false
	}
	if volta.De.Before(ida.De) {
		if !strings.HasPrefix(volta.Expressao, "dia ") {
			return Periodo{}, Periodo{}, false
		}
		d := volta.De.AddDate(0, 1, 0)
		volta.De, volta.Ate = d, d
	}
	return ida, volta, true
}
