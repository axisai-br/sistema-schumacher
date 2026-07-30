package chat

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func markAvailabilityPromptDeliveredForTest(message Message) Message {
	if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
		return message
	}
	if !DeliveryStatusConfirmsOutboundV1(message.ProcessingStatus) {
		return message
	}
	at := message.ReceivedAt
	if at.IsZero() {
		at = message.CreatedAt
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	message.NormalizedPayload = cloneMap(message.NormalizedPayload)
	message.NormalizedPayload["delivery_recorded_at"] = at.UTC().Format(time.RFC3339Nano)
	return message
}

func markAvailabilityPromptHistoryDeliveredForTest(history []Message) []Message {
	history = append([]Message(nil), history...)
	for index := range history {
		history[index] = markAvailabilityPromptDeliveredForTest(history[index])
	}
	return history
}

func collectBookingDraftContext(session Session, history []Message, currentTurn string) BookingDraftContext {
	session = materializePersistedAvailabilitySelectionForTest(session, history)
	state := explicitPassengerClarificationStateForTest(session, history)
	return collectBookingDraftContextWithPassengerState(session, history, currentTurn, state)
}

func mergePassengerReplyIntoBookingDraft(context BookingDraftContext, passengerCount int, childUnder5Count int) BookingDraftContext {
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{
		{
			Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
			MessageID: "booking-draft-merge-passenger", Value: passengerCount, ValueKnown: passengerCount > 0,
			PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
		},
		{
			Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
			MessageID: "booking-draft-merge-child", Value: childUnder5Count, ValueKnown: childUnder5Count >= 0,
		},
	})
	return applyPassengerClarificationStateToBookingDraft(context, state)
}

func parseBookingCreateInput(
	session Session,
	history []Message,
	text string,
	currentAvailability *AvailabilitySearchResult,
) (BookingCreateInput, bool) {
	session = materializePersistedAvailabilitySelectionForTest(session, history)
	state := explicitPassengerClarificationStateForTest(session, history)
	return parseBookingCreateInputWithPassengerState(session, history, text, currentAvailability, state)
}

func materializePersistedAvailabilitySelectionForTest(
	session Session,
	history []Message,
) Session {
	if _, present := availabilitySelectionStateV1ValueFromSession(session); present {
		return session
	}
	state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
	state.BootstrapCompleted = true
	return sessionWithAvailabilitySelectionStateForTest(session, state)
}

func sessionWithAvailabilitySelectionStateForTest(
	session Session,
	state AvailabilitySelectionStateV1,
) Session {
	metadata := cloneMap(session.Metadata)
	memory := cloneMap(asMap(metadata["memory"]))
	memory[availabilitySelectionStateV1MemoryKey] = state
	metadata["memory"] = memory
	session.Metadata = metadata
	return session
}

func bootstrapAvailabilitySelectionStateFromHistoryForTest(
	history []Message,
) AvailabilitySelectionStateV1 {
	return bootstrapAvailabilitySelectionStateV1(
		nil,
		availabilitySelectionStructuredEventsV1(history),
	)
}

func sessionWithReplayedAvailabilitySelectionForTest(
	session Session,
	history []Message,
) Session {
	state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
	state.BootstrapCompleted = true
	return sessionWithAvailabilitySelectionStateForTest(session, state)
}

type availabilitySelectionProjectionAuthorityForTest struct {
	SelectionMessageID    string
	PromptSourceMessageID string
	MaterializesAuthority bool
}

func persistedAvailabilitySelectionPayloadForTest(
	availability AvailabilitySearchResult,
	index int,
	authority availabilitySelectionProjectionAuthorityForTest,
) map[string]interface{} {
	snapshot := selectedAvailabilityResultPayloadFromAvailability(&availability, index)
	selectionMessageID := strings.TrimSpace(authority.SelectionMessageID)
	promptSourceMessageID := strings.TrimSpace(authority.PromptSourceMessageID)
	if authority.MaterializesAuthority &&
		(selectionMessageID == "" || promptSourceMessageID == "") {
		panic("authoritative availability projection fixture requires explicit selection and prompt source IDs")
	}
	if selectionMessageID != "" {
		snapshot[selectedAvailabilitySelectionMessageIDPayloadKey] = selectionMessageID
	}
	if promptSourceMessageID != "" {
		snapshot[availabilityPromptSourceMessageIDPayloadKey] = promptSourceMessageID
	}
	if authority.MaterializesAuthority {
		snapshot[availabilitySelectionMaterializesAuthorityPayloadKey] = true
	}
	return map[string]interface{}{
		"intent":                             string(IntentSelectAvailabilityOption),
		"template_name":                      string(TemplateAskPassengerCount),
		"selected_option_index":              index,
		selectedAvailabilityResultPayloadKey: snapshot,
		"tool_context": map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
		},
	}
}

