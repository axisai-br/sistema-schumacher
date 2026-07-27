package chat

import "strings"

type ConversationPhase string

const (
	ConversationPhaseDiscovery           ConversationPhase = "DISCOVERY"
	ConversationPhaseRouteSelection      ConversationPhase = "ROUTE_SELECTION"
	ConversationPhaseTripSelection       ConversationPhase = "TRIP_SELECTION"
	ConversationPhasePassengerCollection ConversationPhase = "PASSENGER_COLLECTION"
	ConversationPhaseBookingPending      ConversationPhase = "BOOKING_PENDING"
	ConversationPhaseBooked              ConversationPhase = "BOOKED"
	ConversationPhasePaymentPending      ConversationPhase = "PAYMENT_PENDING"
	ConversationPhasePaidPartial         ConversationPhase = "PAID_PARTIAL"
	ConversationPhasePaidFull            ConversationPhase = "PAID_FULL"
	ConversationPhasePostSale            ConversationPhase = "POST_SALE"
	ConversationPhaseHandoffHuman        ConversationPhase = "HANDOFF_HUMAN"
)

type CanonicalConversationState struct {
	SessionID          string                  `json:"session_id"`
	Phase              ConversationPhase       `json:"phase"`
	Route              CanonicalRouteState     `json:"route"`
	Passengers         CanonicalPassengerState `json:"passengers"`
	Booking            CanonicalBookingState   `json:"booking"`
	Payment            CanonicalPaymentState   `json:"payment"`
	LastToolFacts      map[string]interface{}  `json:"last_tool_facts,omitempty"`
	AllowedNextActions []string                `json:"allowed_next_actions,omitempty"`
}

type CanonicalRouteState struct {
	Origin              string  `json:"origin,omitempty"`
	Destination         string  `json:"destination,omitempty"`
	PackageName         string  `json:"package_name,omitempty"`
	TripDate            string  `json:"trip_date,omitempty"`
	DepartureTime       string  `json:"departure_time,omitempty"`
	Price               float64 `json:"price,omitempty"`
	Currency            string  `json:"currency,omitempty"`
	SelectedOptionIndex int     `json:"selected_option_index,omitempty"`
	TripID              string  `json:"trip_id,omitempty"`
	BoardStopID         string  `json:"board_stop_id,omitempty"`
	AlightStopID        string  `json:"alight_stop_id,omitempty"`
}

type CanonicalPassengerState struct {
	ExpectedCount      int  `json:"expected_count,omitempty"`
	ChildUnder5Count   int  `json:"child_under_5_count,omitempty"`
	DocumentsCollected bool `json:"documents_collected,omitempty"`
}

type CanonicalBookingState struct {
	BookingID       string `json:"booking_id,omitempty"`
	ReservationCode string `json:"reservation_code,omitempty"`
	Status          string `json:"status,omitempty"`
}

type CanonicalPaymentState struct {
	Status     string `json:"status,omitempty"`
	Preference string `json:"preference,omitempty"`
}

func deriveCanonicalConversationState(session Session, history []Message, currentTurn string) CanonicalConversationState {
	draft := collectBookingDraftContextFromState(session, history, currentTurn)
	return deriveCanonicalConversationStateFromBookingDraft(session, history, draft)
}

func deriveCanonicalConversationStateForRoutingBaseline(session Session, history []Message) CanonicalConversationState {
	draft := collectBookingDraftContextForRoutingBaseline(session, history)
	return deriveCanonicalConversationStateFromBookingDraft(session, history, draft)
}

func deriveCanonicalConversationStateFromBookingDraft(
	session Session,
	history []Message,
	draft BookingDraftContext,
) CanonicalConversationState {
	state := CanonicalConversationState{
		SessionID:          strings.TrimSpace(session.ID),
		Phase:              ConversationPhaseDiscovery,
		LastToolFacts:      map[string]interface{}{},
		AllowedNextActions: []string{},
	}
	if strings.EqualFold(strings.TrimSpace(session.HandoffStatus), "HUMAN") || strings.TrimSpace(session.CurrentOwnerUserID) != "" {
		state.Phase = ConversationPhaseHandoffHuman
	}
	state.Route.Origin = draft.Origin
	state.Route.Destination = draft.Destination
	state.Route.PackageName = draft.PackageName
	state.Route.SelectedOptionIndex = draft.SelectedOptionIndex
	state.Route.TripID = draft.TripID
	state.Route.BoardStopID = draft.BoardStopID
	state.Route.AlightStopID = draft.AlightStopID
	state.Route.TripDate = draft.TripDate
	state.Route.DepartureTime = draft.DepartureTime
	state.Route.Price = draft.Price
	state.Route.Currency = draft.Currency
	state.Passengers.ExpectedCount = draft.PassengerCount
	state.Passengers.ChildUnder5Count = draft.ChildUnder5Count
	state.Passengers.DocumentsCollected = draft.HasPassengerDetails

	for i := 0; i < len(history); i++ {
		message := history[i]
		for _, toolContext := range messageToolContexts(message) {
			if draft.HasBookableSelection {
				toolContext = withoutAvailabilityToolFacts(toolContext)
			}
			mergeMessageToolFactsIntoCanonicalState(&state, message, toolContext)
		}
	}
	if selectedAvailability := selectedAvailabilityToolFactsFromBookingDraft(draft); len(selectedAvailability) > 0 {
		state.LastToolFacts[toolNameAvailabilitySearch] = selectedAvailability
	}
	state.Phase = inferConversationPhase(state, draft)
	state.AllowedNextActions = allowedNextActionsForPhase(state.Phase)
	return state
}

