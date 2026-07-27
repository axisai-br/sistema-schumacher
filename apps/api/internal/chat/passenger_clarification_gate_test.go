package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func TestFreshSessionGreetingDoesNotOpenPassengerGate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: "Ola! Como posso ajudar com sua viagem?",
		Model:     "gpt-test",
	}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	out := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-greeting", "oi")
	if out.Draft == nil {
		t.Fatal("expected greeting draft")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != runner.result.ReplyText {
		t.Fatalf("fresh greeting must follow the normal fallback, got %q", got)
	}
	if runner.calls != 1 {
		t.Fatalf("fresh greeting must reach the normal runner exactly once, got %d", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("fresh greeting must not run tools, availability=%d tools=%+v", searcher.calls, out.ToolCalls)
	}
	assertFreshSessionPassengerGateInactive(t, store, out)
}

func TestFreshSessionReservationAsksRouteBeforePassengerGate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: "De qual cidade voce vai sair, para qual cidade quer ir e em qual data?",
		Model:     "gpt-test",
	}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	out := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-reservation", "quero uma passagem")
	if out.Draft == nil {
		t.Fatal("expected route clarification draft")
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("reservation start must not search before route data, availability=%d tools=%+v", searcher.calls, out.ToolCalls)
	}
	if runner.calls != 1 {
		t.Fatalf("reservation start must continue through the normal fallback once, runner=%d", runner.calls)
	}
	folded := foldChatText(out.Draft.Body)
	if !strings.Contains(folded, "de qual cidade") || !strings.Contains(folded, "para qual cidade") || !strings.Contains(folded, "qual data") {
		t.Fatalf("reservation start must ask for the missing route, got %q", out.Draft.Body)
	}
	assertFreshSessionPassengerGateInactive(t, store, out)
}

func TestAvailabilityRunsBeforePassengerGateInFreshSession(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  freshSessionPassengerGateAvailabilityResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	out := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-earliest", "moncao para videira na data mais proxima")
	if out.Draft == nil {
		t.Fatal("expected availability result draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("availability_search must execute exactly once before passenger collection, got %d", searcher.calls)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected exactly one availability_search tool call, got %+v", out.ToolCalls)
	}
	if got := readInt(out.Draft.NormalizedPayload["tool_call_count"]); got != 1 {
		t.Fatalf("expected tool_call_count=1, got %d payload=%+v", got, out.Draft.NormalizedPayload)
	}
	results := asInterfaceSliceMaps(out.ToolCalls[0].ResponsePayload["results"])
	if len(results) != 1 || strings.TrimSpace(asString(results[0]["trip_id"])) != searcher.result.Results[0].TripID {
		t.Fatalf("expected the availability result to be recorded before passenger collection, response=%+v", out.ToolCalls[0].ResponsePayload)
	}
	assertFreshSessionPassengerGateInactive(t, store, out)
}

func TestFreshSessionSelectionPersistsCompleteOptionThenAsksPassengers(t *testing.T) {
	store := newFakeStore()
	availability := freshSessionPassengerGateAvailabilityResult()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: buildAvailabilityListReply(availability), Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true, result: availability}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	first := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-selection-search", "moncao para videira na data mais proxima")
	if first.Draft == nil || searcher.calls != 1 || len(first.ToolCalls) != 1 {
		t.Fatalf("expected initial availability list, draft=%+v calls=%d tools=%+v", first.Draft, searcher.calls, first.ToolCalls)
	}
	assertFreshSessionPassengerGateInactive(t, store, first)
	runnerCallsAfterSearch := runner.calls
	markSessionMessagesAutomationSent(t, store, first.Session.ID)
	history, err := store.ListMessages(context.Background(), first.Session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list delivered availability prompt: %v", err)
	}
	canonicalState := deriveCanonicalConversationState(store.sessions[first.Session.ID], history, "")
	activePrompt := InferActivePromptContext(history, canonicalState)
	if promptContext, ok := activeAvailabilitySelectionPromptContext(activePrompt, history); !ok ||
		promptContext.OptionCount != len(availability.Results) || !promptContext.HasCurrentFacts {
		t.Fatalf("delivered availability list must be the active trusted option prompt, active=%+v context=%+v ok=%t history=%+v",
			activePrompt, promptContext, ok, history)
	}

	second := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-selection-choice", "1")
	if second.Draft == nil {
		t.Fatal("expected passenger question after valid selection")
	}
	if runner.calls != runnerCallsAfterSearch {
		t.Fatalf("valid selection must not add a runner call, before=%d after=%d", runnerCallsAfterSearch, runner.calls)
	}
	if searcher.calls != 1 || len(second.ToolCalls) != 0 {
		t.Fatalf("selection must reuse the visible list without another tool, availability=%d tools=%+v", searcher.calls, second.ToolCalls)
	}
	if got := strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("selection must ask passengers only after materialization, got template=%q payload=%+v", got, second.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(second.Draft.Body); got != askPassengerCountReply {
		t.Fatalf("expected passenger count question %q, got %q", askPassengerCountReply, got)
	}

	want := availability.Results[0]
	for _, payload := range []map[string]interface{}{second.Draft.Payload, second.Draft.NormalizedPayload} {
		if got := readInt(payload["selected_option_index"]); got != 1 {
			t.Fatalf("expected selected_option_index=1, got %d payload=%+v", got, payload)
		}
		snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
		for key, expected := range map[string]string{
			"trip_id":            want.TripID,
			"board_stop_id":      want.BoardStopID,
			"alight_stop_id":     want.AlightStopID,
			"origin":             want.OriginDisplayName,
			"destination":        want.DestinationDisplayName,
			"origin_depart_time": want.OriginDepartTime,
			"trip_date":          want.TripDate,
			"currency":           want.Currency,
		} {
			if got := strings.TrimSpace(asString(snapshot[key])); got != expected {
				t.Fatalf("expected complete selected option %s=%q, got %q snapshot=%+v", key, expected, got, snapshot)
			}
		}
		if got := readInt(snapshot["selected_option_index"]); got != 1 {
			t.Fatalf("expected snapshot selected_option_index=1, got %d snapshot=%+v", got, snapshot)
		}
		if got := asFloat64(snapshot["price"]); got != want.Price {
			t.Fatalf("expected selected price %.2f, got %.2f snapshot=%+v", want.Price, got, snapshot)
		}
	}
	if _, ok := second.Draft.NormalizedPayload[passengerPendingPromptEventV1MessageKey]; !ok {
		t.Fatalf("passenger question must carry the pending structural prompt event, payload=%+v", second.Draft.NormalizedPayload)
	}
	canonicalValue, ok := second.Memory["canonical_state"]
	if !ok {
		t.Fatal("expected canonical state after complete option materialization")
	}
	canonical := canonicalConversationStateFromTestValue(t, canonicalValue, "complete fresh selection canonical state")
	if canonical.Route.SelectedOptionIndex != 1 ||
		canonical.Route.TripID != want.TripID ||
		canonical.Route.BoardStopID != want.BoardStopID ||
		canonical.Route.AlightStopID != want.AlightStopID {
		t.Fatalf("complete option must persist the route atomically before passenger collection: %+v", canonical.Route)
	}
	persistedCanonicalValue := asMap(store.sessions[second.Session.ID].Metadata["agent"])["canonical_state"]
	persistedCanonical := canonicalConversationStateFromTestValue(t, persistedCanonicalValue, "persisted complete fresh selection canonical state")
	if persistedCanonical.Route.SelectedOptionIndex != 1 ||
		persistedCanonical.Route.TripID != want.TripID ||
		persistedCanonical.Route.BoardStopID != want.BoardStopID ||
		persistedCanonical.Route.AlightStopID != want.AlightStopID {
		t.Fatalf("complete option route was not persisted in the session: %+v", persistedCanonical.Route)
	}
	state, ok := passengerClarificationStateV1FromSession(store.sessions[second.Session.ID])
	if !ok {
		t.Fatal("expected durable passenger state after selection")
	}
	if state.HasEvidence || state.PassengerPromptMessageID != "" || state.ChildPromptMessageID != "" {
		t.Fatalf("draft-only passenger prompt must not open the gate before delivery, state=%+v", state)
	}
	reply, err := store.CreateAutomationReply(context.Background(), CreateAutomationReplyInput{
		SessionID: second.Session.ID, DraftMessageID: second.Draft.ID,
		IdempotencyKey: "fresh-selection-passenger-outbound",
	}, 0)
	if err != nil {
		t.Fatalf("create passenger outbound after materialized selection: %v", err)
	}
	markPassengerPromptDeliveredForTest(t, store, reply, "provider-fresh-selection-passenger")
	deliveredState, ok := passengerClarificationStateV1FromSession(store.sessions[second.Session.ID])
	if !ok || !deliveredState.HasEvidence ||
		deliveredState.PassengerPromptMessageID != reply.Message.ID ||
		len(deliveredState.AppliedEventIDs) == 0 {
		t.Fatalf("real passenger prompt must open its epoch only after delivery: %+v exists=%t", deliveredState, ok)
	}
}

func TestFreshSessionSingleOptionConfirmationMaterializesBeforePassengerPrompt(t *testing.T) {
	store := newFakeStore()
	availability := freshSessionPassengerGateAvailabilityResult()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: buildAvailabilityListReply(availability), Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true, result: availability}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	first := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-confirmation-search", "moncao para videira na data mais proxima")
	if first.Draft == nil || searcher.calls != 1 || len(first.ToolCalls) != 1 {
		t.Fatalf("expected initial single-option availability, draft=%+v calls=%d tools=%+v", first.Draft, searcher.calls, first.ToolCalls)
	}
	markSessionMessagesAutomationSent(t, store, first.Session.ID)
	runnerCallsAfterSearch := runner.calls

	second := ingestAndReprocessFreshPassengerGateTurn(t, svc, "fresh-confirmation-ok", "ok")
	if second.Draft == nil {
		t.Fatal("expected passenger question after single-option confirmation")
	}
	if got := strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("single-option confirmation must materialize before passenger collection, got template=%q payload=%+v", got, second.Draft.NormalizedPayload)
	}
	if runner.calls != runnerCallsAfterSearch || searcher.calls != 1 || len(second.ToolCalls) != 0 {
		t.Fatalf("single-option confirmation dispatched external work: runner_before=%d runner_after=%d availability=%d tools=%+v",
			runnerCallsAfterSearch, runner.calls, searcher.calls, second.ToolCalls)
	}
	want := availability.Results[0]
	for label, payload := range map[string]map[string]interface{}{
		"payload": second.Draft.Payload, "normalized_payload": second.Draft.NormalizedPayload,
	} {
		if index := payloadSelectedOptionIndex(payload); index != 1 {
			t.Fatalf("%s selected index=%d, want 1: %+v", label, index, payload)
		}
		snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
		if strings.TrimSpace(asString(snapshot["trip_id"])) != want.TripID ||
			strings.TrimSpace(asString(snapshot["board_stop_id"])) != want.BoardStopID ||
			strings.TrimSpace(asString(snapshot["alight_stop_id"])) != want.AlightStopID {
			t.Fatalf("%s did not persist the complete single option: %+v", label, snapshot)
		}
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(store.sessions[second.Session.ID].Metadata) {
		t.Fatalf("complete single-option confirmation left availability invalidated: %+v", store.sessions[second.Session.ID].Metadata)
	}
}

