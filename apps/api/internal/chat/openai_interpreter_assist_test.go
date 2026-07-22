package chat

import (
	"context"
	"encoding/json"
	"errors"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"testing"
	"time"
)

func TestOpenAIInterpreterAssistDeterministicDecisionWins(t *testing.T) {
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: assistTestInterpretation(StructuredIntentAvailabilitySearch),
		},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:               true,
		OpenAIInterpreter:     fake,
		StructuredInput:       StructuredInterpreterInput{CurrentTurn: "1"},
		LocalInterpretation:   unknownStructuredInterpretation("test", 0),
		DeterministicDecision: IntentDecision{Intent: IntentSelectAvailabilityOption, Source: "deterministic"},
	})

	if fake.calls != 0 {
		t.Fatalf("expected deterministic decision to skip OpenAI, got calls=%d", fake.calls)
	}
	if got.Status != OpenAIInterpreterAssistSkipped || got.Considered {
		t.Fatalf("expected skipped/not considered, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistLocalUnknownAcceptsAvailabilityProposal(t *testing.T) {
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     assistTestInterpretation(StructuredIntentAvailabilitySearch),
			ProviderResponseID: "resp_assist_accept",
		},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "como faço pra fazer uma reserva?", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
		IdempotencyKey:      "assist-accept",
	})

	if fake.calls != 1 {
		t.Fatalf("expected one OpenAI call, got %d", fake.calls)
	}
	if got.Status != OpenAIInterpreterAssistAccepted || !got.Accepted {
		t.Fatalf("expected accepted assist, got %+v", got)
	}
	if got.IntentDecision.Intent != IntentAvailabilitySearch ||
		got.IntentDecision.TemplateName != TemplateAskReservationRouteSC ||
		got.IntentDecision.Action != "template" {
		t.Fatalf("unexpected runtime decision: %+v", got.IntentDecision)
	}
	metadata := got.Metadata()
	if asString(metadata["openai_assist_status"]) != string(OpenAIInterpreterAssistAccepted) {
		t.Fatalf("expected accepted metadata, got %+v", metadata)
	}
}

func TestOpenAIInterpreterAssistLowLocalConfidenceAllowsOpenAI(t *testing.T) {
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: assistTestInterpretation(StructuredIntentAvailabilitySearch),
		},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:           true,
		OpenAIInterpreter: fake,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "como faço pra fazer uma reserva?",
			ObservedAt:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
		LocalInterpretation: StructuredInterpretation{
			Intent:      StructuredIntentGreeting,
			TurnMeaning: TurnMeaningGreeting,
			Confidence:  0.42,
			Source:      "test_low_confidence",
		},
	})

	if fake.calls != 1 {
		t.Fatalf("expected low-confidence local result to call OpenAI, got %d", fake.calls)
	}
	if got.Status != OpenAIInterpreterAssistAccepted {
		t.Fatalf("expected accepted assist, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistLowConfidenceRejectsBeforeValidationWithoutFallback(t *testing.T) {
	proposal := assistTestInterpretation(StructuredIntentSelectAvailabilityOption)
	proposal.Confidence = 0.2
	proposal.Booking.SelectedOptionIndexKnown = true
	proposal.Booking.SelectedOptionIndex = 1
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_low_confidence",
		},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:           true,
		OpenAIInterpreter: fake,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "talvez",
			ObservedAt:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})

	if got.Status != OpenAIInterpreterAssistRejected ||
		got.ValidationStatus != OpenAIInterpreterAssistValidationSkipped ||
		got.RejectReason != "openai_confidence_below_threshold" ||
		got.FallbackTemplate != "" {
		t.Fatalf("expected low confidence rejection without fallback, got %+v", got)
	}
	metadata := got.Metadata()
	if _, ok := metadata["fallback_template"]; ok {
		t.Fatalf("low confidence metadata must not include fallback template: %+v", metadata)
	}
}