func selectedAvailabilityToolFactsFromBookingDraft(draft BookingDraftContext) map[string]interface{} {
	selected := selectedAvailabilityResultPayloadFromBookingDraft(draft)
	if len(selected) == 0 {
		return nil
	}
	results := make([]map[string]interface{}, draft.SelectedOptionIndex)
	for index := range results {
		results[index] = map[string]interface{}{}
	}
	results[draft.SelectedOptionIndex-1] = cloneMap(selected)

	facts := map[string]interface{}{
		"result_count":          len(results),
		"results":               results,
		"selected_option_index": draft.SelectedOptionIndex,
	}
	if origin := strings.TrimSpace(draft.Origin); origin != "" {
		facts["origin"] = origin
	}
	if destination := strings.TrimSpace(draft.Destination); destination != "" {
		facts["destination"] = destination
	}
	if packageName := strings.TrimSpace(draft.PackageName); packageName != "" {
		facts["package_name"] = packageName
	}
	if tripDate := strings.TrimSpace(draft.TripDate); tripDate != "" {
		facts["trip_date"] = tripDate
	}
	return facts
}

func mergeMessageToolFactsIntoCanonicalState(state *CanonicalConversationState, message Message, toolContext map[string]interface{}) {
	if !shouldMergeAvailabilityFactsFromMessage(message) {
		toolContext = withoutAvailabilityToolFacts(toolContext)
	}
	mergeToolFactsIntoCanonicalState(state, toolContext)
}

func shouldMergeAvailabilityFactsFromMessage(message Message) bool {
	if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") {
		return true
	}
	if isBotAutoReplyMessage(message) {
		return false
	}
	return isReliableActivePromptOutbound(message)
}

func withoutAvailabilityToolFacts(toolContext map[string]interface{}) map[string]interface{} {
	if _, ok := toolContext[toolNameAvailabilitySearch]; !ok {
		return toolContext
	}
	if len(toolContext) == 1 {
		return nil
	}
	filtered := make(map[string]interface{}, len(toolContext)-1)
	for key, value := range toolContext {
		if key == toolNameAvailabilitySearch {
			continue
		}
		filtered[key] = value
	}
	return filtered
}

func mergeToolFactsIntoCanonicalState(state *CanonicalConversationState, toolContext map[string]interface{}) {
	if state == nil {
		return
	}
	if availability := asMap(toolContext[toolNameAvailabilitySearch]); len(availability) > 0 {
		state.LastToolFacts[toolNameAvailabilitySearch] = availability
		if packageName := strings.TrimSpace(asString(availability["package_name"])); packageName != "" {
			state.Route.PackageName = packageName
		}
	}
	if booking := asMap(toolContext[toolNameBookingCreate]); len(booking) > 0 {
		state.LastToolFacts[toolNameBookingCreate] = booking
		if bookingID := strings.TrimSpace(asString(booking["booking_id"])); bookingID != "" {
			state.Booking.BookingID = bookingID
		}
		if reservationCode := strings.TrimSpace(asString(booking["reservation_code"])); reservationCode != "" {
			state.Booking.ReservationCode = reservationCode
		}
		if status := strings.TrimSpace(asString(booking["status"])); status != "" {
			state.Booking.Status = status
		}
	}
	if payment := asMap(toolContext[toolNamePaymentStatus]); len(payment) > 0 {
		state.LastToolFacts[toolNamePaymentStatus] = payment
	}
	if payment := asMap(toolContext[toolNamePaymentCreate]); len(payment) > 0 {
		state.LastToolFacts[toolNamePaymentCreate] = payment
		if status := strings.TrimSpace(asString(payment["payment_status"])); status != "" {
			state.Payment.Status = status
		}
	}
}

