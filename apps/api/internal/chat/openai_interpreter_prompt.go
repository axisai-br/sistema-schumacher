package chat

import (
	"encoding/json"
	"strings"
)

const openAIStructuredInterpreterRecentTurnsLimit = 6

type OpenAIStructuredInterpreterCompactInput struct {
	CurrentTurn         string                                     `json:"current_turn"`
	State               OpenAIStructuredInterpreterStateSnapshot   `json:"state"`
	RecentTurns         []string                                   `json:"recent_turns,omitempty"`
	LocalInterpretation OpenAIStructuredLocalInterpretationSummary `json:"local_interpretation"`
}

type OpenAIStructuredInterpreterStateSnapshot struct {
	Phase                                  string   `json:"phase"`
	AllowedNextActions                     []string `json:"allowed_next_actions"`
	Origin                                 string   `json:"origin,omitempty"`
	Destination                            string   `json:"destination,omitempty"`
	PackageName                            string   `json:"package_name,omitempty"`
	TripDate                               string   `json:"trip_date,omitempty"`
	DepartureTime                          string   `json:"departure_time,omitempty"`
	SelectedOptionIndex                    int      `json:"selected_option_index,omitempty"`
	PassengerCount                         int      `json:"passenger_count,omitempty"`
	ChildUnder5Count                       int      `json:"child_under_5_count,omitempty"`
	DocumentsCollected                     bool     `json:"documents_collected"`
	BookingStatus                          string   `json:"booking_status,omitempty"`
	PaymentStatus                          string   `json:"payment_status,omitempty"`
	PaymentPreference                      string   `json:"payment_preference,omitempty"`
	HasAvailabilityFacts                   bool     `json:"has_availability_facts"`
	HasBookingFacts                        bool     `json:"has_booking_facts"`
	HasPaymentStatusFacts                  bool     `json:"has_payment_status_facts"`
	HasPaymentCreateFacts                  bool     `json:"has_payment_create_facts"`
	AvailableOptionCount                   int      `json:"available_option_count,omitempty"`
	LastAssistantAskedDocumentConfirmation bool     `json:"last_assistant_asked_document_confirmation"`
	LastAssistantAskedLapChildAssignment   bool     `json:"last_assistant_asked_lap_child_assignment"`
	HasPaymentPreferenceContext            bool     `json:"has_payment_preference_context"`
	HasPassengerCountContext               bool     `json:"has_passenger_count_context"`
	HasPassengerDocumentContext            bool     `json:"has_passenger_document_context"`
}

type OpenAIStructuredLocalInterpretationSummary struct {
	Intent                   string   `json:"intent"`
	TurnMeaning              string   `json:"turn_meaning"`
	Confidence               float64  `json:"confidence"`
	SelectedOptionIndex      int      `json:"selected_option_index,omitempty"`
	SelectedOptionIndexKnown bool     `json:"selected_option_index_known"`
	PassengerCount           int      `json:"passenger_count,omitempty"`
	PassengerCountKnown      bool     `json:"passenger_count_known"`
	ChildUnder5Count         int      `json:"child_under_5_count,omitempty"`
	ChildUnder5CountKnown    bool     `json:"child_under_5_count_known"`
	LapChildPassengerIndexes []int    `json:"lap_child_passenger_indexes,omitempty"`
	DocumentLikeText         bool     `json:"document_like_text"`
	PaymentPreference        string   `json:"payment_preference,omitempty"`
	Reasons                  []string `json:"reasons,omitempty"`
}

func buildOpenAIStructuredInterpreterSystemPrompt() string {
	return strings.Join([]string{
		"Classifique a mensagem atual do cliente em uma interpretacao estruturada.",
		"Retorne somente JSON valido no schema fornecido.",
		"Nao execute ferramentas, nao acione templates, nao envie mensagens e nao altere estado.",
		"Use UNKNOWN quando o texto ou o contexto nao forem suficientes.",
		"DOCUMENT_CONFIRMATION exige que a ultima pergunta do assistente tenha confirmado dados documentais.",
		"SELECT_AVAILABILITY_OPTION exige opcao dentro da lista disponivel.",
		"PAYMENT_PREFERENCE aceita somente sinal ou integral.",
	}, "\n")
}

