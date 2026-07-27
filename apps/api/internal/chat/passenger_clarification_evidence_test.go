package chat

import "testing"

func TestPassengerClarificationStructuredEventsIgnoreBodyAndPayload(t *testing.T) {
	message := Message{
		ID:        "message-structured-1",
		Direction: "INBOUND",
		Body:      "eu e meus 99 parentes",
		Payload: map[string]interface{}{
			passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{{
				Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
				Value: 9, ValueKnown: true,
			}},
		},
		NormalizedPayload: map[string]interface{}{
			passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{{
				Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
				Value: 2, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
			}},
		},
	}
	events := passengerClarificationStructuredEventsV1([]Message{message})
	if len(events) != 1 || events[0].MessageID != message.ID || events[0].Value != 2 ||
		events[0].Type != PassengerClarificationEventPassengerCountSet {
		t.Fatalf("expected only the normalized structured event, got %+v", events)
	}
}

func TestPassengerClarificationMalformedStructuredArtifactInvalidatesBothSlots(t *testing.T) {
	message := Message{
		ID: "message-malformed-1", Direction: "INBOUND",
		NormalizedPayload: map[string]interface{}{
			passengerClarificationEventsV1MessageKey: "not-an-event-list",
		},
	}
	events := passengerClarificationStructuredEventsV1([]Message{message})
	state := ReducePassengerClarificationEventsV1(completePassengerStateForTest(2, 0), events)
	if len(events) != 2 || state.PassengerCountKnown || state.ChildUnder5CountKnown ||
		!passengerClarificationStateUnsafeV1(state) {
		t.Fatalf("malformed structured artifact must fail closed: events=%+v state=%+v", events, state)
	}
}

func TestPassengerClarificationPromptEventRequiresSentOutboundArtifact(t *testing.T) {
	event, ok := passengerClarificationPromptEventV1(ActivePromptLapChildQuestion, "outbound-prompt-1")
	if !ok {
		t.Fatal("expected child prompt event")
	}
	message := Message{
		ID: "outbound-prompt-1", Direction: "OUTBOUND", Body: "wording can change",
		ProcessingStatus: messageStatusAutomationDraft,
		NormalizedPayload: map[string]interface{}{
			passengerPromptEventV1MessageKey: event,
		},
	}
	if got, found := passengerClarificationPromptEventFromMessageV1(message); found {
		t.Fatalf("draft must not open a prompt epoch: %+v", got)
	}
	message.ProcessingStatus = messageStatusAutomationSent
	message.NormalizedPayload["delivery_recorded_at"] = "2026-07-20T12:00:00Z"
	got, found := passengerClarificationPromptEventFromMessageV1(message)
	if !found || got.Type != PassengerClarificationEventChildPromptOpened || got.MessageID != message.ID {
		t.Fatalf("sent outbound must expose persisted prompt event: found=%v event=%+v", found, got)
	}
	message.Body = "completely different wording"
	replayed, found := passengerClarificationPromptEventFromMessageV1(message)
	if !found || replayed.EventID != got.EventID {
		t.Fatalf("body changes must not affect the structural prompt event: first=%+v replay=%+v", got, replayed)
	}
}

func TestPassengerPromptEventCopiesFromDraftToActualOutbound(t *testing.T) {
	pending, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, "draft-1")
	if !ok {
		t.Fatal("expected passenger prompt event")
	}
	draft := Message{
		ID: "draft-1", Body: "question", NormalizedPayload: map[string]interface{}{
			passengerPendingPromptEventV1MessageKey: pending,
		},
	}
	event, ok := passengerPromptEventForOutboundV1(draft, "outbound-1", true)
	if !ok || event.MessageID != "outbound-1" || event.EventID == pending.EventID {
		t.Fatalf("expected canonical event tied to actual outbound: ok=%v event=%+v", ok, event)
	}
	if _, ok := passengerPromptEventForOutboundV1(draft, "outbound-edited", false); ok {
		t.Fatal("an incompatible edited draft must not copy the pending prompt event")
	}
}

func TestPassengerClarificationPromptEventForRunUsesExplicitTemplateAllowlist(t *testing.T) {
	tests := []struct {
		name     string
		template ResponseTemplateName
		action   string
		wantType PassengerClarificationEventTypeV1
		want     bool
	}{
		{
			name: "ask passenger count after selection", template: TemplateAskPassengerCount, action: "template",
			wantType: PassengerClarificationEventPassengerPromptOpened, want: true,
		},
		{
			name: "ask passenger clarification", template: TemplateAskPassengerCount, action: string(BookingNextAskPassengerClarification),
			wantType: PassengerClarificationEventPassengerPromptOpened, want: true,
		},
		{
			name: "passenger count clarification", template: TemplateContextFallbackPassengerCount, action: "template",
			wantType: PassengerClarificationEventPassengerPromptOpened, want: true,
		},
		{
			name: "ask child under five", template: TemplateAskChildUnder5, action: string(BookingNextAskPassengerClarification),
			wantType: PassengerClarificationEventChildPromptOpened, want: true,
		},
		{
			name: "child under five clarification", template: TemplateContextFallbackChildUnder5, action: "template",
			wantType: PassengerClarificationEventChildPromptOpened, want: true,
		},
		{name: "passenger template with wrong action", template: TemplateAskPassengerCount, action: "safe_fallback"},
		{name: "generic safe fallback", template: ResponseTemplateName(safePhaseFallbackTemplateName), action: "template"},
		{name: "human handoff", template: TemplateHumanHandoff, action: "template"},
		{name: "parallel information", template: TemplatePaymentOptionsInfo, action: "template"},
		{name: "availability guardrail", template: TemplateContextFallbackAvailabilityOption, action: "template"},
	}

	state := newPassengerClarificationStateV1()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := RunAgentResult{
				ResponsePayload: map[string]interface{}{
					"template_name": string(test.template),
					"action":        test.action,
				},
			}
			event, ok := passengerClarificationPromptEventForRunV1(run, "draft-prompt", state)
			if ok != test.want {
				t.Fatalf("prompt event exists=%t, want %t for template %s: %+v", ok, test.want, test.template, event)
			}
			if test.want && (event.Type != test.wantType || event.MessageID != "draft-prompt") {
				t.Fatalf("unexpected prompt event for template %s: %+v", test.template, event)
			}
		})
	}
}
