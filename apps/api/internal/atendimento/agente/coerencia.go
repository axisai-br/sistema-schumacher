package agente

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
)

// opcoesDoTurno devolve as opcoes que buscar_viagens devolveu NESTE turno
// (a ultima busca com resultado vem por ultimo).
func opcoesDoTurno(resultados []string) [][]conversa.Opcao {
	var out [][]conversa.Opcao
	for _, r := range resultados {
		var s struct {
			OK    bool `json:"ok"`
			Dados struct {
				Opcoes []conversa.Opcao `json:"opcoes"`
			} `json:"dados"`
		}
		if json.Unmarshal([]byte(r), &s) == nil && s.OK && len(s.Dados.Opcoes) > 0 {
			out = append(out, s.Dados.Opcoes)
		}
	}
	return out
}

func chaveRota(origem, destino string) string {
	return semAcento(strings.ToLower(origem)) + ">" + semAcento(strings.ToLower(destino))
}

// rotasConhecidas reune os pares origem>destino com dados vindos de ferramenta:
// opcoes deste turno, opcoes e trechos do estado (antes e depois).
func rotasConhecidas(tc *turno) map[string]bool {
	m := map[string]bool{}
	add := func(os []conversa.Opcao) {
		for _, o := range os {
			m[chaveRota(o.Origem, o.Destino)] = true
		}
	}
	for _, os := range opcoesDoTurno(tc.resultados) {
		add(os)
	}
	for _, e := range []conversa.Estado{tc.antes, tc.estado} {
		add(e.Opcoes)
		for _, t := range e.Trechos {
			add([]conversa.Opcao{t.Viagem})
		}
	}
	return m
}

// reSeparadorRota: o que pode haver entre duas cidades para formar uma rota.
var reSeparadorRota = regexp.MustCompile(`^\s*(→|->|=>|➡️?|para|pra|ate|x|-|–|—)\s*$`)

// rotasSemBusca acha rotas "X → Y" / "X para Y" citadas na resposta que nao
// vieram de nenhuma busca. So vale quando a resposta cita data ou horario: e o
// caso de apresentar horarios de uma rota que nao foi buscada (ex.: inventar a
// volta copiando as datas da ida).
func rotasSemBusca(texto string, cidades []string, conhecidas map[string]bool) []string {
	if len(cidades) == 0 {
		return nil
	}
	temHorario := false
	for _, it := range extrairItens(texto) {
		if it.Tipo == "data" || it.Tipo == "hora" {
			temHorario = true
			break
		}
	}
	if !temHorario {
		return nil
	}
	norm := make([]string, len(cidades))
	for i, c := range cidades {
		norm[i] = semAcento(strings.ToLower(c))
	}
	visto := map[string]bool{}
	var out []string
	for _, linha := range strings.Split(texto, "\n") {
		l := semAcento(strings.ToLower(linha))
		type ocorr struct {
			ini, fim int
			idx      int
		}
		var oc []ocorr
		for i, c := range norm {
			for p := 0; ; {
				k := strings.Index(l[p:], c)
				if k < 0 {
					break
				}
				oc = append(oc, ocorr{p + k, p + k + len(c), i})
				p += k + len(c)
			}
		}
		for i := range oc {
			for j := range oc {
				a, b := oc[i], oc[j]
				if b.ini < a.fim || a.idx == b.idx {
					continue
				}
				if !reSeparadorRota.MatchString(l[a.fim:b.ini]) {
					continue
				}
				k := norm[a.idx] + ">" + norm[b.idx]
				if conhecidas[k] || visto[k] {
					continue
				}
				visto[k] = true
				out = append(out, cidades[a.idx]+" → "+cidades[b.idx])
			}
		}
	}
	return out
}

var diasCurtos = [...]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

func formatarReais(v float64) string {
	if v == float64(int64(v)) {
		s := fmt.Sprintf("%d", int64(v))
		var b strings.Builder
		for i, r := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteByte('.')
			}
			b.WriteRune(r)
		}
		return "R$ " + b.String()
	}
	return "R$ " + strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1)
}

// textoOpcoes monta, em codigo, a lista de opcoes da ultima busca. E a resposta
// de seguranca quando o LLM insiste em citar dados sem origem.
func textoOpcoes(os []conversa.Opcao) string {
	var b strings.Builder
	rota := ""
	for _, o := range os {
		if r := o.Origem + " → " + o.Destino; r != rota {
			if rota != "" {
				b.WriteString("\n")
			}
			rota = r
			fmt.Fprintf(&b, "Opções de %s:\n", r)
		}
		data := o.Data
		if t, err := time.Parse("2006-01-02", o.Data); err == nil {
			data = diasCurtos[t.Weekday()] + " " + t.Format("02/01")
		}
		fmt.Fprintf(&b, "%d. %s às %s, %s\n", o.Numero, data, o.Horario, formatarReais(o.Preco))
	}
	b.WriteString("\nQual delas você prefere?")
	return b.String()
}

// pixDoTurno devolve os itens de PIX gerados NESTE turno (ultima chamada de
// gerar_pix com codigos).
func pixDoTurno(resultados []string) []map[string]any {
	var ult []map[string]any
	for _, r := range resultados {
		var s struct {
			Dados struct {
				Pix []map[string]any `json:"pix"`
			} `json:"dados"`
		}
		if json.Unmarshal([]byte(r), &s) != nil {
			continue
		}
		var com []map[string]any
		for _, p := range s.Dados.Pix {
			if c, _ := p["pix_copia_e_cola"].(string); c != "" {
				com = append(com, p)
			}
		}
		if len(com) > 0 {
			ult = com
		}
	}
	return ult
}

// textoPix monta, em codigo, a mensagem com os PIX gerados. E a resposta de
// seguranca quando o LLM insiste em citar dados sem origem depois de criar a
// reserva: o cliente recebe os codigos em vez de ser transferido.
func textoPix(itens []map[string]any) string {
	var b strings.Builder
	b.WriteString("Reserva feita! ✅ Seguem os PIX (copia e cola):\n")
	total := 0.0
	for _, p := range itens {
		rota, _ := p["rota"].(string)
		data, _ := p["data"].(string)
		if t, err := time.Parse("2006-01-02", data); err == nil {
			data = t.Format("02/01")
		}
		valor, _ := p["valor"].(float64)
		total += valor
		fmt.Fprintf(&b, "\n%s, %s: %s\n%s\n", rota, data, formatarReais(valor), p["pix_copia_e_cola"])
	}
	if len(itens) > 1 {
		fmt.Fprintf(&b, "\nTotal a pagar agora: %s.", formatarReais(total))
	}
	return strings.TrimRight(b.String(), "\n")
}