func canonicalAvailabilitySelectionHistoryForTest(
	history []Message,
	availability AvailabilitySearchResult,
	index int,
	promptIndex int,
	selectionIndex int,
	projectionIndex int,
) []Message {
	if promptIndex < 0 || promptIndex >= len(history) ||
		selectionIndex < 0 || selectionIndex >= len(history) ||
		projectionIndex < 0 || projectionIndex >= len(history) ||
		!(promptIndex < selectionIndex && selectionIndex < projectionIndex) {
		panic("invalid canonical availability fixture indexes")
	}
	snapshot, ok := availabilitySelectionSnapshotV1FromAvailability(&availability, index)
	if !ok {
		panic("invalid canonical availability fixture snapshot")
	}
	history = append([]Message(nil), history...)
	identity := strings.TrimSpace(snapshot.TripID) + "-" + strconv.Itoa(index)
	prompt := history[promptIndex]
	prompt.ID = firstNonEmpty(strings.TrimSpace(prompt.ID), "test-availability-prompt-"+identity)
	prompt.Payload = cloneMap(prompt.Payload)
	prompt.NormalizedPayload = cloneMap(prompt.NormalizedPayload)
	for _, payload := range []map[string]interface{}{prompt.Payload, prompt.NormalizedPayload} {
		payload["intent"] = string(IntentAvailabilitySearch)
		payload["template_name"] = string(TemplateAvailabilityList)
		toolContext := cloneMap(asMap(payload["tool_context"]))
		if len(asMap(toolContext[toolNameAvailabilitySearch])) == 0 {
			toolContext[toolNameAvailabilitySearch] = buildAvailabilityToolResponsePayload(availability)
		}
		payload["tool_context"] = toolContext
	}
	prompt = markAvailabilityPromptDeliveredForTest(prompt)
	history[promptIndex] = prompt

	selection := history[selectionIndex]
	selection.ID = firstNonEmpty(strings.TrimSpace(selection.ID), "test-selection-inbound-"+identity)
	selection.Direction = "INBOUND"
	selection.NormalizedPayload = cloneMap(selection.NormalizedPayload)

	projection := history[projectionIndex]
	projection.ID = firstNonEmpty(strings.TrimSpace(projection.ID), "test-selection-projection-"+identity)
	projectionPayload := persistedAvailabilitySelectionPayloadForTest(
		availability,
		index,
		availabilitySelectionProjectionAuthorityForTest{
			SelectionMessageID:    selection.ID,
			PromptSourceMessageID: prompt.ID,
		},
	)
	projection.Payload = cloneMap(projection.Payload)
	projection.NormalizedPayload = cloneMap(projection.NormalizedPayload)
	for key, value := range projectionPayload {
		projection.Payload[key] = value
		projection.NormalizedPayload[key] = value
	}
	projection = markAvailabilityPromptDeliveredForTest(projection)

	event := materializedAvailabilitySelectionEventForTest(
		selection.ID,
		projection.ID,
		prompt.ID,
		snapshot,
	)
	event.EventID = ""
	event.Order = AvailabilitySelectionEventOrderV1{}
	selection.NormalizedPayload[availabilitySelectionEventsV1MessageKey] =
		[]AvailabilitySelectionEventV1{event}

	history[selectionIndex] = selection
	history[projectionIndex] = projection
	return history
}

func appendCanonicalAvailabilitySelectionForTest(
	history []Message,
	availability AvailabilitySearchResult,
	index int,
	promptAt time.Time,
) []Message {
	promptIndex := len(history)
	fixtureID := strconv.Itoa(promptIndex) + "-" + strconv.Itoa(index)
	sessionID := availabilitySelectionFixtureSessionIDForTest(history)
	history = append(history,
		Message{
			ID:               "test-availability-prompt-" + fixtureID,
			SessionID:        sessionID,
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(availability),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       promptAt,
			CreatedAt:        promptAt,
		},
	)
	return appendCanonicalAvailabilitySelectionForPromptForTest(
		history,
		availability,
		index,
		promptIndex,
		promptAt.Add(time.Second),
	)
}

