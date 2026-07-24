package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"schumacher-tur/api/internal/shared/config"
)

func TestPassengerStateFoundationDoesNotAddLexicalFamilyRules(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob chat package: %v", err)
	}
	regexpInventory := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		contents, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("read %s: %v", file, readErr)
		}
		regexpInventory += strings.Count(string(contents), "regexp.MustCompile")
	}
	if regexpInventory != 54 {
		t.Fatalf("passenger foundation changed regexp.MustCompile inventory: got %d want 54", regexpInventory)
	}

	for _, file := range []string{"passenger_clarification_evidence.go", "passenger_clarification_reducer.go"} {
		contents, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("read %s: %v", file, readErr)
		}
		foundation := string(contents)
		for _, forbidden := range []string{"regexp.", "containsAnyFolded", "CorrectionCue", "meus filhos", "minha filha", "meu filho"} {
			if strings.Contains(foundation, forbidden) {
				t.Fatalf("%s contains forbidden lexical foundation rule %q", file, forbidden)
			}
		}
	}
}

func TestPassengerEvidenceStructuralFieldsRejectHasEvidenceFalse(t *testing.T) {
	fresh := newPassengerClarificationStateV1()
	fresh.BootstrapCompleted = true
	if err := validatePassengerClarificationStateV1(fresh); err != nil {
		t.Fatalf("literal fresh UNKNOWN with HasEvidence=false must remain valid: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*PassengerClarificationStateV1)
	}{
		{
			name: "passenger_prompt_message_id",
			mutate: func(state *PassengerClarificationStateV1) {
				state.PassengerPromptMessageID = "passenger-prompt"
			},
		},
		{
			name: "child_prompt_message_id",
			mutate: func(state *PassengerClarificationStateV1) {
				state.ChildPromptMessageID = "child-prompt"
			},
		},
		{
			name: "passenger_last_message_id",
			mutate: func(state *PassengerClarificationStateV1) {
				state.PassengerLastMessageID = "passenger-answer"
			},
		},
		{
			name: "child_last_message_id",
			mutate: func(state *PassengerClarificationStateV1) {
				state.ChildLastMessageID = "child-answer"
			},
		},
		{
			name: "applied_event_ids",
			mutate: func(state *PassengerClarificationStateV1) {
				state.AppliedEventIDs = []string{"event-1"}
			},
		},
		{
			name: "applied_message_ids",
			mutate: func(state *PassengerClarificationStateV1) {
				state.AppliedMessageIDs = []string{"message-1"}
			},
		},
		{
			name: "passenger_slot_open",
			mutate: func(state *PassengerClarificationStateV1) {
				state.PassengerSlotStatus = PassengerClarificationSlotOpen
				state.PassengerPromptMessageID = "passenger-prompt"
			},
		},
		{
			name: "child_slot_open",
			mutate: func(state *PassengerClarificationStateV1) {
				state.ChildSlotStatus = PassengerClarificationSlotOpen
				state.ChildPromptMessageID = "child-prompt"
			},
		},
		{
			name: "passenger_slot_answered_and_count_known",
			mutate: func(state *PassengerClarificationStateV1) {
				state.PassengerSlotStatus = PassengerClarificationSlotAnswered
				state.PassengerCountKnown = true
				state.PassengerCount = 1
				state.PassengerCountProvenance = PassengerCountProvenanceAbsoluteTotal
			},
		},
		{
			name: "child_slot_answered_and_count_known",
			mutate: func(state *PassengerClarificationStateV1) {
				state.ChildSlotStatus = PassengerClarificationSlotAnswered
				state.ChildUnder5CountKnown = true
			},
		},
		{
			name: "passenger_slot_conflicting",
			mutate: func(state *PassengerClarificationStateV1) {
				state.PassengerSlotStatus = PassengerClarificationSlotConflicting
			},
		},
		{
			name: "child_slot_conflicting",
			mutate: func(state *PassengerClarificationStateV1) {
				state.ChildSlotStatus = PassengerClarificationSlotConflicting
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := fresh
			test.mutate(&state)
			if err := validatePassengerClarificationStateV1(state); err == nil {
				t.Fatalf("HasEvidence=false must reject structural passenger evidence: %+v", state)
			}
		})
	}
}

func TestPassengerStateBootstrapUsesStructuredEvidenceOnly(t *testing.T) {
	rawOnly := Message{
		ID: "raw-only", Direction: "INBOUND", Body: "somos 99 e uma crianca",
		Payload: map[string]interface{}{
			passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{{
				Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
				Value: 99, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
			}},
		},
	}
	unknown := bootstrapPassengerClarificationStateV1(nil, nil, passengerClarificationStructuredEventsV1([]Message{rawOnly}))
	if !unknown.BootstrapCompleted || unknown.PassengerCountKnown || unknown.ChildUnder5CountKnown || unknown.HasEvidence {
		t.Fatalf("raw body/payload must bootstrap UNKNOWN once, got %+v", unknown)
	}

	structured := rawOnly
	structured.ID = "structured"
	structured.NormalizedPayload = map[string]interface{}{
		passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{
			{
				Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
				Value: 2, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
			},
			{
				Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
				Value: 0, ValueKnown: true,
			},
		},
	}
	bootstrapped := bootstrapPassengerClarificationStateV1(nil, nil, passengerClarificationStructuredEventsV1([]Message{structured}))
	if !bootstrapped.PassengerCountKnown || bootstrapped.PassengerCount != 2 ||
		!bootstrapped.ChildUnder5CountKnown || bootstrapped.ChildUnder5Count != 0 {
		t.Fatalf("canonical normalized events must bootstrap state, got %+v", bootstrapped)
	}

	booking := &PassengerClarificationBookingAuthorityV1{BookingID: uuid.NewString(), PassengerCount: 3, LapChildCount: 1}
	postBooking := bootstrapPassengerClarificationStateV1(nil, booking, passengerClarificationStructuredEventsV1([]Message{structured}))
	if postBooking.Authority != PassengerClarificationAuthorityPostBooking || postBooking.BookingID != booking.BookingID ||
		postBooking.PassengerCount != 3 || postBooking.ChildUnder5Count != 1 {
		t.Fatalf("persisted booking authority must win bootstrap precedence, got %+v", postBooking)
	}
}

func TestPassengerBootstrapStructuredEventOutsideReprocessWindow(t *testing.T) {
	base := newFakeStore()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{Channel: "WHATSAPP", ContactKey: uuid.NewString()})
	if err != nil {
		t.Fatalf("create bootstrap session: %v", err)
	}
	if _, err := base.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "old structured authority",
		ProcessingStatus: "AUTOMATION_PROCESSED", ReceivedAt: time.Now().UTC().Add(-time.Hour),
		NormalizedPayload: map[string]interface{}{
			passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{
				{Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger, Value: 3, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal},
				{Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild, Value: 1, ValueKnown: true},
			},
		},
	}); err != nil {
		t.Fatalf("create old structured event: %v", err)
	}
	for i := 0; i < 55; i++ {
		if _, err := base.CreateMessage(context.Background(), CreateMessageInput{
			SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: "window filler",
			ProcessingStatus: "SENT", ReceivedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("create filler %d: %v", i, err)
		}
	}
	if _, err := base.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "current turn",
		ProcessingStatus: "BUFFERED_PENDING", ReceivedAt: time.Now().UTC().Add(time.Minute),
	}); err != nil {
		t.Fatalf("create current turn: %v", err)
	}

	store := &windowedPassengerBootstrapStore{fakeStore: base}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{enabled: false})
	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID}); err != nil {
		t.Fatalf("reprocess with truncated window: %v", err)
	}
	state, ok := passengerClarificationStateV1FromSession(base.sessions[session.ID])
	if !ok || state.PassengerCount != 3 || !state.PassengerCountKnown || state.ChildUnder5Count != 1 || !state.ChildUnder5CountKnown {
		t.Fatalf("dedicated full-session bootstrap lost event outside latest 50: %+v", state)
	}
}

