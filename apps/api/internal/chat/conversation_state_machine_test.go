package chat

import "testing"

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
