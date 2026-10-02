package agente

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/ferramentas"
	"schumacher-tur/api/internal/atendimento/llm"
)

// Mudancas na lista de passageiros feitas em codigo: novos, correcao de
// documento ("errei o cpf da maria, o certo é ..."), remocao ("o bruno não vai
// mais") e conflitos (CPF repetido, CPF invalido). O LLM nunca precisa
// "atualizar" a lista: o codigo grava e o template mostra o resultado.

var (
	// "cpf da maria ... 987.654.321-00", "documento do joão é 123...".
	reCorrecaoNome = regexp.MustCompile(`(?i)(?:cpf|documento|doc|rg)\s+d[aoe]\s+([\p{L}]+)[^\d]{0,40}?(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)
	// "o cpf tá errado, o certo é 529..." (sem nome: so com um adulto).
	reCorrecaoSolta = regexp.MustCompile(`(?i)(?:cpf|documento)[^\d]{0,30}(?:errad|cert|corret)[^\d]{0,30}?(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)
	// "o bruno não vai mais", "a ana não vai".
	reNaoVai = regexp.MustCompile(`(?i)((?:[\p{L}]+\s+){1,3})n[aã]o\s+vai(?:\s+mais)?\b`)
	// "tira o bruno", "remove a ana".
	reTirar = regexp.MustCompile(`(?i)\b(?:tira|tirar|remove|remover)\s+(?:o|a)\s+([\p{L}]+)`)
)

// mudancaPassageiros aplica o texto do cliente a lista atual. Devolve a lista
// final, se mudou, se a mensagem so tratava de passageiros (para responder por
// template) e avisos para o cliente (o que nao entrou e por que).
func mudancaPassageiros(atuais []conversa.Passageiro, texto string, criancasAte5 int) (lista []conversa.Passageiro, mudou, completo bool, avisos []string) {
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
	for _, n := range novos {
		if n.TipoDocumento == "CPF" && n.Documento != "" && !ferramentas.ValidarCPF(n.Documento) {
			avisos = append(avisos, fmt.Sprintf("O CPF de %s (%s) não é válido. Confere e me manda de novo?", n.Nome, conversa.MascararDocumento(n.Documento)))
			continue
		}
		var aviso string
		var entrou bool
		lista, entrou, aviso = juntarPassageiro(lista, n)
		if aviso != "" {
			avisos = append(avisos, aviso)
		}
		mudou = mudou || entrou
	}

	// Correcoes de documento de quem ja esta na lista.
	corrigiu := false
	for _, m := range reCorrecaoNome.FindAllStringSubmatch(texto, -1) {
		if i := acharPassageiro(lista, m[1]); i >= 0 {
			corrigiu = corrigirDocumento(&lista[i], m[2], &avisos) || corrigiu
		}
	}
	if !corrigiu {
		if m := reCorrecaoSolta.FindStringSubmatch(texto); m != nil {
			if i := unicoAdulto(lista); i >= 0 {
				corrigiu = corrigirDocumento(&lista[i], m[1], &avisos)
			}
		}
	}
	mudou = mudou || removeu || corrigiu
	if removeu || corrigiu {
		completo = true // a resposta mostra a lista para o cliente conferir
	}
	return lista, mudou, completo, avisos
}

// juntarPassageiro poe n na lista: mesmo nome com documento novo corrige;
// mesmo documento com outro nome e conflito (fica o primeiro, avisa).
func juntarPassageiro(lista []conversa.Passageiro, n conversa.Passageiro) ([]conversa.Passageiro, bool, string) {
	for i, a := range lista {
		mesmoNome := strings.EqualFold(semAcento(a.Nome), semAcento(n.Nome))
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
