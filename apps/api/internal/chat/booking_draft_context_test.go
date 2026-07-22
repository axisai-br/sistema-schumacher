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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest()), history, "e so para mim")
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
		PassengerDetailsCount: 1,
		PassengerDetails: []BookingCreatePassengerInput{{
			Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		}},
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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0)), history, "nao")

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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, passengerOnlyStateForTest(2, PassengerCountProvenanceIncludesSpeakerComposition)), history, "eu e mais uma pessoa")
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
	context = collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 0)), history, "nao")
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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, passengerOnlyStateForTest(4, PassengerCountProvenanceIncludesSpeakerComposition)), history, "eu e mais 3 pessoas")
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
	context = collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(4, 1)), history, "sim")
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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 1)), history, "")
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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 1)), history, "")
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

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 0)), history, "")
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

func TestPassengerSlotCombinedMeAndChildDoesNotDoubleCountChildDocuments(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 1)), history, "para mim e meu filho de 4 anos")

	if context.PassengerCount != 2 || !context.PassengerCountKnown {
		t.Fatalf("expected two travelers from combined reply, got %+v", context)
	}
	if context.ChildUnder5Count != 1 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected one child under 5 from combined reply, got %+v", context)
	}
	if context.ChildUnder5AddsTraveler {
		t.Fatalf("combined reply must not add child a second time, got %+v", context)
	}
	if context.ExpectedDocumentCount != 2 {
		t.Fatalf("expected exactly two documents, got %+v", context)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if !strings.Contains(reply, "documentos dos 2 passageiros") ||
		strings.Contains(reply, "documentos dos 3 passageiros") {
		t.Fatalf("expected document request for two travelers only, got %q", reply)
	}
}

func TestPassengerSlotFlowSoEuDoesNotAssumeNoChild(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, passengerOnlyStateForTest(1, PassengerCountProvenanceSoloSpeaker)), history, "so eu")

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

func TestPassengerSlotFlowSoEuThenNaoKeepsOneDocument(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: "Tem crianca de 5 anos ou menos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: time.Now().UTC().Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0)), history, "nao")

	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger_count=1 known, got %+v", context)
	}
	if context.ChildUnder5Count != 0 || !context.ChildUnder5CountKnown {
		t.Fatalf("expected child_under_5_count=0 known, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	if context.ExpectedDocumentCount != 1 {
		t.Fatalf("expected one document when there is no child, got %+v", context)
	}
}

func TestPassengerSlotFlowSoEuThenVerboseChildAddsExpectedDocument(t *testing.T) {
	assertSoloChildReplyAddsExpectedDocument(t, "sim, meu filho de 4 anos")
}

func TestPassengerSlotFlowSoEuThenUmaCriancaAddsExpectedDocument(t *testing.T) {
	assertSoloChildReplyAddsExpectedDocument(t, "sim, uma crianca")
}

func TestPassengerSlotFlowSoEuThenTemCriancaAddsExpectedDocument(t *testing.T) {
	assertSoloChildReplyAddsExpectedDocument(t, "tem uma crianca de 4 anos")
}

func TestPassengerSlotFlowSoEuThenVaiUmaCriancaAddsExpectedDocument(t *testing.T) {
	assertSoloChildReplyAddsExpectedDocument(t, "vai uma crianca")
}

func TestPassengerSlotFlowSoEuThenSimAsksDocumentsForAdultAndChild(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: time.Now().UTC().Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, "sim")

	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected one paying passenger, got %+v", context)
	}
	if context.ChildUnder5Count != 1 || !context.ChildUnder5CountKnown || !context.ChildUnder5AddsTraveler {
		t.Fatalf("expected one added child traveler, got %+v", context)
	}
	if context.ExpectedDocumentCount != 2 {
		t.Fatalf("expected documents for adult and child, got %+v", context)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if !strings.Contains(reply, "documentos dos 2 passageiros") {
		t.Fatalf("expected document request for two travelers, got %q", reply)
	}
	run := buildBookingContinuationDraftRun(reply, action, context)
	if got := strings.TrimSpace(asString(run.RequestPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected template %s, got %q", TemplateAskDocuments, got)
	}
	if got := asInt(run.RequestPayload["passenger_count"]); got != 1 {
		t.Fatalf("expected passenger_count=1, got %d payload=%+v", got, run.RequestPayload)
	}
	if got := asInt(run.RequestPayload["expected_document_count"]); got != 2 {
		t.Fatalf("expected expected_document_count=2, got %d payload=%+v", got, run.RequestPayload)
	}
}

func TestPassengerSlotFlowSoEuThenSimOneAdultDocumentKeepsAskingDocuments(t *testing.T) {
	now := time.Now().UTC()
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "INBOUND", Body: "sim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-90 * time.Second)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, "")

	if context.PassengerCount != 1 || context.ExpectedDocumentCount != 2 || context.PassengerDetailsCount != 1 {
		t.Fatalf("expected one paying passenger, two expected documents and one collected document, got %+v", context)
	}
	if context.NeedsLapChildAssignment {
		t.Fatalf("must not ask lap child assignment before collecting child document, got %+v", context)
	}
	action := decideNextBookingStep(context)
	if action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
	reply := buildBookingContinuationReply(context, action)
	if !strings.Contains(reply, "Ainda falta o documento da crianca de ate 5 anos") ||
		strings.Contains(reply, "Qual deles e a crianca") {
		t.Fatalf("expected missing child document reply, got %q", reply)
	}
}

func TestBookingDraftVerboseChildKeepsAskingDocumentsAfterOnlyAdultDoc(t *testing.T) {
	now := time.Now().UTC()
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "INBOUND", Body: "sim, meu filho de 4 anos", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-90 * time.Second)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, "")

	if context.PassengerCount != 1 || context.ExpectedDocumentCount != 2 || context.PassengerDetailsCount != 1 {
		t.Fatalf("expected one paying passenger, two expected documents and one collected document, got %+v", context)
	}
	if context.NeedsLapChildAssignment {
		t.Fatalf("must not ask lap child assignment before collecting child document, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s, got %s", BookingNextAskPassengerDocuments, action)
	}
}

