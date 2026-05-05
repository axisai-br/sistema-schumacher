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