func TestPassengerPromptAvailabilityFactsDoNotReactivateSelectionGate(t *testing.T) {
	tests := []struct {
		name               string
		body               string
		wantPassengerCount bool
	}{
		{name: "numeric_passenger_count", body: "1", wantPassengerCount: true},
		{name: "passenger_confirmation", body: "sim"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

			now := availabilityTestObservedAt()
			contactKey := "passenger-prompt-availability-facts-" + test.name
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed passenger prompt identity session: %v", err)
			}
			availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
			seedAvailabilitySelectionPromptForFailClosedTest(
				t,
				store,
				session.ID,
				"passenger-prompt-identity-list-"+test.name,
				availability,
				messageStatusAutomationSent,
				true,
				now.Add(-2*time.Minute),
			)

			selected := ingestAndReprocessActivePromptFlowTurn(
				t,
				svc,
				contactKey,
				"passenger-prompt-identity-select-"+test.name,
				"2",
			)
			if selected.Draft == nil || strings.TrimSpace(asString(selected.Draft.NormalizedPayload["template_name"])) != string(TemplateAskPassengerCount) {
				t.Fatalf("expected option 2 to materialize before passenger collection, draft=%+v", selected.Draft)
			}
			if got := payloadSelectedOptionIndex(selected.Draft.NormalizedPayload); got != 2 {
				t.Fatalf("expected materialized option 2, got %d payload=%+v", got, selected.Draft.NormalizedPayload)
			}
			facts := asMap(asMap(selected.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
			if len(asInterfaceSliceMaps(facts["results"])) != len(availability.Results) {
				t.Fatalf("passenger prompt must carry continuity facts for both options: %+v", selected.Draft.Payload)
			}

			outbound, err := store.CreateAutomationReply(context.Background(), CreateAutomationReplyInput{
				SessionID: selected.Session.ID, DraftMessageID: selected.Draft.ID,
				IdempotencyKey: "passenger-prompt-identity-outbound-" + test.name,
			}, 0)
			if err != nil {
				t.Fatalf("create delivered passenger prompt: %v", err)
			}
			markPassengerPromptDeliveredForTest(t, store, outbound, "provider-passenger-prompt-identity-"+test.name)

			normalizedPayload := map[string]interface{}{}
			if test.wantPassengerCount {
				normalizedPayload[passengerClarificationEventsV1MessageKey] = []PassengerClarificationEventV1{{
					Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
					PromptMessageID: outbound.Message.ID,
					Value:           1, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
				}}
			}
			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: contactKey,
				Message: IngestMessagePayload{
					Direction: "INBOUND", ProviderMessageID: "passenger-prompt-identity-answer-" + test.name,
					IdempotencyKey: "passenger-prompt-identity-answer-" + test.name, Body: test.body,
					NormalizedPayload: normalizedPayload,
				},
			})
			if err != nil {
				t.Fatalf("ingest passenger prompt identity answer %q: %v", test.body, err)
			}
			if test.wantPassengerCount && len(passengerClarificationEventsV1FromMessage(ingested.Message)) == 0 {
				t.Fatalf("numeric passenger answer lost its structured PassengerCount event: message=%+v", ingested.Message)
			}
			answered, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess passenger prompt identity answer %q: %v", test.body, err)
			}
			if answered.Draft == nil {
				t.Fatal("expected deterministic passenger continuation")
			}
			if got := strings.TrimSpace(asString(answered.Draft.NormalizedPayload["template_name"])); got == string(TemplateContextFallbackAvailabilityOption) {
				t.Fatalf("passenger answer %q was misclassified as availability clarification: payload=%+v body=%q", test.body, answered.Draft.NormalizedPayload, answered.Draft.Body)
			}
			for label, payload := range map[string]map[string]interface{}{
				"payload": answered.Draft.Payload, "normalized_payload": answered.Draft.NormalizedPayload,
			} {
				if got := payloadSelectedOptionIndex(payload); got > 0 && got != 2 {
					t.Fatalf("%s passenger answer rematerialized option %d instead of preserving option 2: %+v", label, got, payload)
				}
			}

			persisted := store.sessions[session.ID]
			canonical := canonicalConversationStateFromTestValue(
				t,
				asMap(persisted.Metadata["agent"])["canonical_state"],
				"passenger prompt identity canonical state",
			)
			want := availability.Results[1]
			if canonical.Route.SelectedOptionIndex != 2 || canonical.Route.TripID != want.TripID ||
				canonical.Route.BoardStopID != want.BoardStopID || canonical.Route.AlightStopID != want.AlightStopID {
				t.Fatalf("passenger answer %q replaced the selected trip: got=%+v want_option=2/%+v", test.body, canonical.Route, want)
			}
			if test.wantPassengerCount {
				state, ok := passengerClarificationStateV1FromSession(persisted)
				if !ok || !state.PassengerCountKnown || state.PassengerCount != 1 {
					t.Fatalf("numeric passenger answer must record PassengerCount=1: state=%+v exists=%t", state, ok)
				}
			}
			if runner.calls != 0 || searcher.calls != 0 || len(answered.ToolCalls) != 0 {
				t.Fatalf("passenger prompt identity path dispatched external work: runner=%d availability=%d tools=%+v", runner.calls, searcher.calls, answered.ToolCalls)
			}
		})
	}
}

func TestFreshSessionIncompleteAvailabilityRequiresMaterializationBeforePassenger(t *testing.T) {
	missingFields := []struct {
		name   string
		mutate func(*AvailabilitySearchItem)
	}{
		{
			name: "missing_trip_id",
			mutate: func(item *AvailabilitySearchItem) {
				item.TripID = ""
			},
		},
		{
			name: "missing_board_stop_id",
			mutate: func(item *AvailabilitySearchItem) {
				item.BoardStopID = ""
			},
		},
		{
			name: "missing_alight_stop_id",
			mutate: func(item *AvailabilitySearchItem) {
				item.AlightStopID = ""
			},
		},
	}

	for _, missing := range missingFields {
		for _, selection := range []string{"1", "essa msm", "ok"} {
			t.Run(missing.name+"/"+strings.ReplaceAll(selection, " ", "_"), func(t *testing.T) {
				base := newFakeStore()
				store := newFakeTravelQueryV2ShadowClaimStore()
				store.fakeStore = base
				runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
				jsonRunner := &fakeJSONDecisionRunner{enabled: true}
				openAI := &fakeOpenAIInterpreter{enabled: true}
				travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
				searcher := &fakeAvailabilitySearcher{enabled: true}
				booking := &fakeBookingCreator{enabled: true}
				payment := &fakePaymentCreator{enabled: true}
				paymentStatus := &fakePaymentStatusSearcher{enabled: true}
				svc := NewService(store, config.Config{
					ChatDebounceWindowMS:               1500,
					ChatOpenAIInterpreterShadowEnabled: true,
					ChatOpenAITravelV2ShadowEnabled:    true,
					ChatAgentMode:                      chatAgentModeHybridJSON,
				}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentStatus)

				now := time.Now().UTC()
				contactKey := "fresh-incomplete-" + missing.name + "-" + strings.ReplaceAll(selection, " ", "-")
				session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
					Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
					LastMessageAt: &now, LastOutboundAt: &now,
				})
				if err != nil {
					t.Fatalf("seed fresh incomplete session: %v", err)
				}
				availability := freshSessionPassengerGateAvailabilityResult()
				availability.Results = append([]AvailabilitySearchItem(nil), availability.Results...)
				missing.mutate(&availability.Results[0])
				if _, err := base.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
					SessionID: session.ID, IdempotencyKey: "fresh-incomplete-prompt-" + missing.name + "-" + selection,
					Body: buildAvailabilityListReply(availability), SenderName: "SHABAS",
					ProcessingStatus: messageStatusAutomationSent,
					Payload: map[string]interface{}{
						"tool_context": map[string]interface{}{
							toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
						},
					},
					RecordedAt: now.Add(-time.Minute),
				}); err != nil {
					t.Fatalf("seed visible incomplete availability: %v", err)
				}

				out := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "fresh-incomplete-selection-"+missing.name+"-"+selection, selection)
				if out.Draft == nil {
					t.Fatal("expected deterministic safe selection response")
				}
				if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateContextFallbackAvailabilityOption) {
					t.Fatalf("incomplete option must remain at safe trip selection, got template=%q draft=%+v", got, out.Draft)
				}
				if strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])) == string(TemplateAskPassengerCount) ||
					strings.Contains(foldChatText(out.Draft.Body), "passagem e so para voce") {
					t.Fatalf("incomplete option opened passenger collection: %+v", out.Draft)
				}
				for label, payload := range map[string]map[string]interface{}{
					"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
				} {
					if index := payloadSelectedOptionIndex(payload); index != 0 {
						t.Fatalf("%s leaked selected_option_index=%d: %+v", label, index, payload)
					}
					if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
						t.Fatalf("%s leaked selected availability snapshot: %+v", label, snapshot)
					}
					for _, key := range []string{passengerPendingPromptEventV1MessageKey, passengerPromptEventV1MessageKey} {
						if _, exists := payload[key]; exists {
							t.Fatalf("%s opened passenger epoch through %s: %+v", label, key, payload)
						}
					}
					assertNoRouteIDKeysAtAnyDepthForTest(t, payload, "incomplete selection "+label)
				}

				canonicalValue, ok := out.Memory["canonical_state"]
				if !ok {
					t.Fatal("expected canonical state in incomplete selection memory")
				}
				canonical := canonicalConversationStateFromTestValue(t, canonicalValue, "incomplete selection canonical state")
				assertCanonicalAvailabilityInvalidatedForTest(t, canonical, "incomplete selection canonical state")

				persisted, err := base.GetSession(context.Background(), session.ID)
				if err != nil {
					t.Fatalf("reload incomplete selection session: %v", err)
				}
				passengerState, ok := passengerClarificationStateV1FromSession(persisted)
				if !ok {
					t.Fatal("expected literal fresh passenger state to remain persisted")
				}
				if passengerState.HasEvidence || passengerState.PassengerCountKnown || passengerState.ChildUnder5CountKnown ||
					passengerState.PassengerSlotStatus != PassengerClarificationSlotPending ||
					passengerState.ChildSlotStatus != PassengerClarificationSlotPending ||
					passengerState.PassengerPromptMessageID != "" || passengerState.ChildPromptMessageID != "" ||
					passengerState.PassengerLastMessageID != "" || passengerState.ChildLastMessageID != "" ||
					len(passengerState.AppliedEventIDs) != 0 || len(passengerState.AppliedMessageIDs) != 0 {
					t.Fatalf("incomplete selection must not open a passenger epoch: %+v", passengerState)
				}
				history, err := base.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
				if err != nil {
					t.Fatalf("list incomplete selection history: %v", err)
				}
				boundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(persisted.Metadata)
				bookingDraft := collectBookingDraftContextWithPassengerState(
					persisted,
					availabilityInferenceHistory(history, boundary),
					"",
					passengerState,
				)
				if bookingDraft.SelectedOptionIndex != 0 || bookingDraft.TripID != "" ||
					bookingDraft.BoardStopID != "" || bookingDraft.AlightStopID != "" {
					t.Fatalf("incomplete selection leaked into booking draft: %+v", bookingDraft)
				}

				if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
					searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentStatus.calls != 0 || len(out.ToolCalls) != 0 {
					t.Fatalf("incomplete selection dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
						runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentStatus.calls, out.ToolCalls)
				}
				select {
				case <-store.claimAttempts:
					t.Fatal("incomplete selection scheduled a Travel V2 shadow claim")
				case <-time.After(25 * time.Millisecond):
				}
			})
		}
	}
}

