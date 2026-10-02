package main

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed roteiros
var roteirosFS embed.FS

// tipoLinha classifica uma linha de roteiro.
type tipoLinha int

const (
	linhaMensagem  tipoLinha = iota // mensagem normal: registra e processa
	linhaEnfileira                  // "+texto": registra sem processar
	linhaComando                    // "/comando args"
)

type linhaRoteiro struct {
	Tipo  tipoLinha
	Texto string // para comandos, inclui a barra
}

// parseRoteiro le um roteiro: cada linha e o que o cliente digitaria. Linhas
// vazias e as que comecam com # sao ignoradas.
func parseRoteiro(r io.Reader) ([]linhaRoteiro, error) {
	var out []linhaRoteiro
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	primeira := true
	for sc.Scan() {
		l := sc.Text()
		if primeira {
			l = strings.TrimPrefix(l, bom)
			primeira = false
		}
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(l, "/"):
			out = append(out, linhaRoteiro{linhaComando, l})
		case strings.HasPrefix(l, "+"):
			if t := strings.TrimSpace(l[1:]); t != "" {
				out = append(out, linhaRoteiro{linhaEnfileira, t})
			}
		default:
			out = append(out, linhaRoteiro{linhaMensagem, l})
		}
	}
	return out, sc.Err()
}

// abrirRoteiro aceita um caminho de arquivo ou o nome de um roteiro embutido
// (com ou sem .txt).
func abrirRoteiro(ref string) ([]linhaRoteiro, string, error) {
	if b, err := os.ReadFile(ref); err == nil {
		l, err := parseRoteiro(strings.NewReader(string(b)))
		return l, ref, err
	}
	for _, nome := range []string{
		strings.TrimSuffix(filepath.ToSlash(ref), ".txt"), // "stress/a01_girias"
		strings.TrimSuffix(filepath.Base(ref), ".txt"),
	} {
		if b, err := roteirosFS.ReadFile("roteiros/" + nome + ".txt"); err == nil {
			l, err := parseRoteiro(strings.NewReader(string(b)))
			return l, nome, err
		}
	}
	return nil, "", fmt.Errorf("roteiro %q não encontrado (use um caminho de arquivo ou um nome de -lista)", ref)
}

// nomesRoteiros lista os roteiros da raiz (casos reais de producao).
func nomesRoteiros() []string { return nomesEm("roteiros", "") }

// nomesStress lista os roteiros do teste de estresse (roteiros/stress).
func nomesStress() []string { return nomesEm("roteiros/stress", "stress/") }

func nomesEm(dir, prefixo string) []string {
	ents, _ := roteirosFS.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".txt") {
			out = append(out, prefixo+strings.TrimSuffix(n, ".txt"))
		}
	}
	sort.Strings(out)
	return out
}

// grupoRoteiros resolve os nomes especiais de -roteiro: todos (raiz), stress
// e tudo (os dois).
func grupoRoteiros(ref string) ([]string, bool) {
	switch ref {
	case "todos":
		return nomesRoteiros(), true
	case "stress":
		return nomesStress(), true
	case "tudo":
		return append(nomesRoteiros(), nomesStress()...), true
	case "rapido":
		return append([]string(nil), roteirosRapidos...), true
	}
	return nil, false
}

// roteirosRapidos: amostra de ~15 min que cobre os casos que mais quebraram
// (passageiros, pagamento, pos-reserva, troca de rota, fora do fluxo,
// persona). Para conferir uma mudanca antes da rodada completa (-roteiro tudo).
var roteirosRapidos = []string{
	"certidao_crianca",
	"correcao_cpf",
	"ida_volta_mesma_mensagem",
	"ja_paguei",
	"troca_pagamento_apos_pix",
	"troca_passageiro_apos_reserva",
	"stress/a01_girias",
	"stress/a07_tudo_junto",
	"stress/b03_cpf_duplicado",
	"stress/b08_somos3_dois_nomes",
	"stress/b11_corrigir_cpf_maria",
	"stress/c01_quanto_sinal",
	"stress/c07_nao_fechar_desistir",
	"stress/d04_perguntas_soltas",
	"stress/d05_muda_rota",
	"stress/d08_injecao",
	"stress/e01_fora_do_fluxo",
}
