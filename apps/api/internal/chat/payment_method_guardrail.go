package chat

import "strings"

func looksLikeUnsupportedPaymentMethodQuestion(text string) bool {
	folded := normalizeGuardrailPhrase(text)
	if folded == "" {
		return false
	}
	if strings.Contains(folded, "forma") && strings.Contains(folded, "pagamento") {
		return true
	}
	if strings.Contains(folded, "outra forma") ||
		strings.Contains(folded, "sem ser pix") ||
		strings.Contains(folded, "sem pix") {
		return true
	}
	for _, phrase := range []string{
		"cartao",
		"credito",
		"debito",
		"dinheiro",
		"boleto",
		"transferencia",
		"parcelar",
		"parcelamento",
		"pagar presencialmente",
		"pagar no onibus",
		"pagar no embarque",
	} {
		if strings.Contains(folded, normalizeGuardrailPhrase(phrase)) {
			return true
		}
	}
	return false
}