func TestPassengerSlotFlowSoEuThenSimTwoDocumentsCanAskLapChildAssignment(t *testing.T) {
	now := time.Now().UTC()
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute)},
		Message{Direction: "INBOUND", Body: "sim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086\nIvoneide Messias 04822340082", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, "")

	if context.PassengerCount != 1 || context.ExpectedDocumentCount != 2 || context.PassengerDetailsCount != 2 {
		t.Fatalf("expected one paying passenger and two collected travelers, got %+v", context)
	}
	if !context.NeedsLapChildAssignment {
		t.Fatalf("expected lap child assignment after both documents, got %+v", context)
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

func TestCollectBookingDraftContextMergesTextProgressWithCombinedDocumentSnapshot(t *testing.T) {
	history := soloChildTextAndCombinedDocumentSnapshotHistory(t)

	progress := findLatestPassengerDocumentProgress(history, Session{})
	if len(progress.Passengers) != 1 || progress.Passengers[0].Name != "Joao Vitor Messias" {
		t.Fatalf("expected outbound formatted assignment to stay excluded from textual progress, got %+v", progress)
	}

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, "")
	if context.PassengerDetailsCount != 2 || len(context.PassengerDetails) != 2 {
		t.Fatalf("expected text progress and combined snapshot to reconstruct two passengers, got %+v", context)
	}
	if context.PassengerDetails[0].Name != "Joao Vitor Messias Corrigido" ||
		context.PassengerDetails[0].BirthCity != "Santa Ines" ||
		context.PassengerDetails[1].Name != "Maria Messias" {
		t.Fatalf("expected newer structured snapshot fields and order without duplicate adult, got %+v", context.PassengerDetails)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskLapChildAssignment {
		t.Fatalf("expected combined snapshot to remain on lap child assignment, action=%s context=%+v", action, context)
	}
}

func TestCollectBookingDraftContextCorrectionPreservesOtherSnapshotPassenger(t *testing.T) {
	history := soloChildTextAndCombinedDocumentSnapshotHistory(t)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, "Maria Messias 06645648103")

	if context.PassengerDetailsCount != 2 || len(context.PassengerDetails) != 2 {
		t.Fatalf("expected correction to preserve both snapshot passengers, got %+v", context)
	}
	if context.PassengerDetails[0].Name != "Joao Vitor Messias Corrigido" || context.PassengerDetails[0].CPF != "52998224725" {
		t.Fatalf("expected adult to remain unchanged after child correction, got %+v", context.PassengerDetails)
	}
	if context.PassengerDetails[1].Name != "Maria Messias" || context.PassengerDetails[1].CPF != "06645648103" {
		t.Fatalf("expected later child correction to enrich the matching snapshot passenger, got %+v", context.PassengerDetails)
	}
}

func TestOutboundDocumentConfirmationWithoutSnapshotDoesNotReplaceInboundProgress(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "INBOUND",
			Body:             "Joao Vitor Messias CPF 52998224725\nMaria Messias RG 1234567",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Consegui identificar estes dados. Eles conferem?\n1. Joao Vitor Messias | CPF | 529.***.***-25\n2. Maria Messias | RG | 1234567",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now,
		},
	}

	progress := findLatestPassengerDocumentProgress(history, Session{})
	if len(progress.Passengers) != 2 || progress.SourceHistoryIndex != 0 {
		t.Fatalf("expected only the inbound document list as progress, got %+v", progress)
	}
	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest()), history, "")
	if context.PassengerDetailsCount != 2 {
		t.Fatalf("outbound confirmation must not reduce passenger details, got %+v", context)
	}
}

func TestOutboundDocumentBodiesNeverBecomePassengerProgress(t *testing.T) {
	tests := map[string]string{
		"confirmation with cnh": "Consegui identificar estes dados. Eles conferem?\n1. Maria Messias | CNH | 12345678901",
		"lap child assignment":  "Qual deles e a crianca de ate 5 anos?\n1. Maria Messias | RG | 1234567",
		"document request":      "Envie o documento. Exemplo: Maria Messias | RG | 1234567",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			progress := findLatestPassengerDocumentProgress([]Message{{
				Direction:        "OUTBOUND",
				Body:             body,
				ProcessingStatus: messageStatusAutomationSent,
			}}, Session{})
			if len(progress.Passengers) != 0 || len(progress.Partials) != 0 || progress.SourceText != "" {
				t.Fatalf("outbound text must never become document progress, got %+v", progress)
			}
		})
	}
}

func TestOutboundPartialDocumentTextDoesNotMergeIntoInboundProgress(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Maria 52998224725", ProcessingStatus: messageStatusAutomationSent},
		{Direction: "INBOUND", Body: "Maria Messias 52998224725", ProcessingStatus: "PROCESSED"},
	}

	merged := findLatestPassengerDocumentProgress(history, Session{})

	if merged.SourceText != "Maria Messias 52998224725" || len(merged.Partials) != 0 || len(merged.Passengers) != 1 {
		t.Fatalf("outbound partial text must not enrich inbound progress, got %+v", merged)
	}
}

func TestHistoricalPartialForDifferentPassengerRemainsUnresolved(t *testing.T) {
	session := Session{ID: "session-historical-partial", ContactKey: "5549988709221", CustomerPhone: "5549988709221", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "so pra mim", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", Confidence: 0.98},
		},
	})
	confirmation := history[len(history)-1]
	history = append(history[:len(history)-1],
		Message{Direction: "INBOUND", Body: "ivoneide 46643591104", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-time.Minute)},
		confirmation,
	)

	context := collectBookingDraftContext(session, history, "sim")
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if context.PassengerDetailsCount != 1 || context.PartialPassengerDetailsCount != 1 {
		t.Fatalf("unrelated historical partial must remain beside the later complete passenger, got %+v", context)
	}
	if context.PartialPassengerDetails[0].Document != "46643591104" || len(readiness.UnresolvedPartials) != 1 || readiness.Ready {
		t.Fatalf("historical partial identity must keep canonical readiness blocked, got %+v", readiness)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("unrelated historical partial must block booking_create, got %+v", input)
	}
}

