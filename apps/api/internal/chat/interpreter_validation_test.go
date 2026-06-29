package chat

import (
	"testing"
	"time"
)

func TestValidateStructuredInterpretationRejectsPixAsPaymentPreference(t *testing.T) {
	proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
	proposal.Payment.PaymentPreference = "pix"

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "pix",
		State:        CanonicalConversationState{Phase: ConversationPhaseBooked},
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPaymentPreference},
	})

	assertInterpretationRejected(t, got, "pix_is_method_not_payment_preference", TemplateContextFallbackPaymentPreference)
}

func TestValidateStructuredInterpretationAcceptsSinalPaymentPreference(t *testing.T) {
	proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
	proposal.Payment.PaymentPreference = "sinal"

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "sinal",
		State:        CanonicalConversationState{Phase: ConversationPhaseBooked},
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPaymentPreference},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationAcceptsIntegralPaymentPreference(t *testing.T) {
	proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
	proposal.Payment.PaymentPreference = "integral"

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "integral",
		State:        CanonicalConversationState{Phase: ConversationPhaseBooked},
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPaymentPreference},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationAcceptsAvailabilityOptionInRange(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(1)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "1",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityOptionChoice, AvailabilityOptionCount: 5},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsAvailabilityOptionOutOfRange(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(10)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "10",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityOptionChoice, AvailabilityOptionCount: 5},
	})

	assertInterpretationRejected(t, got, "availability_option_index_out_of_range", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationRejectsAmbiguousAvailabilityOption(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(1)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "ok",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityOptionChoice, AvailabilityOptionCount: 5},
	})

	assertInterpretationRejected(t, got, "ambiguous_availability_option_reply", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationAcceptsAvailabilityDate(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningAnswerToQuestion),
		CurrentTurn:  "06/07",
		History:      availabilityDateChoiceAfterRouteQuestionHistory(t),
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityDateChoice},
		ObservedAt:   time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsInvalidAvailabilityDate(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningAnswerToQuestion),
		CurrentTurn:  "ok",
		History:      availabilityDateChoiceAfterRouteQuestionHistory(t),
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityDateChoice},
		ObservedAt:   time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	assertInterpretationRejected(t, got, "invalid_availability_date", TemplateContextFallbackAvailabilityDate)
}

func TestValidateStructuredInterpretationRejectsCalendarInvalidAvailabilityDate(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningAnswerToQuestion),
		CurrentTurn:  "31/02",
		History:      availabilityDateChoiceAfterRouteQuestionHistory(t),
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityDateChoice},
		ObservedAt:   time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	assertInterpretationRejected(t, got, "invalid_availability_date", TemplateContextFallbackAvailabilityDate)
}

func TestValidateStructuredInterpretationAcceptsPassengerCount(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerCountReply, TurnMeaningAnswerToQuestion)
	proposal.Booking.PassengerCountKnown = true
	proposal.Booking.PassengerCount = 2

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "duas pessoas",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPassengerCount},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsAmbiguousPassengerCount(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerCountReply, TurnMeaningAnswerToQuestion)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "ok",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPassengerCount},
	})

	assertInterpretationRejected(t, got, "passenger_count_required", TemplateContextFallbackPassengerCount)
}

func TestValidateStructuredInterpretationAcceptsChildUnder5Reply(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerCountReply, TurnMeaningAnswerToQuestion)
	proposal.Booking.ChildUnder5CountKnown = true
	proposal.Booking.ChildUnder5Count = 1

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "tem uma crianca",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptLapChildQuestion},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsAmbiguousChildUnder5Reply(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerCountReply, TurnMeaningAnswerToQuestion)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "ok",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptLapChildQuestion},
	})

	assertInterpretationRejected(t, got, "child_under_5_reply_required", TemplateContextFallbackChildUnder5)
}

func TestValidateStructuredInterpretationAcceptsPassengerDocuments(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion)
	proposal.PassengerDocument.DocumentLikeText = true

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "Joao Silva CPF 529.982.247-25",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPassengerDocuments},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsMissingPassengerDocuments(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion)
	proposal.PassengerDocument.DocumentLikeText = true

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "ok",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPassengerDocuments},
	})

	assertInterpretationRejected(t, got, "passenger_documents_required", TemplateContextFallbackPassengerDocuments)
}

func TestValidateStructuredInterpretationRejectsInvalidPassengerDocumentCPF(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion)
	proposal.PassengerDocument.DocumentLikeText = true

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "12345678900",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPassengerDocuments},
	})

	assertInterpretationRejected(t, got, "invalid_passenger_document", TemplateContextFallbackPassengerDocuments)
}

