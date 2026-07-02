package chat

import (
	"strings"
	"time"
)

type InterpretationValidationInput struct {
	Proposal     StructuredInterpretation
	CurrentTurn  string
	History      []Message
	State        CanonicalConversationState
	ActivePrompt ActivePromptContext
	ObservedAt   time.Time
}

type InterpretationValidationResult struct {
	Accepted         bool
	RejectReason     string
	FallbackTemplate ResponseTemplateName
}

func ValidateStructuredInterpretation(input InterpretationValidationInput) InterpretationValidationResult {
	body := NormalizeIncomingCustomerText(input.CurrentTurn)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	activePrompt := input.ActivePrompt
	if activePrompt.Kind == "" {
		activePrompt = InferActivePromptContext(input.History, input.State)
	}

	if body == "" {
		return rejectInterpretation("empty_current_turn", "")
	}
	if structuredInterpretationClaimsSideEffect(input.Proposal) {
		return rejectInterpretation("unsafe_side_effect_claim", "")
	}
	if input.Proposal.Intent == StructuredIntentUnknown {
		return rejectInterpretation("unknown_intent", "")
	}
	if _, unsupported := inferUnsupportedPackageQuery(body); unsupported &&
		input.Proposal.Intent == StructuredIntentAvailabilitySearch {
		return rejectInterpretation("unsupported_destination", TemplateUnsupportedPackage)
	}
	if activePrompt.Kind == ActivePromptUnknown && structuredIntentRequiresActivePrompt(input.Proposal.Intent) {
		return rejectInterpretation("active_prompt_required", structuredIntentFallbackTemplate(input.Proposal.Intent))
	}

	switch activePrompt.Kind {
	case ActivePromptPaymentPreference:
		return validatePaymentPreferenceInterpretation(input.Proposal, body, folded)
	case ActivePromptAvailabilityOptionChoice:
		return validateAvailabilityOptionInterpretation(input.Proposal, body, folded, input.History, activePrompt)
	case ActivePromptAvailabilityDateChoice:
		return validateAvailabilityDateInterpretation(input.Proposal, body, input.History, input.ObservedAt)
	case ActivePromptPassengerCount:
		return validatePassengerCountInterpretation(input.Proposal, body, TemplateContextFallbackPassengerCount)
	case ActivePromptLapChildQuestion:
		return validateChildUnder5Interpretation(input.Proposal, body)
	case ActivePromptPassengerDocuments:
		return validatePassengerDocumentsInterpretation(input.Proposal, body)
	case ActivePromptDocumentConfirmation:
		return validateDocumentConfirmationInterpretation(input.Proposal, body, folded)
	case ActivePromptPayerCPF:
		return validatePayerCPFInterpretation(input.Proposal, body, input.State)
	case ActivePromptLapChildAssignment:
		return validateLapChildAssignmentInterpretation(input.Proposal, body, activePrompt)
	case ActivePromptReservationRoute:
		return validateReservationRouteInterpretation(input.Proposal, body, input.History, input.ObservedAt)
	default:
		return validateGeneralStructuredInterpretation(input.Proposal, body, input.State)
	}
}

func validatePaymentPreferenceInterpretation(proposal StructuredInterpretation, body string, folded string) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentPaymentPreference {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackPaymentPreference)
	}
	if looksLikePixOnlyPaymentReply(folded) {
		return rejectInterpretation("pix_is_method_not_payment_preference", TemplateContextFallbackPaymentPreference)
	}
	preference := detectStructuredPaymentPreference(body)
	if !structuredPaymentPreferenceAllowed(preference) {
		return rejectInterpretation("payment_preference_required", TemplateContextFallbackPaymentPreference)
	}
	if proposal.Payment.PaymentPreference != preference {
		return rejectInterpretation("payment_preference_mismatch", TemplateContextFallbackPaymentPreference)
	}
	return acceptInterpretation()
}

func validateAvailabilityOptionInterpretation(proposal StructuredInterpretation, body string, folded string, history []Message, activePrompt ActivePromptContext) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentSelectAvailabilityOption {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackAvailabilityOption)
	}
	promptContext := currentAvailabilitySelectionPromptContext(history)
	optionCount := promptContext.OptionCount
	if optionCount <= 0 {
		return rejectInterpretation("availability_options_unavailable", TemplateContextFallbackAvailabilityOption)
	}
	if !promptContext.HasCurrentFacts {
		return rejectInterpretation("availability_selection_missing_current_facts", TemplateContextFallbackAvailabilityOption)
	}
	textIndex := validationSelectedOptionIndex(body)
	if textIndex <= 0 {
		if optionCount == 1 && (looksLikeContextualAvailabilitySelection(folded) || looksLikeBookingCreateConfirmation(body)) {
			textIndex = 1
		}
	}
	if textIndex <= 0 {
		if looksLikeContextualAvailabilitySelection(folded) || looksLikeAmbiguousAvailabilityOptionReply(body, folded) {
			return rejectInterpretation("ambiguous_availability_option_reply", TemplateContextFallbackAvailabilityOption)
		}
		return rejectInterpretation("selected_option_index_required", TemplateContextFallbackAvailabilityOption)
	}
	if !proposal.Booking.SelectedOptionIndexKnown || proposal.Booking.SelectedOptionIndex <= 0 {
		return rejectInterpretation("selected_option_index_required", TemplateContextFallbackAvailabilityOption)
	}
	if proposal.Booking.SelectedOptionIndex != textIndex {
		return rejectInterpretation("selected_option_index_mismatch", TemplateContextFallbackAvailabilityOption)
	}
	if proposal.Booking.SelectedOptionIndex > optionCount {
		return rejectInterpretation("availability_option_index_out_of_range", TemplateContextFallbackAvailabilityOption)
	}
	return acceptInterpretation()
}

