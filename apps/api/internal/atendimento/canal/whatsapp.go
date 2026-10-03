package canal

import (
	"regexp"
	"strings"
)

// Formatacao do texto para o WhatsApp, aplicada pelo agente ANTES de gravar a
// saida: o eco fromMe do webhook e reconhecido pelo texto gravado, entao o que
// fica no historico precisa ser exatamente o que foi enviado.

var (
	// Codigo PIX copia e cola (BR Code): comeca em 000201 e termina no CRC
	// (6304 + 4 hex). Pode ter espacos (nome do recebedor), entao vai ate o fim
	// da linha; sem CRC reconhecivel, ate o primeiro espaco.
	rePixCodigo = regexp.MustCompile(`000201[^\n]*6304[0-9A-Fa-f]{4}|000201\S+`)
	// **negrito** / __italico__ do markdown. As bordas nao podem ser asterisco
	// (ou sublinhado), para nao casar documento mascarado como "***725".
	reNegritoMD = regexp.MustCompile(`(^|[^*])\*\*([^*\s](?:[^*\n]*[^*\s])?)\*\*($|[^*])`)
	reItalicoMD = regexp.MustCompile(`(^|[^_\w])__([^_\s](?:[^_\n]*[^_\s])?)__($|[^_\w])`)
	reTachadoMD = regexp.MustCompile(`~~([^~\n]+)~~`)
	reTituloMD  = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*\s*$`)
)

// PartesWhatsApp converte o markdown para o formato do WhatsApp e separa cada
// codigo PIX copia e cola numa mensagem so dele, para o cliente copiar com um
// toque. Devolve as mensagens na ordem de envio (nunca vazias).
func PartesWhatsApp(texto string) []string {
	var partes []string
	add := func(s string, formatar bool) {
		if formatar {
			s = FormatarWhatsApp(s)
		}
		if s = strings.TrimSpace(s); s != "" {
			partes = append(partes, s)
		}
	}
	resto := texto
	for {
		loc := rePixCodigo.FindStringIndex(resto)
		if loc == nil {
			add(resto, true)
			return partes
		}
		add(resto[:loc[0]], true)
		add(resto[loc[0]:loc[1]], false)
		resto = resto[loc[1]:]
	}
}

// FormatarWhatsApp troca o markdown que o WhatsApp nao entende pelo dele:
// **x** -> *x*, __x__ -> _x_, ~~x~~ -> ~x~ e "# Titulo" -> *Titulo*.
func FormatarWhatsApp(texto string) string {
	texto = reTituloMD.ReplaceAllStringFunc(texto, func(l string) string {
		t := strings.Trim(reTituloMD.ReplaceAllString(l, "$1"), "* ")
		return "*" + t + "*"
	})
	texto = substituirTudo(reNegritoMD, texto, "$1*$2*$3")
	texto = substituirTudo(reItalicoMD, texto, "${1}_${2}_$3")
	return reTachadoMD.ReplaceAllString(texto, "~$1~")
}

// substituirTudo repete a troca ate estabilizar: a borda consumida por um
// casamento ("**a**,**b**") impede o vizinho de casar na mesma passada.
func substituirTudo(re *regexp.Regexp, texto, repl string) string {
	for i := 0; i < 5; i++ {
		novo := re.ReplaceAllString(texto, repl)
		if novo == texto {
			break
		}
		texto = novo
	}
	return texto
}
