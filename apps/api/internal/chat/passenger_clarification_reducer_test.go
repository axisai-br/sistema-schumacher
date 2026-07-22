package chat

import (
	"reflect"
	"testing"
)

func TestPassengerClarificationStateV1NewPromptResetsSlotWithoutRecognizedAnswer(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
			MessageID: "prompt-child-old",
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "message-child-zero", PromptMessageID: "prompt-child-old",
			Value: 0, ValueKnown: true,
		},
	})
	if !state.ChildUnder5CountKnown || state.ChildUnder5Count != 0 {
		t.Fatalf("expected initial exact zero, got %+v", state)
	}

	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
		MessageID: "prompt-child-new",
	}})
	if state.ChildUnder5CountKnown ||
		state.ChildSlotStatus != PassengerClarificationSlotOpen ||
		state.ChildPromptMessageID != "prompt-child-new" {
		t.Fatalf("new prompt must reset the slot before any recognized answer, got %+v", state)
	}

	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventSlotInvalidated, Slot: PassengerClarificationSlotChild,
		MessageID: "message-unrecognized", PromptMessageID: "prompt-child-new",
		ReasonCode: passengerClarificationReasonAmbiguousPassengerMeaning,
	}})
	if state.ChildUnder5CountKnown || state.ChildSlotStatus != PassengerClarificationSlotPending {
		t.Fatalf("unrecognized reply must stay non-actionable, got %+v", state)
	}
}

func TestPassengerClarificationStateV1CorrectionZeroClearsChildAggregate(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "message-solo", Value: 1, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
		},
		{
			Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
			MessageID: "prompt-child",
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "message-child-positive", PromptMessageID: "prompt-child",
			Value: 1, ValueKnown: true,
			ChildReferences: []PassengerClarificationChildReferenceV1{
				{ID: "child-ref-1", AgeKnown: true, Under5: true},
			},
		},
	})
	if len(state.ChildReferences) != 1 || !state.ChildUnder5AddsTraveler {
		t.Fatalf("test setup did not create child aggregate: %+v", state)
	}

	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventSlotCorrected, Slot: PassengerClarificationSlotChild,
		MessageID: "message-child-correction-zero", PromptMessageID: "prompt-child",
		Value: 0, ValueKnown: true,
	}})
	if !state.ChildUnder5CountKnown || state.ChildUnder5Count != 0 ||
		len(state.ChildReferences) != 0 || state.ChildUnder5AddsTraveler ||
		state.ChildUnder5AddsTravelerOrigin != (PassengerClarificationAddsTravelerOriginV1{}) {
		t.Fatalf("zero correction must replace the entire child slot, got %+v", state)
	}
}

func TestPassengerClarificationStateV1StandaloneChildOriginSurvivesLaterPrompt(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "message-solo", Value: 1, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
		},
		{
			Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
			MessageID: "prompt-child",
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "message-child", PromptMessageID: "prompt-child",
			Value: 1, ValueKnown: true,
		},
	})
	origin := state.ChildUnder5AddsTravelerOrigin

	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
		MessageID: "message-same-child-later", PromptMessageID: "prompt-documents",
		Value: 1, ValueKnown: true,
	}})
	if !state.ChildUnder5AddsTraveler || state.ChildUnder5AddsTravelerOrigin != origin {
		t.Fatalf("later prompt must not recalculate standalone-child provenance, got %+v", state)
	}
	context := applyPassengerClarificationStateToBookingDraft(BookingDraftContext{}, state)
	if got := expectedPassengerDocumentCount(context); got != 2 {
		t.Fatalf("expected adult and child documents after later prompt, got %d context=%+v", got, context)
	}
}

func TestPassengerClarificationStateV1AbsoluteTotalDoesNotBecomeSolo(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "message-total-one", Value: 1, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
		},
		{
			Type: PassengerClarificationEventChildPromptOpened, Slot: PassengerClarificationSlotChild,
			MessageID: "prompt-child",
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "message-child", PromptMessageID: "prompt-child",
			Value: 1, ValueKnown: true,
		},
	})
	if !passengerClarificationStateConflictingV1(state) ||
		state.PassengerCountKnown || state.ChildUnder5CountKnown || state.ChildUnder5AddsTraveler {
		t.Fatalf("absolute total one plus prompted child must be conflicting, got %+v", state)
	}
}

func TestPassengerClarificationStateV1DuplicateEventsAndRoundTripAreStable(t *testing.T) {
	events := []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		EventID: "caller-controlled-id-must-be-ignored", MessageID: "message-solo-stable", Value: 1, ValueKnown: true,
		PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
	}}
	first := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), events)
	if len(first.AppliedEventIDs) != 1 || first.AppliedEventIDs[0] != "message-solo-stable:PASSENGER_COUNT_SET:PASSENGER" {
		t.Fatalf("event ID must be canonical and stable, got %+v", first.AppliedEventIDs)
	}
	duplicate := ReducePassengerClarificationEventsV1(first, events)
	if !reflect.DeepEqual(first, duplicate) {
		t.Fatalf("duplicate event changed state:\nfirst=%+v\nduplicate=%+v", first, duplicate)
	}

	decoded, ok := decodePassengerClarificationStateV1(first)
	if !ok || !reflect.DeepEqual(first, decoded) {
		t.Fatalf("persist/restart round-trip changed state:\nfirst=%+v\ndecoded=%+v", first, decoded)
	}
}

func TestPassengerClarificationStateV1LateDeliveryCallbackDoesNotEraseAnswer(t *testing.T) {
	answer := PassengerClarificationEventV1{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		MessageID: "answer-after-outbound", PromptMessageID: "actual-outbound",
		Value: 2, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
	}
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{answer})
	state = ReducePassengerClarificationEventsV1(state, []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventPassengerPromptOpened, Slot: PassengerClarificationSlotPassenger,
		MessageID: "actual-outbound",
	}})
	if !state.PassengerCountKnown || state.PassengerCount != 2 ||
		state.PassengerSlotStatus != PassengerClarificationSlotAnswered ||
		state.PassengerPromptMessageID != "actual-outbound" {
		t.Fatalf("late delivery callback must not erase the answer for the same actual outbound: %+v", state)
	}
}

func TestBookingDraftContextUsesDurableStateBeyondTruncatedHistory(t *testing.T) {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "message-passenger-before-window", Value: 3, ValueKnown: true,
			PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "message-child-before-window", Value: 1, ValueKnown: true,
		},
	})
	session := Session{Metadata: map[string]interface{}{
		"memory": map[string]interface{}{passengerClarificationStateV1MemoryKey: state},
	}}
	history := make([]Message, 0, 50)
	for i := 0; i < 50; i++ {
		history = append(history, Message{ID: "window-message", Direction: "INBOUND", Body: "somos 1"})
	}
	context := collectBookingDraftContextFromState(session, history, "parser wording changed")
	if !context.PassengerCountKnown || context.PassengerCount != 3 ||
		!context.ChildUnder5CountKnown || context.ChildUnder5Count != 1 ||
		context.ExpectedDocumentCount != 3 {
		t.Fatalf("bounded history or parser text changed durable state: %+v", context)
	}
}
