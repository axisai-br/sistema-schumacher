package chat

import (
	"testing"
	"time"
)

func TestParseBookingCreateInputBlocksLapChildWithoutAssignment(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei estas opcoes para Petrolandia/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-6 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Destination: "Petrolandia/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-1",
								BoardStopID:            "board-1",
								AlightStopID:           "alight-1",
								OriginDisplayName:      "Moncao/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "09:00",
								TripDate:               "2026-04-26",
							},
							{
								TripID:                 "trip-2",
								BoardStopID:            "board-2",
								AlightStopID:           "alight-2",
								OriginDisplayName:      "Igarape do Meio/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "11:00",
								TripDate:               "2026-05-11",
							},
							{
								TripID:                 "trip-3",
								BoardStopID:            "board-3",
								AlightStopID:           "alight-3",
								OriginDisplayName:      "Santa Ines/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "12:00",
								TripDate:               "2026-05-25",
							},
						},
					}),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "a primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-5 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - confirmei a primeira opcao: 26/04/2026, saida de Moncao as 09:00 para Petrolandia/SC. A passagem e so para voce ou tem mais pessoas, e ha crianca de ate 5 anos viajando?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
		},
		{
			Direction:        "INBOUND",
			Body:             "sim eu e minha filha",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - voce e sua filha. A sua filha tem ate 5 anos?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-150 * time.Second),
		},
		{
			Direction:        "INBOUND",
			Body:             "sim",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-120 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Pode enviar os nomes completos e os documentos dos dois.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-90 * time.Second),
		},
		{
			Direction:        "INBOUND",
			Body:             "Joao Vitor Messias 06645648103\nIvoneide Messias 46643591104",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-60 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - os numeros enviados sao os CPFs do Joao Vitor e da Ivoneide?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-30 * time.Second),
		},
	}

	if input, ok := parseBookingCreateInput(session, history, "isso", nil); ok {
		t.Fatalf("expected booking create to be blocked until lap child assignment, got %+v", input)
	}
}

func TestParseBookingCreateInputUsesExplicitLapChildLabel(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	history := lapChildBookingHistory(now, "Criança: Joao Vitor Messias 06645648103\nAdulto: Ivoneide Messias 46643591104")

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create with explicit lap child label")
	}
	if len(input.Passengers) != 2 || !input.Passengers[0].IsLapChild || input.Passengers[1].IsLapChild {
		t.Fatalf("expected first passenger as lap child only, got %+v", input.Passengers)
	}
	payload := buildBookingCreateRequestPayload(input)
	rawPassengers := asInterfaceSliceMaps(payload["passengers"])
	if len(rawPassengers) != 2 || rawPassengers[0]["is_lap_child"] != true || rawPassengers[1]["is_lap_child"] == true {
		t.Fatalf("expected request payload to preserve lap child assignment, got %+v", rawPassengers)
	}
}

func TestBookingPassengerFromDocumentExtractUsesVisibleCPFOnCNH(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passenger := bookingPassengerFromDocumentExtract(DocumentExtractPassenger{
		Name:         "Claudecir Schumacher",
		DocumentType: "CNH",
		Document:     "99999999999",
		CPF:          "066.456.481-03",
		CNH:          "99999999999",
		BirthDate:    "1970-01-02",
		Confidence:   0.92,
	}, session)

	if passenger.DocumentType != "CPF" || passenger.Document != "06645648103" {
		t.Fatalf("expected visible CPF as primary passenger document, got %+v", passenger)
	}
	if passenger.Phone != "5549999999999" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
	if passenger.Notes != "Documentos secundarios extraidos: CNH: 99999999999" {
		t.Fatalf("expected CNH preserved in notes, got %+v", passenger)
	}
}

func TestBookingPassengerFromDocumentExtractRejectsInvalidCPFOnCNH(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passenger := bookingPassengerFromDocumentExtract(DocumentExtractPassenger{
		Name:         "Claudecir Schumacher",
		DocumentType: "CNH",
		Document:     "99999999999",
		CPF:          "123.456.789-01",
		CNH:          "99999999999",
		Confidence:   0.92,
	}, session)

	if passenger.DocumentType != "CNH" || passenger.Document != "99999999999" {
		t.Fatalf("expected invalid CPF to keep CNH as primary document, got %+v", passenger)
	}
	if passenger.Notes != "" {
		t.Fatalf("did not expect CNH duplicated in notes, got %+v", passenger)
	}
}