func TestFallbackRoutedSelectionRequiresMaterialization(t *testing.T) {
	tests := []struct {
		name             string
		selection        string
		withFacts        bool
		wantInvalidation bool
	}{
		{name: "rendered_without_tool_facts_numeric", selection: "1", wantInvalidation: true},
		{name: "rendered_without_tool_facts_contextual", selection: "essa msm", wantInvalidation: true},
		{name: "rendered_without_tool_facts_confirmation", selection: "ok", wantInvalidation: true},
		{name: "index_out_of_range", selection: "2", withFacts: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := newFakeStore()
			store := newFakeTravelQueryV2ShadowClaimStore()
			store.fakeStore = base
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			jsonRunner := &fakeJSONDecisionRunner{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			payment := &fakePaymentCreator{enabled: true}
			paymentStatus := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAITravelV2ShadowEnabled:    true,
				ChatAgentMode:                      chatAgentModeHybridJSON,
			}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentStatus)

			now := time.Now().UTC()
			contactKey := "fallback-selection-" + test.name
			session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed fallback selection session: %v", err)
			}
			availability := freshSessionPassengerGateAvailabilityResult()
			seedAvailabilitySelectionPromptForFailClosedTest(
				t,
				base,
				session.ID,
				"fallback-selection-"+test.name,
				availability,
				messageStatusAutomationSent,
				test.withFacts,
				now.Add(-time.Minute),
			)

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: contactKey,
				Message: IngestMessagePayload{
					Direction: "INBOUND", ProviderMessageID: "fallback-selection-" + test.name,
					IdempotencyKey: "fallback-selection-" + test.name, Body: test.selection,
				},
			})
			if err != nil {
				t.Fatalf("ingest fallback selection: %v", err)
			}
			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess fallback selection: %v", err)
			}
			if out.Draft == nil {
				t.Fatal("expected deterministic availability fallback")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateContextFallbackAvailabilityOption) {
				t.Fatalf("fallback-routed selection must remain in trip selection, got template=%q draft=%+v", got, out.Draft)
			}
			for label, payload := range map[string]map[string]interface{}{
				"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
			} {
				if index := payloadSelectedOptionIndex(payload); index != 0 {
					t.Fatalf("%s leaked selected_option_index=%d: %+v", label, index, payload)
				}
				if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
					t.Fatalf("%s leaked selected availability snapshot: %+v", label, snapshot)
				}
				if _, exists := payload[passengerPendingPromptEventV1MessageKey]; exists {
					t.Fatalf("%s opened a passenger prompt event: %+v", label, payload)
				}
			}

			persisted, err := base.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("reload fallback selection session: %v", err)
			}
			boundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(persisted.Metadata)
			if test.wantInvalidation {
				if !canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) ||
					boundary.AfterMessageID != ingested.Message.ID ||
					!boundary.AfterCreatedAt.Equal(canonicalAvailabilityHistoryMessageTime(ingested.Message)) {
					t.Fatalf("fail-closed selection did not persist marker + boundary: marker=%t boundary=%+v inbound=%+v",
						canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata), boundary, ingested.Message)
				}
			} else {
				if canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) || boundary.known() {
					t.Fatalf("safe clarification invalidated complete availability: marker=%t boundary=%+v metadata=%+v",
						canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata), boundary, persisted.Metadata)
				}
				facts := asMap(asMap(out.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
				if len(asInterfaceSliceMaps(facts["results"])) != len(availability.Results) {
					t.Fatalf("safe clarification did not preserve current availability facts: %+v", out.Draft.Payload)
				}
			}
			if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
				searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentStatus.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("fallback-routed selection dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
					runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentStatus.calls, out.ToolCalls)
			}
			select {
			case <-store.claimAttempts:
				t.Fatal("fallback-routed selection scheduled a Travel V2 shadow claim")
			case <-time.After(25 * time.Millisecond):
			}
		})
	}
}

func TestAmbiguousAvailabilityClarificationPreservesCompleteOptionsForLaterIndex(t *testing.T) {
	base := newFakeStore()
	store := newFakeTravelQueryV2ShadowClaimStore()
	store.fakeStore = base
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	paymentStatus := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
		ChatAgentMode:                      chatAgentModeHybridJSON,
	}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentStatus)

	now := availabilityTestObservedAt()
	contactKey := "ambiguous-selection-preserves-options"
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed ambiguous selection session: %v", err)
	}
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, base, session.ID, "ambiguous-complete-options", availability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)

	first := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "ambiguous-selection-first", "essa msm")
	if first.Draft == nil || strings.TrimSpace(asString(first.Draft.NormalizedPayload["template_name"])) != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("ambiguous deictic reply must request an index, draft=%+v", first.Draft)
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(base.sessions[session.ID].Metadata) {
		t.Fatalf("ambiguous deictic reply invalidated a complete list: %+v", base.sessions[session.ID].Metadata)
	}
	for label, payload := range map[string]map[string]interface{}{
		"payload": first.Draft.Payload, "normalized_payload": first.Draft.NormalizedPayload,
	} {
		if payloadSelectedOptionIndex(payload) != 0 || len(asMap(payload[selectedAvailabilityResultPayloadKey])) != 0 {
			t.Fatalf("%s ambiguous clarification persisted a selection: %+v", label, payload)
		}
		if _, exists := payload[passengerPendingPromptEventV1MessageKey]; exists {
			t.Fatalf("%s ambiguous clarification opened passenger collection: %+v", label, payload)
		}
		if label == "payload" {
			facts := asMap(asMap(payload["tool_context"])[toolNameAvailabilitySearch])
			results := asInterfaceSliceMaps(facts["results"])
			if len(results) != len(availability.Results) ||
				strings.TrimSpace(asString(results[0]["trip_id"])) != availability.Results[0].TripID ||
				strings.TrimSpace(asString(results[1]["trip_id"])) != availability.Results[1].TripID {
				t.Fatalf("%s ambiguous clarification did not preserve both complete options: %+v", label, payload)
			}
		}
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentStatus.calls != 0 || len(first.ToolCalls) != 0 {
		t.Fatalf("ambiguous clarification dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentStatus.calls, first.ToolCalls)
	}
	select {
	case <-store.claimAttempts:
		t.Fatal("ambiguous clarification scheduled a Travel V2 shadow claim")
	case <-time.After(25 * time.Millisecond):
	}
	if _, err := base.UpdateMessage(context.Background(), UpdateMessageInput{
		MessageID: first.Draft.ID, ProcessingStatus: messageStatusAutomationSent,
	}); err != nil {
		t.Fatalf("mark clarification visible: %v", err)
	}

	secondService := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{enabled: true})
	second := ingestAndReprocessActivePromptFlowTurn(t, secondService, contactKey, "ambiguous-selection-second", "1")
	if second.Draft == nil || strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])) != string(TemplateAskPassengerCount) {
		t.Fatalf("index after clarification must select normally, draft=%+v", second.Draft)
	}
	if index := payloadSelectedOptionIndex(second.Draft.NormalizedPayload); index != 1 {
		t.Fatalf("later explicit index=%d, want 1: %+v", index, second.Draft.NormalizedPayload)
	}
	snapshot := asMap(second.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
	if strings.TrimSpace(asString(snapshot["trip_id"])) != availability.Results[0].TripID ||
		strings.TrimSpace(asString(snapshot["board_stop_id"])) != availability.Results[0].BoardStopID ||
		strings.TrimSpace(asString(snapshot["alight_stop_id"])) != availability.Results[0].AlightStopID {
		t.Fatalf("later explicit index selected the wrong option: %+v", snapshot)
	}
}

