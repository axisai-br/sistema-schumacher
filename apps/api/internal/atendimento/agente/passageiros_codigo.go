package agente

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// Passageiros extraidos em codigo, para quando o modelo nao chama
// registrar_passageiros: das fotos de documento (dados estruturados pela
// leitura de imagem) e, como ultimo recurso, de "nome sobrenome cpf 123...".

// reFotoDocumento acha "[foto de documento: ...]" (texto montado pela midia).
var reFotoDocumento = regexp.MustCompile(`\[foto de documento: ([^\]]+)\]`)

// passageirosDeFotos le os documentos fotografados nos textos do cliente.
// Certidao sem CPF so entra se a data de nascimento indicar ate 5 anos (a
// crianca nao precisa de documento); sem nome ou sem documento valido fica de
// fora (o LLM pede o que faltar). avisos explica, para o cliente, certidoes
// de quem tem mais de 5 anos e veio sem CPF/RG.
func passageirosDeFotos(textos []string, hoje time.Time) (out []conversa.Passageiro, avisos []string) {
	for _, t := range textos {
		for _, m := range reFotoDocumento.FindAllStringSubmatch(t, -1) {
			var p conversa.Passageiro
			certidao, nasc := false, ""
			for _, campo := range strings.Split(m[1], ", ") {
				c := strings.TrimSpace(campo)
				switch {
				case strings.HasPrefix(c, "certidão de nascimento"):
					certidao = true
				case strings.HasPrefix(c, "nome "):
					p.Nome = nomeProprio(strings.TrimPrefix(c, "nome "))
				case strings.HasPrefix(c, "CPF "):
					p.Documento, p.TipoDocumento = soDigitos(strings.TrimPrefix(c, "CPF ")), "CPF"
				case strings.HasPrefix(c, "RG ") && p.Documento == "":
					p.Documento, p.TipoDocumento = strings.TrimSpace(strings.TrimPrefix(c, "RG ")), "RG"
				case strings.HasPrefix(c, "nascimento "):
					nasc = strings.TrimPrefix(c, "nascimento ")
				}
			}
			anos, temIdade := idade(nasc, hoje)
			if temIdade && anos <= 5 {
				p.CriancaAte5 = true
			}
			if certidao && p.Nome != "" && p.Documento == "" && temIdade && anos > 5 {
				avisos = append(avisos, fmt.Sprintf("Recebi a certidão de %s (%d anos). Acima de 5 anos a criança paga passagem e precisa de CPF ou RG: me manda o número?", p.Nome, anos))
				continue
			}
			if p.Nome == "" || (p.Documento == "" && !p.CriancaAte5) {
				continue
			}
			out = append(out, p)
		}
	}
	return out, avisos
}

// idade em anos completos a partir de "DD/MM/AAAA".
func idade(nasc string, hoje time.Time) (int, bool) {
	d, err := time.Parse("02/01/2006", strings.TrimSpace(nasc))
	if err != nil || d.After(hoje) {
		return 0, false
	}
	anos := hoje.Year() - d.Year()
	if hoje.YearDay() < d.YearDay() {
		anos--
	}
	return anos, true
}

// reNomeCPF: "nome sobrenome [,:-] [cpf] 123.456.789-09". Sem a palavra "cpf"
// o numero so vale se for um CPF valido (evita telefone, codigo etc.).
var reNomeCPF = regexp.MustCompile(`(?i)([\p{L}]+(?:\s+[\p{L}]+){1,7})\s*[,:-]?\s*(cpf\s*[:é]?\s*)?(\d{3}\.?\d{3}\.?\d{3}-?\d{2})\b`)

// reNomeRG: "helena prado, RG 4.512.887 SSP/SC", "jose lima cnh 12345678900".
var reNomeRG = regexp.MustCompile(`(?i)([\p{L}]+(?:\s+[\p{L}]+){1,7})\s*[,:-]?\s*(rg|cnh)\s*[:é]?\s*(\d[\d.\-]{3,14}[\dxX])`)

// reChamaSe: "ele se chama Lucas Martins, tem 10 anos, cpf 123...".
var reChamaSe = regexp.MustCompile(`(?i)(?:se chama|chama-se|o nome d[ea]l[ea] (?:é|e))\s+([\p{L}]+(?:\s+[\p{L}]+){1,7})`)