func TestExtractBookingPassengerDocumentRejectsInvalidCPF(t *testing.T) {
	document, documentType := extractBookingPassengerDocument("CPF 123.456.789-01")

	if document != "" || documentType != "" {
		t.Fatalf("expected invalid CPF to be ignored, got document=%q type=%q", document, documentType)
	}
}

func TestNormalizePassengerDocumentValueCleansRGIssuer(t *testing.T) {
	cases := []struct {
		value    string
		expected string
	}{
		{value: "2817314 SSP SC", expected: "2817314"},
		{value: "2817314SSPSC", expected: "2817314"},
		{value: "12.345.678-9 SSP/SC", expected: "123456789"},
		{value: "SSP/SC", expected: ""},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if document := normalizePassengerDocumentValue(tc.value, "RG"); document != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, document)
			}
		})
	}
}

func TestParseBookingCreateFromLapChildAssignmentReplyByIndex(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	history := lapChildBookingHistory(now, "Joao Vitor Messias 06645648103\nIvoneide Messias 46643591104")
	history = append(history, Message{
		Direction:        "OUTBOUND",
		Body:             "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias",
		ProcessingStatus: messageStatusAutomationDraft,
		ReceivedAt:       now.Add(-30 * time.Second),
	})

	input, ok := parseBookingCreateFromLapChildAssignment(session, history, "1")
	if !ok {
		t.Fatalf("expected booking create from lap child assignment")
	}
	if len(input.Passengers) != 2 || !input.Passengers[0].IsLapChild || input.Passengers[1].IsLapChild {
		t.Fatalf("expected first passenger as lap child only, got %+v", input.Passengers)
	}
}

func TestParseBookingCreateInputUsesAssistantExtractedPassengerConfirmationOnHistory(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei estas opcoes para Ituporanga/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Destination: "Ituporanga/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-1",
								BoardStopID:            "board-1",
								AlightStopID:           "alight-1",
								OriginDisplayName:      "Moncao/MA",
								DestinationDisplayName: "Ituporanga/SC",
								OriginDepartTime:       "09:00",
								TripDate:               "2026-04-26",
							},
						},
					}),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "a primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 06645648103",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
		},
	}

	input, ok := parseBookingCreateInput(session, history, "isso", nil)
	if !ok {
		assistantBody := history[len(history)-1].Body
		t.Fatalf(
			"expected booking create input from assistant extraction history: selected_option=%d availability=%v assistant_passengers=%+v passenger_text=%q passengers=%+v qty=%d",
			findLatestSelectedOptionIndex(history),
			findLatestAvailabilityContext(history) != nil,
			extractBookingCreatePassengers(assistantBody, session),
			findLatestPassengerDetailsText(history, session),
			extractBookingCreatePassengers(findLatestPassengerDetailsText(history, session), session),
			extractPassengerQuantity(findLatestPassengerDetailsText(history, session)),
		)
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	if input.Passengers[0].Name != "Joao Vitor Messias" {
		t.Fatalf("unexpected passenger name: %+v", input.Passengers[0])
	}
	if input.Passengers[0].DocumentType != "CPF" || input.Passengers[0].Document != "06645648103" {
		t.Fatalf("unexpected passenger document: %+v", input.Passengers[0])
	}
}

func TestParseBookingCreateInputRejectsConfirmationWhenPassengerCountStillIncomplete(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei estas opcoes para Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Destination: "Fraiburgo/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-1",
								BoardStopID:            "board-1",
								AlightStopID:           "alight-1",
								OriginDisplayName:      "Moncao/MA",
								DestinationDisplayName: "Fraiburgo/SC",
								OriginDepartTime:       "09:00",
								TripDate:               "2026-05-11",
							},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou ha mais passageiros? Tem crianca de ate 5 anos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "INBOUND", Body: "eu e minha filha", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-90 * time.Second)},
		{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos dois.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-60 * time.Second)},
		{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 06645648103", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-30 * time.Second)},
	}

	if input, ok := parseBookingCreateInput(session, history, "isso", nil); ok {
		t.Fatalf("expected incomplete passenger confirmation to block booking create, got %+v", input)
	}
}