func TestOutOfRangeAvailabilityClarificationDoesNotContaminateLaterIndex(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	now := availabilityTestObservedAt()
	contactKey := "out-of-range-preserves-options"
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed out-of-range session: %v", err)
	}
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "out-of-range-complete-options", availability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)

	first := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "out-of-range-first", "3")
	if first.Draft == nil || strings.TrimSpace(asString(first.Draft.NormalizedPayload["template_name"])) != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("out-of-range index must request a valid number, draft=%+v", first.Draft)
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(store.sessions[session.ID].Metadata) ||
		payloadSelectedOptionIndex(first.Draft.NormalizedPayload) != 0 ||
		len(asMap(first.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])) != 0 {
		t.Fatalf("out-of-range clarification contaminated selection authority: session=%+v draft=%+v", store.sessions[session.ID], first.Draft)
	}
	if runner.calls != 0 || len(first.ToolCalls) != 0 {
		t.Fatalf("out-of-range clarification dispatched external work: runner=%d tools=%+v", runner.calls, first.ToolCalls)
	}
	if _, err := store.UpdateMessage(context.Background(), UpdateMessageInput{
		MessageID: first.Draft.ID, ProcessingStatus: messageStatusAutomationSent,
	}); err != nil {
		t.Fatalf("mark out-of-range clarification visible: %v", err)
	}

	second := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "out-of-range-second", "1")
	if second.Draft == nil || strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])) != string(TemplateAskPassengerCount) {
		t.Fatalf("valid index after out-of-range clarification must select normally, draft=%+v", second.Draft)
	}
	if index := payloadSelectedOptionIndex(second.Draft.NormalizedPayload); index != 1 {
		t.Fatalf("later valid index=%d, want 1: %+v", index, second.Draft.NormalizedPayload)
	}
	snapshot := asMap(second.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
	if strings.TrimSpace(asString(snapshot["trip_id"])) != availability.Results[0].TripID ||
		strings.TrimSpace(asString(snapshot["board_stop_id"])) != availability.Results[0].BoardStopID ||
		strings.TrimSpace(asString(snapshot["alight_stop_id"])) != availability.Results[0].AlightStopID {
		t.Fatalf("later valid index selected the wrong option: %+v", snapshot)
	}
}

func TestFallbackSelectionMaterializationInvalidationBlocksRawIndexOnLaterTurn(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC()
	contactKey := "fallback-selection-later-turn"
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed later-turn session: %v", err)
	}
	availability := freshSessionPassengerGateAvailabilityResult()
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "later-turn-no-facts", availability,
		messageStatusAutomationSent, false, now.Add(-time.Minute),
	)
	firstService := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "must not run", Model: "gpt-test"},
	})
	first := ingestAndReprocessActivePromptFlowTurn(t, firstService, contactKey, "fallback-selection-later-turn-first", "1")
	if first.Draft == nil {
		t.Fatal("expected first availability fallback draft")
	}
	if _, err := store.UpdateMessage(context.Background(), UpdateMessageInput{
		MessageID: first.Draft.ID, ProcessingStatus: messageStatusAutomationSent,
	}); err != nil {
		t.Fatalf("mark first fallback visible: %v", err)
	}

	secondRunner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	secondService := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, secondRunner)
	second := ingestAndReprocessActivePromptFlowTurn(t, secondService, contactKey, "fallback-selection-later-turn-second", "1")
	if second.Draft == nil || strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])) != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("repeated raw selection must remain fail-closed in trip selection: %+v", second.Draft)
	}
	if secondRunner.calls != 0 || len(second.ToolCalls) != 0 {
		t.Fatalf("repeated raw selection dispatched external work: runner=%d tools=%+v", secondRunner.calls, second.ToolCalls)
	}
	persisted, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload later-turn session: %v", err)
	}
	if !canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) {
		t.Fatalf("later turn lost the availability invalidation marker: %+v", persisted.Metadata)
	}
	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list later-turn history: %v", err)
	}
	boundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(persisted.Metadata)
	projection := collectBookingDraftContextWithPassengerState(
		persisted,
		availabilityInferenceHistory(history, boundary),
		"1",
		passengerClarificationStateV1OrDefault(persisted),
	)
	if projection.SelectedOptionIndex != 0 || projection.TripID != "" ||
		projection.BoardStopID != "" || projection.AlightStopID != "" {
		t.Fatalf("raw numeric reply reappeared on a later turn: %+v second=%+v", projection, second)
	}
}

func TestFallbackSelectionMaterializationIgnoresNumberOutsideAvailabilityContext(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta normal", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	out := ingestAndReprocessFreshPassengerGateTurn(t, svc, "number-outside-availability", "1")
	if runner.calls != 1 {
		t.Fatalf("number outside availability context must follow normal routing, runner=%d draft=%+v", runner.calls, out.Draft)
	}
	persisted, err := store.GetSession(context.Background(), out.Session.ID)
	if err != nil {
		t.Fatalf("reload number-outside session: %v", err)
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) {
		t.Fatalf("number outside availability context opened the materialization gate: %+v", persisted.Metadata)
	}
}

func TestAvailabilityConfirmationOutsidePromptDoesNotOpenSelectionGate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resposta normal", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	out := ingestAndReprocessFreshPassengerGateTurn(t, svc, "confirmation-outside-availability", "ok")
	if out.Draft == nil {
		t.Fatal("expected normal response outside availability context")
	}
	persisted, err := store.GetSession(context.Background(), out.Session.ID)
	if err != nil {
		t.Fatalf("reload confirmation-outside session: %v", err)
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) {
		t.Fatalf("confirmation outside availability context opened the selection gate: %+v", persisted.Metadata)
	}
	for label, payload := range map[string]map[string]interface{}{
		"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
	} {
		if payloadSelectedOptionIndex(payload) != 0 || len(asMap(payload[selectedAvailabilityResultPayloadKey])) != 0 {
			t.Fatalf("%s confirmation outside availability persisted a selection: %+v", label, payload)
		}
		if _, exists := payload[passengerPendingPromptEventV1MessageKey]; exists {
			t.Fatalf("%s confirmation outside availability opened passenger collection: %+v", label, payload)
		}
	}
}

func TestIncompleteSelectionPassengerUnsafeUsesAvailabilityFallback(t *testing.T) {
	base := newFakeStore()
	store := newFakeTravelQueryV2ShadowClaimStore()
	store.fakeStore = base
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	paymentStatus := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
		ChatAgentMode:                      chatAgentModeHybridJSON,
	}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentStatus)

	now := time.Now().UTC()
	contactKey := "passenger-unsafe-incomplete-selection"
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed passenger-unsafe session: %v", err)
	}
	before := seedDeliveredPassengerPromptForTest(t, base, session.ID, now.Add(-2*time.Minute))
	availability := freshSessionPassengerGateAvailabilityResult()
	availability.Results = append([]AvailabilitySearchItem(nil), availability.Results...)
	availability.Results[0].BoardStopID = ""
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, base, session.ID, "passenger-unsafe-incomplete", availability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)

	out := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "passenger-unsafe-incomplete-selection", "1")
	if out.Draft == nil {
		t.Fatal("expected safe availability fallback")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("passengerUnsafe + incomplete option must remain in trip selection, got template=%q draft=%+v", got, out.Draft)
	}
	for label, payload := range map[string]map[string]interface{}{
		"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
	} {
		for _, key := range []string{passengerPendingPromptEventV1MessageKey, passengerPromptEventV1MessageKey} {
			if _, exists := payload[key]; exists {
				t.Fatalf("%s created passenger prompt event %s: %+v", label, key, payload)
			}
		}
		if index := payloadSelectedOptionIndex(payload); index != 0 {
			t.Fatalf("%s leaked selected_option_index=%d: %+v", label, index, payload)
		}
		if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
			t.Fatalf("%s leaked selected availability snapshot: %+v", label, snapshot)
		}
	}
	persisted, err := base.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload passenger-unsafe incomplete session: %v", err)
	}
	after, ok := passengerClarificationStateV1FromSession(persisted)
	if !ok || !after.HasEvidence || after.PassengerPromptMessageID != before.PassengerPromptMessageID ||
		len(after.AppliedEventIDs) != len(before.AppliedEventIDs) {
		t.Fatalf("incomplete selection altered the passenger prompt epoch: before=%+v after=%+v exists=%t", before, after, ok)
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentStatus.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("passengerUnsafe incomplete selection dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentStatus.calls, out.ToolCalls)
	}
}

func TestPassengerGateSeparatesFreshUnknownFromActiveContext(t *testing.T) {
	fresh := newPassengerClarificationStateV1()
	fresh.BootstrapCompleted = true
	active := ReducePassengerClarificationEventsV1(fresh, []PassengerClarificationEventV1{{
		Type:      PassengerClarificationEventPassengerPromptOpened,
		Slot:      PassengerClarificationSlotPassenger,
		MessageID: "delivered-passenger-prompt",
	}})
	active.BootstrapCompleted = true
	conflicting := fresh
	conflicting.PassengerSlotStatus = PassengerClarificationSlotConflicting
	conflicting.ChildSlotStatus = PassengerClarificationSlotConflicting
	invalid := fresh
	invalid.Authority = PassengerClarificationAuthorityV1("CORRUPT")

	tests := []struct {
		name       string
		state      PassengerClarificationStateV1
		wantUnsafe bool
	}{
		{name: "fresh unknown slots outside passenger context", state: fresh, wantUnsafe: false},
		{name: "unknown slots after delivered passenger prompt", state: active, wantUnsafe: true},
		{name: "conflicting state without active context", state: conflicting, wantUnsafe: true},
		{name: "invalid state without active context", state: invalid, wantUnsafe: true},
		{name: "complete state", state: completePassengerStateForTest(1, 0), wantUnsafe: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := passengerClarificationStateUnsafeV1(test.state); got != test.wantUnsafe {
				t.Fatalf("passenger gate unsafe=%t, want %t for state %+v", got, test.wantUnsafe, test.state)
			}
		})
	}
}