func TestCanonicalEvidenceTimelineKeepsLatestDuplicatePartial(t *testing.T) {
	events := []passengerDocumentEvidence{
		{SourceHistoryIndex: 1, Kind: passengerDocumentEvidenceInbound, Partials: []BookingPassengerDocumentPartial{{NameFragment: "ivoneide", DocumentType: "CPF", Document: "46643591104"}}},
		{SourceHistoryIndex: 2, Kind: passengerDocumentEvidenceInbound, Partials: []BookingPassengerDocumentPartial{{NameFragment: "ivoneide pereira", DocumentType: "CPF", Document: "466.435.911-04"}}},
	}

	progress := reconstructPassengerDocumentEvidence(events, 0)
	if len(progress.Partials) != 1 || progress.Partials[0].NameFragment != "ivoneide pereira" {
		t.Fatalf("latest partial for the same normalized identity must win, got %+v", progress.Partials)
	}
}

func TestCorrectionChronologyNewerExtractWinsOlderCorrection(t *testing.T) {
	history := documentCorrectionBookingHistory(time.Now().UTC())
	extract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Claudecir Schumacher", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901", Confidence: 0.99},
		},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(extract)}
	history = append(history, Message{
		Direction:         "OUTBOUND",
		Body:              buildConfirmExtractedDocumentReply(extract),
		ProcessingStatus:  messageStatusAutomationDraft,
		ReceivedAt:        time.Now().UTC(),
		Payload:           map[string]interface{}{"tool_context": toolContext},
		NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
	})

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0)), history, "sim")
	if len(context.PassengerDetails) != 1 || context.PassengerDetails[0].DocumentType != "CNH" || context.PassengerDetails[0].Document != "12345678901" {
		t.Fatalf("newer CNH snapshot must win over the older CPF correction, got %+v", context.PassengerDetails)
	}
}

func TestCorrectionChronologyNewerCorrectionWinsOlderExtract(t *testing.T) {
	history := knownPassengerCountDocumentConfirmationHistory(t, "so pra mim", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Claudecir Schumacher", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901", Confidence: 0.98},
		},
	})

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0)), history, "corrigir documento para CPF 52998224725")
	if len(context.PassengerDetails) != 1 || context.PassengerDetails[0].DocumentType != "CPF" || context.PassengerDetails[0].Document != "52998224725" {
		t.Fatalf("newer CPF correction must win over the older CNH snapshot, got %+v", context.PassengerDetails)
	}
}

func TestCanonicalEvidenceTimelineCurrentTurnWinsNonEmptyFields(t *testing.T) {
	history := knownPassengerCountDocumentConfirmationHistory(t, "so pra mim", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Nome Intermediario", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthCity: "Santa Ines", Confidence: 0.98},
		},
	})
	confirmation := history[len(history)-1]
	history = append(history[:len(history)-1],
		Message{Direction: "INBOUND", Body: "Nome Antigo CPF 52998224725", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-time.Minute)},
		confirmation,
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0)), history, "nome: Nome Final CPF 52998224725")
	if len(context.PassengerDetails) != 1 || context.PassengerDetails[0].Name != "Nome Final" || context.PassengerDetails[0].BirthCity != "Santa Ines" {
		t.Fatalf("current turn must win non-empty matching fields without erasing the extract, got %+v", context.PassengerDetails)
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

func TestExpectedPassengerDocumentCountDoesNotPromoteKnownCountFromReconstructedDetails(t *testing.T) {
	context := BookingDraftContext{
		HasAvailabilityShown:  true,
		TripID:                "trip-1",
		BoardStopID:           "board-1",
		AlightStopID:          "alight-1",
		Origin:                "Santa Ines/MA",
		Destination:           "Fraiburgo/SC",
		TripDate:              "2026-07-20",
		PassengerCount:        1,
		PassengerCountKnown:   true,
		ChildUnder5CountKnown: true,
		HasPassengerDetails:   true,
		PassengerDetailsCount: 2,
		PassengerDetails: []BookingCreatePassengerInput{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567"},
		},
	}

	if expected := expectedPassengerDocumentCount(context); expected != 1 {
		t.Fatalf("known passenger count must remain authoritative, got %d", expected)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerClarification {
		t.Fatalf("mismatched reconstructed details must stay blocked, got %s", action)
	}
	reply := buildBookingContinuationReply(context, BookingNextAskPassengerClarification)
	if !strings.Contains(reply, "informou 1 passageiros") || !strings.Contains(reply, "documentos de 2 pessoas") {
		t.Fatalf("overflow must request passenger count confirmation, got %q", reply)
	}
}

func TestExpectedPassengerDocumentCountUsesDetailsOnlyWhenCountIsUnknown(t *testing.T) {
	context := BookingDraftContext{
		PassengerCount:        1,
		PassengerCountKnown:   false,
		PassengerDetailsCount: 2,
	}

	if expected := expectedPassengerDocumentCount(context); expected != 2 {
		t.Fatalf("reconstructed details must be the fallback while passenger count is unknown, got %d", expected)
	}
}

func TestCollectBookingDraftContextKeepsCombinedDeclaredCountWhenSnapshotHasExcessPassenger(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	extract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 3,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", Confidence: 0.98},
			{Name: "Ana Messias", DocumentType: "CNH", Document: "12345678901", Confidence: 0.98},
		},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(extract)}
	history = append(history,
		Message{Direction: "INBOUND", Body: "eu e meu filho de 4 anos", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos 2 passageiros.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-time.Minute)},
		Message{
			Direction:         "OUTBOUND",
			Body:              buildConfirmExtractedDocumentReply(extract),
			ProcessingStatus:  messageStatusAutomationDraft,
			ReceivedAt:        now.Add(-30 * time.Second),
			Payload:           map[string]interface{}{"tool_context": toolContext},
			NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
		},
	)

	passengerSession := sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 1))
	context := collectBookingDraftContext(passengerSession, history, "sim")
	if !context.PassengerCountKnown || context.PassengerCount != 2 || context.ExpectedDocumentCount != 2 {
		t.Fatalf("declared passenger count must remain authoritative, got %+v", context)
	}
	if context.PassengerDetailsCount != 3 || context.ChildUnder5AddsTraveler || context.NeedsLapChildAssignment {
		t.Fatalf("expected visible overflow without lap-child promotion, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerClarification {
		t.Fatalf("expected overflow to request passenger clarification, got %s", action)
	}
	reply := buildBookingContinuationReply(context, BookingNextAskPassengerClarification)
	if !strings.Contains(reply, "informou 2 passageiros") || !strings.Contains(reply, "documentos de 3 pessoas") || strings.Contains(reply, "Ainda falta") {
		t.Fatalf("expected explicit overflow reply instead of missing-document copy, got %q", reply)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(passengerSession, history, "sim"); ok {
		t.Fatalf("overflow must not authorize booking_create, got %+v", input)
	}
}

func TestCollectBookingDraftContextDoesNotPromoteUnknownCountFromDocuments(t *testing.T) {
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	delete(asMap(asMap(history[0].Payload["tool_context"])[toolNameAvailabilitySearch]), "qtd")
	now := time.Now().UTC()
	extract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", Confidence: 0.98},
		},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(extract)}
	history = append(history,
		Message{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos passageiros.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-time.Minute)},
		Message{
			Direction:         "OUTBOUND",
			Body:              buildConfirmExtractedDocumentReply(extract),
			ProcessingStatus:  messageStatusAutomationDraft,
			ReceivedAt:        now.Add(-30 * time.Second),
			Payload:           map[string]interface{}{"tool_context": toolContext},
			NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
		},
	)

	session := sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest())
	context := collectBookingDraftContext(session, history, "sim")
	if context.PassengerCountKnown || context.PassengerCount != 0 {
		t.Fatalf("documents must not turn an unknown passenger count into a declaration, got %+v", context)
	}
	if context.ExpectedDocumentCount != 2 || context.PassengerDetailsCount != 2 {
		t.Fatalf("expected document count only as temporary reconstruction fallback, got %+v", context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerClarification {
		t.Fatalf("unknown passenger count must request clarification, got %s", action)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("unknown passenger count must not authorize booking_create, got %+v", input)
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

func assertSoloChildReplyAddsExpectedDocument(t *testing.T, childReply string) {
	t.Helper()
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: time.Now().UTC().Add(-90 * time.Second)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: time.Now().UTC().Add(-60 * time.Second)},
	)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, soloPlusChildPassengerStateForTest()), history, childReply)

	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected one paying passenger for %q, got %+v", childReply, context)
	}
	if context.ChildUnder5Count != 1 || !context.ChildUnder5CountKnown || !context.ChildUnder5AddsTraveler {
		t.Fatalf("expected one added child traveler for %q, got %+v", childReply, context)
	}
	if context.ExpectedDocumentCount != 2 {
		t.Fatalf("expected adult and child documents for %q, got %+v", childReply, context)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskPassengerDocuments {
		t.Fatalf("expected next action %s for %q, got %s", BookingNextAskPassengerDocuments, childReply, action)
	}
}