func validateAvailabilityDateInterpretation(proposal StructuredInterpretation, body string, history []Message, observedAt time.Time) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentAvailabilitySearch {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackAvailabilityDate)
	}
	if _, ok := parseAvailabilityDateSelectionInput(history, body, observedAt); ok {
		return acceptInterpretation()
	}
	return rejectInterpretation("invalid_availability_date", TemplateContextFallbackAvailabilityDate)
}

func validatePassengerCountInterpretation(proposal StructuredInterpretation, body string, fallback ResponseTemplateName) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentPassengerCountReply {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", fallback)
	}
	slots := parsePassengerClarificationSlots(body)
	if !slots.PassengerCountKnown && !slots.ChildUnder5CountKnown {
		return rejectInterpretation("passenger_count_required", fallback)
	}
	if slots.PassengerCountKnown &&
		(!proposal.Booking.PassengerCountKnown || proposal.Booking.PassengerCount != slots.PassengerCount) {
		return rejectInterpretation("passenger_count_mismatch", fallback)
	}
	if slots.ChildUnder5CountKnown &&
		(!proposal.Booking.ChildUnder5CountKnown || proposal.Booking.ChildUnder5Count != slots.ChildUnder5Count) {
		return rejectInterpretation("child_under_5_count_mismatch", fallback)
	}
	return acceptInterpretation()
}

func validateChildUnder5Interpretation(proposal StructuredInterpretation, body string) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentPassengerCountReply {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackChildUnder5)
	}
	slots := parsePassengerClarificationSlots(body)
	if !slots.ChildUnder5CountKnown {
		return rejectInterpretation("child_under_5_reply_required", TemplateContextFallbackChildUnder5)
	}
	if !proposal.Booking.ChildUnder5CountKnown || proposal.Booking.ChildUnder5Count != slots.ChildUnder5Count {
		return rejectInterpretation("child_under_5_count_mismatch", TemplateContextFallbackChildUnder5)
	}
	return acceptInterpretation()
}

func validatePassengerDocumentsInterpretation(proposal StructuredInterpretation, body string) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentPassengerDocumentsProvided {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackPassengerDocuments)
	}
	if !proposal.PassengerDocument.DocumentLikeText {
		return rejectInterpretation("passenger_document_slot_required", TemplateContextFallbackPassengerDocuments)
	}
	if looksLikeInvalidPassengerCPF(body) {
		return rejectInterpretation("invalid_passenger_document", TemplateContextFallbackPassengerDocuments)
	}
	if !looksLikePassengerDocumentText(body, Session{}) {
		return rejectInterpretation("passenger_documents_required", TemplateContextFallbackPassengerDocuments)
	}
	return acceptInterpretation()
}

func validateDocumentConfirmationInterpretation(proposal StructuredInterpretation, body string, folded string) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentDocumentConfirmation {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackDocumentConfirmation)
	}
	if strings.Contains(body, "?") && (folded == "ok" || folded == "sim" || folded == "certo") {
		return rejectInterpretation("ambiguous_document_confirmation", TemplateContextFallbackDocumentConfirmation)
	}
	if !looksLikeDocumentConfirmation(body) {
		return rejectInterpretation("document_confirmation_required", TemplateContextFallbackDocumentConfirmation)
	}
	return acceptInterpretation()
}

func validatePayerCPFInterpretation(proposal StructuredInterpretation, body string, state CanonicalConversationState) InterpretationValidationResult {
	if state.Phase != "" && !isPaymentDocumentReplyPhase(state.Phase) {
		return rejectInterpretation("payer_cpf_not_allowed_in_phase", TemplateContextFallbackPayerCPF)
	}
	if proposal.Intent != StructuredIntentPassengerDocumentsProvided {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackPayerCPF)
	}
	if !proposal.PassengerDocument.DocumentLikeText {
		return rejectInterpretation("payer_cpf_slot_required", TemplateContextFallbackPayerCPF)
	}
	if !looksLikeBareCPF(body) {
		return rejectInterpretation("payer_cpf_required", TemplateContextFallbackPayerCPF)
	}
	return acceptInterpretation()
}

