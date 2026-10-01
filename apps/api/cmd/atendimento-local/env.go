package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// bom e a marca de ordem de bytes UTF-8 que alguns editores do Windows gravam.
const bom = "\xef\xbb\xbf"

// parseEnv le o formato KEY=VALUE: ignora linhas vazias e comentarios (#),
// aceita o prefixo "export ", aspas simples ou duplas no valor e comentario
// no fim da linha (" # ...") quando o valor nao esta entre aspas.
func parseEnv(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	primeira := true
	for sc.Scan() {
		linha := sc.Text()
		if primeira {
			linha = strings.TrimPrefix(linha, bom)
			primeira = false
		}
		linha = strings.TrimSpace(linha)
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		linha = strings.TrimSpace(strings.TrimPrefix(linha, "export "))
		i := strings.Index(linha, "=")
		if i <= 0 {
			continue
		}
		chave := strings.TrimSpace(linha[:i])
		out[chave] = valorEnv(strings.TrimSpace(linha[i+1:]))
	}
	return out, sc.Err()
}

func valorEnv(v string) string {
	if len(v) >= 1 && (v[0] == '"' || v[0] == '\'') {
		if fim := strings.IndexByte(v[1:], v[0]); fim >= 0 {
			return v[1 : 1+fim]
		}
		return v[1:]
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "\t#"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// aplicarEnv copia para o ambiente as variaveis do arquivo que ainda nao
// existem (nao vazias) no ambiente do SO: o SO tem prioridade.
func aplicarEnv(vars map[string]string, lookup func(string) (string, bool), set func(k, v string)) {
	for k, v := range vars {
		if atual, ok := lookup(k); ok && strings.TrimSpace(atual) != "" {
			continue
		}
		set(k, v)
	}
}

// carregarArquivoEnv le o arquivo e aplica ao ambiente do processo. Se
// obrigatorio e falso, arquivo inexistente nao e erro. Devolve se carregou.
func carregarArquivoEnv(caminho string, obrigatorio bool) (bool, error) {
	f, err := os.Open(caminho)
	if err != nil {
		if os.IsNotExist(err) && !obrigatorio {
			return false, nil
		}
		return false, fmt.Errorf("arquivo de ambiente %s: %w", caminho, err)
	}
	defer f.Close()
	vars, err := parseEnv(f)
	if err != nil {
		return false, fmt.Errorf("arquivo de ambiente %s: %w", caminho, err)
	}
	aplicarEnv(vars, os.LookupEnv, func(k, v string) { _ = os.Setenv(k, v) })
	return true, nil
}
