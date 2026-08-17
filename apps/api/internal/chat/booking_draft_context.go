package chat

import (
	"fmt"
	"strings"
)

const (
	selectedAvailabilityResultPayloadKey             = "selected_availability_result"
	selectedAvailabilitySelectionMessageIDPayloadKey = "selection_message_id"
	availabilityPromptSourceMessageIDPayloadKey      = "availability_prompt_source_message_id"
)

type BookingNextAction string

const (
	BookingNextCallCreate                  BookingNextAction = "call_create"
	BookingNextAwaitTripSelection          BookingNextAction = "await_trip_selection"
	BookingNextAskPassengerClarification   BookingNextAction = "ask_passenger_clarification"
	BookingNextAskPassengerDocuments       BookingNextAction = "ask_passenger_documents"
	BookingNextAskLapChildAssignment       BookingNextAction = "ask_lap_child_assignment"
	BookingNextAskBookingPaymentPreference BookingNextAction = "ask_booking_payment_preference"
)

type BookingDraftContext struct {
	Origin                            string
	Destination                       string
	PackageName                       string
	SelectionMessageID                string
	AvailabilityPromptSourceMessageID string
	SelectedOptionIndex               int
	TripID                            string
	BoardStopID                       string
	AlightStopID                      string
	TripDate                          string
	DepartureTime                     string
	Price                             float64
	Currency                          string
	PassengerCount                    int
	PassengerCountProvenance          PassengerCountProvenance
	ExpectedDocumentCount             int
	ChildUnder5Count                  int
	PassengerCountKnown               bool
	PassengerCountConflicting         bool
	ChildUnder5CountKnown             bool
	ChildUnder5AddsTraveler           bool
	PassengerSlotStatus               PassengerClarificationSlotStatusV1
	ChildSlotStatus                   PassengerClarificationSlotStatusV1
	PassengerPromptMessageID          string
	ChildPromptMessageID              string
	PassengerReasonCodes              []string
	ChildReasonCodes                  []string
	ChildUnder5AddsTravelerOrigin     PassengerClarificationAddsTravelerOriginV1
	ChildReferences                   []PassengerClarificationChildReferenceV1
	LapChildAssignmentKnown           bool
	LapChildPassengerIndexes          []int
	NeedsLapChildAssignment           bool
	HasPassengerDetails               bool
	PassengerDetailsCount             int
	PassengerDetailsText              string
	PassengerDetails                  []BookingCreatePassengerInput
	PassengerSnapshot                 bookingPassengerSnapshot
	PartialPassengerDetails           []BookingPassengerDocumentPartial
	PartialPassengerDetailsCount      int
	HasAvailabilityShown              bool
	HasBookableSelection              bool
	AskedPassengerQuestion            bool
	PassengerCountContextActive       bool
	RequestedPassengerDocuments       bool
	BookingCreated                    bool
	AskedPaymentPreference            bool
}

type availabilitySelectionStatus string

const (
	availabilitySelectionNone                availabilitySelectionStatus = "none"
	availabilitySelectionBlockedMetadataOnly availabilitySelectionStatus = "blocked_metadata_only"
	availabilitySelectionIncomplete          availabilitySelectionStatus = "incomplete"
	availabilitySelectionRejected            availabilitySelectionStatus = "rejected_by_user"
	availabilitySelectionBookable            availabilitySelectionStatus = "bookable"
)

type availabilitySelectionEvidence struct {
	Status                            availabilitySelectionStatus
	SourceHistoryIndex                int
	SelectionMessageID                string
	RejectionMessageID                string
	AvailabilityPromptSourceMessageID string
	MaterializesAuthority             bool
	SelectedOptionIndex               int
	RejectedOptionIndexes             []int
	RejectedTripDates                 []string
	RejectedWholeContext              bool
	LaterRejections                   []availabilitySelectionRejection
	TripID                            string
	BoardStopID                       string
	AlightStopID                      string
	OriginDisplayName                 string
	DestinationDisplayName            string
	PackageName                       string
	TripDate                          string
	DepartureTime                     string
	Price                             float64
	Currency                          string
}

type availabilitySelectionRejection struct {
	SourceHistoryIndex                int
	RejectionMessageID                string
	AvailabilityPromptSourceMessageID string
	OptionIndexes                     []int
	TripDates                         []string
	WholeContext                      bool
}

func (e availabilitySelectionEvidence) found() bool {
	return e.Status != "" && e.Status != availabilitySelectionNone
}

func (e availabilitySelectionEvidence) bookable() bool {
	return e.Status == availabilitySelectionBookable
}

func (e availabilitySelectionEvidence) blocksOlderEvidence() bool {
	return e.Status == availabilitySelectionBlockedMetadataOnly ||
		e.Status == availabilitySelectionIncomplete ||
		(e.Status == availabilitySelectionRejected && e.RejectedWholeContext)
}

func (e availabilitySelectionEvidence) blocksHistoryIndex(historyIndex int) bool {
	return e.blocksOlderEvidence() && historyIndex >= 0 && e.SourceHistoryIndex >= historyIndex
}

func (e availabilitySelectionEvidence) rejectsAvailabilityOptionForHistory(historyIndex int, index int, tripDate string) bool {
	return e.rejectsAvailabilityOptionForPrompt("", historyIndex, index, tripDate)
}

func (e availabilitySelectionEvidence) rejectsAvailabilityOptionForPrompt(
	promptSourceMessageID string,
	historyIndex int,
	index int,
	tripDate string,
) bool {
	if historyIndex < 0 {
		return false
	}
	promptSourceMessageID = strings.TrimSpace(promptSourceMessageID)
	if len(e.LaterRejections) > 0 {
		for _, rejection := range e.LaterRejections {
			if rejection.SourceHistoryIndex < historyIndex {
				continue
			}
			if !availabilityRejectionMatchesPromptSource(
				promptSourceMessageID,
				rejection.AvailabilityPromptSourceMessageID,
			) {
				continue
			}
			if rejection.WholeContext ||
				availabilityOptionIndexRejected(rejection.OptionIndexes, index) ||
				availabilityTripDateRejected(rejection.TripDates, tripDate) {
				return true
			}
		}
		return false
	}
	if e.Status != availabilitySelectionRejected || e.SourceHistoryIndex < historyIndex {
		return false
	}
	if !availabilityRejectionMatchesPromptSource(
		promptSourceMessageID,
		e.AvailabilityPromptSourceMessageID,
	) {
		return false
	}
	if e.RejectedWholeContext || e.rejectsOptionIndex(index) {
		return true
	}
	return e.rejectsTripDate(tripDate)
}

func availabilityRejectionMatchesPromptSource(promptSourceMessageID string, rejectionPromptSourceMessageID string) bool {
	promptSourceMessageID = strings.TrimSpace(promptSourceMessageID)
	rejectionPromptSourceMessageID = strings.TrimSpace(rejectionPromptSourceMessageID)
	if promptSourceMessageID == "" || rejectionPromptSourceMessageID == "" {
		return promptSourceMessageID == "" && rejectionPromptSourceMessageID == ""
	}
	return promptSourceMessageID == rejectionPromptSourceMessageID
}

func availabilityOptionIndexRejected(rejectedIndexes []int, index int) bool {
	if index <= 0 {
		return false
	}
	for _, rejected := range rejectedIndexes {
		if rejected == index {
			return true
		}
	}
	return false
}

func availabilityTripDateRejected(rejectedDates []string, tripDate string) bool {
	date := canonicalRejectedTripDate(tripDate)
	if date == "" {
		return false
	}
	for _, rejected := range rejectedDates {
		if canonicalRejectedTripDate(rejected) == date {
			return true
		}
	}
	return false
}

func (e availabilitySelectionEvidence) rejectsOptionIndex(index int) bool {
	return availabilityOptionIndexRejected(e.RejectedOptionIndexes, index)
}

func (e availabilitySelectionEvidence) rejectsTripDate(tripDate string) bool {
	return availabilityTripDateRejected(e.RejectedTripDates, tripDate)
}

func (e availabilitySelectionEvidence) availabilityItem() (AvailabilitySearchItem, bool) {
	item := AvailabilitySearchItem{
		TripID:                 strings.TrimSpace(e.TripID),
		BoardStopID:            strings.TrimSpace(e.BoardStopID),
		AlightStopID:           strings.TrimSpace(e.AlightStopID),
		OriginDisplayName:      strings.TrimSpace(e.OriginDisplayName),
		DestinationDisplayName: strings.TrimSpace(e.DestinationDisplayName),
		PackageName:            strings.TrimSpace(e.PackageName),
		TripDate:               strings.TrimSpace(e.TripDate),
		OriginDepartTime:       strings.TrimSpace(e.DepartureTime),
		Price:                  e.Price,
		Currency:               strings.TrimSpace(e.Currency),
	}
	return item, e.bookable() && hasCompleteSelectedTripFacts(item.TripID, item.BoardStopID, item.AlightStopID)
}

func (c BookingDraftContext) IsAdvancedBookingFlow() bool {
	return c.HasAvailabilityShown ||
		c.RequestedPassengerDocuments ||
		c.BookingCreated ||
		c.HasPassengerDetails ||
		strings.TrimSpace(c.TripID) != ""
}

type bookingDraftAvailabilityProjectionPolicy string

const (
	bookingDraftAvailabilityEnvelopeOnly      bookingDraftAvailabilityProjectionPolicy = "ENVELOPE_ONLY"
	bookingDraftAvailabilityBookableSelection bookingDraftAvailabilityProjectionPolicy = "BOOKABLE_SELECTION"
)

type bookingDraftTurnProjection struct {
	CurrentTurn        string
	AvailabilityPolicy bookingDraftAvailabilityProjectionPolicy
}

func bookingDraftCurrentTurnProjection(currentTurn string) bookingDraftTurnProjection {
	return bookingDraftTurnProjection{
		CurrentTurn:        currentTurn,
		AvailabilityPolicy: bookingDraftAvailabilityBookableSelection,
	}
}

func bookingDraftRoutingBaselineProjection() bookingDraftTurnProjection {
	return bookingDraftTurnProjection{
		AvailabilityPolicy: bookingDraftAvailabilityEnvelopeOnly,
	}
}

func collectBookingDraftContextFromState(session Session, history []Message, currentTurn string) BookingDraftContext {
	state, ok := passengerClarificationStateV1FromSession(session)
	if !ok {
		state = newPassengerClarificationStateV1()
	}
	return collectBookingDraftContextWithPassengerState(session, history, currentTurn, state)
}

func collectBookingDraftContextForRoutingBaseline(session Session, history []Message) BookingDraftContext {
	state, ok := passengerClarificationStateV1FromSession(session)
	if !ok {
		state = newPassengerClarificationStateV1()
	}
	return collectBookingDraftContextForRoutingBaselineWithPassengerState(session, history, state)
}

