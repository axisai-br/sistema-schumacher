package chat

import (
	"strings"
	"time"
)

type Intent string

const (
	IntentUnknown                    Intent = "UNKNOWN"
	IntentAvailabilitySearch         Intent = "AVAILABILITY_SEARCH"
	IntentSelectAvailabilityOption   Intent = "SELECT_AVAILABILITY_OPTION"
	IntentPassengerCountReply        Intent = "PASSENGER_COUNT_REPLY"
	IntentPassengerDocumentsProvided Intent = "PASSENGER_DOCUMENTS_PROVIDED"
	IntentBookingCreateConfirmation  Intent = "BOOKING_CREATE_CONFIRMATION"
	IntentPaymentPreference          Intent = "PAYMENT_PREFERENCE"
	IntentPaymentStatusQuery         Intent = "PAYMENT_STATUS_QUERY"
	IntentPaymentCreate              Intent = "PAYMENT_CREATE"
	IntentBookingCancel              Intent = "BOOKING_CANCEL"
	IntentReschedule                 Intent = "RESCHEDULE"
	IntentUnsupportedCargo           Intent = "UNSUPPORTED_CARGO"
	IntentUnsupportedPackage         Intent = "UNSUPPORTED_PACKAGE"
	IntentHumanSupport               Intent = "HUMAN_SUPPORT"
)

type IntentDecision struct {
	Intent              Intent
	Source              string
	SelectedOptionIndex int
	AvailabilityInput   *AvailabilitySearchInput
	TemplateName        ResponseTemplateName
	Action              string
	TemplateData        map[string]interface{}
}

func routeDeterministicIntent(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
	body := NormalizeIncomingCustomerText(currentTurn)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if body == "" {
		return IntentDecision{Intent: IntentUnknown, Source: "deterministic"}
	}
	if _, ok := inferUnsupportedCargoQuery(body); ok {
		return IntentDecision{Intent: IntentUnsupportedCargo, Source: "deterministic", TemplateName: TemplateUnsupportedCargo, Action: "template"}
	}
	if looksLikeHumanSupportIntent(folded) {
		return IntentDecision{Intent: IntentHumanSupport, Source: "deterministic"}
	}
	if looksLikeBookingCancelIntent(body) {
		return IntentDecision{Intent: IntentBookingCancel, Source: "deterministic"}
	}
	if looksLikePaymentLookupIntent(body) {
		return IntentDecision{Intent: IntentPaymentStatusQuery, Source: "deterministic"}
	}
	if looksLikePaymentCreateIntent(body) {
		intent := IntentPaymentCreate
		if state.Phase == ConversationPhaseBooked && strings.TrimSpace(state.Payment.Preference) == "" {
			intent = IntentPaymentPreference
		}
		return IntentDecision{Intent: intent, Source: "deterministic"}
	}
	if looksLikeRescheduleIntent(folded) {
		return IntentDecision{Intent: IntentReschedule, Source: "deterministic"}
	}
	if decision, ok := routeBroadStateTemplateIntent(body, folded); ok {
		return decision
	}
	if index := extractSelectedOptionIndex(body); index > 0 && hasPreviousAvailabilityList(history) {
		return IntentDecision{
			Intent:              IntentSelectAvailabilityOption,
			Source:              "deterministic",
			SelectedOptionIndex: index,
			TemplateName:        TemplateAskPassengerCount,
			Action:              "template",
		}
	}
	if looksLikeContextualAvailabilitySelection(folded) && hasPreviousAvailabilityList(history) {
		return IntentDecision{
			Intent:              IntentSelectAvailabilityOption,
			Source:              "deterministic",
			SelectedOptionIndex: firstAvailableOptionIndex(history),
			TemplateName:        TemplateAskPassengerCount,
			Action:              "template",
		}
	}
	if looksLikeCreateBookingIntent(body) || looksLikeBookingCreateConfirmation(body) {
		return IntentDecision{Intent: IntentBookingCreateConfirmation, Source: "deterministic", Action: "legacy_tool"}
	}
	if passengerCount, _, ok := parsePassengerCountReply(body); ok && passengerCount > 0 {
		return IntentDecision{Intent: IntentPassengerCountReply, Source: "deterministic"}
	}
	if input, ok := parseSCDestinationFollowUpAfterPublicTable(history, body); ok {
		return IntentDecision{
			Intent:            IntentAvailabilitySearch,
			Source:            "deterministic",
			AvailabilityInput: &input,
			TemplateName:      TemplateAskMAOrigin,
			Action:            "template",
		}
	}
	if input, ok := parseMADestinationFollowUpAfterSCOrigin(history, body); ok {
		return IntentDecision{
			Intent:            IntentAvailabilitySearch,
			Source:            "deterministic_ma_destination_followup",
			AvailabilityInput: &input,
			Action:            "tool",
		}
	}
	if input, ok := parseSCOriginFollowUpAfterMaranhaoQuery(history, body); ok {
		return IntentDecision{
			Intent:            IntentAvailabilitySearch,
			Source:            "deterministic",
			AvailabilityInput: &input,
			TemplateName:      TemplateAskMADestination,
			Action:            "template",
		}
	}
	if state.Phase == ConversationPhaseBooked {
		if detectRequestedPaymentType(body) != "" ||
			looksLikePixOnlyPaymentReply(folded) ||
			looksLikePaymentCreateConfirmationReply(folded) {
			return IntentDecision{
				Intent: IntentPaymentCreate,
				Source: "deterministic_payment_context",
				Action: "tool",
			}
		}
	}
	if query, ok := inferUnsupportedRouteFollowUp(history, body); ok {
		return IntentDecision{
			Intent:       IntentUnsupportedPackage,
			Source:       "deterministic_unsupported_followup",
			TemplateName: TemplateUnsupportedPackage,
			Action:       "template",
			TemplateData: map[string]interface{}{"destination": query.Destination},
		}
	}
	if input, ok := parseOriginAnswerAvailabilitySearchInput(history, body, observedAt); ok {
		return IntentDecision{Intent: IntentAvailabilitySearch, Source: "deterministic_origin_followup", AvailabilityInput: &input, Action: "tool"}
	}
	if input, ok := parseAvailabilitySearchInput(history, body, observedAt); ok {
		return IntentDecision{Intent: IntentAvailabilitySearch, Source: "deterministic", AvailabilityInput: &input, Action: "tool"}
	}
	return IntentDecision{Intent: IntentUnknown, Source: "deterministic"}
}

