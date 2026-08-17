package chat

import (
	"encoding/json"
	"strings"
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
			payload := map[string]interface{}{}
			normalizedPayload := map[string]interface{}{
				"delivery_recorded_at": now.Add(-time.Minute).Format(time.RFC3339Nano),
			}
			if tc.want == ActivePromptAvailabilityDateChoice {
				payload[outOfTurnActivePromptTemplateDataKey] = string(ActivePromptAvailabilityDateChoice)
				normalizedPayload[outOfTurnActivePromptTemplateDataKey] = string(ActivePromptAvailabilityDateChoice)
			}
			history := []Message{
				{ID: "old", Direction: "OUTBOUND", Body: "De qual cidade voce vai sair?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
				{
					ID:                "current",
					Direction:         "OUTBOUND",
					Body:              tc.body,
					ProcessingStatus:  messageStatusAutomationSent,
					Payload:           payload,
					NormalizedPayload: normalizedPayload,
					ReceivedAt:        now.Add(-1 * time.Minute),
				},
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
	toolContext := map[string]interface{}{
		toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
	}
	history := []Message{
		{
			ID:               "draft-list",
			Direction:        "OUTBOUND",
			Body:             body,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"mode":         messageStatusAutomationDraft,
				"tool_context": toolContext,
			},
			NormalizedPayload: map[string]interface{}{
				"tool_context": cloneMap(toolContext),
			},
		},
		{
			ID:               "mirror-list",
			Direction:        "OUTBOUND",
			Body:             body,
			ProcessingStatus: "SENT",
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"mode":             "BOT_AUTO_REPLY",
				"draft_message_id": "draft-list",
			},
			NormalizedPayload: map[string]interface{}{
				"mode":                 "BOT_AUTO_REPLY",
				"draft_message_id":     "draft-list",
				"delivery_recorded_at": now.Add(-time.Minute).Format(time.RFC3339Nano),
			},
		},
	}

	got := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
	promptContext := currentAvailabilitySelectionPromptContext(history)

	if got.Kind != ActivePromptAvailabilityOptionChoice {
		t.Fatalf("expected availability option prompt, got %+v", got)
	}
	if got.SourceMessageID != "mirror-list" {
		t.Fatalf("expected delivered source message, got %+v", got)
	}
	if got.AvailabilityOptionCount != 1 || !got.HasAvailabilityList {
		t.Fatalf("expected source draft availability count, got %+v", got)
	}
	if promptContext.OptionCount != 1 || !promptContext.HasCurrentFacts {
		t.Fatalf("expected source draft current availability facts, got %+v", promptContext)
	}
}

func TestResolveDeliveredPromptSourceMessageV1RejectsAmbiguousOrCausallyInvalidDraft(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	old := availabilityPromptAuthorityMessage("resolver-old-prompt", "SENT", raw, []int{0})
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"resolver-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		old.ReceivedAt.Add(2*time.Minute),
		old.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		old.ReceivedAt.Add(time.Minute),
	)
	bookablePrefix := []Message{old, selectionInbound, selection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookablePrefix); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE authority: %+v", state)
	}

	for _, projectionKind := range []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: string(deliveredPromptProjectionBotAutoReplyV1)},
		{name: "approved review", mode: string(deliveredPromptProjectionDraftReviewV1), reviewAction: "APPROVED_AS_IS"},
	} {
		t.Run(projectionKind.name, func(t *testing.T) {
			newFixture := func(suffix string) (Message, Message) {
				return availabilityPromptAuthorityProjectionFixtureV1(
					t,
					raw,
					projectionKind.mode,
					projectionKind.reviewAction,
					"resolver-"+strings.ReplaceAll(projectionKind.name+"-"+suffix, " ", "-"),
				)
			}
			duplicateDraft := func(draft Message) Message {
				duplicate := draft
				duplicate.Payload = cloneMap(draft.Payload)
				duplicate.NormalizedPayload = cloneMap(draft.NormalizedPayload)
				return duplicate
			}
			divergentDraft := func(draft Message) Message {
				divergentRaw := raw
				divergentRaw.Results = append([]AvailabilitySearchItem(nil), raw.Results...)
				divergentRaw.Results[2].Price = 4
				divergent := availabilityPromptAuthorityMessage(
					draft.ID,
					draft.ProcessingStatus,
					divergentRaw,
					[]int{2},
				)
				divergent.Payload["mode"] = messageStatusAutomationDraft
				divergent.NormalizedPayload["mode"] = messageStatusAutomationDraft
				if projectionKind.reviewAction != "" {
					divergent.Payload["review_action"] = projectionKind.reviewAction
					divergent.NormalizedPayload["review_action"] = projectionKind.reviewAction
				}
				return divergent
			}

			cases := []struct {
				name             string
				build            func() ([]Message, int, Message)
				checkLatestFails bool
			}{
				{
					name: "missing draft",
					build: func() ([]Message, int, Message) {
						_, projection := newFixture("missing")
						history := append(append([]Message(nil), bookablePrefix...), projection)
						return history, len(history) - 1, projection
					},
					checkLatestFails: true,
				},
				{
					name: "duplicate identical drafts",
					build: func() ([]Message, int, Message) {
						draft, projection := newFixture("duplicate-identical")
						history := append(append([]Message(nil), bookablePrefix...), draft, duplicateDraft(draft), projection)
						return history, len(history) - 1, projection
					},
					checkLatestFails: true,
				},
				{
					name: "duplicate drafts with different facts",
					build: func() ([]Message, int, Message) {
						draft, projection := newFixture("duplicate-divergent")
						history := append(append([]Message(nil), bookablePrefix...), draft, divergentDraft(draft), projection)
						return history, len(history) - 1, projection
					},
					checkLatestFails: true,
				},
				{
					name: "valid and invalid duplicate drafts",
					build: func() ([]Message, int, Message) {
						draft, projection := newFixture("duplicate-invalid")
						invalid := duplicateDraft(draft)
						invalid.Direction = "INBOUND"
						history := append(append([]Message(nil), bookablePrefix...), draft, invalid, projection)
						return history, len(history) - 1, projection
					},
					checkLatestFails: true,
				},
				{
					name: "draft appears after projection",
					build: func() ([]Message, int, Message) {
						draft, projection := newFixture("draft-after")
						history := append(append([]Message(nil), bookablePrefix...), projection, draft)
						return history, len(history) - 2, projection
					},
				},
				{
					name: "draft timestamp is later than projection",
					build: func() ([]Message, int, Message) {
						draft, projection := newFixture("later-timestamp")
						draft.ReceivedAt = projection.ReceivedAt.Add(time.Minute)
						draft.CreatedAt = projection.CreatedAt.Add(time.Minute)
						history := append(append([]Message(nil), bookablePrefix...), draft, projection)
						return history, len(history) - 1, projection
					},
					checkLatestFails: true,
				},
			}

			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					history, projectionIndex, projection := test.build()
					if _, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, projectionIndex, projection); ok {
						t.Fatal("ambiguous or causally invalid draft resolved")
					}
					if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
						t.Fatal("irreconcilable delivered projection did not create a barrier")
					}
					state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
					if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
						t.Fatalf("irreconcilable projection preserved BOOKABLE authority: %+v", state)
					}
					if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar a opção 1", state); ok {
						t.Fatal("irreconcilable projection granted booking_create authority")
					}
					assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
					if test.checkLatestFails {
						if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
							t.Fatalf("historical prompt crossed projection barrier: %+v", availability)
						}
					}
				})
			}

			t.Run("delivered index identity mismatch", func(t *testing.T) {
				draft, projection := newFixture("index-mismatch")
				other := Message{ID: "not-the-projection", Direction: "OUTBOUND"}
				history := append(append([]Message(nil), bookablePrefix...), draft, other, projection)
				if _, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, len(history)-2, projection); ok {
					t.Fatal("resolver accepted an index that does not identify the delivered projection")
				}
			})

			t.Run("unique causal draft controls", func(t *testing.T) {
				for _, control := range []struct {
					name      string
					clearTime bool
				}{
					{name: "equal timestamps"},
					{name: "missing timestamps use slice order", clearTime: true},
				} {
					t.Run(control.name, func(t *testing.T) {
						draft, projection := newFixture("positive-" + strings.ReplaceAll(control.name, " ", "-"))
						if control.clearTime {
							draft.ReceivedAt = time.Time{}
							draft.CreatedAt = time.Time{}
							projection.ReceivedAt = time.Time{}
							projection.CreatedAt = time.Time{}
						}
						history := []Message{draft, projection}
						resolved, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, 1, projection)
						if !ok || sourceIndex != 1 || resolved.ID != projection.ID {
							t.Fatalf("unique causal draft did not resolve delivered identity: ok=%t index=%d resolved=%+v", ok, sourceIndex, resolved)
						}
						if deliveredInvalidAvailabilityPromptBarrierAtV1(history, 1) {
							t.Fatal("valid projection created a barrier")
						}
					})
				}
			})

			t.Run("undelivered ambiguity stays invisible", func(t *testing.T) {
				draft, projection := newFixture("undelivered-duplicate")
				projection.ProcessingStatus = "SEND_FAILED"
				delete(projection.NormalizedPayload, "delivery_recorded_at")
				history := []Message{draft, duplicateDraft(draft), projection}
				if deliveredInvalidAvailabilityPromptBarrierAtV1(history, 2) {
					t.Fatal("UNDELIVERED ambiguous projection created a barrier")
				}
			})
		})
	}
}

