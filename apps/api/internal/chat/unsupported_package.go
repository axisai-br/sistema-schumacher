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
	if isSupportedPackageDestination(destination) {
		return unsupportedPackageQuery{}, false
	}
	return unsupportedPackageQuery{Destination: destination}, true
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
			"mode":        "UNSUPPORTED_PACKAGE_ROUTE",
			"destination": query.Destination,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text": reply,
		},
	}
}

func buildUnsupportedPackageReply() string {
	return "No momento atendemos apenas viagens dos pacotes Santa Catarina e Maranhao. Para outras rotas, fale com nosso atendimento pelo numero " + unsupportedPackageSupportPhone + "."
}