func routeBroadStateTemplateIntent(body string, folded string) (IntentDecision, bool) {
	if looksLikeBroadStateScheduleLookup(body) {
		return IntentDecision{}, false
	}
	context := inferRouteContextFromText(body)
	if strings.TrimSpace(context.Origin) != "" || strings.TrimSpace(context.Destination) != "" {
		return IntentDecision{}, false
	}
	switch detectBroadTravelState(folded) {
	case "SC":
		input := enrichAvailabilitySearchInput(AvailabilitySearchInput{
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       8,
		})
		return IntentDecision{
			Intent:            IntentAvailabilitySearch,
			Source:            "deterministic_broad_state",
			TemplateName:      TemplatePublicSCTable,
			Action:            "template",
			AvailabilityInput: &input,
		}, true
	case "MA":
		return IntentDecision{
			Intent:       IntentAvailabilitySearch,
			Source:       "deterministic_broad_state",
			TemplateName: TemplateAskSCOriginForMA,
			Action:       "template",
			AvailabilityInput: &AvailabilitySearchInput{
				PackageName: packageToMaranhao,
				Qty:         1,
				Limit:       8,
			},
		}, true
	default:
		return IntentDecision{}, false
	}
}

func parseSCDestinationFollowUpAfterPublicTable(history []Message, currentTurn string) (AvailabilitySearchInput, bool) {
	if !lastAssistantSentPublicSCTable(history) {
		return AvailabilitySearchInput{}, false
	}
	destination, ok := findSingleSupportedCityInText(currentTurn, scPackageDestinations)
	if !ok {
		return AvailabilitySearchInput{}, false
	}
	input := AvailabilitySearchInput{
		Destination: destination,
		PackageName: packageToSantaCatarina,
		Qty:         1,
		Limit:       8,
	}
	return enrichAvailabilitySearchInput(input), true
}

func lastAssistantSentPublicSCTable(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		body := strings.Join(strings.Fields(foldChatText(messageTurnText(history[i]))), " ")
		if body == "" {
			continue
		}

		return strings.Contains(body, "santa catarina") &&
			strings.Contains(body, "fraiburgo") &&
			strings.Contains(body, "ituporanga") &&
			(strings.Contains(body, "precos") || strings.Contains(body, "valores") || strings.Contains(body, "tabela"))
	}
	return false
}

func hasPreviousAvailabilityList(history []Message) bool {
	latest := findLatestAvailabilityContext(history)
	return latest != nil && len(latest.Results) > 0
}