func TestResolveDeliveredPromptSourceMessageV1RequiresCompleteDeliveredIndexIdentity(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()

	for _, projectionKind := range []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: string(deliveredPromptProjectionBotAutoReplyV1)},
		{name: "approved review", mode: string(deliveredPromptProjectionDraftReviewV1), reviewAction: "APPROVED_AS_IS"},
	} {
		t.Run(projectionKind.name, func(t *testing.T) {
			newFixture := func(suffix string) (Message, Message) {
				return availabilityPromptAuthorityProjectionFixtureV1(
					t,
					raw,
					projectionKind.mode,
					projectionKind.reviewAction,
					"identity-"+strings.ReplaceAll(projectionKind.name+"-"+suffix, " ", "-"),
				)
			}
			cloneMessage := func(message Message) Message {
				cloned := message
				cloned.Payload = cloneMap(message.Payload)
				cloned.NormalizedPayload = cloneMap(message.NormalizedPayload)
				if message.SentAt != nil {
					sentAt := *message.SentAt
					cloned.SentAt = &sentAt
				}
				return cloned
			}

			for _, test := range []struct {
				name   string
				mutate func(*Message)
			}{
				{
					name: "different message with same id",
					mutate: func(indexed *Message) {
						indexed.Direction = "INBOUND"
						indexed.Body = "Outra mensagem persistida"
					},
				},
				{
					name: "kind divergence",
					mutate: func(indexed *Message) {
						indexed.Kind = "AUDIO"
					},
				},
				{
					name: "timestamp divergence",
					mutate: func(indexed *Message) {
						indexed.CreatedAt = indexed.CreatedAt.Add(time.Minute)
					},
				},
				{
					name: "delivery metadata divergence",
					mutate: func(indexed *Message) {
						indexed.NormalizedPayload["delivery_recorded_at"] = indexed.CreatedAt.Add(time.Minute).Format(time.RFC3339Nano)
					},
				},
				{
					name: "event divergence",
					mutate: func(indexed *Message) {
						indexed.Payload[testAvailabilityPromptEventV1MessageKey] = "malformed-indexed-event"
						indexed.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = "malformed-indexed-event"
					},
				},
				{
					name: "draft message id divergence",
					mutate: func(indexed *Message) {
						indexed.Payload["draft_message_id"] = "other-draft"
						indexed.NormalizedPayload["draft_message_id"] = "other-draft"
					},
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					draft, delivered := newFixture(strings.ReplaceAll(test.name, " ", "-"))
					indexed := cloneMessage(delivered)
					test.mutate(&indexed)
					history := []Message{draft, indexed}
					if resolved, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, 1, delivered); ok {
						t.Fatalf("resolver accepted mismatched delivered identity: index=%d resolved=%+v", sourceIndex, resolved)
					}
				})
			}

			t.Run("complete identity control", func(t *testing.T) {
				draft, delivered := newFixture("positive")
				indexed := cloneMessage(delivered)
				history := []Message{draft, indexed}
				resolved, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, 1, delivered)
				if !ok || sourceIndex != 1 || resolved.ID != delivered.ID {
					t.Fatalf("complete delivered identity did not resolve: ok=%t index=%d resolved=%+v", ok, sourceIndex, resolved)
				}
			})
		})
	}
}

func TestAvailabilityPromptBarrierScopesUnresolvedProjectionToAvailability(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	prompt := availabilityPromptAuthorityMessage("scoped-old-prompt", "DELIVERED", raw, []int{0})
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"scoped-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		prompt.ReceivedAt.Add(2*time.Minute),
		prompt.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		prompt.ReceivedAt.Add(time.Minute),
	)
	bookableHistory := []Message{prompt, selectionInbound, selection}
	bookable := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookableHistory)
	if bookable.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize BOOKABLE: %+v", bookable)
	}

	draftAt := selection.ReceivedAt.Add(time.Minute)
	passengerDraft := Message{
		ID:               "passenger-review-draft",
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationReviewed,
		ReceivedAt:       draftAt,
		CreatedAt:        draftAt,
		Payload: map[string]interface{}{
			"mode":          messageStatusAutomationDraft,
			"review_action": "APPROVED_AS_IS",
			"tool_context":  "malformed-tool-context",
		},
		NormalizedPayload: map[string]interface{}{
			"mode":          messageStatusAutomationDraft,
			"review_action": "APPROVED_AS_IS",
			"tool_context":  "malformed-tool-context",
		},
	}
	now := draftAt.Add(time.Minute)
	passengerReview := Message{
		ID:               "passenger-review-projection",
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             askPassengerCountReply,
		ProcessingStatus: "SENT",
		ReceivedAt:       now,
		CreatedAt:        now,
		Payload: map[string]interface{}{
			"mode":             string(deliveredPromptProjectionDraftReviewV1),
			"review_action":    "APPROVED_AS_IS",
			"draft_message_id": passengerDraft.ID,
		},
		NormalizedPayload: map[string]interface{}{
			"mode":                 string(deliveredPromptProjectionDraftReviewV1),
			"review_action":        "APPROVED_AS_IS",
			"draft_message_id":     passengerDraft.ID,
			"delivery_recorded_at": now.Format(time.RFC3339Nano),
		},
	}
	history := append(append([]Message(nil), bookableHistory...), passengerDraft, passengerReview)
	projectionIndex := len(history) - 1
	if messageMayCarryAvailabilityPromptV1(passengerReview) {
		t.Fatal("passenger-only review projection became an availability candidate")
	}
	resolved, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, projectionIndex, passengerReview)
	if !ok || sourceIndex != projectionIndex || resolved.ID != passengerReview.ID {
		t.Fatalf("passenger review did not resolve its approved draft: ok=%t index=%d message=%+v", ok, sourceIndex, resolved)
	}
	if messageMayCarryAvailabilityPromptV1(resolved) {
		t.Fatal("resolved passenger-only review became an availability candidate")
	}
	if authority := classifyAvailabilityPromptCandidateV1(resolved); authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
		t.Fatalf("malformed non-availability facts were reclassified as legacy authority: %+v", authority)
	}
	if message, _, found := classifiedAvailabilityPromptMessageAtV1(history, projectionIndex); found {
		t.Fatalf("availability reader published resolved passenger prompt as ABSENT_LEGACY: %+v", message)
	}
	if count := availabilityOptionCountFromMessage(resolved); count != 0 {
		t.Fatalf("resolved passenger prompt exposed availability option count=%d", count)
	}
	if deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
		t.Fatal("passenger-only review projection created an availability barrier")
	}
	if got := latestDeliveredInvalidAvailabilityPromptIndexV1(history, len(history)); got >= 0 {
		t.Fatalf("passenger-only review projection produced barrier index=%d", got)
	}
	active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})
	if active.Kind != ActivePromptPassengerCount || active.SourceMessageID != passengerReview.ID {
		t.Fatalf("passenger-only review projection was not preserved as passenger prompt: %+v", active)
	}
	availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history)
	if !ok || availability == nil || sourceIndex < 0 {
		t.Fatalf("unrelated passenger prompt hid prior availability: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
	}
	bootstrapped := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
	if bootstrapped.Status != AvailabilitySelectionStatusBookable || bootstrapped.SelectedOptionIndex != bookable.SelectedOptionIndex {
		t.Fatalf("unrelated passenger prompt erased bootstrapped BOOKABLE: got=%+v want=%+v", bootstrapped, bookable)
	}
	session := sessionWithAvailabilitySelectionStateForTest(Session{
		ContactKey:    "passenger-review-scope",
		CustomerPhone: "5500000000000",
		CustomerName:  "Pessoa Teste",
	}, bookable)
	readState := availabilitySelectionStateV1ForRead(session, history)
	if readState.Status != AvailabilitySelectionStatusBookable || readState.SelectedOptionIndex != bookable.SelectedOptionIndex {
		t.Fatalf("unrelated passenger prompt erased BOOKABLE: got=%+v want=%+v", readState, bookable)
	}
	draftContext := collectBookingDraftContextWithPassengerState(
		session,
		history,
		"quero reservar",
		completePassengerStateForTest(1, 0),
	)
	if !draftContext.HasBookableSelection || draftContext.SelectedOptionIndex != bookable.SelectedOptionIndex {
		t.Fatalf("unrelated passenger prompt erased booking draft authority: %+v", draftContext)
	}
	if input, ok := parseBookingCreateInputWithPassengerState(
		session,
		history,
		"quero reservar\nPessoa Teste | CPF | 84960815086",
		nil,
		completePassengerStateForTest(1, 0),
	); !ok || input.SelectedOptionIndex != bookable.SelectedOptionIndex {
		t.Fatalf("unrelated passenger prompt erased booking_create authority: ok=%t input=%+v", ok, input)
	}
}

