package chat

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Intent string

const (
	IntentUnknown                          Intent = "UNKNOWN"
	IntentAvailabilitySearch               Intent = "AVAILABILITY_SEARCH"
	IntentSelectAvailabilityOption         Intent = "SELECT_AVAILABILITY_OPTION"
	IntentPassengerCountReply              Intent = "PASSENGER_COUNT_REPLY"
	IntentPassengerDocumentsProvided       Intent = "PASSENGER_DOCUMENTS_PROVIDED"
	IntentLapChildAssignmentAnswer         Intent = "LAP_CHILD_ASSIGNMENT_ANSWER"
	IntentDocumentConfirmation             Intent = "DOCUMENT_CONFIRMATION"
	IntentBookingCreateConfirmation        Intent = "BOOKING_CREATE_CONFIRMATION"
	IntentPaymentPreference                Intent = "PAYMENT_PREFERENCE"
	IntentPaymentInfoQuestion              Intent = "PAYMENT_INFO_QUESTION"
	IntentPaymentMethodQuestion            Intent = "PAYMENT_METHOD_QUESTION"
	IntentPayingPassengerInfoQuestion      Intent = "PAYING_PASSENGER_INFO_QUESTION"
	IntentDocumentRequirementsInfoQuestion Intent = "DOCUMENT_REQUIREMENTS_INFO_QUESTION"
	IntentChildPolicyInfoQuestion          Intent = "CHILD_POLICY_INFO_QUESTION"
	IntentBaggageInfoQuestion              Intent = "BAGGAGE_INFO_QUESTION"
	IntentBoardingInfoQuestion             Intent = "BOARDING_INFO_QUESTION"
	IntentHumanSupportInfoQuestion         Intent = "HUMAN_SUPPORT_INFO_QUESTION"
	IntentPaymentStatusQuery               Intent = "PAYMENT_STATUS_QUERY"
	IntentPaymentCreate                    Intent = "PAYMENT_CREATE"
	IntentBookingCancel                    Intent = "BOOKING_CANCEL"
	IntentReschedule                       Intent = "RESCHEDULE"
	IntentUnsupportedCargo                 Intent = "UNSUPPORTED_CARGO"
	IntentUnsupportedPackage               Intent = "UNSUPPORTED_PACKAGE"
	IntentHumanSupport                     Intent = "HUMAN_SUPPORT"
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

var (
	availabilityFoldedDateReferencePattern = regexp.MustCompile(`\b([0-9]{1,2})\s+([0-9]{1,2})(?:\s+[0-9]{2,4})?\b`)
	availabilityFoldedDayReferencePattern  = regexp.MustCompile(`\bdia\s+([0-9]{1,2})\b`)
)

const availabilityNegationTargetPattern = `(?:opcao\s*0?[1-5]|primeira|primeiro|segunda|segundo|terceira|terceiro|quarta|quarto|quinta|quinto|[0-9]{1,2}\s+[0-9]{1,2}(?:\s+[0-9]{2,4})?|dia\s+[0-9]{1,2}|essa\s+opcao|esta\s+opcao|essa\s+data|esta\s+data|esse\s+dia|esse\s+horario|essa|esta|esse|isso)`

var (
	availabilityTargetThenRejectionPattern = regexp.MustCompile(`\b` + availabilityNegationTargetPattern + `\s+(?:nao\s+pode\s+ser|nao\s+fica\s+bom|nao\s+consigo|nao\s+funciona|nao\s+serve|nao\s+rola|nao\s+da|nao\s+e|nao\s+eh|fica\s+ruim)\b`)
	availabilityRejectionThenTargetPattern = regexp.MustCompile(`\b(?:nao\s+pode\s+ser|nao\s+quero|nao\s+serve|nao\s+da|nao\s+rola|nao\s+funciona|nao\s+consigo|nao\s+e|nao\s+eh|nao\s+fico\s+com|nao\s+vou\s+com)\s+(?:a\s+)?` + availabilityNegationTargetPattern + `\b`)
	availabilityTargetNoSuffixPattern      = regexp.MustCompile(`\b` + availabilityNegationTargetPattern + `\s+nao\b`)
	bareOptionNumberRejectAfterPattern     = regexp.MustCompile(`\b0?([1-5])\s+nao\s+(?:pode\s+ser|serve|da|rola|funciona|fica\s+bom|consigo)\b`)
	bareOptionNumberRejectBeforePattern    = regexp.MustCompile(`\bnao\s+(?:pode\s+ser|serve|da|rola|funciona|quero|consigo|e|eh)\s+(?:a\s+)?0?([1-5])\b`)
	bareOptionNumberBadPattern             = regexp.MustCompile(`\b0?([1-5])\s+fica\s+ruim\b`)
	bareOptionNumberNoSuffixPattern        = regexp.MustCompile(`\b0?([1-5])\s+nao\b`)
	deicticOptionNumberPattern             = regexp.MustCompile(`\b(?:essa|esta|esse|isso)\s+0?([1-5])\b`)
)

var reservationStartTemplateIntentPhrases = []string{
	"como posso fazer para reservar",
	"como posso fazer pra reservar",
	"como posso fazer uma reserva",
	"como e que eu faco para reservar",
	"como e que eu faco para fazer uma reserva",
	"como e que eu faco pra reservar",
	"como e que eu faco pra fazer uma reserva",
	"como faco para reservar",
	"como faco para fazer uma reserva",
	"como faco pra reservar",
	"como faco pra fazer uma reserva",
	"como faco uma reserva",
	"como fazer uma reserva",
	"como reservar uma passagem",
	"como reservar passagem",
	"quero fazer uma reserva",
	"quero reservar",
}

func routeDeterministicIntent(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
	body := NormalizeIncomingCustomerText(currentTurn)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if body == "" {
		return IntentDecision{Intent: IntentUnknown, Source: "deterministic"}
	}
	activePrompt := InferActivePromptContext(history, state)
	if looksLikeAdministrativeNotesSupportQuestion(body) && !looksLikePassengerDocumentText(body, Session{}) {
		return buildAdministrativeNotesSupportIntentDecision()
	}
	if _, ok := inferUnsupportedCargoQuery(body); ok {
		return IntentDecision{Intent: IntentUnsupportedCargo, Source: "deterministic", TemplateName: TemplateUnsupportedCargo, Action: "template"}
	}
	if looksLikeHumanSupportIntent(folded) && !looksLikeHumanSupportInfoQuestion(folded) {
		return IntentDecision{Intent: IntentHumanSupport, Source: "deterministic", TemplateName: TemplateHumanHandoff, Action: "template"}
	}
	if looksLikeBookingCancelIntent(body) {
		return IntentDecision{Intent: IntentBookingCancel, Source: "deterministic"}
	}
	if looksLikePaymentStatusInfoQuestion(folded) {
		return IntentDecision{Intent: IntentPaymentStatusQuery, Source: "deterministic_payment_status_query"}
	}
	if decision, ok := routeActivePromptAnswer(activePrompt, history, body, folded, state, observedAt); ok {
		return decision
	}
	if decision, ok := buildOutOfTurnInfoDecision(body, activePrompt); ok {
		return decision
	}
	if looksLikePaymentOptionsInfoQuestion(folded) {
		return IntentDecision{Intent: IntentPaymentInfoQuestion, Source: "deterministic_payment_options_info", TemplateName: TemplatePaymentOptionsInfo, Action: "template"}
	}
	if looksLikePayingPassengerInfoQuestion(folded) {
		return IntentDecision{Intent: IntentPayingPassengerInfoQuestion, Source: "deterministic_paying_passenger_info", TemplateName: TemplatePayingPassengerInfo, Action: "template"}
	}
	if looksLikeHumanSupportInfoQuestion(folded) {
		return IntentDecision{Intent: IntentHumanSupportInfoQuestion, Source: "deterministic_human_support_info", TemplateName: TemplateHumanSupportInfo, Action: "template"}
	}
	if activePrompt.Kind == ActivePromptPaymentPreference &&
		looksLikePixOnlyPaymentReply(folded) &&
		!looksLikePaymentCreateIntent(body) {
		if decision, ok := buildActivePromptContextualFallbackDecision(activePrompt, body, folded, state); ok {
			return decision
		}
	}
	if activePrompt.Kind != ActivePromptAvailabilityOptionChoice && looksLikeUnsupportedPaymentMethodQuestion(body) {
		return IntentDecision{Intent: IntentPaymentMethodQuestion, Source: "deterministic_payment_method_question", TemplateName: TemplatePaymentMethods, Action: "template"}
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
	if isPaymentDocumentReplyPhase(state.Phase) && lastAssistantAskedPayerCPF(history) && looksLikeBareCPF(body) {
		return IntentDecision{
			Intent: IntentPaymentCreate,
			Source: "deterministic_payer_document_reply",
			Action: "tool",
		}
	}
	if looksLikeRescheduleIntent(folded) {
		return IntentDecision{Intent: IntentReschedule, Source: "deterministic"}
	}

	if decision, ok := buildActivePromptContextualFallbackDecision(activePrompt, body, folded, state); ok {
		return decision
	}

	if activePrompt.Kind != ActivePromptAvailabilityOptionChoice {
		optionCount := activePrompt.AvailabilityOptionCount
		if optionCount <= 0 {
			optionCount = currentAvailabilitySelectionOptionCount(history)
		}
		if decision, ok := routeAvailabilityOptionAnswer(optionCount, history, body, folded, "deterministic", state.Phase == ConversationPhaseTripSelection); ok {
			return decision
		}
		if optionCount > 1 && looksLikeAmbiguousAvailabilityOptionReply(body, folded) {
			return IntentDecision{Intent: IntentUnknown, Source: "deterministic_availability_option_ambiguous"}
		}
	}
	if input, ok := parseAvailabilityDateSelectionInput(history, body, observedAt); ok {
		return IntentDecision{
			Intent:            IntentAvailabilitySearch,
			Source:            "deterministic_availability_date_selection",
			AvailabilityInput: &input,
			Action:            "tool",
		}
	}
	if looksLikeUnsupportedPaymentMethodQuestion(body) {
		return IntentDecision{Intent: IntentPaymentMethodQuestion, Source: "deterministic_payment_method_question", TemplateName: TemplatePaymentMethods, Action: "template"}
	}
	if looksLikeVerifyAllOptionsIntent(body) {
		input, missing := parseVerifyAllOptionsAvailabilityInput(history, observedAt)
		switch missing {
		case "":
			return IntentDecision{
				Intent:            IntentAvailabilitySearch,
				Source:            "deterministic_verify_all_options",
				AvailabilityInput: &input,
				Action:            "tool",
			}
		case "origin":
			return IntentDecision{
				Intent:            IntentAvailabilitySearch,
				Source:            "deterministic_verify_all_options_missing_origin",
				AvailabilityInput: &input,
				TemplateName:      TemplateAskMAOrigin,
				Action:            "template",
			}
		default:
			return IntentDecision{
				Intent: IntentAvailabilitySearch,
				Source: "deterministic_verify_all_options_missing_" + missing,
				Action: "safe_fallback",
			}
		}
	}
	if decision, ok := routeBroadStateTemplateIntent(body, folded); ok {
		return decision
	}
	if looksLikeReservationHowToProceedIntent(body) {
		if looksLikeReservationStartTemplateIntent(body) && shouldUseReservationStartTemplate(state, history) {
			return IntentDecision{
				Intent:       IntentAvailabilitySearch,
				Source:       "deterministic_reservation_start",
				TemplateName: TemplateAskReservationRouteSC,
				Action:       "template",
			}
		}
		return IntentDecision{
			Intent: IntentBookingCreateConfirmation,
			Source: "deterministic_reservation_next_step",
			Action: "safe_fallback",
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
	if state.Phase != ConversationPhasePassengerCollection &&
		state.Phase != ConversationPhaseBookingPending {
		if query, ok := inferUnsupportedRouteFollowUp(history, body); ok && activePromptAllowsUnsupportedFollowUp(activePrompt) {
			return IntentDecision{
				Intent:       IntentUnsupportedPackage,
				Source:       "deterministic_unsupported_followup",
				TemplateName: TemplateUnsupportedPackage,
				Action:       "template",
				TemplateData: map[string]interface{}{"destination": query.Destination},
			}
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

func isPaymentDocumentReplyPhase(phase ConversationPhase) bool {
	return phase == ConversationPhaseBooked || phase == ConversationPhasePaymentPending
}

func routeActivePromptAnswer(ctx ActivePromptContext, history []Message, body string, folded string, state CanonicalConversationState, observedAt time.Time) (IntentDecision, bool) {
	switch ctx.Kind {
	case ActivePromptAvailabilityOptionChoice:
		optionCount := ctx.AvailabilityOptionCount
		if optionCount <= 0 {
			optionCount = currentAvailabilitySelectionOptionCount(history)
		}
		if decision, ok := routeAvailabilityOptionAnswer(optionCount, history, body, folded, "deterministic_active_prompt_availability_option", true); ok {
			return decision, true
		}
		outOfTurnInfoQuestion := detectOutOfTurnInfoQuestion(body, ctx) != OutOfTurnInfoUnknown
		continuationReminder := activePromptIsContinuationReminder(ctx)
		if outOfTurnInfoQuestion || continuationReminder {
			if decision, ok := routeAvailabilityOptionDateAnswer(history, body, observedAt, "deterministic_active_prompt_availability_option_date"); ok {
				return decision, true
			}
			if continuationReminder && !outOfTurnInfoQuestion && extractTripDate(body, observedAt) != nil {
				return buildActivePromptContextualFallbackTemplateDecision(
					"deterministic_availability_option_date_missing_current_facts",
					TemplateContextFallbackAvailabilityOption,
				), true
			}
		}
	case ActivePromptAvailabilityDateChoice:
		if input, ok := parseAvailabilityDateSelectionInput(history, body, observedAt); ok {
			return IntentDecision{
				Intent:            IntentAvailabilitySearch,
				Source:            "deterministic_availability_date_selection",
				AvailabilityInput: &input,
				Action:            "tool",
			}, true
		}
	case ActivePromptPassengerCount, ActivePromptLapChildQuestion:
		slots := parsePassengerClarificationSlots(body)
		if slots.PassengerCountKnown || slots.ChildUnder5CountKnown {
			return IntentDecision{Intent: IntentPassengerCountReply, Source: "deterministic_active_prompt_passenger_count"}, true
		}
	case ActivePromptPassengerDocuments:
		if looksLikePassengerDocumentText(body, Session{}) {
			return IntentDecision{Intent: IntentPassengerDocumentsProvided, Source: "deterministic_active_prompt_passenger_documents"}, true
		}
	case ActivePromptDocumentConfirmation:
		if looksLikeDocumentConfirmation(body) {
			return IntentDecision{Intent: IntentDocumentConfirmation, Source: "deterministic_active_prompt_document_confirmation"}, true
		}
	case ActivePromptPaymentPreference:
		if preference := detectActivePromptPaymentPreferenceReply(body, folded); preference != "" {
			return IntentDecision{Intent: IntentPaymentPreference, Source: "deterministic_active_prompt_payment_preference"}, true
		}
	case ActivePromptPayerCPF:
		if (state.Phase == "" || isPaymentDocumentReplyPhase(state.Phase)) && looksLikeBareCPF(body) {
			return IntentDecision{Intent: IntentPaymentCreate, Source: "deterministic_payer_document_reply", Action: "tool"}, true
		}
	case ActivePromptLapChildAssignment:
		if index := activePromptLapChildAssignmentAnswerIndex(body); index > 0 {
			if count := activePromptLapChildAssignmentOptionCount(ctx); count > 0 && index > count {
				return IntentDecision{}, false
			}
			return IntentDecision{
				Intent:              IntentLapChildAssignmentAnswer,
				Source:              "deterministic_active_prompt_lap_child_assignment",
				SelectedOptionIndex: index,
			}, true
		}
	}

	return IntentDecision{}, false
}

func activePromptIsContinuationReminder(ctx ActivePromptContext) bool {
	return activePromptContinuationReminderFolded(activePromptFolded(ctx.SourceMessageBody)) != ""
}

func routeAvailabilityOptionDateAnswer(history []Message, body string, observedAt time.Time, source string) (IntentDecision, bool) {
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if looksLikeNegatedAvailabilitySelection(folded) {
		return IntentDecision{}, false
	}
	promptContext := currentAvailabilitySelectionPromptContext(history)
	if promptContext.OptionCount <= 0 || !promptContext.HasCurrentFacts {
		return IntentDecision{}, false
	}
	selectedDate := extractTripDate(body, observedAt)
	if selectedDate == nil {
		return IntentDecision{}, false
	}
	latest := currentAvailabilitySelectionPromptAvailabilityContextAt(history, observedAt)
	if latest == nil || len(latest.Results) == 0 {
		return IntentDecision{}, false
	}

	selectedIndex := 0
	for i, item := range latest.Results {
		itemDate := parseISODatePtr(item.TripDate)
		if !sameCalendarDate(itemDate, selectedDate) {
			continue
		}
		if selectedIndex > 0 {
			return IntentDecision{}, false
		}
		selectedIndex = i + 1
	}
	if selectedIndex <= 0 {
		return IntentDecision{}, false
	}
	if latestAvailabilitySelectionEvidence(history).rejectsAvailabilityOptionForHistory(
		promptContext.SourceHistoryIndex,
		selectedIndex,
		latest.Results[selectedIndex-1].TripDate,
	) {
		return IntentDecision{}, false
	}
	return IntentDecision{
		Intent:              IntentSelectAvailabilityOption,
		Source:              source,
		SelectedOptionIndex: selectedIndex,
		TemplateName:        TemplateAskPassengerCount,
		Action:              "template",
	}, true
}

func routeAvailabilityOptionAnswer(optionCount int, history []Message, body string, folded string, source string, allowConfirmation bool) (IntentDecision, bool) {
	if looksLikeNegatedAvailabilitySelection(folded) {
		return IntentDecision{}, false
	}
	promptContext := currentAvailabilitySelectionPromptContext(history)
	if promptContext.OptionCount > 0 {
		optionCount = promptContext.OptionCount
	} else if optionCount <= 0 {
		optionCount = currentAvailabilitySelectionOptionCount(history)
	}
	if optionCount <= 0 {
		return IntentDecision{}, false
	}

	if index := extractSelectedOptionIndex(body); index > 0 {
		if index > optionCount {
			return IntentDecision{}, false
		}
		if !promptContext.HasCurrentFacts {
			return buildActivePromptContextualFallbackTemplateDecision(
				"deterministic_availability_option_missing_current_facts",
				TemplateContextFallbackAvailabilityOption,
			), true
		}
		if availabilityPromptSelectionRejected(history, promptContext, index) {
			return buildActivePromptContextualFallbackTemplateDecision(
				"deterministic_availability_option_rejected",
				TemplateContextFallbackAvailabilityOption,
			), true
		}
		return IntentDecision{
			Intent:              IntentSelectAvailabilityOption,
			Source:              source,
			SelectedOptionIndex: index,
			TemplateName:        TemplateAskPassengerCount,
			Action:              "template",
		}, true
	}
	if optionCount == 1 && looksLikeContextualAvailabilitySelection(folded) {
		if !promptContext.HasCurrentFacts {
			return buildActivePromptContextualFallbackTemplateDecision(
				"deterministic_availability_option_missing_current_facts",
				TemplateContextFallbackAvailabilityOption,
			), true
		}
		if availabilityPromptSelectionRejected(history, promptContext, 1) {
			return buildActivePromptContextualFallbackTemplateDecision(
				"deterministic_availability_option_rejected",
				TemplateContextFallbackAvailabilityOption,
			), true
		}
		return IntentDecision{
			Intent:              IntentSelectAvailabilityOption,
			Source:              source,
			SelectedOptionIndex: 1,
			TemplateName:        TemplateAskPassengerCount,
			Action:              "template",
		}, true
	}
	if optionCount == 1 && allowConfirmation && looksLikeBookingCreateConfirmation(body) {
		if !promptContext.HasCurrentFacts {
			return buildActivePromptContextualFallbackTemplateDecision(
				"deterministic_availability_option_missing_current_facts",
				TemplateContextFallbackAvailabilityOption,
			), true
		}
		if availabilityPromptSelectionRejected(history, promptContext, 1) {
			return buildActivePromptContextualFallbackTemplateDecision(
				"deterministic_availability_option_rejected",
				TemplateContextFallbackAvailabilityOption,
			), true
		}
		return IntentDecision{
			Intent:              IntentSelectAvailabilityOption,
			Source:              "deterministic_trip_confirmation_recovery",
			SelectedOptionIndex: 1,
			TemplateName:        TemplateAskPassengerCount,
			Action:              "template",
		}, true
	}

	return IntentDecision{}, false
}

func availabilityPromptSelectionRejected(history []Message, promptContext availabilitySelectionPromptContext, index int) bool {
	if index <= 0 {
		return false
	}

	tripDate := ""
	if current := currentAvailabilitySelectionPromptAvailabilityContext(history); current != nil {
		selectedIndex := index - 1
		if selectedIndex >= 0 && selectedIndex < len(current.Results) {
			tripDate = current.Results[selectedIndex].TripDate
		}
	}

	return latestAvailabilitySelectionEvidence(history).rejectsAvailabilityOptionForHistory(
		promptContext.SourceHistoryIndex,
		index,
		tripDate,
	)
}

type availabilitySelectionPromptContext struct {
	OptionCount        int
	HasCurrentFacts    bool
	SourceHistoryIndex int
}

func looksLikeAmbiguousAvailabilityOptionReply(body string, folded string) bool {
	if looksLikeContextualAvailabilitySelection(folded) || looksLikeBookingCreateConfirmation(body) {
		return true
	}
	switch folded {
	case "pode ser", "pode ser sim":
		return true
	default:
		return false
	}
}

func currentAvailabilitySelectionOptionCount(history []Message) int {
	return currentAvailabilitySelectionPromptContext(history).OptionCount
}

func currentAvailabilitySelectionPromptContext(history []Message) availabilitySelectionPromptContext {
	message, sourceHistoryIndex, ok := latestReliableAssistantMessageWithIndex(history)
	if !ok {
		return availabilitySelectionPromptContext{}
	}

	context := availabilitySelectionPromptContextFromMessage(message)
	context.SourceHistoryIndex = sourceHistoryIndex
	return context
}

func availabilitySelectionPromptContextFromMessage(message Message) availabilitySelectionPromptContext {
	body := messageTurnText(message)
	renderedCount := availabilityOptionCountFromRenderedPrompt(body)
	currentFactsCount := availabilityOptionCountFromMessageToolContext(message)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if renderedCount <= 0 && currentFactsCount <= 0 && !looksLikeAvailabilitySelectionPrompt(folded) {
		return availabilitySelectionPromptContext{}
	}

	optionCount := renderedCount
	if optionCount <= 0 {
		optionCount = currentFactsCount
	}
	hasCurrentFacts := currentFactsCount > 0 && (renderedCount <= 0 || renderedCount == currentFactsCount)
	return availabilitySelectionPromptContext{
		OptionCount:        optionCount,
		HasCurrentFacts:    hasCurrentFacts,
		SourceHistoryIndex: -1,
	}
}

func currentAvailabilitySelectionPromptAvailabilityContext(history []Message) *AvailabilitySearchResult {
	return currentAvailabilitySelectionPromptAvailabilityContextAt(history, time.Now())
}

func currentAvailabilitySelectionPromptAvailabilityContextAt(history []Message, observedAt time.Time) *AvailabilitySearchResult {
	message, ok := latestReliableAssistantMessage(history)
	if !ok {
		return nil
	}

	return visibleAvailabilityContextFromPromptMessageAt(message, observedAt)
}

func activePromptAllowsUnsupportedFollowUp(ctx ActivePromptContext) bool {
	switch ctx.Kind {
	case ActivePromptReservationRoute:
		return true
	case ActivePromptUnknown:
		return ctx.Phase == "" ||
			ctx.Phase == ConversationPhaseDiscovery ||
			ctx.Phase == ConversationPhaseRouteSelection
	default:
		return false
	}
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

func hasCurrentAvailabilitySelectionContext(history []Message) bool {
	return currentAvailabilitySelectionPromptContext(history).OptionCount > 0
}

func looksLikeContextualAvailabilitySelection(folded string) bool {
	if looksLikeNegatedAvailabilitySelection(folded) {
		return false
	}
	switch folded {
	case "certo",
		"confirmo",
		"e isso",
		"eh isso",
		"essa",
		"essa ai",
		"essa mesmo",
		"essa mesma",
		"essa msm",
		"essa opcao",
		"essa opcao mesmo",
		"essa passagem",
		"esta",
		"esta msm",
		"esta opcao",
		"esse",
		"esse horario",
		"esse msm",
		"fechado",
		"isso",
		"isso mesmo",
		"isso msm",
		"pode ser",
		"pode ser essa",
		"pode ser sim",
		"sim":
		return true
	default:
		return looksLikeMixedContextualAvailabilitySelection(folded)
	}
}

func looksLikeMixedContextualAvailabilitySelection(folded string) bool {
	if looksLikeNegatedAvailabilitySelection(folded) {
		return false
	}
	return containsFoldedAny(
		folded,
		"essa msm",
		"essa mesmo",
		"essa mesma",
		"esse msm",
		"esta msm",
		"esta mesmo",
		"esta mesma",
		"isso msm",
		"isso mesmo",
		"pode ser essa",
		"pode ser esta",
		"quero essa",
		"quero esta",
		"fico com essa",
		"fico com esta",
	)
}

func looksLikeNegatedAvailabilitySelection(folded string) bool {
	return parseAvailabilityRejectionEvidence(folded).Found
}

type availabilityRejectionEvidence struct {
	Found         bool
	WholeContext  bool
	OptionIndexes []int
	TripDates     []string
}

func (e availabilityRejectionEvidence) hasSpecificTarget() bool {
	return len(e.OptionIndexes) > 0 || len(e.TripDates) > 0
}

func parseAvailabilityRejectionEvidence(folded string) availabilityRejectionEvidence {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return availabilityRejectionEvidence{}
	}

	found := hasAvailabilityTargetThenRejection(folded) ||
		hasAvailabilityRejectionThenTarget(folded) ||
		hasAvailabilityRejectionSuffix(folded) ||
		hasBareAvailabilityOptionNumberRejection(folded)
	if !found {
		return availabilityRejectionEvidence{}
	}

	evidence := availabilityRejectionEvidence{
		Found:         true,
		OptionIndexes: availabilityRejectedOptionIndexes(folded),
		TripDates:     availabilityRejectedTripDates(folded),
	}
	if !evidence.hasSpecificTarget() {
		evidence.WholeContext = true
	}
	return evidence
}

func availabilityRejectedOptionIndexes(folded string) []int {
	indexes := []int{}
	for _, match := range optionIndexPattern.FindAllStringSubmatch(folded, -1) {
		if len(match) == 2 {
			indexes = appendUniqueAvailabilityOptionIndex(indexes, availabilityOptionIndexFromText(match[1]))
		}
	}
	for _, item := range []struct {
		phrase string
		index  int
	}{
		{"primeira", 1},
		{"primeiro", 1},
		{"segunda", 2},
		{"segundo", 2},
		{"terceira", 3},
		{"terceiro", 3},
		{"quarta", 4},
		{"quarto", 4},
		{"quinta", 5},
		{"quinto", 5},
	} {
		if foldedContainsPhrase(folded, item.phrase) {
			indexes = appendUniqueAvailabilityOptionIndex(indexes, item.index)
		}
	}
	for _, pattern := range []*regexp.Regexp{
		bareOptionNumberRejectAfterPattern,
		bareOptionNumberRejectBeforePattern,
		bareOptionNumberBadPattern,
		bareOptionNumberNoSuffixPattern,
		deicticOptionNumberPattern,
	} {
		for _, match := range pattern.FindAllStringSubmatchIndex(folded, -1) {
			if len(match) >= 4 && match[2] >= 0 && match[3] >= 0 {
				if availabilityCaptureInsideFoldedDate(folded, match[2], match[3]) {
					continue
				}
				indexes = appendUniqueAvailabilityOptionIndex(indexes, availabilityOptionIndexFromText(folded[match[2]:match[3]]))
			}
		}
	}
	return indexes
}

func availabilityCaptureInsideFoldedDate(folded string, captureStart int, captureEnd int) bool {
	for _, dateRange := range availabilityFoldedDateReferencePattern.FindAllStringIndex(folded, -1) {
		if len(dateRange) == 2 && captureStart >= dateRange[0] && captureEnd <= dateRange[1] {
			return true
		}
	}
	return false
}

func availabilityOptionIndexFromText(text string) int {
	index, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0
	}
	return index
}

func appendUniqueAvailabilityOptionIndex(indexes []int, index int) []int {
	if index < 1 || index > 5 {
		return indexes
	}
	for _, existing := range indexes {
		if existing == index {
			return indexes
		}
	}
	return append(indexes, index)
}

func availabilityRejectedTripDates(folded string) []string {
	dates := []string{}
	for _, match := range availabilityFoldedDateReferencePattern.FindAllStringSubmatch(folded, -1) {
		if len(match) != 3 || !validAvailabilityDayMonth(match[1], match[2]) {
			continue
		}
		dates = appendUniqueAvailabilityTripDate(dates, canonicalAvailabilityDayMonth(match[1], match[2]))
	}
	return dates
}

func appendUniqueAvailabilityTripDate(dates []string, date string) []string {
	date = strings.TrimSpace(date)
	if date == "" {
		return dates
	}
	for _, existing := range dates {
		if existing == date {
			return dates
		}
	}
	return append(dates, date)
}

func canonicalAvailabilityDayMonth(dayText string, monthText string) string {
	day, _ := strconv.Atoi(dayText)
	month, _ := strconv.Atoi(monthText)
	return twoDigitDatePart(day) + "/" + twoDigitDatePart(month)
}

func twoDigitDatePart(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func hasAvailabilityTargetThenRejection(folded string) bool {
	return availabilityTargetThenRejectionPattern.MatchString(folded)
}

func hasAvailabilityRejectionThenTarget(folded string) bool {
	return availabilityRejectionThenTargetPattern.MatchString(folded)
}

func hasBareAvailabilityOptionNumberRejection(folded string) bool {
	return bareOptionNumberRejectAfterPattern.MatchString(folded) ||
		bareOptionNumberRejectBeforePattern.MatchString(folded) ||
		bareOptionNumberBadPattern.MatchString(folded) ||
		bareOptionNumberNoSuffixPattern.MatchString(folded)
}

func hasAvailabilityRejectionSuffix(folded string) bool {
	return availabilityTargetNoSuffixPattern.MatchString(folded)
}

func hasAvailabilitySelectionTarget(folded string) bool {
	return hasAvailabilityOptionReference(folded) ||
		hasAvailabilityDateReference(folded) ||
		hasAvailabilityDeicticReference(folded)
}

func hasAvailabilityOptionReference(folded string) bool {
	if optionIndexPattern.MatchString(folded) {
		return true
	}
	return foldedContainsAnyPhrase(
		folded,
		"primeira",
		"primeiro",
		"segunda",
		"segundo",
		"terceira",
		"terceiro",
		"quarta",
		"quarto",
		"quinta",
		"quinto",
	)
}

func hasAvailabilityDateReference(folded string) bool {
	for _, match := range availabilityFoldedDateReferencePattern.FindAllStringSubmatch(folded, -1) {
		if len(match) == 3 && validAvailabilityDayMonth(match[1], match[2]) {
			return true
		}
	}
	for _, match := range availabilityFoldedDayReferencePattern.FindAllStringSubmatch(folded, -1) {
		if len(match) == 2 && validAvailabilityDay(match[1]) {
			return true
		}
	}
	return false
}

func hasAvailabilityDeicticReference(folded string) bool {
	return foldedContainsAnyPhrase(
		folded,
		"essa",
		"esta",
		"esse",
		"isso",
		"essa opcao",
		"esta opcao",
		"essa data",
		"esta data",
		"esse dia",
		"esse horario",
	)
}

func hasAvailabilityRejectionPhrase(folded string) bool {
	return foldedContainsAnyPhrase(
		folded,
		"nao quero",
		"nao pode ser",
		"nao e",
		"nao eh",
		"nao serve",
		"nao da",
		"nao rola",
		"nao funciona",
		"nao fica bom",
		"fica ruim",
		"nao consigo",
	)
}

func validAvailabilityDayMonth(dayText string, monthText string) bool {
	day, dayErr := strconv.Atoi(dayText)
	month, monthErr := strconv.Atoi(monthText)
	return dayErr == nil && monthErr == nil && day >= 1 && day <= 31 && month >= 1 && month <= 12
}

func validAvailabilityDay(dayText string) bool {
	day, err := strconv.Atoi(dayText)
	return err == nil && day >= 1 && day <= 31
}

func foldedContainsAnyPhrase(folded string, phrases ...string) bool {
	for _, phrase := range phrases {
		if foldedContainsPhrase(folded, phrase) {
			return true
		}
	}
	return false
}

func foldedContainsPhrase(folded string, phrase string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	phrase = strings.Join(strings.Fields(phrase), " ")
	if folded == "" || phrase == "" {
		return false
	}
	return strings.Contains(" "+folded+" ", " "+phrase+" ")
}

func looksLikePaymentOptionsInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" || looksLikePaymentStatusInfoQuestion(folded) {
		return false
	}
	if looksLikePaymentPreferenceAssertion(folded) {
		return false
	}

	hasPaymentWord := strings.Contains(folded, "pagamento") ||
		strings.Contains(folded, "pagar") ||
		strings.Contains(folded, "paga")
	if !hasPaymentWord {
		return false
	}
	if strings.Contains(folded, "como funciona") && strings.Contains(folded, "pagamento") {
		return true
	}
	if (strings.Contains(folded, "forma") || strings.Contains(folded, "formas") || strings.Contains(folded, "opcoes")) &&
		strings.Contains(folded, "pagamento") {
		return true
	}
	if containsFoldedAny(folded, "agora", "logo") && containsFoldedAny(folded, "dia", "embarque") {
		return true
	}
	if looksLikePaymentTimingInfoQuestion(folded) {
		return true
	}
	if strings.Contains(folded, "paga tudo agora") || strings.Contains(folded, "pagar tudo agora") {
		return true
	}
	if strings.Contains(folded, "sinal") &&
		(strings.Contains(folded, "so") || strings.Contains(folded, "apenas") || strings.Contains(folded, "pagar") || strings.Contains(folded, "paga")) {
		return true
	}
	if strings.Contains(folded, "restante") && containsFoldedAny(folded, "embarque", "dia") {
		return true
	}
	return false
}

func looksLikePaymentTimingInfoQuestion(folded string) bool {
	if containsFoldedAny(
		folded,
		"paga agora",
		"pagar agora",
		"pagamento e agora",
		"pagamento eh agora",
		"pagamento faz logo",
		"paga no dia",
		"pagar no dia",
		"paga so no dia",
		"pagar so no dia",
		"paga no embarque",
		"pagar no embarque",
	) {
		return true
	}
	if containsFoldedAny(folded, "precisa", "tem que", "tenho que", "devo", "obrigatorio") &&
		containsFoldedAny(folded, "pagar", "paga") &&
		containsFoldedAny(folded, "agora", "no dia", "embarque") {
		return true
	}
	if containsFoldedAny(folded, "pode", "posso", "da pra", "tem como", "consigo") &&
		containsFoldedAny(folded, "pagar", "paga") &&
		containsFoldedAny(folded, "no dia", "embarque", "depois") {
		return true
	}
	return false
}

func looksLikePayingPassengerInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	if strings.Contains(folded, "passageiro pagante") {
		if looksLikePaymentPreferenceAssertion(folded) {
			return false
		}
		return containsFoldedAny(
			folded,
			"o que",
			"quem",
			"qual",
			"significa",
			"quer dizer",
			"idade",
			"crianca",
			"maior",
			"anos",
			"paga",
		)
	}
	if strings.Contains(folded, "crianca") && strings.Contains(folded, "paga") {
		return true
	}
	if strings.Contains(folded, "maior de 5 anos") && strings.Contains(folded, "paga") {
		return true
	}
	if strings.Contains(folded, "maior que 5") && strings.Contains(folded, "paga") {
		return true
	}
	return false
}

func detectActivePromptPaymentPreferenceReply(body string, folded string) string {
	if looksLikePaymentStatusInfoQuestion(folded) {
		return ""
	}
	preference := detectStructuredPaymentPreference(body)
	if preference == "" {
		return ""
	}
	if looksLikePaymentPreferenceInfoQuestion(folded) {
		return ""
	}
	return preference
}

func looksLikePaymentPreferenceInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	return containsFoldedAny(
		folded,
		"posso pagar",
		"pode pagar",
		"tem como pagar",
		"da pra pagar",
		"consigo pagar",
	)
}

func looksLikePaymentPreferenceAssertion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	if folded == "sinal" || folded == "integral" {
		return true
	}
	if strings.HasPrefix(folded, "sinal ") || strings.HasPrefix(folded, "integral ") {
		return true
	}
	if strings.HasPrefix(folded, "pagar ") &&
		containsFoldedAny(folded, "sinal", "integral", "tudo", "total") {
		return true
	}
	return containsFoldedAny(
		folded,
		"quero pagar",
		"vou pagar",
		"irei pagar",
		"prefiro pagar",
		"escolho",
		"fico com",
	)
}

func looksLikePaymentStatusInfoQuestion(folded string) bool {
	folded = strings.Join(strings.Fields(folded), " ")
	if folded == "" {
		return false
	}
	if strings.Contains(folded, "paguei") || strings.Contains(folded, "ja paguei") {
		return true
	}
	if strings.Contains(folded, "status") && strings.Contains(folded, "pagamento") {
		return true
	}
	if strings.Contains(folded, "pagamento") && containsFoldedAny(folded, "aprovado", "caiu", "entrou") {
		return true
	}
	if folded == "caiu" || folded == "ja caiu" || strings.Contains(folded, "ja caiu") {
		return true
	}
	return false
}

func containsFoldedAny(folded string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(folded, value) {
			return true
		}
	}
	return false
}

func looksLikeReservationHowToProceedIntent(text string) bool {
	body := NormalizeIncomingCustomerText(text)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if folded == "" {
		return false
	}
	if hasExplicitRouteContextForReservationHelp(body) {
		return false
	}
	if foldedContainsAnyReservationPhrase(folded, reservationStartTemplateIntentPhrases) {
		return true
	}
	for _, phrase := range []string{
		"como reserva",
		"como prosseguir",
		"como agendar",
		"quero agendar",
		"agendar",
	} {
		if folded == phrase || strings.Contains(folded, phrase) {
			return true
		}
	}
	return false
}

func hasExplicitRouteContextForReservationHelp(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	if strings.Contains(folded, " de ") && strings.Contains(folded, " para ") {
		return true
	}
	if _, _, ok := extractExplicitRouteFromText(text); ok {
		return true
	}
	if locations := extractCanonicalLocations(text); len(locations) >= 2 {
		return true
	}
	if locations := extractSupportedPackageLocationsInOrder(text); len(locations) >= 2 {
		return true
	}
	return false
}

func shouldUseReservationStartTemplate(state CanonicalConversationState, history []Message) bool {
	if state.Phase != "" && state.Phase != ConversationPhaseDiscovery {
		return false
	}
	if hasPreviousAvailabilityList(history) {
		return false
	}
	if hasCanonicalRoute(state) {
		return false
	}
	if strings.TrimSpace(state.Route.TripID) != "" ||
		state.Route.SelectedOptionIndex > 0 ||
		strings.TrimSpace(state.Route.BoardStopID) != "" ||
		strings.TrimSpace(state.Route.AlightStopID) != "" ||
		strings.TrimSpace(state.Route.TripDate) != "" {
		return false
	}
	if state.Passengers.ExpectedCount > 0 || state.Passengers.DocumentsCollected {
		return false
	}
	if strings.TrimSpace(state.Booking.BookingID) != "" ||
		strings.TrimSpace(state.Booking.ReservationCode) != "" ||
		strings.TrimSpace(state.Booking.Status) != "" {
		return false
	}
	if strings.TrimSpace(state.Payment.Status) != "" ||
		strings.TrimSpace(state.Payment.Preference) != "" {
		return false
	}
	return true
}

func looksLikeReservationStartTemplateIntent(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(NormalizeIncomingCustomerText(text))), " ")
	if folded == "" {
		return false
	}
	return foldedContainsAnyReservationPhrase(folded, reservationStartTemplateIntentPhrases)
}