func TestOpenAIInterpreterAssistValidatorRejectsPixPaymentPreference(t *testing.T) {
	history := []Message{{
		Direction: "OUTBOUND",
		Body:      askPaymentChoiceReply,
	}}
	proposal := assistTestInterpretation(StructuredIntentPaymentPreference)
	proposal.Payment.PaymentPreference = "sinal"
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result:  OpenAIStructuredInterpreterRunResult{Interpretation: proposal},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:           true,
		OpenAIInterpreter: fake,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "pix",
			History:     history,
			State:       CanonicalConversationState{Phase: ConversationPhaseBooked},
			ObservedAt:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
		ActivePrompt:        InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseBooked}),
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})

	if got.Status != OpenAIInterpreterAssistRejected ||
		got.ValidationStatus != OpenAIInterpreterAssistValidationRejected ||
		got.RejectReason != "pix_is_method_not_payment_preference" ||
		got.FallbackTemplate != TemplateContextFallbackPaymentPreference {
		t.Fatalf("expected payment preference rejection, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistRejectsSafetySideEffectClaim(t *testing.T) {
	proposal := assistTestInterpretation(StructuredIntentAvailabilitySearch)
	proposal.Safety.ExecutesTool = true
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Validation: OpenAIStructuredInterpretationValidationResult{Interpretation: proposal},
		},
		err: ErrOpenAIStructuredInterpreterInvalidOutput,
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "como faço pra fazer uma reserva?", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})

	if got.Status != OpenAIInterpreterAssistRejected ||
		got.ValidationStatus != OpenAIInterpreterAssistValidationRejected ||
		got.RejectReason != "unsafe_side_effect_claim" {
		t.Fatalf("expected safety rejection, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistBlocksDangerousToolIntent(t *testing.T) {
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: assistTestInterpretation(StructuredIntentBookingCancelRequest),
		},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "talvez", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})

	if got.Status != OpenAIInterpreterAssistRejected ||
		got.ValidationStatus != OpenAIInterpreterAssistValidationAccepted ||
		got.RejectReason != "booking_cancel_not_allowed_in_openai_assist" {
		t.Fatalf("expected dangerous intent block, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistBlocksAvailabilityToolAction(t *testing.T) {
	proposal := assistTestInterpretation(StructuredIntentAvailabilitySearch)
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result:  OpenAIStructuredInterpreterRunResult{Interpretation: proposal},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:           true,
		OpenAIInterpreter: fake,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "Fraiburgo para Monção 18/05",
			ObservedAt:  time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})

	if got.Status != OpenAIInterpreterAssistRejected ||
		got.ValidationStatus != OpenAIInterpreterAssistValidationAccepted ||
		got.RejectReason != "openai_assist_tool_action_not_allowed" ||
		got.IntentDecision.Action == "tool" {
		t.Fatalf("expected availability tool action block, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistOpenAIErrorIsSkipped(t *testing.T) {
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		err:     errors.New("provider unavailable"),
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "talvez"},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})

	if got.Status != OpenAIInterpreterAssistSkipped ||
		got.RejectReason != "openai_error" ||
		!got.Considered {
		t.Fatalf("expected skipped OpenAI error, got %+v", got)
	}
}

func TestOpenAIInterpreterAssistMetadataDoesNotPersistRawOpenAIReasons(t *testing.T) {
	proposal := assistTestInterpretation(StructuredIntentBookingCancelRequest)
	proposal.Reasons = []string{
		"cliente 5549999999999 CPF 52998224725 PIX 00020126580014br.gov.bcb.pix",
	}
	fake := &fakeOpenAIInterpreter{
		enabled: true,
		result:  OpenAIStructuredInterpreterRunResult{Interpretation: proposal},
	}

	got := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:             true,
		OpenAIInterpreter:   fake,
		StructuredInput:     StructuredInterpreterInput{CurrentTurn: "talvez", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
	})
	metadata := got.Metadata()
	for _, rawField := range []string{"local_result", "openai_result", "provider_response_id"} {
		if _, ok := metadata[rawField]; ok {
			t.Fatalf("metadata should not persist raw field %s: %+v", rawField, metadata)
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"5549999999999", "52998224725", "00020126580014br.gov.bcb.pix"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("metadata leaked raw OpenAI reason %q in %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "booking_cancel_not_allowed_in_openai_assist") {
		t.Fatalf("expected controlled reason code in metadata, got %s", text)
	}
}

func TestReprocessOpenAIInterpreterAssistRejectedProposalUsesFallbackMetadata(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	proposal := assistTestInterpretation(StructuredIntentSelectAvailabilityOption)
	proposal.Booking.SelectedOptionIndex = 1
	proposal.Booking.SelectedOptionIndexKnown = true
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_assist_rejected",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterAssistEnabled: true,
	}, runner, openAI)
	session, _ := store.seedSessionWithMessage("5511990003601", "talvez")

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected fallback template to avoid free-form runner, got calls=%d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft")
	}
	if !strings.Contains(out.Draft.Body, "Não consegui identificar qual opção") {
		t.Fatalf("expected availability option fallback, got %q", out.Draft.Body)
	}
	metadata := asMap(out.Draft.NormalizedPayload[openAIInterpreterAssistMetadataKey])
	if asString(metadata["openai_assist_status"]) != string(OpenAIInterpreterAssistRejected) {
		t.Fatalf("expected rejected assist metadata, got %+v", metadata)
	}
	if asString(metadata["validation_status"]) != string(OpenAIInterpreterAssistValidationRejected) {
		t.Fatalf("expected rejected validation metadata, got %+v", metadata)
	}
	if asString(metadata["reject_reason"]) != "active_prompt_required" {
		t.Fatalf("expected active prompt reject reason, got %+v", metadata)
	}
	if asString(metadata["fallback_template"]) != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("expected fallback template metadata, got %+v", metadata)
	}
}