func soloChildTextAndCombinedDocumentSnapshotHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Now().UTC()
	history := passengerSlotAvailabilityHistory(t, "Perfeito. A passagem e so para voce ou vai mais alguem junto?")
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		Message{Direction: "INBOUND", Body: "sim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 52998224725", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		Message{
			Direction:        "OUTBOUND",
			Body:             "Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Maria Messias",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
						Mode:                   "EXTRACTED",
						ExpectedPassengerCount: 2,
						Passengers: []DocumentExtractPassenger{
							{Name: "Joao Vitor Messias Corrigido", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthCity: "Santa Ines", Confidence: 1},
							{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567", Confidence: 1},
						},
					}),
				},
			},
		},
	)
	return history
}

func TestSequentialDocumentMergePreservesAdultThenChildOrder(t *testing.T) {
	adult := BookingCreatePassengerInput{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}
	child := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567"}

	merged := mergeBookingPassengerDetails([]BookingCreatePassengerInput{adult}, []BookingCreatePassengerInput{child})

	if len(merged) != 2 || merged[0].Document != adult.Document || merged[1].Document != child.Document {
		t.Fatalf("expected adult then child in deterministic order, got %+v", merged)
	}
}

func TestSequentialDocumentMergePreservesChildThenAdultOrder(t *testing.T) {
	child := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567", IsLapChild: true}
	adult := BookingCreatePassengerInput{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}

	merged := mergeBookingPassengerDetails([]BookingCreatePassengerInput{child}, []BookingCreatePassengerInput{adult})

	if len(merged) != 2 || merged[0].Document != child.Document || merged[1].Document != adult.Document {
		t.Fatalf("expected child then adult in deterministic order, got %+v", merged)
	}
}

func TestDocumentMergeLatestWinsSamePassengerFields(t *testing.T) {
	prior := BookingCreatePassengerInput{Name: "Joao Vitor Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901", CPF: "52998224725"}
	latest := BookingCreatePassengerInput{Name: "Nome Corrigido", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthDate: "1990-03-04", BirthCity: "Santa Ines"}

	merged := mergeBookingPassengerDetails([]BookingCreatePassengerInput{prior}, []BookingCreatePassengerInput{latest})

	if len(merged) != 1 {
		t.Fatalf("expected duplicate document to remain one passenger, got %+v", merged)
	}
	if merged[0].Name != latest.Name || merged[0].Document != latest.Document || merged[0].DocumentType != latest.DocumentType || merged[0].BirthDate != latest.BirthDate || merged[0].BirthCity != latest.BirthCity {
		t.Fatalf("expected newer non-empty fields to win, got %+v", merged[0])
	}
}

func TestDocumentMergeLatestWinsAndEmptyFieldsPreserveOlderValues(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Antiga", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2021-01-01", BirthCity: "Videira", Phone: "5549999999999", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Corrigida", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-02-02",
	}

	merged := mergeBookingPassengerDetails([]BookingCreatePassengerInput{older}, []BookingCreatePassengerInput{newer})

	if len(merged) != 1 || merged[0].Name != newer.Name || merged[0].BirthDate != newer.BirthDate {
		t.Fatalf("expected newer name and birth date, got %+v", merged)
	}
	if merged[0].BirthCity != older.BirthCity || merged[0].Phone != older.Phone || !merged[0].IsLapChild {
		t.Fatalf("expected empty newer fields and lap-child state to preserve older values, got %+v", merged[0])
	}
}