func TestPassengerStateFakeUnknownWithoutTranscriptParsing(t *testing.T) {
	store := newFakeStore()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{Channel: "WHATSAPP", ContactKey: uuid.NewString()})
	if err != nil {
		t.Fatalf("create fake UNKNOWN session: %v", err)
	}
	for _, message := range []CreateMessageInput{
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: askPassengerCountReply, ProcessingStatus: "SENT", ReceivedAt: time.Now().UTC()},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "somos 7 e duas criancas", ProcessingStatus: "BUFFERED_PENDING", ReceivedAt: time.Now().UTC()},
	} {
		if _, err := store.CreateMessage(context.Background(), message); err != nil {
			t.Fatalf("create raw transcript fixture: %v", err)
		}
	}
	result, err := store.ApplyPassengerClarificationEventsV1(context.Background(), ApplyPassengerClarificationEventsV1Input{SessionID: session.ID})
	if err != nil {
		t.Fatalf("bootstrap fake UNKNOWN: %v", err)
	}
	if !result.State.BootstrapCompleted || result.State.HasEvidence || result.State.PassengerCountKnown || result.State.ChildUnder5CountKnown {
		t.Fatalf("fake must not parse transcript or fabricate 1/0 authority: %+v", result.State)
	}
}

func TestPassengerPromptEventFollowsReviewedAndAutoSentOutboundBeyondHistoryWindow(t *testing.T) {
	t.Run("pending_and_failed_do_not_replay", func(t *testing.T) {
		event, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, "undelivered-prompt")
		if !ok {
			t.Fatal("build undelivered prompt event")
		}
		for _, status := range []string{"MANUAL_PENDING", "SEND_FAILED", "AUTOMATION_PENDING", "SENT"} {
			message := Message{
				ID: "undelivered-prompt", Direction: "OUTBOUND", ProcessingStatus: status,
				NormalizedPayload: map[string]interface{}{passengerPromptEventV1MessageKey: event},
			}
			if events := passengerClarificationStructuredEventsV1([]Message{message}); len(events) != 0 {
				t.Fatalf("status %s without canonical delivery marker replayed prompt: %+v", status, events)
			}
		}
	})

	t.Run("auto_send", func(t *testing.T) {
		store, session, draft := passengerPromptDeliveryFixture(t, PassengerClarificationSlotPassenger)
		reply, err := store.CreateAutomationReply(context.Background(), CreateAutomationReplyInput{
			SessionID: session.ID, DraftMessageID: draft.ID, IdempotencyKey: "auto-outbound",
		}, 0)
		if err != nil {
			t.Fatalf("create auto outbound: %v", err)
		}
		assertPassengerPromptCopiedToActualOutbound(t, draft, reply)
		delete(store.messages, draft.ID)
		store.messageOrder = removePassengerTestMessageID(store.messageOrder, draft.ID)

		markPassengerPromptDeliveredForTest(t, store, reply, "provider-auto")
		state, ok := passengerClarificationStateV1FromSession(store.sessions[session.ID])
		if !ok || state.PassengerPromptMessageID != reply.Message.ID || state.PassengerSlotStatus != PassengerClarificationSlotOpen {
			t.Fatalf("actual auto outbound must open the durable epoch beyond the draft window: %+v", state)
		}
		markPassengerPromptDeliveredForTest(t, store, reply, "provider-auto-retry")
		replayed, _ := passengerClarificationStateV1FromSession(store.sessions[session.ID])
		if !reflect.DeepEqual(state, replayed) {
			t.Fatalf("delivery retry must apply prompt event once: first=%+v retry=%+v", state, replayed)
		}
	})

	t.Run("manual_review", func(t *testing.T) {
		store, session, draft := passengerPromptDeliveryFixture(t, PassengerClarificationSlotChild)
		reply, err := store.CreateReply(context.Background(), ReplyInput{
			SessionID: session.ID, OwnerUserID: uuid.NewString(), DraftMessageID: draft.ID,
			Body: draft.Body, IdempotencyKey: "manual-reviewed-outbound",
		}, 0)
		if err != nil {
			t.Fatalf("create reviewed outbound: %v", err)
		}
		assertPassengerPromptCopiedToActualOutbound(t, draft, reply)
		markPassengerPromptDeliveredForTest(t, store, reply, "provider-manual")
		state, ok := passengerClarificationStateV1FromSession(store.sessions[session.ID])
		if !ok || state.ChildPromptMessageID != reply.Message.ID || state.ChildSlotStatus != PassengerClarificationSlotOpen {
			t.Fatalf("reviewed outbound must open the durable child epoch: %+v", state)
		}
	})

	t.Run("failure_then_retry", func(t *testing.T) {
		store, session, draft := passengerPromptDeliveryFixture(t, PassengerClarificationSlotPassenger)
		reply, err := store.CreateAutomationReply(context.Background(), CreateAutomationReplyInput{
			SessionID: session.ID, DraftMessageID: draft.ID, IdempotencyKey: "retry-outbound",
		}, 0)
		if err != nil {
			t.Fatalf("create retry outbound: %v", err)
		}
		if _, err := store.MarkReplyDeliveryFailure(context.Background(), MarkReplyDeliveryFailureInput{
			SessionID: session.ID, MessageID: reply.Message.ID, OutboundID: reply.Outbound.ID, ErrorText: "sanitized test failure",
		}); err != nil {
			t.Fatalf("mark delivery failure: %v", err)
		}
		before, _ := passengerClarificationStateV1FromSession(store.sessions[session.ID])
		if before.PassengerPromptMessageID != "" || before.PassengerSlotStatus == PassengerClarificationSlotOpen {
			t.Fatalf("failed/unsent outbound must not open an epoch: %+v", before)
		}
		markPassengerPromptDeliveredForTest(t, store, reply, "provider-retry")
		after, _ := passengerClarificationStateV1FromSession(store.sessions[session.ID])
		if after.PassengerPromptMessageID != reply.Message.ID || after.PassengerSlotStatus != PassengerClarificationSlotOpen {
			t.Fatalf("successful retry must use the original actual outbound event: %+v", after)
		}
	})
}