func TestAvailabilityPromptTextFallbackRejectsExplicitNonAvailabilityDomains(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	prompt := availabilityPromptAuthorityMessage("fallback-domain-old-prompt", "DELIVERED", raw, []int{0})
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"fallback-domain-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		prompt.ReceivedAt.Add(2*time.Minute),
		prompt.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		prompt.ReceivedAt.Add(time.Minute),
	)
	bookableHistory := []Message{prompt, selectionInbound, selection}
	bookable := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookableHistory)
	if bookable.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize BOOKABLE: %+v", bookable)
	}

	domains := []struct {
		name           string
		body           string
		intent         Intent
		templateName   ResponseTemplateName
		activePrompt   ActivePromptKind
		wantActiveKind ActivePromptKind
	}{
		{
			name:   "payment intent",
			body:   "Pode pagar no PIX ou no cartão. Qual opção você prefere?",
			intent: IntentPaymentMethodQuestion,
		},
		{
			name:           "passenger count",
			body:           "A passagem é só para você ou vai mais alguém? Qual opção você prefere?",
			templateName:   TemplateAskPassengerCount,
			wantActiveKind: ActivePromptPassengerCount,
		},
		{
			name:           "document prompt kind",
			body:           "Pode enviar uma foto do documento ou escrever os dados. Qual opção você prefere?",
			activePrompt:   ActivePromptPassengerDocuments,
			wantActiveKind: ActivePromptPassengerDocuments,
		},
		{
			name:         "human support template",
			body:         "Qual opção você prefere para falar com o atendimento humano?",
			templateName: TemplateHumanHandoff,
		},
		{
			name:   "cancellation",
			body:   "Qual opção você prefere para cancelar a reserva?",
			intent: IntentBookingCancel,
		},
	}
	modes := []struct {
		name         string
		mode         deliveredPromptProjectionKindV1
		reviewAction bool
	}{
		{name: "bot auto reply", mode: deliveredPromptProjectionBotAutoReplyV1},
		{name: "approved review", mode: deliveredPromptProjectionDraftReviewV1, reviewAction: true},
	}

	for _, mode := range modes {
		mode := mode
		for _, domain := range domains {
			domain := domain
			t.Run(mode.name+"/"+domain.name, func(t *testing.T) {
				if !messageLooksLikeAvailabilitySelectionPrompt(Message{Body: domain.body}) {
					t.Fatal("test precondition did not trigger the generic availability text fallback")
				}
				now := selection.ReceivedAt.Add(2 * time.Minute)
				payload := map[string]interface{}{
					"mode":             string(mode.mode),
					"draft_message_id": "missing-" + strings.ReplaceAll(domain.name, " ", "-"),
				}
				if domain.intent != "" {
					payload["intent"] = string(domain.intent)
				}
				if domain.templateName != "" {
					payload["template_name"] = string(domain.templateName)
				}
				if domain.activePrompt != "" {
					payload[outOfTurnActivePromptTemplateDataKey] = string(domain.activePrompt)
				}
				if mode.reviewAction {
					payload["review_action"] = "APPROVED_AS_IS"
				}
				normalized := cloneMap(payload)
				normalized["delivery_recorded_at"] = now.Format(time.RFC3339Nano)
				projection := Message{
					ID:                "fallback-domain-" + strings.ReplaceAll(mode.name+"-"+domain.name, " ", "-"),
					Direction:         "OUTBOUND",
					Kind:              "TEXT",
					Body:              domain.body,
					ProcessingStatus:  "SENT",
					ReceivedAt:        now,
					CreatedAt:         now,
					Payload:           payload,
					NormalizedPayload: normalized,
				}
				history := append(append([]Message(nil), bookableHistory...), projection)
				projectionIndex := len(history) - 1

				if messageMayCarryAvailabilityPromptV1(projection) {
					t.Fatal("explicit non-availability projection became an availability candidate")
				}
				if deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
					t.Fatal("explicit non-availability projection created an availability barrier")
				}
				if message, _, found := classifiedAvailabilityPromptMessageAtV1(history, projectionIndex); found {
					t.Fatalf("availability reader published non-availability projection as ABSENT_LEGACY: %+v", message)
				}
				if count := availabilityOptionCountFromMessage(projection); count != 0 {
					t.Fatalf("non-availability projection exposed availability option count=%d", count)
				}
				latest, latestIndex, ok := latestReliableAssistantMessageWithIndex(history)
				if !ok || latestIndex != projectionIndex || latest.ID != projection.ID {
					t.Fatalf("own-domain projection was not preserved by the normal scan: ok=%t index=%d message=%+v", ok, latestIndex, latest)
				}
				if domain.intent != "" {
					if got := Intent(payloadMetadataString(latest.Payload, "intent")); got != domain.intent {
						t.Fatalf("own-domain intent was lost: got=%s want=%s", got, domain.intent)
					}
				}
				if domain.templateName != "" {
					if got := ResponseTemplateName(payloadMetadataString(latest.Payload, "template_name")); got != domain.templateName {
						t.Fatalf("own-domain template was lost: got=%s want=%s", got, domain.templateName)
					}
				}
				if domain.activePrompt != "" {
					if got := ActivePromptKind(payloadMetadataString(latest.Payload, outOfTurnActivePromptTemplateDataKey)); got != domain.activePrompt {
						t.Fatalf("own-domain prompt kind was lost: got=%s want=%s", got, domain.activePrompt)
					}
				}
				if domain.wantActiveKind != "" {
					active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})
					if active.Kind != domain.wantActiveKind || active.SourceMessageID != projection.ID {
						t.Fatalf("own-domain active prompt was not preserved: got=%+v want=%s", active, domain.wantActiveKind)
					}
				}
				availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history)
				if !ok || availability == nil || sourceIndex < 0 {
					t.Fatalf("non-availability projection hid prior availability: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
				}
				bootstrapped := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
				if bootstrapped.Status != AvailabilitySelectionStatusBookable || bootstrapped.SelectedOptionIndex != bookable.SelectedOptionIndex {
					t.Fatalf("non-availability projection erased bootstrapped BOOKABLE: got=%+v want=%+v", bootstrapped, bookable)
				}
				session := sessionWithAvailabilitySelectionStateForTest(Session{
					ContactKey:    "fallback-domain-" + domain.name,
					CustomerPhone: "5500000000000",
					CustomerName:  "Pessoa Teste",
				}, bookable)
				readState := availabilitySelectionStateV1ForRead(session, history)
				if readState.Status != AvailabilitySelectionStatusBookable || readState.SelectedOptionIndex != bookable.SelectedOptionIndex {
					t.Fatalf("non-availability projection erased BOOKABLE: got=%+v want=%+v", readState, bookable)
				}
				draftContext := collectBookingDraftContextWithPassengerState(
					session,
					history,
					"quero reservar",
					completePassengerStateForTest(1, 0),
				)
				if !draftContext.HasBookableSelection || draftContext.SelectedOptionIndex != bookable.SelectedOptionIndex {
					t.Fatalf("non-availability projection erased booking draft authority: %+v", draftContext)
				}
				if input, ok := parseBookingCreateInputWithPassengerState(
					session,
					history,
					"quero reservar\nPessoa Teste | CPF | 84960815086",
					nil,
					completePassengerStateForTest(1, 0),
				); !ok || input.SelectedOptionIndex != bookable.SelectedOptionIndex {
					t.Fatalf("non-availability projection erased booking_create authority: ok=%t input=%+v", ok, input)
				}
			})
		}
	}

	t.Run("legacy availability body remains a fallback candidate", func(t *testing.T) {
		now := prompt.ReceivedAt.Add(10 * time.Minute)
		legacy := Message{
			ID:               "fallback-domain-legacy-availability",
			Direction:        "OUTBOUND",
			Kind:             "TEXT",
			Body:             buildAvailabilityListReplyForResultIndexes(raw, []int{0}),
			ProcessingStatus: "SENT",
			ReceivedAt:       now,
			CreatedAt:        now,
			NormalizedPayload: map[string]interface{}{
				"delivery_recorded_at": now.Format(time.RFC3339Nano),
			},
		}
		if !messageMayCarryAvailabilityPromptV1(legacy) {
			t.Fatal("real legacy availability list lost the text fallback")
		}
	})

	t.Run("structural availability artifacts do not mask conflicting prompt metadata", func(t *testing.T) {
		structural := availabilityPromptAuthorityMessage("fallback-domain-structural-control", "DELIVERED", raw, []int{0})
		structural.Payload[outOfTurnActivePromptTemplateDataKey] = string(ActivePromptPassengerCount)
		structural.NormalizedPayload[outOfTurnActivePromptTemplateDataKey] = string(ActivePromptPassengerCount)
		if !messageMayCarryAvailabilityPromptV1(structural) {
			t.Fatal("invalid structural availability artifact lost domain candidacy")
		}
		authority := classifyAvailabilityPromptCandidateV1(structural)
		if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
			t.Fatalf("conflicting structural availability classification=%+v, want INVALID without Presented", authority)
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{structural}, 0) {
			t.Fatal("delivered cross-domain availability conflict did not create a barrier")
		}
	})
}

