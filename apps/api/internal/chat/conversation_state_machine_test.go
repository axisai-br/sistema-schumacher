package chat

import (
	"strings"
	"testing"
	"time"
)

func TestConversationStateMachineBlocksInvalidTransitions(t *testing.T) {
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}
	if validateConversationTransition(ConversationPhasePassengerCollection, ConversationPhaseBookingPending, state) {
		t.Fatal("expected booking pending transition to require passengers")
	}
	state.Passengers.ExpectedCount = 1
	state.Passengers.DocumentsCollected = true
	if !validateConversationTransition(ConversationPhasePassengerCollection, ConversationPhaseBookingPending, state) {
		t.Fatal("expected passenger collection to booking pending with passenger data")
	}
}

func TestConversationStateMachinePaymentRequiresToolStatus(t *testing.T) {
	state := CanonicalConversationState{Phase: ConversationPhasePaymentPending}
	if validateConversationTransition(ConversationPhasePaymentPending, ConversationPhasePaidFull, state) {
		t.Fatal("expected payment transition to require payment status")
	}
	state.Payment.Status = "PAID"
	if !validateConversationTransition(ConversationPhasePaymentPending, ConversationPhasePaidFull, state) {
		t.Fatal("expected payment transition with payment status")
	}
}

func TestDeriveCanonicalConversationStateKeepsLatestAvailabilityFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes antigas.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionFiveOptionsTestResult()),
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes:\n1. Videira/SC para Santa Ines/MA, 2026-07-13, saida 13:00, R$ 950\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
				},
			},
		},
	}

	got := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	availability := asMap(got.LastToolFacts[toolNameAvailabilitySearch])
	if count := len(asInterfaceSliceMaps(availability["results"])); count != 1 {
		t.Fatalf("expected latest availability facts with 1 result, got %d facts=%+v", count, availability)
	}
}

func TestDeriveCanonicalConversationStateIgnoresInvisibleAvailabilityFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	hiddenResult := availabilityDateSelectionFiveOptionsTestResult()

	for _, tc := range []struct {
		name       string
		status     string
		normalized bool
	}{
		{name: "draft_payload", status: messageStatusAutomationDraft},
		{name: "reviewed_payload", status: messageStatusAutomationReviewed},
		{name: "pending_payload", status: messageStatusAutomationPending},
		{name: "draft_normalized_payload", status: messageStatusAutomationDraft, normalized: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hidden := Message{
				Direction:        "OUTBOUND",
				Body:             buildAvailabilityListReply(hiddenResult),
				ProcessingStatus: tc.status,
				ReceivedAt:       now,
			}
			payload := map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(hiddenResult),
				},
			}
			if tc.normalized {
				hidden.NormalizedPayload = payload
			} else {
				hidden.Payload = payload
			}

			got := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, []Message{hidden}, "")

			if availability := asMap(got.LastToolFacts[toolNameAvailabilitySearch]); len(availability) > 0 {
				t.Fatalf("expected invisible availability facts to be ignored, got %+v", availability)
			}
		})
	}
}

func TestDeriveCanonicalConversationStateInvisibleAvailabilityFactsDoNotOverwriteVisibleFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	visibleResult := availabilityDateSelectionTestResult()
	hiddenResult := availabilityDateSelectionFiveOptionsTestResult()

	for _, status := range []string{
		messageStatusAutomationDraft,
		messageStatusAutomationReviewed,
		messageStatusAutomationPending,
	} {
		t.Run(status, func(t *testing.T) {
			history := []Message{
				{
					Direction:        "OUTBOUND",
					Body:             buildAvailabilityListReply(visibleResult),
					ProcessingStatus: messageStatusAutomationSent,
					ReceivedAt:       now.Add(-2 * time.Minute),
					Payload: map[string]interface{}{
						"tool_context": map[string]interface{}{
							toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(visibleResult),
						},
					},
				},
				{
					Direction:        "OUTBOUND",
					Body:             buildAvailabilityListReply(hiddenResult),
					ProcessingStatus: status,
					ReceivedAt:       now.Add(-1 * time.Minute),
					Payload: map[string]interface{}{
						"tool_context": map[string]interface{}{
							toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(hiddenResult),
						},
					},
				},
			}

			got := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

			availability := asMap(got.LastToolFacts[toolNameAvailabilitySearch])
			results := asInterfaceSliceMaps(availability["results"])
			if len(results) != 1 {
				t.Fatalf("expected visible availability facts with 1 result, got %d facts=%+v", len(results), availability)
			}
			if tripID := strings.TrimSpace(asString(results[0]["trip_id"])); tripID != "trip-2026-07-06" {
				t.Fatalf("expected visible trip facts to remain, got trip_id=%q facts=%+v", tripID, availability)
			}
		})
	}
}

func TestDeriveCanonicalConversationStateInvisibleSingleOptionAvailabilityDoesNotPreseedRoute(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	visibleResult := availabilityDateSelectionTestResult()
	hiddenResult := availabilityDateSelectionTestResult()
	hiddenResult.Results[0].TripID = "hidden-trip"
	hiddenResult.Results[0].BoardStopID = "hidden-board"
	hiddenResult.Results[0].AlightStopID = "hidden-alight"
	hiddenResult.Results[0].OriginDepartTime = "23:59"

	for _, status := range []string{
		messageStatusAutomationDraft,
		messageStatusAutomationReviewed,
		messageStatusAutomationPending,
	} {
		t.Run(status, func(t *testing.T) {
			history := []Message{
				{
					Direction:        "OUTBOUND",
					Body:             buildAvailabilityListReply(visibleResult),
					ProcessingStatus: messageStatusAutomationSent,
					ReceivedAt:       now.Add(-2 * time.Minute),
					Payload: map[string]interface{}{
						"tool_context": map[string]interface{}{
							toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(visibleResult),
						},
					},
				},
				{
					Direction:        "OUTBOUND",
					Body:             buildAvailabilityListReply(hiddenResult),
					ProcessingStatus: status,
					ReceivedAt:       now.Add(-1 * time.Minute),
					Payload: map[string]interface{}{
						"tool_context": map[string]interface{}{
							toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(hiddenResult),
						},
					},
				},
			}

			got := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

			if got.Route.TripID != "trip-2026-07-06" {
				t.Fatalf("expected route to use visible trip, got %+v", got.Route)
			}
			if got.Route.TripID == "hidden-trip" || got.Route.DepartureTime == "23:59" {
				t.Fatalf("invisible availability facts preseeded route: %+v", got.Route)
			}
		})
	}
}