func TestPassengerChildAddsTravelerUsesPersistedPromptEpoch(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "solo-answer", Value: 1, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
		},
		{
			Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
			MessageID: "persisted-child-prompt",
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "child-answer", PromptMessageID: "persisted-child-prompt", Value: 1, ValueKnown: true,
		},
	})
	if !state.ChildUnder5AddsTraveler || state.ChildUnder5AddsTravelerOrigin.SourcePromptMessageID != "persisted-child-prompt" {
		t.Fatalf("matching persisted epoch must derive adds_traveler, got %+v", state)
	}
	before := state
	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
		MessageID: "stale-answer", PromptMessageID: "text-inferred-other-prompt", Value: 2, ValueKnown: true,
	}})
	if !reflect.DeepEqual(before, state) {
		t.Fatalf("stale/text-inferred prompt epoch must not alter state: before=%+v after=%+v", before, state)
	}
}

func TestPassengerGateAfterDeliveredPromptStopsExternalWorkBeforeDispatch(t *testing.T) {
	store := newFakeTravelQueryV2ShadowClaimStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	availability := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
		ChatAgentMode:                      chatAgentModeHybridJSON,
	}, runner, jsonRunner, openAI, travel, availability, booking, payment)

	session := seedPassengerCollectionPhase(t, store.fakeStore)
	passengerState, ok := passengerClarificationStateV1FromSession(store.fakeStore.sessions[session.ID])
	if !ok || !passengerState.HasEvidence || strings.TrimSpace(passengerState.PassengerPromptMessageID) == "" ||
		passengerState.PassengerCountKnown || passengerState.ChildUnder5CountKnown {
		t.Fatalf("expected delivered passenger prompt with unknown slots, got %+v exists=%t", passengerState, ok)
	}
	prompt := store.fakeStore.messages[passengerState.PassengerPromptMessageID]
	if !passengerPromptDeliveryConfirmedV1(prompt) {
		t.Fatalf("passenger gate fixture must use an effectively delivered prompt, got %+v", prompt)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction: "INBOUND", Kind: "IMAGE", ProviderMessageID: "unsafe-passenger-message",
			IdempotencyKey: "unsafe-passenger-message", Body: "nao sei informar",
			NormalizedPayload: map[string]interface{}{
				"image_url":       "https://files.example.test/unsafe-document.jpg",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest unsafe state turn: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess unsafe state: %v", err)
	}
	if out.Draft == nil || !strings.Contains(foldChatText(out.Draft.Body), "passagem e so para voce") {
		t.Fatalf("unsafe state must preserve the deterministic passenger clarification, got %+v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != safePhaseFallbackTemplateName {
		t.Fatalf("expected generic safe phase fallback, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if _, ok := out.Draft.NormalizedPayload[passengerPendingPromptEventV1MessageKey]; ok {
		t.Fatalf("generic safe phase fallback must not synthesize a passenger prompt event, got %+v", out.Draft.NormalizedPayload)
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		availability.calls != 0 || booking.calls != 0 || payment.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("unsafe state dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, availability.calls, booking.calls, payment.calls, out.ToolCalls)
	}
	select {
	case <-store.claimAttempts:
		t.Fatal("unsafe state scheduled a Travel V2 shadow claim")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestBookingDraftProjectionIgnoresBookingCreatePassengerCount(t *testing.T) {
	state := completePassengerStateForTest(3, 1)
	session := Session{Metadata: map[string]interface{}{"memory": map[string]interface{}{
		passengerClarificationStateV1MemoryKey: state,
	}}}
	history := []Message{{
		ID: "historical-booking-create", Direction: "OUTBOUND", Body: "somos 99",
		Payload: map[string]interface{}{"tool_context": map[string]interface{}{
			toolNameBookingCreate: map[string]interface{}{"passenger_count": 99, "booking_id": uuid.NewString()},
		}},
		NormalizedPayload: map[string]interface{}{"tool_context": map[string]interface{}{
			toolNameBookingCreate: map[string]interface{}{"passenger_count": 98},
		}},
	}}
	projection := collectBookingDraftContextFromState(session, history, "somos 97")
	if projection.PassengerCount != 3 || !projection.PassengerCountKnown || projection.ChildUnder5Count != 1 ||
		projection.ExpectedDocumentCount != 3 || projection.BookingCreated {
		t.Fatalf("BookingDraftContext must project only durable passenger authority, got %+v", projection)
	}
}

func TestBookingCreatedWithUnknownPassengerSlotsFailsClosed(t *testing.T) {
	unknown := newPassengerClarificationStateV1()
	unknown.BootstrapCompleted = true
	session := Session{Metadata: map[string]interface{}{"memory": map[string]interface{}{
		passengerClarificationStateV1MemoryKey: unknown,
	}}}
	history := []Message{{
		ID: "historical-booking", Direction: "OUTBOUND", ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{"tool_context": map[string]interface{}{
			toolNameBookingCreate: map[string]interface{}{"booking_id": uuid.NewString(), "passenger_count": 2},
		}},
	}}
	projection := collectBookingDraftContextFromState(session, history, "pagar agora")
	if projection.BookingCreated {
		t.Fatalf("historical tool_context must not become post-booking authority: %+v", projection)
	}
	if action := decideNextBookingStep(projection); action != BookingNextAskPassengerClarification {
		t.Fatalf("unknown mandatory slots must precede BookingCreated/payment, got %s context=%+v", action, projection)
	}
}

func TestPassengerAggregateCorrectionClearsDependentAddsTraveler(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "solo", Value: 1, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
		},
		{Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild, MessageID: "child-prompt"},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "child-one", PromptMessageID: "child-prompt", Value: 1, ValueKnown: true,
			ChildReferences: []PassengerClarificationChildReferenceV1{{ID: "child-1", Under5: true, AgeKnown: true}},
		},
	})
	if !state.ChildUnder5AddsTraveler || expectedPassengerDocumentCount(applyPassengerClarificationStateToBookingDraft(BookingDraftContext{}, state)) != 2 {
		t.Fatalf("test setup must contain solo plus standalone child, got %+v", state)
	}
	soloWithChild := state
	includesSpeaker := ReducePassengerClarificationEventsV1(soloWithChild, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventSlotCorrected, Slot: PassengerClarificationSlotPassenger,
		MessageID: "composition-correction", Value: 2, ValueKnown: true,
		PassengerProvenance: PassengerCountProvenanceIncludesSpeakerComposition,
	}})
	if includesSpeaker.PassengerCount != 2 || includesSpeaker.PassengerCountProvenance != PassengerCountProvenanceIncludesSpeakerComposition ||
		includesSpeaker.ChildUnder5AddsTraveler || includesSpeaker.ChildUnder5AddsTravelerOrigin != (PassengerClarificationAddsTravelerOriginV1{}) ||
		validatePassengerClarificationStateV1(includesSpeaker) != nil {
		t.Fatalf("every non-solo correction must clear dependent adds relation: %+v", includesSpeaker)
	}
	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventSlotCorrected, Slot: PassengerClarificationSlotPassenger,
		MessageID: "total-correction", Value: 2, ValueKnown: true,
		PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
	}})
	if state.PassengerCount != 2 || state.PassengerCountProvenance != PassengerCountProvenanceAbsoluteTotal ||
		state.ChildUnder5AddsTraveler || state.ChildUnder5AddsTravelerOrigin != (PassengerClarificationAddsTravelerOriginV1{}) ||
		expectedPassengerDocumentCount(applyPassengerClarificationStateToBookingDraft(BookingDraftContext{}, state)) != 2 {
		t.Fatalf("absolute-total correction must replace dependent adds relation, got %+v", state)
	}
	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventSlotCorrected, Slot: PassengerClarificationSlotChild,
		MessageID: "child-zero-correction", Value: 0, ValueKnown: true,
	}})
	if !state.ChildUnder5CountKnown || state.ChildUnder5Count != 0 || len(state.ChildReferences) != 0 ||
		state.ChildUnder5AddsTraveler || state.ChildUnder5AddsTravelerOrigin != (PassengerClarificationAddsTravelerOriginV1{}) {
		t.Fatalf("child-zero correction must replace the full child aggregate, got %+v", state)
	}
}

