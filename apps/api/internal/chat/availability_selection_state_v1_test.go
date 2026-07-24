package chat

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func TestAvailabilitySelectionStateV1SupersededAuthorityNeverResurrects(t *testing.T) {
	now := availabilityTestObservedAt()
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "trip-b"
	availabilityB.Results[0].BoardStopID = "board-b"
	availabilityB.Results[0].AlightStopID = "alight-b"
	snapshotA := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1)
	snapshotB := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1)

	state := ReduceAvailabilitySelectionEventsV1(
		newAvailabilitySelectionStateV1(),
		orderedAvailabilitySelectionEventsForTest(
			materializedAvailabilitySelectionEventForTest("selection-event-a", "projection-a", "prompt-a", snapshotA),
			materializedAvailabilitySelectionEventForTest("selection-event-b", "projection-b", "prompt-b", snapshotB),
			AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         "rejection-b",
				AvailabilityPromptSourceMessageID: "prompt-b",
				RejectedOptionIndexes:             []int{1},
			},
		),
	)
	if state.Status != AvailabilitySelectionStatusRejected ||
		state.SelectedOptionIndex != 0 ||
		state.SelectionEventMessageID != "" ||
		state.SelectionProjectionMessageID != "" ||
		state.AvailabilityPromptSourceMessageID != "" ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("S1 -> S2 -> reject S2 must end with no authority: %+v", state)
	}

	state = ReduceAvailabilitySelectionEventsV1(
		state,
		orderedAvailabilitySelectionEventsForTestFrom(
			3,
			AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventMaterialized,
				MessageID:                         "copied-projection-a",
				ProjectionMessageID:               "copied-projection-a",
				AvailabilityPromptSourceMessageID: "prompt-a",
				SelectedOptionIndex:               1,
				Snapshot:                          snapshotA,
				MaterializesAuthority:             false,
			},
		),
	)
	if state.Status != AvailabilitySelectionStatusRejected ||
		state.SelectedOptionIndex != 0 ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("copied S1 projection resurrected superseded authority: %+v", state)
	}
}

func TestAvailabilitySelectionStateV1ScopesRejectionToPromptSource(t *testing.T) {
	availability := availabilityOptionPromptFutureResultAt(availabilityTestObservedAt())
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	state := ReduceAvailabilitySelectionEventsV1(
		newAvailabilitySelectionStateV1(),
		orderedAvailabilitySelectionEventsForTest(
			materializedAvailabilitySelectionEventForTest(
				"selection-event-a",
				"projection-a",
				"prompt-a",
				snapshot,
			),
			AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         "rejection-b",
				AvailabilityPromptSourceMessageID: "prompt-b",
				RejectedOptionIndexes:             []int{1},
			},
		),
	)
	if state.Status != AvailabilitySelectionStatusBookable ||
		state.SelectionEventMessageID != "selection-event-a" ||
		state.AvailabilityPromptSourceMessageID != "prompt-a" ||
		state.Snapshot.TripID != snapshot.TripID {
		t.Fatalf("rejection from prompt B cleared selection from prompt A: %+v", state)
	}
	if !state.rejectsPromptOption("prompt-b", 1, snapshot.TripDate) {
		t.Fatalf("prompt B tombstone was not retained durably: %+v", state.Rejections)
	}
	if state.rejectsPromptOption("prompt-a", 1, snapshot.TripDate) {
		t.Fatalf("prompt B tombstone leaked into prompt A: %+v", state.Rejections)
	}
}

func TestAvailabilitySelectionStateV1ProjectionOnlyWindowHasNoAuthority(t *testing.T) {
	availability := availabilityOptionPromptFutureResultAt(availabilityTestObservedAt())
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	state := bootstrapAvailabilitySelectionStateV1(
		nil,
		orderedAvailabilitySelectionEventsForTest(AvailabilitySelectionEventV1{
			Type:                              AvailabilitySelectionEventMaterialized,
			MessageID:                         "projection-only",
			ProjectionMessageID:               "projection-only",
			AvailabilityPromptSourceMessageID: "superseded-prompt",
			SelectedOptionIndex:               1,
			Snapshot:                          snapshot,
			MaterializesAuthority:             false,
		}),
	)
	if state.Status != AvailabilitySelectionStatusNone ||
		state.SelectedOptionIndex != 0 ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("projection-only bootstrap created authority: %+v", state)
	}
}

func TestAvailabilitySelectionStateV1SimpleRejectionPersistsSourceAndTombstone(t *testing.T) {
	store := newFakeStore()
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-state-restart",
		CustomerPhone:  "availability-selection-state-restart",
		LastMessageAt:  &now,
		LastInboundAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	selectionInbound := createAvailabilitySelectionInboundForTest(t, store, session.ID, "1", now)
	materialized, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					selectionInbound.ID,
					"selection-projection",
					"availability-prompt",
					snapshot,
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("materialize selection: %v", err)
	}
	if materialized.AvailabilitySelectionState.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("selection did not become bookable: %+v", materialized.AvailabilitySelectionState)
	}

	rejectionInbound := createAvailabilitySelectionInboundForTest(
		t,
		store,
		session.ID,
		"não quero a opção 1",
		now.Add(time.Minute),
	)
	rejected, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         rejectionInbound.ID,
				AvailabilityPromptSourceMessageID: "availability-prompt",
				RejectedOptionIndexes:             []int{1},
			}},
		},
	)
	if err != nil {
		t.Fatalf("reject selection: %v", err)
	}
	state := rejected.AvailabilitySelectionState
	if state.Status != AvailabilitySelectionStatusRejected ||
		state.Tombstone == nil ||
		state.Tombstone.AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("simple rejection did not retain its real prompt source: %+v", state)
	}
	persistedRejection := store.messages[rejectionInbound.ID]
	events := availabilitySelectionEventsV1FromMessage(persistedRejection)
	if len(events) != 1 ||
		events[0].AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("inbound rejection event lost prompt source: %+v", persistedRejection.NormalizedPayload)
	}

	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	reloadedState, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || reloadedState.Status != AvailabilitySelectionStatusRejected ||
		reloadedState.Tombstone == nil ||
		reloadedState.Tombstone.AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("restart lost rejection tombstone: %+v", reloadedState)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", reloadedState); ok {
		t.Fatal("booking_create reopened without BOOKABLE state")
	}
}

