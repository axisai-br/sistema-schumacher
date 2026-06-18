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

func TestPassengerSlotFlowEuEMaisUmaPessoaAsksChildThenDocumentsForTwo(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)

	context := collectBookingDraftContext(Session{}, history, "eu e mais uma pessoa")
	if context.PassengerCount != 2 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger_count=2 known, got %+v", context)
	}
	if context.ChildUnder5CountKnown {
		t.Fatalf("expected child_under_5_count unknown before child answer, got %+v", context)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerClarification {
		t.Fatalf("expected child question action %s, got %s", BookingNextAskPassengerClarification, action)
	}
	if reply := buildBookingContinuationReply(context, action); reply != askChildUnder5Reply {
		t.Fatalf("expected child question %q, got %q", askChildUnder5Reply, reply)
	}

	history = append(history,
		Message{Direction: "INBOUND", Body: "eu e mais uma pessoa", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: time.Now().UTC().Add(-60 * time.Second)},
	)
	context = collectBookingDraftContext(Session{}, history, "nao")
	if context.PassengerCount != 2 || !context.PassengerCountKnown || context.ChildUnder5Count != 0 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected two passengers and no child under 5, got %+v", context)
	}
	action = decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected documents action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	if reply := buildBookingContinuationReply(context, action); !strings.Contains(reply, "documentos dos 2 passageiros") {
		t.Fatalf("expected documents request for two passengers, got %q", reply)
	}
}

func TestPassengerSlotFlowEuEMaisTresThenSimAsksDocumentsForFour(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)

	context := collectBookingDraftContext(Session{}, history, "eu e mais 3 pessoas")
	if context.PassengerCount != 4 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger_count=4 known, got %+v", context)
	}
	if context.ChildUnder5CountKnown {
		t.Fatalf("expected child_under_5_count unknown before child answer, got %+v", context)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerClarification {
		t.Fatalf("expected child question action %s, got %s", BookingNextAskPassengerClarification, action)
	}
	if reply := buildBookingContinuationReply(context, action); reply != askChildUnder5Reply {
		t.Fatalf("expected child question %q, got %q", askChildUnder5Reply, reply)
	}

	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "eu e mais 3 pessoas", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-60 * time.Second)},
	)
	context = collectBookingDraftContext(Session{}, history, "sim")
	if context.PassengerCount != 4 || !context.PassengerCountKnown || context.ChildUnder5Count != 1 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected four passengers and one child under 5, got %+v", context)
	}
	action = decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected documents action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	if reply := buildBookingContinuationReply(context, action); !strings.Contains(reply, "documentos dos 4 passageiros") {
		t.Fatalf("expected documents request for four passengers, got %q", reply)
	}
}

func TestTypedPassengerDocsWithoutChildLabelAskLapChildAssignment(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "eu e meu filho de 4 anos", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF ou RG).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 06645648103\nIvoneide Messias 46643591104", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	)

	context := collectBookingDraftContext(Session{}, history, "")
	if context.PassengerCount != 2 || context.ChildUnder5Count != 1 || !context.NeedsLapChildAssignment {
		t.Fatalf("expected pending lap child assignment, got %+v", context)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskLapChildAssignment {
		t.Fatalf("expected next action %s, got %s", BookingNextAskLapChildAssignment, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if !strings.Contains(reply, "Qual deles e a crianca de ate 5 anos?") ||
		!strings.Contains(reply, "1. Joao Vitor Messias") ||
		!strings.Contains(reply, "2. Ivoneide Messias") {
		t.Fatalf("unexpected lap child assignment reply: %q", reply)
	}
}

func TestInlinePassengerDocsWithIncompleteSecondNameAsksResendWithNameAndCPF(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "eu e minha filha de 4 anos", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF ou RG).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "INBOUND", Body: "joão vitor messias 06645648103 ivoneide 46643591104", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	)

	context := collectBookingDraftContext(Session{}, history, "")
	if context.PassengerCount != 2 || context.ChildUnder5Count != 1 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected two passengers and one child context, got %+v", context)
	}
	if context.PassengerDetailsCount != 1 || context.PartialPassengerDetailsCount != 1 {
		t.Fatalf("expected one complete passenger and one partial passenger, got %+v", context)
	}
	if context.PassengerDetails[0].Name != "joão vitor messias" || context.PassengerDetails[0].Document != "06645648103" {
		t.Fatalf("unexpected complete passenger: %+v", context.PassengerDetails)
	}
	if context.PartialPassengerDetails[0].NameFragment != "ivoneide" || context.PartialPassengerDetails[0].Document != "46643591104" {
		t.Fatalf("unexpected partial passenger: %+v", context.PartialPassengerDetails)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if !strings.Contains(reply, "joão vitor messias") ||
		!strings.Contains(reply, "reenvie ivoneide") ||
		!strings.Contains(reply, "nome completo + CPF") ||
		!strings.Contains(reply, "mesma linha") ||
		strings.Contains(reply, "documentos dos 2 passageiros faltantes") ||
		strings.Contains(reply, "Tem crianca") {
		t.Fatalf("unexpected partial passenger reply: %q", reply)
	}
}