func TestReprocessOpenAIInterpreterAssistLowConfidenceDoesNotUseProposalFallback(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	proposal := assistTestInterpretation(StructuredIntentSelectAvailabilityOption)
	proposal.Confidence = 0.2
	proposal.Booking.SelectedOptionIndex = 1
	proposal.Booking.SelectedOptionIndexKnown = true
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_assist_low_confidence",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterAssistEnabled: true,
	}, runner, openAI)
	session, _ := store.seedSessionWithMessage("5511990003605", "talvez")

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("expected local fallback runner after low-confidence rejection, got calls=%d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft")
	}
	if strings.Contains(out.Draft.Body, "Não consegui identificar qual opção") {
		t.Fatalf("low-confidence OpenAI proposal must not select availability fallback, got %q", out.Draft.Body)
	}
	metadata := asMap(out.Draft.NormalizedPayload[openAIInterpreterAssistMetadataKey])
	if asString(metadata["openai_assist_status"]) != string(OpenAIInterpreterAssistRejected) ||
		asString(metadata["reject_reason"]) != "openai_confidence_below_threshold" ||
		asString(metadata["fallback_template"]) != "" {
		t.Fatalf("expected low-confidence rejected metadata without fallback, got %+v", metadata)
	}
}

func TestReprocessOpenAIInterpreterShadowAndAssistReuseSingleProviderCall(t *testing.T) {
	t.Setenv("CHAT_INTENT_ROUTER_ENABLED", "false")
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	proposal := assistTestInterpretation(StructuredIntentSelectAvailabilityOption)
	proposal.Booking.SelectedOptionIndex = 1
	proposal.Booking.SelectedOptionIndexKnown = true
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_shadow_assist_once",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAIInterpreterAssistEnabled: true,
	}, runner, openAI)
	session, _ := store.seedSessionWithMessage("5511990003606", "talvez")

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if openAI.calls != 1 {
		t.Fatalf("expected shadow+assist to reuse one OpenAI call, got calls=%d", openAI.calls)
	}

	shadow := mustStructuredInterpreterShadowMap(t, out.Draft.NormalizedPayload[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["provider_response_id"]); got != "resp_shadow_assist_once" {
		t.Fatalf("expected shadow provider response id from single call, got %q", got)
	}
	assist := asMap(out.Draft.NormalizedPayload[openAIInterpreterAssistMetadataKey])
	if asString(assist["openai_assist_status"]) != string(OpenAIInterpreterAssistRejected) {
		t.Fatalf("expected rejected assist metadata, got %+v", assist)
	}
	if asString(assist["openai_intent"]) != asString(openAIShadow["intent"]) ||
		assist["openai_confidence"] != openAIShadow["confidence"] {
		t.Fatalf("expected assist metadata to reuse shadow structured result, shadow=%+v assist=%+v", openAIShadow, assist)
	}
	validation := mustNestedMap(t, shadow, "openai_validation")
	if asString(assist["reject_reason"]) != asString(validation["reject_reason"]) ||
		asString(assist["fallback_template"]) != asString(validation["fallback_template"]) {
		t.Fatalf("expected assist decision to match shadow validation, validation=%+v assist=%+v", validation, assist)
	}
}

