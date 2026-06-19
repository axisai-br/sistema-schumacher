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
		CurrentTurn: "2",
		History:     history,
	})

	if got.Intent != StructuredIntentLapChildAssignmentAnswer {
		t.Fatalf("expected lap child assignment answer, got %+v", got)
	}
	if !reflect.DeepEqual(got.Booking.LapChildPassengerIndexes, []int{2}) {
		t.Fatalf("expected lap child index [2], got %+v", got.Booking.LapChildPassengerIndexes)
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
			if got.Intent == StructuredIntentPaymentPreference {
				t.Fatalf("pix-only payment method reply must not become payment preference: %+v", got)
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