func TestParseBookingCreateInputBlocksShortPaymentReplyAfterBookingCreated(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Reserva criada com sucesso. Codigo ABC12345.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
						Mode:            "created",
						BookingID:       "BK-ABC123456",
						ReservationCode: "ABC12345",
						Status:          "PENDING",
						TotalAmount:     950,
						RemainderAmount: 950,
					}),
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	}

	if input, ok := parseBookingCreateInput(session, history, "pix", nil); ok {
		t.Fatalf("expected payment reply to block booking create, got %+v", input)
	}
}

func TestParseBookingCreateFromDocumentConfirmation(t *testing.T) {
	session := Session{
		ID:            "session-document-confirmation",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", true)

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem")
	if !ok {
		t.Fatalf("expected booking create from document confirmation")
	}
	if input.TripID != "trip-doc-1" || input.BoardStopID != "board-doc-1" || input.AlightStopID != "alight-doc-1" {
		t.Fatalf("unexpected trip identifiers: %+v", input)
	}
	if input.OriginDisplayName != "Santa Inês/MA" || input.DestinationDisplayName != "Fraiburgo/SC" {
		t.Fatalf("unexpected route: %+v", input)
	}
	if input.TripDate != "2026-05-11" || input.DepartureTime != "12:00" {
		t.Fatalf("unexpected schedule: %+v", input)
	}
	if input.Qty != 1 || len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input)
	}
	if input.Passengers[0].Name != "Joao Vitor Messias" {
		t.Fatalf("unexpected passenger name: %+v", input.Passengers[0])
	}
	if input.Passengers[0].DocumentType != "CPF" || input.Passengers[0].Document != "06645648103" {
		t.Fatalf("unexpected passenger document: %+v", input.Passengers[0])
	}
	if input.IdempotencyKey == "" {
		t.Fatalf("expected idempotency key")
	}
}

func TestLooksLikeDocumentConfirmationAcceptsNaturalConfirmations(t *testing.T) {
	cases := []string{
		"sim esta correto",
		"sim está correto",
		"esta correto",
		"está correto",
		"pode prosseguir",
		"pode criar",
		"confirmado",
	}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			if !looksLikeDocumentConfirmation(tc) {
				t.Fatalf("expected %q to be accepted as document confirmation", tc)
			}
		})
	}
}

func TestParseBookingCreateFromManualPassengerConfirmation(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:            "session-manual-confirmation",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Datas para Igarape do Meio/MA -> Petrolandia/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Origin:      "Igarape do Meio/MA",
							Destination: "Petrolandia/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-manual-1",
								BoardStopID:            "board-manual-1",
								AlightStopID:           "alight-manual-1",
								OriginDisplayName:      "Igarape do Meio/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "11:00",
								TripDate:               "2026-05-11",
								Price:                  950,
								Currency:               "BRL",
								PackageName:            packageToSantaCatarina,
							},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "11/05", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem é só para você ou tem mais alguém, informe também se há criança até 5 anos?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: "Joao Vitor Messias 06645648103", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Nome e CPF confirmados. Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
	}

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create from manual passenger confirmation")
	}
	if input.Qty != 1 {
		t.Fatalf("expected qty 1, got %d", input.Qty)
	}
	if input.TripID == "" || input.BoardStopID == "" || input.AlightStopID == "" {
		t.Fatalf("expected trip identifiers, got %+v", input)
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	passenger := input.Passengers[0]
	if passenger.Name != "Joao Vitor Messias" || passenger.DocumentType != "CPF" || passenger.Document != "06645648103" {
		t.Fatalf("unexpected passenger: %+v", passenger)
	}
	if passenger.Phone != "5549988709047" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
}

func TestBookingCreateDoesNotInferLapChildFromGenericSim(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:            "session-no-lap-child",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Datas para Igarape do Meio/MA -> Petrolandia/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{Origin: "Igarape do Meio/MA", Destination: "Petrolandia/SC", Qty: 1, Limit: 5},
						Results: []AvailabilitySearchItem{{
							TripID: "trip-no-child-1", BoardStopID: "board-no-child-1", AlightStopID: "alight-no-child-1",
							OriginDisplayName: "Igarape do Meio/MA", DestinationDisplayName: "Petrolandia/SC", OriginDepartTime: "11:00", TripDate: "2026-05-11",
						}},
					}),
				},
			},
		},
		{Direction: "OUTBOUND", Body: "A passagem é só para você ou tem mais alguém, informe também se há criança até 5 anos?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar seu nome completo e o documento.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: "Joao Vitor Messias 06645648103", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
	}

	context := collectBookingDraftContext(session, history, "sim")
	if context.ChildUnder5Count != 0 {
		t.Fatalf("expected child_under_5_count 0, got %+v", context)
	}
	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create from confirmation")
	}
	for _, passenger := range input.Passengers {
		if passenger.IsLapChild {
			t.Fatalf("did not expect lap child from generic sim, got %+v", input.Passengers)
		}
	}
}

