package agente

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
)

var substAcentos = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// semAcento devolve o texto em minusculas e sem acentos.
func semAcento(s string) string {
	return substAcentos.Replace(strings.ToLower(s))
}

// palavras normaliza (sem acento, sem pontuacao) e separa em palavras.
func palavras(s string) []string {
	s = semAcento(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// similaridade devolve o indice de Jaccard entre os conjuntos de palavras.
func similaridade(a, b string) float64 {
	pa, pb := palavras(a), palavras(b)
	if len(pa) == 0 && len(pb) == 0 {
		return 1
	}
	ca := map[string]bool{}
	for _, p := range pa {
		ca[p] = true
	}
	cb := map[string]bool{}
	for _, p := range pb {
		cb[p] = true
	}
	inter := 0
	for p := range ca {
		if cb[p] {
			inter++
		}
	}
	uniao := len(ca) + len(cb) - inter
	if uniao == 0 {
		return 1
	}
	return float64(inter) / float64(uniao)
}

var (
	reValorRS    = regexp.MustCompile(`R\$\s*(\d+(?:[.,]\d+)*)`)
	reValorReais = regexp.MustCompile(`(\d+(?:[.,]\d+)*)\s*reais`)
	reNumero     = regexp.MustCompile(`\d+(?:[.,]\d+)*`)
	reMilhar     = regexp.MustCompile(`^\d{1,3}([.,]\d{3})+$`)
	reDataBR     = regexp.MustCompile(`(\d{1,2})/(\d{1,2})(?:/(\d{2,4}))?`)
	reDataISO    = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	reHoraDP     = regexp.MustCompile(`(\d{1,2}):(\d{2})`)
	reHoraH      = regexp.MustCompile(`(?i)(\d{1,2})h(\d{2})?`)
)

func ehDigito(b byte) bool { return b >= '0' && b <= '9' }
func ehLetra(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b >= 0x80
}

// achar devolve os submatches de re em s, descartando os que encostam em digito
// (ou, se semLetra, em letra) antes ou depois do match.
func achar(re *regexp.Regexp, s string, semLetra bool) [][]string {
	var out [][]string
	for _, idx := range re.FindAllStringSubmatchIndex(s, -1) {
		ini, fim := idx[0], idx[1]
		if ini > 0 && (ehDigito(s[ini-1]) || (semLetra && ehLetra(s[ini-1]))) {
			continue
		}
		if fim < len(s) && (ehDigito(s[fim]) || (semLetra && ehLetra(s[fim]))) {
			continue
		}
		grupos := make([]string, 0, len(idx)/2)
		for i := 0; i < len(idx); i += 2 {
			if idx[i] < 0 {
				grupos = append(grupos, "")
			} else {
				grupos = append(grupos, s[idx[i]:idx[i+1]])
			}
		}
		out = append(out, grupos)
	}
	return out
}

// centavos devolve os valores (em centavos) que um token numerico pode
// representar: "1.100" e 1100 (milhar BR) ou 1,1 (decimal); "950.00" e 950.
func centavos(tok string) []int64 {
	var cands []string
	temPonto, temVirg := strings.Contains(tok, "."), strings.Contains(tok, ",")
	switch {
	case temPonto && temVirg:
		if strings.LastIndex(tok, ",") > strings.LastIndex(tok, ".") {
			cands = append(cands, strings.ReplaceAll(strings.ReplaceAll(tok, ".", ""), ",", "."))
		} else {
			cands = append(cands, strings.ReplaceAll(tok, ",", ""))
		}
	case temPonto || temVirg:
		if strings.Count(tok, ".")+strings.Count(tok, ",") == 1 {
			cands = append(cands, strings.ReplaceAll(tok, ",", "."))
		}
		if reMilhar.MatchString(tok) {
			cands = append(cands, strings.NewReplacer(".", "", ",", "").Replace(tok))
		}
	default:
		cands = append(cands, tok)
	}
	var out []int64
	for _, c := range cands {
		v, err := strconv.ParseFloat(c, 64)
		if err != nil {
			continue
		}
		out = append(out, int64(math.Round(v*100)))
	}
	return out
}

func chaveData(d, m string) (string, bool) {
	di, e1 := strconv.Atoi(d)
	mi, e2 := strconv.Atoi(m)
	if e1 != nil || e2 != nil || di < 1 || di > 31 || mi < 1 || mi > 12 {
		return "", false
	}
	return fmt.Sprintf("d:%02d/%02d", di, mi), true
}

func chaveHora(h, m string) (string, bool) {
	hi, e1 := strconv.Atoi(h)
	mi := 0
	var e2 error
	if m != "" {
		mi, e2 = strconv.Atoi(m)
	}
	if e1 != nil || e2 != nil || hi > 23 || mi > 59 {
		return "", false
	}
	return fmt.Sprintf("h:%02d:%02d", hi, mi), true
}

// itemCitado e um valor, data ou horario extraido da resposta do modelo.
type itemCitado struct {
	Tipo   string   // "valor" | "data" | "hora"
	Texto  string   // como apareceu
	Chaves []string // chaves canonicas alternativas (basta uma estar no corpus)
}

// extrairItens pega valores monetarios, datas e horarios citados no texto.
func extrairItens(texto string) []itemCitado {
	var out []itemCitado
	addValor := func(tok string) {
		var ch []string
		for _, c := range centavos(tok) {
			ch = append(ch, "v:"+strconv.FormatInt(c, 10))
		}
		if len(ch) > 0 {
			out = append(out, itemCitado{Tipo: "valor", Texto: tok, Chaves: ch})
		}
	}
	for _, g := range achar(reValorRS, texto, false) {
		addValor(g[1])
	}
	for _, g := range reValorReais.FindAllStringSubmatch(texto, -1) {
		addValor(g[1])
	}
	for _, g := range achar(reDataBR, texto, false) {
		if k, ok := chaveData(g[1], g[2]); ok {
			out = append(out, itemCitado{Tipo: "data", Texto: g[0], Chaves: []string{k}})
		}
	}
	for _, g := range achar(reDataISO, texto, false) {
		if k, ok := chaveData(g[3], g[2]); ok {
			out = append(out, itemCitado{Tipo: "data", Texto: g[0], Chaves: []string{k}})
		}
	}
	for _, g := range achar(reHoraDP, texto, false) {
		if k, ok := chaveHora(g[1], g[2]); ok {
			out = append(out, itemCitado{Tipo: "hora", Texto: g[0], Chaves: []string{k}})
		}
	}
	for _, g := range achar(reHoraH, texto, true) {
		if k, ok := chaveHora(g[1], g[2]); ok {
			out = append(out, itemCitado{Tipo: "hora", Texto: g[0], Chaves: []string{k}})
		}
	}
	return out
}

// chavesCorpus extrai, de um texto de referencia (resultados de ferramentas,
// estado, catalogo), todas as chaves canonicas possiveis. Para valores, todo
// numero do corpus conta.
func chavesCorpus(corpus string, set map[string]bool, comValores bool) {
	if comValores {
		for _, tok := range reNumero.FindAllString(corpus, -1) {
			for _, c := range centavos(tok) {
				set["v:"+strconv.FormatInt(c, 10)] = true
			}
		}
	}
	for _, g := range reDataBR.FindAllStringSubmatch(corpus, -1) {
		if k, ok := chaveData(g[1], g[2]); ok {
			set[k] = true
		}
	}
	for _, g := range reDataISO.FindAllStringSubmatch(corpus, -1) {
		if k, ok := chaveData(g[3], g[2]); ok {
			set[k] = true
		}
	}
	for _, g := range reHoraDP.FindAllStringSubmatch(corpus, -1) {
		if k, ok := chaveHora(g[1], g[2]); ok {
			set[k] = true
		}
	}
	for _, g := range reHoraH.FindAllStringSubmatch(corpus, -1) {
		if k, ok := chaveHora(g[1], g[2]); ok {
			set[k] = true
		}
	}
}

// itensSemOrigem devolve os itens citados que nao aparecem nas fontes
// confiaveis (ferramentas, estado, catalogo). Datas e horarios tambem podem
// vir do que o proprio cliente escreveu neste turno (eco de confirmacao).
func itensSemOrigem(texto string, fontes []string, cliente []string) []itemCitado {
	itens := extrairItens(texto)
	if len(itens) == 0 {
		return nil
	}
	confiavel := map[string]bool{}
	for _, f := range fontes {
		chavesCorpus(f, confiavel, true)
	}
	doCliente := map[string]bool{}
	for _, f := range cliente {
		chavesCorpus(f, doCliente, false)
	}
	var faltam []itemCitado
	for _, it := range itens {
		achou := false
		for _, k := range it.Chaves {
			if confiavel[k] || (it.Tipo != "valor" && doCliente[k]) {
				achou = true
				break
			}
		}
		if !achou {
			faltam = append(faltam, it)
		}
	}
	return faltam
}

func listarItens(its []itemCitado) string {
	seen := map[string]bool{}
	var out []string
	for _, it := range its {
		t := strings.TrimSpace(it.Texto)
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// estadoAvancou compara dois estados ignorando Falhas e MotivoHumano.
func estadoAvancou(antes, depois conversa.Estado) bool {
	antes.Falhas, depois.Falhas = 0, 0
	antes.MotivoHumano, depois.MotivoHumano = "", ""
	a, _ := json.Marshal(antes)
	b, _ := json.Marshal(depois)
	return string(a) != string(b)
}

var gatilhosHumano = []string{
	"atendente", "falar com alguem", "falar com uma pessoa", "humano",
	"preciso de ajuda", "quero ajuda", "ajuda por favor", "suporte",
}

// pedeHumano e o atalho deterministico: pedido explicito de atendente.
func pedeHumano(texto string) bool {
	t := semAcento(texto)
	for _, g := range gatilhosHumano {
		if strings.Contains(t, g) {
			return true
		}
	}
	return false
}
