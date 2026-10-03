package main

// Veredito automatico de roteiro: a linha "# ESPERA: chave=valor ..." no
// roteiro diz o resultado esperado e o simulador confere no fim, contra o
// estado final, os fakes e as respostas do bot. Assim "-roteiro todos" mede a
// taxa de sucesso sem leitura manual.
//
// Chaves: reservas, pix, trechos, passageiros, criancas (numeros);
// transferiu (sim|nao); pagamento (integral|sinal|nenhum); origem, destino
// (nome da cidade do primeiro trecho, aspas para nomes com espaco);
// proibido=/regex/ (nenhuma resposta do bot pode casar) e exige=/regex/
// (alguma resposta precisa casar), estado=/regex/ (casa com o resumo do
// estado final), repetiveis.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"schumacher-tur/api/internal/atendimento/conversa"
)

type espera struct {
	nums      map[string]int
	textos    map[string]string
	proibidos []*regexp.Regexp
	exigidos  []*regexp.Regexp
	estado    []*regexp.Regexp // devem casar com Estado.Resumo() (documentos mascarados)
}

func (e espera) vazia() bool {
	return len(e.nums) == 0 && len(e.textos) == 0 && len(e.proibidos) == 0 && len(e.exigidos) == 0 && len(e.estado) == 0
}

// regrasGlobais valem para todo roteiro: persona, ferramentas internas,
// markdown e documento completo nunca podem aparecer para o cliente.
var regrasGlobais = []struct {
	nome string
	re   *regexp.Regexp
}{
	{"vazou_persona", regexp.MustCompile(`(?i)nvidia|nemotron|modelo de linguagem|language model|\bLLM\b`)},
	{"vazou_ferramenta", regexp.MustCompile(`(?i)\b(buscar_viagens|escolher_viagem|registrar_passageiros|criar_reserva|gerar_pix|trocar_pagamento|consultar_reserva|transferir_para_humano|remover_trecho|interpretar_data|listar_rotas)\b`)},
	{"markdown", regexp.MustCompile(`(^|[^*])\*\*[^*\d\s]|__\S`)}, // "***725" (documento mascarado) nao conta
	{"cpf_completo", regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)},
}

var (
	reEspera  = regexp.MustCompile(`(?m)^#\s*ESPERA:\s*(.+)$`)
	reParEsp  = regexp.MustCompile(`(\w+)=("[^"]*"|/(?:[^/\\]|\\.)*/|\S+)`)
	chavesNum = map[string]bool{"reservas": true, "pix": true, "trechos": true, "passageiros": true, "criancas": true}
	chavesTxt = map[string]bool{"transferiu": true, "pagamento": true, "origem": true, "destino": true}
)

// parseEspera le as linhas "# ESPERA:" de um roteiro.
func parseEspera(txt string) (espera, error) {
	e := espera{nums: map[string]int{}, textos: map[string]string{}}
	for _, m := range reEspera.FindAllStringSubmatch(txt, -1) {
		for _, p := range reParEsp.FindAllStringSubmatch(m[1], -1) {
			k, v := strings.ToLower(p[1]), p[2]
			switch {
			case chavesNum[k]:
				n, err := strconv.Atoi(v)
				if err != nil {
					return e, fmt.Errorf("ESPERA %s=%q: numero invalido", k, v)
				}
				e.nums[k] = n
			case chavesTxt[k]:
				e.textos[k] = strings.ToLower(strings.Trim(v, `"`))
			case k == "proibido" || k == "exige" || k == "estado":
				if !strings.HasPrefix(v, "/") || !strings.HasSuffix(v, "/") || len(v) < 2 {
					return e, fmt.Errorf("ESPERA %s=%q: use /regex/", k, v)
				}
				re, err := regexp.Compile("(?i)" + v[1:len(v)-1])
				if err != nil {
					return e, fmt.Errorf("ESPERA %s: %v", k, err)
				}
				switch k {
				case "proibido":
					e.proibidos = append(e.proibidos, re)
				case "exige":
					e.exigidos = append(e.exigidos, re)
				default:
					e.estado = append(e.estado, re)
				}
			default:
				return e, fmt.Errorf("ESPERA: chave desconhecida %q", k)
			}
		}
	}
	return e, nil
}