func appendCanonicalAvailabilitySelectionForPromptForTest(
	history []Message,
	availability AvailabilitySearchResult,
	index int,
	promptIndex int,
	selectionAt time.Time,
) []Message {
	selectionIndex := len(history)
	fixtureID := strconv.Itoa(selectionIndex) + "-" + strconv.Itoa(index)
	sessionID := ""
	if promptIndex >= 0 && promptIndex < len(history) {
		sessionID = strings.TrimSpace(history[promptIndex].SessionID)
	}
	if sessionID == "" {
		sessionID = availabilitySelectionFixtureSessionIDForTest(history)
	}
	history = append(history,
		Message{
			ID:               "test-selection-inbound-" + fixtureID,
			SessionID:        sessionID,
			Direction:        "INBOUND",
			Body:             "opcao " + strconv.Itoa(index),
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       selectionAt,
			CreatedAt:        selectionAt,
		},
		Message{
			ID:               "test-selection-projection-" + fixtureID,
			SessionID:        sessionID,
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       selectionAt.Add(time.Second),
			CreatedAt:        selectionAt.Add(time.Second),
		},
	)
	return canonicalAvailabilitySelectionHistoryForTest(
		history,
		availability,
		index,
		promptIndex,
		selectionIndex,
		selectionIndex+1,
	)
}

func availabilitySelectionFixtureSessionIDForTest(history []Message) string {
	for index := len(history) - 1; index >= 0; index-- {
		if sessionID := strings.TrimSpace(history[index].SessionID); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

type seededCanonicalAvailabilitySelectionForTest struct {
	Prompt     Message
	Selection  Message
	Projection Message
	State      AvailabilitySelectionStateV1
	Session    Session
}

func seedCanonicalAvailabilitySelectionForTest(
	t *testing.T,
	store *fakeStore,
	sessionID string,
	availability AvailabilitySearchResult,
	index int,
	promptAt time.Time,
	projectionBody string,
) seededCanonicalAvailabilitySelectionForTest {
	t.Helper()
	if strings.TrimSpace(projectionBody) == "" {
		projectionBody = askPassengerCountReply
	}
	promptPayload := map[string]interface{}{
		"intent":        string(IntentAvailabilitySearch),
		"template_name": string(TemplateAvailabilityList),
		"tool_context": map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
		},
	}
	promptNormalizedPayload := cloneMap(promptPayload)
	promptNormalizedPayload["delivery_recorded_at"] = promptAt.UTC().Format(time.RFC3339Nano)
	prompt, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         sessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              buildAvailabilityListReply(availability),
		ProcessingStatus:  messageStatusAutomationSent,
		Payload:           cloneMap(promptPayload),
		NormalizedPayload: promptNormalizedPayload,
		ReceivedAt:        promptAt,
	})
	if err != nil {
		t.Fatalf("seed canonical availability prompt: %v", err)
	}
	selection, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		Body:             "opcao " + strconv.Itoa(index),
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       promptAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("seed canonical availability selection inbound: %v", err)
	}
	projectionPayload := persistedAvailabilitySelectionPayloadForTest(
		availability,
		index,
		availabilitySelectionProjectionAuthorityForTest{
			SelectionMessageID:    selection.ID,
			PromptSourceMessageID: prompt.ID,
		},
	)
	projectionNormalizedPayload := cloneMap(projectionPayload)
	projectionNormalizedPayload["delivery_recorded_at"] = promptAt.Add(2 * time.Minute).UTC().Format(time.RFC3339Nano)
	projection, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         sessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              projectionBody,
		ProcessingStatus:  messageStatusAutomationSent,
		Payload:           cloneMap(projectionPayload),
		NormalizedPayload: projectionNormalizedPayload,
		ReceivedAt:        promptAt.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed canonical availability projection: %v", err)
	}
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, index)
	applied, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: sessionID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					selection.ID,
					projection.ID,
					prompt.ID,
					snapshot,
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("persist canonical availability selection: %v", err)
	}
	state := applied.AvailabilitySelectionState
	if state.Status != AvailabilitySelectionStatusBookable ||
		state.SelectionEventMessageID != selection.ID ||
		state.SelectionProjectionMessageID != projection.ID ||
		state.AvailabilityPromptSourceMessageID != prompt.ID {
		t.Fatalf("canonical availability fixture is not BOOKABLE: %+v", state)
	}
	return seededCanonicalAvailabilitySelectionForTest{
		Prompt:     prompt,
		Selection:  selection,
		Projection: projection,
		State:      state,
		Session:    applied.Session,
	}
}