var (
	reCPFSolto  = regexp.MustCompile(`\d{3}\.?\d{3}\.?\d{3}-?\d{2}`)
	reAnosSolto = regexp.MustCompile(`(?i)\b(\d{1,2})\s*anos?\b`)
)

// palavrasNaoNome: se aparecerem, o trecho antes do CPF nao e um nome.
var palavrasNaoNome = map[string]bool{"meu": true, "minha": true, "seu": true, "sua": true, "nome": true, "numero": true, "número": true,
	"é": true, "eh": true, "sou": true, "cpf": true, "rg": true, "documento": true, "dele": true, "dela": true, "passageiro": true}

// conectivosInicio: palavras soltas no comeco do trecho que nao fazem parte do nome.
var conectivosInicio = map[string]bool{"e": true, "a": true, "o": true, "com": true, "mais": true, "tambem": true, "também": true, "eu": true,
	"pronto": true, "ok": true, "segue": true, "seguem": true, "aqui": true, "vai": true, "vão": true, "vao": true, "então": true, "entao": true,
	"tá": true, "ta": true, "sim": true, "isso": true, "no": true, "na": true, "lugar": true, "dele": true, "dela": true, "outro": true, "outra": true}

// passageirosDeTexto extrai pares nome + CPF escritos pelo cliente.
func passageirosDeTexto(textos []string) []conversa.Passageiro {
	var out []conversa.Passageiro
	for _, t := range textos {
		ps, _ := extrairPassageirosTexto(t)
		out = append(out, ps...)
	}
	return out
}

// reBebe: "bebê Sofia Reis", "neném Ana Lima" (crianca de colo, ate 5 anos).
var reBebe = regexp.MustCompile(`(?i)(?:beb[eê]|nen[eê]m?|de colo)\s+([\p{L}]+(?:\s+[\p{L}]+){1,4})`)

// reIdade: "Sofia Reis de 2 anos", "Pedro Lima, 4 anos".
var reIdade = regexp.MustCompile(`(?i)([\p{L}]+(?:\s+[\p{L}]+){1,5})\s*,?\s*(?:de|com|tem)?\s*(\d{1,2})\s*anos?`)

// palavrasCrianca: palavras antes do nome que dizem "crianca", nao fazem parte dele.
var palavrasCrianca = map[string]bool{"bebe": true, "bebê": true, "nenem": true, "neném": true, "nenê": true, "crianca": true, "criança": true,
	"filho": true, "filha": true, "meu": true, "minha": true, "sobrinho": true, "sobrinha": true, "neto": true, "neta": true}

// limparNome tira conectivos/palavras de crianca do comeco e "de"/"e"/"com" do
// fim; vazio se sobrar menos de 2 palavras ou uma palavra que nao e de nome.
func limparNome(s string) string {
	ws := strings.Fields(s)
	for len(ws) > 0 && (conectivosInicio[strings.ToLower(ws[0])] || palavrasCrianca[strings.ToLower(ws[0])] || palavrasNaoNome[strings.ToLower(ws[0])]) {
		ws = ws[1:]
	}
	for len(ws) > 0 {
		u := strings.ToLower(ws[len(ws)-1])
		if u != "de" && u != "e" && u != "com" && u != "tem" {
			break
		}
		ws = ws[:len(ws)-1]
	}
	if len(ws) < 2 {
		return ""
	}
	for _, w := range ws {
		if palavrasNaoNome[strings.ToLower(w)] {
			return ""
		}
	}
	return nomeProprio(strings.Join(ws, " "))
}

// palavrasVazias: o que pode sobrar numa mensagem que so traz passageiros.
var palavrasVazias = map[string]bool{"e": true, "a": true, "o": true, "as": true, "os": true, "com": true, "mais": true, "tambem": true, "também": true,
	"eu": true, "sou": true, "meu": true, "minha": true, "nome": true, "é": true, "eh": true, "cpf": true, "dados": true, "passageiros": true,
	"são": true, "sao": true, "segue": true, "seguem": true, "aqui": true, "vai": true, "vão": true, "vao": true, "criancas": true, "crianças": true}

