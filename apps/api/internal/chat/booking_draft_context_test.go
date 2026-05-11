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
		RequestedPassengerDocuments: true,
		HasPassengerDetails:         false,
	}

	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
}