func TestAvailabilitySelectionStateV1BookableStateFeedsDraftAndBookingCreate(t *testing.T) {
	availability := availabilityOptionPromptFutureResultAt(availabilityTestObservedAt())
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	state := ReduceAvailabilitySelectionEventsV1(
		newAvailabilitySelectionStateV1(),
		[]AvailabilitySelectionEventV1{
			materializedAvailabilitySelectionEventForTest(
				"selection-event",
				"selection-projection",
				"availability-prompt",
				snapshot,
			),
		},
	)
	state.BootstrapCompleted = true
	session := Session{Metadata: map[string]interface{}{
		"memory": map[string]interface{}{availabilitySelectionStateV1MemoryKey: state},
	}}
	draft := collectBookingDraftContextWithPassengerState(
		session,
		nil,
		"",
		completePassengerStateForTest(1, 0),
	)
	if !draft.HasBookableSelection ||
		draft.TripID != snapshot.TripID ||
		draft.BoardStopID != snapshot.BoardStopID ||
		draft.AlightStopID != snapshot.AlightStopID ||
		draft.SelectionMessageID != "selection-projection" ||
		draft.AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("durable BOOKABLE state did not feed booking draft: %+v", draft)
	}
	index, item, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state)
	if !ok || index != 1 ||
		item.TripID != snapshot.TripID ||
		item.BoardStopID != snapshot.BoardStopID ||
		item.AlightStopID != snapshot.AlightStopID {
		t.Fatalf("durable BOOKABLE state did not feed booking_create: index=%d item=%+v ok=%t", index, item, ok)
	}
}

func TestAvailabilitySelectionStateV1RoundTripKeepsTombstone(t *testing.T) {
	state := invalidAvailabilitySelectionStateV1("RESTART_TOMBSTONE")
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var value map[string]interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshal state map: %v", err)
	}
	decoded, ok := decodeAvailabilitySelectionStateV1(value)
	if !ok || decoded.Status != AvailabilitySelectionStatusInvalidated ||
		decoded.Tombstone == nil ||
		decoded.Tombstone.ReasonCode != "RESTART_TOMBSTONE" {
		t.Fatalf("round trip lost tombstone: %+v ok=%t", decoded, ok)
	}
}

func TestAvailabilitySelectionStateV1BookingCreateRequiresBookableStatus(t *testing.T) {
	for _, state := range []AvailabilitySelectionStateV1{
		func() AvailabilitySelectionStateV1 {
			state := newAvailabilitySelectionStateV1()
			state.BootstrapCompleted = true
			return state
		}(),
		invalidAvailabilitySelectionStateV1("TEST_INVALIDATED"),
		func() AvailabilitySelectionStateV1 {
			state := ReduceAvailabilitySelectionEventsV1(
				newAvailabilitySelectionStateV1(),
				orderedAvailabilitySelectionEventsForTest(AvailabilitySelectionEventV1{
					Type:                              AvailabilitySelectionEventRejected,
					MessageID:                         "test-rejection",
					AvailabilityPromptSourceMessageID: "test-prompt",
					RejectedWholeContext:              true,
				}),
			)
			state.BootstrapCompleted = true
			return state
		}(),
	} {
		if _, _, ok := resolveBookingCreateSelectionFromState(
			"quero reservar opção 1",
			state,
		); ok {
			t.Fatalf("booking_create accepted non-BOOKABLE state: %+v", state)
		}
	}
}

func TestAvailabilitySelectionStateV1ConcurrentFakeApplyKeepsBothEvents(t *testing.T) {
	base := newFakeStore()
	store := &serializedAvailabilitySelectionFakeStore{fakeStore: base}
	now := availabilityTestObservedAt()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-concurrent-fake",
		CustomerPhone:  "availability-selection-concurrent-fake",
		LastMessageAt:  &now,
		LastInboundAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed concurrent fake session: %v", err)
	}
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "concurrent-trip-b"
	availabilityB.Results[0].BoardStopID = "concurrent-board-b"
	availabilityB.Results[0].AlightStopID = "concurrent-alight-b"
	snapshots := []AvailabilitySelectionSnapshotV1{
		mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1),
		mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1),
	}
	messageIDs := []string{"concurrent-selection-a", "concurrent-selection-b"}
	for index, messageID := range messageIDs {
		createAvailabilitySelectionInboundForTest(
			t,
			base,
			session.ID,
			messageID,
			now.Add(time.Duration(index)*time.Second),
		)
	}
	inputs := make([]ApplyPassengerClarificationEventsV1Input, 0, len(messageIDs))
	for index, messageID := range messageIDs {
		inputs = append(inputs, ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					base.byIdempotencyKey["availability-selection-"+messageID],
					"concurrent-projection-"+messageID,
					"concurrent-prompt-"+messageID,
					snapshots[index],
				),
			},
		})
	}

	results := make(chan error, len(inputs))
	for _, input := range inputs {
		input := input
		go func() {
			_, applyErr := store.ApplyPassengerClarificationEventsV1(context.Background(), input)
			results <- applyErr
		}()
	}
	for range inputs {
		if err := <-results; err != nil {
			t.Fatalf("concurrent fake apply: %v", err)
		}
	}
	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload concurrent fake state: %v", err)
	}
	state, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || state.Status != AvailabilitySelectionStatusBookable ||
		len(state.AppliedEventIDs) != 2 {
		t.Fatalf("serialized fake apply lost an event: %+v", state)
	}
	if state.Snapshot.TripID != snapshots[0].TripID &&
		state.Snapshot.TripID != snapshots[1].TripID {
		t.Fatalf("serialized fake apply produced an unknown authority: %+v", state)
	}
}

