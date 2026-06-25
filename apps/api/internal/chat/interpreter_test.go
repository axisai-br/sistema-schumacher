package chat

import (
	"reflect"
	"testing"
	"time"
)

func TestInterpretStructuredTurnLapChildAssignmentPrecedesAvailabilityOption(t *testing.T) {
	history := availabilitySelectionStructuredHistory(t)
	history = append(history, Message{
		Direction:        "OUTBOUND",
		Body:             "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	})

	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "1",
		History:     history,
	})

	if got.Intent != StructuredIntentLapChildAssignmentAnswer {
		t.Fatalf("expected lap child assignment answer, got %+v", got)
	}
	if !reflect.DeepEqual(got.Booking.LapChildPassengerIndexes, []int{1}) {
		t.Fatalf("expected lap child index [1], got %+v", got.Booking.LapChildPassengerIndexes)
	}
	if got.Booking.SelectedOptionIndexKnown {
		t.Fatalf("lap child answer must not set selected availability option: %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnAvailabilityOptionSelection(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "2",
		History:     availabilitySelectionStructuredHistory(t),
	})

	if got.Intent != StructuredIntentSelectAvailabilityOption {
		t.Fatalf("expected availability option selection, got %+v", got)
	}
	if !got.Booking.SelectedOptionIndexKnown || got.Booking.SelectedOptionIndex != 2 {
		t.Fatalf("expected selected option 2, got %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnRejectsUnavailableAvailabilityOption(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "5",
		History:     availabilitySelectionStructuredHistory(t),
	})

	if got.Intent == StructuredIntentSelectAvailabilityOption {
		t.Fatalf("did not expect out-of-range availability option to be selected: %+v", got)
	}
	if got.Booking.SelectedOptionIndexKnown {
		t.Fatalf("out-of-range availability option must not set selected option: %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnStaleAvailabilityListDoesNotSelectOption(t *testing.T) {
	history := availabilitySelectionStructuredHistory(t)
	history = append(history,
		Message{
			Direction:        "INBOUND",
			Body:             "primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       time.Now().UTC().Add(-90 * time.Second),
		},
		Message{
			Direction:        "OUTBOUND",
			Body:             "A passagem e so para voce ou tem mais alguem?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		},
	)

	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "2",
		History:     history,
	})

	if got.Intent == StructuredIntentSelectAvailabilityOption {
		t.Fatalf("did not expect stale availability list to select option: %+v", got)
	}
	if got.Booking.SelectedOptionIndexKnown {
		t.Fatalf("stale availability list must not set selected option: %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnActivePromptSingleAvailabilityOptionContextualSelection(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "essa mesmo",
		History:     availabilityDateSelectionAfterRouteQuestionHistory(t),
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
		ObservedAt:  time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	if got.Intent != StructuredIntentSelectAvailabilityOption {
		t.Fatalf("expected active prompt availability option selection, got %+v", got)
	}
	if !got.Booking.SelectedOptionIndexKnown || got.Booking.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option 1, got %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnActivePromptAvailabilityNumericSelection(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "1",
		History:     availabilityDateSelectionWithFiveOptionsHistory(t),
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
		ObservedAt:  time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	if got.Intent != StructuredIntentSelectAvailabilityOption {
		t.Fatalf("expected active prompt availability option selection, got %+v", got)
	}
	if !got.Booking.SelectedOptionIndexKnown || got.Booking.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option 1, got %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnActivePromptRejectsOutOfRangeAvailabilityOption(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "5",
		History:     availabilityDateSelectionAfterRouteQuestionHistory(t),
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
		ObservedAt:  time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	if got.Intent != StructuredIntentUnknown {
		t.Fatalf("expected unknown for out-of-range option, got %+v", got)
	}
	if got.Booking.SelectedOptionIndexKnown {
		t.Fatalf("out-of-range availability option must not set selected option: %+v", got.Booking)
	}
	if !structuredReasonsContain(got, "availability_option_index_out_of_range") {
		t.Fatalf("expected explicit out-of-range reason, got %+v", got.Reasons)
	}
}

func TestInterpretStructuredTurnActivePromptDoesNotSelectAmbiguousAvailabilityReplyFromMultipleOptions(t *testing.T) {
	for _, text := range []string{"sim", "ok", "certo"} {
		t.Run(text, func(t *testing.T) {
			got := InterpretStructuredTurn(StructuredInterpreterInput{
				CurrentTurn: text,
				History:     availabilityDateSelectionWithFiveOptionsHistory(t),
				State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
				ObservedAt:  time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
			})

			if got.Intent == StructuredIntentSelectAvailabilityOption {
				t.Fatalf("ambiguous reply must not select option 1 from multiple options: %+v", got)
			}
			if got.Booking.SelectedOptionIndexKnown {
				t.Fatalf("ambiguous reply must not set selected option index: %+v", got.Booking)
			}
		})
	}
}

func TestInterpretStructuredTurnBookingCancelPrecedesActivePromptAnswers(t *testing.T) {
	paymentPromptHistory := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}

	cases := []struct {
		name       string
		text       string
		input      StructuredInterpreterInput
		disallowed StructuredIntent
	}{
		{
			name: "availability_option",
			text: "quero cancelar a opção 1",
			input: StructuredInterpreterInput{
				History:    availabilityDateSelectionWithFiveOptionsHistory(t),
				State:      CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
				ObservedAt: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
			},
			disallowed: StructuredIntentSelectAvailabilityOption,
		},
		{
			name: "availability_date",
			text: "quero cancelar 06/07",
			input: StructuredInterpreterInput{
				History:    availabilityDateChoiceAfterRouteQuestionHistory(t),
				State:      CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
				ObservedAt: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
			},
			disallowed: StructuredIntentAvailabilitySearch,
		},
		{
			name: "payment_preference",
			text: "quero cancelar sinal",
			input: StructuredInterpreterInput{
				History: paymentPromptHistory,
				State:   CanonicalConversationState{Phase: ConversationPhasePaymentPending},
			},
			disallowed: StructuredIntentPaymentPreference,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.CurrentTurn = tc.text
			got := InterpretStructuredTurn(tc.input)
			if got.Intent != StructuredIntentBookingCancelRequest {
				t.Fatalf("expected booking cancel request, got %+v", got)
			}
			if got.Intent == tc.disallowed {
				t.Fatalf("cancel text must not be swallowed by active prompt as %s: %+v", tc.disallowed, got)
			}
			if got.Booking.SelectedOptionIndexKnown {
				t.Fatalf("cancel text must not set selected option: %+v", got.Booking)
			}
			if got.Payment.PaymentPreference != "" {
				t.Fatalf("cancel text must not set payment preference: %+v", got.Payment)
			}
		})
	}
}

func TestInterpretStructuredTurnDocumentConfirmationRequiresContext(t *testing.T) {
	withContext := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "sim",
		History: []Message{{
			Direction:        "OUTBOUND",
			Body:             "Consegui identificar estes dados. Eles conferem?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		}},
	})
	if withContext.Intent != StructuredIntentDocumentConfirmation {
		t.Fatalf("expected document confirmation with context, got %+v", withContext)
	}

	withoutContext := InterpretStructuredTurn(StructuredInterpreterInput{CurrentTurn: "sim"})
	if withoutContext.Intent == StructuredIntentDocumentConfirmation {
		t.Fatalf("did not expect document confirmation without context: %+v", withoutContext)
	}

	genericBookingProceed := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "sim",
		History: []Message{{
			Direction:        "OUTBOUND",
			Body:             "Posso prosseguir e criar a reserva?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		}},
	})
	if genericBookingProceed.Intent == StructuredIntentDocumentConfirmation {
		t.Fatalf("did not expect generic booking proceed prompt to become document confirmation: %+v", genericBookingProceed)
	}
}

func TestInterpretStructuredTurnDocumentConfirmationIgnoresStaleAvailabilityContext(t *testing.T) {
	history := append(availabilitySelectionStructuredHistory(t), Message{
		Direction:        "OUTBOUND",
		Body:             "Consegui identificar estes dados. Eles conferem?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	})

	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "isso mesmo",
		History:     history,
	})

	if got.Intent != StructuredIntentDocumentConfirmation {
		t.Fatalf("expected document confirmation with stale availability context, got %+v", got)
	}
	if got.Booking.SelectedOptionIndexKnown {
		t.Fatalf("document confirmation must not select availability option: %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnGreeting(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{CurrentTurn: "oi"})

	if got.Intent != StructuredIntentGreeting {
		t.Fatalf("expected greeting, got %+v", got)
	}
}

func TestInterpretStructuredTurnPaymentPreference(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	for _, tc := range []struct {
		text string
		want string
	}{
		{text: "sinal", want: "sinal"},
		{text: "integral", want: "integral"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := InterpretStructuredTurn(StructuredInterpreterInput{
				CurrentTurn: tc.text,
				History:     history,
			})
			if got.Intent != StructuredIntentPaymentPreference {
				t.Fatalf("expected payment preference, got %+v", got)
			}
			if got.Payment.PaymentPreference != tc.want {
				t.Fatalf("expected payment preference %q, got %+v", tc.want, got.Payment)
			}
		})
	}
}

func TestInterpretStructuredTurnPixIsNotPaymentPreference(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	for _, text := range []string{"pix", "pagar via pix"} {
		t.Run(text, func(t *testing.T) {
			got := InterpretStructuredTurn(StructuredInterpreterInput{
				CurrentTurn: text,
				History:     history,
			})
			if got.Payment.PaymentPreference == "pix" {
				t.Fatalf("pix must not be stored as payment preference: %+v", got)
			}
			if got.Intent != StructuredIntentUnknown {
				t.Fatalf("pix-only payment method reply must remain unknown: %+v", got)
			}
			if !structuredReasonsContain(got, "pix_is_payment_method_not_payment_preference") {
				t.Fatalf("expected explicit pix reason, got %+v", got.Reasons)
			}
		})
	}
}

func TestInterpretStructuredTurnPassengerDocumentsProvided(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "Joao Vitor Messias CPF 066.456.481-03",
		History: []Message{{
			Direction:        "OUTBOUND",
			Body:             "Perfeito. Agora pode enviar seu nome completo e o documento.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		}},
	})

	if got.Intent != StructuredIntentPassengerDocumentsProvided {
		t.Fatalf("expected passenger documents provided, got %+v", got)
	}
	if !got.PassengerDocument.DocumentLikeText {
		t.Fatalf("expected passenger document-like text, got %+v", got.PassengerDocument)
	}
}

func TestInterpretStructuredTurnActivePromptPassengerCountSoloReply(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "só eu",
		History: []Message{{
			Direction:        "OUTBOUND",
			Body:             "A passagem e so para voce ou vai mais alguem junto?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		}},
		State: CanonicalConversationState{Phase: ConversationPhasePassengerCollection},
	})

	if got.Intent != StructuredIntentPassengerCountReply {
		t.Fatalf("expected passenger count reply, got %+v", got)
	}
	if !got.Booking.PassengerCountKnown || got.Booking.PassengerCount != 1 {
		t.Fatalf("expected passenger count 1, got %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnActivePromptLapChildQuestionNoReply(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "não",
		History: []Message{{
			Direction:        "OUTBOUND",
			Body:             "Tem crianca de ate 5 anos viajando?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		}},
		State: CanonicalConversationState{Phase: ConversationPhasePassengerCollection},
	})

	if got.Intent != StructuredIntentPassengerCountReply {
		t.Fatalf("expected passenger count reply, got %+v", got)
	}
	if !got.Booking.ChildUnder5CountKnown || got.Booking.ChildUnder5Count != 0 {
		t.Fatalf("expected known zero child under 5 count, got %+v", got.Booking)
	}
}

func TestInterpretStructuredTurnPassengerCountAndChildUnder5(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	for _, tc := range []struct {
		name           string
		text           string
		passengerCount int
		passengerKnown bool
		childCount     int
		childKnown     bool
	}{
		{name: "solo", text: "so eu", passengerCount: 1, passengerKnown: true},
		{name: "two_passengers", text: "2 pessoas", passengerCount: 2, passengerKnown: true},
		{name: "me_plus_one", text: "eu e mais uma", passengerCount: 2, passengerKnown: true},
		{name: "no_child", text: "sem crianca", childCount: 0, childKnown: true},
		{name: "one_child", text: "uma crianca de 4 anos", childCount: 1, childKnown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := InterpretStructuredTurn(StructuredInterpreterInput{
				CurrentTurn: tc.text,
				History:     history,
			})
			if got.Intent != StructuredIntentPassengerCountReply {
				t.Fatalf("expected passenger count reply, got %+v", got)
			}
			if got.Booking.PassengerCountKnown != tc.passengerKnown || got.Booking.PassengerCount != tc.passengerCount {
				t.Fatalf("unexpected passenger count slots: got %+v want known=%t count=%d", got.Booking, tc.passengerKnown, tc.passengerCount)
			}
			if got.Booking.ChildUnder5CountKnown != tc.childKnown || got.Booking.ChildUnder5Count != tc.childCount {
				t.Fatalf("unexpected child count slots: got %+v want known=%t count=%d", got.Booking, tc.childKnown, tc.childCount)
			}
		})
	}
}

func TestInterpretStructuredTurnActivePromptAvailabilityDateChoice(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "06/07",
		History:     availabilityDateChoiceAfterRouteQuestionHistory(t),
		State:       CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
		ObservedAt:  time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	if got.Intent != StructuredIntentAvailabilitySearch {
		t.Fatalf("expected availability search for date choice, got %+v", got)
	}
	if got.TurnMeaning != TurnMeaningAnswerToQuestion {
		t.Fatalf("expected answer-to-question meaning, got %+v", got)
	}
}

func TestInterpretStructuredTurnActivePromptReservationRouteAnswer(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{
		CurrentTurn: "de Monção para Videira",
		History: []Message{{
			Direction:        "OUTBOUND",
			Body:             "Qual trecho da viagem?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
		}},
		State:      CanonicalConversationState{Phase: ConversationPhaseRouteSelection},
		ObservedAt: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	})

	if got.Intent != StructuredIntentAvailabilitySearch {
		t.Fatalf("expected availability search for route answer, got %+v", got)
	}
	if got.TurnMeaning != TurnMeaningAnswerToQuestion {
		t.Fatalf("expected answer-to-question meaning, got %+v", got)
	}
}

func TestInterpretStructuredTurnAmbiguousUnknown(t *testing.T) {
	got := InterpretStructuredTurn(StructuredInterpreterInput{CurrentTurn: "talvez"})

	if got.Intent != StructuredIntentUnknown {
		t.Fatalf("expected unknown for ambiguous text, got %+v", got)
	}
}

func TestStructuredInterpretationDoesNotExposeExecutionFields(t *testing.T) {
	typ := reflect.TypeOf(StructuredInterpretation{})
	for _, field := range []string{"Action", "ToolName", "TemplateName"} {
		if _, ok := typ.FieldByName(field); ok {
			t.Fatalf("StructuredInterpretation must not expose execution field %s", field)
		}
	}
}

func structuredReasonsContain(got StructuredInterpretation, reason string) bool {
	for _, gotReason := range got.Reasons {
		if gotReason == reason {
			return true
		}
	}
	return false
}

func availabilitySelectionStructuredHistory(t *testing.T) []Message {
	t.Helper()
	return []Message{{
		Direction:        "OUTBOUND",
		Body:             "Achei duas opcoes para Santa Ines/MA -> Fraiburgo/SC.",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-2 * time.Minute),
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
					Filter: AvailabilitySearchInput{
						Origin:      "Santa Ines/MA",
						Destination: "Fraiburgo/SC",
						Qty:         1,
						Limit:       2,
					},
					Results: []AvailabilitySearchItem{
						{
							TripID:                 "trip-1",
							BoardStopID:            "board-1",
							AlightStopID:           "alight-1",
							OriginDisplayName:      "Santa Ines/MA",
							DestinationDisplayName: "Fraiburgo/SC",
							OriginDepartTime:       "08:00",
							TripDate:               "2026-06-20",
						},
						{
							TripID:                 "trip-2",
							BoardStopID:            "board-2",
							AlightStopID:           "alight-2",
							OriginDisplayName:      "Santa Ines/MA",
							DestinationDisplayName: "Fraiburgo/SC",
							OriginDepartTime:       "12:00",
							TripDate:               "2026-06-21",
						},
					},
				}),
			},
		},
	}}
}