func TestLatestLapChildAgeAdultExtractClearsOldMarker(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "1990-03-12",
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)

	if len(merged) != 1 || merged[0].IsLapChild {
		t.Fatalf("newer explicit adult age must clear the old lap-child marker, got %+v", merged)
	}
}

func TestCanonicalEvidenceTimelineLatestLapChildAgeWins(t *testing.T) {
	oldChild := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newAdult := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "1990-03-12",
	}
	events := []passengerDocumentEvidence{
		{SourceHistoryIndex: 1, Kind: passengerDocumentEvidenceExtract, Passengers: []BookingCreatePassengerInput{oldChild}},
		{SourceHistoryIndex: 2, Kind: passengerDocumentEvidenceExtract, Passengers: []BookingCreatePassengerInput{newAdult}},
	}

	progress := reconstructPassengerDocumentEvidence(events, 1)
	if len(progress.Passengers) != 1 || progress.Passengers[0].IsLapChild {
		t.Fatalf("latest explicit adult age must win in the canonical extract timeline, got %+v", progress.Passengers)
	}
}

func TestCanonicalEvidenceTimelineLapChildNameFallbackPreservesMarker(t *testing.T) {
	oldChild := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newAdult := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "1990-03-12",
	}
	events := []passengerDocumentEvidence{
		{SourceHistoryIndex: 1, Kind: passengerDocumentEvidenceExtract, Passengers: []BookingCreatePassengerInput{oldChild}},
		{SourceHistoryIndex: 2, Kind: passengerDocumentEvidenceExtract, Passengers: []BookingCreatePassengerInput{newAdult}},
	}

	progress := reconstructPassengerDocumentEvidence(events, 1)
	if len(progress.Passengers) != 1 || progress.Passengers[0].DocumentType != "CNH" || !progress.Passengers[0].IsLapChild {
		t.Fatalf("timeline name fallback may replace RG with CNH but must preserve the old marker, got %+v", progress.Passengers)
	}
}

func TestLatestLapChildAgeMissingBirthPreservesOldMarker(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)

	if len(merged) != 1 || !merged[0].IsLapChild {
		t.Fatalf("extract without explicit age must preserve the old lap-child marker, got %+v", merged)
	}
}

func TestLatestLapChildAgeChildExtractSetsMarker(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "1990-03-12",
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "2022-05-10", IsLapChild: true,
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)

	if len(merged) != 1 || !merged[0].IsLapChild {
		t.Fatalf("newer explicit child age must set the lap-child marker, got %+v", merged)
	}
}

func TestLatestLapChildAgeAdultExtractOnlyChangesMatchingPassenger(t *testing.T) {
	adult := BookingCreatePassengerInput{
		Name: "Joao Messias", DocumentType: "CPF", Document: "46643591104", CPF: "46643591104",
		BirthDate: "1988-01-02",
	}
	incorrectChild := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	corrected := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "1990-03-12",
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{adult, incorrectChild},
		[]BookingCreatePassengerInput{corrected},
		2,
	)

	if len(merged) != 2 || merged[0] != adult || merged[1].IsLapChild {
		t.Fatalf("adult extract must change only the matching passenger, got %+v", merged)
	}
}

func TestLatestLapChildAgeAdultExtractMakesZeroChildReadinessReady(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
		BirthDate: "1990-03-12",
	}
	passengers := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)
	context := BookingDraftContext{
		HasAvailabilityShown: true,
		TripID:               "trip-age-1", BoardStopID: "board-age-1", AlightStopID: "alight-age-1",
		Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", TripDate: "2026-08-25",
		PassengerCount: 1, PassengerCountKnown: true,
		ChildUnder5Count: 0, ChildUnder5CountKnown: true,
		HasPassengerDetails: true, PassengerDetailsCount: 1, PassengerDetails: passengers,
	}

	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if !readiness.Ready || !readiness.LapChildAssignmentComplete {
		t.Fatalf("adult evidence must remove the obsolete marker for zero declared children, got %+v", readiness)
	}
	if action := decideNextBookingStep(context); action != BookingNextCallCreate {
		t.Fatalf("obsolete marker must not ask lap-child assignment, got %s", action)
	}
}

func TestLatestLapChildAgeAdultExtractKeepsExpectedChildReadinessBlocked(t *testing.T) {
	incorrectChild := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	correctedAdult := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "1990-03-12",
	}
	otherAdult := BookingCreatePassengerInput{
		Name: "Joao Messias", DocumentType: "CPF", Document: "46643591104", CPF: "46643591104",
		BirthDate: "1988-01-02",
	}
	passengers := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{incorrectChild, otherAdult},
		[]BookingCreatePassengerInput{correctedAdult},
		2,
	)
	context := BookingDraftContext{
		HasAvailabilityShown: true,
		TripID:               "trip-age-2", BoardStopID: "board-age-2", AlightStopID: "alight-age-2",
		Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", TripDate: "2026-08-25",
		PassengerCount: 2, PassengerCountKnown: true,
		ChildUnder5Count: 1, ChildUnder5CountKnown: true,
		HasPassengerDetails: true, PassengerDetailsCount: 2, PassengerDetails: passengers,
	}

	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if readiness.Ready || readiness.LapChildAssignmentComplete {
		t.Fatalf("one declared child must remain blocked until exactly one valid passenger is marked, got %+v", readiness)
	}
	if action := decideNextBookingStep(context); action != BookingNextAskLapChildAssignment {
		t.Fatalf("expected exact child assignment after removing obsolete marker, got %s", action)
	}
}

func TestLapChildNameFallbackIdentifierChangesPreservesOldTrue(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "1990-03-12",
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)

	if len(merged) != 1 || merged[0].DocumentType != "CNH" || merged[0].Document != "12345678901" || !merged[0].IsLapChild {
		t.Fatalf("name-only RG to CNH fallback must preserve old true marker, got %+v", merged)
	}
}

