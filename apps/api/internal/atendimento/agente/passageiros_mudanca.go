package agente

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// Mudancas na lista de passageiros feitas em codigo: novos, correcao de
// documento ("errei o cpf da maria, o certo é ..."), remocao ("o bruno não vai
// mais") e conflitos (CPF repetido, CPF invalido). O LLM nunca precisa
// "atualizar" a lista: o codigo grava e o template mostra o resultado.
//
// Pendentes: pessoas citadas antes que ainda nao entraram (certidao de quem
// tem mais de 5 anos, CPF invalido, CPF repetido, parente sem documento). Um
// documento mandado depois ("o cpf dele é ...", "o da sandra é ...") completa
// o pendente.

var (
	// "cpf da maria ... 987.654.321-00", "documento do joão é 123...".
	reCorrecaoNome = regexp.MustCompile(`(?i)(?:cpf|documento|doc|rg)\s+d[aoe]\s+([\p{L}]+)[^\d]{0,40}?(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)
	// "o da sandra é 390...", "do gabriel: 987...".
	reDoNomeE = regexp.MustCompile(`(?i)\bd[ao]\s+([\p{L}]+)\s*(?:[eé]|fica|:)\s*:?\s*(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)
	// "o cpf dele é 111...", "documento dela 529...".
	reDocDele = regexp.MustCompile(`(?i)(?:cpf|documento|doc)\s+d(?:ele|ela)\s*(?:[eé]|:)?\s*(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)
	// "o cpf tá errado, o certo é 529..." (sem nome).
	reCorrecaoSolta = regexp.MustCompile(`(?i)(?:\b(?:certo|correto)\b|errad[oa])[^\d]{0,30}?(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)
	// "o bruno não vai mais", "a ana não vai".
	reNaoVai = regexp.MustCompile(`(?i)((?:[\p{L}]+\s+){1,3})n[aã]o\s+vai(?:\s+mais)?\b`)
	// "tira o bruno", "remove a ana".
	reTirar = regexp.MustCompile(`(?i)\b(?:tira|tirar|remove|remover)\s+(?:o|a)\s+([\p{L}]+)`)
	// "minha mãe rosa alves", "meu marido, joão lima" (parente com nome).
	reParenteNome = regexp.MustCompile(`(?i)\b(?:minha|meu)\s+(?:m[aã]e|pai|esposa|esposo|marido|mulher|irm[aã]o?|sogr[ao]|av[oóô]|ti[ao]|namorad[ao]|cunhad[ao]|amig[ao]|prim[ao])\s*,?\s*([\p{L}]+(?:\s+[\p{L}]+){1,5})`)
	// "meu filho de 4 anos, Pedro Alves", "o menino é Davi Rocha".
	reCriancaNome = regexp.MustCompile(`(?i)\b(?:(?:meu|minha|o|a)\s+)?(?:filh[oa]|net[oa]|menin[oa]|crian[cç]a|beb[eê]|nen[eé]m?)\s*(?:de\s+(\d{1,2})\s+anos?)?\s*,?\s*(?:[eé]\s+|se chama\s+|chamad[oa]\s+)?([\p{L}]+(?:\s+[\p{L}]+){1,5})`)
)

// mudancaPassageiros aplica o texto do cliente a lista atual. pend sao as
// pessoas pendentes de antes. Devolve a lista final, se mudou, se a mensagem
// so tratava de passageiros (para responder por template), avisos para o
// cliente e quantas pessoas citadas ficaram de fora (para a reserva esperar).
func mudancaPassageiros(atuais []conversa.Passageiro, texto string, criancasAte5 int, pend []conversa.Passageiro) (lista []conversa.Passageiro, mudou, completo bool, avisos []string, fora int) {
	lista = append([]conversa.Passageiro{}, atuais...)

	// Remocoes primeiro: "no lugar dele vai o Carlos" troca, nao acumula.
	removeu := false
	for _, m := range reNaoVai.FindAllStringSubmatch(texto, -1) {
		for _, w := range strings.Fields(m[1]) {
			if i := acharPassageiro(lista, w); i >= 0 {
				lista = append(lista[:i], lista[i+1:]...)
				removeu = true
				break
			}
		}
	}
	for _, m := range reTirar.FindAllStringSubmatch(texto, -1) {
		if i := acharPassageiro(lista, m[1]); i >= 0 {
			lista = append(lista[:i], lista[i+1:]...)
			removeu = true
		}
	}

	novos, completo := extrairPassageirosTexto(texto, criancasAte5)
	novos = append(novos, criancasPorNome(texto, criancasAte5, novos)...)
	for _, n := range novos {
		if n.TipoDocumento == "CPF" && n.Documento != "" && !ferramentas.ValidarCPF(n.Documento) {
			avisos = append(avisos, fmt.Sprintf("O CPF de %s (%s) não é válido. Confere e me manda de novo?", n.Nome, conversa.MascararDocumento(n.Documento)))
			fora++
			continue
		}
		var aviso string
		var entrou bool
		lista, entrou, aviso = juntarPassageiro(lista, n)
		if aviso != "" {
			avisos = append(avisos, aviso)
			fora++
		}
		mudou = mudou || entrou
	}
	// Parente citado sem documento ("e minha mãe rosa alves"): pede o documento.
	for _, p := range parentesSemDocumento(texto, lista) {
		avisos = append(avisos, fmt.Sprintf("Falta o CPF (ou RG) de %s.", p.Nome))
		fora++
	}

	// Documento de quem ja esta na lista ou esta pendente.
	completou := func(nome, doc string) bool {
		if i := acharPassageiro(lista, nome); i >= 0 {
			return corrigirDocumento(&lista[i], doc, &avisos)
		}
		if i := acharPassageiro(pend, nome); i >= 0 {
			return incluirPendente(&lista, pend[i], doc, &avisos)
		}
		return false
	}
	corrigiu := false
	for _, re := range []*regexp.Regexp{reCorrecaoNome, reDoNomeE} {
		for _, m := range re.FindAllStringSubmatch(texto, -1) {
			corrigiu = completou(m[1], m[2]) || corrigiu
		}
	}
	if !corrigiu {
		if m := reDocDele.FindStringSubmatch(texto); m != nil && len(pend) == 1 {
			corrigiu = incluirPendente(&lista, pend[0], m[1], &avisos)
		}
	}
	if !corrigiu && len(novos) == 0 {
		if m := reCorrecaoSolta.FindStringSubmatch(texto); m != nil {
			switch {
			case len(pend) == 1:
				corrigiu = incluirPendente(&lista, pend[0], m[1], &avisos)
			case unicoAdulto(lista) >= 0:
				corrigiu = corrigirDocumento(&lista[unicoAdulto(lista)], m[1], &avisos)
			}
		}
	}
	mudou = mudou || removeu || corrigiu
	if removeu || corrigiu {
		completo = true // a resposta mostra a lista para o cliente conferir
	}
	return lista, mudou, completo, avisos, fora
}

// incluirPendente poe na lista a pessoa pendente com o documento recebido.
func incluirPendente(lista *[]conversa.Passageiro, p conversa.Passageiro, doc string, avisos *[]string) bool {
	d := soDigitos(doc)
	if !ferramentas.ValidarCPF(d) {
		*avisos = append(*avisos, fmt.Sprintf("O CPF de %s (%s) não é válido. Confere?", p.Nome, conversa.MascararDocumento(d)))
		return false
	}
	p.Documento, p.TipoDocumento = d, "CPF"
	l, entrou, aviso := juntarPassageiro(*lista, p)
	if aviso != "" {
		*avisos = append(*avisos, aviso)
	}
	*lista = l
	return entrou
}

// criancasPorNome: "meu filho de 4 anos, Pedro Alves", "o menino é Davi
// Rocha" (com criancas ate 5 anos ja informadas). Sem idade, so quando o
// cliente ja disse que vai crianca ate 5.
func criancasPorNome(texto string, criancasAte5 int, ja []conversa.Passageiro) []conversa.Passageiro {
	var out []conversa.Passageiro
	for _, m := range reCriancaNome.FindAllStringSubmatch(texto, -1) {
		nome := limparNome(m[2])
		if nome == "" {
			continue
		}
		anos := -1
		if m[1] != "" {
			anos, _ = strconv.Atoi(m[1])
		}
		if (anos >= 0 && anos <= 5) || (anos < 0 && criancasAte5 > 0) {
			dup := false
			for _, p := range ja {
				if nomesCompativeis(p.Nome, nome) {
					dup = true
				}
			}
			if !dup {
				out = append(out, conversa.Passageiro{Nome: nome, CriancaAte5: true})
			}
		}
	}
	return out
}

// parentesSemDocumento: parentes citados com nome que nao entraram na lista.
func parentesSemDocumento(texto string, lista []conversa.Passageiro) []conversa.Passageiro {
	var out []conversa.Passageiro
	for _, m := range reParenteNome.FindAllStringSubmatch(texto, -1) {
		nome := limparNome(m[1])
		if nome == "" {
			continue
		}
		achou := false
		for _, p := range lista {
			if nomesCompativeis(p.Nome, nome) {
				achou = true
			}
		}
		if !achou {
			out = append(out, conversa.Passageiro{Nome: nome})
		}
	}
	return out
}

// pendentesDoHistorico: pessoas citadas nas mensagens anteriores do cliente
// (fora as do turno atual) que nao estao na lista.
func pendentesDoHistorico(hist []conversa.Mensagem, lista []conversa.Passageiro, hoje time.Time) []conversa.Passageiro {
	novas := 0
	for i := len(hist) - 1; i >= 0 && hist[i].Autor == conversa.AutorCliente; i-- {
		novas++
	}
	var textos []string
	for _, m := range hist[:len(hist)-novas] {
		if m.Autor == conversa.AutorCliente {
			textos = append(textos, m.Texto)
		}
	}
	var cand []conversa.Passageiro
	_, _, pendFotos := lerFotos(textos, hoje)
	cand = append(cand, pendFotos...)
	for _, t := range textos {
		ps, _ := extrairPassageirosTexto(t)
		cand = append(cand, ps...)
		cand = append(cand, parentesSemDocumento(t, nil)...)
	}
	var out []conversa.Passageiro
	for _, c := range cand {
		dentro := false
		for _, p := range lista {
			if nomesCompativeis(p.Nome, c.Nome) {
				dentro = true
			}
		}
		for _, p := range out {
			if nomesCompativeis(p.Nome, c.Nome) {
				dentro = true
			}
		}
		if !dentro {
			out = append(out, conversa.Passageiro{Nome: c.Nome})
		}
	}
	return out
}

// nomesCompativeis: um nome contem todas as palavras do outro ("Valdir" e
// "Valdir Costa"), sem acento e sem caixa.
func nomesCompativeis(a, b string) bool {
	wa := strings.Fields(semAcento(strings.ToLower(a)))
	wb := strings.Fields(semAcento(strings.ToLower(b)))
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	if len(wa) > len(wb) {
		wa, wb = wb, wa
	}
	tem := map[string]bool{}
	for _, w := range wb {
		tem[w] = true
	}
	for _, w := range wa {
		if !tem[w] {
			return false
		}
	}
	return true
}

// juntarPassageiro poe n na lista: mesmo nome com documento novo corrige;
// mesmo documento com nome compativel e a mesma pessoa; mesmo documento com
// outro nome e conflito (fica o primeiro, avisa).
func juntarPassageiro(lista []conversa.Passageiro, n conversa.Passageiro) ([]conversa.Passageiro, bool, string) {
	for i, a := range lista {
		mesmoNome := nomesCompativeis(a.Nome, n.Nome)
		mesmoDoc := n.Documento != "" && soDigitos(a.Documento) != "" && strings.EqualFold(normDoc(a.Documento), normDoc(n.Documento))
		switch {
		case mesmoNome && mesmoDoc:
			return lista, false, ""
		case mesmoNome:
			if n.Documento == "" || normDoc(a.Documento) == normDoc(n.Documento) {
				return lista, false, ""
			}
			lista[i].Documento, lista[i].TipoDocumento = n.Documento, n.TipoDocumento
			return lista, true, ""
		case mesmoDoc:
			return lista, false, fmt.Sprintf("O documento %s apareceu para %s e para %s. Me manda o documento certo de %s?",
				conversa.MascararDocumento(n.Documento), a.Nome, n.Nome, n.Nome)
		}
	}
	return append(lista, n), true, ""
}

func corrigirDocumento(p *conversa.Passageiro, doc string, avisos *[]string) bool {
	d := soDigitos(doc)
	if !ferramentas.ValidarCPF(d) {
		*avisos = append(*avisos, fmt.Sprintf("O CPF novo de %s (%s) não é válido. Confere?", p.Nome, conversa.MascararDocumento(d)))
		return false
	}
	if soDigitos(p.Documento) == d {
		return false
	}
	p.Documento, p.TipoDocumento = d, "CPF"
	return true
}

// acharPassageiro acha pelo primeiro nome (ou nome inteiro), sem acento.
func acharPassageiro(lista []conversa.Passageiro, nome string) int {
	alvo := semAcento(strings.ToLower(strings.TrimSpace(nome)))
	if alvo == "" || palavrasNaoNome[alvo] || conectivosInicio[alvo] || palavrasCrianca[alvo] {
		return -1
	}
	for i, p := range lista {
		ns := strings.Fields(semAcento(strings.ToLower(p.Nome)))
		if len(ns) > 0 && (ns[0] == alvo || semAcento(strings.ToLower(p.Nome)) == alvo) {
			return i
		}
	}
	return -1
}

func unicoAdulto(lista []conversa.Passageiro) int {
	idx := -1
	for i, p := range lista {
		if !p.CriancaAte5 {
			if idx >= 0 {
				return -1
			}
			idx = i
		}
	}
	return idx
}

func normDoc(s string) string {
	return strings.ToUpper(strings.NewReplacer(".", "", "-", "", "/", "", " ", "").Replace(s))
}

// registrarLista grava a lista completa com registrar_passageiros (antes de
// qualquer reserva). So altera o estado se a ferramenta aceitar.
func (a *Agente) registrarLista(ctx context.Context, tc *turno, lista []conversa.Passageiro) ([]llm.Mensagem, bool) {
	if tc.estado.AlgumReservado() || len(lista) == 0 {
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

// ---- quantidade de pessoas escrita ----

var numerosPorExtenso = map[string]int{"um": 1, "uma": 1, "dois": 2, "duas": 2, "tres": 3, "três": 3, "quatro": 4, "cinco": 5, "seis": 6, "sete": 7, "oito": 8}

var (
	reSomosN    = regexp.MustCompile(`(?i)\b(?:somos|seremos|vamos em|vao|vão|sao|são)\s+(\d|um|uma|dois|duas|tr[eê]s|quatro|cinco|seis|sete|oito)\b`)
	reNPessoas  = regexp.MustCompile(`(?i)\b(\d|um|uma|dois|duas|tr[eê]s|quatro|cinco|seis|sete|oito)\s+(?:pessoas|passageiros|passagens|adultos)\b`)
	reSoEu      = regexp.MustCompile(`(?i)\b(?:s[oó]|somente|apenas)\s+eu\b|\bvou sozinh[oa]\b`)
	reEuEMais   = regexp.MustCompile(`(?i)\beu\s+e\s+mais\s+(\d|um|uma|dois|duas|tr[eê]s|quatro|cinco)\b`)
	reEuEAlguem = regexp.MustCompile(`(?i)\beu\s+e\s+(?:a\s+|o\s+)?(?:minha|meu)\s+[\p{L}]+`)
)

// quantidadeDoTexto le quantas pessoas o cliente disse que vao ("somos 3",
// "3 pessoas", "só eu", "eu e minha esposa", "eu e mais 2").
func quantidadeDoTexto(texto string) (int, bool) {
	num := func(s string) int {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		return numerosPorExtenso[strings.ToLower(s)]
	}
	switch {
	case reEuEMais.MatchString(texto):
		return 1 + num(reEuEMais.FindStringSubmatch(texto)[1]), true
	case reSomosN.MatchString(texto):
		return num(reSomosN.FindStringSubmatch(texto)[1]), true
	case reNPessoas.MatchString(texto):
		return num(reNPessoas.FindStringSubmatch(texto)[1]), true
	case reSoEu.MatchString(texto):
		return 1, true
	case reEuEAlguem.MatchString(texto):
		return 2, true
	}
	return 0, false
}

// aplicarQuantidadeTexto grava a quantidade escrita quando o Roteador nao
// gravou e ela e maior que a lista atual. Devolve se mudou.
func aplicarQuantidadeTexto(texto string, est *conversa.Estado) bool {
	n, ok := quantidadeDoTexto(texto)
	if !ok || n < 1 || n > 9 || n == est.PessoasInformadas || n < len(est.Passageiros) {
		return false
	}
	est.PessoasInformadas = n
	if est.CriancasInformadas >= n {
		est.CriancasInformadas = 0
	}
	return true
}