func TestOpenAIInterpreterAssistSelectionTemplateDraftRequiresAtomicAttach(t *testing.T) {
	observedAt := availabilityTestObservedAt()
	proposal := assistTestInterpretation(StructuredIntentSelectAvailabilityOption)
	proposal.Booking.SelectedOptionIndex = 2
	proposal.Booking.SelectedOptionIndexKnown = true
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_assist_select_option",
		},
	}
	temporalFixture := availabilityOptionPromptTodayAndFutureResultAt(observedAt)
	pastOption := availabilityOptionPromptItemAt(temporalFixture.Results[0], availabilityTestDate(observedAt, -1), "08:00")
	visibleOptions := futureAvailabilityOptions(
		[]AvailabilitySearchItem{pastOption, temporalFixture.Results[0], temporalFixture.Results[1]},
		observedAt,
	)
	if len(visibleOptions) != 2 ||
		visibleOptions[0].TripID != temporalFixture.Results[0].TripID ||
		visibleOptions[1].TripID != temporalFixture.Results[1].TripID {
		t.Fatalf("expected explicit past, today and future fixture semantics, got %+v", visibleOptions)
	}
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(observedAt)
	selectedOption := availability.Results[1]
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             buildAvailabilityListReply(availability),
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		ReceivedAt: observedAt,
	}}
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "2")
	activePrompt := InferActivePromptContext(history, state)

	assist := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:           true,
		OpenAIInterpreter: openAI,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "2",
			History:     history,
			State:       state,
			ObservedAt:  observedAt,
		},
		ActivePrompt:        activePrompt,
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
		IdempotencyKey:      "assist-select-option",
	})
	if assist.Status != OpenAIInterpreterAssistAccepted {
		t.Fatalf("expected accepted assist selection, got %+v", assist)
	}
	reply, ok := realizeIntentResponseTemplate(assist.IntentDecision)
	if !ok || strings.TrimSpace(reply) != askPassengerCountReply {
		t.Fatalf("expected passenger count template, ok=%t reply=%q decision=%+v", ok, reply, assist.IntentDecision)
	}
	run := buildTemplateDraftRunFromDecision(assist.IntentDecision, reply)
	if got := asInt(run.RequestPayload["selected_option_index"]); got != 0 {
		t.Fatalf("expected assist template builder not to persist metadata-only selected_option_index, got %d payload=%+v", got, run.RequestPayload)
	}
	run = attachSelectedAvailabilityResultToTemplateRun(run, currentAvailabilitySelectionPromptAvailabilityContextAt(history, observedAt), assist.IntentDecision)
	if got := asInt(run.RequestPayload["selected_option_index"]); got != 2 {
		t.Fatalf("expected atomic attach to persist selected_option_index=2, got %d payload=%+v", got, run.RequestPayload)
	}
	snapshot := asMap(run.RequestPayload[selectedAvailabilityResultPayloadKey])
	if got := strings.TrimSpace(asString(snapshot["trip_id"])); got != selectedOption.TripID {
		t.Fatalf("expected selected snapshot %s, got %q snapshot=%+v", selectedOption.TripID, got, snapshot)
	}
	if got := asInt(snapshot["selected_option_index"]); got != 2 {
		t.Fatalf("expected selected snapshot index 2, got %d snapshot=%+v", got, snapshot)
	}

	payload, normalizedPayload := buildAgentDraftPayload(
		Session{ID: "session-1", Channel: "WHATSAPP", ContactKey: "5511999999999"},
		nil,
		"draft-assist-select-option",
		"",
		"",
		run,
		agentToolContext{Availability: &availability},
		draftAutoSendPolicy{Status: draftAutoSendStatusEligible},
		observedAt,
	)
	history = append(history,
		Message{
			Direction:         "OUTBOUND",
			Body:              run.ReplyText,
			ProcessingStatus:  messageStatusAutomationSent,
			Payload:           payload,
			NormalizedPayload: normalizedPayload,
			ReceivedAt:        observedAt.Add(time.Minute),
		},
		Message{
			Direction:        "INBOUND",
			Body:             "só pra mim",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       observedAt.Add(2 * time.Minute),
		},
	)
	bookingDraft := collectBookingDraftContext(
		sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0)),
		history,
		"",
	)
	if bookingDraft.SelectedOptionIndex != 2 ||
		bookingDraft.TripID != selectedOption.TripID ||
		bookingDraft.BoardStopID != selectedOption.BoardStopID ||
		bookingDraft.AlightStopID != selectedOption.AlightStopID ||
		bookingDraft.TripDate != selectedOption.TripDate ||
		bookingDraft.PassengerCount != 1 ||
		!bookingDraft.PassengerCountKnown {
		t.Fatalf("expected passenger turn to keep assist-selected trip facts, got %+v", bookingDraft)
	}
}