func TestAvailabilityPromptDomainReconciliationV1(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	newMessage := func(id string, body string, payload map[string]interface{}, normalized map[string]interface{}) Message {
		if payload == nil {
			payload = map[string]interface{}{}
		}
		if normalized == nil {
			normalized = map[string]interface{}{}
		}
		normalized = cloneMap(normalized)
		normalized["delivery_recorded_at"] = now.Format(time.RFC3339Nano)
		return Message{
			ID:                id,
			Direction:         "OUTBOUND",
			Kind:              "TEXT",
			Body:              body,
			ProcessingStatus:  "SENT",
			ReceivedAt:        now,
			CreatedAt:         now,
			Payload:           cloneMap(payload),
			NormalizedPayload: normalized,
		}
	}
	genericBody := "Qual opção você prefere?"
	negativeCases := []struct {
		name       string
		payload    map[string]interface{}
		normalized map[string]interface{}
		candidate  bool
	}{
		{
			name:      "payload availability versus normalized payment",
			candidate: true,
			payload: map[string]interface{}{
				"intent": string(IntentAvailabilitySearch),
			},
			normalized: map[string]interface{}{
				"intent": string(IntentPaymentMethodQuestion),
			},
		},
		{
			name:      "intent conflicts with template",
			candidate: true,
			payload: map[string]interface{}{
				"intent":        string(IntentAvailabilitySearch),
				"template_name": string(TemplateAskPaymentChoice),
			},
			normalized: map[string]interface{}{
				"intent":        string(IntentAvailabilitySearch),
				"template_name": string(TemplateAskPaymentChoice),
			},
		},
		{
			name:      "template conflicts with prompt kind",
			candidate: true,
			payload: map[string]interface{}{
				"template_name": string(TemplateAvailabilityList),
				"template_data": map[string]interface{}{
					outOfTurnActivePromptTemplateDataKey: string(ActivePromptPassengerCount),
				},
			},
			normalized: map[string]interface{}{
				"template_name": string(TemplateAvailabilityList),
				"template_data": map[string]interface{}{
					outOfTurnActivePromptTemplateDataKey: string(ActivePromptPassengerCount),
				},
			},
		},
		{
			name:      "availability metadata in only one copy",
			candidate: true,
			payload: map[string]interface{}{
				"intent": string(IntentAvailabilitySearch),
			},
			normalized: map[string]interface{}{},
		},
		{
			name:      "availability versus empty metadata",
			candidate: true,
			payload: map[string]interface{}{
				"intent": string(IntentAvailabilitySearch),
			},
			normalized: map[string]interface{}{
				"intent": "",
			},
		},
		{
			name:      "availability versus unknown metadata",
			candidate: true,
			payload: map[string]interface{}{
				"intent": string(IntentAvailabilitySearch),
			},
			normalized: map[string]interface{}{
				"intent": "UNKNOWN_AVAILABILITY_INTENT",
			},
		},
		{
			name:      "availability versus non string metadata",
			candidate: true,
			payload: map[string]interface{}{
				"intent": string(IntentAvailabilitySearch),
			},
			normalized: map[string]interface{}{
				"intent": 1,
			},
		},
		{
			name: "bilateral payment metadata",
			payload: map[string]interface{}{
				"intent": string(IntentPaymentMethodQuestion),
			},
			normalized: map[string]interface{}{
				"intent": string(IntentPaymentMethodQuestion),
			},
		},
	}
	for _, test := range negativeCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			message := newMessage("domain-negative-"+strings.ReplaceAll(test.name, " ", "-"), genericBody, test.payload, test.normalized)
			if got := messageMayCarryAvailabilityPromptV1(message); got != test.candidate {
				t.Fatalf("candidate=%t, want %t", got, test.candidate)
			}
			if got := deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{message}, 0); got != test.candidate {
				t.Fatalf("barrier=%t, want %t", got, test.candidate)
			}
		})
	}

	optionFallbackBody, ok := realizeResponseTemplate(TemplateContextFallbackAvailabilityOption)
	if !ok {
		t.Fatal("missing option fallback template fixture")
	}
	dateFallbackBody, ok := realizeResponseTemplate(TemplateContextFallbackAvailabilityDate)
	if !ok {
		t.Fatal("missing date fallback template fixture")
	}
	positiveCases := []struct {
		name       string
		key        string
		value      string
		body       string
		activeKind ActivePromptKind
	}{
		{
			name:       "option context fallback template",
			key:        "template_name",
			value:      string(TemplateContextFallbackAvailabilityOption),
			body:       optionFallbackBody,
			activeKind: ActivePromptAvailabilityOptionChoice,
		},
		{
			name:       "date context fallback template",
			key:        "template_name",
			value:      string(TemplateContextFallbackAvailabilityDate),
			body:       dateFallbackBody,
			activeKind: ActivePromptAvailabilityDateChoice,
		},
		{
			name:       "availability option prompt kind",
			key:        outOfTurnActivePromptTemplateDataKey,
			value:      string(ActivePromptAvailabilityOptionChoice),
			body:       optionFallbackBody,
			activeKind: ActivePromptAvailabilityOptionChoice,
		},
		{
			name:       "availability date prompt kind",
			key:        outOfTurnActivePromptTemplateDataKey,
			value:      string(ActivePromptAvailabilityDateChoice),
			body:       dateFallbackBody,
			activeKind: ActivePromptAvailabilityDateChoice,
		},
	}
	for _, test := range positiveCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			payload := map[string]interface{}{test.key: test.value}
			if test.key == outOfTurnActivePromptTemplateDataKey {
				payload = map[string]interface{}{
					"template_data": map[string]interface{}{test.key: test.value},
				}
			}
			message := newMessage("domain-positive-"+strings.ReplaceAll(test.name, " ", "-"), test.body, payload, payload)
			if !messageMayCarryAvailabilityPromptV1(message) {
				t.Fatal("bilateral canonical availability metadata was not recognized")
			}
			active := InferActivePromptContext([]Message{message}, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
			if active.Kind != test.activeKind || active.SourceMessageID != message.ID {
				t.Fatalf("canonical availability metadata lost active prompt domain: got=%+v want=%s", active, test.activeKind)
			}
		})
	}

	raw := availabilityPromptAuthorityRawResult()
	t.Run("bilateral structural event conflicts with another domain", func(t *testing.T) {
		message := availabilityPromptAuthorityMessage("domain-structural-event", "DELIVERED", raw, []int{0})
		message.Payload["intent"] = string(IntentPaymentMethodQuestion)
		message.NormalizedPayload["intent"] = string(IntentPaymentMethodQuestion)
		if !messageMayCarryAvailabilityPromptV1(message) {
			t.Fatal("conflicting availability event lost domain candidacy")
		}
		if _, ok := availabilityPromptClassifiedContextV1(message); ok {
			t.Fatal("availability event with conflicting payment metadata published facts")
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{message}, 0) {
			t.Fatal("delivered cross-domain availability conflict did not create a barrier")
		}
	})
	t.Run("bilateral structural facts conflict with another domain", func(t *testing.T) {
		message := availabilityPromptAuthorityMessage("domain-structural-facts", "DELIVERED", raw, []int{0})
		delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
		delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
		message.Payload["intent"] = string(IntentPaymentMethodQuestion)
		message.NormalizedPayload["intent"] = string(IntentPaymentMethodQuestion)
		if !messageMayCarryAvailabilityPromptV1(message) {
			t.Fatal("conflicting bilateral availability facts lost domain candidacy")
		}
		if _, ok := availabilityPromptClassifiedContextV1(message); ok {
			t.Fatal("availability facts with conflicting payment metadata were published")
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{message}, 0) {
			t.Fatal("delivered conflicting availability facts did not create a barrier")
		}
	})
	t.Run("bilateral structural selection conflicts with another domain", func(t *testing.T) {
		snapshot := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
		payload := map[string]interface{}{
			"intent":                             string(IntentPaymentMethodQuestion),
			"selected_option_index":              1,
			selectedAvailabilityResultPayloadKey: snapshot,
		}
		message := newMessage("domain-structural-selection", "Seleção persistida", payload, payload)
		if !messageMayCarryAvailabilityPromptV1(message) {
			t.Fatal("conflicting bilateral availability selection lost domain candidacy")
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{message}, 0) {
			t.Fatal("delivered conflicting availability selection did not create a barrier")
		}
	})
	t.Run("availability and passenger events conflict", func(t *testing.T) {
		message := availabilityPromptAuthorityMessage("domain-event-conflict", "DELIVERED", raw, []int{0})
		passengerEvent, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, message.ID)
		if !ok {
			t.Fatal("failed to build passenger event fixture")
		}
		message.Payload[passengerPromptEventV1MessageKey] = passengerEvent
		message.NormalizedPayload[passengerPromptEventV1MessageKey] = passengerEvent
		if !messageMayCarryAvailabilityPromptV1(message) {
			t.Fatal("availability event with passenger conflict lost domain candidacy")
		}
		if _, ok := availabilityPromptClassifiedContextV1(message); ok {
			t.Fatal("cross-domain event conflict published availability facts")
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{message}, 0) {
			t.Fatal("delivered availability/passenger conflict did not create a barrier")
		}
	})
	t.Run("valid facts do not mask malformed availability artifact", func(t *testing.T) {
		message := availabilityPromptAuthorityMessage("domain-malformed-selection", "DELIVERED", raw, []int{0})
		delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
		delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
		message.Payload["selected_option_index"] = "one"
		message.NormalizedPayload["selected_option_index"] = "one"
		if authority := classifyAvailabilityPromptCandidateV1(message); authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
			t.Fatalf("malformed availability selection was masked by valid facts: %+v", authority)
		}
		if _, ok := availabilityPromptClassifiedContextV1(message); ok {
			t.Fatal("malformed availability selection published facts")
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{message}, 0) {
			t.Fatal("malformed availability artifact did not remain fail-closed")
		}
	})
}