func TestPassengerGateInvalidOrConflictingStateAlwaysFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *fakeStore, string)
	}{
		{
			name: "corrupt",
			seed: func(_ *testing.T, store *fakeStore, sessionID string) {
				session := store.sessions[sessionID]
				metadata := cloneMap(session.Metadata)
				memory := cloneMap(asMap(metadata["memory"]))
				memory[passengerClarificationStateV1MemoryKey] = map[string]interface{}{
					"version":   passengerClarificationStateV1Version + 1,
					"authority": "CORRUPT",
				}
				metadata["memory"] = memory
				session.Metadata = metadata
				store.sessions[sessionID] = session
			},
		},
		{
			name: "conflicting without active passenger context",
			seed: func(_ *testing.T, store *fakeStore, sessionID string) {
				state := newPassengerClarificationStateV1()
				state.BootstrapCompleted = true
				state.PassengerSlotStatus = PassengerClarificationSlotConflicting
				state.ChildSlotStatus = PassengerClarificationSlotConflicting
				seedPassengerClarificationStateV1ForTest(store, sessionID, state)
			},
		},
		{
			name: "conflicting after delivered passenger prompt",
			seed: func(t *testing.T, store *fakeStore, sessionID string) {
				fresh := newPassengerClarificationStateV1()
				fresh.BootstrapCompleted = true
				seedPassengerClarificationStateV1ForTest(store, sessionID, fresh)
				state := seedDeliveredPassengerPromptForTest(t, store, sessionID, time.Now().UTC().Add(-time.Minute))
				state.PassengerSlotStatus = PassengerClarificationSlotConflicting
				state.ChildSlotStatus = PassengerClarificationSlotConflicting
				seedPassengerClarificationStateV1ForTest(store, sessionID, state)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeTravelQueryV2ShadowClaimStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run"}}
			jsonRunner := &fakeJSONDecisionRunner{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
			availability := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			payment := &fakePaymentCreator{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAITravelV2ShadowEnabled:    true,
				ChatAgentMode:                      chatAgentModeHybridJSON,
			}, runner, jsonRunner, openAI, travel, availability, booking, payment)

			contactKey := "invalid-passenger-gate-" + test.name
			session, err := store.fakeStore.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:    "WHATSAPP",
				ContactKey: contactKey,
			})
			if err != nil {
				t.Fatalf("create invalid state session: %v", err)
			}
			test.seed(t, store.fakeStore, session.ID)
			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: contactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "invalid-passenger-gate-message-" + test.name,
					IdempotencyKey:    "invalid-passenger-gate-message-" + test.name,
					Body:              "quero continuar",
				},
			}); err != nil {
				t.Fatalf("ingest invalid state turn: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess invalid state turn: %v", err)
			}
			if out.Draft == nil || strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])) != string(TemplateAskPassengerCount) {
				t.Fatalf("invalid/conflicting state must fail closed with deterministic clarification, got %+v", out.Draft)
			}
			if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
				availability.calls != 0 || booking.calls != 0 || payment.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("invalid/conflicting state dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d tools=%+v",
					runner.calls, jsonRunner.calls, openAI.calls, travel.calls, availability.calls, booking.calls, payment.calls, out.ToolCalls)
			}
			select {
			case <-store.claimAttempts:
				t.Fatal("invalid/conflicting state scheduled a Travel V2 shadow claim")
			case <-time.After(25 * time.Millisecond):
			}
		})
	}
}

func TestPassengerHasEvidenceFalseContradictoryStateFailsClosedBeforeExternalWork(t *testing.T) {
	base := newFakeStore()
	store := newFakeTravelQueryV2ShadowClaimStore()
	store.fakeStore = base
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	availability := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	paymentStatus := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
		ChatAgentMode:                      chatAgentModeHybridJSON,
	}, runner, jsonRunner, openAI, travel, availability, booking, payment, paymentStatus)

	now := time.Now().UTC()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "has-evidence-false-contradiction", CustomerPhone: "has-evidence-false-contradiction",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed contradictory passenger session: %v", err)
	}
	prompt, err := base.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("seed delivered passenger prompt evidence: %v", err)
	}

	// Persist the contradictory snapshot directly. Do not pass it through a
	// reducer/helper that would normalize HasEvidence to true.
	session = base.sessions[session.ID]
	metadata := cloneMap(session.Metadata)
	memory := cloneMap(asMap(metadata["memory"]))
	memory[passengerClarificationStateV1MemoryKey] = PassengerClarificationStateV1{
		Version:                  passengerClarificationStateV1Version,
		Authority:                PassengerClarificationAuthorityPreBooking,
		PassengerCountProvenance: PassengerCountProvenanceUnknown,
		PassengerSlotStatus:      PassengerClarificationSlotOpen,
		PassengerPromptMessageID: prompt.ID,
		ChildSlotStatus:          PassengerClarificationSlotPending,
		BootstrapCompleted:       true,
		HasEvidence:              false,
	}
	metadata["memory"] = memory
	session.Metadata = metadata
	base.sessions[session.ID] = session

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction: "INBOUND", ProviderMessageID: "has-evidence-false-turn",
			IdempotencyKey: "has-evidence-false-turn", Body: "quero continuar",
		},
	}); err != nil {
		t.Fatalf("ingest contradictory passenger turn: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess contradictory passenger state: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("contradictory passenger state must return deterministic safe clarification: %+v", out.Draft)
	}
	templateName := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"]))
	if templateName != string(TemplateAskPassengerCount) && templateName != string(TemplateContextFallbackPassengerCount) {
		t.Fatalf("contradictory passenger state must use a deterministic passenger clarification, got %q draft=%+v", templateName, out.Draft)
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		availability.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentStatus.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("contradictory passenger state dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, availability.calls, booking.calls, payment.calls, paymentStatus.calls, out.ToolCalls)
	}
	select {
	case <-store.claimAttempts:
		t.Fatal("contradictory passenger state scheduled a Travel V2 shadow claim")
	case <-time.After(25 * time.Millisecond):
	}
	persistedState, ok := passengerClarificationStateV1FromSession(base.sessions[session.ID])
	if !ok || !persistedState.HasEvidence ||
		persistedState.PassengerSlotStatus != PassengerClarificationSlotConflicting ||
		persistedState.ChildSlotStatus != PassengerClarificationSlotConflicting ||
		!containsPassengerClarificationID(persistedState.PassengerReasonCodes, passengerClarificationReasonInvalidState) ||
		!containsPassengerClarificationID(persistedState.ChildReasonCodes, passengerClarificationReasonInvalidState) {
		t.Fatalf("contradictory persisted snapshot must become invalidPassengerClarificationStateV1: %+v exists=%t", persistedState, ok)
	}
}

func TestPassengerGateStrongCancellationWinsUnsafeState(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedPassengerCollectionPhase(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "passenger-gate-strong-cancel",
			IdempotencyKey:    "passenger-gate-strong-cancel",
			Body:              "quero cancelar minha reserva",
		},
	}); err != nil {
		t.Fatalf("ingest cancellation: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess cancellation: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("expected strong cancellation guardrail draft")
	}
	if runner.calls != 0 || searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("strong cancellation must remain local, runner=%d availability=%d tools=%+v", runner.calls, searcher.calls, out.ToolCalls)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentBookingCancel) {
		t.Fatalf("strong cancellation must win passenger gate, got intent=%q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	folded := foldChatText(out.Draft.Body)
	if !strings.Contains(folded, "cancelamento") || strings.Contains(folded, "passagem e so para voce") {
		t.Fatalf("expected only cancellation guardrail, got %q", out.Draft.Body)
	}
}

func TestSingleOptionStrongBaselinePreservesHumanAndCancellation(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantIntent   Intent
		wantTemplate ResponseTemplateName
	}{
		{name: "deictic_human", body: "essa msm, quero falar com atendente", wantIntent: IntentHumanSupport, wantTemplate: TemplateHumanHandoff},
		{name: "numeric_human", body: "1, quero falar com atendente", wantIntent: IntentHumanSupport, wantTemplate: TemplateHumanHandoff},
		{name: "deictic_cancel", body: "essa msm, quero cancelar", wantIntent: IntentBookingCancel},
		{name: "numeric_cancel", body: "1, quero cancelar", wantIntent: IntentBookingCancel},
		{name: "option_keyword_cancel", body: "opção 1, quero cancelar", wantIntent: IntentBookingCancel},
		{name: "ack_cancel", body: "ok, quero cancelar", wantIntent: IntentBookingCancel},
		{name: "ordinal_option_human", body: "primeira opção, quero falar com atendente", wantIntent: IntentHumanSupport, wantTemplate: TemplateHumanHandoff},
		{name: "numeric_person", body: "1, quero falar com uma pessoa", wantIntent: IntentHumanSupport, wantTemplate: TemplateHumanHandoff},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, booking)
			routerStates := []CanonicalConversationState{}
			svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
				routerStates = append(routerStates, canonicalConversationStateFromTestValue(t, state, "mixed STRONG router baseline"))
				return routeDeterministicIntent(history, currentTurn, state, observedAt)
			}

			now := availabilityTestObservedAt()
			contactKey := "selection-strong-" + test.name
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed strong selection session: %v", err)
			}
			availability := singleOptionUnselectedAvailabilityResultAt(now)
			seedAvailabilitySelectionPromptForFailClosedTest(
				t,
				store,
				session.ID,
				"selection-strong-list-"+test.name,
				availability,
				messageStatusAutomationSent,
				true,
				now.Add(-time.Minute),
			)

			out := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "selection-strong-turn-"+test.name, test.body)
			if out.Draft == nil {
				t.Fatal("expected local STRONG guardrail draft")
			}
			if len(routerStates) != 1 {
				t.Fatalf("mixed STRONG turn routed %d times, want exactly one baseline route", len(routerStates))
			}
			assertSingleOptionEnvelopeOnlyRouteForTest(t, routerStates[0].Route, availability, "router baseline")
			if got := Intent(strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"]))); got != test.wantIntent {
				t.Fatalf("selection gate replaced STRONG intent %s with %s: payload=%+v body=%q", test.wantIntent, got, out.Draft.NormalizedPayload, out.Draft.Body)
			}
			if test.wantTemplate != "" {
				if got := ResponseTemplateName(strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"]))); got != test.wantTemplate {
					t.Fatalf("selection gate replaced STRONG template %s with %s: payload=%+v", test.wantTemplate, got, out.Draft.NormalizedPayload)
				}
			} else if test.wantIntent == IntentBookingCancel {
				if got := ResponseTemplateName(strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"]))); got == TemplateAskPassengerCount {
					t.Fatalf("cancellation STRONG turn received passenger template: payload=%+v", out.Draft.NormalizedPayload)
				}
				if !strings.Contains(foldChatText(out.Draft.Body), "cancelamento") {
					t.Fatalf("cancellation STRONG turn lost its local response: %q", out.Draft.Body)
				}
			}
			for label, payload := range map[string]map[string]interface{}{
				"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
			} {
				if index := payloadSelectedOptionIndex(payload); index != 0 {
					t.Fatalf("%s STRONG turn persisted selected_option_index=%d: %+v", label, index, payload)
				}
				if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
					t.Fatalf("%s STRONG turn persisted selected snapshot: %+v", label, snapshot)
				}
				for _, key := range []string{passengerPendingPromptEventV1MessageKey, passengerPromptEventV1MessageKey} {
					if _, exists := payload[key]; exists {
						t.Fatalf("%s STRONG turn opened passenger prompt event %s: %+v", label, key, payload)
					}
				}
			}
			persisted := store.sessions[session.ID]
			if canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) ||
				canonicalAvailabilityFactsInvalidationBoundaryInMetadata(persisted.Metadata).known() {
				t.Fatalf("STRONG turn changed availability invalidation marker/boundary: %+v", persisted.Metadata)
			}
			canonical := canonicalConversationStateFromTestValue(
				t,
				asMap(persisted.Metadata["agent"])["canonical_state"],
				"strong selection canonical state",
			)
			assertSingleOptionEnvelopeOnlyRouteForTest(t, canonical.Route, availability, "persisted canonical route")
			memoryCanonical := canonicalConversationStateFromTestValue(
				t,
				out.Memory["canonical_state"],
				"mixed STRONG memory canonical state",
			)
			assertSingleOptionEnvelopeOnlyRouteForTest(t, memoryCanonical.Route, availability, "mixed STRONG memory route")
			history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
			if err != nil {
				t.Fatalf("list mixed STRONG history: %v", err)
			}
			bookingDraft := collectBookingDraftContextForRoutingBaseline(persisted, history)
			if bookingDraft.HasBookableSelection || bookingDraft.SelectedOptionIndex != 0 ||
				bookingDraft.TripID != "" || bookingDraft.BoardStopID != "" || bookingDraft.AlightStopID != "" ||
				bookingDraft.TripDate != "" || bookingDraft.DepartureTime != "" ||
				bookingDraft.Price != 0 || bookingDraft.Currency != "" {
				t.Fatalf("mixed STRONG routing baseline created a bookable selection: %+v", bookingDraft)
			}
			if !bookingDraft.HasAvailabilityShown ||
				bookingDraft.Origin != availability.Filter.Origin ||
				bookingDraft.Destination != availability.Filter.Destination {
				t.Fatalf("mixed STRONG routing baseline lost its availability envelope: %+v", bookingDraft)
			}
			if runner.calls != 0 || searcher.calls != 0 || booking.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("STRONG turn dispatched external work: runner=%d availability=%d booking=%d tools=%+v", runner.calls, searcher.calls, booking.calls, out.ToolCalls)
			}
		})
	}
}

