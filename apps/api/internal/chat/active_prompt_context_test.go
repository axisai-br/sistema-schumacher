package chat

import (
	"encoding/json"
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

func TestAvailabilityInvalidationHistoryBoundaryOverlayPreservesIndependentContext(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	availability := availabilityOptionPromptFutureResultAt(now)
	oldAvailability := Message{
		ID: "old-availability", Direction: "OUTBOUND", Body: buildAvailabilityListReply(availability),
		ProcessingStatus: messageStatusAutomationSent, CreatedAt: now.Add(-4 * time.Minute),
		Payload: map[string]interface{}{
			"selected_option_index":              1,
			selectedAvailabilityResultPayloadKey: selectedAvailabilityResultPayloadFromAvailability(&availability, 1),
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				toolNameBookingCreate:      map[string]interface{}{"booking_id": "booking-independent"},
				toolNameDocumentExtract:    map[string]interface{}{"status": "document-independent"},
				toolNamePaymentStatus:      map[string]interface{}{"payment_status": "PENDING"},
			},
		},
	}
	passengerPrompt := Message{
		ID: "passenger-prompt", Direction: "OUTBOUND", Body: askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent, CreatedAt: now.Add(-3 * time.Minute),
		Payload: map[string]interface{}{"handoff_status": "BOT"},
	}
	boundaryMessage := Message{
		ID: "boundary", Direction: "INBOUND", Body: "essa msm",
		ProcessingStatus: "AUTOMATION_PROCESSED", CreatedAt: now.Add(-2 * time.Minute),
	}
	history := []Message{oldAvailability, passengerPrompt, boundaryMessage}
	original, err := json.Marshal(history)
	if err != nil {
		t.Fatalf("marshal original history: %v", err)
	}
	boundary := canonicalAvailabilityFactsInvalidationBoundary{
		AfterMessageID: boundaryMessage.ID,
		AfterCreatedAt: boundaryMessage.CreatedAt,
	}

	overlay := availabilityInferenceHistory(history, boundary)
	after, err := json.Marshal(history)
	if err != nil {
		t.Fatalf("marshal history after overlay: %v", err)
	}
	if string(after) != string(original) {
		t.Fatalf("read-only overlay mutated persisted history:\n got=%s\nwant=%s", after, original)
	}
	if latest := findLatestAvailabilityContext(overlay); latest != nil {
		t.Fatalf("overlay exposed pre-boundary availability: %+v", latest)
	}
	if index := findLatestSelectedOptionIndex(overlay); index != 0 {
		t.Fatalf("overlay exposed pre-boundary selected_option_index=%d", index)
	}
	if snapshot := selectedAvailabilityResultFromMessage(overlay[0]); len(snapshot) != 0 {
		t.Fatalf("overlay exposed pre-boundary selected snapshot: %+v", snapshot)
	}
	toolContext := asMap(overlay[0].Payload["tool_context"])
	if availabilityFacts := asMap(toolContext[toolNameAvailabilitySearch]); len(availabilityFacts) != 0 {
		t.Fatalf("overlay retained availability_search: %+v", availabilityFacts)
	}
	if booking := asMap(toolContext[toolNameBookingCreate]); asString(booking["booking_id"]) != "booking-independent" {
		t.Fatalf("overlay removed independent booking facts: %+v", toolContext)
	}
	if document := asMap(toolContext[toolNameDocumentExtract]); asString(document["status"]) != "document-independent" {
		t.Fatalf("overlay removed independent document facts: %+v", toolContext)
	}
	if payment := asMap(toolContext[toolNamePaymentStatus]); asString(payment["payment_status"]) != "PENDING" {
		t.Fatalf("overlay removed independent payment facts: %+v", toolContext)
	}
	activePrompt := InferActivePromptContext(overlay, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})
	if activePrompt.Kind != ActivePromptPassengerCount || activePrompt.SourceMessageID != passengerPrompt.ID {
		t.Fatalf("overlay erased independent passenger prompt: %+v", activePrompt)
	}
	if got := asString(overlay[1].Payload["handoff_status"]); got != "BOT" {
		t.Fatalf("overlay removed independent handoff metadata: %+v", overlay[1].Payload)
	}
}

