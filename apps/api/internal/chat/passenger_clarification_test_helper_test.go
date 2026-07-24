package chat

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

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
	session = materializeExplicitAvailabilitySelectionForTest(
		session,
		history,
		text,
		currentAvailability,
	)
	session = materializePersistedAvailabilitySelectionForTest(session, history)
	state := explicitPassengerClarificationStateForTest(session, history)
	return parseBookingCreateInputWithPassengerState(session, history, text, currentAvailability, state)
}

func materializeExplicitAvailabilitySelectionForTest(
	session Session,
	history []Message,
	text string,
	currentAvailability *AvailabilitySearchResult,
) Session {
	if _, present := availabilitySelectionStateV1ValueFromSession(session); present {
		return session
	}
	index := extractSelectedOptionIndex(text)
	if index <= 0 || looksLikeNegatedAvailabilitySelection(
		strings.Join(strings.Fields(foldChatText(text)), " "),
	) {
		return session
	}
	availability := currentAvailability
	sourceHistoryIndex := len(history)
	promptSourceMessageID := "test-current-availability"
	rejectionPromptSourceMessageID := promptSourceMessageID
	if availability == nil {
		previous, historyIndex, ok := latestVisibleAvailabilitySelectionContextWithSource(history)
		if !ok {
			return session
		}
		availability = previous
		sourceHistoryIndex = historyIndex
		promptSourceMessageID = availabilityPromptSourceMessageIDAtHistoryIndex(history, historyIndex)
		rejectionPromptSourceMessageID = promptSourceMessageID
		if promptSourceMessageID == "" {
			promptSourceMessageID = "test-history-prompt"
		}
	}
	selected, ok := selectedAvailabilityItemForMaterialization(availability, index)
	if !ok {
		return session
	}
	evidence := latestAvailabilitySelectionEvidence(history)
	if evidence.blocksHistoryIndex(sourceHistoryIndex) ||
		evidence.rejectsAvailabilityOptionForPrompt(
			rejectionPromptSourceMessageID,
			sourceHistoryIndex,
			index,
			selected.TripDate,
		) {
		return session
	}
	snapshot, ok := availabilitySelectionSnapshotV1FromAvailability(availability, index)
	if !ok {
		return session
	}
	selectionState := ReduceAvailabilitySelectionEventsV1(newAvailabilitySelectionStateV1(), []AvailabilitySelectionEventV1{
		materializedAvailabilitySelectionEventForTest(
			"test-selection-event",
			"test-selection-projection",
			promptSourceMessageID,
			snapshot,
		),
	})
	selectionState.BootstrapCompleted = true
	return sessionWithAvailabilitySelectionStateForTest(session, selectionState)
}

func materializePersistedAvailabilitySelectionForTest(
	session Session,
	history []Message,
) Session {
	if _, present := availabilitySelectionStateV1ValueFromSession(session); present {
		return session
	}
	evidence := latestAvailabilitySelectionEvidence(history)
	if !evidence.bookable() ||
		!evidence.MaterializesAuthority ||
		strings.TrimSpace(evidence.SelectionMessageID) == "" ||
		strings.TrimSpace(evidence.AvailabilityPromptSourceMessageID) == "" {
		return session
	}
	for index := len(history) - 1; index >= 0; index-- {
		messageEvidence := messageAvailabilitySelectionEvidence(history[index])
		if !messageEvidence.bookable() ||
			messageEvidence.SelectionMessageID != evidence.SelectionMessageID {
			continue
		}
		snapshot, ok := availabilitySelectionSnapshotV1FromPayload(
			selectedAvailabilityResultFromMessage(history[index]),
		)
		if !ok {
			return session
		}
		projectionMessageID := strings.TrimSpace(history[index].ID)
		if projectionMessageID == "" {
			projectionMessageID = evidence.SelectionMessageID
		}
		state := ReduceAvailabilitySelectionEventsV1(
			newAvailabilitySelectionStateV1(),
			[]AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					evidence.SelectionMessageID,
					projectionMessageID,
					evidence.AvailabilityPromptSourceMessageID,
					snapshot,
				),
			},
		)
		state.BootstrapCompleted = true
		return sessionWithAvailabilitySelectionStateForTest(session, state)
	}
	return session
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

func persistedAvailabilitySelectionPayloadForTest(availability AvailabilitySearchResult, index int) map[string]interface{} {
	snapshot := selectedAvailabilityResultPayloadFromAvailability(&availability, index)
	selectionIdentity := strings.TrimSpace(asString(snapshot["trip_id"])) + "-" + strconv.Itoa(index)
	snapshot[selectedAvailabilitySelectionMessageIDPayloadKey] = "test-selection-event-" + selectionIdentity
	snapshot[availabilityPromptSourceMessageIDPayloadKey] = "test-availability-prompt-" + selectionIdentity
	snapshot[availabilitySelectionMaterializesAuthorityPayloadKey] = true
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