func collectBookingDraftContextForRoutingBaselineWithPassengerState(
	session Session,
	history []Message,
	passengerState PassengerClarificationStateV1,
) BookingDraftContext {
	return collectBookingDraftContextWithPassengerStateProjection(
		session,
		history,
		passengerState,
		bookingDraftRoutingBaselineProjection(),
	)
}

func collectBookingDraftContextWithPassengerState(
	session Session,
	history []Message,
	currentTurn string,
	passengerState PassengerClarificationStateV1,
) BookingDraftContext {
	return collectBookingDraftContextWithPassengerStateProjection(
		session,
		history,
		passengerState,
		bookingDraftCurrentTurnProjection(currentTurn),
	)
}

func collectBookingDraftContextWithPassengerStateProjection(
	session Session,
	history []Message,
	passengerState PassengerClarificationStateV1,
	projection bookingDraftTurnProjection,
) BookingDraftContext {
	selectionState := availabilitySelectionStateV1ForRead(session, history)
	history = availabilityInferenceHistoryForSession(session, history)
	context := applyAvailabilitySelectionStateV1ToBookingDraft(BookingDraftContext{}, selectionState)
	context = applyPassengerClarificationStateToBookingDraft(context, passengerState)

	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		body := messageTurnText(message)
		folded := strings.Join(strings.Fields(foldChatText(body)), " ")

		if strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			if !context.RequestedPassengerDocuments && looksLikePassengerDocumentRequest(folded) {
				context.RequestedPassengerDocuments = true
			}
			if !context.AskedPaymentPreference && looksLikePaymentPreferencePrompt(folded) {
				context.AskedPaymentPreference = true
			}
		}

		if snapshot := selectedAvailabilityResultFromMessage(message); len(snapshot) > 0 {
			context.HasAvailabilityShown = true
		}

		classifiedMessage, _, classifiedOK := classifiedAvailabilityPromptMessageAtV1(history, i)
		if classifiedOK {
			availability, availabilityOK := availabilityPromptClassifiedContextV1(classifiedMessage)
			if availabilityOK && availability != nil {
				context.HasAvailabilityShown = true
				if !context.HasBookableSelection {
					mergeAvailabilityEnvelopeIntoBookingDraft(
						&context,
						buildAvailabilityToolResponsePayload(*availability),
					)
				}
			}
		}
	}

	expectedForReconstruction := 0
	if context.PassengerCountKnown && context.PassengerCount > 0 {
		expectedForReconstruction = expectedPassengerDocumentCount(context)
	}
	evidence := reconstructPassengerDocumentEvidence(
		passengerDocumentEvidenceTimeline(history, projection.CurrentTurn, session, context.TripDate),
		expectedForReconstruction,
	)
	passengerDetailsText := evidence.SourceText
	passengers := evidence.Passengers
	unresolvedPartials := evidence.Partials
	context.PassengerSnapshot = evidence.Snapshot
	if len(passengers) > 0 {
		context.HasPassengerDetails = true
		context.PassengerDetailsCount = len(passengers)
		context.PassengerDetailsText = passengerDetailsText
		context.PassengerDetails = passengers
		context.LapChildPassengerIndexes = lapChildIndexesFromPassengers(passengers)
		if len(context.LapChildPassengerIndexes) > 0 {
			context.LapChildAssignmentKnown = context.ChildUnder5CountKnown &&
				len(context.LapChildPassengerIndexes) == context.ChildUnder5Count
		}
	}
	if len(unresolvedPartials) > 0 {
		context.PartialPassengerDetails = unresolvedPartials
		context.PartialPassengerDetailsCount = len(unresolvedPartials)
		if context.PassengerDetailsText == "" {
			context.PassengerDetailsText = passengerDetailsText
		}
	}
	context.ExpectedDocumentCount = expectedPassengerDocumentCount(context)
	var explicitLapChildIndexes []int
	hasExplicitLapChildAssignment := false
	if !context.LapChildAssignmentKnown {
		explicitLapChildIndexes, hasExplicitLapChildAssignment = explicitLapChildAssignmentIndexes(
			history,
			projection.CurrentTurn,
			passengers,
			context.ChildUnder5Count,
		)
	}
	if hasExplicitLapChildAssignment {
		context.LapChildPassengerIndexes = explicitLapChildIndexes
		context.LapChildAssignmentKnown = true
	} else if context.ChildUnder5Count > 0 &&
		context.ExpectedDocumentCount > 0 &&
		len(passengers) == context.ExpectedDocumentCount &&
		!context.LapChildAssignmentKnown {
		if indexes, ok := inferLapChildAssignmentIndexes(history, projection.CurrentTurn, passengers, context.ChildUnder5Count); ok {
			context.LapChildPassengerIndexes = indexes
			context.LapChildAssignmentKnown = true
		}
	}
	if context.LapChildAssignmentKnown && len(context.LapChildPassengerIndexes) > 0 && len(context.PassengerDetails) > 0 {
		applyLapChildPassengerIndexes(context.PassengerDetails, context.LapChildPassengerIndexes)
	}
	context.PassengerSnapshot = alignBookingPassengerSnapshot(
		context.PassengerSnapshot,
		context.PassengerDetails,
		context.ExpectedDocumentCount,
	)
	if hasExplicitLapChildAssignment {
		context.PassengerSnapshot = markBookingPassengerSnapshotExplicitAssignment(
			context.PassengerSnapshot,
			context.PassengerDetails,
			context.ExpectedDocumentCount,
		)
	}
	context.NeedsLapChildAssignment = context.ChildUnder5Count > 0 &&
		context.HasPassengerDetails &&
		context.ExpectedDocumentCount > 0 &&
		context.PassengerDetailsCount == context.ExpectedDocumentCount &&
		!context.LapChildAssignmentKnown

	return context
}

func applyAvailabilitySelectionStateV1ToBookingDraft(
	context BookingDraftContext,
	state AvailabilitySelectionStateV1,
) BookingDraftContext {
	if state.Status != AvailabilitySelectionStatusBookable {
		return context
	}
	item, ok := state.Snapshot.availabilityItem()
	if !ok || state.SelectedOptionIndex <= 0 ||
		state.Snapshot.SelectedOptionIndex != state.SelectedOptionIndex {
		return context
	}
	context = applyBookableAvailabilityItemToBookingDraft(context, state.SelectedOptionIndex, item)
	if !context.HasBookableSelection {
		return context
	}
	context.Origin = firstNonEmpty(state.Snapshot.Origin, context.Origin)
	context.Destination = firstNonEmpty(state.Snapshot.Destination, context.Destination)
	context.PackageName = strings.TrimSpace(state.Snapshot.PackageName)
	context.SelectionMessageID = strings.TrimSpace(state.SelectionProjectionMessageID)
	context.AvailabilityPromptSourceMessageID = strings.TrimSpace(state.AvailabilityPromptSourceMessageID)
	return context
}

func applyPassengerClarificationStateToBookingDraft(
	context BookingDraftContext,
	state PassengerClarificationStateV1,
) BookingDraftContext {
	context.PassengerCount = state.PassengerCount
	context.PassengerCountProvenance = state.PassengerCountProvenance
	context.PassengerCountKnown = state.PassengerCountKnown
	context.PassengerCountConflicting = passengerClarificationStateConflictingV1(state)
	context.ChildUnder5Count = state.ChildUnder5Count
	context.ChildUnder5CountKnown = state.ChildUnder5CountKnown
	context.ChildUnder5AddsTraveler = state.ChildUnder5AddsTraveler
	context.PassengerSlotStatus = state.PassengerSlotStatus
	context.ChildSlotStatus = state.ChildSlotStatus
	context.PassengerPromptMessageID = state.PassengerPromptMessageID
	context.ChildPromptMessageID = state.ChildPromptMessageID
	context.PassengerReasonCodes = append([]string(nil), state.PassengerReasonCodes...)
	context.ChildReasonCodes = append([]string(nil), state.ChildReasonCodes...)
	context.ChildUnder5AddsTravelerOrigin = state.ChildUnder5AddsTravelerOrigin
	context.ChildReferences = clonePassengerClarificationChildReferencesV1(state.ChildReferences)
	context.BookingCreated = state.Authority == PassengerClarificationAuthorityPostBooking
	if passengerClarificationContextActiveV1(state) {
		context.AskedPassengerQuestion = true
		context.PassengerCountContextActive = true
	}
	return context
}

func latestAvailabilitySelectionEvidence(history []Message) availabilitySelectionEvidence {
	rejections := []availabilitySelectionRejection{}
	authority := availabilitySelectionEvidence{Status: availabilitySelectionNone}

	for i := 0; i < len(history); i++ {
		message := history[i]
		evidence := availabilitySelectionEvidence{Status: availabilitySelectionNone}
		if strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			folded := strings.Join(strings.Fields(foldChatText(messageTurnText(message))), " ")
			rejection := parseAvailabilityRejectionEvidence(folded)
			if rejection.Found && hasPriorAvailabilityContextBefore(history, i) {
				rejection.AvailabilityPromptSourceMessageID = availabilityPromptSourceMessageIDBefore(history, i)
				evidence = availabilitySelectionEvidenceFromRejection(i, strings.TrimSpace(message.ID), rejection)
			}
		}

		if messageEvidence := messageAvailabilitySelectionEvidence(message); messageEvidence.found() {
			evidence = messageEvidence
		}
		if !evidence.found() {
			continue
		}
		evidence.SourceHistoryIndex = i
		if evidence.Status == availabilitySelectionRejected && evidence.RejectionMessageID == "" {
			evidence.RejectionMessageID = strings.TrimSpace(message.ID)
		} else if evidence.SelectionMessageID == "" {
			evidence.SelectionMessageID = strings.TrimSpace(message.ID)
		}
		switch evidence.Status {
		case availabilitySelectionRejected:
			rejection := availabilitySelectionRejectionFromEvidence(evidence)
			rejections = append(rejections, rejection)
			if authority.bookable() {
				if availabilityRejectionTargetsSelection(rejection, authority) {
					authority = evidence
				}
				continue
			}
			if evidence.RejectedWholeContext ||
				!authority.found() ||
				authority.Status == availabilitySelectionRejected {
				authority = evidence
			}
		case availabilitySelectionBlockedMetadataOnly, availabilitySelectionIncomplete:
			authority = evidence
		case availabilitySelectionBookable:
			if evidence.MaterializesAuthority {
				authority = evidence
			}
		}
	}

	if !authority.found() {
		return availabilitySelectionEvidence{Status: availabilitySelectionNone}
	}
	authority.LaterRejections = cloneAvailabilitySelectionRejections(rejections)
	if authority.Status == availabilitySelectionRejected {
		authority.RejectedOptionIndexes = availabilitySelectionRejectedOptionIndexes(rejections)
		authority.RejectedTripDates = availabilitySelectionRejectedTripDates(rejections)
	}
	return authority
}

