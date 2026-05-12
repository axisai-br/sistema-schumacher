package chat

import (
	"context"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func TestReprocessAvailabilitySearchUsesConfirmedRouteFromHistory(t *testing.T) {
	cases := []struct {
		name        string
		currentTurn string
	}{
		{name: "quais_datas_disponiveis", currentTurn: "quais datas disponiveis"},
		{name: "tem_datas", currentTurn: "tem datas?"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{
				enabled: true,
				result: RunAgentResult{
					ReplyText:          "ok",
					Model:              "gpt-test",
					ProviderResponseID: "resp-confirmed-route",
				},
			}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

			session, _ := store.seedSessionWithMessage("5511999999999", "oi")
			now := time.Now().UTC()
			if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
				SessionID:        session.ID,
				Direction:        "OUTBOUND",
				Kind:             "TEXT",
				ProcessingStatus: messageStatusAutomationSent,
				ReceivedAt:       now.Add(-2 * time.Minute),
				Body:             "Confirmando: saída de Santa Inês (MA) para Fraiburgo (SC). Qual data você pretende viajar?",
			}); err != nil {
				t.Fatalf("create outbound confirmation: %v", err)
			}

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-current-" + tc.name,
					IdempotencyKey:    "idem-current-" + tc.name,
					Body:              tc.currentTurn,
				},
			})
			if err != nil {
				t.Fatalf("ingest current turn: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess: %v", err)
			}
			if len(out.ToolCalls) != 1 {
				t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
			}
			if out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
				t.Fatalf("expected tool name %s, got %s", toolNameAvailabilitySearch, out.ToolCalls[0].ToolName)
			}
			if searcher.calls != 1 {
				t.Fatalf("expected one availability search, got %d", searcher.calls)
			}
			if searcher.lastInput.Origin != "Santa Inês/MA" || searcher.lastInput.Destination != "Fraiburgo/SC" {
				t.Fatalf("unexpected search route: %+v", searcher.lastInput)
			}
			if got := asString(out.ToolCalls[0].RequestPayload["origin"]); got != "Santa Inês/MA" {
				t.Fatalf("expected request payload origin Santa Inês/MA, got %q", got)
			}
			if got := asString(out.ToolCalls[0].RequestPayload["destination"]); got != "Fraiburgo/SC" {
				t.Fatalf("expected request payload destination Fraiburgo/SC, got %q", got)
			}
		})
	}
}

func TestParseDirectAvailabilitySearchInputRejectsGenericPassageSentence(t *testing.T) {
	input, ok := parseDirectAvailabilitySearchInput("gostaria de passagem para sc", time.Now().UTC())
	if ok {
		t.Fatalf("expected generic sentence to stay out of direct route parsing, got %+v", input)
	}

	if origin, destination, ok := extractExplicitRouteFromText("gostaria de passagem para sc"); ok || origin != "" || destination != "" {
		t.Fatalf("expected no explicit route from generic sentence, got origin=%q destination=%q ok=%v", origin, destination, ok)
	}
}

func TestNormalizeLocationDisplayNameSupportsParentheses(t *testing.T) {
	if got := normalizeLocationDisplayName("Santa Inês (MA)"); got != "Santa Inês/MA" {
		t.Fatalf("expected parenthesized uf to normalize, got %q", got)
	}
}

func TestLastConfirmedRouteFromHistoryParsesBotConfirmation(t *testing.T) {
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Confirmando: saída de Santa Inês (MA) para Fraiburgo (SC). Qual data você pretende viajar?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-2 * time.Minute),
		},
	}

	origin, destination, ok := lastConfirmedRouteFromHistory(history)
	if !ok {
		t.Fatalf("expected confirmed route from history")
	}
	if origin != "Santa Inês/MA" {
		t.Fatalf("expected origin Santa Inês/MA, got %q", origin)
	}
	if destination != "Fraiburgo/SC" {
		t.Fatalf("expected destination Fraiburgo/SC, got %q", destination)
	}
}

