package chat

func collectBookingDraftContext(session Session, history []Message, currentTurn string) BookingDraftContext {
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
	state := explicitPassengerClarificationStateForTest(session, history)
	return parseBookingCreateInputWithPassengerState(session, history, text, currentAvailability, state)
}

func parseBookingCreateFromDocumentConfirmation(
	session Session,
	history []Message,
	currentTurn string,
) (BookingCreateInput, bool) {
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
