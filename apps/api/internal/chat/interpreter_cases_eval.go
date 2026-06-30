package chat

import (
	"fmt"
	"strings"
	"time"
)

type InterpreterCaseEvaluationResult struct {
	CaseID                         string
	Category                       string
	LocalIntent                    string
	LocalTurnMeaning               string
	LocalTemplate                  string
	ActivePromptFallbackTemplate   string
	LocalSelectedOptionIndex       int
	LocalPaymentPreference         string
	LocalPassengerCount            int
	LocalChildUnder5Count          int
	ValidatorEvaluated             bool
	ValidatorProposalIntent        string
	ValidatorAccepted              bool
	ValidatorRejectReason          string
	ValidatorFallbackTemplate      string
	ValidatorProbeEvaluated        bool
	ValidatorProbeIntent           string
	ValidatorProbeAccepted         bool
	ValidatorProbeRejectReason     string
	ValidatorProbeFallbackTemplate string
	EffectiveIntent                string
	EffectiveTemplate              string
	EffectiveAction                string
	Passed                         bool
	Skipped                        bool
	SkippedReason                  string
	Failures                       []string
}

type InterpreterCaseEvaluationReport struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
	Results []InterpreterCaseEvaluationResult
}

func EvaluateInterpreterCases(cases []InterpreterCase, observedAt time.Time) InterpreterCaseEvaluationReport {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	report := InterpreterCaseEvaluationReport{
		Total:   len(cases),
		Results: make([]InterpreterCaseEvaluationResult, 0, len(cases)),
	}
	for _, item := range cases {
		result := evaluateInterpreterCase(item, observedAt)
		report.Results = append(report.Results, result)
		switch {
		case result.Skipped:
			report.Skipped++
		case result.Passed:
			report.Passed++
		default:
			report.Failed++
		}
	}
	return report
}

func evaluateInterpreterCase(item InterpreterCase, observedAt time.Time) InterpreterCaseEvaluationResult {
	fixture := buildInterpreterCaseFixture(item, observedAt)
	local := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: item.CurrentTurn,
		State:       fixture.state,
		History:     fixture.history,
		ObservedAt:  observedAt,
	})

	validation := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     local,
		CurrentTurn:  item.CurrentTurn,
		History:      fixture.history,
		State:        fixture.state,
		ActivePrompt: fixture.activePrompt,
		ObservedAt:   observedAt,
	})
	activePromptFallbackTemplate := interpreterCaseActivePromptFallbackTemplate(item, fixture, validation)

	result := InterpreterCaseEvaluationResult{
		CaseID:                       item.ID,
		Category:                     item.Category,
		LocalIntent:                  string(local.Intent),
		LocalTurnMeaning:             string(local.TurnMeaning),
		ActivePromptFallbackTemplate: activePromptFallbackTemplate,
		LocalSelectedOptionIndex:     interpreterCaseSelectedOptionIndex(local),
		LocalPaymentPreference:       strings.TrimSpace(local.Payment.PaymentPreference),
		LocalPassengerCount:          local.Booking.PassengerCount,
		LocalChildUnder5Count:        local.Booking.ChildUnder5Count,
		ValidatorEvaluated:           true,
		ValidatorProposalIntent:      string(local.Intent),
		ValidatorAccepted:            validation.Accepted,
		ValidatorRejectReason:        strings.TrimSpace(validation.RejectReason),
		ValidatorFallbackTemplate:    string(validation.FallbackTemplate),
	}
	if probe, ok := interpreterCaseValidatorProbe(item); ok {
		probeValidation := ValidateStructuredInterpretation(InterpretationValidationInput{
			Proposal:     probe,
			CurrentTurn:  item.CurrentTurn,
			History:      fixture.history,
			State:        fixture.state,
			ActivePrompt: fixture.activePrompt,
			ObservedAt:   observedAt,
		})
		result.ValidatorProbeEvaluated = true
		result.ValidatorProbeIntent = string(probe.Intent)
		result.ValidatorProbeAccepted = probeValidation.Accepted
		result.ValidatorProbeRejectReason = strings.TrimSpace(probeValidation.RejectReason)
		result.ValidatorProbeFallbackTemplate = string(probeValidation.FallbackTemplate)
	}
	result.EffectiveIntent = interpreterCaseEffectiveIntent(fixture.activePrompt, local, validation, activePromptFallbackTemplate)
	result.EffectiveTemplate = interpreterCaseEffectiveTemplate(item, fixture.activePrompt, local, validation, activePromptFallbackTemplate, result.EffectiveIntent)
	result.EffectiveAction = interpreterCaseEffectiveAction(fixture.activePrompt, validation, result.EffectiveIntent, result.EffectiveTemplate)
	result.LocalTemplate = result.EffectiveTemplate
	result.Failures = interpreterCaseEvaluationFailures(item, result, local, validation)
	result.Passed = len(result.Failures) == 0
	return result
}