// reListaCriancas: "as crianças são Pedro Reis e Lara Reis", "crianças: Ana Lima, Bia Lima".
var reListaCriancas = regexp.MustCompile(`(?i)crian[cç]as?\s*(?:s[aã]o|:)\s*([\p{L}\s,]+)`)

// reSeparaNomes separa "Pedro Reis, Ana Lima e Lara Reis".
var reSeparaNomes = regexp.MustCompile(`\s*,\s*|\s+e\s+`)

// extrairPassageirosTexto acha passageiros escritos pelo cliente: "nome cpf",
// "bebê Nome" e "Nome de N anos" (ate 5 anos vira crianca; acima fica para o
// LLM, que pede documento). criancasAte5 e quantas criancas ate 5 anos o
// cliente ja disse que vao: com ela, "as crianças são X e Y" tambem entra.
// completo: a mensagem nao tem mais nada alem disso.
func extrairPassageirosTexto(texto string, criancasAte5 ...int) (ps []conversa.Passageiro, completo bool) {
	usado := make([]bool, len(texto))
	marcar := func(ini, fim int) {
		for i := ini; i < fim; i++ {
			usado[i] = true
		}
	}
	for _, m := range reNomeCPF.FindAllStringSubmatchIndex(texto, -1) {
		doc := soDigitos(texto[m[6]:m[7]])
		nome, comCPF := texto[m[2]:m[3]], m[4] >= 0
		// O nome guloso pode engolir a palavra "cpf" ("ana lima cpf 123..."):
		// devolve ela para o lugar dela.
		if ws := strings.Fields(nome); len(ws) > 0 && strings.EqualFold(strings.TrimRight(ws[len(ws)-1], ":"), "cpf") {
			nome, comCPF = strings.Join(ws[:len(ws)-1], " "), true
		}
		if !comCPF && !ferramentas.ValidarCPF(doc) {
			continue // sem "cpf" escrito, so aceita CPF valido
		}
		if n := limparNome(nome); n != "" {
			ps = append(ps, conversa.Passageiro{Nome: n, Documento: doc, TipoDocumento: "CPF"})
			marcar(m[0], m[1])
		}
	}
	for _, m := range reNomeRG.FindAllStringSubmatchIndex(texto, -1) {
		if n := limparNome(texto[m[2]:m[3]]); n != "" {
			ps = append(ps, conversa.Passageiro{Nome: n, Documento: strings.ToUpper(strings.NewReplacer(".", "", "-", "").Replace(texto[m[6]:m[7]])), TipoDocumento: strings.ToUpper(texto[m[4]:m[5]])})
			marcar(m[0], m[1])
		}
	}
	for _, m := range reChamaSe.FindAllStringSubmatchIndex(texto, -1) {
		n := limparNome(texto[m[2]:m[3]])
		if n == "" {
			continue
		}
		depois := texto[m[1]:min(len(texto), m[1]+80)]
		p := conversa.Passageiro{Nome: n}
		if c := reCPFSolto.FindString(depois); c != "" {
			p.Documento, p.TipoDocumento = soDigitos(c), "CPF"
		}
		if a := reAnosSolto.FindStringSubmatch(depois); a != nil {
			if anos, _ := strconv.Atoi(a[1]); anos <= 5 {
				p.CriancaAte5 = true
			}
		}
		if p.Documento == "" && !p.CriancaAte5 {
			continue
		}
		ps = append(ps, p)
		marcar(m[0], min(len(texto), m[1]+80))
	}
	for _, m := range reBebe.FindAllStringSubmatchIndex(texto, -1) {
		if n := limparNome(texto[m[2]:m[3]]); n != "" {
			ps = append(ps, conversa.Passageiro{Nome: n, CriancaAte5: true})
			marcar(m[0], m[1])
		}
	}
	for _, m := range reIdade.FindAllStringSubmatchIndex(texto, -1) {
		anos, err := strconv.Atoi(texto[m[4]:m[5]])
		n := limparNome(texto[m[2]:m[3]])
		if err != nil || n == "" {
			continue
		}
		if anos <= 5 {
			ps = append(ps, conversa.Passageiro{Nome: n, CriancaAte5: true})
			marcar(m[0], m[1])
		}
	}
	if len(criancasAte5) > 0 && criancasAte5[0] > 0 {
		for _, m := range reListaCriancas.FindAllStringSubmatchIndex(texto, -1) {
			var nomes []string
			for _, parte := range reSeparaNomes.Split(texto[m[2]:m[3]], -1) {
				if n := limparNome(parte); n != "" {
					nomes = append(nomes, n)
				}
			}
			// So quando a lista cabe na quantidade de criancas ja dita.
			if len(nomes) == 0 || len(nomes) > criancasAte5[0] {
				continue
			}
			for _, n := range nomes {
				ps = append(ps, conversa.Passageiro{Nome: n, CriancaAte5: true})
			}
			marcar(m[0], m[1])
		}
	}
	ps, _ = mesclarPassageiros(nil, ps)
	var resto strings.Builder
	for i := range texto {
		if !usado[i] {
			resto.WriteByte(texto[i])
		}
	}
	completo = len(ps) > 0
	for _, w := range strings.FieldsFunc(strings.ToLower(resto.String()), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !palavrasVazias[w] {
			completo = false
			break
		}
	}
	return ps, completo
}