func TestInferConversationTurnRouteContextCompletesOriginFromHistoryDestination(t *testing.T) {
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Confirmando: saída de Santa Inês (MA) para Fraiburgo (SC). Qual data você pretende viajar?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       time.Now().UTC().Add(-2 * time.Minute),
		},
	}
	base := inferLatestRouteContextFromHistory(history)
	context := mergeInferredRouteContext(inferConversationTurnRouteContext("to saindo de santa inês", base), base)

	if context.Origin != "Santa Inês/MA" && context.Origin != "Santa Ines/MA" {
		t.Fatalf("expected origin Santa Inês/MA or Santa Ines/MA, got %+v", context)
	}
	if context.Destination != "Fraiburgo/SC" {
		t.Fatalf("expected destination Fraiburgo/SC, got %+v", context)
	}
}

func TestNormalizeIncomingCustomerTextRepairsFraiburgoTranscription(t *testing.T) {
	if got := NormalizeIncomingCustomerText("e passagem para Freiburg."); got != "passagem para Fraiburgo." {
		t.Fatalf("expected normalized transcription, got %q", got)
	}
}

func TestInferRouteContextFromTextNormalizesSupportedDestination(t *testing.T) {
	context := inferRouteContextFromText("quero passagem para Freiburg")
	if context.Destination != "Fraiburgo/SC" {
		t.Fatalf("expected destination Fraiburgo/SC, got %+v", context)
	}
	if context.RouteDirection != "TO_SC" {
		t.Fatalf("expected route direction TO_SC, got %+v", context)
	}
}

func TestParseAvailabilitySearchInputKeepsSCDestinationWhenUserAnswersMAOrigin(t *testing.T) {
	history := []Message{
		{Direction: "INBOUND", Body: "Passagem para Santa Catarina"},
		{Direction: "OUTBOUND", Body: "Oi Messias, temos sim. Valores por cidade em Santa Catarina:\nVideira R$ 950"},
		{Direction: "INBOUND", Body: "Videira"},
		{Direction: "OUTBOUND", Body: "Perfeito, Messias. De qual cidade do Maranhão você vai sair?"},
	}

	input, ok := parseAvailabilitySearchInput(history, "Monção", time.Now().UTC())
	if !ok {
		t.Fatalf("expected availability search input")
	}
	if input.Origin != "Moncao/MA" && input.Origin != "Monção/MA" {
		t.Fatalf("expected origin Moncao/MA or Monção/MA, got %+v", input)
	}
	if input.Destination != "Videira/SC" {
		t.Fatalf("expected destination Videira/SC, got %+v", input)
	}
	if input.PackageName != packageToSantaCatarina {
		t.Fatalf("expected package %q, got %+v", packageToSantaCatarina, input)
	}
	if input.Qty != 1 {
		t.Fatalf("expected qty 1, got %+v", input)
	}
	if input.Limit != 8 {
		t.Fatalf("expected limit 8, got %+v", input)
	}
}

func TestParseAvailabilitySearchInputKeepsMADestinationWhenUserAnswersSCOrigin(t *testing.T) {
	history := []Message{
		{Direction: "INBOUND", Body: "Passagem para Maranhão"},
		{Direction: "OUTBOUND", Body: "Oi Messias, temos sim. Valores por cidade no Maranhão:\nSanta Inês R$ 950"},
		{Direction: "INBOUND", Body: "Santa Inês"},
		{Direction: "OUTBOUND", Body: "Perfeito. Saindo de qual cidade de Santa Catarina?"},
	}

	input, ok := parseAvailabilitySearchInput(history, "Chapecó", time.Now().UTC())
	if !ok {
		t.Fatalf("expected availability search input")
	}
	if input.Origin != "Chapeco/SC" && input.Origin != "Chapecó/SC" {
		t.Fatalf("expected origin Chapeco/SC or Chapecó/SC, got %+v", input)
	}
	if input.Destination != "Santa Ines/MA" && input.Destination != "Santa Inês/MA" {
		t.Fatalf("expected destination Santa Ines/MA or Santa Inês/MA, got %+v", input)
	}
	if input.PackageName != packageToMaranhao {
		t.Fatalf("expected package %q, got %+v", packageToMaranhao, input)
	}
	if input.Qty != 1 {
		t.Fatalf("expected qty 1, got %+v", input)
	}
	if input.Limit != 8 {
		t.Fatalf("expected limit 8, got %+v", input)
	}
}

