package agente

import (
	"context"
	"fmt"
	"strings"
	"time"

	"schumacher-tur/api/internal/atendimento/conversa"
	"schumacher-tur/api/internal/atendimento/llm"
	"schumacher-tur/api/internal/atendimento/politica"
)

// Resposta livre no motor por comandos: quando o codigo nao tem uma resposta
// especifica para a mensagem (pergunta fora do fluxo, comentario, duvida,
// "nao entendi"), o LLM responde com autoridade de conversa, mas SEM
// ferramentas: nao reserva, nao registra, nao escolhe. O texto passa pelas
// mesmas checagens do motor antigo (fatos com origem, acao afirmada sem ter
// acontecido, texto quebrado) e pelo filtro de saida; se falhar, vale a
// resposta montada em codigo.

const instrucoesLivre = `

# COMO RESPONDER AGORA
- Você conversa com o cliente, mas NÃO executa nada nesta resposta: não reserva, não registra passageiro, não escolhe viagem, não gera PIX. Nunca diga que fez alguma dessas coisas.
- Responda de verdade ao que o cliente disse ou perguntou, usando só a política, o catálogo, a SITUAÇÃO e os DADOS CONSULTADOS abaixo. Se não souber, diga que o suporte confirma (+55 49 9886-2222).
- Nossas viagens ligam cidades do Maranhão a cidades de Santa Catarina (nos dois sentidos). Não há viagem entre duas cidades do mesmo estado.
- Se o cliente já disse algo (cidade, data, quantidade), não peça de novo: use.
- Termine conduzindo para o próximo passo da compra, numa frase curta. Próximo passo sugerido pelo sistema: %s
- No máximo 4 frases curtas, uma pergunta só, texto simples de WhatsApp.`

// responderLivre devolve a resposta do LLM ou ok=false (sem LLM, erro,
// timeout ou reprovada nas checagens).
func (a *Agente) responderLivre(ctx context.Context, tc *turno, hist []conversa.Mensagem, proximo string) (string, bool) {
	catalogo := ""
	if a.d.Catalogo != nil {
		catalogo, _ = a.d.Catalogo.TextoCatalogo(ctx)
	}
	agora := a.d.Agora().In(a.loc)
	var b strings.Builder
	b.WriteString(politica.Texto())
	b.WriteString("\n\n# CATÁLOGO\n")
	b.WriteString(catalogo)
	b.WriteString("\n\n")
	b.WriteString(RegrasSempre)
	b.WriteString("\n\n# SITUAÇÃO DA COMPRA AGORA (fonte da verdade)\n")
	b.WriteString(TextoSituacao(tc.estado, a.cfg.SinalPorPagante))
	if len(tc.resultados) > 0 {
		b.WriteString("\n\n# DADOS CONSULTADOS NESTE TURNO (resultado de busca do sistema)\n")
		for _, r := range tc.resultados {
			b.WriteString(truncarRunas(r, 1500))
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, instrucoesLivre, strings.ReplaceAll(proximo, "\n", " "))
	nome := strings.TrimSpace(tc.c.Nome)
	if nome == "" {
		nome = "(desconhecido)"
	}
	fmt.Fprintf(&b, "\n\n# CONTEXTO\nData e hora atuais: %s, %s (America/Sao_Paulo)\nNome do cliente: %s\n",
		diasSemana[agora.Weekday()], agora.Format("02/01/2006 15:04"), nome)

	ultimas := hist
	if len(ultimas) > 10 {
		ultimas = ultimas[len(ultimas)-10:]
	}
	ctxL, cancel := context.WithTimeout(ctx, a.cfg.OrcamentoExtrator)
	defer cancel()
	t0 := a.d.Agora()
	resp, err := a.d.Modelo.Gerar(ctxL, llm.Pedido{
		Modelo: a.cfg.Modelo, Instrucoes: b.String(), Mensagens: mapearHistorico(ultimas), MaxTokens: 400,
	})
	passo := conversa.Passo{Tipo: "llm", Nome: "resposta_livre", DuracaoMS: time.Duration(a.d.Agora().Sub(t0)).Milliseconds()}
	defer func() { tc.passos = append(tc.passos, passo) }()
	if err != nil {
		passo.Erro = err.Error()
		return "", false
	}
	tc.tokIn += resp.TokensEntrada
	tc.tokOut += resp.TokensSaida
	texto := strings.TrimSpace(resp.Texto)
	motivo := ""
	cliente := clienteRecente(hist)
	switch {
	case texto == "" || len(resp.Chamadas) > 0:
		motivo = "vazio_ou_chamada"
	case strings.HasPrefix(texto, "{") || strings.HasPrefix(texto, "[") || strings.Contains(texto, `":`):
		motivo = "json"
	case textoQuebrado(texto) != "" || degenerado(texto):
		motivo = "quebrado"
	case problemasForma(texto, tc, cliente) != "":
		motivo = "forma: " + problemasForma(texto, tc, cliente)
	case a.problemasResposta(tc, texto, catalogo, agora, cliente, a.nomesCidades(ctx)) != "":
		motivo = "fatos: " + a.problemasResposta(tc, texto, catalogo, agora, cliente, a.nomesCidades(ctx))
	}
	passo.Saida = map[string]any{"texto": texto, "reprovado": motivo}
	if motivo != "" {
		return "", false
	}
	return texto, true
}

// clienteRecente: textos das mensagens novas do cliente (fim do historico).
func clienteRecente(hist []conversa.Mensagem) []string {
	var out []string
	for i := len(hist) - 1; i >= 0 && hist[i].Autor == conversa.AutorCliente; i-- {
		out = append([]string{hist[i].Texto}, out...)
	}
	return out
}