func TestLapChildNameFallbackIdentifierChangesPreservesOldFalse(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "1990-03-12",
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "2022-05-10", IsLapChild: true,
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)

	if len(merged) != 1 || merged[0].DocumentType != "CNH" || merged[0].Document != "12345678901" || merged[0].IsLapChild {
		t.Fatalf("name-only RG to CNH fallback must preserve old false marker, got %+v", merged)
	}
}

func TestLapChildNameFallbackChangesOnlyMatchingPassenger(t *testing.T) {
	other := BookingCreatePassengerInput{
		Name: "Joao Messias", DocumentType: "CPF", Document: "46643591104", CPF: "46643591104",
		BirthDate: "1988-01-02",
	}
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "1990-03-12",
	}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{other, older},
		[]BookingCreatePassengerInput{newer},
		2,
	)

	if len(merged) != 2 || merged[0] != other || merged[1].DocumentType != "CNH" || !merged[1].IsLapChild {
		t.Fatalf("name fallback must update only its unique slot and preserve the other passenger, got %+v", merged)
	}
}

func TestLapChildDocumentMatchNoMatchAppendsAndBlocksExcess(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Ana Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "1990-03-12",
	}
	passengers := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{older},
		[]BookingCreatePassengerInput{newer},
		1,
	)
	context := BookingDraftContext{
		PassengerCount: 1, PassengerCountKnown: true,
		ChildUnder5Count: 1, ChildUnder5CountKnown: true,
		PassengerDetailsCount: len(passengers), PassengerDetails: passengers,
	}

	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if len(passengers) != 2 || readiness.QuantityStatus != bookingCreateQuantityExcess || readiness.Ready {
		t.Fatalf("unmatched extract must append and keep canonical excess blocked, passengers=%+v readiness=%+v", passengers, readiness)
	}
}

func TestPassengerSnapshotNameFallbackAdultToChildReplayPreservesAdult(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "1990-03-12",
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	snapshot := mergeCurrentDocumentExtractionPassengerSnapshot(
		newBookingPassengerSnapshot([]BookingCreatePassengerInput{older}, 1),
		[]BookingCreatePassengerInput{newer},
		1,
	)
	if got := snapshot.Passengers[0].LapChildSource; got != bookingPassengerLapChildSourceNameFallbackPreserved {
		t.Fatalf("expected nominal fallback provenance, got %q", got)
	}

	for replay := 1; replay <= 2; replay++ {
		message := bookingPassengerSnapshotReplayMessage(snapshot, DocumentExtractResult{
			Mode: "EXTRACTED", ExpectedPassengerCount: 1,
			Passengers: []DocumentExtractPassenger{{
				Name: newer.Name, DocumentType: newer.DocumentType, Document: newer.Document, CNH: newer.CNH,
				BirthDate: newer.BirthDate, IsLapChild: true, Confidence: 0.98,
			}},
		})
		events := passengerDocumentEvidenceTimeline([]Message{message}, "", Session{}, "2026-08-25")
		if len(events) != 1 || events[0].Kind != passengerDocumentEvidenceCanonicalSnapshot {
			t.Fatalf("replay %d must use only the canonical snapshot event, got %+v", replay, events)
		}
		progress := reconstructPassengerDocumentEvidence(events, 1)
		if len(progress.Passengers) != 1 || progress.Passengers[0].IsLapChild {
			t.Fatalf("replay %d changed preserved adult classification: %+v", replay, progress.Passengers)
		}
		snapshot = progress.Snapshot
	}
}

func TestPassengerSnapshotNameFallbackChildToAdultReplayPreservesChild(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
		BirthDate: "2022-05-10", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901",
		BirthDate: "1990-03-12",
	}
	snapshot := mergeCurrentDocumentExtractionPassengerSnapshot(
		newBookingPassengerSnapshot([]BookingCreatePassengerInput{older}, 1),
		[]BookingCreatePassengerInput{newer},
		1,
	)
	message := bookingPassengerSnapshotReplayMessage(snapshot, DocumentExtractResult{
		Mode: "EXTRACTED", ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{{
			Name: newer.Name, DocumentType: newer.DocumentType, Document: newer.Document, CNH: newer.CNH,
			BirthDate: newer.BirthDate, Confidence: 0.98,
		}},
	})
	progress := reconstructPassengerDocumentEvidence(
		passengerDocumentEvidenceTimeline([]Message{message}, "", Session{}, "2026-08-25"),
		1,
	)
	if len(progress.Passengers) != 1 || !progress.Passengers[0].IsLapChild {
		t.Fatalf("name-only adult extract must not clear the restored child marker, got %+v", progress.Passengers)
	}
}

func TestPassengerSnapshotDocumentIdentityAgeChangesSurviveReplay(t *testing.T) {
	cases := []struct {
		name      string
		older     BookingCreatePassengerInput
		newer     BookingCreatePassengerInput
		wantChild bool
	}{
		{
			name:  "child to adult",
			older: BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthDate: "2022-05-10", IsLapChild: true},
			newer: BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthDate: "1990-03-12"},
		},
		{
			name:      "adult to child",
			older:     BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthDate: "1990-03-12"},
			newer:     BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", BirthDate: "2022-05-10", IsLapChild: true},
			wantChild: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := mergeCurrentDocumentExtractionPassengerSnapshot(
				newBookingPassengerSnapshot([]BookingCreatePassengerInput{tc.older}, 1),
				[]BookingCreatePassengerInput{tc.newer},
				1,
			)
			if got := snapshot.Passengers[0].LapChildSource; got != bookingPassengerLapChildSourceDocumentIdentityAge {
				t.Fatalf("expected document identity age provenance, got %q", got)
			}
			message := bookingPassengerSnapshotReplayMessage(snapshot, DocumentExtractResult{
				Mode: "EXTRACTED", ExpectedPassengerCount: 1,
				Passengers: []DocumentExtractPassenger{{
					Name: tc.newer.Name, DocumentType: tc.newer.DocumentType, Document: tc.newer.Document, CPF: tc.newer.CPF,
					BirthDate: tc.newer.BirthDate, IsLapChild: tc.newer.IsLapChild, Confidence: 0.98,
				}},
			})
			progress := reconstructPassengerDocumentEvidence(
				passengerDocumentEvidenceTimeline([]Message{message}, "", Session{}, "2026-08-25"),
				1,
			)
			if len(progress.Passengers) != 1 || progress.Passengers[0].IsLapChild != tc.wantChild {
				t.Fatalf("document identity result changed on replay: %+v", progress.Passengers)
			}
		})
	}
}