type interpreterCaseFixture struct {
	state        CanonicalConversationState
	history      []Message
	activePrompt ActivePromptContext
}

func buildInterpreterCaseFixture(item InterpreterCase, observedAt time.Time) interpreterCaseFixture {
	state := CanonicalConversationState{
		SessionID: "interpreter-case-eval",
		Phase:     ConversationPhase(strings.TrimSpace(item.Phase)),
	}
	if state.Phase == "" {
		state.Phase = ConversationPhaseDiscovery
	}
	if state.Phase == ConversationPhaseBooked || state.Phase == ConversationPhasePaymentPending {
		state.Booking.BookingID = "booking-eval-001"
	}

	history := interpreterCaseHistory(item, observedAt)
	activePrompt := InferActivePromptContext(history, state)
	if activePrompt.Kind == ActivePromptUnknown {
		activePrompt.Kind = ActivePromptKind(strings.TrimSpace(item.ActivePromptKind))
		activePrompt.Phase = state.Phase
	}
	if activePrompt.Kind == ActivePromptAvailabilityOptionChoice && activePrompt.AvailabilityOptionCount <= 0 {
		activePrompt.AvailabilityOptionCount = interpreterCaseAvailabilityOptionCount(item)
		activePrompt.HasAvailabilityList = activePrompt.AvailabilityOptionCount > 0
	}
	return interpreterCaseFixture{state: state, history: history, activePrompt: activePrompt}
}

func interpreterCaseHistory(item InterpreterCase, observedAt time.Time) []Message {
	receivedAt := observedAt.Add(-2 * time.Minute)
	switch ActivePromptKind(strings.TrimSpace(item.ActivePromptKind)) {
	case ActivePromptAvailabilityDateChoice:
		return []Message{interpreterCaseOutboundPrompt(
			"Tenho datas disponiveis para essa viagem. Qual data prefere?",
			receivedAt,
			interpreterCaseAvailabilityPayload(1),
		)}
	case ActivePromptAvailabilityOptionChoice:
		count := interpreterCaseAvailabilityOptionCount(item)
		return []Message{interpreterCaseOutboundPrompt(
			interpreterCaseAvailabilityOptionPrompt(count),
			receivedAt,
			interpreterCaseAvailabilityPayload(count),
		)}
	case ActivePromptPassengerCount:
		return []Message{interpreterCaseOutboundPrompt(
			"A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?",
			receivedAt,
			nil,
		)}
	case ActivePromptLapChildQuestion:
		return []Message{interpreterCaseOutboundPrompt(
			"Tem crianca de 5 anos ou menos viajando?",
			receivedAt,
			nil,
		)}
	case ActivePromptLapChildAssignment:
		return []Message{interpreterCaseOutboundPrompt(
			interpreterCaseLapChildAssignmentPrompt(10),
			receivedAt,
			nil,
		)}
	case ActivePromptPassengerDocuments:
		return []Message{interpreterCaseOutboundPrompt(
			"Perfeito. Agora pode enviar seu nome completo e CPF ou RG do passageiro.",
			receivedAt,
			nil,
		)}
	case ActivePromptDocumentConfirmation:
		return []Message{interpreterCaseOutboundPrompt(
			"Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?",
			receivedAt,
			nil,
		)}
	case ActivePromptPaymentPreference:
		return []Message{interpreterCaseOutboundPrompt(
			"Voce prefere pagar o valor integral ou apenas o sinal de R$ 250?",
			receivedAt,
			nil,
		)}
	case ActivePromptPayerCPF:
		return []Message{interpreterCaseOutboundPrompt(
			"Para gerar o PIX, preciso do CPF do pagador.",
			receivedAt,
			nil,
		)}
	case ActivePromptReservationRoute:
		return []Message{interpreterCaseOutboundPrompt(
			"Qual trecho da viagem? Me diga a cidade de origem e destino.",
			receivedAt,
			nil,
		)}
	default:
		return nil
	}
}

func interpreterCaseOutboundPrompt(body string, receivedAt time.Time, availabilityPayload map[string]interface{}) Message {
	message := Message{
		Direction:        "OUTBOUND",
		Body:             body,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       receivedAt,
	}
	if len(availabilityPayload) > 0 {
		message.Payload = map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: availabilityPayload,
			},
		}
	}
	return message
}