func TestPassengerStateConcurrentReprocessPreservesBothEvents(t *testing.T) {
	base := newFakeStore()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{Channel: "WHATSAPP", ContactKey: "5511999990002"})
	if err != nil {
		t.Fatalf("create concurrent test session: %v", err)
	}
	unknown := newPassengerClarificationStateV1()
	unknown.BootstrapCompleted = true
	seedPassengerClarificationStateV1ForTest(base, session.ID, unknown)

	passengerMessage := createPassengerStructuredEventMessageForTest(t, base, session.ID, "lane-passenger", PassengerClarificationEventV1{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		Value: 2, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
	})
	childMessage := createPassengerStructuredEventMessageForTest(t, base, session.ID, "lane-child", PassengerClarificationEventV1{
		Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
		Value: 0, ValueKnown: true,
	})
	store := newConcurrentPassengerReprocessStore(base, map[string]string{
		"passenger": passengerMessage.ID,
		"child":     childMessage.ID,
	})
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{enabled: false})

	type outcome struct{ err error }
	results := make(chan outcome, 2)
	for _, lane := range []string{"passenger", "child"} {
		lane := lane
		go func() {
			ctx := context.WithValue(context.Background(), passengerConcurrentLaneKey{}, lane)
			_, runErr := svc.Reprocess(ctx, ReprocessInput{SessionID: session.ID})
			results <- outcome{err: runErr}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-store.applyReady:
		case <-time.After(time.Second):
			t.Fatal("concurrent Reprocess did not reach serialized passenger apply")
		}
	}
	close(store.releaseApply)
	for i := 0; i < 2; i++ {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatalf("concurrent Reprocess failed: %v", got.err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent Reprocess timed out")
		}
	}
	finalSession, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload concurrent session: %v", err)
	}
	state, ok := passengerClarificationStateV1FromSession(finalSession)
	if !ok || !state.PassengerCountKnown || state.PassengerCount != 2 ||
		!state.ChildUnder5CountKnown || state.ChildUnder5Count != 0 ||
		!containsString(state.AppliedMessageIDs, passengerMessage.ID) || !containsString(state.AppliedMessageIDs, childMessage.ID) {
		t.Fatalf("concurrent Reprocess lost a passenger event: %+v", state)
	}
}

