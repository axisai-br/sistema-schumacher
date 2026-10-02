package politica

import (
	_ "embed"
	"strings"
)

//go:embed faq.md
var faqTexto string

// FAQ devolve as respostas fixas por assunto (faq.md). Assunto presente com
// texto vazio quer dizer "nao temos essa informacao".
func FAQ() map[string]string {
	out := map[string]string{}
	chave := ""
	var b strings.Builder
	fechar := func() {
		if chave != "" {
			out[chave] = strings.TrimSpace(b.String())
		}
		b.Reset()
	}
	for _, l := range strings.Split(strings.ReplaceAll(faqTexto, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(l, "## ") {
			fechar()
			chave = strings.TrimSpace(strings.TrimPrefix(l, "## "))
			continue
		}
		if chave != "" {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	fechar()
	return out
}
