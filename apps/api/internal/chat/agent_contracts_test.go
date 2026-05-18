package chat

import (
	"strings"
	"testing"
)

func TestBuildJSONDecisionCompactInputIncludesCanonicalState(t *testing.T) {
	state := CanonicalConversationState{
		Phase:              ConversationPhaseDiscovery,
		AllowedNextActions: []string{"clarify", "tool"},
		Route: CanonicalRouteState{
			Origin:      "Chapeco/SC",
			Destination: "Santa Ines/MA",
			PackageName: "Pacote p/ Maranhão",
			TripDate:    "2026-05-20",
		},
	}

	got := buildJSONDecisionCompactInput("Quero viajar", state, []Message{{Direction: "INBOUND", Body: "Quero viajar"}})

	if !strings.Contains(got, `"state"`) {
		t.Fatalf("expected compact input to include canonical state, got %s", got)
	}
	if !strings.Contains(got, `"phase":"DISCOVERY"`) {
		t.Fatalf("expected compact input to include phase, got %s", got)
	}
	if !strings.Contains(got, `"current_turn":"Quero viajar"`) {
		t.Fatalf("expected compact input to include current turn, got %s", got)
	}
}
