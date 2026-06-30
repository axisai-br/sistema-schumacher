package chat

import (
	"testing"
	"time"
)

func TestInferActivePromptContextKindsFromLatestReliablePrompt(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		body string
		want ActivePromptKind
	}{
		{
			name: "availability option choice",
			body: "Encontrei estas opcoes:\n1. Moncao/MA para Videira/SC, 2026-07-06, saida 09:00, R$ 950\n\nQual opcao voce prefere?",
			want: ActivePromptAvailabilityOptionChoice,
		},
		{
			name: "availability date choice",
			body: "Qual data prefere?",
			want: ActivePromptAvailabilityDateChoice,
		},
		{
			name: "availability date choice among dates",
			body: "Qual dessas datas você prefere?",
			want: ActivePromptAvailabilityDateChoice,
		},
		{
			name: "passenger count",
			body: "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?",
			want: ActivePromptPassengerCount,
		},
		{
			name: "lap child question",
			body: "Tem crianca de 5 anos ou menos viajando?",
			want: ActivePromptLapChildQuestion,
		},
		{
			name: "lap child assignment",
			body: "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao\n2. Maria",
			want: ActivePromptLapChildAssignment,
		},
		{
			name: "passenger documents",
			body: "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento.",
			want: ActivePromptPassengerDocuments,
		},
		{
			name: "document confirmation",
			body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Joao | CPF | 529.***.***-25",
			want: ActivePromptDocumentConfirmation,
		},
		{
			name: "payment preference",
			body: askPaymentChoiceReply,
			want: ActivePromptPaymentPreference,
		},
		{
			name: "payment options info is not a prompt",
			body: paymentOptionsInfoReply,
			want: ActivePromptUnknown,
		},
		{
			name: "paying passenger info is not a prompt",
			body: payingPassengerInfoReply,
			want: ActivePromptUnknown,
		},
		{
			name: "payer cpf",
			body: "Para gerar o PIX, preciso do CPF do pagador.",
			want: ActivePromptPayerCPF,
		},
		{
			name: "reservation route",
			body: "Para fazer a reserva, primeiro preciso saber o trecho da viagem. Me diga de qual cidade você vai sair e para qual cidade de Santa Catarina quer ir.",
			want: ActivePromptReservationRoute,
		},
		{
			name: "unknown",
			body: "Obrigado, ja te retorno.",
			want: ActivePromptUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []Message{
				{ID: "old", Direction: "OUTBOUND", Body: "De qual cidade voce vai sair?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
				{ID: "current", Direction: "OUTBOUND", Body: tc.body, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-1 * time.Minute)},
			}

			got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseRouteSelection})
			if got.Kind != tc.want {
				t.Fatalf("expected kind %s, got %+v", tc.want, got)
			}
			if got.SourceMessageID != "current" {
				t.Fatalf("expected current source message, got %+v", got)
			}
		})
	}
}

func TestInferActivePromptContextUsesLatestPromptPriority(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	history := []Message{
		{ID: "route", Direction: "OUTBOUND", Body: "De qual cidade do Maranhao voce vai sair?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{ID: "availability", Direction: "OUTBOUND", Body: "Encontrei estas opcoes:\n1. Moncao/MA para Videira/SC\n\nQual opcao voce prefere?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute)},
		{ID: "document", Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-1 * time.Minute)},
	}

	state := CanonicalConversationState{
		Phase: ConversationPhaseBookingPending,
		LastToolFacts: map[string]interface{}{
			toolNameAvailabilitySearch: map[string]interface{}{
				"results": []interface{}{map[string]interface{}{"trip_id": "trip-1"}},
			},
		},
	}
	got := InferActivePromptContext(history, state)

	if got.Kind != ActivePromptDocumentConfirmation {
		t.Fatalf("expected document confirmation to win over stale availability, got %+v", got)
	}
	if got.SourceMessageID != "document" {
		t.Fatalf("expected latest document prompt as source, got %+v", got)
	}
	if got.HasAvailabilityList || got.AvailabilityOptionCount != 0 {
		t.Fatalf("document confirmation must not expose stale availability context, got %+v", got)
	}
}

func TestInferActivePromptContextSkipsUnsentDrafts(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	history := []Message{
		{ID: "sent", Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		{ID: "draft", Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
		{ID: "blocked", Direction: "OUTBOUND", Body: "Para gerar o PIX, preciso do CPF do pagador.", ProcessingStatus: "REVIEW_REQUIRED", ReceivedAt: now},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})

	if got.Kind != ActivePromptPassengerCount {
		t.Fatalf("expected sent passenger prompt after skipping unsent drafts, got %+v", got)
	}
	if got.SourceMessageID != "sent" {
		t.Fatalf("expected sent message source, got %+v", got)
	}
}

func TestInferActivePromptContextReturnsUnknownWithoutReliablePrompt(t *testing.T) {
	history := []Message{
		{ID: "customer", Direction: "INBOUND", Body: "oi", ProcessingStatus: "PROCESSED"},
		{ID: "draft", Direction: "OUTBOUND", Body: "Qual opcao voce prefere?", ProcessingStatus: messageStatusAutomationDraft},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseDiscovery})

	if got.Kind != ActivePromptUnknown {
		t.Fatalf("expected unknown without reliable prompt, got %+v", got)
	}
	if got.SourceMessageID != "" {
		t.Fatalf("did not expect source message, got %+v", got)
	}
}

func TestInferActivePromptContextReadsAvailabilityOptionCount(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	history := []Message{
		{
			ID:               "availability",
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes:\n1. Moncao/MA para Videira/SC\n2. Moncao/MA para Fraiburgo/SC\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now,
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"trip_id": "trip-1"},
							map[string]interface{}{"trip_id": "trip-2"},
						},
					},
				},
			},
		},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})

	if got.Kind != ActivePromptAvailabilityOptionChoice {
		t.Fatalf("expected availability option choice, got %+v", got)
	}
	if got.AvailabilityOptionCount != 2 {
		t.Fatalf("expected availability option count 2, got %+v", got)
	}
	if !got.HasAvailabilityList {
		t.Fatalf("expected availability list flag, got %+v", got)
	}
}

func TestInferActivePromptContextLeavesGenericPhrasesUnknown(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	for _, body := range []string{
		"isso mesmo",
		"pode ser",
		"sim",
		"não",
		"ja enviei",
	} {
		t.Run(body, func(t *testing.T) {
			history := []Message{
				{
					ID:               "generic",
					Direction:        "OUTBOUND",
					Body:             body,
					ProcessingStatus: messageStatusAutomationSent,
					ReceivedAt:       now,
				},
			}

			got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseDiscovery})

			if got.Kind != ActivePromptUnknown {
				t.Fatalf("expected generic phrase %q to remain unknown, got %+v", body, got)
			}
			if got.HasAvailabilityList || got.AvailabilityOptionCount != 0 {
				t.Fatalf("did not expect availability context for generic phrase %q, got %+v", body, got)
			}
		})
	}
}