func TestStrongPreservesPriorBookableSelectionAgainstUnselectedSingleOption(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, booking)

	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "strong-preserves-option-2", CustomerPhone: "strong-preserves-option-2",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed prior-selection session: %v", err)
	}
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	const (
		priorSelectedPackage = "package-strong-selected-a"
		laterEnvelopePackage = "package-strong-unselected-b"
	)
	availability.Filter.PackageName = priorSelectedPackage
	for index := range availability.Results {
		availability.Results[index].PackageName = priorSelectedPackage
	}
	seedCanonicalAvailabilitySelectionForTest(
		t,
		store,
		session.ID,
		availability,
		2,
		now.Add(-4*time.Minute),
		askPassengerCountReply,
	)
	currentAvailability := singleOptionUnselectedAvailabilityResultAt(now.Add(30 * 24 * time.Hour))
	currentAvailability.Filter.PackageName = laterEnvelopePackage
	currentAvailability.Results[0].PackageName = laterEnvelopePackage
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "strong-current-list", currentAvailability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)

	historyBefore, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list prior-selection baseline: %v", err)
	}
	replayed, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("replay prior-selection baseline: %v", err)
	}
	session = replayed.Session
	baseline := deriveCanonicalConversationStateForRoutingBaseline(session, historyBefore)
	want := availability.Results[1]
	assertCanonicalRouteSelectionForTest(t, baseline.Route, 2, want, "prior routing baseline")
	assertStringValuesAbsentAtAnyDepthForTest(t, baseline, "prior routing baseline state", laterEnvelopePackage)

	routerStates := []CanonicalConversationState{}
	svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
		routerStates = append(routerStates, canonicalConversationStateFromTestValue(t, state, "prior-selection STRONG router baseline"))
		return routeDeterministicIntent(history, currentTurn, state, observedAt)
	}
	out := ingestAndReprocessActivePromptFlowTurn(
		t, svc, session.ContactKey, "strong-preserves-option-2-turn", "opção 1, quero cancelar",
	)
	if out.Draft == nil {
		t.Fatal("expected cancellation guardrail draft")
	}
	if got := Intent(strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"]))); got != IntentBookingCancel {
		t.Fatalf("expected cancellation STRONG response, got %s payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if len(routerStates) != 1 {
		t.Fatalf("prior-selection STRONG turn routed %d times, want one", len(routerStates))
	}
	assertCanonicalRouteSelectionForTest(t, routerStates[0].Route, 2, want, "router baseline")
	assertStringValuesAbsentAtAnyDepthForTest(t, routerStates[0], "router baseline state", laterEnvelopePackage)
	for label, payload := range map[string]map[string]interface{}{
		"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
	} {
		if index := payloadSelectedOptionIndex(payload); index != 0 {
			t.Fatalf("%s mixed STRONG draft selected option %d: %+v", label, index, payload)
		}
		if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
			t.Fatalf("%s mixed STRONG draft persisted a selection snapshot: %+v", label, snapshot)
		}
		for _, key := range []string{passengerPendingPromptEventV1MessageKey, passengerPromptEventV1MessageKey} {
			if _, exists := payload[key]; exists {
				t.Fatalf("%s mixed STRONG draft opened passenger prompt event %s: %+v", label, key, payload)
			}
		}
	}
	if intentMemory := asMap(out.Memory["intent_decision"]); payloadSelectedOptionIndex(intentMemory) != 0 ||
		len(asMap(intentMemory[selectedAvailabilityResultPayloadKey])) != 0 {
		t.Fatalf("mixed STRONG memory selected option 1: %+v", intentMemory)
	}
	persisted := store.sessions[session.ID]
	persistedCanonical := canonicalConversationStateFromTestValue(
		t, asMap(persisted.Metadata["agent"])["canonical_state"], "persisted prior selection after STRONG",
	)
	assertCanonicalRouteSelectionForTest(t, persistedCanonical.Route, 2, want, "persisted baseline")
	assertStringValuesAbsentAtAnyDepthForTest(t, persisted.Metadata, "persisted metadata after STRONG", laterEnvelopePackage)
	memoryCanonical := canonicalConversationStateFromTestValue(
		t, out.Memory["canonical_state"], "prior selection memory after STRONG",
	)
	assertCanonicalRouteSelectionForTest(t, memoryCanonical.Route, 2, want, "memory baseline")
	assertStringValuesAbsentAtAnyDepthForTest(t, out.Memory, "memory after STRONG", laterEnvelopePackage)
	historyAfter, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list prior-selection history after STRONG: %v", err)
	}
	bookingDraft := collectBookingDraftContextForRoutingBaseline(persisted, historyAfter)
	if !bookingDraft.HasBookableSelection {
		t.Fatalf("mixed STRONG turn removed the prior bookable selection: %+v", bookingDraft)
	}
	if bookingDraft.SelectedOptionIndex != 2 || bookingDraft.TripID != want.TripID ||
		bookingDraft.BoardStopID != want.BoardStopID || bookingDraft.AlightStopID != want.AlightStopID ||
		bookingDraft.Origin != want.OriginDisplayName || bookingDraft.Destination != want.DestinationDisplayName ||
		bookingDraft.TripDate != want.TripDate || bookingDraft.DepartureTime != want.OriginDepartTime ||
		bookingDraft.Price != want.Price || bookingDraft.Currency != want.Currency {
		t.Fatalf("mixed STRONG routing baseline changed prior option 2: %+v", bookingDraft)
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) ||
		canonicalAvailabilityFactsInvalidationBoundaryInMetadata(persisted.Metadata).known() {
		t.Fatalf("mixed STRONG turn changed marker/boundary: %+v", persisted.Metadata)
	}
	if runner.calls != 0 || searcher.calls != 0 || booking.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("mixed STRONG turn dispatched external work: runner=%d availability=%d booking=%d tools=%+v",
			runner.calls, searcher.calls, booking.calls, out.ToolCalls)
	}
}

