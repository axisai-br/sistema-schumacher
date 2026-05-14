package chat

import "testing"

func TestResponseRealizerTemplates(t *testing.T) {
	reply, ok := realizeResponseTemplate(TemplateAskPassengerCount)
	if !ok || reply != askPassengerCountReply {
		t.Fatalf("unexpected passenger count template: ok=%t reply=%q", ok, reply)
	}
	reply, ok = realizeResponseTemplate(TemplateUnsupportedCargo)
	if !ok || reply == "" {
		t.Fatalf("expected unsupported cargo template")
	}
}

func TestResponseRealizerNoDuplicateMADestinationCase(t *testing.T) {
	directReply, ok := realizeResponseTemplate(TemplateAskMADestination)
	if !ok || directReply == "" {
		t.Fatalf("expected direct MADestination template reply")
	}

	intentReply, ok := realizeIntentResponseTemplate(IntentDecision{TemplateName: TemplateAskMADestination})
	if !ok || intentReply != directReply {
		t.Fatalf("expected intent realizer to fall back to direct template reply, got ok=%t reply=%q direct=%q", ok, intentReply, directReply)
	}
}

func TestResponseRealizerRequiresToolFactForSelection(t *testing.T) {
	decision := IntentDecision{Intent: IntentSelectAvailabilityOption, SelectedOptionIndex: 1, TemplateName: TemplateAskPassengerCount}
	if canRealizeWithoutLLM(decision, CanonicalConversationState{}) {
		t.Fatal("expected selection template to require previous availability fact")
	}
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, availabilitySelectionHistory(t), "")
	if !canRealizeWithoutLLM(decision, state) {
		t.Fatal("expected selection template with previous availability fact")
	}
}