func invalidateCanonicalAvailabilityFacts(state CanonicalConversationState) CanonicalConversationState {
	state.Route.SelectedOptionIndex = 0
	state.Route.TripID = ""
	state.Route.BoardStopID = ""
	state.Route.AlightStopID = ""
	state.Route.TripDate = ""
	state.Route.DepartureTime = ""
	state.Route.Price = 0
	state.Route.Currency = ""
	state.Route.PackageName = ""

	if len(state.LastToolFacts) > 0 {
		facts := make(map[string]interface{}, len(state.LastToolFacts))
		for name, value := range state.LastToolFacts {
			if name == toolNameAvailabilitySearch {
				continue
			}
			facts[name] = value
		}
		state.LastToolFacts = facts
	}
	return state
}

func inferConversationPhase(state CanonicalConversationState, draft BookingDraftContext) ConversationPhase {
	if state.Phase == ConversationPhaseHandoffHuman {
		return state.Phase
	}
	if strings.TrimSpace(state.Payment.Status) != "" {
		status := strings.ToUpper(strings.TrimSpace(state.Payment.Status))
		if status == "PAID" || status == "PAID_FULL" {
			return ConversationPhasePaidFull
		}
		if status == "PAID_PARTIAL" {
			return ConversationPhasePaidPartial
		}
		return ConversationPhasePaymentPending
	}
	if strings.TrimSpace(state.Booking.BookingID) != "" || draft.BookingCreated {
		return ConversationPhaseBooked
	}
	if draft.HasPassengerDetails {
		return ConversationPhaseBookingPending
	}
	if (draft.AskedPassengerQuestion || state.Passengers.ExpectedCount > 0) &&
		(strings.TrimSpace(state.Route.TripID) != "" || state.Route.SelectedOptionIndex > 0) {
		return ConversationPhasePassengerCollection
	}
	if strings.TrimSpace(state.Route.TripID) != "" || state.Route.SelectedOptionIndex > 0 {
		return ConversationPhaseTripSelection
	}
	if draft.HasAvailabilityShown || strings.TrimSpace(state.Route.Origin) != "" || strings.TrimSpace(state.Route.Destination) != "" {
		return ConversationPhaseRouteSelection
	}
	return ConversationPhaseDiscovery
}

func allowedNextActionsForPhase(phase ConversationPhase) []string {
	switch phase {
	case ConversationPhaseDiscovery:
		return []string{string(IntentAvailabilitySearch), string(IntentHumanSupport)}
	case ConversationPhaseRouteSelection:
		return []string{string(IntentAvailabilitySearch), string(IntentSelectAvailabilityOption), string(IntentHumanSupport)}
	case ConversationPhaseTripSelection:
		return []string{string(IntentSelectAvailabilityOption), string(IntentPassengerCountReply), string(IntentHumanSupport)}
	case ConversationPhasePassengerCollection:
		return []string{string(IntentPassengerCountReply), string(IntentPassengerDocumentsProvided), string(IntentHumanSupport)}
	case ConversationPhaseBookingPending:
		return []string{string(IntentBookingCreateConfirmation), string(IntentPassengerDocumentsProvided), string(IntentHumanSupport)}
	case ConversationPhaseBooked:
		return []string{string(IntentPaymentPreference), string(IntentPaymentCreate), string(IntentPaymentStatusQuery), string(IntentBookingCancel), string(IntentHumanSupport)}
	case ConversationPhasePaymentPending:
		return []string{string(IntentPaymentStatusQuery), string(IntentHumanSupport)}
	default:
		return []string{string(IntentHumanSupport)}
	}
}

func validateConversationTransition(before ConversationPhase, after ConversationPhase, state CanonicalConversationState) bool {
	if before == after {
		return true
	}
	switch before {
	case ConversationPhaseDiscovery:
		return after == ConversationPhaseRouteSelection
	case ConversationPhaseRouteSelection:
		return after == ConversationPhaseTripSelection && hasCanonicalRoute(state)
	case ConversationPhaseTripSelection:
		return after == ConversationPhasePassengerCollection && state.Route.SelectedOptionIndex > 0
	case ConversationPhasePassengerCollection:
		return after == ConversationPhaseBookingPending && state.Passengers.ExpectedCount > 0 && state.Passengers.DocumentsCollected
	case ConversationPhaseBookingPending:
		return after == ConversationPhaseBooked && strings.TrimSpace(state.Booking.BookingID) != ""
	case ConversationPhaseBooked:
		return after == ConversationPhasePaymentPending && strings.TrimSpace(state.Payment.Preference) != ""
	case ConversationPhasePaymentPending:
		return (after == ConversationPhasePaidPartial || after == ConversationPhasePaidFull) && strings.TrimSpace(state.Payment.Status) != ""
	default:
		return false
	}
}

func hasCanonicalRoute(state CanonicalConversationState) bool {
	return strings.TrimSpace(state.Route.Origin) != "" ||
		strings.TrimSpace(state.Route.Destination) != "" ||
		strings.TrimSpace(state.Route.PackageName) != ""
}
