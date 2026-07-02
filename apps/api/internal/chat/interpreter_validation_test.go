package chat

import (
	"fmt"
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
		History:      validationAvailabilityHistory(validationAvailabilityResult(5), true),
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityOptionChoice, AvailabilityOptionCount: 5},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsAvailabilityOptionOutOfRange(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(10)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "10",
		History:      validationAvailabilityHistory(validationAvailabilityResult(5), true),
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityOptionChoice, AvailabilityOptionCount: 5},
	})

	assertInterpretationRejected(t, got, "availability_option_index_out_of_range", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationRejectsAmbiguousAvailabilityOption(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(1)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:     proposal,
		CurrentTurn:  "ok",
		History:      validationAvailabilityHistory(validationAvailabilityResult(5), true),
		ActivePrompt: ActivePromptContext{Kind: ActivePromptAvailabilityOptionChoice, AvailabilityOptionCount: 5},
	})

	assertInterpretationRejected(t, got, "ambiguous_availability_option_reply", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationRejectsRenderedAvailabilityOptionWithoutCurrentFacts(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(1)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    proposal,
		CurrentTurn: "1",
		History:     validationAvailabilityHistory(validationAvailabilityResult(1), false),
		State: CanonicalConversationState{
			Phase: ConversationPhaseRouteSelection,
			LastToolFacts: map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(validationAvailabilityResult(5)),
			},
		},
	})

	assertInterpretationRejected(t, got, "availability_selection_missing_current_facts", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationAcceptsCurrentSingleAvailabilityOptionContextualReply(t *testing.T) {
	proposal := validationAvailabilityOptionProposal(1)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    proposal,
		CurrentTurn: "essa msm",
		History:     validationAvailabilityHistory(validationAvailabilityResult(1), true),
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationAcceptsBotAutoReplyDeliveryMirrorSourceDraft(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := availabilityDeliveryMirrorHistory(now, availabilityDateSelectionTestResult(), true)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(1),
		CurrentTurn: "essa msm",
		History:     history,
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationRejectsBotAutoReplyWithoutDraftSource(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := availabilityDeliveryMirrorHistory(now, availabilityDateSelectionTestResult(), false)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(1),
		CurrentTurn: "essa msm",
		History:     history,
		State: CanonicalConversationState{
			Phase: ConversationPhaseRouteSelection,
			LastToolFacts: map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
			},
		},
	})

	assertInterpretationRejected(t, got, "availability_selection_missing_current_facts", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationUsesReliableAvailabilityPromptWhenInvisibleDraftFollows(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := availabilitySingleOptionHistoryWithInvisibleFollowUp(now, messageStatusAutomationDraft, availabilityDateSelectionFiveOptionsTestResult(), true)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(1),
		CurrentTurn: "essa msm",
		History:     history,
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationInvisibleAvailabilityDraftDoesNotAuthorizeUnseenOption(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := availabilitySingleOptionHistoryWithInvisibleFollowUp(now, messageStatusAutomationDraft, availabilityDateSelectionFiveOptionsTestResult(), true)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(5),
		CurrentTurn: "5",
		History:     history,
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
	})

	assertInterpretationRejected(t, got, "availability_option_index_out_of_range", TemplateContextFallbackAvailabilityOption)
}

func TestValidateStructuredInterpretationInvisibleDraftWithoutFactsDoesNotInvalidateSentAvailabilityPrompt(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := availabilitySingleOptionHistoryWithInvisibleFollowUp(now, messageStatusAutomationDraft, availabilityDateSelectionTestResult(), false)

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(1),
		CurrentTurn: "1",
		History:     history,
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
	})

	assertInterpretationAccepted(t, got)
}

func TestValidateStructuredInterpretationUsesVisibleCurrentAvailabilityOptionCount(t *testing.T) {
	history := validationAvailabilityHistory(validationAvailabilityResult(8), true)

	for _, index := range []int{1, 5} {
		t.Run(fmt.Sprintf("accepts_%d", index), func(t *testing.T) {
			got := ValidateStructuredInterpretation(InterpretationValidationInput{
				Proposal:    validationAvailabilityOptionProposal(index),
				CurrentTurn: fmt.Sprintf("%d", index),
				History:     history,
				State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
			})

			assertInterpretationAccepted(t, got)
		})
	}

	got := ValidateStructuredInterpretation(InterpretationValidationInput{
		Proposal:    validationAvailabilityOptionProposal(6),
		CurrentTurn: "6",
		History:     history,
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
	})

	assertInterpretationRejected(t, got, "availability_option_index_out_of_range", TemplateContextFallbackAvailabilityOption)
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

func validationAvailabilityHistory(result AvailabilitySearchResult, withFacts bool) []Message {
	message := Message{
		Direction:        "OUTBOUND",
		Body:             buildAvailabilityListReply(result),
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
	}
	if withFacts {
		message.Payload = map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
			},
		}
	}
	return []Message{message}
}

func validationAvailabilityResult(count int) AvailabilitySearchResult {
	result := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Ines/MA",
			Destination: "Videira/SC",
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       count,
		},
		Results: make([]AvailabilitySearchItem, 0, count),
	}
	for index := 1; index <= count; index++ {
		result.Results = append(result.Results, AvailabilitySearchItem{
			TripID:                 fmt.Sprintf("trip-validation-%02d", index),
			BoardStopID:            fmt.Sprintf("board-validation-%02d", index),
			AlightStopID:           fmt.Sprintf("alight-validation-%02d", index),
			OriginDisplayName:      "Santa Ines/MA",
			DestinationDisplayName: "Videira/SC",
			OriginDepartTime:       fmt.Sprintf("%02d:00", 7+index),
			SeatsAvailable:         8,
			Price:                  950,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToSantaCatarina,
		})
	}
	return result
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
