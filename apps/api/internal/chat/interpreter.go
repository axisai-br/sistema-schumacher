package chat

import (
	"strings"
	"time"
)

type StructuredIntent string

const (
	StructuredIntentUnknown                    StructuredIntent = "UNKNOWN"
	StructuredIntentGreeting                   StructuredIntent = "GREETING"
	StructuredIntentAvailabilitySearch         StructuredIntent = "AVAILABILITY_SEARCH"
	StructuredIntentSelectAvailabilityOption   StructuredIntent = "SELECT_AVAILABILITY_OPTION"
	StructuredIntentPassengerCountReply        StructuredIntent = "PASSENGER_COUNT_REPLY"
	StructuredIntentLapChildAssignmentAnswer   StructuredIntent = "LAP_CHILD_ASSIGNMENT_ANSWER"
	StructuredIntentPassengerDocumentsProvided StructuredIntent = "PASSENGER_DOCUMENTS_PROVIDED"
	StructuredIntentDocumentConfirmation       StructuredIntent = "DOCUMENT_CONFIRMATION"
	StructuredIntentPaymentPreference          StructuredIntent = "PAYMENT_PREFERENCE"
	StructuredIntentBookingCancelRequest       StructuredIntent = "BOOKING_CANCEL_REQUEST"
)

type TurnMeaning string

const (
	TurnMeaningUnknown          TurnMeaning = "UNKNOWN"
	TurnMeaningGreeting         TurnMeaning = "GREETING"
	TurnMeaningNewRequest       TurnMeaning = "NEW_REQUEST"
	TurnMeaningAnswerToQuestion TurnMeaning = "ANSWER_TO_QUESTION"
	TurnMeaningConfirmation     TurnMeaning = "CONFIRMATION"
)

type StructuredInterpreterInput struct {
	CurrentTurn string
	State       CanonicalConversationState
	History     []Message
	ObservedAt  time.Time
}

type StructuredInterpretation struct {
	Intent            StructuredIntent
	TurnMeaning       TurnMeaning
	Confidence        float64
	Source            string
	Booking           StructuredBookingSlots
	PassengerDocument StructuredPassengerDocumentSlots
	Payment           StructuredPaymentSlots
	Safety            StructuredInterpretationSafety
	Reasons           []string
}

type StructuredBookingSlots struct {
	SelectedOptionIndex      int
	SelectedOptionIndexKnown bool
	PassengerCount           int
	PassengerCountKnown      bool
	ChildUnder5Count         int
	ChildUnder5CountKnown    bool
	LapChildPassengerIndexes []int
}

type StructuredPassengerDocumentSlots struct {
	DocumentLikeText bool
}

type StructuredPaymentSlots struct {
	PaymentPreference string
}

type StructuredInterpretationSafety struct {
	ExecutesTool          bool
	MutatesState          bool
	SendsMessage          bool
	CallsExternalProvider bool
}

func InterpretStructuredTurn(input StructuredInterpreterInput) StructuredInterpretation {
	body := NormalizeIncomingCustomerText(input.CurrentTurn)
	folded := normalizeStructuredFolded(body)
	if body == "" {
		return unknownStructuredInterpretation("empty_current_turn", 0)
	}

	if index := extractSelectedOptionIndex(body); index > 0 && lastAssistantAskedLapChildAssignment(input.History) {
		out := newStructuredInterpretation(
			StructuredIntentLapChildAssignmentAnswer,
			TurnMeaningAnswerToQuestion,
			0.98,
			"deterministic_lap_child_assignment",
		)
		out.Booking.LapChildPassengerIndexes = []int{index}
		out.Reasons = []string{"latest_assistant_asked_lap_child_assignment"}
		return out
	}

	if looksLikeDocumentConfirmation(body) && lastAssistantAskedDocumentConfirmationPrompt(input.History) {
		out := newStructuredInterpretation(
			StructuredIntentDocumentConfirmation,
			TurnMeaningConfirmation,
			0.96,
			"deterministic_document_confirmation",
		)
		out.Reasons = []string{"latest_assistant_asked_document_confirmation"}
		return out
	}

	if looksLikeBookingCancelIntent(body) {
		out := newStructuredInterpretation(
			StructuredIntentBookingCancelRequest,
			TurnMeaningNewRequest,
			0.94,
			"deterministic_booking_cancel_request",
		)
		out.Reasons = []string{"cancel_keyword"}
		return out
	}

	if preference := detectStructuredPaymentPreference(body); preference != "" && hasStructuredPaymentPreferenceContext(input.History, input.State) {
		out := newStructuredInterpretation(
			StructuredIntentPaymentPreference,
			TurnMeaningAnswerToQuestion,
			0.94,
			"deterministic_payment_preference",
		)
		out.Payment.PaymentPreference = preference
		out.Reasons = []string{"payment_preference_context"}
		return out
	}

	if index := extractSelectedOptionIndex(body); index > 0 {
		if optionCount, ok := structuredAvailabilitySelectionOptionCount(input.History); ok && index <= optionCount {
			out := newStructuredInterpretation(
				StructuredIntentSelectAvailabilityOption,
				TurnMeaningAnswerToQuestion,
				0.96,
				"deterministic_availability_selection",
			)
			out.Booking.SelectedOptionIndex = index
			out.Booking.SelectedOptionIndexKnown = true
			out.Reasons = []string{"availability_selection_context"}
			return out
		}
	}

	if slots := parsePassengerClarificationSlots(body); (slots.PassengerCountKnown || slots.ChildUnder5CountKnown) && hasStructuredPassengerCountContext(input.History, input.State) {
		out := newStructuredInterpretation(
			StructuredIntentPassengerCountReply,
			TurnMeaningAnswerToQuestion,
			0.93,
			"deterministic_passenger_count_reply",
		)
		out.Booking.PassengerCount = slots.PassengerCount
		out.Booking.PassengerCountKnown = slots.PassengerCountKnown
		out.Booking.ChildUnder5Count = slots.ChildUnder5Count
		out.Booking.ChildUnder5CountKnown = slots.ChildUnder5CountKnown
		out.Reasons = []string{"passenger_count_context"}
		return out
	}

	if looksLikePassengerDocumentText(body, Session{}) && hasStructuredPassengerDocumentContext(input.History, input.State) {
		out := newStructuredInterpretation(
			StructuredIntentPassengerDocumentsProvided,
			TurnMeaningAnswerToQuestion,
			0.92,
			"deterministic_passenger_documents",
		)
		out.PassengerDocument.DocumentLikeText = true
		out.Reasons = []string{"passenger_document_context"}
		return out
	}

	if looksLikeStructuredGreeting(folded) {
		out := newStructuredInterpretation(
			StructuredIntentGreeting,
			TurnMeaningGreeting,
			0.90,
			"deterministic_greeting",
		)
		out.Reasons = []string{"greeting_only"}
		return out
	}

	return unknownStructuredInterpretation("no_deterministic_match", 0)
}

