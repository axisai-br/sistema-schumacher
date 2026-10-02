package agente

import (
	"context"
	"errors"

	"encoding/json"
	"fmt"
	"regexp"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"strings"
	"time"
	"unicode"

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
	for _, os := range opcoesDoTurno(append(append([]string{}, tc.anteriores...), tc.resultados...)) {
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
			if o.Origem != "" && o.Destino != "" {
				fmt.Fprintf(&b, "Opções de %s:\n", r)
			} else {
				b.WriteString("Opções:\n")
			}
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

// textoFechamento monta a resposta quando o codigo fechou a compra (reserva +
// PIX) sem o LLM: os PIX, o codigo da reserva e, no sinal, o restante.
func textoFechamento(px []map[string]any, resultados []string, pagamento string) string {
	var b strings.Builder
	b.WriteString(textoPix(px))
	var codigos []string
	restante := 0.0
	for _, r := range resultados {
		var s struct {
			OK    bool `json:"ok"`
			Dados struct {
				Restante *float64 `json:"restante_no_embarque_total"`
				Trechos  []struct {
					Codigo string `json:"codigo_reserva"`
				} `json:"trechos"`
			} `json:"dados"`
		}
		if json.Unmarshal([]byte(r), &s) != nil || !s.OK || s.Dados.Trechos == nil {
			continue
		}
		codigos = codigos[:0]
		for _, t := range s.Dados.Trechos {
			if t.Codigo != "" {
				codigos = append(codigos, t.Codigo)
			}
		}
		if s.Dados.Restante != nil {
			restante = *s.Dados.Restante
		}
	}
	if len(codigos) == 1 {
		fmt.Fprintf(&b, "\n\nCódigo da reserva: %s", codigos[0])
	} else if len(codigos) > 1 {
		fmt.Fprintf(&b, "\n\nCódigos das reservas: %s", strings.Join(codigos, ", "))
	}
	if pagamento == PagamentoSinal && restante > 0 {
		fmt.Fprintf(&b, "\n\nO restante (%s) você paga no embarque.", formatarReais(restante))
	}
	b.WriteString("\n\nO PIX expira em alguns minutos. Depois de pagar, me avisa que eu confirmo pra você. 😊")
	return b.String()
}

// textoProximoPasso e a resposta montada em codigo a partir da primeira
// pendencia do estado: ultimo recurso quando o LLM insiste em uma resposta com
// problema (texto quebrado ou acao afirmada sem ferramenta).
func textoProximoPasso(e conversa.Estado) string {
	pend := ferramentas.FaltaParaReserva(e)
	if len(pend) == 0 {
		pend = e.Pendencias()
	}
	if len(pend) == 0 {
		return "Sua compra está completa ✅ Posso ajudar em mais alguma coisa?"
	}
	p := pend[0]
	switch {
	case strings.HasPrefix(p, "escolher viagem"):
		if len(e.Opcoes) > 0 {
			return textoOpcoes(e.Opcoes)
		}
		return TextoPedirRota
	case strings.HasPrefix(p, "informar passageiros"), strings.HasPrefix(p, "completar dados"), strings.HasPrefix(p, "incluir ao menos"):
		return textoPedirPassageiros(e)
	case strings.HasPrefix(p, "escolher pagamento"):
		return "Você prefere pagar o valor integral agora no PIX, ou só o sinal agora e o restante no embarque?"
	}
	return "Posso fechar a reserva e te mandar o PIX? 😊"
}

// textoPedirPassageiros pede nome e documento, usando a quantidade ja dita.
func textoPedirPassageiros(e conversa.Estado) string {
	n, cri := e.PessoasInformadas, e.CriancasInformadas
	if len(e.Passageiros) > 0 || n <= 0 {
		return "Pra seguir com a reserva, me manda o nome completo e o CPF (ou RG/CNH) de cada passageiro. Criança de até 5 anos precisa só do nome. 😊"
	}
	if n == 1 {
		return "Pra seguir com a reserva, me manda o nome completo e o CPF (ou RG/CNH) do passageiro. 😊"
	}
	if cri > 0 {
		return fmt.Sprintf("Certo, %d passageiros, sendo %d criança(s) de até 5 anos. Me manda o nome completo e o CPF (ou RG/CNH) de cada adulto, e só o nome das crianças. 😊", n, cri)
	}
	return fmt.Sprintf("Certo, %d passageiros. Me manda o nome completo e o CPF (ou RG/CNH) de cada um. 😊", n)
}

// textoTrecho: "Monção → Fraiburgo, seg 05/10 às 08:40, R$ 950 por pessoa".
func textoTrecho(o conversa.Opcao) string {
	data := o.Data
	if t, err := time.Parse("2006-01-02", o.Data); err == nil {
		data = diasCurtos[t.Weekday()] + " " + t.Format("02/01")
	}
	return fmt.Sprintf("%s → %s, %s às %s, %s por pessoa", o.Origem, o.Destino, data, o.Horario, formatarReais(o.Preco))
}

// textoEscolhido confirma a(s) viagem(ns) escolhida(s) e pede o proximo passo.
func textoEscolhido(e conversa.Estado) string {
	var b strings.Builder
	if len(e.Trechos) == 1 {
		fmt.Fprintf(&b, "Escolhido ✅\n%s", textoTrecho(e.Trechos[0].Viagem))
	} else {
		b.WriteString("Escolhido ✅")
		for i, t := range e.Trechos {
			fmt.Fprintf(&b, "\nTrecho %d: %s", i+1, textoTrecho(t.Viagem))
		}
	}
	b.WriteString("\n\n" + textoProximoPasso(e))
	return b.String()
}

// textoConfirmarRegistro lista os passageiros registrados e segue.
func textoConfirmarRegistro(e conversa.Estado) string {
	return textoRegistrados(e) + "\n\nSe algo estiver errado, é só me falar. " + textoProximoPasso(e)
}

// marcadoresVazamento: pedacos de formato interno do modelo que nunca podem
// chegar ao cliente.
var marcadoresVazamento = []string{"<tool_call", "</tool_call", "<think", "</think", "<|", "|>", "<function", "[TOOL_CALLS]", `{"name":`}

// textoQuebrado diz se a resposta tem formato interno vazado ou repeticao
// degenerada (ex.: "ellsellsellsells").
func textoQuebrado(texto string) string {
	for _, m := range marcadoresVazamento {
		if strings.Contains(texto, m) {
			return "formato interno vazado (" + m + ")"
		}
	}
	r := []rune(strings.ToLower(texto))
	for i := range r {
		for l := 2; l <= 6 && i+l <= len(r); l++ {
			seg := r[i : i+l]
			letras := 0
			for _, c := range seg {
				if unicode.IsLetter(c) {
					letras++
				}
			}
			if letras < 2 {
				continue
			}
			rep := 1
			for j := i + l; j+l <= len(r) && string(r[j:j+l]) == string(seg); j += l {
				rep++
			}
			if rep >= 5 {
				return "texto repetido (" + strings.Repeat(string(seg), 2) + "...)"
			}
		}
	}
	return ""
}

// afirmacao: frase que diz que uma acao foi feita, e como conferir no estado.
type afirmacao struct {
	re    *regexp.Regexp
	acao  string
	feito func(e conversa.Estado) bool
}

var afirmacoes = []afirmacao{
	{regexp.MustCompile(`\b(registrei|cadastrei|passageir[oa]s? (ja )?(estao |foram )?registrad|dados (ja )?(estao |foram )?registrad|anotei (seu|o|os|a|as) (cpf|documento|dados|nome))`), "registrou os passageiros (registrar_passageiros)",
		func(e conversa.Estado) bool { return len(e.Passageiros) > 0 }},
	{regexp.MustCompile(`\b(adicionei|selecionei|foi adicionad[oa]|esta escolhid[oa]|ficou escolhid[oa]|escolhi a (viagem|opcao)|trecho adicionado)`), "escolheu a viagem (escolher_viagem)",
		func(e conversa.Estado) bool { return len(e.Trechos) > 0 }},
	{regexp.MustCompile(`\b(reservei|reserva (feita|criada|confirmada|realizada|efetuada)|(sua )?reserva (esta|ta|foi) (feita|criada|confirmada))`), "criou a reserva (criar_reserva)",
		func(e conversa.Estado) bool { return e.AlgumReservado() }},
	{regexp.MustCompile(`\b(segue[m]? o[s]? pix|pix (gerado|copia e cola)|aqui esta o pix|gerei o pix)`), "gerou o PIX (gerar_pix)",
		func(e conversa.Estado) bool {
			for _, t := range e.Trechos {
				if t.PagamentoID != "" {
					return true
				}
			}
			return false
		}},
}

// afirmacoesSemAcao lista acoes que a resposta diz ter feito mas que o estado
// nao mostra (o modelo afirmou sem chamar a ferramenta).
func afirmacoesSemAcao(texto string, e conversa.Estado) []string {
	t := semAcento(strings.ToLower(texto))
	var out []string
	for _, af := range afirmacoes {
		if af.re.MatchString(t) && !af.feito(e) {
			out = append(out, af.acao)
		}
	}
	return out
}

// reCPF acha CPFs (com ou sem pontuacao) no texto do cliente.
var reCPF = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)

// cpfsNaoRegistrados devolve CPFs validos que o cliente mandou neste turno e
// que nao estao nos passageiros, enquanto ainda faltam dados de passageiros e
// nao ha reserva (depois disso um CPF pode ser so do pagador).
func cpfsNaoRegistrados(cliente []string, e conversa.Estado) []string {
	if e.AlgumReservado() || len(ferramentas.FaltaParaReserva(e)) == 0 && len(e.Passageiros) > 0 {
		return nil
	}
	tem := map[string]bool{}
	for _, p := range e.Passageiros {
		tem[soDigitos(p.Documento)] = true
	}
	var out []string
	for _, t := range cliente {
		for _, m := range reCPF.FindAllString(t, -1) {
			d := soDigitos(m)
			if !tem[d] && ferramentas.ValidarCPF(d) {
				tem[d] = true
				out = append(out, m)
			}
		}
	}
	return out
}

func soDigitos(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// problemasForma junta os problemas de forma da resposta (texto quebrado,
// acao afirmada sem ferramenta, CPF do cliente ignorado); vazio se ok.
func problemasForma(texto string, tc *turno, cliente []string) string {
	var p []string
	if q := textoQuebrado(texto); q != "" {
		p = append(p, q)
	}
	if as := afirmacoesSemAcao(texto, tc.estado); len(as) > 0 {
		p = append(p, "a resposta diz que "+strings.Join(as, " e ")+", mas a ferramenta nao foi chamada")
	}
	if cs := cpfsNaoRegistrados(cliente, tc.estado); len(cs) > 0 {
		p = append(p, "o cliente enviou o(s) CPF(s) "+strings.Join(cs, ", ")+" e registrar_passageiros nao foi chamada")
	}
	return strings.Join(p, "; ")
}

// TextoPedirRota e a resposta segura quando ainda nao ha rota para buscar.
const TextoPedirRota = "Pra eu te passar as datas e horários certinhos, me diz: de qual cidade você sai e pra qual cidade vai?"

// respostaSegura e o ultimo recurso quando o LLM insiste em citar dados sem
// origem e nao houve busca no turno: com rota no estado, busca e responde com
// as opcoes reais; sem rota, pede a rota. Vazio = transferir.
func (a *Agente) respostaSegura(ctx context.Context, tc *turno) string {
	o, d := tc.estado.Origem, tc.estado.Destino
	if o == nil && d == nil {
		return TextoPedirRota
	}
	args := map[string]string{}
	if o != nil {
		args["origem"] = o.Nome
	}
	if d != nil {
		args["destino"] = d.Nome
	}
	js, _ := json.Marshal(args)
	s := a.d.Ferramentas.Executar(ctx, &ferramentas.Contexto{Conversa: tc.c, Estado: &tc.estado, Agora: a.d.Agora()}, "buscar_viagens", js)
	tc.passos = append(tc.passos, conversa.Passo{Tipo: "ferramenta", Nome: "buscar_viagens", Entrada: json.RawMessage(js), Saida: s})
	if out, err := json.Marshal(s); err == nil {
		tc.resultados = append(tc.resultados, string(out))
	}
	if bs := opcoesDoTurno(tc.resultados); s.OK && len(bs) > 0 {
		return textoOpcoes(bs[len(bs)-1])
	}
	return ""
}

const motivoLimitePassos = "sem resposta apos o limite de passos"

// recuperar trata estouro do orcamento de tempo do turno ou do limite de
// passos: responde com o que ja foi obtido (PIX, opcoes da busca, ou busca da
// rota do estado). Sem nada aproveitavel, devolve false e o erro segue.
func (a *Agente) recuperar(ctx context.Context, tc *turno, err error) (string, bool) {
	var t *transf
	limite := errors.As(err, &t) && t.motivo == motivoLimitePassos
	tempo := errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil
	if !limite && !tempo {
		return "", false
	}
	motivo := "limite de passos"
	if tempo {
		motivo = "orcamento de tempo do turno"
	}
	texto := ""
	switch px, bs := pixDoTurno(tc.resultados), opcoesDoTurno(tc.resultados); {
	case len(px) > 0:
		texto = textoPix(px)
	case len(bs) > 0:
		texto = textoOpcoes(bs[len(bs)-1])
	case tc.estado.Origem != nil || tc.estado.Destino != nil:
		texto = a.respostaSegura(ctx, tc)
	}
	tc.passos = append(tc.passos, conversa.Passo{Tipo: "checagem", Nome: "recuperacao", Saida: map[string]any{"motivo": motivo, "resposta_segura": texto != ""}})
	return texto, texto != ""
}

// semRecuperacao: estouro do orcamento sem nada aproveitavel vira
// transferencia tecnica (o cliente recebe aviso, nao fica sem resposta).
func (a *Agente) semRecuperacao(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return &transf{motivo: "LLM lento: orcamento de tempo do turno esgotado", tecnico: true}
	}
	return err
}
