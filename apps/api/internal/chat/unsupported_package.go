package chat

import "strings"

const unsupportedPackageSupportPhone = "+55 49 9886-2222"

type unsupportedPackageQuery struct {
	Destination string
}

func inferUnsupportedPackageQuery(text string) (unsupportedPackageQuery, bool) {
	text = NormalizeIncomingCustomerText(text)
	if looksLikeRescheduleIntent(text) {
		return unsupportedPackageQuery{}, false
	}
	destination, ok := inferExplicitTravelDestination(text)
	if !ok {
		return unsupportedPackageQuery{}, false
	}
	if isReservationHelpActionDestinationFragment(text, destination) {
		return unsupportedPackageQuery{}, false
	}
	if isSupportedPackageDestination(destination) {
		return unsupportedPackageQuery{}, false
	}
	return unsupportedPackageQuery{Destination: destination}, true
}

func isReservationHelpActionDestinationFragment(text string, destination string) bool {
	if !looksLikeReservationHowToProceedIntent(text) {
		return false
	}
	switch strings.Join(strings.Fields(foldChatText(destination)), " ") {
	case "fazer reserva", "fazer uma reserva", "reservar", "reservar passagem", "reservar uma passagem":
		return true
	default:
		return false
	}
}

func inferUnsupportedRouteFollowUp(history []Message, text string) (unsupportedPackageQuery, bool) {
	body := NormalizeIncomingCustomerText(text)
	if body == "" || !looksLikePotentialRouteFollowUpAnswer(body) {
		return unsupportedPackageQuery{}, false
	}

	switch {
	case lastAssistantAskedMADestination(history):
		return unsupportedIfNoSupportedCity(body, maPackageDestinations)
	case lastAssistantAskedOriginInMaranhao(history):
		return unsupportedIfNoSupportedCity(body, maPackageDestinations)
	case lastAssistantAskedOriginInSantaCatarina(history):
		return unsupportedIfNoSupportedCity(body, scPackageDestinations)
	default:
		return unsupportedPackageQuery{}, false
	}
}

func unsupportedIfNoSupportedCity(text string, candidates map[string]string) (unsupportedPackageQuery, bool) {
	if _, ok := findSingleSupportedCityInText(text, candidates); ok {
		return unsupportedPackageQuery{}, false
	}
	return unsupportedPackageQuery{Destination: strings.Join(strings.Fields(strings.TrimSpace(text)), " ")}, true
}

func lastAssistantAskedMADestination(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !isAssistantRouteQuestionMessage(message) {
			continue
		}
		folded := foldChatText(message.Body)
		if folded == "" {
			continue
		}
		return strings.Contains(folded, " para qual cidade ") &&
			(strings.Contains(folded, " maranhao ") || strings.Contains(folded, " ma "))
	}
	return false
}

func looksLikePotentialRouteFollowUpAnswer(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	if looksLikeHumanSupportIntent(folded) ||
		looksLikeBookingCancelIntent(text) ||
		looksLikePaymentLookupIntent(text) ||
		looksLikePaymentCreateIntent(text) ||
		looksLikeRescheduleIntent(folded) ||
		looksLikeShortDateFollowUp(text) {
		return false
	}
	if passengerCount, _, ok := parsePassengerCountReply(text); ok && passengerCount > 0 {
		return false
	}
	if _, ok := inferExplicitTravelDestination(text); ok {
		return true
	}

	words := strings.Fields(folded)
	if len(words) == 0 || len(words) > 4 {
		return false
	}
	blocked := map[string]struct{}{
		"nao": {}, "sei": {}, "sim": {}, "ok": {}, "isso": {}, "pode": {},
		"quero": {}, "passagem": {}, "valor": {}, "preco": {}, "data": {},
		"quando": {}, "horario": {}, "tem": {}, "vaga": {}, "vagas": {},
		"opcao": {}, "primeira": {}, "segunda": {}, "terceira": {},
	}
	for _, word := range words {
		if _, blockedWord := blocked[word]; blockedWord {
			return false
		}
	}
	return true
}

func inferExplicitTravelDestination(text string) (string, bool) {
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return "", false
	}

	folded := foldChatText(body)
	if match := routeFromToPattern.FindStringSubmatch(body); len(match) == 3 {
		destination := strings.Join(strings.Fields(strings.TrimSpace(match[2])), " ")
		if destination != "" {
			return destination, true
		}
	}

	destination := destinationAfterLastConnector(folded)
	if destination == "" {
		return "", false
	}

	if looksLikeExplicitTravelDestinationQuery(body, folded) || looksLikeBareFromToRoute(folded) {
		return destination, true
	}
	return "", false
}

func destinationAfterLastConnector(folded string) string {
	index := strings.LastIndex(folded, " para ")
	length := len(" para ")
	if praIndex := strings.LastIndex(folded, " pra "); praIndex > index {
		index = praIndex
		length = len(" pra ")
	}
	if index < 0 {
		return ""
	}
	return strings.Join(strings.Fields(strings.TrimSpace(folded[index+length:])), " ")
}

func looksLikeExplicitTravelDestinationQuery(text string, folded string) bool {
	locations := extractCanonicalLocations(text)
	if looksLikeAvailabilityIntent(text, len(locations)) {
		return true
	}

	patterns := []string{
		" quero ir ",
		" ir para ",
		" ir pra ",
		" viajar para ",
		" viajar pra ",
		" viagem para ",
		" viagem pra ",
		" tem viagem para ",
		" tem viagem pra ",
		" passagem para ",
		" passagem pra ",
		" onibus para ",
		" onibus pra ",
	}
	for _, pattern := range patterns {
		if strings.Contains(folded, pattern) {
			return true
		}
	}
	return false
}

func looksLikeBareFromToRoute(folded string) bool {
	index := strings.LastIndex(folded, " para ")
	if praIndex := strings.LastIndex(folded, " pra "); praIndex > index {
		index = praIndex
	}
	if index < 0 {
		return false
	}

	before := strings.Fields(strings.TrimSpace(folded[:index]))
	after := strings.Fields(destinationAfterLastConnector(folded))
	return len(before) >= 2 && len(after) >= 1
}

func isSupportedPackageDestination(destination string) bool {
	destination = NormalizeIncomingCustomerText(destination)
	folded := foldChatText(destination)
	switch detectBroadTravelState(folded) {
	case "SC", "MA":
		return true
	}

	for key := range scPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return true
		}
	}
	for key := range maPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return true
		}
	}
	return false
}

func foldChatDestinationKey(key string) string {
	return strings.TrimSpace(foldChatText(key))
}

func buildUnsupportedPackageDraftRun(query unsupportedPackageQuery) RunAgentResult {
	reply := buildUnsupportedPackageReply()
	return RunAgentResult{
		ReplyText: reply,
		Model:     "deterministic_unsupported_package",
		RequestPayload: map[string]interface{}{
			"mode":          "UNSUPPORTED_PACKAGE_ROUTE",
			"intent":        string(IntentUnsupportedPackage),
			"template_name": string(TemplateUnsupportedPackage),
			"destination":   query.Destination,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"intent":        string(IntentUnsupportedPackage),
			"template_name": string(TemplateUnsupportedPackage),
		},
	}
}

func buildUnsupportedPackageReply() string {
	return "No momento atendemos apenas viagens dos pacotes Santa Catarina e Maranhao. Para outras rotas, fale com nosso atendimento pelo numero " + unsupportedPackageSupportPhone + "."
}
