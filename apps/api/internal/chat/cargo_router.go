package chat

import "strings"

const unsupportedCargoReply = "No atendimento automático, consigo ajudar apenas com passagens e bagagens comuns do passageiro. Para enviar moto ou qualquer item que não seja bagagem, fale com o suporte: +55 49 9886-2222."

type UnsupportedCargoQuery struct {
	Intent string
	Item   string
}

func inferUnsupportedCargoQuery(text string) (UnsupportedCargoQuery, bool) {
	folded := " " + strings.Join(strings.Fields(foldChatText(NormalizeIncomingCustomerText(text))), " ") + " "
	if folded == "" {
		return UnsupportedCargoQuery{}, false
	}

	intent, hasIntent := inferUnsupportedCargoIntent(folded)
	if !hasIntent {
		return UnsupportedCargoQuery{}, false
	}

	item, hasUnsupportedItem := inferUnsupportedCargoItem(folded)
	if !hasUnsupportedItem {
		return UnsupportedCargoQuery{}, false
	}

	return UnsupportedCargoQuery{
		Intent: intent,
		Item:   item,
	}, true
}

func inferUnsupportedCargoIntent(folded string) (string, bool) {
	patterns := []struct {
		term   string
		intent string
	}{
		{term: " enviar ", intent: "enviar"},
		{term: " envia ", intent: "enviar"},
		{term: " levar ", intent: "levar"},
		{term: " levam ", intent: "levar"},
		{term: " leva ", intent: "levar"},
		{term: " transportar ", intent: "transportar"},
		{term: " transporta ", intent: "transportar"},
		{term: " transportam ", intent: "transportar"},
		{term: " despachar ", intent: "despachar"},
		{term: " despacha ", intent: "despachar"},
		{term: " mandar ", intent: "mandar"},
		{term: " manda ", intent: "mandar"},
		{term: " aceitam ", intent: "aceitar"},
		{term: " aceita ", intent: "aceitar"},
	}
	for _, pattern := range patterns {
		if strings.Contains(folded, pattern.term) {
			return pattern.intent, true
		}
	}
	return "", false
}

func inferUnsupportedCargoItem(folded string) (string, bool) {
	unsupportedItems := []string{
		"moto",
		"motocicleta",
		"carro",
		"veiculo",
		"bicicleta",
		"bike",
		"encomenda",
		"mercadoria",
		"carga",
		"mudanca",
		"movel",
		"moveis",
		"geladeira",
		"fogao",
		"eletrodomestico",
		"eletrodomesticos",
		"animal",
		"animais",
		"pet",
		"cachorro",
		"gato",
	}
	for _, item := range unsupportedItems {
		if strings.Contains(folded, " "+item+" ") {
			return item, true
		}
	}
	return "", false
}

func buildUnsupportedCargoDraftRun(query UnsupportedCargoQuery) RunAgentResult {
	return RunAgentResult{
		ReplyText: unsupportedCargoReply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":          "TEMPLATE_FIRST_REPLY",
			"intent":        string(IntentUnsupportedCargo),
			"template_name": string(TemplateUnsupportedCargo),
			"cargo_intent":  query.Intent,
			"item":          query.Item,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    unsupportedCargoReply,
			"intent":        string(IntentUnsupportedCargo),
			"template_name": string(TemplateUnsupportedCargo),
		},
	}
}