func TestPassengerSnapshotNameFallbackAllowsFutureDocumentIdentityAgeUpdate(t *testing.T) {
	oldAdult := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567", BirthDate: "1990-03-12",
	}
	newChildCNH := BookingCreatePassengerInput{
		Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901", BirthDate: "2022-05-10", IsLapChild: true,
	}
	persistedFallback := mergeCurrentDocumentExtractionPassengerSnapshot(
		newBookingPassengerSnapshot([]BookingCreatePassengerInput{oldAdult}, 1),
		[]BookingCreatePassengerInput{newChildCNH},
		1,
	)
	if persistedFallback.Passengers[0].Passenger.IsLapChild {
		t.Fatalf("initial name fallback must preserve adult marker, got %+v", persistedFallback)
	}

	future := mergeCurrentDocumentExtractionPassengerSnapshot(persistedFallback, []BookingCreatePassengerInput{newChildCNH}, 1)
	if !future.Passengers[0].Passenger.IsLapChild || future.Passengers[0].LapChildSource != bookingPassengerLapChildSourceDocumentIdentityAge {
		t.Fatalf("future matching CNH must allow the explicit child age to win, got %+v", future)
	}
}

func TestPassengerSnapshotExplicitAssignmentByIndexSurvivesReplay(t *testing.T) {
	now := time.Now().UTC()
	history := lapChildBookingHistory(now, "Joao Vitor Messias CPF 84960815086\nIvoneide Messias CPF 04822340082")
	history = append(history, Message{
		Direction: "OUTBOUND", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now,
		Body: "Recebi os dados dos 2 passageiros. Qual passageiro e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias",
	})
	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(2, 1)), history, "2")
	if len(context.PassengerSnapshot.Passengers) != 2 ||
		context.PassengerSnapshot.Passengers[0].LapChildSource != bookingPassengerLapChildSourceExplicitAssignment ||
		context.PassengerSnapshot.Passengers[1].LapChildSource != bookingPassengerLapChildSourceExplicitAssignment {
		t.Fatalf("explicit index assignment must be persisted for every classified passenger, got %+v", context.PassengerSnapshot)
	}
	if context.PassengerSnapshot.Passengers[0].Passenger.IsLapChild || !context.PassengerSnapshot.Passengers[1].Passenger.IsLapChild {
		t.Fatalf("expected passenger 2 as the only explicit lap child, got %+v", context.PassengerSnapshot.Passengers)
	}

	message := bookingPassengerSnapshotReplayMessage(context.PassengerSnapshot, DocumentExtractResult{})
	progress := reconstructPassengerDocumentEvidence(
		passengerDocumentEvidenceTimeline([]Message{message}, "", Session{}, context.TripDate),
		2,
	)
	if len(progress.Passengers) != 2 || progress.Passengers[0].IsLapChild || !progress.Passengers[1].IsLapChild {
		t.Fatalf("explicit assignment changed on replay, got %+v", progress.Passengers)
	}
}

func TestPassengerSnapshotProvenanceDoesNotLeakAcrossPassengers(t *testing.T) {
	adult := BookingCreatePassengerInput{Name: "Joao Messias", DocumentType: "CPF", Document: "46643591104", CPF: "46643591104", BirthDate: "1988-01-02"}
	child := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567", BirthDate: "2022-05-10", IsLapChild: true}
	replacement := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901", BirthDate: "1990-03-12"}

	snapshot := newBookingPassengerSnapshot([]BookingCreatePassengerInput{adult, child}, 2)
	merged := mergeCurrentDocumentExtractionPassengerSnapshot(snapshot, []BookingCreatePassengerInput{replacement}, 2)
	if len(merged.Passengers) != 2 || merged.Passengers[0].Passenger != adult {
		t.Fatalf("other passenger changed during named replacement, got %+v", merged.Passengers)
	}
	if merged.Passengers[0].LapChildSource != bookingPassengerLapChildSourceDocumentIdentityAge {
		t.Fatalf("other passenger provenance changed, got %q", merged.Passengers[0].LapChildSource)
	}
	if merged.Passengers[1].LapChildSource != bookingPassengerLapChildSourceNameFallbackPreserved || !merged.Passengers[1].Passenger.IsLapChild {
		t.Fatalf("replacement passenger did not retain isolated fallback provenance, got %+v", merged.Passengers[1])
	}
}

func TestPassengerSnapshotLegacyDocumentExtractStillUsesAgeReconstruction(t *testing.T) {
	legacy := DocumentExtractResult{
		Mode: "EXTRACTED", ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{{
			Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567", BirthDate: "2022-05-10", Confidence: 0.98,
		}},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(legacy)}
	message := Message{
		Direction: "OUTBOUND", ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{"tool_context": toolContext},
	}
	events := passengerDocumentEvidenceTimeline([]Message{message}, "", Session{}, "2026-08-25")
	if len(events) != 1 || events[0].Kind != passengerDocumentEvidenceExtract {
		t.Fatalf("legacy payload must keep document_extract evidence, got %+v", events)
	}
	progress := reconstructPassengerDocumentEvidence(events, 1)
	if len(progress.Passengers) != 1 || !progress.Passengers[0].IsLapChild {
		t.Fatalf("legacy document_extract must keep age reconstruction, got %+v", progress.Passengers)
	}
}

func bookingPassengerSnapshotReplayMessage(snapshot bookingPassengerSnapshot, extract DocumentExtractResult) Message {
	toolContext := map[string]interface{}{
		toolNameBookingPassengerSnapshot: buildBookingPassengerSnapshotPayload(snapshot),
	}
	if strings.TrimSpace(extract.Mode) != "" {
		toolContext[toolNameDocumentExtract] = buildDocumentExtractResponsePayload(extract)
	}
	return Message{
		Direction:        "OUTBOUND",
		ProcessingStatus: messageStatusAutomationSent,
		Payload:          map[string]interface{}{"tool_context": toolContext},
		NormalizedPayload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameBookingPassengerSnapshot: buildBookingPassengerSnapshotPayload(snapshot),
			},
		},
	}
}