func TestDocumentConfirmationDoesNotCreateBookingWithoutPreviousDocumentExtract(t *testing.T) {
	session := Session{
		ID:            "session-document-missing",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", false)

	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem"); !ok || input.Qty != 1 {
		t.Fatalf("expected missing document_extract to use passenger details fallback, got ok=%v input=%+v", ok, input)
	}

	history = documentConfirmationBookingHistory(time.Now().UTC(), "PARTIAL", true)
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem"); ok {
		t.Fatalf("expected partial document_extract to block booking create, got %+v", input)
	}
}

func TestLastBotAskedPassengerCount(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Perfeito, a passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?"},
		{Direction: "INBOUND", Body: "é só para mim"},
	}
	if !lastBotAskedPassengerCount(history) {
		t.Fatal("expected passenger count question to be detected")
	}
}

func documentConfirmationBookingHistory(now time.Time, documentMode string, includeDocumentExtract bool) []Message {
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Opções para Santa Inês/MA -> Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-5 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Origin:      "Santa Inês/MA",
							Destination: "Fraiburgo/SC",
							Qty:         1,
							Limit:       8,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-doc-1",
								BoardStopID:            "board-doc-1",
								AlightStopID:           "alight-doc-1",
								OriginDisplayName:      "Santa Inês/MA",
								DestinationDisplayName: "Fraiburgo/SC",
								OriginDepartTime:       "12:00",
								TripDate:               "2026-05-11",
								Price:                  950,
								Currency:               "BRL",
								PackageName:            packageToSantaCatarina,
							},
						},
					}),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "a primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-4 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
	}
	if includeDocumentExtract {
		history = append(history, Message{
			Direction:        "OUTBOUND",
			Body:             "Messias, consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 06645648103",
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
						Mode:                   documentMode,
						ExpectedPassengerCount: 1,
						MediaCount:             1,
						Passengers: []DocumentExtractPassenger{
							{
								Name:         "Joao Vitor Messias",
								DocumentType: "CPF",
								Document:     "06645648103",
								Confidence:   0.98,
							},
						},
					}),
				},
			},
		})
	} else {
		history = append(history, Message{
			Direction:        "OUTBOUND",
			Body:             "Messias, consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 06645648103",
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       now.Add(-2 * time.Minute),
		})
	}
	return history
}

func lapChildBookingHistory(now time.Time, passengerDetails string) []Message {
	return []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Opcoes para Santa Ines/MA -> Monte Carlo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Monte Carlo/SC", Qty: 2, Limit: 5},
						Results: []AvailabilitySearchItem{{
							TripID: "trip-lap-1", BoardStopID: "board-lap-1", AlightStopID: "alight-lap-1",
							OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Monte Carlo/SC",
							OriginDepartTime: "12:00", TripDate: "2026-05-25", Price: 950, Currency: "BRL",
						}},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "eu e meu filho de 4 anos", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF ou RG).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: passengerDetails, ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
	}
}

func TestLastBotAskedPassengerCountIgnoresOlderPassengerQuestionAfterDocumentRequest(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Perfeito, a passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?"},
		{Direction: "INBOUND", Body: "é só para mim"},
		{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos dois. Se preferir, pode mandar foto legivel do documento."},
	}
	if lastBotAskedPassengerCount(history) {
		t.Fatal("expected latest outbound document request to clear passenger count context")
	}
}

func TestRouteAndPassengerCollectionPromptDoesNotActLikePassengerOnlyContext(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Qual a cidade de origem, a data da viagem e quantos passageiros vao viajar?"},
	}
	if !lastBotAskedPassengerCount(history) {
		t.Fatal("expected broad prompt to contain passenger count question")
	}
	if !lastBotAskedRouteAndPassengerCollection(history) {
		t.Fatal("expected broad route/date/passenger prompt to be classified separately")
	}
}