func TestParseAvailabilitySearchInputKeepsRouteWhenUserSelectsListedDate(t *testing.T) {
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Inês/MA",
			Destination: "Fraiburgo/SC",
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       8,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "ma-sc-2026-05-11",
				BoardStopID:            "board-11",
				AlightStopID:           "alight-11",
				OriginDisplayName:      "Santa Inês/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "12:00",
				TripDate:               "2026-05-11",
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ATIVO",
				TripStatus:             "ATIVO",
				PackageName:            packageToSantaCatarina,
			},
			{
				TripID:                 "ma-sc-2026-05-25",
				BoardStopID:            "board-25",
				AlightStopID:           "alight-25",
				OriginDisplayName:      "Santa Inês/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "12:00",
				TripDate:               "2026-05-25",
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ATIVO",
				TripStatus:             "ATIVO",
				PackageName:            packageToSantaCatarina,
			},
		},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			ProcessingStatus: messageStatusAutomationDraft,
			Body:             "Opções (Santa Inês → Fraiburgo — saída 12:00 — R$950):\n11/05/2026\n25/05/2026",
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
	}

	cases := []string{
		"Eu quero a primeira data do dia 11/05/2026.",
		"Quero a primeira data",
	}
	for _, currentTurn := range cases {
		t.Run(currentTurn, func(t *testing.T) {
			input, ok := parseAvailabilitySearchInput(history, currentTurn, time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC))
			if !ok {
				t.Fatalf("expected availability search input")
			}
			if input.Origin != "Santa Inês/MA" {
				t.Fatalf("expected origin Santa Inês/MA, got %+v", input)
			}
			if input.Destination != "Fraiburgo/SC" {
				t.Fatalf("expected destination Fraiburgo/SC, got %+v", input)
			}
			if input.PackageName != packageToSantaCatarina {
				t.Fatalf("expected package %q, got %+v", packageToSantaCatarina, input)
			}
			if input.TripDate == nil || input.TripDate.UTC().Format("2006-01-02") != "2026-05-11" {
				t.Fatalf("expected trip date 2026-05-11, got %+v", input)
			}
			if input.Qty != 1 {
				t.Fatalf("expected qty 1, got %+v", input)
			}
			if input.Limit != 8 {
				t.Fatalf("expected limit 8, got %+v", input)
			}
		})
	}
}

func TestParseAvailabilitySearchInputSupportedCityPairWithDate(t *testing.T) {
	input, ok := parseAvailabilitySearchInput(nil, "Fraiburgo para monção 18/05", time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatalf("expected availability search input")
	}
	if input.Origin != "Fraiburgo/SC" {
		t.Fatalf("expected origin Fraiburgo/SC, got %+v", input)
	}
	if input.Destination != "Moncao/MA" {
		t.Fatalf("expected destination Moncao/MA, got %+v", input)
	}
	if input.PackageName != packageToMaranhao {
		t.Fatalf("expected package %q, got %+v", packageToMaranhao, input)
	}
	if input.TripDate == nil || input.TripDate.UTC().Format("2006-01-02") != "2026-05-18" {
		t.Fatalf("expected trip date 2026-05-18, got %+v", input)
	}
	if input.Qty != 1 {
		t.Fatalf("expected qty 1, got %+v", input)
	}
	if input.Limit != 8 {
		t.Fatalf("expected limit 8, got %+v", input)
	}
}

