package chat

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type OpenAIStructuredInterpretationValidationResult struct {
	Candidate      OpenAIStructuredInterpretationJSON
	Interpretation StructuredInterpretation
	Valid          bool
	Reasons        []string
}

func validateOpenAIStructuredInterpretation(input StructuredInterpreterInput, candidate OpenAIStructuredInterpretationJSON) OpenAIStructuredInterpretationValidationResult {
	candidate = normalizeOpenAIStructuredInterpretationMetadata(candidate)
	interpretation := openAIStructuredInterpretationToStructured(candidate)
	reasons := validateOpenAIStructuredInterpretationReasons(input, candidate)
	if len(reasons) > 0 {
		return OpenAIStructuredInterpretationValidationResult{
			Candidate:      candidate,
			Interpretation: interpretation,
			Valid:          false,
			Reasons:        reasons,
		}
	}
	return OpenAIStructuredInterpretationValidationResult{
		Candidate:      candidate,
		Interpretation: interpretation,
		Valid:          true,
	}
}

func validateOpenAIStructuredInterpretationPayload(input StructuredInterpreterInput, payload []byte) OpenAIStructuredInterpretationValidationResult {
	if reasons := validateOpenAIStructuredInterpretationPayloadRequiredFields(payload); len(reasons) > 0 {
		return OpenAIStructuredInterpretationValidationResult{
			Valid:   false,
			Reasons: reasons,
		}
	}

	var candidate OpenAIStructuredInterpretationJSON
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return invalidOpenAIStructuredInterpretationPayloadResult()
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return invalidOpenAIStructuredInterpretationPayloadResult()
	}
	return validateOpenAIStructuredInterpretation(input, candidate)
}

func invalidOpenAIStructuredInterpretationPayloadResult(reasons ...string) OpenAIStructuredInterpretationValidationResult {
	allReasons := []string{"invalid_json_payload"}
	allReasons = append(allReasons, reasons...)
	return OpenAIStructuredInterpretationValidationResult{
		Valid:   false,
		Reasons: dedupeOpenAIStructuredReasons(allReasons),
	}
}

func validateOpenAIStructuredInterpretationPayloadRequiredFields(payload []byte) []string {
	var root map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&root); err != nil {
		return []string{"invalid_json_payload"}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return []string{"invalid_json_payload"}
	}
	if root == nil {
		return []string{"invalid_json_payload", "missing_required_fields"}
	}

	reasons := validateOpenAIRawRequiredFields(root, "root", []string{
		"intent",
		"turn_meaning",
		"confidence",
		"booking",
		"passenger_document",
		"payment",
		"safety",
		"reasons",
	})

	nestedRequirements := []struct {
		Field    string
		Required []string
	}{
		{
			Field: "booking",
			Required: []string{
				"selected_option_index",
				"selected_option_index_known",
				"passenger_count",
				"passenger_count_known",
				"child_under_5_count",
				"child_under_5_count_known",
				"lap_child_passenger_indexes",
			},
		},
		{Field: "passenger_document", Required: []string{"document_like_text"}},
		{Field: "payment", Required: []string{"payment_preference"}},
		{
			Field: "safety",
			Required: []string{
				"executes_tool",
				"mutates_state",
				"sends_message",
				"calls_external_provider",
			},
		},
	}
	for _, item := range nestedRequirements {
		nested, nestedReasons := openAIRawRequiredObject(root, item.Field)
		reasons = append(reasons, nestedReasons...)
		if len(nestedReasons) > 0 {
			continue
		}
		reasons = append(reasons, validateOpenAIRawRequiredFields(nested, item.Field, item.Required)...)
	}

	if len(reasons) == 0 {
		return nil
	}
	return append([]string{"invalid_json_payload"}, dedupeOpenAIStructuredReasons(reasons)...)
}

func validateOpenAIRawRequiredFields(raw map[string]json.RawMessage, path string, required []string) []string {
	reasons := []string{}
	for _, field := range required {
		value, ok := raw[field]
		if !ok {
			reasons = append(reasons, "missing_required_field:"+path+"."+field)
			continue
		}
		if openAIRawJSONIsNull(value) {
			reasons = append(reasons, "null_required_field:"+path+"."+field)
		}
	}
	return reasons
}

func openAIRawRequiredObject(raw map[string]json.RawMessage, field string) (map[string]json.RawMessage, []string) {
	value, ok := raw[field]
	if !ok || openAIRawJSONIsNull(value) {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return nil, []string{"invalid_required_object:" + field}
	}
	return object, nil
}

func openAIRawJSONIsNull(value json.RawMessage) bool {
	return string(bytes.TrimSpace(value)) == "null"
}

func normalizeOpenAIStructuredInterpretationMetadata(candidate OpenAIStructuredInterpretationJSON) OpenAIStructuredInterpretationJSON {
	candidate.Reasons = normalizeOpenAIStructuredReasons(candidate.Reasons)
	return candidate
}

func normalizeOpenAIStructuredReasons(reasons []string) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		out = append(out, reason)
	}
	if out == nil {
		return []string{}
	}
	return out
}