func availabilitySelectionEvidenceFromRejection(
	sourceHistoryIndex int,
	rejectionMessageID string,
	rejection availabilityRejectionEvidence,
) availabilitySelectionEvidence {
	return availabilitySelectionEvidence{
		Status:                            availabilitySelectionRejected,
		SourceHistoryIndex:                sourceHistoryIndex,
		RejectionMessageID:                strings.TrimSpace(rejectionMessageID),
		AvailabilityPromptSourceMessageID: strings.TrimSpace(rejection.AvailabilityPromptSourceMessageID),
		RejectedOptionIndexes:             append([]int(nil), rejection.OptionIndexes...),
		RejectedTripDates:                 append([]string(nil), rejection.TripDates...),
		RejectedWholeContext:              rejection.WholeContext,
	}
}

func availabilitySelectionRejectionFromEvidence(evidence availabilitySelectionEvidence) availabilitySelectionRejection {
	return availabilitySelectionRejection{
		SourceHistoryIndex:                evidence.SourceHistoryIndex,
		RejectionMessageID:                strings.TrimSpace(evidence.RejectionMessageID),
		AvailabilityPromptSourceMessageID: strings.TrimSpace(evidence.AvailabilityPromptSourceMessageID),
		OptionIndexes:                     append([]int(nil), evidence.RejectedOptionIndexes...),
		TripDates:                         append([]string(nil), evidence.RejectedTripDates...),
		WholeContext:                      evidence.RejectedWholeContext,
	}
}

func cloneAvailabilitySelectionRejections(rejections []availabilitySelectionRejection) []availabilitySelectionRejection {
	cloned := make([]availabilitySelectionRejection, 0, len(rejections))
	for _, rejection := range rejections {
		cloned = append(cloned, availabilitySelectionRejection{
			SourceHistoryIndex:                rejection.SourceHistoryIndex,
			RejectionMessageID:                strings.TrimSpace(rejection.RejectionMessageID),
			AvailabilityPromptSourceMessageID: strings.TrimSpace(rejection.AvailabilityPromptSourceMessageID),
			OptionIndexes:                     append([]int(nil), rejection.OptionIndexes...),
			TripDates:                         append([]string(nil), rejection.TripDates...),
			WholeContext:                      rejection.WholeContext,
		})
	}
	return cloned
}

func availabilityRejectionTargetsSelection(
	rejection availabilitySelectionRejection,
	selection availabilitySelectionEvidence,
) bool {
	if !availabilitySelectionAndRejectionSharePromptSource(selection, rejection) {
		return false
	}
	return rejection.WholeContext ||
		availabilityOptionIndexRejected(rejection.OptionIndexes, selection.SelectedOptionIndex) ||
		availabilityTripDateRejected(rejection.TripDates, selection.TripDate)
}

func availabilitySelectionAndRejectionSharePromptSource(
	selection availabilitySelectionEvidence,
	rejection availabilitySelectionRejection,
) bool {
	selectionPromptSourceMessageID := strings.TrimSpace(selection.AvailabilityPromptSourceMessageID)
	rejectionPromptSourceMessageID := strings.TrimSpace(rejection.AvailabilityPromptSourceMessageID)
	if selectionPromptSourceMessageID != "" || rejectionPromptSourceMessageID != "" {
		if selectionPromptSourceMessageID != "" && rejectionPromptSourceMessageID != "" {
			return selectionPromptSourceMessageID == rejectionPromptSourceMessageID
		}
		return selectionPromptSourceMessageID == "" &&
			rejectionPromptSourceMessageID == strings.TrimSpace(selection.SelectionMessageID)
	}
	return true
}

func availabilityPromptSourceMessageIDBefore(history []Message, beforeIndex int) string {
	if beforeIndex > len(history) {
		beforeIndex = len(history)
	}
	for i := beforeIndex - 1; i >= 0; i-- {
		if deliveredInvalidAvailabilityPromptBarrierAtV1(history, i) {
			return ""
		}
		message, _, ok := classifiedAvailabilityPromptMessageAtV1(history, i)
		if !ok {
			continue
		}
		if context := availabilitySelectionPromptContextFromMessage(message); context.OptionCount > 0 {
			return availabilityPromptSourceMessageIDFromMessage(message)
		}
	}
	return ""
}

func availabilityPromptSourceMessageIDAtHistoryIndex(history []Message, historyIndex int) string {
	if historyIndex < 0 || historyIndex >= len(history) {
		return ""
	}
	message, _, ok := classifiedAvailabilityPromptMessageAtV1(history, historyIndex)
	if !ok {
		return ""
	}
	return availabilityPromptSourceMessageIDFromMessage(message)
}

func availabilitySelectionRejectedOptionIndexes(rejections []availabilitySelectionRejection) []int {
	indexes := []int{}
	for _, rejection := range rejections {
		for _, index := range rejection.OptionIndexes {
			indexes = appendUniqueAvailabilityOptionIndex(indexes, index)
		}
	}
	return indexes
}

func availabilitySelectionRejectedTripDates(rejections []availabilitySelectionRejection) []string {
	dates := []string{}
	for _, rejection := range rejections {
		for _, date := range rejection.TripDates {
			dates = appendUniqueAvailabilityTripDate(dates, canonicalRejectedTripDate(date))
		}
	}
	return dates
}

func messageAvailabilitySelectionEvidence(message Message) availabilitySelectionEvidence {
	if rejection := availabilityRejectionEvidenceFromMessageMetadata(message); rejection.Found {
		return availabilitySelectionEvidence{
			Status:                            availabilitySelectionRejected,
			RejectionMessageID:                strings.TrimSpace(message.ID),
			AvailabilityPromptSourceMessageID: strings.TrimSpace(rejection.AvailabilityPromptSourceMessageID),
			RejectedOptionIndexes:             append([]int(nil), rejection.OptionIndexes...),
			RejectedTripDates:                 append([]string(nil), rejection.TripDates...),
			RejectedWholeContext:              rejection.WholeContext,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") ||
		!shouldMergeAvailabilityFactsFromMessage(message) {
		return availabilitySelectionEvidence{Status: availabilitySelectionNone}
	}

	metadataOnlyIndex := 0
	incompleteIndex := 0
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if len(payload) == 0 {
			continue
		}
		if strings.EqualFold(payloadMetadataString(payload, "intent"), string(IntentLapChildAssignmentAnswer)) {
			continue
		}
		index := payloadSelectedOptionIndex(payload)
		if index <= 0 {
			continue
		}
		status, selected := messageAvailabilitySelectionStatusForIndex(message, index)
		if status == availabilitySelectionBookable {
			evidence := availabilitySelectionEvidenceFromSelected(status, index, selected)
			evidence.SelectionMessageID, evidence.AvailabilityPromptSourceMessageID =
				availabilitySelectionIdentityFromMessage(message, index)
			evidence.MaterializesAuthority = availabilitySelectionMessageMaterializesAuthority(
				message,
				evidence.SelectionMessageID,
			)
			return evidence
		}
		if status == availabilitySelectionIncomplete && incompleteIndex <= 0 {
			incompleteIndex = index
		}
		if metadataOnlyIndex <= 0 {
			metadataOnlyIndex = index
		}
	}
	if incompleteIndex > 0 {
		selectionMessageID, promptSourceMessageID := availabilitySelectionIdentityFromMessage(message, incompleteIndex)
		return availabilitySelectionEvidence{
			Status:                            availabilitySelectionIncomplete,
			SelectionMessageID:                selectionMessageID,
			AvailabilityPromptSourceMessageID: promptSourceMessageID,
			SelectedOptionIndex:               incompleteIndex,
		}
	}
	if metadataOnlyIndex > 0 {
		selectionMessageID, promptSourceMessageID := availabilitySelectionIdentityFromMessage(message, metadataOnlyIndex)
		return availabilitySelectionEvidence{
			Status:                            availabilitySelectionBlockedMetadataOnly,
			SelectionMessageID:                selectionMessageID,
			AvailabilityPromptSourceMessageID: promptSourceMessageID,
			SelectedOptionIndex:               metadataOnlyIndex,
		}
	}
	return availabilitySelectionEvidence{Status: availabilitySelectionNone}
}

func availabilitySelectionIdentityFromMessage(message Message, index int) (string, string) {
	selectionMessageID := ""
	promptSourceMessageID := ""
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if len(payload) == 0 {
			continue
		}
		snapshot := selectedAvailabilityResultFromPayload(payload)
		if len(snapshot) == 0 {
			continue
		}
		snapshotIndex := asInt(snapshot["selected_option_index"])
		if snapshotIndex > 0 && index > 0 && snapshotIndex != index {
			continue
		}
		if snapshotSelectionMessageID := strings.TrimSpace(
			asString(snapshot[selectedAvailabilitySelectionMessageIDPayloadKey]),
		); snapshotSelectionMessageID != "" {
			selectionMessageID = snapshotSelectionMessageID
		}
		promptSourceMessageID = firstNonEmpty(
			strings.TrimSpace(asString(snapshot[availabilityPromptSourceMessageIDPayloadKey])),
			promptSourceMessageID,
		)
	}
	return selectionMessageID, promptSourceMessageID
}

func availabilitySelectionMessageMaterializesAuthority(message Message, _ string) bool {
	materializes, present := availabilitySelectionMessageAuthorityMarker(message)
	return present && materializes && availabilitySelectionMessageHasStructuralSelectIntentV1(message)
}

func availabilitySelectionMessageAuthorityMarker(message Message) (bool, bool) {
	foundTrue := false
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if snapshot := selectedAvailabilityResultFromPayload(payload); len(snapshot) > 0 {
			if materializes, exists := snapshot[availabilitySelectionMaterializesAuthorityPayloadKey]; exists {
				if !readBool(materializes) {
					return false, true
				}
				foundTrue = true
			}
		}
	}
	return foundTrue, foundTrue
}

func availabilityRejectionEvidenceFromMessageMetadata(message Message) availabilityRejectionEvidence {
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if len(payload) == 0 {
			continue
		}
		if rejection := availabilityRejectionEvidenceFromTemplateData(asMap(payload["template_data"])); rejection.Found {
			return rejection
		}
	}
	return availabilityRejectionEvidence{}
}