func interpreterCaseAvailabilityOptionCount(item InterpreterCase) int {
	if item.ExpectedSelectedOptionIndex != nil && *item.ExpectedSelectedOptionIndex > 1 {
		return *item.ExpectedSelectedOptionIndex
	}
	if strings.Contains(strings.ToLower(item.CurrentTurn), "essa") {
		return 1
	}
	if item.FallbackTemplate == string(TemplateContextFallbackAvailabilityOption) {
		return 5
	}
	return 5
}

func interpreterCaseAvailabilityOptionPrompt(count int) string {
	if count <= 1 {
		return "Encontrei esta opcao:\n1. Santa Ines/MA para Videira/SC, 06/07 as 08:00\n\nQual opcao voce prefere?"
	}
	var builder strings.Builder
	builder.WriteString("Encontrei estas opcoes:\n")
	for index := 1; index <= count; index++ {
		builder.WriteString(fmt.Sprintf("%d. Santa Ines/MA para Videira/SC, 06/07 as %02d:00\n", index, 7+index))
	}
	builder.WriteString("\nQual opcao voce prefere?")
	return builder.String()
}

func interpreterCaseAvailabilityPayload(count int) map[string]interface{} {
	if count <= 0 {
		count = 1
	}
	result := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Ines/MA",
			Destination: "Videira/SC",
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       count,
		},
		Results: make([]AvailabilitySearchItem, 0, count),
	}
	for index := 1; index <= count; index++ {
		result.Results = append(result.Results, AvailabilitySearchItem{
			TripID:                 fmt.Sprintf("trip-eval-%02d", index),
			BoardStopID:            fmt.Sprintf("board-eval-%02d", index),
			AlightStopID:           fmt.Sprintf("alight-eval-%02d", index),
			OriginDisplayName:      "Santa Ines/MA",
			DestinationDisplayName: "Videira/SC",
			OriginDepartTime:       fmt.Sprintf("%02d:00", 7+index),
			TripDate:               "2026-07-06",
			SeatsAvailable:         8,
			Price:                  950,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToSantaCatarina,
		})
	}
	return buildAvailabilityToolResponsePayload(result)
}

func interpreterCaseLapChildAssignmentPrompt(count int) string {
	var builder strings.Builder
	builder.WriteString("Recebi os dados dos passageiros. Qual deles e a crianca de ate 5 anos?\n")
	for index := 1; index <= count; index++ {
		builder.WriteString(fmt.Sprintf("%d. Passageiro Teste %02d\n", index, index))
	}
	return strings.TrimRight(builder.String(), "\n")
}

func interpreterCaseValidatorProbe(item InterpreterCase) (StructuredInterpretation, bool) {
	for _, forbidden := range item.ForbiddenIntents {
		if proposal, ok := interpreterCaseSyntheticProposal(forbidden, item); ok {
			return proposal, true
		}
	}
	return StructuredInterpretation{}, false
}