func TestAvailabilitySelectionReplayUsesFullStructuredSessionEveryTime(t *testing.T) {
	store := newFakeStore()
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-full-bootstrap",
		CustomerPhone:  "availability-selection-full-bootstrap",
		LastMessageAt:  &now,
		LastInboundAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed bootstrap session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	bootstrapEvent := materializedAvailabilitySelectionEventForTest(
		"bootstrap-selection-inbound",
		"bootstrap-selection-projection",
		"bootstrap-availability-prompt",
		snapshot,
	)
	bootstrapMessage, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		IdempotencyKey:   "bootstrap-selection-inbound",
		Body:             "body is not a bootstrap input",
		ProcessingStatus: "PROCESSED",
		NormalizedPayload: map[string]interface{}{
			availabilitySelectionEventsV1MessageKey: []AvailabilitySelectionEventV1{bootstrapEvent},
		},
		ReceivedAt: now,
	})
	if err != nil {
		t.Fatalf("seed structured bootstrap event: %v", err)
	}
	for index := 0; index < 60; index++ {
		if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
			SessionID:        session.ID,
			Direction:        "OUTBOUND",
			Kind:             "TEXT",
			IdempotencyKey:   "bootstrap-filler-" + strconv.Itoa(index),
			Body:             "window filler",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatalf("seed bootstrap filler %d: %v", index, err)
		}
	}
	bootstrapped, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("bootstrap full structured session: %v", err)
	}
	if bootstrapped.AvailabilitySelectionState.Status != AvailabilitySelectionStatusBookable ||
		bootstrapped.AvailabilitySelectionState.SelectionEventMessageID != bootstrapMessage.ID ||
		!bootstrapped.AvailabilitySelectionState.BootstrapCompleted {
		t.Fatalf("full-session bootstrap lost old structured authority: %+v", bootstrapped.AvailabilitySelectionState)
	}

	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		IdempotencyKey:   "late-direct-rejection",
		ProcessingStatus: "PROCESSED",
		NormalizedPayload: map[string]interface{}{
			availabilitySelectionEventsV1MessageKey: []AvailabilitySelectionEventV1{{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         "late-direct-rejection",
				AvailabilityPromptSourceMessageID: bootstrapEvent.AvailabilityPromptSourceMessageID,
				RejectedWholeContext:              true,
			}},
		},
		ReceivedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed late direct structured event: %v", err)
	}
	reapplied, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("reapply bootstrapped state: %v", err)
	}
	if reapplied.AvailabilitySelectionState.Status != AvailabilitySelectionStatusRejected ||
		reapplied.AvailabilitySelectionState.Tombstone == nil ||
		len(reapplied.AvailabilitySelectionState.AppliedEventIDs) != 2 {
		t.Fatalf("canonical replay ignored a late durable event: %+v", reapplied.AvailabilitySelectionState)
	}
}

func TestAvailabilitySelectionStateV1UnitListRejectionDoesNotReopenBookingCreate(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback", Model: "gpt-test"},
	}
	booking := &fakeBookingCreator{enabled: true}
	svc := NewService(
		store,
		configForAvailabilitySelectionStateTest(),
		runner,
		booking,
	)
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-unit-list-rejection",
		CustomerPhone:  "availability-unit-list-rejection",
		CustomerName:   "Joao Vitor Messias",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed unit-list session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	prompt, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "availability-unit-list-prompt",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now,
	})
	if err != nil {
		t.Fatalf("seed unit-list prompt: %v", err)
	}

	materialized := ingestAndReprocessActivePromptFlowTurn(
		t,
		svc,
		session.ContactKey,
		"availability-unit-list-select",
		"1",
	)
	if materialized.Draft == nil {
		t.Fatal("expected materialized unit-list draft")
	}
	selectedSession, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload selected session: %v", err)
	}
	selectedState, ok := availabilitySelectionStateV1FromSession(selectedSession)
	if !ok || selectedState.Status != AvailabilitySelectionStatusBookable ||
		selectedState.AvailabilityPromptSourceMessageID != prompt.Message.ID ||
		selectedState.SelectionProjectionMessageID != materialized.Draft.ID {
		t.Fatalf("unit-list selection authority was not persisted: %+v", selectedState)
	}
	markSessionMessagesAutomationSent(t, store, session.ID)

	_ = ingestAndReprocessActivePromptFlowTurn(
		t,
		svc,
		session.ContactKey,
		"availability-unit-list-reject",
		"não quero a opção 1",
	)
	rejectedSession, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload rejected session: %v", err)
	}
	rejectedState, ok := availabilitySelectionStateV1FromSession(rejectedSession)
	if !ok || rejectedState.Status != AvailabilitySelectionStatusRejected ||
		rejectedState.Tombstone == nil ||
		rejectedState.Tombstone.AvailabilityPromptSourceMessageID != prompt.Message.ID {
		t.Fatalf("unit-list rejection did not persist its tombstone: %+v", rejectedState)
	}
	var rejectionEvents []AvailabilitySelectionEventV1
	for _, messageID := range store.messageOrder {
		message := store.messages[messageID]
		if message.SessionID == session.ID && message.Direction == "INBOUND" &&
			message.Body == "não quero a opção 1" {
			rejectionEvents = availabilitySelectionEventsV1FromMessage(message)
		}
	}
	if len(rejectionEvents) != 1 ||
		rejectionEvents[0].AvailabilityPromptSourceMessageID != prompt.Message.ID {
		t.Fatalf("simple inbound rejection did not persist the original prompt source: %+v", rejectionEvents)
	}
	projectionOnlyHistory := []Message{*materialized.Draft}
	bookingDraft := collectBookingDraftContextWithPassengerState(
		rejectedSession,
		projectionOnlyHistory,
		"",
		completePassengerStateForTest(1, 0),
	)
	if bookingDraft.HasBookableSelection ||
		bookingDraft.SelectedOptionIndex != 0 ||
		bookingDraft.TripID != "" ||
		bookingDraft.BoardStopID != "" ||
		bookingDraft.AlightStopID != "" {
		t.Fatalf("projection-only window reopened rejected authority: %+v", bookingDraft)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState(
		"quero reservar opção 1",
		rejectedState,
	); ok {
		t.Fatal("booking_create resolved a rejected unit-list selection")
	}
	if booking.calls != 0 {
		t.Fatalf("unit-list selection/rejection dispatched booking_create %d times", booking.calls)
	}
}