func availabilityRejectionEvidenceFromTemplateData(data map[string]interface{}) availabilityRejectionEvidence {
	if !templateDataBool(data, outOfTurnRejectedAvailabilityDataKey) {
		return availabilityRejectionEvidence{}
	}
	rejection := availabilityRejectionEvidence{
		Found:                             true,
		WholeContext:                      templateDataBool(data, outOfTurnRejectedWholeContextDataKey),
		AvailabilityPromptSourceMessageID: strings.TrimSpace(asString(data[outOfTurnActivePromptSourceIDDataKey])),
		OptionIndexes:                     availabilityRejectedOptionIndexesFromMetadata(data[outOfTurnRejectedOptionIndexesDataKey]),
		TripDates:                         availabilityRejectedTripDatesFromMetadata(data[outOfTurnRejectedTripDatesDataKey]),
	}
	if !rejection.WholeContext && !rejection.hasSpecificTarget() {
		rejection.WholeContext = true
	}
	return rejection
}

func availabilityRejectedOptionIndexesFromMetadata(value interface{}) []int {
	switch typed := value.(type) {
	case []int:
		return append([]int(nil), typed...)
	case []interface{}:
		indexes := make([]int, 0, len(typed))
		for _, raw := range typed {
			indexes = appendUniqueAvailabilityOptionIndex(indexes, asInt(raw))
		}
		return indexes
	default:
		return nil
	}
}

func availabilityRejectedTripDatesFromMetadata(value interface{}) []string {
	dates := []string{}
	for _, raw := range asStringSlice(value) {
		if date := canonicalRejectedTripDate(raw); date != "" {
			dates = appendUniqueAvailabilityTripDate(dates, date)
		}
	}
	return dates
}

func canonicalRejectedTripDate(tripDate string) string {
	tripDate = strings.TrimSpace(tripDate)
	if tripDate == "" {
		return ""
	}
	if parsed := parseISODatePtr(tripDate); parsed != nil {
		return parsed.Format("02/01")
	}
	folded := strings.Join(strings.Fields(foldChatText(tripDate)), " ")
	for _, match := range availabilityFoldedDateReferencePattern.FindAllStringSubmatch(folded, -1) {
		if len(match) == 3 && validAvailabilityDayMonth(match[1], match[2]) {
			return canonicalAvailabilityDayMonth(match[1], match[2])
		}
	}
	return ""
}

func messageAvailabilitySelectionStatusForIndex(message Message, index int) (availabilitySelectionStatus, map[string]interface{}) {
	if index <= 0 {
		return availabilitySelectionNone, nil
	}
	hasSelectionFacts := false
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if len(payload) == 0 {
			continue
		}
		snapshot := selectedAvailabilityResultFromPayload(payload)
		if len(snapshot) == 0 {
			continue
		}
		hasSelectionFacts = true
		if selectedAvailabilitySnapshotMaterializesIndex(snapshot, index) {
			return availabilitySelectionBookable, snapshot
		}
	}
	for _, toolContext := range messageToolContexts(message) {
		availability := asMap(toolContext[toolNameAvailabilitySearch])
		if len(availability) == 0 {
			continue
		}
		selected, ok := selectedAvailabilityPayloadItem(availability, index)
		if !ok {
			hasSelectionFacts = true
			continue
		}
		hasSelectionFacts = true
		if selectedAvailabilitySnapshotMaterializesIndex(selected, index) {
			return availabilitySelectionBookable, selected
		}
	}
	if hasSelectionFacts {
		return availabilitySelectionIncomplete, nil
	}
	return availabilitySelectionBlockedMetadataOnly, nil
}

func availabilitySelectionEvidenceFromSelected(status availabilitySelectionStatus, index int, selected map[string]interface{}) availabilitySelectionEvidence {
	return availabilitySelectionEvidence{
		Status:                 status,
		SelectedOptionIndex:    index,
		TripID:                 strings.TrimSpace(asString(selected["trip_id"])),
		BoardStopID:            strings.TrimSpace(asString(selected["board_stop_id"])),
		AlightStopID:           strings.TrimSpace(asString(selected["alight_stop_id"])),
		OriginDisplayName:      strings.TrimSpace(firstNonEmpty(asString(selected["origin"]), asString(selected["origin_display_name"]))),
		DestinationDisplayName: strings.TrimSpace(firstNonEmpty(asString(selected["destination"]), asString(selected["destination_display_name"]))),
		PackageName:            strings.TrimSpace(asString(selected["package_name"])),
		TripDate:               strings.TrimSpace(asString(selected["trip_date"])),
		DepartureTime:          strings.TrimSpace(asString(selected["origin_depart_time"])),
		Price:                  asFloat64(selected["price"]),
		Currency:               strings.TrimSpace(asString(selected["currency"])),
	}
}

func hasCompleteSelectedTripFacts(tripID, boardStopID, alightStopID string) bool {
	return strings.TrimSpace(tripID) != "" &&
		strings.TrimSpace(boardStopID) != "" &&
		strings.TrimSpace(alightStopID) != ""
}

func selectedAvailabilitySnapshotMaterializesIndex(snapshot map[string]interface{}, index int) bool {
	if len(snapshot) == 0 || index <= 0 {
		return false
	}
	snapshotIndex := asInt(snapshot["selected_option_index"])
	if snapshotIndex > 0 && snapshotIndex != index {
		return false
	}
	return hasCompleteSelectedTripFacts(
		asString(snapshot["trip_id"]),
		asString(snapshot["board_stop_id"]),
		asString(snapshot["alight_stop_id"]),
	)
}

func selectedAvailabilityPayloadItem(payload map[string]interface{}, index int) (map[string]interface{}, bool) {
	if len(payload) == 0 || index <= 0 {
		return nil, false
	}
	results := asInterfaceSliceMaps(payload["results"])
	selectedIndex := index - 1
	if selectedIndex < 0 || selectedIndex >= len(results) {
		return nil, false
	}
	return results[selectedIndex], true
}

func shouldMergeSelectedAvailabilitySnapshotForBookingDraft(
	selection availabilitySelectionEvidence,
	historyIndex int,
	messageID string,
	context BookingDraftContext,
	snapshot map[string]interface{},
) bool {
	if !selection.found() {
		return true
	}
	if selection.rejectsSelectedAvailabilitySnapshotForHistory(historyIndex, messageID, snapshot) {
		return false
	}
	if !selection.bookable() {
		return !selection.blocksHistoryIndex(historyIndex)
	}
	if !availabilitySelectionRecoverySourceMatchesMessage(selection, historyIndex, messageID) ||
		strings.TrimSpace(context.TripID) == "" {
		return false
	}
	return selectedAvailabilitySnapshotMatchesBookingDraft(context, snapshot)
}

func shouldMergeAvailabilityPayloadForBookingDraft(
	selection availabilitySelectionEvidence,
	historyIndex int,
	messageID string,
	context BookingDraftContext,
	payload map[string]interface{},
) bool {
	if !selection.found() {
		return true
	}
	if selection.rejectsAvailabilityPayloadForBookingDraft(historyIndex, messageID, context, payload) {
		return false
	}
	if !selection.bookable() {
		return !selection.blocksHistoryIndex(historyIndex)
	}
	return availabilitySelectionRecoverySourceMatchesMessage(selection, historyIndex, messageID) &&
		availabilityPayloadSelectionMatchesBookingDraft(context, payload)
}

func availabilitySelectionRecoverySourceMatchesMessage(
	selection availabilitySelectionEvidence,
	historyIndex int,
	messageID string,
) bool {
	messageID = strings.TrimSpace(messageID)
	selectionMessageID := strings.TrimSpace(selection.SelectionMessageID)
	promptSourceMessageID := strings.TrimSpace(selection.AvailabilityPromptSourceMessageID)
	if selectionMessageID != "" || promptSourceMessageID != "" {
		return messageID != "" &&
			(messageID == selectionMessageID || messageID == promptSourceMessageID)
	}
	return historyIndex == selection.SourceHistoryIndex
}

func (e availabilitySelectionEvidence) rejectsSelectedAvailabilitySnapshotForHistory(
	historyIndex int,
	messageID string,
	snapshot map[string]interface{},
) bool {
	if len(snapshot) == 0 {
		return false
	}
	return e.rejectsAvailabilityOptionForPrompt(
		strings.TrimSpace(messageID),
		historyIndex,
		asInt(snapshot["selected_option_index"]),
		asString(snapshot["trip_date"]),
	)
}

func (e availabilitySelectionEvidence) rejectsAvailabilityPayloadForBookingDraft(
	historyIndex int,
	messageID string,
	context BookingDraftContext,
	payload map[string]interface{},
) bool {
	results := asInterfaceSliceMaps(payload["results"])
	if len(results) == 0 {
		return false
	}
	selectedIndex := context.SelectedOptionIndex
	if selectedIndex <= 0 && len(results) == 1 {
		selectedIndex = 1
	}
	if selectedIndex <= 0 || selectedIndex > len(results) {
		return false
	}
	return e.rejectsAvailabilityOptionForPrompt(
		strings.TrimSpace(messageID),
		historyIndex,
		selectedIndex,
		asString(results[selectedIndex-1]["trip_date"]),
	)
}

func availabilityPayloadSelectionMatchesBookingDraft(context BookingDraftContext, payload map[string]interface{}) bool {
	if strings.TrimSpace(context.TripID) == "" || context.SelectedOptionIndex <= 0 {
		return false
	}
	results := asInterfaceSliceMaps(payload["results"])
	selectedIndex := context.SelectedOptionIndex - 1
	if selectedIndex < 0 || selectedIndex >= len(results) {
		return false
	}
	return selectedAvailabilitySnapshotMatchesBookingDraft(context, results[selectedIndex])
}

func isShortYesReply(text string) bool {
	switch strings.Join(strings.Fields(foldChatText(text)), " ") {
	case "sim", "s", "ss", "positivo", "posi", "tem", "tem sim", "sim tem",
		"sim ela", "sim ele",
		"ela", "ele",
		"ela sim", "ele sim",
		"isso ela", "isso ele",
		"e ela", "eh ela", "é ela",
		"e ele", "eh ele", "é ele":
		return true
	default:
		return false
	}
}

func lastAssistantAskedChildUnder5(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return looksLikeChildUnder5Question(history[i].Body)
	}
	return false
}

func previousAssistantAskedChildUnder5(history []Message, beforeIndex int) bool {
	for i := beforeIndex - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return looksLikeChildUnder5Question(history[i].Body)
	}
	return false
}

func lastAssistantAskedPassengerAndChildCombined(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return looksLikePassengerAndChildCombinedQuestion(history[i].Body)
	}
	return false
}

func previousAssistantAskedPassengerAndChildCombined(history []Message, beforeIndex int) bool {
	for i := beforeIndex - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		return looksLikePassengerAndChildCombinedQuestion(history[i].Body)
	}
	return false
}

func looksLikePassengerAndChildCombinedQuestion(text string) bool {
	return looksLikePassengerQuantityQuestion(text) && looksLikeChildUnder5Question(text)
}