func foldedContainsAnyReservationPhrase(folded string, phrases []string) bool {
	for _, phrase := range phrases {
		if folded == phrase || strings.Contains(folded, phrase) {
			return true
		}
	}
	return false
}

func looksLikeVerifyAllOptionsIntent(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(NormalizeIncomingCustomerText(text))), " ")
	if folded == "" {
		return false
	}
	switch folded {
	case "verificar todas as opcoes", "ver todas", "ver todas as opcoes", "todas as opcoes", "pode verificar todas":
		return true
	default:
		return strings.Contains(folded, "verificar todas as opcoes") ||
			strings.Contains(folded, "todas as opcoes")
	}
}

func looksLikeHumanSupportIntent(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	if strings.Contains(folded, "atendente") ||
		strings.Contains(folded, "humano") ||
		strings.Contains(folded, "suporte") {
		return true
	}
	for _, phrase := range []string{
		"falar com alguem",
		"falar com uma pessoa",
		"me passa para alguem",
		"me passa pra alguem",
		"me passe para alguem",
		"me passe pra alguem",
		"alguem pode me atender",
		"pode me atender",
		"me atende",
		"atendimento humano",
	} {
		if folded == phrase || strings.Contains(folded, phrase) {
			return true
		}
	}
	switch folded {
	case "tem alguem", "tem alguem ai", "tem alguem aqui":
		return true
	default:
		return false
	}
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