func TestAvailabilitySelectionStateV1CurrentTurnRequiresAppliedMaterialization(t *testing.T) {
	base := newFakeStoreWithPassengerAuthority()
	store := &droppingAvailabilitySelectionEventStore{fakeStore: base}
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "unsafe runner reply", Model: "gpt-test"},
	}
	booking := &fakeBookingCreator{enabled: true}
	svc := NewService(
		store,
		configForAvailabilitySelectionStateTest(),
		runner,
		booking,
	)
	now := availabilityTestObservedAt()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-unapplied-event",
		CustomerPhone:  "availability-selection-unapplied-event",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed unapplied-event session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	if _, err := base.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "availability-unapplied-event-prompt",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now,
	}); err != nil {
		t.Fatalf("seed unapplied-event prompt: %v", err)
	}

	out := ingestAndReprocessActivePromptFlowTurn(
		t,
		svc,
		session.ContactKey,
		"availability-unapplied-event-select",
		"1",
	)
	if out.Draft == nil {
		t.Fatal("expected fail-closed draft when materialization was not applied")
	}
	if snapshot := asMap(out.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
		t.Fatalf("unapplied materialization leaked a selected snapshot: %+v", snapshot)
	}
	reloaded, err := base.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload unapplied-event session: %v", err)
	}
	state, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || state.Status == AvailabilitySelectionStatusBookable {
		t.Fatalf("unapplied event became bookable: %+v ok=%t", state, ok)
	}
	if runner.calls != 0 || booking.calls != 0 {
		t.Fatalf(
			"unapplied materialization reached external work: runner=%d booking_create=%d",
			runner.calls,
			booking.calls,
		)
	}
}

func TestAvailabilitySelectionStateV1LegacyInvalidationBoundaryOrdersBootstrap(t *testing.T) {
	now := availabilityTestObservedAt()
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "post-boundary-trip"
	availabilityB.Results[0].BoardStopID = "post-boundary-board"
	availabilityB.Results[0].AlightStopID = "post-boundary-alight"
	snapshotA := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1)
	snapshotB := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1)

	for _, test := range []struct {
		name             string
		addPostBoundary  bool
		wantStatus       AvailabilitySelectionStatusV1
		wantSelectedTrip string
	}{
		{
			name:       "pre-boundary selection stays invalidated",
			wantStatus: AvailabilitySelectionStatusInvalidated,
		},
		{
			name:             "post-boundary materialization reopens bookable",
			addPostBoundary:  true,
			wantStatus:       AvailabilitySelectionStatusBookable,
			wantSelectedTrip: snapshotB.TripID,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:       "WHATSAPP",
				ContactKey:    "legacy-boundary-" + test.name,
				CustomerPhone: "legacy-boundary-" + test.name,
			})
			if err != nil {
				t.Fatalf("seed invalidated session: %v", err)
			}
			oldSelection := createAvailabilitySelectionInboundForTest(
				t,
				store,
				session.ID,
				"legacy-pre-boundary-selection-"+test.name,
				now,
			)
			persistAvailabilitySelectionEventOnFakeMessageForTest(
				t,
				store,
				oldSelection.ID,
				materializedAvailabilitySelectionEventForTest(
					oldSelection.ID,
					"legacy-pre-boundary-projection",
					"legacy-pre-boundary-prompt",
					snapshotA,
				),
			)
			boundary := createAvailabilitySelectionInboundForTest(
				t,
				store,
				session.ID,
				"legacy-invalidation-boundary-"+test.name,
				now.Add(time.Minute),
			)
			if test.addPostBoundary {
				postBoundary := createAvailabilitySelectionInboundForTest(
					t,
					store,
					session.ID,
					"legacy-post-boundary-selection-"+test.name,
					now.Add(2*time.Minute),
				)
				persistAvailabilitySelectionEventOnFakeMessageForTest(
					t,
					store,
					postBoundary.ID,
					materializedAvailabilitySelectionEventForTest(
						postBoundary.ID,
						"legacy-post-boundary-projection",
						"legacy-post-boundary-prompt",
						snapshotB,
					),
				)
			}
			session = store.sessions[session.ID]
			session.Metadata = map[string]interface{}{
				"agent": map[string]interface{}{
					canonicalAvailabilityFactsInvalidatedMetadataKey:               true,
					canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey: boundary.ID,
					canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey: boundary.CreatedAt.UTC().Format(time.RFC3339Nano),
				},
			}
			store.sessions[session.ID] = session

			applied, err := store.ApplyPassengerClarificationEventsV1(
				context.Background(),
				ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
			)
			if err != nil {
				t.Fatalf("bootstrap invalidated availability state: %v", err)
			}
			state := applied.AvailabilitySelectionState
			if state.Status != test.wantStatus || state.Snapshot.TripID != test.wantSelectedTrip {
				t.Fatalf(
					"boundary bootstrap status/trip=(%s,%q), want (%s,%q): %+v",
					state.Status,
					state.Snapshot.TripID,
					test.wantStatus,
					test.wantSelectedTrip,
					state,
				)
			}
		})
	}
}