// nomeProprio: "JOAO DA SILVA" -> "Joao da Silva".
func nomeProprio(s string) string {
	ws := strings.Fields(strings.ToLower(s))
	for i, w := range ws {
		if i > 0 && (w == "da" || w == "de" || w == "do" || w == "das" || w == "dos" || w == "e") {
			continue
		}
		r := []rune(w)
		ws[i] = strings.ToUpper(string(r[:1])) + string(r[1:])
	}
	return strings.Join(ws, " ")
}

// mesclarPassageiros junta os novos aos atuais (sem repetir documento ou
// nome). Devolve a lista e se entrou alguem.
func mesclarPassageiros(atuais, novos []conversa.Passageiro) ([]conversa.Passageiro, bool) {
	out := append([]conversa.Passageiro{}, atuais...)
	mudou := false
	for _, n := range novos {
		dup := false
		for _, a := range out {
			if (n.Documento != "" && soDigitos(a.Documento) == soDigitos(n.Documento)) || strings.EqualFold(a.Nome, n.Nome) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, n)
			mudou = true
		}
	}
	return out, mudou
}

// registrarEmCodigo chama registrar_passageiros com os atuais + novos (antes
// de qualquer reserva). Devolve as mensagens da chamada e se deu certo.
func (a *Agente) registrarEmCodigo(ctx context.Context, tc *turno, novos []conversa.Passageiro) ([]llm.Mensagem, bool) {
	if tc.estado.AlgumReservado() || len(novos) == 0 {
		return nil, false
	}
	lista, mudou := mesclarPassageiros(tc.estado.Passageiros, novos)
	if !mudou {
		return nil, false
	}
	args := make([]map[string]any, 0, len(lista))
	for _, p := range lista {
		m := map[string]any{"nome": p.Nome, "crianca_ate_5": p.CriancaAte5}
		if p.Documento != "" {
			m["documento"] = p.Documento
			if p.TipoDocumento != "" {
				m["tipo_documento"] = p.TipoDocumento
			}
		}
		args = append(args, m)
	}
	return a.preExecutar(ctx, tc, "registrar_passageiros", map[string]any{"passageiros": args})
}

// textoRegistrados lista os passageiros do estado (documento mascarado).
func textoRegistrados(e conversa.Estado) string {
	var b strings.Builder
	b.WriteString("Anotei os passageiros:\n")
	for i, p := range e.Passageiros {
		fmt.Fprintf(&b, "%d. %s", i+1, p.Nome)
		if p.Documento != "" {
			fmt.Fprintf(&b, " (%s %s)", valorOuTipo(p.TipoDocumento), conversa.MascararDocumento(p.Documento))
		}
		if p.CriancaAte5 {
			b.WriteString(" (criança até 5 anos, não paga)")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func valorOuTipo(t string) string {
	if t == "" {
		return "doc"
	}
	return t
}