func TestPassengerMetadataConcurrentReprocessFailureRetryPreservesState(t *testing.T) {
	base := newFakeStore()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{Channel: "WHATSAPP", ContactKey: uuid.NewString()})
	if err != nil {
		t.Fatalf("create metadata concurrency session: %v", err)
	}
	unknown := newPassengerClarificationStateV1()
	unknown.BootstrapCompleted = true
	seedPassengerClarificationStateV1ForTest(base, session.ID, unknown)
	eventMessage := createPassengerStructuredEventMessageForTest(t, base, session.ID, "metadata", PassengerClarificationEventV1{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		Value: 2, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
	})
	draft, err := base.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID: session.ID, IdempotencyKey: "metadata-concurrent-draft", Body: "retry fixture",
		ProcessingStatus: messageStatusAutomationDraft, Payload: map[string]interface{}{}, NormalizedPayload: map[string]interface{}{},
		Agent: map[string]interface{}{"status": agentStatusDraftGenerated}, Buffer: map[string]interface{}{}, RecordedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create metadata concurrency draft: %v", err)
	}
	reply, err := base.CreateAutomationReply(context.Background(), CreateAutomationReplyInput{
		SessionID: session.ID, DraftMessageID: draft.Message.ID, IdempotencyKey: "metadata-concurrent-outbound",
	}, 0)
	if err != nil {
		t.Fatalf("create metadata concurrency outbound: %v", err)
	}

	store := newConcurrentPassengerReprocessStore(base, map[string]string{"metadata": eventMessage.ID})
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{enabled: false})
	reprocessDone := make(chan error, 1)
	go func() {
		ctx := context.WithValue(context.Background(), passengerConcurrentLaneKey{}, "metadata")
		_, runErr := svc.Reprocess(ctx, ReprocessInput{SessionID: session.ID})
		reprocessDone <- runErr
	}()
	select {
	case <-store.applyReady:
	case <-time.After(time.Second):
		t.Fatal("Reprocess did not reach passenger apply")
	}
	close(store.releaseApply)
	select {
	case <-store.applyDone:
	case <-time.After(time.Second):
		t.Fatal("Reprocess did not persist passenger event before metadata race")
	}

	metadataDone := make(chan error, 1)
	go func() {
		if _, failureErr := store.MarkReplyDeliveryFailure(context.Background(), MarkReplyDeliveryFailureInput{
			SessionID: session.ID, MessageID: reply.Message.ID, OutboundID: reply.Outbound.ID, ErrorText: "sanitized concurrent failure",
		}); failureErr != nil {
			metadataDone <- failureErr
			return
		}
		_, retryErr := store.UpdateDraftAutoSendState(context.Background(), UpdateDraftAutoSendStateInput{
			SessionID: session.ID, DraftMessageID: draft.Message.ID,
			AutoSendStatus: draftAutoSendStatusRetryPending,
			Agent:          map[string]interface{}{"status": draftAutoSendStatusRetryPending},
		})
		metadataDone <- retryErr
	}()
	for name, ch := range map[string]<-chan error{"reprocess": reprocessDone, "metadata": metadataDone} {
		select {
		case runErr := <-ch:
			if runErr != nil {
				t.Fatalf("%s concurrency path failed: %v", name, runErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s concurrency path timed out", name)
		}
	}

	finalSession, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload metadata concurrency session: %v", err)
	}
	state, ok := passengerClarificationStateV1FromSession(finalSession)
	if !ok || !state.PassengerCountKnown || state.PassengerCount != 2 || !containsString(state.AppliedMessageIDs, eventMessage.ID) {
		t.Fatalf("failure/retry metadata writer lost passenger snapshot: %+v", state)
	}
	if status := asString(asMap(finalSession.Metadata["agent"])["status"]); status == "" {
		t.Fatalf("concurrent metadata update was lost: %+v", finalSession.Metadata)
	}
}