func TestParseAvailabilitySearchInputSupportedCityPairReverseDirection(t *testing.T) {
	input, ok := parseAvailabilitySearchInput(nil, "Monção para Fraiburgo 18/05", time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatalf("expected availability search input")
	}
	if input.Origin != "Moncao/MA" {
		t.Fatalf("expected origin Moncao/MA, got %+v", input)
	}
	if input.Destination != "Fraiburgo/SC" {
		t.Fatalf("expected destination Fraiburgo/SC, got %+v", input)
	}
	if input.PackageName != packageToSantaCatarina {
		t.Fatalf("expected package %q, got %+v", packageToSantaCatarina, input)
	}
	if input.TripDate == nil || input.TripDate.UTC().Format("2006-01-02") != "2026-05-18" {
		t.Fatalf("expected trip date 2026-05-18, got %+v", input)
	}
	if input.Qty != 1 {
		t.Fatalf("expected qty 1, got %+v", input)
	}
	if input.Limit != 8 {
		t.Fatalf("expected limit 8, got %+v", input)
	}
}

func TestReprocessUsesAvailabilityToolForSupportedCityPairWithoutUF(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei uma opcao as 15:00.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-supported-city-pair",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					OriginDisplayName:      "Fraiburgo/SC",
					DestinationDisplayName: "Moncao/MA",
					OriginDepartTime:       "15:00",
					TripDate:               "2026-05-18",
					Price:                  950,
					Currency:               "BRL",
					PackageName:            packageToMaranhao,
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-supported-city-pair",
			IdempotencyKey:    "idem-supported-city-pair",
			Body:              "Fraiburgo para monção 18/05",
		},
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected one availability_search tool call, got %+v", out.ToolCalls)
	}
	if searcher.lastInput.Origin != "Fraiburgo/SC" || searcher.lastInput.Destination != "Moncao/MA" {
		t.Fatalf("unexpected route: %+v", searcher.lastInput)
	}
	if searcher.lastInput.PackageName != packageToMaranhao {
		t.Fatalf("expected package %q, got %+v", packageToMaranhao, searcher.lastInput)
	}
	if searcher.lastInput.TripDate == nil || searcher.lastInput.TripDate.UTC().Format("2006-01-02") != "2026-05-18" {
		t.Fatalf("expected trip date 2026-05-18, got %+v", searcher.lastInput.TripDate)
	}
}

func TestResolveContextualActionToolsCreatesBookingAfterDocumentConfirmation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true}
	creator := &fakeBookingCreator{
		enabled: true,
		result: BookingCreateResult{
			Mode:            "created",
			BookingID:       "BK-DOC123",
			ReservationCode: "DOC12345",
			Status:          "PENDING",
			TotalAmount:     950,
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := Session{
		ID:            "session-resolve-document-confirmation",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", true)

	context, used, err := svc.resolveContextualActionTools(context.Background(), session, history, "conferem", agentToolContext{})
	if err != nil {
		t.Fatalf("resolve contextual tools: %v", err)
	}
	if !used {
		t.Fatalf("expected contextual action tool to be used")
	}
	if creator.calls != 1 {
		t.Fatalf("expected one booking create call, got %d", creator.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d", runner.calls)
	}
	if len(context.Calls) != 1 || context.Calls[0].ToolName != toolNameBookingCreate {
		t.Fatalf("expected one booking_create tool call, got %+v", context.Calls)
	}
	if context.BookingCreate == nil {
		t.Fatalf("expected booking create result in context")
	}
	if creator.lastInput.OriginDisplayName != "Santa Inês/MA" || creator.lastInput.DestinationDisplayName != "Fraiburgo/SC" {
		t.Fatalf("unexpected route: %+v", creator.lastInput)
	}
	if creator.lastInput.TripID != "trip-doc-1" || creator.lastInput.TripDate != "2026-05-11" {
		t.Fatalf("unexpected selected trip: %+v", creator.lastInput)
	}
}