func TestAvailabilityInvalidationHistoryBoundaryOutsideWindowUsesTimestampFallback(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	availability := availabilityOptionPromptFutureResultAt(now)
	history := []Message{
		{
			ID: "old-availability", Direction: "OUTBOUND", Body: buildAvailabilityListReply(availability),
			ProcessingStatus: messageStatusAutomationSent, CreatedAt: now.Add(-10 * time.Minute),
			Payload: map[string]interface{}{"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			}},
		},
		{
			ID: "passenger-prompt", Direction: "OUTBOUND", Body: askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent, CreatedAt: now.Add(-9 * time.Minute),
		},
	}
	boundary := canonicalAvailabilityFactsInvalidationBoundary{
		AfterMessageID: "boundary-outside-limit-50",
		AfterCreatedAt: now.Add(-5 * time.Minute),
	}

	overlay := availabilityInferenceHistory(history, boundary)
	if current := currentAvailabilitySelectionPromptAvailabilityContextAt(overlay, now); current != nil {
		t.Fatalf("timestamp fallback reauthorized pre-boundary availability: %+v", current)
	}
	activePrompt := InferActivePromptContext(overlay, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})
	if activePrompt.Kind != ActivePromptPassengerCount || activePrompt.SourceMessageID != "passenger-prompt" {
		t.Fatalf("timestamp fallback removed independent passenger prompt: %+v", activePrompt)
	}
}

func TestAvailabilityInvalidationHistoryBoundaryPreservesStrongHumanAndCancellation(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	availability := availabilityOptionPromptFutureResultAt(now)
	history := []Message{
		{
			ID: "old-availability", Direction: "OUTBOUND", Body: buildAvailabilityListReply(availability),
			ProcessingStatus: messageStatusAutomationSent, CreatedAt: now.Add(-3 * time.Minute),
			Payload: map[string]interface{}{"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			}},
		},
		{ID: "boundary", Direction: "INBOUND", Body: "invalidar", CreatedAt: now.Add(-2 * time.Minute)},
	}
	overlay := availabilityInferenceHistory(history, canonicalAvailabilityFactsInvalidationBoundary{
		AfterMessageID: "boundary",
		AfterCreatedAt: now.Add(-2 * time.Minute),
	})
	state := CanonicalConversationState{Booking: CanonicalBookingState{BookingID: "booking-1"}}
	for _, testCase := range []struct {
		name string
		body string
		want Intent
	}{
		{name: "human", body: "quero falar com um atendente", want: IntentHumanSupport},
		{name: "cancellation", body: "quero cancelar minha reserva", want: IntentBookingCancel},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			decision := routeDeterministicIntent(overlay, testCase.body, state, now)
			if decision.Intent != testCase.want {
				t.Fatalf("strong decision lost after availability overlay: got=%+v want=%s", decision, testCase.want)
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
		{ID: "reviewed", Direction: "OUTBOUND", Body: "Qual opcao voce prefere?", ProcessingStatus: messageStatusAutomationReviewed, ReceivedAt: now.Add(-45 * time.Second)},
		{ID: "pending", Direction: "OUTBOUND", Body: "Para gerar o PIX, preciso do CPF do pagador.", ProcessingStatus: messageStatusAutomationPending, ReceivedAt: now.Add(-30 * time.Second)},
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

func TestInferActivePromptContextSkipsInvisibleAutomationModes(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	for _, mode := range []string{messageStatusAutomationDraft, messageStatusAutomationReviewed, messageStatusAutomationPending} {
		t.Run(mode, func(t *testing.T) {
			history := []Message{
				{ID: "sent", Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
				{
					ID:               "invisible",
					Direction:        "OUTBOUND",
					Body:             "Consegui identificar estes dados. Eles conferem?",
					ProcessingStatus: "",
					Payload:          map[string]interface{}{"mode": mode},
					ReceivedAt:       now.Add(-1 * time.Minute),
				},
			}

			got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})

			if got.Kind != ActivePromptPassengerCount {
				t.Fatalf("expected sent passenger prompt after skipping mode %s, got %+v", mode, got)
			}
			if got.SourceMessageID != "sent" {
				t.Fatalf("expected sent message source after skipping mode %s, got %+v", mode, got)
			}
		})
	}
}

func TestInferActivePromptContextAcceptsSentAutomationWithDraftMode(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	history := []Message{
		{
			ID:                "sent",
			Direction:         "OUTBOUND",
			Body:              "A passagem e so para voce ou vai mais alguem junto?",
			ProcessingStatus:  messageStatusAutomationSent,
			NormalizedPayload: map[string]interface{}{"mode": messageStatusAutomationDraft},
			ReceivedAt:        now,
		},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})

	if got.Kind != ActivePromptPassengerCount {
		t.Fatalf("expected sent automation prompt to remain reliable despite draft mode, got %+v", got)
	}
	if got.SourceMessageID != "sent" {
		t.Fatalf("expected sent message source, got %+v", got)
	}
}

func TestInferActivePromptContextResolvesBotAutoReplyMirrorToDraftSource(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	result := availabilityDateSelectionTestResult()
	body := buildAvailabilityListReply(result)
	history := []Message{
		{
			ID:               "draft-list",
			Direction:        "OUTBOUND",
			Body:             body,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"mode": messageStatusAutomationDraft,
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
				},
			},
		},
		{
			ID:               "mirror-list",
			Direction:        "OUTBOUND",
			Body:             body,
			ProcessingStatus: "PENDING",
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"mode":             "BOT_AUTO_REPLY",
				"draft_message_id": "draft-list",
			},
		},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
	promptContext := currentAvailabilitySelectionPromptContext(history)

	if got.Kind != ActivePromptAvailabilityOptionChoice {
		t.Fatalf("expected availability option prompt, got %+v", got)
	}
	if got.SourceMessageID != "draft-list" {
		t.Fatalf("expected draft source message, got %+v", got)
	}
	if got.AvailabilityOptionCount != 1 || !got.HasAvailabilityList {
		t.Fatalf("expected source draft availability count, got %+v", got)
	}
	if promptContext.OptionCount != 1 || !promptContext.HasCurrentFacts {
		t.Fatalf("expected source draft current availability facts, got %+v", promptContext)
	}
}