func TestMaterializeReplacesPriorBookableSelectionAggregateAndSurvivesReload(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, booking)

	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "materialize-replaces-aggregate", CustomerPhone: "materialize-replaces-aggregate",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed aggregate replacement session: %v", err)
	}

	const (
		previousPackage    = "package-materialized-a"
		replacementPackage = "package-materialized-b"
	)
	previousAvailability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	previousAvailability.Filter.PackageName = previousPackage
	for index := range previousAvailability.Results {
		previousAvailability.Results[index].PackageName = previousPackage
	}
	seedCanonicalAvailabilitySelectionForTest(
		t,
		store,
		session.ID,
		previousAvailability,
		2,
		now.Add(-4*time.Minute),
		askPassengerCountReply,
	)

	replacementAvailability := availabilityOptionPromptFutureResultAt(now.Add(30 * 24 * time.Hour))
	replacementAvailability.Filter.PackageName = replacementPackage
	replacementAvailability.Results[0].PackageName = replacementPackage
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "materialize-replacement-list", replacementAvailability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)

	out := ingestAndReprocessActivePromptFlowTurn(
		t, svc, session.ContactKey, "materialize-replacement-option-1", "1",
	)
	if out.Draft == nil {
		t.Fatal("expected materialized replacement draft")
	}
	want := replacementAvailability.Results[0]
	assertDraftSelectionForTest(t, out.Draft.Payload, 1, want, "replacement payload")
	assertDraftSelectionForTest(t, out.Draft.NormalizedPayload, 1, want, "replacement normalized payload")
	replacementSnapshot := asMap(out.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
	wantSelectionMessageID := strings.TrimSpace(asString(replacementSnapshot[selectedAvailabilitySelectionMessageIDPayloadKey]))
	wantPromptSourceMessageID := strings.TrimSpace(asString(replacementSnapshot[availabilityPromptSourceMessageIDPayloadKey]))
	if wantSelectionMessageID != out.Draft.ID || wantPromptSourceMessageID == "" {
		t.Fatalf(
			"replacement authority IDs not persisted: selection=%q want=%q prompt_source=%q snapshot=%+v",
			wantSelectionMessageID,
			out.Draft.ID,
			wantPromptSourceMessageID,
			replacementSnapshot,
		)
	}

	memoryCanonical := canonicalConversationStateFromTestValue(
		t, out.Memory["canonical_state"], "replacement memory canonical state",
	)
	assertCanonicalRouteSelectionForTest(t, memoryCanonical.Route, 1, want, "replacement memory route")
	assertStringValuesAbsentAtAnyDepthForTest(
		t,
		out.Memory,
		"replacement memory",
		previousPackage,
		previousAvailability.Results[1].TripID,
		previousAvailability.Results[1].BoardStopID,
		previousAvailability.Results[1].AlightStopID,
	)

	persisted := store.sessions[session.ID]
	persistedCanonical := canonicalConversationStateFromTestValue(
		t, asMap(persisted.Metadata["agent"])["canonical_state"], "replacement persisted canonical state",
	)
	assertCanonicalRouteSelectionForTest(t, persistedCanonical.Route, 1, want, "replacement persisted route")
	assertStringValuesAbsentAtAnyDepthForTest(
		t,
		persisted.Metadata,
		"replacement persisted metadata",
		previousPackage,
		previousAvailability.Results[1].TripID,
		previousAvailability.Results[1].BoardStopID,
		previousAvailability.Results[1].AlightStopID,
	)

	markSessionMessagesAutomationSent(t, store, session.ID)
	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload replacement session: %v", err)
	}
	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list replacement history after reload: %v", err)
	}
	reloadedBaseline := deriveCanonicalConversationStateForRoutingBaseline(reloaded, history)
	assertCanonicalRouteSelectionForTest(t, reloadedBaseline.Route, 1, want, "replacement baseline after reload")
	reloadedDraft := collectBookingDraftContextForRoutingBaseline(reloaded, history)
	if !reloadedDraft.HasBookableSelection || reloadedDraft.SelectedOptionIndex != 1 ||
		reloadedDraft.TripID != want.TripID ||
		reloadedDraft.BoardStopID != want.BoardStopID ||
		reloadedDraft.AlightStopID != want.AlightStopID ||
		reloadedDraft.PackageName != want.PackageName ||
		reloadedDraft.SelectionMessageID != wantSelectionMessageID ||
		reloadedDraft.AvailabilityPromptSourceMessageID != wantPromptSourceMessageID ||
		reloadedDraft.TripDate != want.TripDate ||
		reloadedDraft.DepartureTime != want.OriginDepartTime ||
		reloadedDraft.Price != want.Price ||
		reloadedDraft.Currency != want.Currency {
		t.Fatalf("replacement booking draft drifted after reload: got=%+v want=%+v", reloadedDraft, want)
	}
	assertStringValuesAbsentAtAnyDepthForTest(
		t,
		reloadedBaseline,
		"replacement baseline after reload",
		previousPackage,
		previousAvailability.Results[1].TripID,
		previousAvailability.Results[1].BoardStopID,
		previousAvailability.Results[1].AlightStopID,
	)
	if runner.calls != 0 || searcher.calls != 0 || booking.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("aggregate materialization dispatched external work: runner=%d availability=%d booking=%d tools=%+v",
			runner.calls, searcher.calls, booking.calls, out.ToolCalls)
	}
}

func TestMixedStrongAvailabilitySelectionStatePreservesInvalidationBoundary(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "strong-preserves-boundary", CustomerPhone: "strong-preserves-boundary",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed boundary session: %v", err)
	}
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "strong-boundary-list", availability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)
	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil || len(history) == 0 {
		t.Fatalf("list boundary seed history: count=%d err=%v", len(history), err)
	}
	markCanonicalAvailabilityInvalidatedAfterMessageForTest(store, session.ID, history[len(history)-1])
	wantBoundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(store.sessions[session.ID].Metadata)
	if !wantBoundary.known() {
		t.Fatal("expected seeded invalidation boundary")
	}

	out := ingestAndReprocessActivePromptFlowTurn(
		t, svc, session.ContactKey, "strong-preserves-boundary-turn", "opção 1, quero cancelar",
	)
	if out.Draft == nil {
		t.Fatal("expected cancellation guardrail draft")
	}
	if got := Intent(strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"]))); got != IntentBookingCancel {
		t.Fatalf("STRONG cancellation lost to invalidated-selection fallback: got=%s payload=%+v body=%q",
			got, out.Draft.NormalizedPayload, out.Draft.Body)
	}
	for label, payload := range map[string]map[string]interface{}{
		"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
	} {
		if payloadSelectedOptionIndex(payload) != 0 || len(asMap(payload[selectedAvailabilityResultPayloadKey])) != 0 {
			t.Fatalf("%s invalidated mixed STRONG turn persisted selection: %+v", label, payload)
		}
		if _, exists := payload[passengerPendingPromptEventV1MessageKey]; exists {
			t.Fatalf("%s invalidated mixed STRONG turn opened passenger prompt: %+v", label, payload)
		}
	}
	persisted := store.sessions[session.ID]
	gotBoundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(persisted.Metadata)
	if !canonicalAvailabilityFactsInvalidatedInMetadata(persisted.Metadata) ||
		gotBoundary.AfterMessageID != wantBoundary.AfterMessageID ||
		!gotBoundary.AfterCreatedAt.Equal(wantBoundary.AfterCreatedAt) {
		t.Fatalf("mixed STRONG turn changed marker/boundary: got=%+v want=%+v metadata=%+v",
			gotBoundary, wantBoundary, persisted.Metadata)
	}
	canonical := canonicalConversationStateFromTestValue(
		t, asMap(persisted.Metadata["agent"])["canonical_state"], "invalidated mixed STRONG canonical state",
	)
	assertCanonicalRouteHasNoSelectionForTest(t, canonical.Route, "invalidated mixed STRONG route")
	if runner.calls != 0 || searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("invalidated mixed STRONG turn dispatched external work: runner=%d availability=%d tools=%+v",
			runner.calls, searcher.calls, out.ToolCalls)
	}
}

func TestPureSingleOptionMaterializePersistsSelection(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "ack", body: "ok"},
		{name: "numeric", body: "1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, booking)

			now := availabilityTestObservedAt()
			contactKey := "pure-single-selection-" + test.name
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey,
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed pure-selection session: %v", err)
			}
			availability := singleOptionUnselectedAvailabilityResultAt(now)
			seedAvailabilitySelectionPromptForFailClosedTest(
				t, store, session.ID, "pure-single-selection-list-"+test.name, availability,
				messageStatusAutomationSent, true, now.Add(-time.Minute),
			)

			out := ingestAndReprocessActivePromptFlowTurn(
				t, svc, session.ContactKey, "pure-single-selection-turn-"+test.name, test.body,
			)
			if out.Draft == nil {
				t.Fatal("expected passenger prompt after pure selection")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
				t.Fatalf("pure MATERIALIZE must ask passengers, got template=%q payload=%+v", got, out.Draft.NormalizedPayload)
			}
			want := availability.Results[0]
			assertDraftSelectionForTest(t, out.Draft.Payload, 1, want, "pure selection payload")
			assertDraftSelectionForTest(t, out.Draft.NormalizedPayload, 1, want, "pure selection normalized payload")
			if _, exists := out.Draft.NormalizedPayload[passengerPendingPromptEventV1MessageKey]; !exists {
				t.Fatalf("pure materialized selection did not create passenger prompt event: %+v", out.Draft.NormalizedPayload)
			}
			persisted := store.sessions[session.ID]
			canonical := canonicalConversationStateFromTestValue(
				t, asMap(persisted.Metadata["agent"])["canonical_state"], "pure selection canonical state",
			)
			assertCanonicalRouteSelectionForTest(t, canonical.Route, 1, want, "pure selection canonical route")
			memoryCanonical := canonicalConversationStateFromTestValue(
				t, out.Memory["canonical_state"], "pure selection memory canonical state",
			)
			assertCanonicalRouteSelectionForTest(t, memoryCanonical.Route, 1, want, "pure selection memory route")
			if runner.calls != 0 || searcher.calls != 0 || booking.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("pure selection dispatched external work: runner=%d availability=%d booking=%d tools=%+v",
					runner.calls, searcher.calls, booking.calls, out.ToolCalls)
			}
		})
	}
}

func TestSingleOptionStrongBaselineDoesNotReachNextTurnState(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "Posso ajudar com outra coisa.", Model: "gpt-test"}}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI, searcher)

	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "selection-after-mixed-strong", CustomerPhone: "selection-after-mixed-strong",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed next-turn session: %v", err)
	}
	availability := singleOptionUnselectedAvailabilityResultAt(now)
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "selection-before-mixed-strong", availability,
		messageStatusAutomationSent, true, now.Add(-time.Minute),
	)
	first := ingestAndReprocessActivePromptFlowTurn(
		t, svc, session.ContactKey, "mixed-strong-before-next-turn", "opção 1, quero cancelar",
	)
	if first.Draft == nil || Intent(strings.TrimSpace(asString(first.Draft.NormalizedPayload["intent"]))) != IntentBookingCancel {
		t.Fatalf("expected first turn cancellation guardrail, draft=%+v", first.Draft)
	}
	if openAI.calls != 0 {
		t.Fatalf("mixed STRONG turn reached interpreter %d times", openAI.calls)
	}
	markSessionMessagesAutomationSent(t, store, session.ID)

	routerStates := []CanonicalConversationState{}
	svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
		routerStates = append(routerStates, canonicalConversationStateFromTestValue(t, state, "post-STRONG router state"))
		return routeDeterministicIntent(history, currentTurn, state, observedAt)
	}
	second := ingestAndReprocessActivePromptFlowTurn(
		t, svc, session.ContactKey, "turn-after-mixed-strong", "oi",
	)
	if second.Draft == nil {
		t.Fatal("expected next-turn draft")
	}
	if len(routerStates) == 0 {
		t.Fatal("next turn did not route")
	}
	for i, state := range routerStates {
		assertSingleOptionEnvelopeOnlyRouteForTest(
			t, state.Route, availability, fmt.Sprintf("next router state %d", i),
		)
	}
	if openAI.calls == 0 {
		t.Fatal("next safe turn did not exercise the structured interpreter shadow")
	}
	assertSingleOptionEnvelopeOnlyRouteForTest(
		t, openAI.lastInput.StructuredInput.State.Route, availability, "next interpreter state",
	)
	persisted := store.sessions[session.ID]
	canonical := canonicalConversationStateFromTestValue(
		t, asMap(persisted.Metadata["agent"])["canonical_state"], "post-STRONG persisted state",
	)
	assertCanonicalRouteHasNoSingleOptionItemForTest(t, canonical.Route, "post-STRONG persisted route")
	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list next-turn history: %v", err)
	}
	bookingDraft := collectBookingDraftContextForRoutingBaseline(persisted, history)
	if bookingDraft.HasBookableSelection || bookingDraft.SelectedOptionIndex != 0 ||
		bookingDraft.TripID != "" || bookingDraft.BoardStopID != "" || bookingDraft.AlightStopID != "" ||
		bookingDraft.TripDate != "" || bookingDraft.DepartureTime != "" ||
		bookingDraft.Price != 0 || bookingDraft.Currency != "" {
		t.Fatalf("next routing baseline reused mixed STRONG selection: %+v", bookingDraft)
	}
}