func TestPassengerStateRestartAndTruncatedWindowUseDurableSnapshot(t *testing.T) {
	state := completePassengerStateForTest(3, 1)
	decoded, ok := decodePassengerClarificationStateV1(state)
	if !ok || !reflect.DeepEqual(state, decoded) {
		t.Fatalf("restart round-trip changed durable state: before=%+v after=%+v", state, decoded)
	}
	session := Session{Metadata: map[string]interface{}{"memory": map[string]interface{}{
		passengerClarificationStateV1MemoryKey: decoded,
	}}}
	history := make([]Message, 50)
	for i := range history {
		history[i] = Message{ID: uuid.NewString(), Direction: "INBOUND", Body: "somos 99"}
	}
	projection := collectBookingDraftContextFromState(session, history, "parser wording changed")
	if projection.PassengerCount != 3 || projection.ChildUnder5Count != 1 || projection.ExpectedDocumentCount != 3 {
		t.Fatalf("truncated transcript changed durable projection: %+v", projection)
	}
}

func passengerPromptDeliveryFixture(t *testing.T, slot PassengerClarificationSlotV1) (*fakeStore, Session, Message) {
	t.Helper()
	store := newFakeStore()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{Channel: "WHATSAPP", ContactKey: uuid.NewString()})
	if err != nil {
		t.Fatalf("create prompt fixture session: %v", err)
	}
	state := newPassengerClarificationStateV1()
	state.BootstrapCompleted = true
	if slot == PassengerClarificationSlotChild {
		state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "fixture-passenger", Value: 1, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
		}})
	}
	seedPassengerClarificationStateV1ForTest(store, session.ID, state)
	draftID := uuid.NewString()
	kind := ActivePromptPassengerCount
	body := askPassengerCountReply
	if slot == PassengerClarificationSlotChild {
		kind = ActivePromptLapChildQuestion
		body = askChildUnder5Reply
	}
	pending, ok := passengerClarificationPromptEventV1(kind, draftID)
	if !ok {
		t.Fatal("create pending prompt event")
	}
	saved, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		MessageID: draftID, SessionID: session.ID, IdempotencyKey: "draft-" + draftID,
		Body: body, SenderName: "SHABAS", ProcessingStatus: messageStatusAutomationDraft,
		Payload:           map[string]interface{}{passengerPendingPromptEventV1MessageKey: pending},
		NormalizedPayload: map[string]interface{}{passengerPendingPromptEventV1MessageKey: pending},
		Agent:             map[string]interface{}{}, Buffer: map[string]interface{}{}, RecordedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("save prompt draft: %v", err)
	}
	before, _ := passengerClarificationStateV1FromSession(store.sessions[session.ID])
	if before.PassengerPromptMessageID == draftID || before.ChildPromptMessageID == draftID {
		t.Fatalf("unsent draft opened prompt state: %+v", before)
	}
	return store, session, saved.Message
}

