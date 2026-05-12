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
	IntentHumanSupport               Intent = "HUMAN_SUPPORT"
)

type IntentDecision struct {
	Intent              Intent
	Source              string
	SelectedOptionIndex int
	AvailabilityInput   *AvailabilitySearchInput
	TemplateName        ResponseTemplateName
	Action              string
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
	if looksLikeCreateBookingIntent(body) || looksLikeBookingCreateConfirmation(body) {
		return IntentDecision{Intent: IntentBookingCreateConfirmation, Source: "deterministic", Action: "legacy_tool"}
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
	if passengerCount, _, ok := parsePassengerCountReply(body); ok && passengerCount > 0 {
		return IntentDecision{Intent: IntentPassengerCountReply, Source: "deterministic"}
	}
	if input, ok := parseAvailabilitySearchInput(history, body, observedAt); ok {
		return IntentDecision{Intent: IntentAvailabilitySearch, Source: "deterministic", AvailabilityInput: &input, Action: "tool"}
	}
	return IntentDecision{Intent: IntentUnknown, Source: "deterministic"}
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