func TestAvailabilitySelectionStateV1LegacyMarkerWithoutBoundaryFailsClosed(t *testing.T) {
	store := newFakeStore()
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "legacy-marker-without-boundary",
		CustomerPhone: "legacy-marker-without-boundary",
		Metadata: map[string]interface{}{
			"memory": map[string]interface{}{
				canonicalAvailabilityFactsInvalidatedMetadataKey: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("seed legacy marker session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	oldSelection := createAvailabilitySelectionInboundForTest(
		t,
		store,
		session.ID,
		"legacy-marker-old-selection",
		now,
	)
	persistAvailabilitySelectionEventOnFakeMessageForTest(
		t,
		store,
		oldSelection.ID,
		materializedAvailabilitySelectionEventForTest(
			oldSelection.ID,
			"legacy-marker-old-projection",
			"legacy-marker-old-prompt",
			snapshot,
		),
	)

	applied, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("bootstrap legacy marker: %v", err)
	}
	if applied.AvailabilitySelectionState.Status != AvailabilitySelectionStatusInvalidated ||
		applied.AvailabilitySelectionState.SelectedOptionIndex != 0 ||
		applied.AvailabilitySelectionState.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("legacy marker without boundary restored past authority: %+v", applied.AvailabilitySelectionState)
	}

	freshSelection := createAvailabilitySelectionInboundForTest(
		t,
		store,
		session.ID,
		"legacy-marker-fresh-explicit-selection",
		now.Add(time.Minute),
	)
	reopened, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					freshSelection.ID,
					"legacy-marker-fresh-projection",
					"legacy-marker-fresh-prompt",
					snapshot,
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("apply explicit materialization after legacy marker: %v", err)
	}
	if reopened.AvailabilitySelectionState.Status != AvailabilitySelectionStatusInvalidated ||
		reopened.AvailabilitySelectionState.SelectionEventMessageID != "" ||
		reopened.AvailabilitySelectionState.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("boundary-less legacy marker stopped failing closed after replay: %+v", reopened.AvailabilitySelectionState)
	}
}

func TestAvailabilitySelectionStateV1LegacyAuthorityRequiresExactStructuralSource(t *testing.T) {
	now := availabilityTestObservedAt()
	availability := availabilityOptionPromptFutureResultAt(now)
	promptA := legacyAvailabilityPromptMessageForStateTest(
		"legacy-prompt-a",
		availability,
		now,
	)
	promptB := legacyAvailabilityPromptMessageForStateTest(
		"legacy-prompt-b",
		availability,
		now.Add(time.Second),
	)
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"legacy-selection-projection",
		availability,
		IntentSelectAvailabilityOption,
		now.Add(2*time.Second),
	)

	t.Run("exact preceding source becomes bookable", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, selection})
		if state.Status != AvailabilitySelectionStatusBookable ||
			state.AvailabilityPromptSourceMessageID != promptA.ID ||
			state.SelectionProjectionMessageID != selection.ID ||
			state.SelectionEventMessageID != "legacy-selection-event-"+selection.ID {
			t.Fatalf("legacy selection did not resolve exact source without projection authority: %+v", state)
		}
	})

	t.Run("missing explicit selection identity stays none", func(t *testing.T) {
		identityMissing := legacyAvailabilitySelectionMessageForStateTest(
			"legacy-selection-missing-id",
			availability,
			IntentSelectAvailabilityOption,
			now.Add(2*time.Second),
		)
		delete(
			asMap(identityMissing.Payload[selectedAvailabilityResultPayloadKey]),
			selectedAvailabilitySelectionMessageIDPayloadKey,
		)
		delete(
			asMap(identityMissing.NormalizedPayload[selectedAvailabilityResultPayloadKey]),
			selectedAvailabilitySelectionMessageIDPayloadKey,
		)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, identityMissing})
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection without explicit identity became authoritative: %+v", state)
		}
	})

	t.Run("missing source stays none", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{selection})
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection without source became authoritative: %+v", state)
		}
	})

	t.Run("ambiguous source stays none", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, promptB, selection})
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection with ambiguous sources became authoritative: %+v", state)
		}
	})

	t.Run("selection projection is not a prompt source", func(t *testing.T) {
		projectionSource := legacyAvailabilitySelectionMessageForStateTest(
			"legacy-passenger-projection-source",
			availability,
			IntentPassengerCountReply,
			now.Add(time.Second),
		)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{projectionSource, selection},
		)
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection projection became a prompt source: %+v", state)
		}
	})

	for _, test := range []struct {
		name   string
		intent Intent
	}{
		{name: "passenger reply", intent: IntentPassengerCountReply},
		{name: "passenger documents", intent: IntentPassengerDocumentsProvided},
		{name: "booking continuation", intent: IntentDocumentConfirmation},
	} {
		t.Run(test.name+" projection never materializes", func(t *testing.T) {
			projection := legacyAvailabilitySelectionMessageForStateTest(
				"legacy-"+strings.ReplaceAll(test.name, " ", "-")+"-projection",
				availability,
				test.intent,
				now.Add(2*time.Second),
			)
			state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, projection})
			if state.Status != AvailabilitySelectionStatusNone {
				t.Fatalf("%s projection became authoritative: %+v", test.intent, state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState(
				"quero reservar opção 1",
				state,
			); ok {
				t.Fatal("booking_create opened without BOOKABLE authority")
			}
		})
	}

	t.Run("legacy rejection targets recovered source", func(t *testing.T) {
		rejection := availabilityAuthorityRejectionMessageForTest(
			"legacy-rejection",
			promptA.ID,
			1,
			now.Add(3*time.Second),
		)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{promptA, selection, rejection},
		)
		if state.Status != AvailabilitySelectionStatusRejected ||
			state.Tombstone == nil ||
			state.Tombstone.AvailabilityPromptSourceMessageID != promptA.ID {
			t.Fatalf("legacy rejection missed recovered selection source: %+v", state)
		}
	})

	t.Run("read consumers require persisted reconstructed state", func(t *testing.T) {
		history := []Message{promptA, selection}
		replayed := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
		if replayed.Status != AvailabilitySelectionStatusBookable {
			t.Fatalf("test replay did not reconstruct BOOKABLE authority: %+v", replayed)
		}
		if read := availabilitySelectionStateV1ForRead(Session{}, history); read.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("bounded history became read authority without persisted replay: %+v", read)
		}
		session := sessionWithAvailabilitySelectionStateForTest(Session{}, replayed)
		if read := availabilitySelectionStateV1ForRead(session, nil); read.Status != AvailabilitySelectionStatusBookable {
			t.Fatalf("persisted validated replay was not consumed: %+v", read)
		}
	})
}

