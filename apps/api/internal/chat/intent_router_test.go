package chat

import (
	"testing"
	"time"
)

func TestIntentRouterDeterministicCases(t *testing.T) {
	history := availabilitySelectionHistory(t)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")
	cases := []struct {
		text   string
		intent Intent
		index  int
	}{
		{text: "primeira", intent: IntentSelectAvailabilityOption, index: 1},
		{text: "segunda", intent: IntentSelectAvailabilityOption, index: 2},
		{text: "opção 1", intent: IntentSelectAvailabilityOption, index: 1},
		{text: "essa opção", intent: IntentSelectAvailabilityOption, index: 1},
		{text: "paguei", intent: IntentPaymentStatusQuery},
		{text: "quero cancelar", intent: IntentBookingCancel},
		{text: "quero levar uma moto", intent: IntentUnsupportedCargo},
	}
	for _, tc := range cases {
		got := routeDeterministicIntent(history, tc.text, state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
		if got.Intent != tc.intent {
			t.Fatalf("routeDeterministicIntent(%q) = %s, want %s", tc.text, got.Intent, tc.intent)
		}
		if tc.index > 0 && got.SelectedOptionIndex != tc.index {
			t.Fatalf("routeDeterministicIntent(%q) selected index = %d, want %d", tc.text, got.SelectedOptionIndex, tc.index)
		}
	}
}

func TestIntentRouterAvailabilitySearch(t *testing.T) {
	got := routeDeterministicIntent(nil, "Fraiburgo para monção 18/05", CanonicalConversationState{}, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if got.Intent != IntentAvailabilitySearch || got.AvailabilityInput == nil {
		t.Fatalf("expected availability search decision, got %+v", got)
	}
}