func assertCanonicalRouteSelectionForTest(
	t *testing.T,
	route CanonicalRouteState,
	index int,
	want AvailabilitySearchItem,
	label string,
) {
	t.Helper()
	if route.SelectedOptionIndex != index || route.TripID != want.TripID ||
		route.BoardStopID != want.BoardStopID || route.AlightStopID != want.AlightStopID ||
		route.Origin != want.OriginDisplayName || route.Destination != want.DestinationDisplayName ||
		route.PackageName != want.PackageName || route.TripDate != want.TripDate ||
		route.DepartureTime != want.OriginDepartTime || route.Price != want.Price ||
		route.Currency != want.Currency {
		t.Fatalf("%s selection mismatch: got=%+v want_option=%d want=%+v", label, route, index, want)
	}
}

func assertCanonicalRouteHasNoSelectionForTest(t *testing.T, route CanonicalRouteState, label string) {
	t.Helper()
	if route.SelectedOptionIndex != 0 || route.TripID != "" || route.BoardStopID != "" || route.AlightStopID != "" {
		t.Fatalf("%s retained mixed STRONG selection: %+v", label, route)
	}
}

func assertSingleOptionEnvelopeOnlyRouteForTest(
	t *testing.T,
	route CanonicalRouteState,
	availability AvailabilitySearchResult,
	label string,
) {
	t.Helper()
	if route.Origin != availability.Filter.Origin || route.Destination != availability.Filter.Destination {
		t.Fatalf("%s lost filter route: got=%+v filter=%+v", label, route, availability.Filter)
	}
	if route.SelectedOptionIndex != 0 || route.TripID != "" ||
		route.BoardStopID != "" || route.AlightStopID != "" ||
		route.TripDate != "" || route.DepartureTime != "" ||
		route.Price != 0 || route.Currency != "" || route.PackageName != "" {
		t.Fatalf("%s promoted item-only facts from the unselected single option: %+v", label, route)
	}
}

func assertCanonicalRouteHasNoSingleOptionItemForTest(t *testing.T, route CanonicalRouteState, label string) {
	t.Helper()
	if route.SelectedOptionIndex != 0 || route.TripID != "" ||
		route.BoardStopID != "" || route.AlightStopID != "" ||
		route.TripDate != "" || route.DepartureTime != "" ||
		route.Price != 0 || route.Currency != "" || route.PackageName != "" {
		t.Fatalf("%s retained item-only facts from the unselected single option: %+v", label, route)
	}
}

func assertDraftSelectionForTest(
	t *testing.T,
	payload map[string]interface{},
	index int,
	want AvailabilitySearchItem,
	label string,
) {
	t.Helper()
	if got := payloadSelectedOptionIndex(payload); got != index {
		t.Fatalf("%s selected_option_index=%d, want %d: %+v", label, got, index, payload)
	}
	snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
	if strings.TrimSpace(asString(snapshot["trip_id"])) != want.TripID ||
		strings.TrimSpace(asString(snapshot["board_stop_id"])) != want.BoardStopID ||
		strings.TrimSpace(asString(snapshot["alight_stop_id"])) != want.AlightStopID ||
		strings.TrimSpace(asString(snapshot["origin_display_name"])) != want.OriginDisplayName ||
		strings.TrimSpace(asString(snapshot["destination_display_name"])) != want.DestinationDisplayName ||
		strings.TrimSpace(asString(snapshot["package_name"])) != want.PackageName ||
		strings.TrimSpace(asString(snapshot["trip_date"])) != want.TripDate ||
		strings.TrimSpace(asString(snapshot["origin_depart_time"])) != want.OriginDepartTime ||
		asFloat64(snapshot["price"]) != want.Price ||
		strings.TrimSpace(asString(snapshot["currency"])) != want.Currency {
		t.Fatalf("%s incomplete selected snapshot: got=%+v want=%+v", label, snapshot, want)
	}
}

func TestPassengerCancellationAndHumanDoNotCreatePromptEventInUnsafeState(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantIntent Intent
	}{
		{name: "cancellation", body: "quero cancelar minha reserva", wantIntent: IntentBookingCancel},
		{name: "human", body: "quero falar com um atendente", wantIntent: IntentHumanSupport},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{enabled: true})
			session := seedPassengerCollectionPhase(t, store)
			before, ok := passengerClarificationStateV1FromSession(store.sessions[session.ID])
			if !ok || strings.TrimSpace(before.PassengerPromptMessageID) == "" {
				t.Fatalf("expected active passenger prompt before %s: %+v", test.name, before)
			}

			out := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "unsafe-guardrail-"+test.name, test.body)
			if out.Draft == nil {
				t.Fatalf("expected %s guardrail draft", test.name)
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(test.wantIntent) {
				t.Fatalf("expected %s intent %s, got %q payload=%+v", test.name, test.wantIntent, got, out.Draft.NormalizedPayload)
			}
			for label, payload := range map[string]map[string]interface{}{
				"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
			} {
				for _, key := range []string{passengerPendingPromptEventV1MessageKey, passengerPromptEventV1MessageKey} {
					if _, exists := payload[key]; exists {
						t.Fatalf("%s %s draft created %s: %+v", test.name, label, key, payload)
					}
				}
			}

			reply, err := store.CreateAutomationReply(context.Background(), CreateAutomationReplyInput{
				SessionID: session.ID, DraftMessageID: out.Draft.ID,
				IdempotencyKey: "unsafe-guardrail-outbound-" + test.name,
			}, 0)
			if err != nil {
				t.Fatalf("create %s outbound: %v", test.name, err)
			}
			if _, err := store.MarkReplyDeliverySent(context.Background(), MarkReplyDeliverySentInput{
				SessionID: session.ID, MessageID: reply.Message.ID, OutboundID: reply.Outbound.ID,
				ProviderMessageID: "provider-unsafe-guardrail-" + test.name,
				ProviderStatus:    messageStatusAutomationSent, SentAt: time.Now().UTC(),
			}); err != nil {
				t.Fatalf("deliver %s outbound: %v", test.name, err)
			}
			after, ok := passengerClarificationStateV1FromSession(store.sessions[session.ID])
			if !ok || after.PassengerPromptMessageID != before.PassengerPromptMessageID ||
				len(after.AppliedEventIDs) != len(before.AppliedEventIDs) {
				t.Fatalf("delivered %s guardrail changed passenger prompt epoch: before=%+v after=%+v exists=%t", test.name, before, after, ok)
			}
		})
	}
}

func ingestAndReprocessFreshPassengerGateTurn(t *testing.T, svc *Service, key string, body string) ReprocessResult {
	t.Helper()
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "fresh-passenger-gate",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: key,
			IdempotencyKey:    key,
			Body:              body,
		},
	})
	if err != nil {
		t.Fatalf("ingest %q: %v", body, err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess %q: %v", body, err)
	}
	return out
}

func assertFreshSessionPassengerGateInactive(t *testing.T, store *fakeStore, out ReprocessResult) {
	t.Helper()
	if out.Draft == nil {
		t.Fatal("expected draft while checking fresh passenger gate")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateAskPassengerCount) {
		t.Fatalf("fresh unknown slots must not ask passengers before trip selection, payload=%+v body=%q", out.Draft.NormalizedPayload, out.Draft.Body)
	}
	folded := foldChatText(out.Draft.Body)
	for _, forbidden := range []string{"passagem e so para voce", "vai mais alguem", "quantos passageiros", "crianca de 5 anos"} {
		if strings.Contains(folded, forbidden) {
			t.Fatalf("fresh unknown slots opened passenger collection with %q in %q", forbidden, out.Draft.Body)
		}
	}
	for _, payload := range []map[string]interface{}{out.Draft.Payload, out.Draft.NormalizedPayload} {
		for _, key := range []string{passengerPendingPromptEventV1MessageKey, passengerPromptEventV1MessageKey} {
			if _, exists := payload[key]; exists {
				t.Fatalf("fresh flow must not persist %s before trip selection, payload=%+v", key, payload)
			}
		}
	}
	state, ok := passengerClarificationStateV1FromSession(store.sessions[out.Session.ID])
	if !ok {
		t.Fatal("expected fresh passenger state to be bootstrapped")
	}
	if state.HasEvidence || state.PassengerPromptMessageID != "" || state.ChildPromptMessageID != "" {
		t.Fatalf("fresh unknown slots must remain outside active passenger context, state=%+v", state)
	}
}

func freshSessionPassengerGateAvailabilityResult() AvailabilitySearchResult {
	tripDate := time.Now().UTC().Add(7 * 24 * time.Hour).Format("2006-01-02")
	return AvailabilitySearchResult{
		Results: []AvailabilitySearchItem{{
			TripID:                 "fresh-trip-1",
			BoardStopID:            "fresh-board-1",
			AlightStopID:           "fresh-alight-1",
			OriginDisplayName:      "Moncao/MA",
			DestinationDisplayName: "Videira/SC",
			OriginDepartTime:       "09:00",
			TripDate:               tripDate,
			SeatsAvailable:         6,
			Price:                  950,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToSantaCatarina,
		}},
	}
}

func singleOptionUnselectedAvailabilityResultAt(observedAt time.Time) AvailabilitySearchResult {
	availability := availabilityOptionPromptFutureResultAt(observedAt)
	availability.Filter.PackageName = ""
	availability.Results[0].PackageName = "single-item-only-package"
	return availability
}