func assertPassengerPromptCopiedToActualOutbound(t *testing.T, draft Message, reply ReplyResult) {
	t.Helper()
	event, ok := decodePassengerClarificationEventV1(reply.Message.NormalizedPayload[passengerPromptEventV1MessageKey])
	if !ok || event.MessageID != reply.Message.ID || event.MessageID == draft.ID {
		t.Fatalf("prompt event was not canonicalized onto actual outbound: %+v", reply.Message.NormalizedPayload)
	}
	outboundEvent, ok := decodePassengerClarificationEventV1(reply.Outbound.Payload[passengerPromptEventV1MessageKey])
	if !ok || outboundEvent.EventID != event.EventID {
		t.Fatalf("outbound queue did not carry actual prompt event: message=%+v outbound=%+v", event, outboundEvent)
	}
}

func markPassengerPromptDeliveredForTest(t *testing.T, store *fakeStore, reply ReplyResult, providerID string) {
	t.Helper()
	if _, err := store.MarkReplyDeliverySent(context.Background(), MarkReplyDeliverySentInput{
		SessionID: reply.Session.ID, MessageID: reply.Message.ID, OutboundID: reply.Outbound.ID,
		ProviderMessageID: providerID, ProviderStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{}, SentAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("mark prompt delivered: %v", err)
	}
}

func removePassengerTestMessageID(ids []string, target string) []string {
	filtered := ids[:0]
	for _, id := range ids {
		if id != target {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func completePassengerStateForTest(passengerCount, childCount int) PassengerClarificationStateV1 {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "complete-passenger", Value: passengerCount, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "complete-child", Value: childCount, ValueKnown: true,
		},
	})
	state.BootstrapCompleted = true
	return state
}

func passengerOnlyStateForTest(passengerCount int, provenance PassengerCountProvenance) PassengerClarificationStateV1 {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		MessageID: "passenger-only", Value: passengerCount, ValueKnown: true,
		PassengerProvenance: provenance,
	}})
	state.BootstrapCompleted = true
	return state
}

func soloPlusChildPassengerStateForTest() PassengerClarificationStateV1 {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "solo-plus-child-passenger", Value: 1, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
		},
		{
			Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
			MessageID: "solo-plus-child-prompt",
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "solo-plus-child-answer", PromptMessageID: "solo-plus-child-prompt",
			Value: 1, ValueKnown: true,
		},
	})
	state.BootstrapCompleted = true
	return state
}

func seedPassengerClarificationStateV1ForTest(store *fakeStore, sessionID string, state PassengerClarificationStateV1) {
	session := store.sessions[sessionID]
	metadata := cloneMap(session.Metadata)
	memory := cloneMap(asMap(metadata["memory"]))
	if memory == nil {
		memory = map[string]interface{}{}
	}
	memory[passengerClarificationStateV1MemoryKey] = state
	metadata["memory"] = memory
	session.Metadata = metadata
	store.sessions[sessionID] = session
}