func looksLikePassengerQuantityQuestion(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	return strings.Contains(folded, "a passagem e so para voce") ||
		strings.Contains(folded, "e so para voce") ||
		strings.Contains(folded, "tem mais alguem") ||
		strings.Contains(folded, "quantas pessoas") ||
		strings.Contains(folded, "quantos passageiros") ||
		strings.Contains(folded, "passageiros")
}

func looksLikeChildUnder5Question(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	return strings.Contains(folded, "crianca de ate 5 anos") ||
		strings.Contains(folded, "crianca de 5 anos ou menos") ||
		strings.Contains(folded, "tem ate 5 anos") ||
		strings.Contains(folded, "ate 5 anos viajando")
}

func expectedPassengerDocumentCount(context BookingDraftContext) int {
	if context.PassengerCountConflicting {
		return 0
	}
	if !context.PassengerCountKnown || context.PassengerCount <= 0 {
		return context.PassengerDetailsCount
	}
	expected := context.PassengerCount
	if context.ChildUnder5AddsTraveler && context.ChildUnder5Count > 0 {
		expected += context.ChildUnder5Count
	}
	return expected
}

func evaluateCanonicalBookingCreateReadiness(context BookingDraftContext) canonicalBookingCreateReadiness {
	passengers := append([]BookingCreatePassengerInput(nil), context.PassengerDetails...)
	if len(context.LapChildPassengerIndexes) > 0 {
		applyLapChildPassengerIndexes(passengers, context.LapChildPassengerIndexes)
	}

	readiness := canonicalBookingCreateReadiness{
		Passengers:                 passengers,
		QuantityStatus:             bookingCreateQuantityUnknown,
		UnresolvedPartials:         unresolvedPassengerDocumentPartialsForPassengers(context.PartialPassengerDetails, passengers),
		DocumentsComplete:          len(passengers) > 0,
		LapChildAssignmentComplete: true,
		Reason:                     "passenger_count_unknown",
	}
	if context.PassengerCountKnown && context.PassengerCount > 0 {
		readiness.Expected = expectedPassengerDocumentCount(context)
		if readiness.Expected > 0 {
			switch {
			case len(passengers) < readiness.Expected:
				readiness.QuantityStatus = bookingCreateQuantityMissing
				readiness.Reason = "passenger_documents_missing"
			case len(passengers) > readiness.Expected:
				readiness.QuantityStatus = bookingCreateQuantityExcess
				readiness.Reason = "passenger_documents_excess"
			default:
				readiness.QuantityStatus = bookingCreateQuantityExact
				readiness.Reason = "passenger_documents_ready"
			}
		}
	}

	for _, passenger := range passengers {
		if !bookingDraftPassengerDocumentComplete(passenger) {
			readiness.DocumentsComplete = false
			break
		}
	}
	childCount := context.ChildUnder5Count
	if count := countLapChildPassengers(passengers); count > 0 && !context.ChildUnder5CountKnown {
		childCount = count
	}
	if context.ChildUnder5CountKnown || childCount > 0 {
		readiness.LapChildAssignmentComplete = hasExpectedLapChildCount(passengers, childCount)
	}

	switch {
	case !context.HasBookableSelection:
		readiness.Reason = "bookable_selection_missing"
	case readiness.QuantityStatus != bookingCreateQuantityExact:
	case len(readiness.UnresolvedPartials) > 0:
		readiness.Reason = "passenger_document_partial_unresolved"
	case !readiness.DocumentsComplete:
		readiness.Reason = "passenger_document_incomplete"
	case !readiness.LapChildAssignmentComplete:
		readiness.Reason = "lap_child_assignment_missing"
	default:
		readiness.Ready = true
		readiness.Reason = "ready"
	}
	return readiness
}

func passengerDocumentCountMismatch(context BookingDraftContext) bool {
	expected := expectedPassengerDocumentCount(context)
	return expected > 0 &&
		context.PassengerDetailsCount > 0 &&
		context.PassengerDetailsCount != expected
}

func passengerDocumentCountExceedsExpected(context BookingDraftContext) bool {
	expected := expectedPassengerDocumentCount(context)
	return expected > 0 && context.PassengerDetailsCount > expected
}

func decideNextBookingStep(context BookingDraftContext) BookingNextAction {
	return bookingNextActionFromCanonicalReadiness(context, evaluateCanonicalBookingCreateReadiness(context))
}

func bookingNextActionFromCanonicalReadiness(context BookingDraftContext, readiness canonicalBookingCreateReadiness) BookingNextAction {
	if context.PassengerCountConflicting ||
		!context.PassengerCountKnown || context.PassengerCount <= 0 ||
		!context.ChildUnder5CountKnown {
		return BookingNextAskPassengerClarification
	}
	if context.BookingCreated {
		return BookingNextAskBookingPaymentPreference
	}
	if !context.HasAvailabilityShown ||
		!context.HasBookableSelection ||
		strings.TrimSpace(context.TripID) == "" ||
		strings.TrimSpace(context.BoardStopID) == "" ||
		strings.TrimSpace(context.AlightStopID) == "" ||
		strings.TrimSpace(context.Origin) == "" ||
		strings.TrimSpace(context.Destination) == "" ||
		strings.TrimSpace(context.TripDate) == "" {
		return BookingNextAwaitTripSelection
	}
	switch readiness.QuantityStatus {
	case bookingCreateQuantityUnknown:
		return BookingNextAskPassengerClarification
	case bookingCreateQuantityMissing:
		return BookingNextAskPassengerDocuments
	case bookingCreateQuantityExcess:
		return BookingNextAskPassengerClarification
	}
	if len(readiness.UnresolvedPartials) > 0 || !readiness.DocumentsComplete {
		return BookingNextAskPassengerDocuments
	}
	if !readiness.LapChildAssignmentComplete {
		return BookingNextAskLapChildAssignment
	}
	if readiness.Ready {
		return BookingNextCallCreate
	}
	return BookingNextAskPassengerDocuments
}

func buildBookingContinuationReply(context BookingDraftContext, action BookingNextAction) string {
	switch action {
	case BookingNextAskPassengerClarification:
		if passengerDocumentCountExceedsExpected(context) {
			return fmt.Sprintf(
				"Voce informou %d passageiros, mas recebi documentos de %d pessoas. Confirme quantos passageiros vao viajar.",
				expectedPassengerDocumentCount(context),
				context.PassengerDetailsCount,
			)
		}
		if context.PassengerCountKnown && context.PassengerCount > 0 && !context.ChildUnder5CountKnown {
			return "Tem crianca de 5 anos ou menos viajando?"
		}
		if context.HasAvailabilityShown {
			return askPassengerCountReply
		}
		return "Entendi. A passagem e so para voce ou vai mais alguem junto?"
	case BookingNextAwaitTripSelection:
		return "Antes de criar a reserva, preciso que voce escolha uma opcao de viagem disponivel."
	case BookingNextAskPassengerDocuments:
		if reply := buildPassengerDocumentProgressReply(context); reply != "" {
			return reply
		}
		return buildAskPassengerDocumentsReply(context)
	case BookingNextAskLapChildAssignment:
		return buildAskLapChildAssignmentReply(context)
	case BookingNextAskBookingPaymentPreference:
		return "Perfeito. Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?"
	default:
		return ""
	}
}

func buildAskPassengerDocumentsReply(context BookingDraftContext) string {
	expected := expectedPassengerDocumentCount(context)
	if expected <= 0 {
		expected = context.PassengerCount
	}
	if context.ChildUnder5AddsTraveler &&
		context.ChildUnder5Count > 0 &&
		context.PassengerDetailsCount > 0 &&
		context.PassengerDetailsCount < expected {
		if countLapChildPassengers(context.PassengerDetails) >= context.ChildUnder5Count {
			return "Recebi o documento da crianca de ate 5 anos. Ainda falta o documento do passageiro pagante. Pode enviar o nome completo e CPF, RG ou CNH completo?"
		}
		return "Recebi o documento do passageiro pagante. Ainda falta o documento da crianca de ate 5 anos. Pode enviar o nome completo e CPF, RG ou certidao de nascimento da crianca?"
	}
	return buildAskDocumentsReply(expected, context.PassengerDetailsCount)
}

func buildPassengerDocumentProgressReply(context BookingDraftContext) string {
	if context.PartialPassengerDetailsCount == 0 {
		return ""
	}

	partial := context.PartialPassengerDetails[0]
	nameFragment := strings.TrimSpace(partial.NameFragment)
	if nameFragment == "" {
		nameFragment = "desse passageiro"
	}

	if context.PassengerDetailsCount > 0 {
		names := make([]string, 0, context.PassengerDetailsCount)
		for _, passenger := range context.PassengerDetails {
			if name := strings.TrimSpace(passenger.Name); name != "" {
				names = append(names, name)
			}
		}
		received := fmt.Sprintf("Recebi os dados de %d passageiro", context.PassengerDetailsCount)
		if context.PassengerDetailsCount > 1 {
			received = fmt.Sprintf("Recebi os dados de %d passageiros", context.PassengerDetailsCount)
		}
		if len(names) > 0 {
			received = "Recebi os dados de " + strings.Join(names, ", ")
		}
		if nameFragment == "desse passageiro" {
			return fmt.Sprintf("%s. Para esse passageiro, recebi o CPF, mas preciso que voce reenvie esse passageiro na mesma linha com nome completo + CPF.", received)
		}
		return fmt.Sprintf("%s. Para %s, recebi o CPF, mas preciso que voce reenvie %s na mesma linha com nome completo + CPF.", received, nameFragment, nameFragment)
	}

	if context.PartialPassengerDetailsCount == 1 {
		if nameFragment == "desse passageiro" {
			return "Recebi o CPF, mas preciso que voce reenvie esse passageiro na mesma linha com nome completo + CPF para conferir antes de criar a reserva."
		}
		return fmt.Sprintf("Recebi o CPF, mas preciso que voce reenvie %s na mesma linha com nome completo + CPF para conferir antes de criar a reserva.", nameFragment)
	}
	return fmt.Sprintf("Recebi os CPFs, mas preciso que voce reenvie os %d passageiros na mesma linha com nome completo + CPF para conferir antes de criar a reserva.", context.PartialPassengerDetailsCount)
}

func buildBookingCreateMissingDataReply(context BookingDraftContext) string {
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if readiness.QuantityStatus == bookingCreateQuantityExact && !readiness.DocumentsComplete {
		completeDocuments := 0
		for _, passenger := range readiness.Passengers {
			if bookingDraftPassengerDocumentComplete(passenger) {
				completeDocuments++
			}
		}
		return buildAskDocumentsReply(readiness.Expected, completeDocuments)
	}

	missing := missingBookingCreateDataLabels(context)
	if len(missing) == 0 {
		return "Recebi os dados do passageiro, mas ainda falta confirmar se eles conferem para criar a reserva."
	}
	if len(missing) == 1 {
		return "Recebi a confirmacao, mas ainda falta este dado para criar a reserva: " + missing[0] + "."
	}
	return "Recebi a confirmacao, mas ainda faltam estes dados para criar a reserva: " + strings.Join(missing, ", ") + "."
}