func TestAvailabilityPromptTextFallbackUsesRealProjectionShapeV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	prompt := availabilityPromptAuthorityMessage("real-shape-old-prompt", "DELIVERED", raw, []int{0})
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"real-shape-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		prompt.ReceivedAt.Add(2*time.Minute),
		prompt.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(selection, prompt.ReceivedAt.Add(time.Minute))
	bookableHistory := []Message{prompt, selectionInbound, selection}
	bookable := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookableHistory)
	if bookable.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize BOOKABLE: %+v", bookable)
	}

	domains := []struct {
		name string
		body string
	}{
		{name: "payment", body: "Pode pagar no PIX ou no cartão. Qual opção você prefere?"},
		{name: "passenger", body: "A passagem é só para você ou vai mais alguém? Qual opção você prefere?"},
		{name: "document", body: "Pode enviar uma foto do documento ou escrever os dados. Qual opção você prefere?"},
		{name: "human", body: "Qual opção você prefere para falar com o atendimento humano?"},
		{name: "cancellation", body: "Qual opção você prefere para cancelar a reserva?"},
	}
	modes := []struct {
		name         string
		mode         deliveredPromptProjectionKindV1
		reviewAction string
	}{
		{name: "bot auto reply", mode: deliveredPromptProjectionBotAutoReplyV1},
		{name: "approved review", mode: deliveredPromptProjectionDraftReviewV1, reviewAction: "APPROVED_AS_IS"},
	}
	failures := []string{"missing", "duplicate", "posterior", "invalid"}

	for _, mode := range modes {
		mode := mode
		for _, domain := range domains {
			domain := domain
			for _, failure := range failures {
				failure := failure
				t.Run(mode.name+"/"+domain.name+"/"+failure, func(t *testing.T) {
					suffix := strings.ReplaceAll(mode.name+"-"+domain.name+"-"+failure, " ", "-")
					draftID := "real-shape-draft-" + suffix
					now := selection.ReceivedAt.Add(2 * time.Minute)
					projectionPayload := map[string]interface{}{
						"mode":             string(mode.mode),
						"draft_message_id": draftID,
					}
					if mode.mode == deliveredPromptProjectionBotAutoReplyV1 {
						projectionPayload["draft_auto_sent"] = true
					} else {
						projectionPayload["review_mode"] = "CONTROLLED"
						projectionPayload["review_action"] = mode.reviewAction
						projectionPayload["draft_reviewed"] = true
					}
					projectionNormalized := cloneMap(projectionPayload)
					projectionNormalized["delivery_recorded_at"] = now.Format(time.RFC3339Nano)
					projection := Message{
						ID:                "real-shape-projection-" + suffix,
						Direction:         "OUTBOUND",
						Kind:              "TEXT",
						Body:              domain.body,
						ProcessingStatus:  "SENT",
						ReceivedAt:        now,
						CreatedAt:         now,
						Payload:           projectionPayload,
						NormalizedPayload: projectionNormalized,
					}
					for _, payload := range []map[string]interface{}{projection.Payload, projection.NormalizedPayload} {
						if _, present := payload["intent"]; present {
							t.Fatal("real projection fixture must not inject intent")
						}
						if _, present := payload["template_name"]; present {
							t.Fatal("real projection fixture must not inject template_name")
						}
						if _, present := payload[outOfTurnActivePromptTemplateDataKey]; present {
							t.Fatal("real projection fixture must not inject active_prompt_kind")
						}
					}

					draftStatus := messageStatusAutomationSent
					if mode.mode == deliveredPromptProjectionDraftReviewV1 {
						draftStatus = messageStatusAutomationReviewed
					}
					draftPayload := map[string]interface{}{"mode": messageStatusAutomationDraft}
					if mode.reviewAction != "" {
						draftPayload["review_action"] = mode.reviewAction
					}
					draft := Message{
						ID:                draftID,
						Direction:         "OUTBOUND",
						Kind:              "TEXT",
						Body:              domain.body,
						ProcessingStatus:  draftStatus,
						ReceivedAt:        now.Add(-time.Minute),
						CreatedAt:         now.Add(-time.Minute),
						Payload:           cloneMap(draftPayload),
						NormalizedPayload: cloneMap(draftPayload),
					}
					history := append([]Message(nil), bookableHistory...)
					projectionIndex := -1
					switch failure {
					case "missing":
						history = append(history, projection)
						projectionIndex = len(history) - 1
					case "duplicate":
						history = append(history, draft, draft, projection)
						projectionIndex = len(history) - 1
					case "posterior":
						history = append(history, projection, draft)
						projectionIndex = len(history) - 2
					case "invalid":
						draft.Payload["mode"] = "INVALID_DRAFT_MODE"
						draft.NormalizedPayload["mode"] = "INVALID_DRAFT_MODE"
						history = append(history, draft, projection)
						projectionIndex = len(history) - 1
					default:
						t.Fatalf("unknown failure fixture %q", failure)
					}
					if _, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, projectionIndex, projection); ok {
						t.Fatal("real-shape projection unexpectedly resolved")
					}
					if messageMayCarryAvailabilityPromptV1(projection) {
						t.Fatal("non-availability body-only projection became an availability candidate")
					}
					if deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
						t.Fatal("non-availability body-only projection created an availability barrier")
					}
					active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePassengerCollection})
					if active.Kind == ActivePromptAvailabilityOptionChoice || active.Kind == ActivePromptAvailabilityDateChoice {
						t.Fatalf("non-availability body-only projection inferred availability active prompt: %+v", active)
					}
					availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history)
					if !ok || availability == nil || sourceIndex < 0 {
						t.Fatalf("non-availability projection hid prior finder authority: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
					}
					bootstrapped := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
					if bootstrapped.Status != AvailabilitySelectionStatusBookable || bootstrapped.SelectedOptionIndex != bookable.SelectedOptionIndex {
						t.Fatalf("non-availability projection erased bootstrapped BOOKABLE: got=%+v want=%+v", bootstrapped, bookable)
					}
					session := sessionWithAvailabilitySelectionStateForTest(Session{
						ContactKey:    "real-shape-" + suffix,
						CustomerPhone: "5500000000000",
						CustomerName:  "Pessoa Teste",
					}, bookable)
					readState := availabilitySelectionStateV1ForRead(session, history)
					if readState.Status != AvailabilitySelectionStatusBookable || readState.SelectedOptionIndex != bookable.SelectedOptionIndex {
						t.Fatalf("non-availability projection erased AvailabilitySelectionStateV1: got=%+v want=%+v", readState, bookable)
					}
					draftContext := collectBookingDraftContextWithPassengerState(
						session,
						history,
						"quero reservar",
						completePassengerStateForTest(1, 0),
					)
					if !draftContext.HasBookableSelection || draftContext.SelectedOptionIndex != bookable.SelectedOptionIndex {
						t.Fatalf("non-availability projection erased booking draft: %+v", draftContext)
					}
					if input, ok := parseBookingCreateInputWithPassengerState(
						session,
						history,
						"quero reservar\nPessoa Teste | CPF | 84960815086",
						nil,
						completePassengerStateForTest(1, 0),
					); !ok || input.SelectedOptionIndex != bookable.SelectedOptionIndex {
						t.Fatalf("non-availability projection erased booking_create authority: ok=%t input=%+v", ok, input)
					}
				})
			}
		}
	}
}