// lerEspera abre o mesmo roteiro que abrirRoteiro (arquivo ou embutido).
func lerEspera(ref string) (espera, error) {
	if b, err := os.ReadFile(ref); err == nil {
		return parseEspera(string(b))
	}
	for _, nome := range []string{
		strings.TrimSuffix(filepath.ToSlash(ref), ".txt"),
		strings.TrimSuffix(filepath.Base(ref), ".txt"),
	} {
		if b, err := roteirosFS.ReadFile("roteiros/" + nome + ".txt"); err == nil {
			return parseEspera(string(b))
		}
	}
	return espera{nums: map[string]int{}, textos: map[string]string{}}, nil
}

// resultadoRoteiro e o que o veredito compara.
type resultadoRoteiro struct {
	Reservas, Pix int
	Transferiu    bool
	Estado        conversa.Estado
	Respostas     []string
}

// avaliar devolve as falhas (vazio = sucesso). As regras globais sempre valem.
func (e espera) avaliar(r resultadoRoteiro) []string {
	var f []string
	confere := func(k string, got int) {
		if want, ok := e.nums[k]; ok && want != got {
			f = append(f, fmt.Sprintf("%s=%d (esperado %d)", k, got, want))
		}
	}
	confere("reservas", r.Reservas)
	confere("pix", r.Pix)
	confere("trechos", len(r.Estado.Trechos))
	confere("passageiros", len(r.Estado.Passageiros))
	cri := 0
	for _, p := range r.Estado.Passageiros {
		if p.CriancaAte5 {
			cri++
		}
	}
	confere("criancas", cri)
	if want, ok := e.textos["transferiu"]; ok && want != map[bool]string{true: "sim", false: "nao"}[r.Transferiu] {
		f = append(f, fmt.Sprintf("transferiu=%v (esperado %s)", r.Transferiu, want))
	}
	if want, ok := e.textos["pagamento"]; ok {
		got := r.Estado.Pagamento
		if got == "" {
			got = "nenhum"
		}
		if got != want {
			f = append(f, fmt.Sprintf("pagamento=%s (esperado %s)", got, want))
		}
	}
	for _, k := range []string{"origem", "destino"} {
		want, ok := e.textos[k]
		if !ok {
			continue
		}
		got := ""
		if len(r.Estado.Trechos) > 0 {
			got = r.Estado.Trechos[0].Viagem.Origem
			if k == "destino" {
				got = r.Estado.Trechos[0].Viagem.Destino
			}
		}
		if semAcentoMin(got) != semAcentoMin(want) {
			f = append(f, fmt.Sprintf("%s=%q (esperado %q)", k, got, want))
		}
	}
	tudo := strings.Join(r.Respostas, "\n")
	for _, re := range e.proibidos {
		if m := re.FindString(tudo); m != "" {
			f = append(f, fmt.Sprintf("proibido %s: %q", re, m))
		}
	}
	for _, re := range e.exigidos {
		if !re.MatchString(tudo) {
			f = append(f, fmt.Sprintf("exige %s: ausente", re))
		}
	}
	resumo := r.Estado.Resumo()
	for _, re := range e.estado {
		if !re.MatchString(resumo) {
			f = append(f, fmt.Sprintf("estado %s: ausente", re))
		}
	}
	for _, g := range regrasGlobais {
		for _, txt := range r.Respostas {
			if g.nome == "cpf_completo" {
				txt = removerCodigosPix(txt)
			}
			if m := g.re.FindString(txt); m != "" {
				f = append(f, fmt.Sprintf("%s: %q", g.nome, m))
				break
			}
		}
	}
	return f
}

var rePixCopiaCola = regexp.MustCompile(`000201\S+`)

// removerCodigosPix tira o copia-e-cola (cheio de digitos) antes de procurar CPF.
func removerCodigosPix(s string) string { return rePixCopiaCola.ReplaceAllString(s, "") }

func semAcentoMin(s string) string {
	r := strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")
	return r.Replace(strings.ToLower(strings.TrimSpace(s)))
}