func TestValidateStructuredInterpretationAcceptsDocumentConfirmation(t *testing.T) {
	for _, text := range []string{"sim", "certo"} {
		t.Run(text, func(t *testing.T) {
			got := ValidateStructuredInterpretation(InterpretationValidationInput{
				Proposal:     validationProposal(StructuredIntentDocumentConfirmation, TurnMeaningConfirmation),
				CurrentTurn:  text,
				ActivePrompt: ActivePromptContext{Kind: ActivePromptDocumentConfirmation},
			})

			assertInterpretationAccepted(t, got)
		})
	}
}

func TestValidateStructuredInterpretationRejectsAmbiguousDocumentConfirmation(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     validationProposal(StructuredIntentDocumentConfirmation, TurnMeaningConfirmation),
		CurrentTurn:  "talvez",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptDocumentConfirmation},
	})

	assertInterpretationRejected(t, got, "document_confirmation_required", TemplateContextFallbackDocumentConfirmation)
}

func TestValidateStructuredInterpretationRejectsQuestionedDocumentConfirmation(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     validationProposal(StructuredIntentDocumentConfirmation, TurnMeaningConfirmation),
		CurrentTurn:  "ok?",
		ActivePrompt: ActivePromptContext{Kind: ActivePromptDocumentConfirmation},
	})

	assertInterpretationRejected(t, got, "ambiguous_document_confirmation", TemplateContextFallbackDocumentConfirmation)
}

func TestValidateStructuredInterpretationAcceptsPayerCPF(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion)
	proposal.PassengerDocument.DocumentLikeText = true

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "52998224725",
		State:        CanonicalConversationState{Phase: ConversationPhaseBooked},
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPayerCPF},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsInvalidPayerCPF(t *testing.T) {
	proposal := validationProposal(StructuredIntentPassengerDocumentsProvided, TurnMeaningAnswerToQuestion)
	proposal.PassengerDocument.DocumentLikeText = true

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "12345678900",
		State:        CanonicalConversationState{Phase: ConversationPhaseBooked},
		ActivePrompt: ActivePromptContext{Kind: ActivePromptPayerCPF},
	})

	assertInterpretationRejected(t, got, "payer_cpf_required", TemplateContextFallbackPayerCPF)
}

func TestValidateStructuredInterpretationAcceptsReservationHelpWithoutDestination(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningNewRequest),
		CurrentTurn: "como faço pra fazer uma reserva?",
		State:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsReservationHelpWithUnsupportedDestination(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationProposal(StructuredIntentAvailabilitySearch, TurnMeaningNewRequest),
		CurrentTurn: "como faço pra reservar passagem para Bahia",
		State:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
	})

	assertInterpretationRejected(t, got, "unsupported_destination", TemplateUnsupportedPackage)
}

func TestValidateStructuredInterpretationRejectsAnswerOnlyIntentWithoutActivePrompt(t *testing.T) {
	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(1),
		CurrentTurn: "ok",
		State:       CanonicalConversationState{Phase: ConversationPhaseDiscovery},
	})

	assertInterpretationRejected(t, got, "active_prompt_required", TemplateContextFallbackAvailabilityOption)
}

func validationProposal(intent StructuredIntent, meaning TurnMeaning) StructuredInterpretation {
	return StructuredInterpretation{
		Intent:      intent,
		TurnMeaning: meaning,
		Confidence:  0.95,
		Source:      "test",
	}
}

func validationAvailabilityOptionProposal(index int) StructuredInterpretation {
	proposal := validationProposal(StructuredIntentSelectAvailabilityOption, TurnMeaningAnswerToQuestion)
	proposal.Booking.SelectedOptionIndexKnown = true
	proposal.Booking.SelectedOptionIndex = index
	return proposal
}

func assertInterpretationAccepted(t *testing.T, got InterpretationValidationResult) {
	t.Helper()
	if !got.Accepted {
		t.Fatalf("expected accepted interpretation, got %+v", got)
	}
	if got.RejectReason != "" || got.FallbackTemplate != "" {
		t.Fatalf("accepted interpretation must not include reject metadata, got %+v", got)
	}
}

func assertInterpretationRejected(t *testing.T, got InterpretationValidationResult, reason string, fallback ResponseTemplateName) {
	t.Helper()
	if got.Accepted {
		t.Fatalf("expected rejected interpretation")
	}
	if got.RejectReason != reason {
		t.Fatalf("expected reject reason %q, got %+v", reason, got)
	}
	if got.FallbackTemplate != fallback {
		t.Fatalf("expected fallback %q, got %+v", fallback, got)
	}
}