func TestAvailabilityPromptLegacyBuilderFallbacksV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "numbered list", body: buildAvailabilityListReplyForResultIndexes(raw, []int{0, 1})},
		{name: "earliest", body: buildEarliestAvailabilityReply(raw.Results[0])},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			message := Message{
				ID:               "legacy-builder-" + strings.ReplaceAll(test.name, " ", "-"),
				Direction:        "OUTBOUND",
				Kind:             "TEXT",
				Body:             test.body,
				ProcessingStatus: "SENT",
				ReceivedAt:       now,
				CreatedAt:        now,
				NormalizedPayload: map[string]interface{}{
					"delivery_recorded_at": now.Format(time.RFC3339Nano),
				},
			}
			if !messageMayCarryAvailabilityPromptV1(message) {
				t.Fatal("real legacy availability builder body was not recognized")
			}
			active := InferActivePromptContext([]Message{message}, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
			if active.Kind != ActivePromptAvailabilityOptionChoice || active.SourceMessageID != message.ID {
				t.Fatalf("legacy availability builder body lost active prompt: %+v", active)
			}
			if count := availabilityOptionCountFromMessage(message); count <= 0 {
				t.Fatalf("legacy availability builder body lost option count: %d", count)
			}
		})
	}
}

func TestAvailabilityPromptLegacyBuilderFallbackRejectsAlteredBodiesV1(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "generic payment list", body: availabilityListReplyHeader + "1. PIX\n2. Cartão" + availabilityListReplyQuestion},
		{name: "non builder separators", body: availabilityListReplyHeader + "1. Monção/MA | Videira/SC | 2026-08-03" + availabilityListReplyQuestion},
		{name: "arbitrary option", body: availabilityListReplyHeader + "1. qualquer texto" + availabilityListReplyQuestion},
		{name: "non sequential list", body: availabilityListReplyHeader + "1. Monção/MA para Videira/SC, 2026-08-03\n3. Monção/MA para Videira/SC, 2026-08-04" + availabilityListReplyQuestion},
		{name: "partial prefix", body: "Encontrei:\n1. Monção/MA para Videira/SC, 2026-08-03" + availabilityListReplyQuestion},
		{name: "partial suffix", body: availabilityListReplyHeader + "1. Monção/MA para Videira/SC, 2026-08-03\n\nQual prefere?"},
		{name: "free earliest", body: earliestAvailabilityReplyPrefix + "qualquer coisa" + earliestAvailabilityReplyQuestion},
		{name: "altered earliest separators", body: earliestAvailabilityReplyPrefix + "03/08/2026 | saída 08:00 | PIX" + earliestAvailabilityReplyQuestion},
		{name: "empty", body: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if kind, count, ok := legacyAvailabilityPromptBodyV1(test.body); ok || kind != ActivePromptUnknown || count != 0 {
				t.Fatalf("altered body recognized as builder output: kind=%s count=%d", kind, count)
			}
		})
	}
}