func interpreterCaseSyntheticProposal(intent string, item InterpreterCase) (StructuredInterpretation, bool) {
	switch strings.TrimSpace(intent) {
	case string(IntentAvailabilitySearch):
		return newStructuredInterpretation(StructuredIntentAvailabilitySearch, TurnMeaningAnswerToQuestion, 0.95, "interpreter_case_eval"), true
	case string(IntentSelectAvailabilityOption):
		proposal := newStructuredInterpretation(StructuredIntentSelectAvailabilityOption, TurnMeaningAnswerToQuestion, 0.95, "interpreter_case_eval")
		index := 1
		if item.ExpectedSelectedOptionIndex != nil && *item.ExpectedSelectedOptionIndex > 0 {
			index = *item.ExpectedSelectedOptionIndex
		} else if parsed := validationSelectedOptionIndex(item.CurrentTurn); parsed > 0 {
			index = parsed
		}
		proposal.Booking.SelectedOptionIndex = index
		proposal.Booking.SelectedOptionIndexKnown = true
		return proposal, true
	case string(IntentPassengerCountReply):
		proposal := newStructuredInterpretation(StructuredIntentPassengerCountReply, TurnMeaningAnswerToQuestion, 0.95, "interpreter_case_eval")
		if item.ExpectedPassengerCount != nil {
			proposal.Booking.PassengerCount = *item.ExpectedPassengerCount
			proposal.Booking.PassengerCountKnown = true
		}
		if item.ExpectedChildUnder5Count != nil {
			proposal.Booking.ChildUnder5Count = *item.ExpectedChildUnder5Count
			proposal.Booking.ChildUnder5CountKnown = true
		}
		return proposal, true
	case string(IntentLapChildAssignmentAnswer):
		proposal := newStructuredInterpretation(StructuredIntentLapChildAssignmentAnswer, TurnMeaningAnswerToQuestion, 0.95, "interpreter_case_eval")
		index := 1
		if item.ExpectedSelectedOptionIndex != nil && *item.ExpectedSelectedOptionIndex > 0 {
			index = *item.ExpectedSelectedOptionIndex
		} else if parsed := activePromptLapChildAssignmentAnswerIndex(item.CurrentTurn); parsed > 0 {
			index = parsed
		}
		proposal.Booking.LapChildPassengerIndexes = []int{index}
		return proposal, true
	case string(IntentPassengerDocumentsProvided), string(IntentPaymentCreate):
		proposal := newStructuredInterpretation(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion, 0.95, "interpreter_case_eval")
		proposal.PassengerDocument.DocumentLikeText = true
		return proposal, true
	case string(IntentDocumentConfirmation):
		return newStructuredInterpretation(StructuredIntentDocumentConfirmation, TurnMeaningConfirmation, 0.95, "interpreter_case_eval"), true
	case string(IntentPaymentPreference):
		proposal := newStructuredInterpretation(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion, 0.95, "interpreter_case_eval")
		proposal.Payment.PaymentPreference = strings.TrimSpace(item.ExpectedPaymentPreference)
		if proposal.Payment.PaymentPreference == "" {
			proposal.Payment.PaymentPreference = strings.TrimSpace(item.CurrentTurn)
		}
		return proposal, true
	case string(IntentBookingCancel):
		return newStructuredInterpretation(StructuredIntentBookingCancelRequest, TurnMeaningNewRequest, 0.95, "interpreter_case_eval"), true
	default:
		return StructuredInterpretation{}, false
	}
}

func interpreterCaseSelectedOptionIndex(local StructuredInterpretation) int {
	if local.Booking.SelectedOptionIndexKnown {
		return local.Booking.SelectedOptionIndex
	}
	if len(local.Booking.LapChildPassengerIndexes) > 0 {
		return local.Booking.LapChildPassengerIndexes[0]
	}
	return 0
}

func interpreterCaseActivePromptFallbackTemplate(item InterpreterCase, fixture interpreterCaseFixture, validation InterpretationValidationResult) string {
	if validation.Accepted || validation.FallbackTemplate != "" {
		return ""
	}
	body := NormalizeIncomingCustomerText(item.CurrentTurn)
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if decision, ok := buildActivePromptContextualFallbackDecision(fixture.activePrompt, body, folded, fixture.state); ok {
		return string(decision.TemplateName)
	}
	return ""
}

func interpreterCaseEffectiveIntent(activePrompt ActivePromptContext, local StructuredInterpretation, validation InterpretationValidationResult, activePromptFallbackTemplate string) string {
	if validation.Accepted {
		return interpreterCaseCorpusIntent(local.Intent, activePrompt.Kind)
	}
	if validation.FallbackTemplate == TemplateUnsupportedPackage {
		return string(IntentUnsupportedPackage)
	}
	if validation.FallbackTemplate != "" || strings.TrimSpace(activePromptFallbackTemplate) != "" {
		return string(IntentUnknown)
	}
	if local.Intent == StructuredIntentBookingCancelRequest {
		return string(IntentBookingCancel)
	}
	return string(local.Intent)
}

func interpreterCaseCorpusIntent(intent StructuredIntent, activePrompt ActivePromptKind) string {
	switch intent {
	case StructuredIntentBookingCancelRequest:
		return string(IntentBookingCancel)
	case StructuredIntentPassengerDocumentsProvided:
		if activePrompt == ActivePromptPayerCPF {
			return string(IntentPaymentCreate)
		}
		return string(IntentPassengerDocumentsProvided)
	default:
		return string(intent)
	}
}

func interpreterCaseEffectiveTemplate(item InterpreterCase, activePrompt ActivePromptContext, local StructuredInterpretation, validation InterpretationValidationResult, activePromptFallbackTemplate string, effectiveIntent string) string {
	if validation.FallbackTemplate != "" {
		return string(validation.FallbackTemplate)
	}
	if strings.TrimSpace(activePromptFallbackTemplate) != "" {
		return strings.TrimSpace(activePromptFallbackTemplate)
	}
	if effectiveIntent == string(IntentAvailabilitySearch) &&
		local.TurnMeaning == TurnMeaningNewRequest &&
		looksLikeReservationHowToProceedIntent(item.CurrentTurn) {
		return string(TemplateAskReservationRouteSC)
	}
	if effectiveIntent == string(IntentSelectAvailabilityOption) && validation.Accepted && activePrompt.Kind == ActivePromptAvailabilityOptionChoice {
		return string(TemplateAskPassengerCount)
	}
	return ""
}

