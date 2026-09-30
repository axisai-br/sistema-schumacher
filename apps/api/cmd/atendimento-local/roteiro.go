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

//go:embed roteiros/*.txt
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
	nome := strings.TrimSuffix(filepath.Base(ref), ".txt")
	if b, err := roteirosFS.ReadFile("roteiros/" + nome + ".txt"); err == nil {
		l, err := parseRoteiro(strings.NewReader(string(b)))
		return l, nome, err
	}
	return nil, "", fmt.Errorf("roteiro %q não encontrado (use um caminho de arquivo ou um nome de -lista)", ref)
}

func nomesRoteiros() []string {
	ents, _ := roteirosFS.ReadDir("roteiros")
	var out []string
	for _, e := range ents {
		if n := e.Name(); strings.HasSuffix(n, ".txt") {
			out = append(out, strings.TrimSuffix(n, ".txt"))
		}
	}
	sort.Strings(out)
	return out
}