func TestAvailabilitySelectionStateV1CausalOrderBeatsLockOrder(t *testing.T) {
	t.Run("S2 applied before S1 remains S2", func(t *testing.T) {
		store := newFakeStore()
		now := availabilityTestObservedAt()
		session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
			Channel:       "WHATSAPP",
			ContactKey:    "causal-selection-order",
			CustomerPhone: "causal-selection-order",
		})
		if err != nil {
			t.Fatalf("seed causal selection session: %v", err)
		}
		messageS1 := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-selection-s1",
			now,
		)
		messageS2 := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-selection-s2",
			now.Add(time.Minute),
		)
		availabilityS1 := availabilityOptionPromptFutureResultAt(now)
		availabilityS2 := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
		availabilityS2.Results[0].TripID = "causal-trip-s2"
		availabilityS2.Results[0].BoardStopID = "causal-board-s2"
		availabilityS2.Results[0].AlightStopID = "causal-alight-s2"
		snapshotS1 := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityS1, 1)
		snapshotS2 := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityS2, 1)

		if _, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						messageS2.ID,
						"causal-projection-s2",
						"causal-prompt-s2",
						snapshotS2,
					),
				},
			},
		); err != nil {
			t.Fatalf("apply S2 first: %v", err)
		}
		applied, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						messageS1.ID,
						"causal-projection-s1",
						"causal-prompt-s1",
						snapshotS1,
					),
				},
			},
		)
		if err != nil {
			t.Fatalf("apply S1 after S2: %v", err)
		}
		state := applied.AvailabilitySelectionState
		if state.Status != AvailabilitySelectionStatusBookable ||
			state.Snapshot.TripID != snapshotS2.TripID ||
			state.SelectionEventMessageID != messageS2.ID {
			t.Fatalf("lock order replaced newer S2 with older S1: %+v", state)
		}

		session = store.sessions[session.ID]
		metadata := cloneMap(session.Metadata)
		memory := cloneMap(asMap(metadata["memory"]))
		delete(memory, availabilitySelectionStateV1MemoryKey)
		metadata["memory"] = memory
		session.Metadata = metadata
		store.sessions[session.ID] = session
		restarted, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
		)
		if err != nil {
			t.Fatalf("bootstrap causal state after restart: %v", err)
		}
		if restarted.AvailabilitySelectionState.Status != state.Status ||
			restarted.AvailabilitySelectionState.Snapshot != state.Snapshot ||
			restarted.AvailabilitySelectionState.SelectionEventMessageID != state.SelectionEventMessageID {
			t.Fatalf(
				"restart/bootstrap drifted from live causal state: live=%+v restarted=%+v",
				state,
				restarted.AvailabilitySelectionState,
			)
		}
		liveJSON, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal live causal state: %v", err)
		}
		restartedJSON, err := json.Marshal(restarted.AvailabilitySelectionState)
		if err != nil {
			t.Fatalf("marshal restarted causal state: %v", err)
		}
		if string(restartedJSON) != string(liveJSON) {
			t.Fatalf(
				"restart/bootstrap state differs byte-for-byte: live=%s restarted=%s",
				liveJSON,
				restartedJSON,
			)
		}
	})

	t.Run("new rejection applied before old materialization remains rejected", func(t *testing.T) {
		store := newFakeStore()
		now := availabilityTestObservedAt()
		session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
			Channel:       "WHATSAPP",
			ContactKey:    "causal-rejection-order",
			CustomerPhone: "causal-rejection-order",
		})
		if err != nil {
			t.Fatalf("seed causal rejection session: %v", err)
		}
		oldSelection := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-old-selection",
			now,
		)
		newRejection := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-new-rejection",
			now.Add(time.Minute),
		)
		availability := availabilityOptionPromptFutureResultAt(now)
		snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
		promptSource := "causal-shared-prompt"

		if _, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
					Type:                              AvailabilitySelectionEventRejected,
					MessageID:                         newRejection.ID,
					AvailabilityPromptSourceMessageID: promptSource,
					RejectedOptionIndexes:             []int{1},
				}},
			},
		); err != nil {
			t.Fatalf("apply new rejection first: %v", err)
		}
		applied, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						oldSelection.ID,
						"causal-old-projection",
						promptSource,
						snapshot,
					),
				},
			},
		)
		if err != nil {
			t.Fatalf("apply old materialization after rejection: %v", err)
		}
		state := applied.AvailabilitySelectionState
		if state.Status != AvailabilitySelectionStatusRejected ||
			state.Tombstone == nil ||
			state.Tombstone.EventMessageID != newRejection.ID {
			t.Fatalf("old materialization reopened newer rejection: %+v", state)
		}
		if _, _, ok := resolveBookingCreateSelectionFromState(
			"quero reservar opção 1",
			state,
		); ok {
			t.Fatal("booking_create opened after causally newer rejection")
		}

		session = store.sessions[session.ID]
		metadata := cloneMap(session.Metadata)
		memory := cloneMap(asMap(metadata["memory"]))
		delete(memory, availabilitySelectionStateV1MemoryKey)
		metadata["memory"] = memory
		session.Metadata = metadata
		store.sessions[session.ID] = session
		restarted, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
		)
		if err != nil {
			t.Fatalf("bootstrap causal rejection state after restart: %v", err)
		}
		liveJSON, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal live causal rejection state: %v", err)
		}
		restartedJSON, err := json.Marshal(restarted.AvailabilitySelectionState)
		if err != nil {
			t.Fatalf("marshal restarted causal rejection state: %v", err)
		}
		if string(restartedJSON) != string(liveJSON) {
			t.Fatalf(
				"rejection restart/bootstrap state differs byte-for-byte: live=%s restarted=%s",
				liveJSON,
				restartedJSON,
			)
		}
	})
}