func firstAvailableOptionIndex(history []Message) int {
	latest := findLatestAvailabilityContext(history)
	if latest == nil || len(latest.Results) == 0 {
		return 0
	}
	return 1
}

func looksLikeContextualAvailabilitySelection(folded string) bool {
	switch folded {
	case "essa", "essa opcao", "esta", "esta opcao", "esse", "esse horario", "essa passagem":
		return true
	default:
		return false
	}
}

func looksLikeHumanSupportIntent(folded string) bool {
	return strings.Contains(folded, "atendente") ||
		strings.Contains(folded, "humano") ||
		strings.Contains(folded, "suporte")
}

func parseSCOriginFollowUpAfterMaranhaoQuery(history []Message, currentTurn string) (AvailabilitySearchInput, bool) {
	if !lastAssistantAskedSCOriginForMaranhao(history) {
		return AvailabilitySearchInput{}, false
	}

	origin, ok := findSingleSupportedCityInText(currentTurn, scPackageDestinations)
	if !ok {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:      origin,
		PackageName: packageToMaranhao,
		Qty:         1,
		Limit:       8,
	}
	return enrichAvailabilitySearchInput(input), true
}

func lastAssistantAskedSCOriginForMaranhao(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}

		body := strings.Join(strings.Fields(foldChatText(messageTurnText(message))), " ")
		if body == "" {
			continue
		}

		return strings.Contains(body, "de qual cidade de santa catarina") &&
			strings.Contains(body, "maranhao")
	}
	return false
}

func parseMADestinationFollowUpAfterSCOrigin(history []Message, currentTurn string) (AvailabilitySearchInput, bool) {
	pending, ok := findLatestPendingMaranhaoAvailabilityInput(history)
	if !ok || strings.TrimSpace(pending.Origin) == "" {
		return AvailabilitySearchInput{}, false
	}

	destination, ok := findSingleSupportedCityInText(currentTurn, maPackageDestinations)
	if !ok {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:       strings.TrimSpace(pending.Origin),
		OriginStopID: strings.TrimSpace(pending.OriginStopID),
		Destination:  destination,
		PackageName:  packageToMaranhao,
		Qty:          1,
		Limit:        8,
	}
	return enrichAvailabilitySearchInput(input), true
}

func findLatestPendingMaranhaoAvailabilityInput(history []Message) (AvailabilitySearchInput, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}

		payloads := []map[string]interface{}{
			asMap(message.Payload),
			asMap(message.NormalizedPayload),
			asMap(asMap(message.Payload)["request_payload"]),
			asMap(asMap(message.Payload)["response_payload"]),
			asMap(asMap(message.NormalizedPayload)["request_payload"]),
			asMap(asMap(message.NormalizedPayload)["response_payload"]),
		}

		for _, payload := range payloads {
			input, ok := readPendingAvailabilityInput(payload)
			if ok && strings.TrimSpace(input.Origin) != "" && input.PackageName == packageToMaranhao {
				if input.Qty <= 0 {
					input.Qty = 1
				}
				if input.Limit <= 0 {
					input.Limit = 8
				}
				input = enrichAvailabilitySearchInput(input)
				return input, true
			}
		}
	}
	return AvailabilitySearchInput{}, false
}

func readPendingAvailabilityInput(payload map[string]interface{}) (AvailabilitySearchInput, bool) {
	if len(payload) == 0 {
		return AvailabilitySearchInput{}, false
	}

	raw := asMap(payload["pending_availability_input"])
	if len(raw) == 0 {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:            strings.TrimSpace(asString(raw["origin"])),
		Destination:       strings.TrimSpace(asString(raw["destination"])),
		OriginStopID:      strings.TrimSpace(asString(raw["origin_stop_id"])),
		DestinationStopID: strings.TrimSpace(asString(raw["destination_stop_id"])),
		RouteID:           strings.TrimSpace(asString(raw["route_id"])),
		PackageName:       strings.TrimSpace(asString(raw["package_name"])),
		Qty:               firstPositiveInt(readInt(raw["qty"]), readInt(raw["qtd"])),
		Limit:             readInt(raw["limit"]),
	}
	return enrichAvailabilitySearchInput(input), true
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func findSingleSupportedCityInText(text string, candidates map[string]string) (string, bool) {
	city, _, ok := findSingleSupportedCityMention(text, candidates)
	if !ok {
		return "", false
	}
	return city, true
}