func newStructuredInterpretation(intent StructuredIntent, meaning TurnMeaning, confidence float64, source string) StructuredInterpretation {
	return StructuredInterpretation{
		Intent:      intent,
		TurnMeaning: meaning,
		Confidence:  confidence,
		Source:      source,
	}
}

func unknownStructuredInterpretation(reason string, confidence float64) StructuredInterpretation {
	out := newStructuredInterpretation(StructuredIntentUnknown, TurnMeaningUnknown, confidence, "deterministic")
	if strings.TrimSpace(reason) != "" {
		out.Reasons = []string{reason}
	}
	return out
}

func normalizeStructuredFolded(text string) string {
	return strings.Join(strings.Fields(foldChatText(text)), " ")
}

func looksLikeStructuredGreeting(folded string) bool {
	switch strings.TrimSpace(folded) {
	case "oi", "ola", "olá", "bom dia", "boa tarde", "boa noite":
		return true
	default:
		return false
	}
}

func lastAssistantAskedDocumentConfirmationPrompt(history []Message) bool {
	return lastAssistantAskedDocumentConfirmation(history)
}

func detectStructuredPaymentPreference(body string) string {
	if requested := detectRequestedPaymentType(body); requested != "" {
		return requested
	}
	return ""
}

func hasStructuredPaymentPreferenceContext(history []Message, state CanonicalConversationState) bool {
	if state.Phase == ConversationPhaseBooked || state.Phase == ConversationPhasePaymentPending {
		return true
	}
	if body, ok := latestAssistantBody(history); ok {
		folded := normalizeStructuredFolded(body)
		return looksLikePaymentPreferencePrompt(folded)
	}
	return false
}

func structuredAvailabilitySelectionOptionCount(history []Message) (int, bool) {
	message, ok := latestAssistantMessage(history)
	if !ok {
		return 0, false
	}
	body := strings.TrimSpace(messageTurnText(message))
	folded := normalizeStructuredFolded(body)
	if count := availabilityOptionCountFromMessage(message); count > 0 {
		return count, true
	}
	if !looksLikeAvailabilitySelectionPrompt(folded) {
		return 0, false
	}
	latest := findLatestAvailabilityContext(history)
	if latest == nil || len(latest.Results) == 0 {
		return 0, false
	}
	return len(latest.Results), true
}

func availabilityOptionCountFromMessage(message Message) int {
	for _, toolContext := range messageToolContexts(message) {
		payload := asMap(toolContext[toolNameAvailabilitySearch])
		if len(payload) == 0 {
			continue
		}
		result := parseAvailabilityContextPayload(payload)
		if len(result.Results) > 0 {
			return len(result.Results)
		}
	}
	return 0
}

func looksLikeAvailabilitySelectionPrompt(folded string) bool {
	return (strings.Contains(folded, "opcao") ||
		strings.Contains(folded, "opcoes") ||
		strings.Contains(folded, "horario") ||
		strings.Contains(folded, "horarios")) &&
		(strings.Contains(folded, "escolh") ||
			strings.Contains(folded, "qual") ||
			strings.Contains(folded, "primeira") ||
			strings.Contains(folded, "segunda"))
}

func hasStructuredPassengerCountContext(history []Message, state CanonicalConversationState) bool {
	if state.Phase == ConversationPhasePassengerCollection {
		return true
	}
	if body, ok := latestAssistantBody(history); ok {
		return looksLikePassengerCountQuestion(body)
	}
	return false
}

func hasStructuredPassengerDocumentContext(history []Message, state CanonicalConversationState) bool {
	if state.Phase == ConversationPhasePassengerCollection || state.Phase == ConversationPhaseBookingPending {
		return true
	}
	if body, ok := latestAssistantBody(history); ok {
		return looksLikePassengerDocumentRequest(normalizeStructuredFolded(body))
	}
	return false
}

func latestAssistantBody(history []Message) (string, bool) {
	message, ok := latestAssistantMessage(history)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(messageTurnText(message)), true
}

func latestAssistantMessage(history []Message) (Message, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		body := strings.TrimSpace(messageTurnText(history[i]))
		if body == "" {
			continue
		}
		return history[i], true
	}
	return Message{}, false
}