func interpreterCaseEffectiveAction(activePrompt ActivePromptContext, validation InterpretationValidationResult, effectiveIntent string, effectiveTemplate string) string {
	if validation.Accepted {
		if effectiveIntent == string(IntentPaymentCreate) {
			return "tool"
		}
		if effectiveIntent == string(IntentAvailabilitySearch) && activePrompt.Kind == ActivePromptAvailabilityDateChoice {
			return "tool"
		}
	}
	if effectiveTemplate != "" {
		return "template"
	}
	return ""
}

func interpreterCaseEvaluationFailures(item InterpreterCase, result InterpreterCaseEvaluationResult, local StructuredInterpretation, validation InterpretationValidationResult) []string {
	var failures []string
	if item.ExpectedIntent != "" && result.EffectiveIntent != item.ExpectedIntent {
		failures = append(failures, fmt.Sprintf("expected effective intent %s, got %s (local %s)", item.ExpectedIntent, result.EffectiveIntent, result.LocalIntent))
	}
	if item.ExpectedTurnMeaning != "" && result.LocalTurnMeaning != item.ExpectedTurnMeaning {
		failures = append(failures, fmt.Sprintf("expected local turn meaning %s, got %s", item.ExpectedTurnMeaning, result.LocalTurnMeaning))
	}
	if item.ExpectedTemplate != "" && result.EffectiveTemplate != item.ExpectedTemplate {
		failures = append(failures, fmt.Sprintf("expected effective template %s, got %s", item.ExpectedTemplate, result.EffectiveTemplate))
	}
	if item.ExpectedAction != "" && result.EffectiveAction != item.ExpectedAction {
		failures = append(failures, fmt.Sprintf("expected effective action %s, got %s", item.ExpectedAction, result.EffectiveAction))
	}
	if item.ExpectedSelectedOptionIndex != nil && result.LocalSelectedOptionIndex != *item.ExpectedSelectedOptionIndex {
		failures = append(failures, fmt.Sprintf("expected selected option index %d, got %d", *item.ExpectedSelectedOptionIndex, result.LocalSelectedOptionIndex))
	}
	if item.ExpectedPaymentPreference != "" && result.LocalPaymentPreference != item.ExpectedPaymentPreference {
		failures = append(failures, fmt.Sprintf("expected payment preference %s, got %s", item.ExpectedPaymentPreference, result.LocalPaymentPreference))
	}
	if item.ExpectedPassengerCount != nil {
		if !local.Booking.PassengerCountKnown || result.LocalPassengerCount != *item.ExpectedPassengerCount {
			failures = append(failures, fmt.Sprintf("expected passenger count %d, got %d known=%t", *item.ExpectedPassengerCount, result.LocalPassengerCount, local.Booking.PassengerCountKnown))
		}
	}
	if item.ExpectedChildUnder5Count != nil {
		if !local.Booking.ChildUnder5CountKnown || result.LocalChildUnder5Count != *item.ExpectedChildUnder5Count {
			failures = append(failures, fmt.Sprintf("expected child under 5 count %d, got %d known=%t", *item.ExpectedChildUnder5Count, result.LocalChildUnder5Count, local.Booking.ChildUnder5CountKnown))
		}
	}
	if item.FallbackTemplate != "" && interpreterCaseMainFallbackTemplate(result) != item.FallbackTemplate {
		failures = append(failures, fmt.Sprintf("expected main fallback %s, got %s", item.FallbackTemplate, interpreterCaseMainFallbackTemplate(result)))
	}
	for _, forbidden := range item.ForbiddenIntents {
		if result.EffectiveIntent == forbidden {
			failures = append(failures, fmt.Sprintf("forbidden effective intent %s", forbidden))
		}
		if validation.Accepted && interpreterCaseCorpusIntent(StructuredIntent(result.ValidatorProposalIntent), ActivePromptKind(strings.TrimSpace(item.ActivePromptKind))) == forbidden {
			failures = append(failures, fmt.Sprintf("validator accepted forbidden intent %s", forbidden))
		}
	}
	return failures
}

func interpreterCaseMainFallbackTemplate(result InterpreterCaseEvaluationResult) string {
	if strings.TrimSpace(result.ValidatorFallbackTemplate) != "" {
		return strings.TrimSpace(result.ValidatorFallbackTemplate)
	}
	return strings.TrimSpace(result.ActivePromptFallbackTemplate)
}