func TestInferActivePromptContextBotAutoReplyWithoutDraftSourceDoesNotExposeCurrentFacts(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	result := availabilityDateSelectionTestResult()
	body := buildAvailabilityListReply(result)
	history := []Message{
		{
			ID:               "mirror-list",
			Direction:        "OUTBOUND",
			Body:             body,
			ProcessingStatus: "PENDING",
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"mode":             "BOT_AUTO_REPLY",
				"draft_message_id": "missing-draft",
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
				},
			},
		},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
	promptContext := currentAvailabilitySelectionPromptContext(history)

	if got.Kind != ActivePromptAvailabilityOptionChoice {
		t.Fatalf("expected rendered availability prompt to be recognized, got %+v", got)
	}
	if got.SourceMessageID != "mirror-list" {
		t.Fatalf("expected unresolved mirror as prompt body source, got %+v", got)
	}
	if promptContext.OptionCount != 1 {
		t.Fatalf("expected rendered option count from mirror body, got %+v", promptContext)
	}
	if promptContext.HasCurrentFacts {
		t.Fatalf("BOT_AUTO_REPLY without draft source must not expose current facts, got %+v", promptContext)
	}
}

func TestInferActivePromptContextResolvesBotAutoReplyNonAvailabilityPrompts(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		body string
		want ActivePromptKind
	}{
		{
			name: "passenger count",
			body: "A passagem e so para voce ou tem mais alguem junto?",
			want: ActivePromptPassengerCount,
		},
		{
			name: "lap child question",
			body: "Tem crianca de 5 anos ou menos viajando?",
			want: ActivePromptLapChildQuestion,
		},
		{
			name: "payment options info remains informational",
			body: paymentOptionsInfoReply,
			want: ActivePromptUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []Message{
				{
					ID:               "draft-prompt",
					Direction:        "OUTBOUND",
					Body:             tc.body,
					ProcessingStatus: messageStatusAutomationSent,
					ReceivedAt:       now.Add(-2 * time.Minute),
					Payload:          map[string]interface{}{"mode": messageStatusAutomationDraft},
				},
				{
					ID:               "mirror-prompt",
					Direction:        "OUTBOUND",
					Body:             tc.body,
					ProcessingStatus: "PENDING",
					ReceivedAt:       now.Add(-1 * time.Minute),
					Payload: map[string]interface{}{
						"mode":             "BOT_AUTO_REPLY",
						"draft_message_id": "draft-prompt",
					},
				},
			}

			got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})

			if got.Kind != tc.want {
				t.Fatalf("expected kind %s, got %+v", tc.want, got)
			}
			if got.SourceMessageID != "draft-prompt" {
				t.Fatalf("expected draft source message, got %+v", got)
			}
		})
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

func TestInferActivePromptContextReadsAvailabilityOptionCountFromRenderedPrompt(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	history := []Message{
		{
			ID:               "availability",
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes:\n1. Videira/SC para Santa Ines/MA, 2026-07-13, saida 13:00, R$ 950\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now,
		},
	}
	state := CanonicalConversationState{
		Phase: ConversationPhaseTripSelection,
		LastToolFacts: map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionFiveOptionsTestResult()),
		},
	}

	got := InferActivePromptContext(history, state)

	if got.Kind != ActivePromptAvailabilityOptionChoice {
		t.Fatalf("expected availability option choice, got %+v", got)
	}
	if got.AvailabilityOptionCount != 1 {
		t.Fatalf("expected rendered availability option count 1, got %+v", got)
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