func buildOpenAIStructuredInterpreterCompactInput(input StructuredInterpreterInput, local StructuredInterpretation) string {
	compact := OpenAIStructuredInterpreterCompactInput{
		CurrentTurn:         redactOpenAIStructuredInterpreterSensitiveText(input.CurrentTurn),
		State:               snapshotOpenAIStructuredInterpreterState(input),
		RecentTurns:         recentOpenAIStructuredInterpreterTurns(input.History, openAIStructuredInterpreterRecentTurnsLimit),
		LocalInterpretation: summarizeOpenAIStructuredLocalInterpretation(local),
	}
	data, err := json.Marshal(compact)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func snapshotOpenAIStructuredInterpreterState(input StructuredInterpreterInput) OpenAIStructuredInterpreterStateSnapshot {
	state := input.State
	facts := state.LastToolFacts
	optionCount, hasOptions := structuredAvailabilitySelectionOptionCount(input.History)
	if !hasOptions {
		optionCount = 0
	}
	return OpenAIStructuredInterpreterStateSnapshot{
		Phase:                                  string(state.Phase),
		AllowedNextActions:                     append([]string(nil), state.AllowedNextActions...),
		Origin:                                 strings.TrimSpace(state.Route.Origin),
		Destination:                            strings.TrimSpace(state.Route.Destination),
		PackageName:                            strings.TrimSpace(state.Route.PackageName),
		TripDate:                               strings.TrimSpace(state.Route.TripDate),
		DepartureTime:                          strings.TrimSpace(state.Route.DepartureTime),
		SelectedOptionIndex:                    state.Route.SelectedOptionIndex,
		PassengerCount:                         state.Passengers.ExpectedCount,
		ChildUnder5Count:                       state.Passengers.ChildUnder5Count,
		DocumentsCollected:                     state.Passengers.DocumentsCollected,
		BookingStatus:                          strings.TrimSpace(state.Booking.Status),
		PaymentStatus:                          strings.TrimSpace(state.Payment.Status),
		PaymentPreference:                      strings.TrimSpace(state.Payment.Preference),
		HasAvailabilityFacts:                   len(asMap(facts[toolNameAvailabilitySearch])) > 0,
		HasBookingFacts:                        len(asMap(facts[toolNameBookingCreate])) > 0 || strings.TrimSpace(state.Booking.Status) != "",
		HasPaymentStatusFacts:                  len(asMap(facts[toolNamePaymentStatus])) > 0,
		HasPaymentCreateFacts:                  len(asMap(facts[toolNamePaymentCreate])) > 0,
		AvailableOptionCount:                   optionCount,
		LastAssistantAskedDocumentConfirmation: lastAssistantAskedDocumentConfirmationPrompt(input.History),
		LastAssistantAskedLapChildAssignment:   lastAssistantAskedLapChildAssignment(input.History),
		HasPaymentPreferenceContext:            hasStructuredPaymentPreferenceContext(input.History, state),
		HasPassengerCountContext:               hasStructuredPassengerCountContext(input.History, state),
		HasPassengerDocumentContext:            hasStructuredPassengerDocumentContext(input.History, state),
	}
}

func recentOpenAIStructuredInterpreterTurns(history []Message, limit int) []string {
	if limit <= 0 || len(history) == 0 {
		return nil
	}
	start := len(history) - limit
	if start < 0 {
		start = 0
	}
	out := make([]string, 0, len(history)-start)
	for _, message := range history[start:] {
		body := redactOpenAIStructuredInterpreterSensitiveText(messageTurnText(message))
		if body == "" {
			continue
		}
		direction := strings.ToUpper(strings.TrimSpace(message.Direction))
		if direction == "" {
			direction = "UNKNOWN"
		}
		out = append(out, direction+": "+body)
	}
	return out
}

func summarizeOpenAIStructuredLocalInterpretation(local StructuredInterpretation) OpenAIStructuredLocalInterpretationSummary {
	return OpenAIStructuredLocalInterpretationSummary{
		Intent:                   string(local.Intent),
		TurnMeaning:              string(local.TurnMeaning),
		Confidence:               local.Confidence,
		SelectedOptionIndex:      local.Booking.SelectedOptionIndex,
		SelectedOptionIndexKnown: local.Booking.SelectedOptionIndexKnown,
		PassengerCount:           local.Booking.PassengerCount,
		PassengerCountKnown:      local.Booking.PassengerCountKnown,
		ChildUnder5Count:         local.Booking.ChildUnder5Count,
		ChildUnder5CountKnown:    local.Booking.ChildUnder5CountKnown,
		LapChildPassengerIndexes: append([]int(nil), local.Booking.LapChildPassengerIndexes...),
		DocumentLikeText:         local.PassengerDocument.DocumentLikeText,
		PaymentPreference:        strings.TrimSpace(local.Payment.PaymentPreference),
		Reasons:                  append([]string(nil), local.Reasons...),
	}
}

func redactOpenAIStructuredInterpreterSensitiveText(text string) string {
	body := strings.TrimSpace(text)
	if body == "" {
		return ""
	}
	if openAIStructuredInterpreterContainsSensitiveDocument(body) {
		return "[DOCUMENTO_REDACTED]"
	}
	return body
}

func openAIStructuredInterpreterContainsSensitiveDocument(text string) bool {
	body := strings.TrimSpace(text)
	if body == "" {
		return false
	}
	if extractValidCPF(body) != "" || looksLikeInvalidPassengerCPF(body) {
		return true
	}
	if extractPassengerRG(body) != "" || len(typedPassengerDocumentCandidates(body)) > 0 {
		return true
	}
	if looksLikePassengerDocumentText(body, Session{}) {
		return true
	}
	folded := normalizeStructuredFolded(body)
	if looksLikePassengerDocumentRequest(folded) {
		return true
	}
	for _, token := range strings.Fields(folded) {
		switch token {
		case "cpf", "rg", "cnh", "certidao", "matricula", "documento", "documentos":
			return true
		}
	}
	return false
}