func missingBookingCreateDataLabels(context BookingDraftContext) []string {
	missing := []string{}
	if !context.HasAvailabilityShown || strings.TrimSpace(context.TripID) == "" {
		missing = append(missing, "opcao de viagem")
	}
	if strings.TrimSpace(context.BoardStopID) == "" {
		missing = append(missing, "ponto de embarque")
	}
	if strings.TrimSpace(context.AlightStopID) == "" {
		missing = append(missing, "ponto de desembarque")
	}
	if strings.TrimSpace(context.Origin) == "" {
		missing = append(missing, "cidade de origem")
	}
	if strings.TrimSpace(context.Destination) == "" {
		missing = append(missing, "cidade de destino")
	}
	if strings.TrimSpace(context.TripDate) == "" {
		missing = append(missing, "data da viagem")
	}
	if !context.PassengerCountKnown || context.PassengerCount <= 0 {
		missing = append(missing, "quantidade de passageiros")
	}
	if !context.ChildUnder5CountKnown {
		missing = append(missing, "confirmacao se ha crianca de ate 5 anos")
	}
	if !context.HasPassengerDetails || context.PassengerDetailsCount <= 0 {
		if context.PartialPassengerDetailsCount > 0 {
			missing = append(missing, "nome completo dos passageiros com documento ja informado")
		} else {
			missing = append(missing, "nome completo e documento dos passageiros")
		}
	} else if passengerDocumentCountExceedsExpected(context) {
		missing = append(missing, "confirmacao da quantidade de passageiros, pois ha documentos excedentes")
	} else if expected := expectedPassengerDocumentCount(context); expected > 0 && context.PassengerDetailsCount != expected {
		if context.PartialPassengerDetailsCount > 0 {
			missing = append(missing, "nome completo dos passageiros com documento ja informado")
		} else {
			missing = append(missing, "documentos de todos os passageiros")
		}
	}
	if context.NeedsLapChildAssignment {
		missing = append(missing, "qual passageiro e a crianca de ate 5 anos")
	}
	return missing
}

func buildAskLapChildAssignmentReply(context BookingDraftContext) string {
	passengers := context.PassengerDetails
	if len(passengers) == 0 {
		passengers = extractBookingCreatePassengers(context.PassengerDetailsText, Session{})
	}
	if len(passengers) < 2 {
		return buildAskPassengerDocumentsReply(context)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Recebi os dados dos %d passageiros. Qual deles e a crianca de ate 5 anos?", len(passengers)))
	for i, passenger := range passengers {
		name := strings.TrimSpace(passenger.Name)
		if name == "" {
			name = fmt.Sprintf("Passageiro %d", i+1)
		}
		builder.WriteString(fmt.Sprintf("\n%d. %s", i+1, name))
	}
	return builder.String()
}

func buildBookingContinuationDraftRun(reply string, action BookingNextAction, context BookingDraftContext) RunAgentResult {
	reply = strings.TrimSpace(reply)
	templateName := bookingContinuationTemplateName(action, context)
	requestPayload := map[string]interface{}{
		"mode":                        "TEMPLATE_FIRST_REPLY",
		"intent":                      string(IntentPassengerCountReply),
		"action":                      string(action),
		"template_name":               string(templateName),
		"passenger_count":             context.PassengerCount,
		"expected_document_count":     expectedPassengerDocumentCount(context),
		"child_under_5_count":         context.ChildUnder5Count,
		"passenger_count_known":       context.PassengerCountKnown,
		"child_under_5_count_known":   context.ChildUnder5CountKnown,
		"child_under_5_adds_traveler": context.ChildUnder5AddsTraveler,
		"lap_child_assignment_known":  context.LapChildAssignmentKnown,
		"lap_child_passenger_indexes": context.LapChildPassengerIndexes,
		"needs_lap_child_assignment":  context.NeedsLapChildAssignment,
	}
	responsePayload := map[string]interface{}{
		"reply_text":              reply,
		"template_name":           string(templateName),
		"intent":                  string(IntentPassengerCountReply),
		"action":                  string(action),
		"expected_document_count": expectedPassengerDocumentCount(context),
	}
	if snapshot := selectedAvailabilityResultPayloadFromBookingDraft(context); len(snapshot) > 0 {
		requestPayload["selected_option_index"] = context.SelectedOptionIndex
		responsePayload["selected_option_index"] = context.SelectedOptionIndex
		requestPayload[selectedAvailabilityResultPayloadKey] = cloneMap(snapshot)
		responsePayload[selectedAvailabilityResultPayloadKey] = cloneMap(snapshot)
	}
	return RunAgentResult{
		ReplyText:       reply,
		Model:           "template_realizer",
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
	}
}

func shouldDraftPassengerDocumentConfirmation(session Session, history []Message, currentTurn string, context BookingDraftContext) bool {
	if !canDraftPassengerDocumentConfirmation(context, decideNextBookingStep(context)) {
		return false
	}
	if !context.HasPassengerDetails || len(context.PassengerDetails) == 0 {
		return false
	}
	if looksLikeDocumentConfirmation(currentTurn) {
		return false
	}
	if _, ok := parsePassengerDocumentCorrection(currentTurn); ok {
		return true
	}
	if !context.RequestedPassengerDocuments &&
		!lastAssistantAskedDocumentConfirmation(history) &&
		!lastAssistantAskedBookingProceedConfirmation(history) {
		return false
	}
	if context.PartialPassengerDetailsCount > 0 {
		return false
	}
	if context.LapChildAssignmentKnown &&
		context.ChildUnder5Count > 0 &&
		lastAssistantAskedLapChildAssignment(history) {
		return true
	}
	if looksLikeAlreadySentPassengerDocumentsReply(currentTurn) {
		return context.HasPassengerDetails &&
			len(context.PassengerDetails) > 0 &&
			lastAssistantAskedPassengerDocumentRequest(history)
	}
	return len(extractBookingCreatePassengers(currentTurn, session)) > 0
}

func canDraftPassengerDocumentConfirmation(context BookingDraftContext, action BookingNextAction) bool {
	if action != BookingNextCallCreate {
		return false
	}
	return evaluateCanonicalBookingCreateReadiness(context).Ready
}

func bookingDraftPassengerDocumentComplete(passenger BookingCreatePassengerInput) bool {
	if strings.TrimSpace(passenger.Name) == "" {
		return false
	}
	documentType := normalizePassengerDocumentType(passenger.DocumentType)
	if documentType == "" {
		for _, candidate := range []struct {
			Type  string
			Value string
		}{
			{"CPF", passenger.CPF},
			{"RG", passenger.RG},
			{"CNH", passenger.CNH},
			{"CERTIDAO_NASCIMENTO", passenger.BirthCertificateNumber},
		} {
			if normalizePassengerDocumentValue(candidate.Value, candidate.Type) != "" {
				documentType = candidate.Type
				break
			}
		}
	}
	if documentType == "" {
		return false
	}
	if normalizePassengerDocumentValue(passenger.Document, documentType) != "" {
		return true
	}
	switch documentType {
	case "CPF":
		return normalizePassengerDocumentValue(passenger.CPF, "CPF") != ""
	case "RG":
		return normalizePassengerDocumentValue(passenger.RG, "RG") != ""
	case "CNH":
		return normalizePassengerDocumentValue(passenger.CNH, "CNH") != ""
	case "CERTIDAO_NASCIMENTO":
		return normalizePassengerDocumentValue(passenger.BirthCertificateNumber, "CERTIDAO_NASCIMENTO") != ""
	default:
		return false
	}
}

func shouldAskPassengerNameAfterCPF(session Session, currentTurn string, context BookingDraftContext) bool {
	if context.HasPassengerDetails || looksLikeDocumentConfirmation(currentTurn) {
		return false
	}
	if extractValidCPF(currentTurn) == "" {
		return false
	}
	return len(extractBookingCreatePassengers(currentTurn, session)) == 0
}

func shouldPersistBookingPassengerSnapshotForRun(run RunAgentResult) bool {
	action := firstNonEmpty(
		strings.TrimSpace(asString(run.RequestPayload["action"])),
		strings.TrimSpace(asString(run.ResponsePayload["action"])),
	)
	switch action {
	case string(BookingNextAskPassengerDocuments),
		string(BookingNextAskLapChildAssignment),
		"confirm_passenger_documents":
		return true
	default:
		return false
	}
}

func buildInvalidPassengerCPFDraftRun(context BookingDraftContext) RunAgentResult {
	reply := "O CPF informado parece invalido. Pode reenviar o CPF correto com 11 digitos ou mandar uma foto legivel do documento?"
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":            "TEMPLATE_FIRST_REPLY",
			"intent":          string(IntentPassengerDocumentsProvided),
			"action":          "ask_valid_passenger_cpf",
			"template_name":   string(TemplateAskDocuments),
			"passenger_count": context.PassengerCount,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":     reply,
			"intent":         string(IntentPassengerDocumentsProvided),
			"action":         "ask_valid_passenger_cpf",
			"template_name":  string(TemplateAskDocuments),
			"needs_document": true,
		},
	}
}

func buildAskPassengerNameAfterCPFDraftRun(context BookingDraftContext) RunAgentResult {
	reply := "Recebi o CPF. Pode enviar tambem o nome completo do passageiro para eu conferir antes de criar a reserva?"
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":            "TEMPLATE_FIRST_REPLY",
			"intent":          string(IntentPassengerDocumentsProvided),
			"action":          "ask_passenger_name_for_cpf",
			"template_name":   string(TemplateAskDocuments),
			"passenger_count": context.PassengerCount,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"intent":        string(IntentPassengerDocumentsProvided),
			"action":        "ask_passenger_name_for_cpf",
			"template_name": string(TemplateAskDocuments),
		},
	}
}

func buildUnsupportedPDFDocumentDraftRun(context BookingDraftContext) RunAgentResult {
	reply := unsupportedPDFDocumentReply
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":            "TEMPLATE_FIRST_REPLY",
			"intent":          string(IntentPassengerDocumentsProvided),
			"action":          "ask_photo_or_typed_document_after_pdf",
			"template_name":   string(TemplateAskDocuments),
			"passenger_count": context.PassengerCount,
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":    reply,
			"intent":        string(IntentPassengerDocumentsProvided),
			"action":        "ask_photo_or_typed_document_after_pdf",
			"template_name": string(TemplateAskDocuments),
		},
	}
}