func TestAvailabilitySelectionReplayOrderPermutationProperty(t *testing.T) {
	permutations := []struct {
		name  string
		order []int
	}{
		{name: "materialization rejection projection", order: []int{0, 1, 2}},
		{name: "materialization projection rejection", order: []int{0, 2, 1}},
		{name: "rejection materialization projection", order: []int{1, 0, 2}},
		{name: "rejection projection materialization", order: []int{1, 2, 0}},
		{name: "projection materialization rejection", order: []int{2, 0, 1}},
		{name: "projection rejection materialization", order: []int{2, 1, 0}},
	}

	var referenceStateJSON string
	replayObservedAt := availabilityTestObservedAt()
	for _, permutation := range permutations {
		t.Run(permutation.name, func(t *testing.T) {
			store := newFakeStore()
			now := replayObservedAt
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:       "WHATSAPP",
				ContactKey:    "availability-replay-" + strings.ReplaceAll(permutation.name, " ", "-"),
				CustomerPhone: "availability-replay-" + strings.ReplaceAll(permutation.name, " ", "-"),
			})
			if err != nil {
				t.Fatalf("seed replay session: %v", err)
			}
			materializationMessage := seedAvailabilitySelectionReplayMessageForTest(
				store,
				session.ID,
				"00000000-0000-0000-0000-000000000101",
				"replay-old-materialization",
				now,
			)
			rejectionMessage := seedAvailabilitySelectionReplayMessageForTest(
				store,
				session.ID,
				"00000000-0000-0000-0000-000000000102",
				"replay-new-rejection",
				now.Add(time.Minute),
			)
			projectionMessage := seedAvailabilitySelectionReplayMessageForTest(
				store,
				session.ID,
				"00000000-0000-0000-0000-000000000103",
				"replay-new-projection",
				now.Add(2*time.Minute),
			)
			availability := availabilityOptionPromptFutureResultAt(now)
			snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
			inputs := []ApplyPassengerClarificationEventsV1Input{
				{
					SessionID: session.ID,
					AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
						materializedAvailabilitySelectionEventForTest(
							materializationMessage.ID,
							"replay-selection-projection-a",
							"replay-prompt-a",
							snapshot,
						),
					},
				},
				{
					SessionID: session.ID,
					AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
						Type:                              AvailabilitySelectionEventRejected,
						MessageID:                         rejectionMessage.ID,
						AvailabilityPromptSourceMessageID: "replay-prompt-b",
						RejectedOptionIndexes:             []int{1},
					}},
				},
				{
					SessionID: session.ID,
					AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
						Type:                              AvailabilitySelectionEventMaterialized,
						MessageID:                         projectionMessage.ID,
						ProjectionMessageID:               "replay-copied-projection-a",
						AvailabilityPromptSourceMessageID: "replay-prompt-a",
						SelectedOptionIndex:               snapshot.SelectedOptionIndex,
						Snapshot:                          snapshot,
						MaterializesAuthority:             false,
					}},
				},
			}

			var liveState AvailabilitySelectionStateV1
			for _, inputIndex := range permutation.order {
				applied, applyErr := store.ApplyPassengerClarificationEventsV1(
					context.Background(),
					inputs[inputIndex],
				)
				if applyErr != nil {
					t.Fatalf("apply replay event %d: %v", inputIndex, applyErr)
				}
				liveState = applied.AvailabilitySelectionState
				duplicate, duplicateErr := store.ApplyPassengerClarificationEventsV1(
					context.Background(),
					inputs[inputIndex],
				)
				if duplicateErr != nil {
					t.Fatalf("duplicate replay event %d: %v", inputIndex, duplicateErr)
				}
				if availabilitySelectionStateJSONForTest(t, duplicate.AvailabilitySelectionState) !=
					availabilitySelectionStateJSONForTest(t, liveState) {
					t.Fatalf(
						"duplicate event changed replay state: first=%+v duplicate=%+v",
						liveState,
						duplicate.AvailabilitySelectionState,
					)
				}
				liveState = duplicate.AvailabilitySelectionState
			}

			if liveState.Status != AvailabilitySelectionStatusBookable ||
				liveState.SelectionEventMessageID != materializationMessage.ID ||
				liveState.AvailabilityPromptSourceMessageID != "replay-prompt-a" ||
				liveState.Snapshot.TripID != snapshot.TripID ||
				len(liveState.AppliedEventIDs) != 3 {
				t.Fatalf("permuted replay produced wrong authority: %+v", liveState)
			}
			reloaded, err := store.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("reload replay session: %v", err)
			}
			reloadedState, ok := availabilitySelectionStateV1FromSession(reloaded)
			if !ok ||
				availabilitySelectionStateJSONForTest(t, reloadedState) !=
					availabilitySelectionStateJSONForTest(t, liveState) {
				t.Fatalf("reload differs from live replay: live=%+v reload=%+v", liveState, reloadedState)
			}

			reloaded.Metadata = cloneMap(reloaded.Metadata)
			memory := cloneMap(asMap(reloaded.Metadata["memory"]))
			delete(memory, availabilitySelectionStateV1MemoryKey)
			reloaded.Metadata["memory"] = memory
			store.sessions[reloaded.ID] = reloaded
			restarted, err := store.ApplyPassengerClarificationEventsV1(
				context.Background(),
				ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
			)
			if err != nil {
				t.Fatalf("restart canonical replay: %v", err)
			}
			restartedJSON := availabilitySelectionStateJSONForTest(
				t,
				restarted.AvailabilitySelectionState,
			)
			liveJSON := availabilitySelectionStateJSONForTest(t, liveState)
			if restartedJSON != liveJSON {
				t.Fatalf(
					"restart differs from live canonical replay: live=%s restart=%s",
					liveJSON,
					restartedJSON,
				)
			}
			if referenceStateJSON == "" {
				referenceStateJSON = liveJSON
			} else if liveJSON != referenceStateJSON {
				t.Fatalf(
					"arrival permutation changed canonical replay: reference=%s current=%s",
					referenceStateJSON,
					liveJSON,
				)
			}
		})
	}
}

func availabilitySelectionStateJSONForTest(
	t *testing.T,
	state AvailabilitySelectionStateV1,
) string {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal availability selection state: %v", err)
	}
	return string(raw)
}