func createPassengerStructuredEventMessageForTest(
	t *testing.T,
	store *fakeStore,
	sessionID string,
	lane string,
	event PassengerClarificationEventV1,
) Message {
	t.Helper()
	message, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: sessionID, Direction: "INBOUND", Kind: "TEXT", Body: "structured " + lane,
		ProcessingStatus: "BUFFERED_PENDING", ReceivedAt: time.Now().UTC(),
		NormalizedPayload: map[string]interface{}{
			passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{event},
		},
	})
	if err != nil {
		t.Fatalf("create %s structured message: %v", lane, err)
	}
	return message
}

type passengerConcurrentLaneKey struct{}

type concurrentPassengerReprocessStore struct {
	*fakeStore
	mu            sync.Mutex
	reprocessMu   sync.Mutex
	messageByLane map[string]string
	applyReady    chan struct{}
	applyDone     chan struct{}
	releaseApply  chan struct{}
}

func newConcurrentPassengerReprocessStore(base *fakeStore, messageByLane map[string]string) *concurrentPassengerReprocessStore {
	return &concurrentPassengerReprocessStore{
		fakeStore: base, messageByLane: messageByLane,
		applyReady: make(chan struct{}, 2), applyDone: make(chan struct{}, 2), releaseApply: make(chan struct{}),
	}
}

func (s *concurrentPassengerReprocessStore) GetSession(ctx context.Context, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.fakeStore.GetSession(ctx, id)
	return clonePassengerConcurrentValue(session), err
}

func (s *concurrentPassengerReprocessStore) ListMessages(ctx context.Context, sessionID string, filter ListMessagesFilter) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.fakeStore.ListMessages(ctx, sessionID, filter)
	if err != nil {
		return nil, err
	}
	lane, _ := ctx.Value(passengerConcurrentLaneKey{}).(string)
	target := s.messageByLane[lane]
	if target == "" {
		return clonePassengerConcurrentValue(all), nil
	}
	for _, message := range all {
		if message.ID == target {
			return clonePassengerConcurrentValue([]Message{message}), nil
		}
	}
	return nil, nil
}

func (s *concurrentPassengerReprocessStore) ApplyPassengerClarificationEventsV1(
	ctx context.Context,
	input ApplyPassengerClarificationEventsV1Input,
) (ApplyPassengerClarificationEventsV1Result, error) {
	s.applyReady <- struct{}{}
	<-s.releaseApply
	s.reprocessMu.Lock()
	s.mu.Lock()
	result, err := s.fakeStore.ApplyPassengerClarificationEventsV1(ctx, input)
	cloned := clonePassengerConcurrentValue(result)
	s.mu.Unlock()
	s.applyDone <- struct{}{}
	if err != nil {
		s.reprocessMu.Unlock()
		return result, err
	}
	return cloned, nil
}

func (s *concurrentPassengerReprocessStore) MarkReplyDeliveryFailure(ctx context.Context, input MarkReplyDeliveryFailureInput) (ReplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.fakeStore.MarkReplyDeliveryFailure(ctx, input)
	return clonePassengerConcurrentValue(result), err
}

func (s *concurrentPassengerReprocessStore) UpdateDraftAutoSendState(ctx context.Context, input UpdateDraftAutoSendStateInput) (SaveAgentDraftResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.fakeStore.UpdateDraftAutoSendState(ctx, input)
	return clonePassengerConcurrentValue(result), err
}

func (s *concurrentPassengerReprocessStore) SaveReprocessSnapshot(
	ctx context.Context,
	input SaveReprocessSnapshotInput,
) (SaveReprocessSnapshotResult, error) {
	s.mu.Lock()
	result, err := s.fakeStore.SaveReprocessSnapshot(ctx, input)
	cloned := clonePassengerConcurrentValue(result)
	s.mu.Unlock()
	s.reprocessMu.Unlock()
	return cloned, err
}

func (s *concurrentPassengerReprocessStore) SaveAgentDraft(ctx context.Context, input SaveAgentDraftInput) (SaveAgentDraftResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.fakeStore.SaveAgentDraft(ctx, input)
	return clonePassengerConcurrentValue(result), err
}

func (s *concurrentPassengerReprocessStore) FindMessageByKeys(ctx context.Context, providerMessageID, idempotencyKey string) (*Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.fakeStore.FindMessageByKeys(ctx, providerMessageID, idempotencyKey)
	return clonePassengerConcurrentValue(result), err
}

func clonePassengerConcurrentValue[T any](value T) T {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned T
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return value
	}
	return cloned
}

type windowedPassengerBootstrapStore struct{ *fakeStore }

func (s *windowedPassengerBootstrapStore) ListMessages(ctx context.Context, sessionID string, filter ListMessagesFilter) ([]Message, error) {
	items, err := s.fakeStore.ListMessages(ctx, sessionID, filter)
	if err != nil || filter.Limit <= 0 || len(items) <= filter.Limit {
		return items, err
	}
	return items[len(items)-filter.Limit:], nil
}