func buildPassengerDocumentConfirmationDraftRun(context BookingDraftContext) RunAgentResult {
	result := documentExtractResultFromBookingDraft(context)
	reply := buildConfirmExtractedDocumentReply(result)
	return RunAgentResult{
		ReplyText: reply,
		Model:     "template_realizer",
		RequestPayload: map[string]interface{}{
			"mode":                     "TEMPLATE_FIRST_REPLY",
			"template_name":            string(TemplateConfirmDocument),
			"intent":                   string(IntentPassengerDocumentsProvided),
			"action":                   "confirm_passenger_documents",
			"passenger_count":          context.PassengerCount,
			"expected_passenger_count": result.ExpectedPassengerCount,
			"passenger_details_count":  len(result.Passengers),
		},
		ResponsePayload: map[string]interface{}{
			"reply_text":               reply,
			"template_name":            string(TemplateConfirmDocument),
			"intent":                   string(IntentPassengerDocumentsProvided),
			"action":                   "confirm_passenger_documents",
			"expected_passenger_count": result.ExpectedPassengerCount,
			"passenger_details_count":  len(result.Passengers),
		},
	}
}

func documentExtractResultFromBookingDraft(context BookingDraftContext) DocumentExtractResult {
	expected := expectedPassengerDocumentCount(context)
	if expected <= 0 {
		expected = len(context.PassengerDetails)
	}
	passengers := make([]DocumentExtractPassenger, 0, len(context.PassengerDetails))
	for _, passenger := range context.PassengerDetails {
		docType := normalizePassengerDocumentType(passenger.DocumentType)
		document := normalizePassengerDocumentValue(passenger.Document, docType)
		item := DocumentExtractPassenger{
			Name:                   strings.TrimSpace(passenger.Name),
			DocumentType:           docType,
			Document:               document,
			CPF:                    normalizePassengerDocumentValue(passenger.CPF, "CPF"),
			RG:                     normalizePassengerDocumentValue(passenger.RG, "RG"),
			CNH:                    normalizePassengerDocumentValue(passenger.CNH, "CNH"),
			BirthDate:              strings.TrimSpace(passenger.BirthDate),
			BirthCertificateNumber: normalizePassengerDocumentValue(passenger.BirthCertificateNumber, "CERTIDAO_NASCIMENTO"),
			BirthCity:              normalizePassengerBirthCity(passenger.BirthCity),
			IsLapChild:             passenger.IsLapChild,
			Confidence:             1,
		}
		switch docType {
		case "CPF":
			item.CPF = document
		case "RG":
			item.RG = document
		case "CNH":
			item.CNH = document
		case "CERTIDAO_NASCIMENTO":
			item.BirthCertificateNumber = firstNonEmpty(item.BirthCertificateNumber, document)
		}
		passengers = append(passengers, item)
	}
	return DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: expected,
		Passengers:             passengers,
	}
}

func shouldTreatAsPassengerDocumentFlow(phase ConversationPhase, history []Message, currentTurn string, session Session) bool {
	if phase == ConversationPhaseBookingPending || phase == ConversationPhasePassengerCollection {
		return true
	}
	if lastAssistantAskedDocumentConfirmation(history) || lastAssistantAskedBookingProceedConfirmation(history) {
		return true
	}
	for i := len(history) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(history[i].Direction), "OUTBOUND") {
			continue
		}
		folded := strings.Join(strings.Fields(foldChatText(history[i].Body)), " ")
		if looksLikePassengerDocumentRequest(folded) {
			return true
		}
		break
	}
	return looksLikePassengerDocumentText(currentTurn, session)
}

func looksLikeAlreadySentPassengerDocumentsReply(text string) bool {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return false
	}
	phrases := []string{
		"mas ja enviei",
		"ja enviei",
		"ja mandei",
		"enviei sim",
		"mandei acima",
		"ja foi",
		"mande acima",
		"esta acima",
		"ta acima",
	}
	return containsFoldedPhrase(folded, phrases)
}

func shouldReplyPassengerDocumentsAlreadySentNotRecognized(currentTurn string, context BookingDraftContext) bool {
	return looksLikeAlreadySentPassengerDocumentsReply(currentTurn) &&
		context.RequestedPassengerDocuments &&
		!context.HasPassengerDetails &&
		context.PartialPassengerDetailsCount == 0
}

func lastAssistantAskedPassengerDocumentRequest(history []Message) bool {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
			continue
		}
		folded := strings.Join(strings.Fields(foldChatText(message.Body)), " ")
		if folded == "" {
			continue
		}
		return looksLikePassengerDocumentRequest(folded)
	}
	return false
}

func buildPassengerDocumentsAlreadySentNotRecognizedReply(context BookingDraftContext) string {
	expected := expectedPassengerDocumentCount(context)
	if expected <= 0 {
		expected = 1
	}
	if expected == 1 {
		return "Vi que voce enviou uma mensagem antes, mas nao consegui identificar os dados do passageiro. Pode enviar em uma linha? Exemplo: Nome completo - CPF."
	}
	return fmt.Sprintf("Vi que voce enviou uma mensagem antes, mas nao consegui identificar os dados dos %d passageiros. Pode enviar em uma linha por passageiro? Exemplo: Nome completo - CPF.", expected)
}

func bookingContinuationTemplateName(action BookingNextAction, context BookingDraftContext) ResponseTemplateName {
	switch action {
	case BookingNextAskPassengerClarification:
		if context.PassengerCountKnown && context.PassengerCount > 0 && !context.ChildUnder5CountKnown {
			return TemplateAskChildUnder5
		}
		return TemplateAskPassengerCount
	case BookingNextAskPassengerDocuments:
		return TemplateAskDocuments
	case BookingNextAskLapChildAssignment:
		return TemplateAskLapChildAssignment
	case BookingNextAskBookingPaymentPreference:
		return TemplateAskPaymentChoice
	case BookingNextAwaitTripSelection:
		return TemplateAvailabilityList
	default:
		return ""
	}
}

func messageToolContexts(message Message) []map[string]interface{} {
	contexts := make([]map[string]interface{}, 0, 2)
	for _, raw := range []interface{}{message.Payload["tool_context"], message.NormalizedPayload["tool_context"]} {
		context := asMap(raw)
		if len(context) == 0 {
			continue
		}
		contexts = append(contexts, context)
	}
	return contexts
}

func selectedAvailabilityResultFromMessage(message Message) map[string]interface{} {
	if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") ||
		!shouldMergeAvailabilityFactsFromMessage(message) {
		return nil
	}
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if len(payload) == 0 {
			continue
		}
		if snapshot := selectedAvailabilityResultFromPayload(payload); len(snapshot) > 0 {
			return snapshot
		}
	}
	return nil
}

func selectedAvailabilityResultFromPayload(payload map[string]interface{}) map[string]interface{} {
	if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) > 0 {
		return snapshot
	}
	for _, key := range []string{"request_payload", "response_payload"} {
		nested := asMap(payload[key])
		if snapshot := asMap(nested[selectedAvailabilityResultPayloadKey]); len(snapshot) > 0 {
			return snapshot
		}
	}
	return nil
}

func shouldMergeSelectedAvailabilityItemForProjection(
	policy bookingDraftAvailabilityProjectionPolicy,
	selection availabilitySelectionEvidence,
	context BookingDraftContext,
) bool {
	if !context.HasBookableSelection || context.SelectedOptionIndex <= 0 {
		return false
	}
	switch policy {
	case bookingDraftAvailabilityBookableSelection:
		return true
	case bookingDraftAvailabilityEnvelopeOnly:
		return selection.bookable()
	default:
		return false
	}
}

func mergeAvailabilityEnvelopeIntoBookingDraft(context *BookingDraftContext, payload map[string]interface{}) {
	if context == nil {
		return
	}
	if context.HasBookableSelection {
		return
	}

	if origin := strings.TrimSpace(asString(payload["origin"])); origin != "" && context.Origin == "" {
		context.Origin = origin
	}
	if destination := strings.TrimSpace(asString(payload["destination"])); destination != "" && context.Destination == "" {
		context.Destination = destination
	}
	if tripDate := strings.TrimSpace(asString(payload["trip_date"])); tripDate != "" && context.TripDate == "" {
		context.TripDate = tripDate
	}
}

func mergeSelectedAvailabilityItemIntoBookingDraft(context *BookingDraftContext, payload map[string]interface{}) {
	if context == nil || !context.HasBookableSelection || context.SelectedOptionIndex <= 0 {
		return
	}

	item, ok := selectedAvailabilityPayloadItem(payload, context.SelectedOptionIndex)
	if !ok || !selectedAvailabilitySnapshotMaterializesIndex(item, context.SelectedOptionIndex) {
		return
	}

	if context.Origin == "" {
		context.Origin = strings.TrimSpace(asString(item["origin_display_name"]))
	}
	if context.Destination == "" {
		context.Destination = strings.TrimSpace(asString(item["destination_display_name"]))
	}
	if context.PackageName == "" {
		context.PackageName = strings.TrimSpace(asString(item["package_name"]))
	}
	if context.TripID == "" {
		context.TripID = strings.TrimSpace(asString(item["trip_id"]))
	}
	if context.BoardStopID == "" {
		context.BoardStopID = strings.TrimSpace(asString(item["board_stop_id"]))
	}
	if context.AlightStopID == "" {
		context.AlightStopID = strings.TrimSpace(asString(item["alight_stop_id"]))
	}
	if context.TripDate == "" {
		context.TripDate = strings.TrimSpace(asString(item["trip_date"]))
	}
	if context.DepartureTime == "" {
		context.DepartureTime = strings.TrimSpace(asString(item["origin_depart_time"]))
	}
	if context.Price == 0 {
		context.Price = asFloat64(item["price"])
	}
	if context.Currency == "" {
		context.Currency = strings.TrimSpace(asString(item["currency"]))
	}
}

func mergeSelectedAvailabilitySnapshotIntoBookingDraft(context *BookingDraftContext, payload map[string]interface{}) {
	if context == nil || !context.HasBookableSelection || context.SelectedOptionIndex <= 0 || len(payload) == 0 {
		return
	}
	if !selectedAvailabilitySnapshotMatchesBookingDraft(*context, payload) {
		return
	}
	if index := asInt(payload["selected_option_index"]); index > 0 && index != context.SelectedOptionIndex {
		return
	}
	if origin := firstNonEmpty(
		strings.TrimSpace(asString(payload["origin"])),
		strings.TrimSpace(asString(payload["origin_display_name"])),
	); origin != "" && context.Origin == "" {
		context.Origin = origin
	}
	if destination := firstNonEmpty(
		strings.TrimSpace(asString(payload["destination"])),
		strings.TrimSpace(asString(payload["destination_display_name"])),
	); destination != "" && context.Destination == "" {
		context.Destination = destination
	}
	if packageName := strings.TrimSpace(asString(payload["package_name"])); packageName != "" && context.PackageName == "" {
		context.PackageName = packageName
	}
	if tripID := strings.TrimSpace(asString(payload["trip_id"])); tripID != "" && context.TripID == "" {
		context.TripID = tripID
	}
	if boardStopID := strings.TrimSpace(asString(payload["board_stop_id"])); boardStopID != "" && context.BoardStopID == "" {
		context.BoardStopID = boardStopID
	}
	if alightStopID := strings.TrimSpace(asString(payload["alight_stop_id"])); alightStopID != "" && context.AlightStopID == "" {
		context.AlightStopID = alightStopID
	}
	if tripDate := strings.TrimSpace(asString(payload["trip_date"])); tripDate != "" && context.TripDate == "" {
		context.TripDate = tripDate
	}
	if departureTime := strings.TrimSpace(asString(payload["origin_depart_time"])); departureTime != "" && context.DepartureTime == "" {
		context.DepartureTime = departureTime
	}
	if context.Price == 0 {
		context.Price = asFloat64(payload["price"])
	}
	if currency := strings.TrimSpace(asString(payload["currency"])); currency != "" && context.Currency == "" {
		context.Currency = currency
	}
}