func TestInlinePassengerDocsWithIncompleteFirstNameDoesNotUseWrongOrdinal(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "2 pessoas e nenhuma crianca", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF ou RG).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "INBOUND", Body: "joão 06645648103 Ivoneide Pereira 46643591104", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	)

	context := collectBookingDraftContext(Session{}, history, "")
	if context.PassengerDetailsCount != 1 || context.PartialPassengerDetailsCount != 1 {
		t.Fatalf("expected one complete passenger and one partial passenger, got %+v", context)
	}
	if context.PassengerDetails[0].Name != "Ivoneide Pereira" || context.PassengerDetails[0].Document != "46643591104" {
		t.Fatalf("unexpected complete passenger: %+v", context.PassengerDetails)
	}
	if context.PartialPassengerDetails[0].NameFragment != "joão" || context.PartialPassengerDetails[0].Document != "06645648103" {
		t.Fatalf("unexpected partial passenger: %+v", context.PartialPassengerDetails)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if !strings.Contains(reply, "Ivoneide Pereira") ||
		!strings.Contains(reply, "reenvie joão") ||
		!strings.Contains(reply, "nome completo + CPF") ||
		!strings.Contains(reply, "mesma linha") ||
		strings.Contains(reply, "passageiro 2") {
		t.Fatalf("unexpected partial passenger reply: %q", reply)
	}
}

func TestParsePassengerClarificationSlotsEuEMeuFilhoDe4Anos(t *testing.T) {
	slots := parsePassengerClarificationSlots("eu e meu filho de 4 anos")
	if !slots.PassengerCountKnown || slots.PassengerCount != 2 {
		t.Fatalf("expected passenger_count=2, got %+v", slots)
	}
	if !slots.ChildUnder5CountKnown || slots.ChildUnder5Count != 1 {
		t.Fatalf("expected child_under_5_count=1, got %+v", slots)
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

func TestBuildBookingCreateMissingDataReplyListsExactMissingFields(t *testing.T) {
	context := BookingDraftContext{
		HasAvailabilityShown:    true,
		TripID:                  "trip-1",
		BoardStopID:             "board-1",
		Origin:                  "Santa Ines/MA",
		Destination:             "Fraiburgo/SC",
		PassengerCount:          2,
		PassengerCountKnown:     true,
		ChildUnder5CountKnown:   true,
		HasPassengerDetails:     true,
		PassengerDetailsCount:   1,
		NeedsLapChildAssignment: true,
	}

	reply := buildBookingCreateMissingDataReply(context)

	for _, want := range []string{
		"ponto de desembarque",
		"data da viagem",
		"documentos de todos os passageiros",
		"qual passageiro e a crianca de ate 5 anos",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("expected missing field %q in reply %q", want, reply)
		}
	}
	if strings.Contains(reply, unsupportedPackageSupportPhone) {
		t.Fatalf("did not expect unsupported package reply, got %q", reply)
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