func TestParsePassengerCountReply(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		passengers int
		children   int
		ok         bool
	}{
		{name: "solo", text: "é só para mim", passengers: 1, children: 0, ok: true},
		{name: "alone", text: "sou só eu", passengers: 1, children: 0, ok: true},
		{name: "couple", text: "eu e minha esposa", passengers: 2, children: 0, ok: true},
		{name: "two_people", text: "eu e mais uma pessoa", passengers: 2, children: 0, ok: true},
		{name: "child_denial", text: "não tem criança", passengers: 0, children: 0, ok: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			passengers, children, ok := parsePassengerCountReply(tc.text)
			if ok != tc.ok || passengers != tc.passengers || children != tc.children {
				t.Fatalf("unexpected parse result for %q: passengers=%d children=%d ok=%v", tc.text, passengers, children, ok)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsPraMim(t *testing.T) {
	cases := []string{
		"pra mim",
		"para mim",
		"so pra mim",
		"só pra mim",
		"eh so pra mim",
		"so eu",
		"só eu",
		"sou eu",
		"sozinho",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(text)
			if !slots.PassengerCountKnown || slots.PassengerCount != 1 {
				t.Fatalf("expected passenger_count=1 known for %q, got %+v", text, slots)
			}
			if slots.ChildUnder5CountKnown {
				t.Fatalf("expected child slot unknown for %q, got %+v", text, slots)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsMePlusAndExplicitTotals(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		passengers int
	}{
		{name: "me_plus_1_digit", text: "eu e mais 1 pessoa", passengers: 2},
		{name: "me_plus_1_word", text: "eu e mais uma pessoa", passengers: 2},
		{name: "me_plus_2_digit", text: "eu e mais 2 pessoas", passengers: 3},
		{name: "me_plus_2_word", text: "eu e mais duas pessoas", passengers: 3},
		{name: "me_plus_3_digit", text: "eu e mais 3 pessoas", passengers: 4},
		{name: "me_plus_3_word", text: "eu e mais tres pessoas", passengers: 4},
		{name: "pra_mim_plus_2_digit", text: "pra mim e mais 2 pessoas", passengers: 3},
		{name: "para_mim_plus_2_word", text: "para mim e mais duas pessoas", passengers: 3},
		{name: "me_plus_3_passengers", text: "eu e mais 3 passageiros", passengers: 4},
		{name: "three_people", text: "3 pessoas", passengers: 3},
		{name: "three_people_total", text: "3 pessoas no total", passengers: 3},
		{name: "somos_three_people", text: "somos 3 pessoas", passengers: 3},
		{name: "somos_em_three", text: "somos em 3", passengers: 3},
		{name: "vai_three_people", text: "vai 3 pessoas", passengers: 3},
		{name: "four_passengers", text: "4 passageiros", passengers: 4},
		{name: "two_people_word", text: "duas pessoas", passengers: 2},
		{name: "three_people_word", text: "tres pessoas", passengers: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(tc.text)
			if !slots.PassengerCountKnown || slots.PassengerCount != tc.passengers {
				t.Fatalf("expected passenger_count=%d known for %q, got %+v", tc.passengers, tc.text, slots)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsMePlusDoesNotBecomeSolo(t *testing.T) {
	slots := parsePassengerClarificationSlots("eu e mais 3 pessoas")
	if slots.PassengerCount == 1 {
		t.Fatalf("expected me-plus reply not to become solo, got %+v", slots)
	}
	if !slots.PassengerCountKnown || slots.PassengerCount != 4 {
		t.Fatalf("expected passenger_count=4 known, got %+v", slots)
	}
}

func TestParsePassengerClarificationSlotsExplicitTotalWithChild(t *testing.T) {
	slots := parsePassengerClarificationSlots("3 pessoas no total, uma crianca de 4 anos")
	if !slots.PassengerCountKnown || slots.PassengerCount != 3 {
		t.Fatalf("expected passenger_count=3 known, got %+v", slots)
	}
	if !slots.ChildUnder5CountKnown || slots.ChildUnder5Count != 1 {
		t.Fatalf("expected child_under_5_count=1 known, got %+v", slots)
	}
}

func TestParsePassengerClarificationSlotsNaoChild(t *testing.T) {
	cases := []string{
		"nao",
		"não",
		"sem criança",
		"nao tem criança",
		"não tem criança",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(text)
			if !slots.ChildUnder5CountKnown || slots.ChildUnder5Count != 0 {
				t.Fatalf("expected child_under_5_count=0 known for %q, got %+v", text, slots)
			}
			if slots.PassengerCountKnown {
				t.Fatalf("expected passenger slot unknown for %q, got %+v", text, slots)
			}
		})
	}
}