func applySelectedAvailabilityResultToBookingDraft(
	context BookingDraftContext,
	availability *AvailabilitySearchResult,
	selectedOptionIndex int,
) BookingDraftContext {
	item, ok := selectedAvailabilityItemForMaterialization(availability, selectedOptionIndex)
	if !ok {
		return context
	}
	return applyBookableAvailabilityItemToBookingDraft(context, selectedOptionIndex, item)
}

func applyBookableAvailabilityItemToBookingDraft(
	context BookingDraftContext,
	selectedOptionIndex int,
	item AvailabilitySearchItem,
) BookingDraftContext {
	if selectedOptionIndex <= 0 || !hasCompleteAvailabilitySearchItemFacts(item) {
		return context
	}
	context.HasAvailabilityShown = true
	context.HasBookableSelection = true
	context.SelectedOptionIndex = selectedOptionIndex
	context.Origin = strings.TrimSpace(item.OriginDisplayName)
	context.Destination = strings.TrimSpace(item.DestinationDisplayName)
	context.PackageName = strings.TrimSpace(item.PackageName)
	context.SelectionMessageID = ""
	context.AvailabilityPromptSourceMessageID = ""
	context.TripID = strings.TrimSpace(item.TripID)
	context.BoardStopID = strings.TrimSpace(item.BoardStopID)
	context.AlightStopID = strings.TrimSpace(item.AlightStopID)
	context.TripDate = strings.TrimSpace(item.TripDate)
	context.DepartureTime = strings.TrimSpace(item.OriginDepartTime)
	context.Price = item.Price
	context.Currency = strings.TrimSpace(item.Currency)
	return context
}

func selectedAvailabilitySnapshotMatchesBookingDraft(context BookingDraftContext, payload map[string]interface{}) bool {
	currentTripID := strings.TrimSpace(context.TripID)
	if currentTripID == "" {
		return true
	}
	snapshotTripID := strings.TrimSpace(asString(payload["trip_id"]))
	if snapshotTripID == "" || snapshotTripID != currentTripID {
		return false
	}
	return selectedAvailabilitySnapshotIDMatches(context.BoardStopID, payload["board_stop_id"]) &&
		selectedAvailabilitySnapshotIDMatches(context.AlightStopID, payload["alight_stop_id"])
}

func selectedAvailabilitySnapshotIDMatches(current string, snapshot interface{}) bool {
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	snapshotValue := strings.TrimSpace(asString(snapshot))
	return snapshotValue == "" || snapshotValue == current
}

func attachSelectedAvailabilityResultToTemplateRun(run RunAgentResult, availability *AvailabilitySearchResult, decision IntentDecision) RunAgentResult {
	if decision.Intent != IntentSelectAvailabilityOption ||
		decision.SelectedOptionIndex <= 0 {
		return run
	}
	snapshot := selectedAvailabilityResultPayloadFromAvailability(availability, decision.SelectedOptionIndex)
	if len(snapshot) == 0 {
		return run
	}
	if promptSourceMessageID := strings.TrimSpace(decision.AvailabilityPromptSourceMessageID); promptSourceMessageID != "" {
		snapshot[availabilityPromptSourceMessageIDPayloadKey] = promptSourceMessageID
	}
	snapshot[availabilitySelectionMaterializesAuthorityPayloadKey] = false
	if run.RequestPayload == nil {
		run.RequestPayload = map[string]interface{}{}
	}
	if run.ResponsePayload == nil {
		run.ResponsePayload = map[string]interface{}{}
	}
	run.RequestPayload["selected_option_index"] = decision.SelectedOptionIndex
	run.ResponsePayload["selected_option_index"] = decision.SelectedOptionIndex
	run.RequestPayload[selectedAvailabilityResultPayloadKey] = cloneMap(snapshot)
	run.ResponsePayload[selectedAvailabilityResultPayloadKey] = cloneMap(snapshot)
	return run
}

func attachAvailabilitySelectionMessageIDToTemplateRun(run RunAgentResult, selectionMessageID string) RunAgentResult {
	selectionMessageID = strings.TrimSpace(selectionMessageID)
	if selectionMessageID == "" {
		return run
	}
	for _, payload := range []map[string]interface{}{run.RequestPayload, run.ResponsePayload} {
		snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
		if len(snapshot) == 0 {
			continue
		}
		if strings.TrimSpace(asString(snapshot[selectedAvailabilitySelectionMessageIDPayloadKey])) == "" {
			snapshot[selectedAvailabilitySelectionMessageIDPayloadKey] = selectionMessageID
		}
	}
	return run
}

func selectedAvailabilityResultPayloadFromAvailability(availability *AvailabilitySearchResult, selectedOptionIndex int) map[string]interface{} {
	item, ok := selectedAvailabilityItemForMaterialization(availability, selectedOptionIndex)
	if !ok {
		return nil
	}
	return selectedAvailabilityResultPayloadFromItem(item, selectedOptionIndex, availability.Filter)
}

func selectedAvailabilityResultPayloadFromBookingDraft(context BookingDraftContext) map[string]interface{} {
	if !context.HasBookableSelection || context.SelectedOptionIndex <= 0 || !hasCompleteSelectedTripFacts(
		context.TripID,
		context.BoardStopID,
		context.AlightStopID,
	) {
		return nil
	}
	snapshot := map[string]interface{}{
		"selected_option_index":    context.SelectedOptionIndex,
		"trip_id":                  strings.TrimSpace(context.TripID),
		"board_stop_id":            strings.TrimSpace(context.BoardStopID),
		"alight_stop_id":           strings.TrimSpace(context.AlightStopID),
		"origin":                   strings.TrimSpace(context.Origin),
		"destination":              strings.TrimSpace(context.Destination),
		"package_name":             strings.TrimSpace(context.PackageName),
		"origin_display_name":      strings.TrimSpace(context.Origin),
		"destination_display_name": strings.TrimSpace(context.Destination),
		"origin_depart_time":       strings.TrimSpace(context.DepartureTime),
		"trip_date":                strings.TrimSpace(context.TripDate),
		"price":                    context.Price,
		"currency":                 strings.TrimSpace(context.Currency),
		availabilitySelectionMaterializesAuthorityPayloadKey: false,
	}
	if selectionMessageID := strings.TrimSpace(context.SelectionMessageID); selectionMessageID != "" {
		snapshot[selectedAvailabilitySelectionMessageIDPayloadKey] = selectionMessageID
	}
	if promptSourceMessageID := strings.TrimSpace(context.AvailabilityPromptSourceMessageID); promptSourceMessageID != "" {
		snapshot[availabilityPromptSourceMessageIDPayloadKey] = promptSourceMessageID
	}
	return snapshot
}

func selectedAvailabilityItemForMaterialization(
	availability *AvailabilitySearchResult,
	selectedOptionIndex int,
) (AvailabilitySearchItem, bool) {
	if availability == nil || selectedOptionIndex <= 0 || selectedOptionIndex > len(availability.Results) {
		return AvailabilitySearchItem{}, false
	}
	item := availability.Results[selectedOptionIndex-1]
	if !hasCompleteAvailabilitySearchItemFacts(item) {
		return AvailabilitySearchItem{}, false
	}
	item.PackageName = firstNonEmpty(
		strings.TrimSpace(item.PackageName),
		strings.TrimSpace(availability.Filter.PackageName),
	)
	return item, true
}

func selectedAvailabilityResultPayloadFromItem(item AvailabilitySearchItem, selectedOptionIndex int, filter AvailabilitySearchInput) map[string]interface{} {
	return map[string]interface{}{
		"selected_option_index":    selectedOptionIndex,
		"segment_id":               strings.TrimSpace(item.SegmentID),
		"trip_id":                  strings.TrimSpace(item.TripID),
		"route_id":                 strings.TrimSpace(item.RouteID),
		"board_stop_id":            strings.TrimSpace(item.BoardStopID),
		"alight_stop_id":           strings.TrimSpace(item.AlightStopID),
		"origin_stop_id":           strings.TrimSpace(item.OriginStopID),
		"destination_stop_id":      strings.TrimSpace(item.DestinationStopID),
		"origin":                   strings.TrimSpace(filter.Origin),
		"destination":              strings.TrimSpace(filter.Destination),
		"package_name":             firstNonEmpty(strings.TrimSpace(item.PackageName), strings.TrimSpace(filter.PackageName)),
		"origin_display_name":      strings.TrimSpace(item.OriginDisplayName),
		"destination_display_name": strings.TrimSpace(item.DestinationDisplayName),
		"origin_depart_time":       strings.TrimSpace(item.OriginDepartTime),
		"trip_date":                strings.TrimSpace(item.TripDate),
		"seats_available":          item.SeatsAvailable,
		"price":                    item.Price,
		"currency":                 strings.TrimSpace(item.Currency),
		"status":                   strings.TrimSpace(item.Status),
		"trip_status":              strings.TrimSpace(item.TripStatus),
	}
}

func looksLikePassengerDocumentRequest(folded string) bool {
	return strings.Contains(folded, "foto legivel do documento") ||
		(strings.Contains(folded, "nome completo") && strings.Contains(folded, "documento")) ||
		(strings.Contains(folded, "nomes completos") && strings.Contains(folded, "documentos")) ||
		(strings.Contains(folded, "envie") && strings.Contains(folded, "documento")) ||
		(strings.Contains(folded, "pode enviar") && strings.Contains(folded, "documento"))
}

func looksLikePaymentPreferencePrompt(folded string) bool {
	return strings.Contains(folded, "valor integral") ||
		strings.Contains(folded, "apenas o sinal") ||
		strings.Contains(folded, "sinal de r$ 250") ||
		strings.Contains(folded, "prefere pagar")
}