func seedAvailabilitySelectionReplayMessageForTest(
	store *fakeStore,
	sessionID string,
	messageID string,
	body string,
	recordedAt time.Time,
) Message {
	message := Message{
		ID:               messageID,
		SessionID:        sessionID,
		Direction:        "INBOUND",
		Body:             body,
		ProcessingStatus: "BUFFERED_PENDING",
		ReceivedAt:       recordedAt,
		CreatedAt:        recordedAt,
	}
	store.messages[message.ID] = message
	store.messageOrder = append(store.messageOrder, message.ID)
	return message
}

func configForAvailabilitySelectionStateTest() config.Config {
	return config.Config{ChatDebounceWindowMS: 1500}
}

type droppingAvailabilitySelectionEventStore struct {
	*fakeStore
}

func (store *droppingAvailabilitySelectionEventStore) ApplyPassengerClarificationEventsV1(
	ctx context.Context,
	input ApplyPassengerClarificationEventsV1Input,
) (ApplyPassengerClarificationEventsV1Result, error) {
	input.AvailabilitySelectionEvents = nil
	return store.fakeStore.ApplyPassengerClarificationEventsV1(ctx, input)
}

type serializedAvailabilitySelectionFakeStore struct {
	*fakeStore
	mu sync.Mutex
}

func (store *serializedAvailabilitySelectionFakeStore) ApplyPassengerClarificationEventsV1(
	ctx context.Context,
	input ApplyPassengerClarificationEventsV1Input,
) (ApplyPassengerClarificationEventsV1Result, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.fakeStore.ApplyPassengerClarificationEventsV1(ctx, input)
}

func (store *serializedAvailabilitySelectionFakeStore) GetSession(
	ctx context.Context,
	sessionID string,
) (Session, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.fakeStore.GetSession(ctx, sessionID)
}

func mustAvailabilitySelectionSnapshotV1ForTest(
	t *testing.T,
	availability *AvailabilitySearchResult,
	index int,
) AvailabilitySelectionSnapshotV1 {
	t.Helper()
	snapshot, ok := availabilitySelectionSnapshotV1FromAvailability(availability, index)
	if !ok {
		t.Fatalf("build availability selection snapshot index %d: %+v", index, availability)
	}
	return snapshot
}

func materializedAvailabilitySelectionEventForTest(
	messageID string,
	projectionMessageID string,
	promptSourceMessageID string,
	snapshot AvailabilitySelectionSnapshotV1,
) AvailabilitySelectionEventV1 {
	recordedAt := availabilityTestObservedAt()
	return AvailabilitySelectionEventV1{
		Type:                              AvailabilitySelectionEventMaterialized,
		MessageID:                         messageID,
		ProjectionMessageID:               projectionMessageID,
		AvailabilityPromptSourceMessageID: promptSourceMessageID,
		SelectedOptionIndex:               snapshot.SelectedOptionIndex,
		Snapshot:                          snapshot,
		MaterializesAuthority:             true,
		Order: AvailabilitySelectionEventOrderV1{
			ReceivedAt: recordedAt,
			CreatedAt:  recordedAt,
			MessageID:  messageID,
		},
	}
}

func orderedAvailabilitySelectionEventsForTest(
	events ...AvailabilitySelectionEventV1,
) []AvailabilitySelectionEventV1 {
	return orderedAvailabilitySelectionEventsForTestFrom(0, events...)
}

func orderedAvailabilitySelectionEventsForTestFrom(
	start int,
	events ...AvailabilitySelectionEventV1,
) []AvailabilitySelectionEventV1 {
	base := availabilityTestObservedAt().Add(-24 * time.Hour)
	ordered := append([]AvailabilitySelectionEventV1(nil), events...)
	for index := range ordered {
		recordedAt := base.Add(time.Duration(start+index) * time.Minute)
		ordered[index].Order = AvailabilitySelectionEventOrderV1{
			ReceivedAt: recordedAt,
			CreatedAt:  recordedAt,
			MessageID:  strings.TrimSpace(ordered[index].MessageID),
		}
	}
	return ordered
}

func createAvailabilitySelectionInboundForTest(
	t *testing.T,
	store *fakeStore,
	sessionID string,
	body string,
	at time.Time,
) Message {
	t.Helper()
	message, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		IdempotencyKey:   "availability-selection-" + body,
		Body:             body,
		ProcessingStatus: "BUFFERED_PENDING",
		ReceivedAt:       at,
	})
	if err != nil {
		t.Fatalf("create availability selection inbound: %v", err)
	}
	return message
}

func persistAvailabilitySelectionEventOnFakeMessageForTest(
	t *testing.T,
	store *fakeStore,
	messageID string,
	event AvailabilitySelectionEventV1,
) {
	t.Helper()
	message, ok := store.messages[messageID]
	if !ok {
		t.Fatalf("availability event message %q not found", messageID)
	}
	message.NormalizedPayload = cloneMap(message.NormalizedPayload)
	message.NormalizedPayload[availabilitySelectionEventsV1MessageKey] = []AvailabilitySelectionEventV1{event}
	store.messages[messageID] = message
}

func legacyAvailabilityPromptMessageForStateTest(
	messageID string,
	availability AvailabilitySearchResult,
	at time.Time,
) Message {
	return Message{
		ID:               messageID,
		Direction:        "OUTBOUND",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		ReceivedAt: at,
		CreatedAt:  at,
	}
}

func legacyAvailabilitySelectionMessageForStateTest(
	messageID string,
	availability AvailabilitySearchResult,
	intent Intent,
	at time.Time,
) Message {
	payload := persistedAvailabilitySelectionPayloadForTest(availability, 1)
	payload["intent"] = string(intent)
	snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
	snapshot[selectedAvailabilitySelectionMessageIDPayloadKey] = "legacy-selection-event-" + messageID
	delete(snapshot, availabilityPromptSourceMessageIDPayloadKey)
	delete(snapshot, availabilitySelectionMaterializesAuthorityPayloadKey)
	return Message{
		ID:                messageID,
		Direction:         "OUTBOUND",
		ProcessingStatus:  messageStatusAutomationSent,
		Payload:           payload,
		NormalizedPayload: cloneMap(payload),
		ReceivedAt:        at,
		CreatedAt:         at,
	}
}