func TestOpenAIInterpreterAssistSelectionWithoutAvailabilityFactsIsRejected(t *testing.T) {
	observedAt := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	proposal := assistTestInterpretation(StructuredIntentSelectAvailabilityOption)
	proposal.Booking.SelectedOptionIndex = 2
	proposal.Booking.SelectedOptionIndexKnown = true
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_assist_select_option_without_facts",
		},
	}
	availability := availabilityOptionPromptTwoOptionsFutureResult()
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             buildAvailabilityListReply(availability),
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       observedAt,
	}}
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "2")
	activePrompt := InferActivePromptContext(history, state)

	assist := RunOpenAIInterpreterRuntimeAssist(context.Background(), OpenAIInterpreterAssistInput{
		Enabled:           true,
		OpenAIInterpreter: openAI,
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: "2",
			History:     history,
			State:       state,
			ObservedAt:  observedAt,
		},
		ActivePrompt:        activePrompt,
		LocalInterpretation: unknownStructuredInterpretation("test", 0),
		IdempotencyKey:      "assist-select-option-without-facts",
	})
	if assist.Status != OpenAIInterpreterAssistRejected {
		t.Fatalf("expected assist selection without availability facts to be rejected, got %+v", assist)
	}
	if assist.RejectReason != "availability_selection_missing_current_facts" {
		t.Fatalf("expected missing facts rejection, got %q decision=%+v", assist.RejectReason, assist)
	}
}

func TestReprocessOpenAIInterpreterAssistAvailabilityProposalDoesNotRunTools(t *testing.T) {
	t.Setenv("CHAT_INTENT_ROUTER_ENABLED", "false")
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	availability := &fakeAvailabilitySearcher{enabled: true}
	pricing := &fakePricingQuoteSearcher{enabled: true}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     assistTestInterpretation(StructuredIntentAvailabilitySearch),
			ProviderResponseID: "resp_assist_tool_block",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterAssistEnabled: true,
	}, runner, availability, pricing, openAI)
	session, _ := store.seedSessionWithMessage("5511990003602", "Fraiburgo para Monção 18/05")

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if availability.calls != 0 {
		t.Fatalf("expected availability not to run from OpenAI assist, got %d calls", availability.calls)
	}
	if pricing.calls != 0 {
		t.Fatalf("expected pricing not to run from OpenAI assist, got %d calls", pricing.calls)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", out.ToolCalls)
	}
	if out.Draft == nil {
		t.Fatalf("expected fallback draft")
	}
	metadata := asMap(out.Draft.NormalizedPayload[openAIInterpreterAssistMetadataKey])
	if asString(metadata["openai_assist_status"]) != string(OpenAIInterpreterAssistRejected) ||
		asString(metadata["reject_reason"]) != "openai_assist_tool_action_not_allowed" {
		t.Fatalf("expected tool block metadata, got %+v", metadata)
	}
}

func TestReprocessOpenAIInterpreterAssistSkippedMetadataWhenDisabled(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, openAI)
	session, _ := store.seedSessionWithMessage("5511990003603", "talvez")

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if openAI.calls != 0 {
		t.Fatalf("expected disabled assist not to call OpenAI, got %d", openAI.calls)
	}
	metadata := asMap(out.Draft.NormalizedPayload[openAIInterpreterAssistMetadataKey])
	if asString(metadata["openai_assist_status"]) != string(OpenAIInterpreterAssistSkipped) ||
		asString(metadata["reject_reason"]) != "assist_disabled" ||
		asString(metadata["skip_reason"]) != "assist_disabled" ||
		metadata["accepted"] == true {
		t.Fatalf("expected skipped disabled metadata, got %+v", metadata)
	}
}

func TestReprocessOpenAIInterpreterAssistSkippedMetadataWhenLocalNotEligible(t *testing.T) {
	t.Setenv("CHAT_INTENT_ROUTER_ENABLED", "false")
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterAssistEnabled: true,
	}, runner, openAI)
	session, _ := store.seedSessionWithMessage("5511990003604", "oi")

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if openAI.calls != 0 {
		t.Fatalf("expected local-not-eligible skip not to call OpenAI, got %d", openAI.calls)
	}
	metadata := asMap(out.Draft.NormalizedPayload[openAIInterpreterAssistMetadataKey])
	if asString(metadata["openai_assist_status"]) != string(OpenAIInterpreterAssistSkipped) ||
		asString(metadata["reject_reason"]) != "local_result_not_eligible" ||
		asString(metadata["skip_reason"]) != "local_result_not_eligible" ||
		asString(metadata["local_intent"]) != string(StructuredIntentGreeting) ||
		metadata["accepted"] == true {
		t.Fatalf("expected skipped local-not-eligible metadata, got %+v", metadata)
	}
}

func assistTestInterpretation(intent StructuredIntent) StructuredInterpretation {
	meaning := TurnMeaningAnswerToQuestion
	if intent == StructuredIntentAvailabilitySearch || intent == StructuredIntentBookingCancelRequest {
		meaning = TurnMeaningNewRequest
	}
	return StructuredInterpretation{
		Intent:      intent,
		TurnMeaning: meaning,
		Confidence:  0.91,
		Source:      "openai_structured",
	}
}