func validateOpenAIStructuredInterpretationReasons(input StructuredInterpreterInput, candidate OpenAIStructuredInterpretationJSON) []string {
	reasons := []string{}
	if !openAIStructuredIntentAllowed(candidate.Intent) {
		reasons = append(reasons, "invalid_intent")
	}
	if !openAITurnMeaningAllowed(candidate.TurnMeaning) {
		reasons = append(reasons, "invalid_turn_meaning")
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		reasons = append(reasons, "confidence_out_of_range")
	}
	if candidate.Safety.ExecutesTool || candidate.Safety.MutatesState || candidate.Safety.SendsMessage || candidate.Safety.CallsExternalProvider {
		reasons = append(reasons, "unsafe_side_effect_claim")
	}
	if candidate.Payment.PaymentPreference != "" && !openAIPaymentPreferenceAllowed(candidate.Payment.PaymentPreference) {
		reasons = append(reasons, "invalid_payment_preference")
	}

	switch candidate.Intent {
	case StructuredIntentDocumentConfirmation:
		if !lastAssistantAskedDocumentConfirmationPrompt(input.History) {
			reasons = append(reasons, "document_confirmation_without_prompt")
		}
	case StructuredIntentSelectAvailabilityOption:
		reasons = append(reasons, validateOpenAIAvailabilitySelection(input, candidate)...)
	case StructuredIntentPaymentPreference:
		if !openAIPaymentPreferenceAllowed(candidate.Payment.PaymentPreference) {
			reasons = append(reasons, "payment_preference_required")
		}
		if !hasStructuredPaymentPreferenceContext(input.History, input.State) {
			reasons = append(reasons, "payment_preference_without_context")
		}
	case StructuredIntentPassengerCountReply:
		if !hasStructuredPassengerCountContext(input.History, input.State) {
			reasons = append(reasons, "passenger_count_without_context")
		}
	case StructuredIntentPassengerDocumentsProvided:
		if !hasStructuredPassengerDocumentContext(input.History, input.State) {
			reasons = append(reasons, "passenger_documents_without_context")
		}
	case StructuredIntentLapChildAssignmentAnswer:
		if !lastAssistantAskedLapChildAssignment(input.History) {
			reasons = append(reasons, "lap_child_assignment_without_context")
		}
	}
	return dedupeOpenAIStructuredReasons(reasons)
}

func validateOpenAIAvailabilitySelection(input StructuredInterpreterInput, candidate OpenAIStructuredInterpretationJSON) []string {
	reasons := []string{}
	if !candidate.Booking.SelectedOptionIndexKnown || candidate.Booking.SelectedOptionIndex <= 0 {
		return append(reasons, "selected_option_index_required")
	}
	optionCount, ok := structuredAvailabilitySelectionOptionCount(input.History)
	if !ok || optionCount <= 0 {
		return append(reasons, "availability_options_unavailable")
	}
	if candidate.Booking.SelectedOptionIndex > optionCount {
		reasons = append(reasons, "availability_option_out_of_range")
	}
	return reasons
}

func openAIStructuredIntentAllowed(intent StructuredIntent) bool {
	for _, allowed := range openAIStructuredIntentEnum() {
		if string(intent) == allowed {
			return true
		}
	}
	return false
}

func openAITurnMeaningAllowed(meaning TurnMeaning) bool {
	for _, allowed := range openAITurnMeaningEnum() {
		if string(meaning) == allowed {
			return true
		}
	}
	return false
}

func openAIPaymentPreferenceAllowed(preference string) bool {
	switch preference {
	case "sinal", "integral":
		return true
	default:
		return false
	}
}

func dedupeOpenAIStructuredReasons(reasons []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		out = append(out, reason)
	}
	return out
}

func openAIStructuredInterpretationToStructured(candidate OpenAIStructuredInterpretationJSON) StructuredInterpretation {
	return StructuredInterpretation{
		Intent:      candidate.Intent,
		TurnMeaning: candidate.TurnMeaning,
		Confidence:  candidate.Confidence,
		Source:      "openai_structured",
		Booking: StructuredBookingSlots{
			SelectedOptionIndex:      candidate.Booking.SelectedOptionIndex,
			SelectedOptionIndexKnown: candidate.Booking.SelectedOptionIndexKnown,
			PassengerCount:           candidate.Booking.PassengerCount,
			PassengerCountKnown:      candidate.Booking.PassengerCountKnown,
			ChildUnder5Count:         candidate.Booking.ChildUnder5Count,
			ChildUnder5CountKnown:    candidate.Booking.ChildUnder5CountKnown,
			LapChildPassengerIndexes: append([]int(nil), candidate.Booking.LapChildPassengerIndexes...),
		},
		PassengerDocument: StructuredPassengerDocumentSlots{
			DocumentLikeText: candidate.PassengerDocument.DocumentLikeText,
		},
		Payment: StructuredPaymentSlots{
			PaymentPreference: candidate.Payment.PaymentPreference,
		},
		Safety: StructuredInterpretationSafety{
			ExecutesTool:          candidate.Safety.ExecutesTool,
			MutatesState:          candidate.Safety.MutatesState,
			SendsMessage:          candidate.Safety.SendsMessage,
			CallsExternalProvider: candidate.Safety.CallsExternalProvider,
		},
		Reasons: append([]string(nil), candidate.Reasons...),
	}
}