func selectedAvailabilitySnapshotWithoutAuthorityIDsForTest(
	t *testing.T,
	snapshot map[string]interface{},
	selectionMessageID string,
) map[string]interface{} {
	t.Helper()
	if got := strings.TrimSpace(asString(snapshot[selectedAvailabilitySelectionMessageIDPayloadKey])); got != strings.TrimSpace(selectionMessageID) {
		t.Fatalf("selection_message_id=%q, want %q: %+v", got, selectionMessageID, snapshot)
	}
	if got := strings.TrimSpace(asString(snapshot[availabilityPromptSourceMessageIDPayloadKey])); got == "" {
		t.Fatalf("availability_prompt_source_message_id must be persisted: %+v", snapshot)
	}
	withoutIDs := cloneMap(snapshot)
	delete(withoutIDs, selectedAvailabilitySelectionMessageIDPayloadKey)
	delete(withoutIDs, availabilityPromptSourceMessageIDPayloadKey)
	delete(withoutIDs, availabilitySelectionMaterializesAuthorityPayloadKey)
	return withoutIDs
}

func parseBookingCreateFromDocumentConfirmation(
	session Session,
	history []Message,
	currentTurn string,
) (BookingCreateInput, bool) {
	session = materializePersistedAvailabilitySelectionForTest(session, history)
	state := explicitPassengerClarificationStateForTest(session, history)
	return parseBookingCreateFromDocumentConfirmationWithPassengerState(session, history, currentTurn, state)
}

func explicitPassengerClarificationStateForTest(session Session, history []Message) PassengerClarificationStateV1 {
	if state, ok := passengerClarificationStateV1FromSession(session); ok {
		return state
	}
	events := passengerClarificationStructuredEventsV1(history)
	state := bootstrapPassengerClarificationStateV1(nil, nil, events)
	state.BootstrapCompleted = true
	return state
}

func unknownPassengerStateForTest() PassengerClarificationStateV1 {
	state := newPassengerClarificationStateV1()
	state.BootstrapCompleted = true
	return state
}

func sessionWithPassengerClarificationStateForTest(session Session, state PassengerClarificationStateV1) Session {
	metadata := cloneMap(session.Metadata)
	memory := cloneMap(asMap(metadata["memory"]))
	memory[passengerClarificationStateV1MemoryKey] = state
	metadata["memory"] = memory
	session.Metadata = metadata
	return session
}

func seedDeliveredPassengerPromptForTest(
	t *testing.T,
	store *fakeStore,
	sessionID string,
	at time.Time,
) PassengerClarificationStateV1 {
	t.Helper()
	prompt, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       at,
	})
	if err != nil {
		t.Fatalf("create delivered passenger prompt: %v", err)
	}
	event := PassengerClarificationEventV1{
		Type:      PassengerClarificationEventPassengerPromptOpened,
		Slot:      PassengerClarificationSlotPassenger,
		MessageID: prompt.ID,
	}
	if _, err := store.UpdateMessage(context.Background(), UpdateMessageInput{
		MessageID:        prompt.ID,
		ProcessingStatus: messageStatusAutomationSent,
		NormalizedPayload: map[string]interface{}{
			"delivery_recorded_at":           at.UTC().Format(time.RFC3339Nano),
			passengerPromptEventV1MessageKey: event,
		},
	}); err != nil {
		t.Fatalf("record delivered passenger prompt: %v", err)
	}
	applied, err := store.ApplyPassengerClarificationEventsV1(context.Background(), ApplyPassengerClarificationEventsV1Input{
		SessionID: sessionID,
		Events:    []PassengerClarificationEventV1{event},
	})
	if err != nil {
		t.Fatalf("apply delivered passenger prompt: %v", err)
	}
	if !applied.State.HasEvidence || applied.State.PassengerPromptMessageID != prompt.ID ||
		applied.State.PassengerCountKnown || applied.State.ChildUnder5CountKnown {
		t.Fatalf("expected active passenger context with unknown slots, got %+v", applied.State)
	}
	return applied.State
}
