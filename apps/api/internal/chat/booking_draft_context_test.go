package chat

import (
	"strings"
	"testing"
	"time"
)

func TestCollectBookingDraftContextUsesSelectedAvailabilityAndPassengerReply(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes para o trecho.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Origin:      "Santa Ines/MA",
							Destination: "Fraiburgo/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-1",
								BoardStopID:            "board-1",
								AlightStopID:           "alight-1",
								OriginDisplayName:      "Santa Ines/MA",
								DestinationDisplayName: "Fraiburgo/SC",
								OriginDepartTime:       "09:00",
								TripDate:               "2026-05-11",
								Price:                  950,
								Currency:               "BRL",
							},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
	}

	context := collectBookingDraftContext(Session{}, history, "e so para mim")
	context = mergePassengerReplyIntoBookingDraft(context, 1, 0)

	if !context.HasAvailabilityShown {
		t.Fatal("expected availability context to be detected")
	}
	if context.TripID != "trip-1" || context.BoardStopID != "board-1" || context.AlightStopID != "alight-1" {
		t.Fatalf("expected selected trip context, got %+v", context)
	}
	if context.Origin != "Santa Ines/MA" || context.Destination != "Fraiburgo/SC" {
		t.Fatalf("expected route context, got %+v", context)
	}
	if context.PassengerCount != 1 || context.ChildUnder5Count != 0 {
		t.Fatalf("expected passenger reply merged into context, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
}

func TestBuildBookingContinuationReplyAsksOnlyForDocuments(t *testing.T) {
	reply := buildBookingContinuationReply(BookingDraftContext{PassengerCount: 2}, BookingNextAskPassengerDocuments)
	if !strings.Contains(reply, "nomes completos e os documentos dos 2 passageiros") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestDecideNextBookingStepDoesNotCallCreateWithoutPassengerDetails(t *testing.T) {
	context := BookingDraftContext{
		HasAvailabilityShown:        true,
		TripID:                      "trip-1",
		BoardStopID:                 "board-1",
		AlightStopID:                "alight-1",
		Origin:                      "Igarape do Meio/MA",
		Destination:                 "Petrolandia/SC",
		TripDate:                    "2026-05-11",
		PassengerCount:              1,
		ChildUnder5Count:            0,
		PassengerCountKnown:         true,
		ChildUnder5CountKnown:       true,
		RequestedPassengerDocuments: true,
		HasPassengerDetails:         false,
	}

	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
}

func TestPassengerQuestionAloneDoesNotStartAdvancedBookingFlow(t *testing.T) {
	context := BookingDraftContext{AskedPassengerQuestion: true}
	if context.IsAdvancedBookingFlow() {
		t.Fatalf("expected passenger question alone not to be advanced booking flow, got %+v", context)
	}

	state := inferConversationPhase(CanonicalConversationState{}, context)
	if state != ConversationPhaseDiscovery {
		t.Fatalf("expected discovery phase, got %s", state)
	}
}

func TestDecideNextBookingStepCannotCallCreateWithoutSelectedTripData(t *testing.T) {
	complete := BookingDraftContext{
		HasAvailabilityShown:  true,
		TripID:                "trip-1",
		BoardStopID:           "board-1",
		AlightStopID:          "alight-1",
		Origin:                "Igarape do Meio/MA",
		Destination:           "Petrolandia/SC",
		TripDate:              "2026-05-11",
		PassengerCount:        1,
		ChildUnder5Count:      0,
		PassengerCountKnown:   true,
		ChildUnder5CountKnown: true,
		HasPassengerDetails:   true,
	}

	if action := decideNextBookingStep(complete); action != BookingNextCallCreate {
		t.Fatalf("expected complete selected trip to call create, got %s", action)
	}

	cases := map[string]func(BookingDraftContext) BookingDraftContext{
		"availability":  func(c BookingDraftContext) BookingDraftContext { c.HasAvailabilityShown = false; return c },
		"trip_id":       func(c BookingDraftContext) BookingDraftContext { c.TripID = ""; return c },
		"board_stop_id": func(c BookingDraftContext) BookingDraftContext { c.BoardStopID = ""; return c },
		"alight_stop_id": func(c BookingDraftContext) BookingDraftContext {
			c.AlightStopID = ""
			return c
		},
		"origin": func(c BookingDraftContext) BookingDraftContext {
			c.Origin = ""
			return c
		},
		"destination": func(c BookingDraftContext) BookingDraftContext {
			c.Destination = ""
			return c
		},
		"trip_date": func(c BookingDraftContext) BookingDraftContext {
			c.TripDate = ""
			return c
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			context := mutate(complete)
			if action := decideNextBookingStep(context); action == BookingNextCallCreate {
				t.Fatalf("expected missing %s to block call_create, got %+v", name, context)
			}
		})
	}
}

func TestPassengerSlotFlowPraMimThenNaoAsksDocuments(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: "Tem crianca de 5 anos ou menos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: time.Now().UTC().Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(Session{}, history, "nao")

	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger_count=1 known, got %+v", context)
	}
	if context.ChildUnder5Count != 0 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected child_under_5_count=0 known, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
}

func TestPassengerSlotFlowSoEuDoesNotAssumeNoChild(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")

	context := collectBookingDraftContext(Session{}, history, "so eu")

	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger_count=1 known, got %+v", context)
	}
	if context.ChildUnder5CountKnown {
		t.Fatalf("expected child_under_5_count to remain unknown, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerClarification {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerClarification, action)
	}
	reply := buildBookingContinuationReply(context, BookingNextAskPassengerClarification)
	if reply != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only question, got %q", reply)
	}
}

func TestPassengerSlotFlowSoEuThenNaoAsksDocuments(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: "Tem crianca de 5 anos ou menos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: time.Now().UTC().Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(Session{}, history, "nao")

	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger_count=1 known, got %+v", context)
	}
	if context.ChildUnder5Count != 0 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected child_under_5_count=0 known, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
}

func TestBuildBookingContinuationReplyDoesNotRepeatComboQuestionWhenPassengerKnown(t *testing.T) {
	context := BookingDraftContext{
		PassengerCount:      1,
		PassengerCountKnown: true,
	}

	reply := buildBookingContinuationReply(context, decideNextBookingStep(context))

	if reply != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only question, got %q", reply)
	}
}

func TestPassengerCountContextAsksOnlyChildWhenPassengerKnown(t *testing.T) {
	context := BookingDraftContext{
		PassengerCount:      1,
		PassengerCountKnown: true,
	}

	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerClarification {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerClarification, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if strings.Contains(reply, "vai mais alguem junto") {
		t.Fatalf("expected child-only question, got %q", reply)
	}
}

func passengerSlotAvailabilityHistory(t *testing.T, passengerQuestion string) []Message {
	t.Helper()
	now := time.Now().UTC()
	return []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Certo. Opcoes Santa Ines/MA -> Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", Qty: 1, Limit: 5},
						Results: []AvailabilitySearchItem{{
							TripID: "trip-1", BoardStopID: "board-1", AlightStopID: "alight-1",
							OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Fraiburgo/SC",
							OriginDepartTime: "12:00", TripDate: "2026-05-25", Price: 950, Currency: "BRL",
						}},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "OUTBOUND", Body: passengerQuestion, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
	}
}