func TestOutOfTurnAvailabilityAnchorResolvesEffectiveSourceV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	draft, projection := availabilityPromptAuthorityProjectionFixtureV1(
		t,
		raw,
		string(deliveredPromptProjectionBotAutoReplyV1),
		"",
		"out-of-turn-effective-source",
	)
	reminder := outOfTurnAvailabilityReminderForTest(
		"out-of-turn-projection-reminder",
		projection.ID,
		projection.ReceivedAt.Add(time.Minute),
		nil,
	)
	history := []Message{draft, projection, reminder}

	resolved, sourceIndex, kind, ok := reliableOutOfTurnActivePromptSourceV1(history, 2, reminder)
	if !ok || sourceIndex != 1 || resolved.ID != projection.ID || kind != ActivePromptAvailabilityOptionChoice {
		t.Fatalf("projection anchor did not resolve effective source: ok=%t index=%d kind=%s source=%+v", ok, sourceIndex, kind, resolved)
	}
	authority := classifyAvailabilityPromptCandidateV1(resolved)
	if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil || len(authority.Presented.Results) != 1 {
		t.Fatalf("resolved projection lost structural authority: %+v", authority)
	}
	active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
	if active.Kind != ActivePromptAvailabilityOptionChoice || active.SourceMessageID != projection.ID || active.AvailabilityOptionCount != 1 {
		t.Fatalf("out-of-turn projection continuity lost effective source: %+v", active)
	}
}

func TestOutOfTurnAvailabilityAnchorRejectsOtherDomainWithCopiedFactsV1(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	raw := availabilityPromptAuthorityRawResult()
	toolContext := map[string]interface{}{
		toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(raw),
	}
	sourcePayload := map[string]interface{}{
		"intent":       string(IntentPaymentMethodQuestion),
		"tool_context": toolContext,
	}
	source := Message{
		ID:               "payment-source-with-copied-facts",
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             askPaymentChoiceReply,
		ProcessingStatus: "SENT",
		ReceivedAt:       now,
		CreatedAt:        now,
		Payload:          cloneMap(sourcePayload),
		NormalizedPayload: func() map[string]interface{} {
			payload := cloneMap(sourcePayload)
			payload["delivery_recorded_at"] = now.Format(time.RFC3339Nano)
			return payload
		}(),
	}
	reminder := outOfTurnAvailabilityReminderForTest("payment-source-reminder", source.ID, now.Add(time.Minute), nil)
	history := []Message{source, reminder}

	if resolved, sourceIndex, kind, ok := reliableOutOfTurnActivePromptSourceV1(history, 1, reminder); ok {
		t.Fatalf("payment source with copied facts anchored availability: index=%d kind=%s source=%+v", sourceIndex, kind, resolved)
	}
	active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhasePaymentPending})
	if active.Kind == ActivePromptAvailabilityOptionChoice || active.Kind == ActivePromptAvailabilityDateChoice || active.HasAvailabilityList {
		t.Fatalf("payment source with copied facts opened availability continuity: %+v", active)
	}
}

func TestOutOfTurnAvailabilityAnchorRequiresGlobalUniqueCausalSourceV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	source := availabilityPromptAuthorityMessage("out-of-turn-unique-source", "DELIVERED", raw, []int{0})
	reminder := outOfTurnAvailabilityReminderForTest(
		"out-of-turn-unique-reminder",
		source.ID,
		source.ReceivedAt.Add(time.Minute),
		nil,
	)

	t.Run("duplicate after reminder is rejected even when undelivered", func(t *testing.T) {
		duplicate := source
		duplicate.Payload = cloneMap(source.Payload)
		duplicate.NormalizedPayload = cloneMap(source.NormalizedPayload)
		duplicate.ProcessingStatus = "SEND_FAILED"
		delete(duplicate.NormalizedPayload, "delivery_recorded_at")
		duplicate.ReceivedAt = reminder.ReceivedAt.Add(time.Minute)
		duplicate.CreatedAt = duplicate.ReceivedAt
		history := []Message{source, reminder, duplicate}
		if resolved, sourceIndex, kind, ok := reliableOutOfTurnActivePromptSourceV1(history, 1, reminder); ok {
			t.Fatalf("globally duplicated source anchored continuity: index=%d kind=%s source=%+v", sourceIndex, kind, resolved)
		}
	})

	t.Run("source timestamp after reminder is rejected", func(t *testing.T) {
		nonCausal := source
		nonCausal.Payload = cloneMap(source.Payload)
		nonCausal.NormalizedPayload = cloneMap(source.NormalizedPayload)
		nonCausal.ReceivedAt = reminder.ReceivedAt.Add(time.Minute)
		nonCausal.CreatedAt = nonCausal.ReceivedAt
		history := []Message{nonCausal, reminder}
		if resolved, sourceIndex, kind, ok := reliableOutOfTurnActivePromptSourceV1(history, 1, reminder); ok {
			t.Fatalf("temporally posterior source anchored continuity: index=%d kind=%s source=%+v", sourceIndex, kind, resolved)
		}
	})
}