func validateLapChildAssignmentInterpretation(proposal StructuredInterpretation, body string, activePrompt ActivePromptContext) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentLapChildAssignmentAnswer {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", TemplateContextFallbackLapChildAssignment)
	}
	textIndex := activePromptLapChildAssignmentAnswerIndex(body)
	if textIndex <= 0 {
		return rejectInterpretation("lap_child_assignment_index_required", TemplateContextFallbackLapChildAssignment)
	}
	if len(proposal.Booking.LapChildPassengerIndexes) == 0 {
		return rejectInterpretation("lap_child_assignment_index_required", TemplateContextFallbackLapChildAssignment)
	}
	for _, index := range proposal.Booking.LapChildPassengerIndexes {
		if index != textIndex {
			return rejectInterpretation("lap_child_assignment_index_mismatch", TemplateContextFallbackLapChildAssignment)
		}
	}
	if count := activePromptLapChildAssignmentOptionCount(activePrompt); count > 0 && textIndex > count {
		return rejectInterpretation("lap_child_assignment_index_out_of_range", TemplateContextFallbackLapChildAssignment)
	}
	return acceptInterpretation()
}

func validateReservationRouteInterpretation(proposal StructuredInterpretation, body string, history []Message, observedAt time.Time) InterpretationValidationResult {
	if proposal.Intent != StructuredIntentAvailabilitySearch {
		return rejectInterpretation("intent_not_allowed_by_active_prompt", "")
	}
	if activePromptRouteAvailabilitySearchResolved(history, body, observedAt) {
		return acceptInterpretation()
	}
	return rejectInterpretation("availability_route_required", "")
}

func validateGeneralStructuredInterpretation(proposal StructuredInterpretation, body string, state CanonicalConversationState) InterpretationValidationResult {
	switch proposal.Intent {
	case StructuredIntentAvailabilitySearch:
		if looksLikeReservationHowToProceedIntent(body) {
			if proposal.TurnMeaning == TurnMeaningNewRequest {
				return acceptInterpretation()
			}
			return rejectInterpretation("reservation_help_turn_meaning_mismatch", TemplateAskReservationRouteSC)
		}
		if state.Phase != "" &&
			state.Phase != ConversationPhaseDiscovery &&
			state.Phase != ConversationPhaseRouteSelection &&
			state.Phase != ConversationPhaseTripSelection {
			return rejectInterpretation("availability_search_not_allowed_in_phase", "")
		}
		return acceptInterpretation()
	default:
		return acceptInterpretation()
	}
}

func structuredInterpretationClaimsSideEffect(proposal StructuredInterpretation) bool {
	return proposal.Safety.ExecutesTool ||
		proposal.Safety.MutatesState ||
		proposal.Safety.SendsMessage ||
		proposal.Safety.CallsExternalProvider
}

func structuredPaymentPreferenceAllowed(preference string) bool {
	return preference == "sinal" || preference == "integral"
}

func structuredIntentRequiresActivePrompt(intent StructuredIntent) bool {
	switch intent {
	case StructuredIntentSelectAvailabilityOption,
		StructuredIntentPassengerCountReply,
		StructuredIntentLapChildAssignmentAnswer,
		StructuredIntentPassengerDocumentsProvided,
		StructuredIntentDocumentConfirmation,
		StructuredIntentPaymentPreference:
		return true
	default:
		return false
	}
}

func structuredIntentFallbackTemplate(intent StructuredIntent) ResponseTemplateName {
	switch intent {
	case StructuredIntentSelectAvailabilityOption:
		return TemplateContextFallbackAvailabilityOption
	case StructuredIntentPassengerCountReply:
		return TemplateContextFallbackPassengerCount
	case StructuredIntentLapChildAssignmentAnswer:
		return TemplateContextFallbackLapChildAssignment
	case StructuredIntentPassengerDocumentsProvided:
		return TemplateContextFallbackPassengerDocuments
	case StructuredIntentDocumentConfirmation:
		return TemplateContextFallbackDocumentConfirmation
	case StructuredIntentPaymentPreference:
		return TemplateContextFallbackPaymentPreference
	default:
		return ""
	}
}

func validationSelectedOptionIndex(text string) int {
	if index := extractSelectedOptionIndex(text); index > 0 {
		return index
	}
	return activePromptLapChildAssignmentAnswerIndex(text)
}

func acceptInterpretation() InterpretationValidationResult {
	return InterpretationValidationResult{Accepted: true}
}

func rejectInterpretation(reason string, fallback ResponseTemplateName) InterpretationValidationResult {
	return InterpretationValidationResult{
		Accepted:         false,
		RejectReason:     strings.TrimSpace(reason),
		FallbackTemplate: fallback,
	}
}
