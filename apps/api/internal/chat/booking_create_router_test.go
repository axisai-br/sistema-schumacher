package chat

import (
	"testing"
	"time"
)

func TestParseBookingCreateInputUsesHistorySelectionAndPassengerDetailsOnConfirmation(t *testing.T) {
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
			Body:             "joao vitor messias 06645648103\nmarina messias 46643591104",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-60 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - os numeros enviados sao os CPFs do Joao Vitor e da Marina?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-30 * time.Second),
		},
	}

	input, ok := parseBookingCreateInput(session, history, "isso", nil)
	if !ok {
		t.Fatalf(
			"expected booking create input from confirmation history: selected_option=%d availability=%v passenger_text=%q passengers=%+v lap_child_count=%d",
			findLatestSelectedOptionIndex(history),
			findLatestAvailabilityContext(history) != nil,
			findLatestPassengerDetailsText(history, session),
			extractBookingCreatePassengers(findLatestPassengerDetailsText(history, session), session),
			inferLapChildCount(history),
		)
	}
	if input.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option 1, got %d", input.SelectedOptionIndex)
	}
	if input.TripID != "trip-1" || input.BoardStopID != "board-1" || input.AlightStopID != "alight-1" {
		t.Fatalf("unexpected selected trip data: %+v", input)
	}
	if input.OriginDisplayName != "Moncao/MA" || input.DestinationDisplayName != "Petrolandia/SC" {
		t.Fatalf("unexpected route: %+v", input)
	}
	if input.Qty != 2 {
		t.Fatalf("expected qty 2, got %d", input.Qty)
	}
	if len(input.Passengers) != 2 {
		t.Fatalf("expected two passengers, got %+v", input.Passengers)
	}
	if input.Passengers[0].Document != "06645648103" || input.Passengers[1].Document != "46643591104" {
		t.Fatalf("unexpected passengers: %+v", input.Passengers)
	}
	if input.Passengers[1].IsLapChild != true {
		t.Fatalf("expected second passenger marked as lap child, got %+v", input.Passengers)
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

func TestDocumentConfirmationDoesNotCreateBookingWithoutPreviousDocumentExtract(t *testing.T) {
	session := Session{
		ID:            "session-document-missing",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", false)

	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem"); ok {
		t.Fatalf("expected missing document_extract to block booking create, got %+v", input)
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