func TestOutOfTurnAvailabilityConsumersUseAnchoredSourceFactsV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	source := availabilityPromptAuthorityMessage("out-of-turn-consumer-source", "DELIVERED", raw, []int{0})
	poisoned := raw
	poisoned.Filter.Origin = "Origem do lembrete"
	poisoned.Filter.Destination = "Destino do lembrete"
	poisoned.Results = append([]AvailabilitySearchItem(nil), raw.Results...)
	poisoned.Results[0].TripID = "trip-do-lembrete"
	reminder := outOfTurnAvailabilityReminderForTest(
		"out-of-turn-consumer-reminder",
		source.ID,
		source.ReceivedAt.Add(time.Minute),
		&poisoned,
	)
	history := []Message{source, reminder}

	active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
	if active.Kind != ActivePromptAvailabilityOptionChoice || active.SourceMessageID != source.ID || active.AvailabilityOptionCount != 1 {
		t.Fatalf("active prompt did not use anchored source: %+v", active)
	}
	promptContext, ok := activeAvailabilitySelectionPromptContext(active, history)
	if !ok || promptContext.OptionCount != 1 || !promptContext.HasCurrentFacts || promptContext.SourceMessageID != source.ID {
		t.Fatalf("selection prompt context did not use anchored source: ok=%t context=%+v", ok, promptContext)
	}
	current := currentAvailabilitySelectionPromptAvailabilityContextAt(history, source.ReceivedAt)
	if current == nil || len(current.Results) != 1 || current.Results[0].TripID != raw.Results[0].TripID {
		t.Fatalf("current availability did not use anchored source facts: %+v", current)
	}
	gate := classifyAvailabilitySelectionTurn(
		active,
		history,
		"1",
		IntentDecision{Intent: IntentSelectAvailabilityOption, SelectedOptionIndex: 1},
		false,
		AvailabilitySelectionStateV1{},
	)
	if gate.Class != availabilitySelectionGateMaterialize || gate.Availability == nil || len(gate.Availability.Results) != 1 || gate.Availability.Results[0].TripID != raw.Results[0].TripID {
		t.Fatalf("materialization did not use anchored source facts: %+v", gate)
	}
	decisionSource := availabilityContextFromOutOfTurnActivePromptSource(history, IntentDecision{TemplateData: map[string]interface{}{
		outOfTurnActivePromptSourceIDDataKey: source.ID,
	}})
	if decisionSource == nil || len(decisionSource.Results) != 1 || decisionSource.Results[0].TripID != raw.Results[0].TripID {
		t.Fatalf("out-of-turn materialization source used reminder facts: %+v", decisionSource)
	}

	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &AvailabilitySearchResult{
		Filter:  raw.Filter,
		Results: []AvailabilitySearchItem{raw.Results[0]},
	}, 1)
	state := ReduceAvailabilitySelectionEventsV1(AvailabilitySelectionStateV1{}, []AvailabilitySelectionEventV1{
		materializedAvailabilitySelectionEventForTest("out-of-turn-selection", "out-of-turn-selection-projection", source.ID, snapshot),
	})
	state.BootstrapCompleted = true
	session := sessionWithAvailabilitySelectionStateForTest(Session{
		ContactKey:    "out-of-turn-consumers",
		CustomerPhone: "5500000000000",
		CustomerName:  "Pessoa Teste",
	}, state)
	draft := collectBookingDraftContextWithPassengerState(session, history, "quero reservar", completePassengerStateForTest(1, 0))
	if !draft.HasBookableSelection || draft.TripID != raw.Results[0].TripID || draft.Origin == poisoned.Filter.Origin {
		t.Fatalf("booking draft used reminder facts: %+v", draft)
	}
	input, inputOK := parseBookingCreateInputWithPassengerState(
		session,
		history,
		"quero reservar\nPessoa Teste | CPF | 84960815086",
		nil,
		completePassengerStateForTest(1, 0),
	)
	if !inputOK || input.TripID != raw.Results[0].TripID || input.TripID == poisoned.Results[0].TripID {
		t.Fatalf("booking_create used reminder facts: ok=%t input=%+v", inputOK, input)
	}
}

func outOfTurnAvailabilityReminderForTest(
	messageID string,
	sourceID string,
	at time.Time,
	facts *AvailabilitySearchResult,
) Message {
	payload := map[string]interface{}{
		"template_data": map[string]interface{}{
			outOfTurnTemplateDataKey:             true,
			outOfTurnActivePromptTemplateDataKey: string(ActivePromptAvailabilityOptionChoice),
			outOfTurnActivePromptSourceIDDataKey: sourceID,
		},
	}
	if facts != nil {
		payload["tool_context"] = map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(*facts),
		}
	}
	normalized := cloneMap(payload)
	normalized["delivery_recorded_at"] = at.Format(time.RFC3339Nano)
	return Message{
		ID:                messageID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              paymentOptionsInfoReply,
		ProcessingStatus:  "SENT",
		ReceivedAt:        at,
		CreatedAt:         at,
		Payload:           payload,
		NormalizedPayload: normalized,
	}
}

func TestLatestReliableAssistantMessageStopsAtUnresolvedAvailabilityProjection(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	old := availabilityPromptAuthorityMessage("scan-old-prompt", "DELIVERED", raw, []int{0})
	now := old.ReceivedAt.Add(time.Minute)
	projection := Message{
		ID:               "scan-unresolved-bot-projection",
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             buildAvailabilityListReplyForResultIndexes(raw, []int{0}),
		ProcessingStatus: "SENT",
		ReceivedAt:       now,
		CreatedAt:        now,
		Payload: map[string]interface{}{
			"mode":             string(deliveredPromptProjectionBotAutoReplyV1),
			"draft_message_id": "missing-availability-draft",
		},
		NormalizedPayload: map[string]interface{}{
			"mode":                 string(deliveredPromptProjectionBotAutoReplyV1),
			"draft_message_id":     "missing-availability-draft",
			"delivery_recorded_at": now.Format(time.RFC3339Nano),
		},
	}
	history := []Message{old, projection}
	if !messageMayCarryAvailabilityPromptV1(projection) {
		t.Fatal("availability option body was not recognized as an availability candidate")
	}
	if _, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, 1, projection); ok {
		t.Fatal("projection with missing draft unexpectedly resolved")
	}
	if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, 1) {
		t.Fatal("delivered unresolved availability projection did not create a barrier")
	}
	if message, sourceIndex, ok := latestReliableAssistantMessageWithIndex(history); ok {
		t.Fatalf("scan published prompt across unresolved projection: index=%d message=%+v", sourceIndex, message)
	}
	active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
	if active.Kind != ActivePromptUnknown || active.SourceMessageID != "" || active.HasAvailabilityList {
		t.Fatalf("scan inferred availability prompt across barrier: %+v", active)
	}
	if availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil || sourceIndex != -1 {
		t.Fatalf("scan recovered older authority across barrier: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
	}

	t.Run("undelivered remains invisible", func(t *testing.T) {
		undelivered := projection
		undelivered.ProcessingStatus = "SEND_FAILED"
		undelivered.NormalizedPayload = cloneMap(projection.NormalizedPayload)
		delete(undelivered.NormalizedPayload, "delivery_recorded_at")
		undeliveredHistory := []Message{old, undelivered}
		if deliveredInvalidAvailabilityPromptBarrierAtV1(undeliveredHistory, 1) {
			t.Fatal("UNDELIVERED projection created a barrier")
		}
		message, sourceIndex, ok := latestReliableAssistantMessageWithIndex(undeliveredHistory)
		if !ok || sourceIndex != 0 || message.ID != old.ID {
			t.Fatalf("UNDELIVERED projection hid prior prompt: ok=%t index=%d message=%+v", ok, sourceIndex, message)
		}
	})

	t.Run("valid structural projection resolves before barrier", func(t *testing.T) {
		draft, delivered := availabilityPromptAuthorityProjectionFixtureV1(
			t,
			raw,
			string(deliveredPromptProjectionBotAutoReplyV1),
			"",
			"scan-valid-structural",
		)
		resolvedHistory := []Message{draft, delivered}
		message, sourceIndex, ok := latestReliableAssistantMessageWithIndex(resolvedHistory)
		if !ok || sourceIndex != 1 || message.ID != delivered.ID {
			t.Fatalf("valid projection did not resolve delivered identity: ok=%t index=%d message=%+v", ok, sourceIndex, message)
		}
		if deliveredInvalidAvailabilityPromptBarrierAtV1(resolvedHistory, 1) {
			t.Fatal("valid resolved projection created a barrier")
		}
	})
}

func TestInferActivePromptContextPendingBotAutoReplyWithoutDraftSourceDoesNotOpenPrompt(t *testing.T) {
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

	if got.Kind != ActivePromptUnknown || got.SourceMessageID != "" {
		t.Fatalf("isolated PENDING BOT_AUTO_REPLY must not open an active prompt, got %+v", got)
	}
	if promptContext.OptionCount != 0 || promptContext.HasCurrentFacts {
		t.Fatalf("isolated PENDING BOT_AUTO_REPLY must not expose availability context, got %+v", promptContext)
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
			NormalizedPayload: map[string]interface{}{
				"delivery_recorded_at": now.Format(time.RFC3339Nano),
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
			NormalizedPayload: map[string]interface{}{
				"delivery_recorded_at": now.Format(time.RFC3339Nano),
			},
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
