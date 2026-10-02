package agente

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
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

// reNomeCPF: "nome sobrenome [,:-] cpf 123.456.789-09".
var reNomeCPF = regexp.MustCompile(`(?i)([\p{L}]+(?:\s+[\p{L}]+){1,5})\s*[,:-]?\s*cpf\s*[:é]?\s*(\d{3}\.?\d{3}\.?\d{3}-?\d{2})`)

// palavrasNaoNome: se aparecerem, o trecho antes do CPF nao e um nome.
var palavrasNaoNome = map[string]bool{"meu": true, "minha": true, "seu": true, "sua": true, "nome": true, "numero": true, "número": true,
	"é": true, "eh": true, "sou": true, "cpf": true, "rg": true, "documento": true, "dele": true, "dela": true, "passageiro": true}

// conectivosInicio: palavras soltas no comeco do trecho que nao fazem parte do nome.
var conectivosInicio = map[string]bool{"e": true, "a": true, "o": true, "com": true, "mais": true, "tambem": true, "também": true, "eu": true}

// passageirosDeTexto extrai pares nome + CPF escritos pelo cliente.
func passageirosDeTexto(textos []string) []conversa.Passageiro {
	var out []conversa.Passageiro
	for _, t := range textos {
		for _, m := range reNomeCPF.FindAllStringSubmatch(t, -1) {
			ws := strings.Fields(m[1])
			for len(ws) > 0 && conectivosInicio[strings.ToLower(ws[0])] {
				ws = ws[1:]
			}
			ok := len(ws) >= 2
			for _, w := range ws {
				if palavrasNaoNome[strings.ToLower(w)] {
					ok = false
				}
			}
			if !ok {
				continue
			}
			out = append(out, conversa.Passageiro{Nome: nomeProprio(strings.Join(ws, " ")), Documento: soDigitos(m[2]), TipoDocumento: "CPF"})
		}
	}
	return out
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