func TestDocumentMergeLatestWinsAllUpdateableFields(t *testing.T) {
	older := BookingCreatePassengerInput{
		Name: "Nome Antigo", Document: "52998224725", DocumentType: "CPF", CPF: "52998224725",
		RG: "1111111", CNH: "11111111111", BirthDate: "1990-01-01", BirthCertificateNumber: "CERT-OLD",
		BirthCity: "Cidade Antiga", Phone: "5511111111111", Email: "old@example.com", Notes: "nota antiga", IsLapChild: true,
	}
	newer := BookingCreatePassengerInput{
		Name: "Nome Novo", Document: "52998224725", DocumentType: "CPF", CPF: "52998224725",
		RG: "2222222", CNH: "22222222222", BirthDate: "1991-02-02", BirthCertificateNumber: "CERT-NEW",
		BirthCity: "Cidade Nova", Phone: "5522222222222", Email: "new@example.com", Notes: "nota nova",
	}
	expected := newer
	expected.IsLapChild = true

	merged := mergeBookingPassengerDetails([]BookingCreatePassengerInput{older}, []BookingCreatePassengerInput{newer})

	if len(merged) != 1 || merged[0] != expected {
		t.Fatalf("expected all newer non-empty fields and preserved lap-child flag, got %+v", merged)
	}
}

func TestDocumentMergeCorrectionPreservesOtherPassengerAndOrder(t *testing.T) {
	adult := BookingCreatePassengerInput{Name: "Joao", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}
	childOlder := BookingCreatePassengerInput{Name: "Maria Antiga", DocumentType: "RG", Document: "1234567", RG: "1234567", BirthDate: "2021-01-01"}
	childNewer := BookingCreatePassengerInput{Name: "Maria Corrigida", DocumentType: "RG", Document: "1234567", RG: "1234567", BirthDate: "2022-02-02"}

	merged := mergeBookingPassengerDetails(
		[]BookingCreatePassengerInput{adult, childOlder},
		[]BookingCreatePassengerInput{childNewer},
	)

	if len(merged) != 2 || merged[0].CPF != adult.CPF || merged[1].Name != childNewer.Name || merged[1].BirthDate != childNewer.BirthDate {
		t.Fatalf("expected unchanged adult and corrected child in stable order, got %+v", merged)
	}
}

func TestDocumentMergeDoesNotDeduplicateDifferentDocumentsByName(t *testing.T) {
	prior := BookingCreatePassengerInput{Name: "Mesmo Nome", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}
	latest := BookingCreatePassengerInput{Name: "Mesmo Nome", DocumentType: "RG", Document: "1234567", RG: "1234567"}

	merged := mergeBookingPassengerDetails([]BookingCreatePassengerInput{prior}, []BookingCreatePassengerInput{latest})

	if len(merged) != 2 {
		t.Fatalf("different documents must not be deduplicated by name, got %+v", merged)
	}
}

func TestCurrentDocumentExtractionReplacesUniqueNamedSlotWhenDocumentChanges(t *testing.T) {
	adult := BookingCreatePassengerInput{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}
	childWithRG := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567"}
	childWithCNH := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901"}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{adult, childWithRG},
		[]BookingCreatePassengerInput{childWithCNH},
		2,
	)

	if len(merged) != 2 {
		t.Fatalf("document replacement must not append a passenger, got %+v", merged)
	}
	if merged[0].CPF != adult.CPF || merged[1].Name != childWithCNH.Name || merged[1].DocumentType != "CNH" || merged[1].Document != childWithCNH.Document {
		t.Fatalf("expected stable adult slot and updated child document, got %+v", merged)
	}
}

func TestCurrentDocumentExtractionDoesNotReplaceSameNameWhileSlotIsAvailable(t *testing.T) {
	first := BookingCreatePassengerInput{Name: "Mesmo Nome", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}
	second := BookingCreatePassengerInput{Name: "Mesmo Nome", DocumentType: "RG", Document: "1234567", RG: "1234567"}

	merged := mergeCurrentDocumentExtractionPassengerDetails(
		[]BookingCreatePassengerInput{first},
		[]BookingCreatePassengerInput{second},
		2,
	)

	if len(merged) != 2 || merged[0].Document != first.Document || merged[1].Document != second.Document {
		t.Fatalf("available slot must preserve distinct passengers with the same name, got %+v", merged)
	}
}

func TestCurrentDocumentExtractionDoesNotReplaceDifferentNameWithoutDocumentIdentity(t *testing.T) {
	older := []BookingCreatePassengerInput{
		{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"},
		{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567"},
	}
	newer := BookingCreatePassengerInput{Name: "Outra Pessoa", DocumentType: "CNH", Document: "12345678901", CNH: "12345678901"}

	merged := mergeCurrentDocumentExtractionPassengerDetails(older, []BookingCreatePassengerInput{newer}, 2)

	if len(merged) != 3 || merged[2].Name != newer.Name {
		t.Fatalf("ambiguous identity must append and remain blocked by the count mismatch, got %+v", merged)
	}
}

func TestDocumentMergeKeepsTwoPassengersFromSameExtraction(t *testing.T) {
	passengers := []BookingCreatePassengerInput{
		{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"},
		{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567"},
	}

	merged := mergeBookingPassengerDetails(nil, passengers)

	if len(merged) != 2 || merged[0].Document != passengers[0].Document || merged[1].Document != passengers[1].Document {
		t.Fatalf("expected both passengers from the same extraction, got %+v", merged)
	}
}

func TestDocumentMergeKeepsSoloPassenger(t *testing.T) {
	passenger := BookingCreatePassengerInput{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}

	merged := mergeBookingPassengerDetails(nil, []BookingCreatePassengerInput{passenger})

	if len(merged) != 1 || merged[0].Document != passenger.Document {
		t.Fatalf("expected solo passenger unchanged, got %+v", merged)
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
