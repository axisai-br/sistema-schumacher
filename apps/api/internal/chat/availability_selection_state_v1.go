package chat

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	availabilitySelectionStateV1Version                  = 1
	availabilitySelectionStateV1MemoryKey                = "availability_selection_state_v1"
	availabilitySelectionEventsV1MessageKey              = "availability_selection_events_v1"
	availabilitySelectionMaterializesAuthorityPayloadKey = "materializes_authority"
)

var ErrAvailabilitySelectionStateInvalid = errors.New("availability selection state invalid")

type AvailabilitySelectionStatusV1 string

const (
	AvailabilitySelectionStatusNone        AvailabilitySelectionStatusV1 = "NONE"
	AvailabilitySelectionStatusBookable    AvailabilitySelectionStatusV1 = "BOOKABLE"
	AvailabilitySelectionStatusRejected    AvailabilitySelectionStatusV1 = "REJECTED"
	AvailabilitySelectionStatusInvalidated AvailabilitySelectionStatusV1 = "INVALIDATED"
)

type AvailabilitySelectionEventTypeV1 string

const (
	AvailabilitySelectionEventMaterialized AvailabilitySelectionEventTypeV1 = "SELECTION_MATERIALIZED"
	AvailabilitySelectionEventRejected     AvailabilitySelectionEventTypeV1 = "SELECTION_REJECTED"
	AvailabilitySelectionEventInvalidated  AvailabilitySelectionEventTypeV1 = "SELECTION_INVALIDATED"
)

type AvailabilitySelectionSnapshotV1 struct {
	SelectedOptionIndex    int     `json:"selected_option_index"`
	SegmentID              string  `json:"segment_id,omitempty"`
	TripID                 string  `json:"trip_id"`
	RouteID                string  `json:"route_id,omitempty"`
	BoardStopID            string  `json:"board_stop_id"`
	AlightStopID           string  `json:"alight_stop_id"`
	OriginStopID           string  `json:"origin_stop_id,omitempty"`
	DestinationStopID      string  `json:"destination_stop_id,omitempty"`
	Origin                 string  `json:"origin,omitempty"`
	Destination            string  `json:"destination,omitempty"`
	PackageName            string  `json:"package_name,omitempty"`
	OriginDisplayName      string  `json:"origin_display_name,omitempty"`
	DestinationDisplayName string  `json:"destination_display_name,omitempty"`
	OriginDepartTime       string  `json:"origin_depart_time,omitempty"`
	TripDate               string  `json:"trip_date,omitempty"`
	SeatsAvailable         int     `json:"seats_available,omitempty"`
	Price                  float64 `json:"price,omitempty"`
	Currency               string  `json:"currency,omitempty"`
	Status                 string  `json:"status,omitempty"`
	TripStatus             string  `json:"trip_status,omitempty"`
}

type availabilitySelectionSnapshotPresenceV1 uint32

const (
	availabilitySelectionSnapshotSelectedOptionIndexPresentV1 availabilitySelectionSnapshotPresenceV1 = 1 << iota
	availabilitySelectionSnapshotSegmentIDPresentV1
	availabilitySelectionSnapshotTripIDPresentV1
	availabilitySelectionSnapshotRouteIDPresentV1
	availabilitySelectionSnapshotBoardStopIDPresentV1
	availabilitySelectionSnapshotAlightStopIDPresentV1
	availabilitySelectionSnapshotOriginStopIDPresentV1
	availabilitySelectionSnapshotDestinationStopIDPresentV1
	availabilitySelectionSnapshotOriginPresentV1
	availabilitySelectionSnapshotDestinationPresentV1
	availabilitySelectionSnapshotPackageNamePresentV1
	availabilitySelectionSnapshotOriginDisplayNamePresentV1
	availabilitySelectionSnapshotDestinationDisplayNamePresentV1
	availabilitySelectionSnapshotOriginDepartTimePresentV1
	availabilitySelectionSnapshotTripDatePresentV1
	availabilitySelectionSnapshotSeatsAvailablePresentV1
	availabilitySelectionSnapshotPricePresentV1
	availabilitySelectionSnapshotCurrencyPresentV1
	availabilitySelectionSnapshotStatusPresentV1
	availabilitySelectionSnapshotTripStatusPresentV1
)

var availabilitySelectionSnapshotFieldsV1 = []struct {
	key      string
	presence availabilitySelectionSnapshotPresenceV1
	kind     availabilitySelectionSnapshotFieldKindV1
}{
	{key: "selected_option_index", presence: availabilitySelectionSnapshotSelectedOptionIndexPresentV1, kind: availabilitySelectionSnapshotFieldIntV1},
	{key: "segment_id", presence: availabilitySelectionSnapshotSegmentIDPresentV1},
	{key: "trip_id", presence: availabilitySelectionSnapshotTripIDPresentV1},
	{key: "route_id", presence: availabilitySelectionSnapshotRouteIDPresentV1},
	{key: "board_stop_id", presence: availabilitySelectionSnapshotBoardStopIDPresentV1},
	{key: "alight_stop_id", presence: availabilitySelectionSnapshotAlightStopIDPresentV1},
	{key: "origin_stop_id", presence: availabilitySelectionSnapshotOriginStopIDPresentV1},
	{key: "destination_stop_id", presence: availabilitySelectionSnapshotDestinationStopIDPresentV1},
	{key: "origin", presence: availabilitySelectionSnapshotOriginPresentV1},
	{key: "destination", presence: availabilitySelectionSnapshotDestinationPresentV1},
	{key: "package_name", presence: availabilitySelectionSnapshotPackageNamePresentV1},
	{key: "origin_display_name", presence: availabilitySelectionSnapshotOriginDisplayNamePresentV1},
	{key: "destination_display_name", presence: availabilitySelectionSnapshotDestinationDisplayNamePresentV1},
	{key: "origin_depart_time", presence: availabilitySelectionSnapshotOriginDepartTimePresentV1},
	{key: "trip_date", presence: availabilitySelectionSnapshotTripDatePresentV1},
	{key: "seats_available", presence: availabilitySelectionSnapshotSeatsAvailablePresentV1, kind: availabilitySelectionSnapshotFieldIntV1},
	{key: "price", presence: availabilitySelectionSnapshotPricePresentV1, kind: availabilitySelectionSnapshotFieldFloatV1},
	{key: "currency", presence: availabilitySelectionSnapshotCurrencyPresentV1},
	{key: "status", presence: availabilitySelectionSnapshotStatusPresentV1},
	{key: "trip_status", presence: availabilitySelectionSnapshotTripStatusPresentV1},
}

type availabilitySelectionSnapshotFieldKindV1 uint8

const (
	availabilitySelectionSnapshotFieldStringV1 availabilitySelectionSnapshotFieldKindV1 = iota
	availabilitySelectionSnapshotFieldIntV1
	availabilitySelectionSnapshotFieldFloatV1
)

type AvailabilitySelectionEventOrderV1 struct {
	ReceivedAt   time.Time `json:"received_at"`
	CreatedAt    time.Time `json:"created_at"`
	MessageID    string    `json:"message_id"`
	EventOrdinal int       `json:"event_ordinal"`
}

type AvailabilitySelectionTombstoneV1 struct {
	EventID                           string                            `json:"event_id"`
	EventMessageID                    string                            `json:"event_message_id"`
	EventOrder                        AvailabilitySelectionEventOrderV1 `json:"event_order"`
	AvailabilityPromptSourceMessageID string                            `json:"availability_prompt_source_message_id,omitempty"`
	RejectedOptionIndexes             []int                             `json:"rejected_option_indexes,omitempty"`
	RejectedTripDates                 []string                          `json:"rejected_trip_dates,omitempty"`
	RejectedWholeContext              bool                              `json:"rejected_whole_context"`
	ReasonCode                        string                            `json:"reason_code,omitempty"`
}

// AvailabilitySelectionStateV1 is the durable authority for booking selection.
// Once persisted, bounded history and message text are never reducer inputs.
type AvailabilitySelectionStateV1 struct {
	Version                           int                                `json:"version"`
	Status                            AvailabilitySelectionStatusV1      `json:"status"`
	SelectionEventMessageID           string                             `json:"selection_event_message_id,omitempty"`
	SelectionProjectionMessageID      string                             `json:"selection_projection_message_id,omitempty"`
	AvailabilityPromptSourceMessageID string                             `json:"availability_prompt_source_message_id,omitempty"`
	SelectedOptionIndex               int                                `json:"selected_option_index,omitempty"`
	Snapshot                          AvailabilitySelectionSnapshotV1    `json:"snapshot,omitempty"`
	Tombstone                         *AvailabilitySelectionTombstoneV1  `json:"tombstone,omitempty"`
	Rejections                        []AvailabilitySelectionTombstoneV1 `json:"rejections,omitempty"`
	AppliedEventIDs                   []string                           `json:"applied_event_ids,omitempty"`
	LastAppliedEventOrder             AvailabilitySelectionEventOrderV1  `json:"last_applied_event_order,omitempty"`
	BootstrapCompleted                bool                               `json:"bootstrap_completed"`
}

type AvailabilitySelectionEventV1 struct {
	Type                              AvailabilitySelectionEventTypeV1  `json:"type"`
	EventID                           string                            `json:"event_id,omitempty"`
	MessageID                         string                            `json:"message_id"`
	ProjectionMessageID               string                            `json:"projection_message_id,omitempty"`
	AvailabilityPromptSourceMessageID string                            `json:"availability_prompt_source_message_id,omitempty"`
	SelectedOptionIndex               int                               `json:"selected_option_index,omitempty"`
	Snapshot                          AvailabilitySelectionSnapshotV1   `json:"snapshot,omitempty"`
	RejectedOptionIndexes             []int                             `json:"rejected_option_indexes,omitempty"`
	RejectedTripDates                 []string                          `json:"rejected_trip_dates,omitempty"`
	RejectedWholeContext              bool                              `json:"rejected_whole_context,omitempty"`
	ReasonCode                        string                            `json:"reason_code,omitempty"`
	MaterializesAuthority             bool                              `json:"materializes_authority"`
	Order                             AvailabilitySelectionEventOrderV1 `json:"order"`
	snapshotPresence                  availabilitySelectionSnapshotPresenceV1
	snapshotPresenceKnown             bool
}

func (event AvailabilitySelectionEventV1) MarshalJSON() ([]byte, error) {
	type eventJSON AvailabilitySelectionEventV1
	if event.Type != AvailabilitySelectionEventMaterialized || !event.snapshotPresenceKnown {
		return json.Marshal(eventJSON(event))
	}

	raw, err := json.Marshal(eventJSON(event))
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if event.snapshotPresence == 0 {
		delete(payload, "snapshot")
		return json.Marshal(payload)
	}
	snapshotPayload := event.Snapshot.payload()
	persistedSnapshot := make(map[string]interface{}, len(availabilitySelectionSnapshotFieldsV1))
	for _, field := range availabilitySelectionSnapshotFieldsV1 {
		if event.snapshotPresence&field.presence != 0 {
			persistedSnapshot[field.key] = snapshotPayload[field.key]
		}
	}
	payload["snapshot"] = persistedSnapshot
	return json.Marshal(payload)
}

func newAvailabilitySelectionStateV1() AvailabilitySelectionStateV1 {
	return AvailabilitySelectionStateV1{
		Version: availabilitySelectionStateV1Version,
		Status:  AvailabilitySelectionStatusNone,
	}
}

func availabilitySelectionStateV1FromSession(session Session) (AvailabilitySelectionStateV1, bool) {
	memory := asMap(session.Metadata["memory"])
	return decodeAvailabilitySelectionStateV1(memory[availabilitySelectionStateV1MemoryKey])
}

func availabilitySelectionStateV1ValueFromSession(session Session) (interface{}, bool) {
	memory := asMap(session.Metadata["memory"])
	value, ok := memory[availabilitySelectionStateV1MemoryKey]
	return value, ok
}

func availabilitySelectionStateV1ForRead(session Session, history []Message) AvailabilitySelectionStateV1 {
	state := newAvailabilitySelectionStateV1()
	state.BootstrapCompleted = true
	if value, present := availabilitySelectionStateV1ValueFromSession(session); present {
		persisted, ok := decodeAvailabilitySelectionStateV1(value)
		if !ok {
			return invalidAvailabilitySelectionStateV1("INVALID_PERSISTED_STATE")
		}
		state = persisted
	}
	barrierIndex := latestDeliveredInvalidAvailabilityPromptIndexV1(history, len(history))
	if barrierIndex < 0 {
		return state
	}
	barrierOrder := availabilitySelectionEventOrderFromMessageV1(history[barrierIndex], 0)
	if !state.LastAppliedEventOrder.known() ||
		compareAvailabilitySelectionEventOrderV1(state.LastAppliedEventOrder, barrierOrder) <= 0 ||
		state.Status == AvailabilitySelectionStatusBookable &&
			!availabilitySelectionBookableAuthorityAfterBarrierV1(state, history, barrierIndex) {
		state = newAvailabilitySelectionStateV1()
		state.BootstrapCompleted = true
	}
	return state
}

func availabilitySelectionBookableAuthorityAfterBarrierV1(
	state AvailabilitySelectionStateV1,
	history []Message,
	barrierIndex int,
) bool {
	sourceMessageID := strings.TrimSpace(state.AvailabilityPromptSourceMessageID)
	selectionMessageID := strings.TrimSpace(state.SelectionEventMessageID)
	if sourceMessageID == "" || selectionMessageID == "" {
		return false
	}
	sourceIndex := -1
	selectionIndex := -1
	for index := barrierIndex + 1; index < len(history); index++ {
		message := history[index]
		messageID := strings.TrimSpace(message.ID)
		if messageID == sourceMessageID {
			classified, _, ok := classifiedAvailabilityPromptMessageAtV1(history, index)
			if !ok ||
				availabilityPromptSourceMessageIDFromMessage(classified) != sourceMessageID {
				return false
			}
			sourceIndex = index
		}
		if messageID == selectionMessageID {
			if !strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
				return false
			}
			selectionIndex = index
		}
	}
	return sourceIndex >= 0 && selectionIndex > sourceIndex
}

func decodeAvailabilitySelectionStateV1(value interface{}) (AvailabilitySelectionStateV1, bool) {
	if value == nil {
		return AvailabilitySelectionStateV1{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return AvailabilitySelectionStateV1{}, false
	}
	var state AvailabilitySelectionStateV1
	if err := json.Unmarshal(raw, &state); err != nil || state.Version != availabilitySelectionStateV1Version {
		return AvailabilitySelectionStateV1{}, false
	}
	state = normalizeAvailabilitySelectionStateV1(state)
	if validateAvailabilitySelectionStateV1(state) != nil {
		return AvailabilitySelectionStateV1{}, false
	}
	return state, true
}

func normalizeAvailabilitySelectionStateV1(state AvailabilitySelectionStateV1) AvailabilitySelectionStateV1 {
	state.Version = availabilitySelectionStateV1Version
	if state.Status == "" {
		state.Status = AvailabilitySelectionStatusNone
	}
	state.SelectionEventMessageID = strings.TrimSpace(state.SelectionEventMessageID)
	state.SelectionProjectionMessageID = strings.TrimSpace(state.SelectionProjectionMessageID)
	state.AvailabilityPromptSourceMessageID = strings.TrimSpace(state.AvailabilityPromptSourceMessageID)
	state.Snapshot = normalizeAvailabilitySelectionSnapshotV1(state.Snapshot)
	state.AppliedEventIDs = uniqueTrimmedStrings(state.AppliedEventIDs)
	sort.Strings(state.AppliedEventIDs)
	state.LastAppliedEventOrder = normalizeAvailabilitySelectionEventOrderV1(state.LastAppliedEventOrder)
	rejections := make([]AvailabilitySelectionTombstoneV1, 0, len(state.Rejections))
	for _, rejection := range state.Rejections {
		rejection = normalizeAvailabilitySelectionTombstoneV1(rejection)
		if rejection.EventID != "" {
			rejections = append(rejections, rejection)
		}
	}
	sort.SliceStable(rejections, func(i, j int) bool {
		comparison := compareAvailabilitySelectionEventOrderV1(
			rejections[i].EventOrder,
			rejections[j].EventOrder,
		)
		if comparison != 0 {
			return comparison < 0
		}
		return rejections[i].EventID < rejections[j].EventID
	})
	state.Rejections = rejections
	if state.Tombstone != nil {
		tombstone := normalizeAvailabilitySelectionTombstoneV1(*state.Tombstone)
		state.Tombstone = &tombstone
	}
	return state
}

func normalizeAvailabilitySelectionSnapshotV1(snapshot AvailabilitySelectionSnapshotV1) AvailabilitySelectionSnapshotV1 {
	snapshot.SegmentID = strings.TrimSpace(snapshot.SegmentID)
	snapshot.TripID = strings.TrimSpace(snapshot.TripID)
	snapshot.RouteID = strings.TrimSpace(snapshot.RouteID)
	snapshot.BoardStopID = strings.TrimSpace(snapshot.BoardStopID)
	snapshot.AlightStopID = strings.TrimSpace(snapshot.AlightStopID)
	snapshot.OriginStopID = strings.TrimSpace(snapshot.OriginStopID)
	snapshot.DestinationStopID = strings.TrimSpace(snapshot.DestinationStopID)
	snapshot.Origin = strings.TrimSpace(snapshot.Origin)
	snapshot.Destination = strings.TrimSpace(snapshot.Destination)
	snapshot.PackageName = strings.TrimSpace(snapshot.PackageName)
	snapshot.OriginDisplayName = strings.TrimSpace(snapshot.OriginDisplayName)
	snapshot.DestinationDisplayName = strings.TrimSpace(snapshot.DestinationDisplayName)
	snapshot.OriginDepartTime = strings.TrimSpace(snapshot.OriginDepartTime)
	snapshot.TripDate = strings.TrimSpace(snapshot.TripDate)
	snapshot.Currency = strings.TrimSpace(snapshot.Currency)
	snapshot.Status = strings.TrimSpace(snapshot.Status)
	snapshot.TripStatus = strings.TrimSpace(snapshot.TripStatus)
	return snapshot
}

func normalizeAvailabilitySelectionTombstoneV1(tombstone AvailabilitySelectionTombstoneV1) AvailabilitySelectionTombstoneV1 {
	tombstone.EventID = strings.TrimSpace(tombstone.EventID)
	tombstone.EventMessageID = strings.TrimSpace(tombstone.EventMessageID)
	tombstone.EventOrder = normalizeAvailabilitySelectionEventOrderV1(tombstone.EventOrder)
	tombstone.AvailabilityPromptSourceMessageID = strings.TrimSpace(tombstone.AvailabilityPromptSourceMessageID)
	tombstone.ReasonCode = strings.TrimSpace(tombstone.ReasonCode)
	tombstone.RejectedOptionIndexes = uniqueAvailabilitySelectionIndexesV1(tombstone.RejectedOptionIndexes)
	tombstone.RejectedTripDates = uniqueAvailabilitySelectionTripDatesV1(tombstone.RejectedTripDates)
	return tombstone
}

func normalizeAvailabilitySelectionEventOrderV1(
	order AvailabilitySelectionEventOrderV1,
) AvailabilitySelectionEventOrderV1 {
	if !order.ReceivedAt.IsZero() {
		order.ReceivedAt = order.ReceivedAt.UTC()
	}
	if !order.CreatedAt.IsZero() {
		order.CreatedAt = order.CreatedAt.UTC()
	}
	order.MessageID = strings.TrimSpace(order.MessageID)
	return order
}

func (order AvailabilitySelectionEventOrderV1) known() bool {
	order = normalizeAvailabilitySelectionEventOrderV1(order)
	return !order.ReceivedAt.IsZero() &&
		!order.CreatedAt.IsZero() &&
		order.MessageID != "" &&
		order.EventOrdinal >= 0
}

func compareAvailabilitySelectionEventOrderV1(
	left AvailabilitySelectionEventOrderV1,
	right AvailabilitySelectionEventOrderV1,
) int {
	left = normalizeAvailabilitySelectionEventOrderV1(left)
	right = normalizeAvailabilitySelectionEventOrderV1(right)
	if left.ReceivedAt.Before(right.ReceivedAt) {
		return -1
	}
	if left.ReceivedAt.After(right.ReceivedAt) {
		return 1
	}
	if left.CreatedAt.Before(right.CreatedAt) {
		return -1
	}
	if left.CreatedAt.After(right.CreatedAt) {
		return 1
	}
	if left.MessageID < right.MessageID {
		return -1
	}
	if left.MessageID > right.MessageID {
		return 1
	}
	if left.EventOrdinal < right.EventOrdinal {
		return -1
	}
	if left.EventOrdinal > right.EventOrdinal {
		return 1
	}
	return 0
}

func sortAvailabilitySelectionEventsV1(events []AvailabilitySelectionEventV1) []AvailabilitySelectionEventV1 {
	sorted := append([]AvailabilitySelectionEventV1(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return compareAvailabilitySelectionEventOrderV1(sorted[i].Order, sorted[j].Order) < 0
	})
	return sorted
}

func NormalizeAvailabilitySelectionEventsV1(
	state AvailabilitySelectionStateV1,
	events []AvailabilitySelectionEventV1,
) []AvailabilitySelectionEventV1 {
	state = normalizeAvailabilitySelectionStateV1(state)
	normalized := make([]AvailabilitySelectionEventV1, 0, len(events))
	for _, event := range sortAvailabilitySelectionEventsV1(events) {
		event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
		if event.Type == AvailabilitySelectionEventRejected &&
			event.AvailabilityPromptSourceMessageID == "" &&
			state.Status == AvailabilitySelectionStatusBookable {
			event.AvailabilityPromptSourceMessageID = state.AvailabilityPromptSourceMessageID
		}
		if event.Type == AvailabilitySelectionEventRejected &&
			event.AvailabilityPromptSourceMessageID == "" {
			event.Type = AvailabilitySelectionEventInvalidated
			event.RejectedWholeContext = true
			event.ReasonCode = firstNonEmpty(event.ReasonCode, "REJECTION_PROMPT_SOURCE_MISSING")
			event.EventID = ""
			event.EventID = availabilitySelectionEventIDV1(event)
		}
		normalized = append(normalized, event)
		if availabilitySelectionEventValidV1(event) {
			state = ReduceAvailabilitySelectionEventsV1(state, []AvailabilitySelectionEventV1{event})
		}
	}
	return normalized
}

// ReduceAvailabilitySelectionEventsV1 only consumes typed events. Message
// bodies and bounded transcript windows are intentionally outside the reducer.
func ReduceAvailabilitySelectionEventsV1(
	state AvailabilitySelectionStateV1,
	events []AvailabilitySelectionEventV1,
) AvailabilitySelectionStateV1 {
	state = normalizeAvailabilitySelectionStateV1(state)
	for _, event := range sortAvailabilitySelectionEventsV1(events) {
		event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
		if event.EventID == "" || availabilitySelectionStringSliceContains(state.AppliedEventIDs, event.EventID) ||
			!availabilitySelectionEventValidV1(event) {
			continue
		}
		switch event.Type {
		case AvailabilitySelectionEventMaterialized:
			if event.MaterializesAuthority {
				state.Status = AvailabilitySelectionStatusBookable
				state.SelectionEventMessageID = event.MessageID
				state.SelectionProjectionMessageID = event.ProjectionMessageID
				state.AvailabilityPromptSourceMessageID = event.AvailabilityPromptSourceMessageID
				state.SelectedOptionIndex = event.SelectedOptionIndex
				state.Snapshot = event.Snapshot
				state.Tombstone = nil
			}
		case AvailabilitySelectionEventRejected:
			tombstone := availabilitySelectionTombstoneFromEventV1(event)
			state.Rejections = appendAvailabilitySelectionTombstoneV1(state.Rejections, tombstone)
			if state.Status != AvailabilitySelectionStatusBookable ||
				availabilitySelectionRejectionTargetsStateV1(state, tombstone) {
				clearAvailabilitySelectionAuthorityV1(&state)
				state.Status = AvailabilitySelectionStatusRejected
				state.Tombstone = &tombstone
			}
		case AvailabilitySelectionEventInvalidated:
			tombstone := availabilitySelectionTombstoneFromEventV1(event)
			clearAvailabilitySelectionAuthorityV1(&state)
			state.Status = AvailabilitySelectionStatusInvalidated
			state.Tombstone = &tombstone
		}
		state.AppliedEventIDs = append(state.AppliedEventIDs, event.EventID)
		if !state.LastAppliedEventOrder.known() ||
			compareAvailabilitySelectionEventOrderV1(event.Order, state.LastAppliedEventOrder) > 0 {
			state.LastAppliedEventOrder = event.Order
		}
	}
	return normalizeAvailabilitySelectionStateV1(state)
}

func NormalizeAvailabilitySelectionEventsV1WithoutState(event AvailabilitySelectionEventV1) AvailabilitySelectionEventV1 {
	event.MessageID = strings.TrimSpace(event.MessageID)
	event.ProjectionMessageID = strings.TrimSpace(event.ProjectionMessageID)
	event.AvailabilityPromptSourceMessageID = strings.TrimSpace(event.AvailabilityPromptSourceMessageID)
	event.ReasonCode = strings.TrimSpace(event.ReasonCode)
	event.Snapshot = normalizeAvailabilitySelectionSnapshotV1(event.Snapshot)
	event.RejectedOptionIndexes = uniqueAvailabilitySelectionIndexesV1(event.RejectedOptionIndexes)
	sort.Ints(event.RejectedOptionIndexes)
	event.RejectedTripDates = uniqueAvailabilitySelectionTripDatesV1(event.RejectedTripDates)
	sort.Strings(event.RejectedTripDates)
	event.Order = normalizeAvailabilitySelectionEventOrderV1(event.Order)
	event.EventID = availabilitySelectionEventIDV1(event)
	return event
}

func availabilitySelectionEventIDV1(event AvailabilitySelectionEventV1) string {
	if eventID := strings.TrimSpace(event.EventID); eventID != "" {
		return eventID
	}
	messageID := strings.TrimSpace(event.MessageID)
	if messageID == "" || event.Type == "" {
		return ""
	}
	return messageID + ":" + string(event.Type) + ":" + strconv.Itoa(event.Order.EventOrdinal)
}

func availabilitySelectionEventValidV1(event AvailabilitySelectionEventV1) bool {
	event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
	if event.EventID == "" || event.MessageID == "" || !event.Order.known() {
		return false
	}
	switch event.Type {
	case AvailabilitySelectionEventMaterialized:
		if !event.MaterializesAuthority {
			return true
		}
		return event.ProjectionMessageID != "" &&
			event.AvailabilityPromptSourceMessageID != "" &&
			event.SelectedOptionIndex > 0 &&
			event.Snapshot.SelectedOptionIndex == event.SelectedOptionIndex &&
			availabilitySelectionSnapshotCompleteV1(event.Snapshot)
	case AvailabilitySelectionEventRejected:
		return event.AvailabilityPromptSourceMessageID != "" &&
			(event.RejectedWholeContext || len(event.RejectedOptionIndexes) > 0 || len(event.RejectedTripDates) > 0)
	case AvailabilitySelectionEventInvalidated:
		return true
	default:
		return false
	}
}

func validateAvailabilitySelectionStateV1(state AvailabilitySelectionStateV1) error {
	state = normalizeAvailabilitySelectionStateV1(state)
	switch state.Status {
	case AvailabilitySelectionStatusNone:
		if availabilitySelectionStateHasAggregateV1(state) || state.Tombstone != nil {
			return ErrAvailabilitySelectionStateInvalid
		}
	case AvailabilitySelectionStatusBookable:
		if state.SelectionEventMessageID == "" ||
			state.SelectionProjectionMessageID == "" ||
			state.AvailabilityPromptSourceMessageID == "" ||
			state.SelectedOptionIndex <= 0 ||
			state.Snapshot.SelectedOptionIndex != state.SelectedOptionIndex ||
			!availabilitySelectionSnapshotCompleteV1(state.Snapshot) ||
			state.Tombstone != nil {
			return ErrAvailabilitySelectionStateInvalid
		}
	case AvailabilitySelectionStatusRejected, AvailabilitySelectionStatusInvalidated:
		if availabilitySelectionStateHasAggregateV1(state) || state.Tombstone == nil ||
			strings.TrimSpace(state.Tombstone.EventID) == "" ||
			strings.TrimSpace(state.Tombstone.EventMessageID) == "" ||
			!state.Tombstone.EventOrder.known() {
			return ErrAvailabilitySelectionStateInvalid
		}
	default:
		return ErrAvailabilitySelectionStateInvalid
	}
	if len(state.AppliedEventIDs) != len(uniqueTrimmedStrings(state.AppliedEventIDs)) {
		return ErrAvailabilitySelectionStateInvalid
	}
	if len(state.AppliedEventIDs) > 0 && !state.LastAppliedEventOrder.known() {
		return ErrAvailabilitySelectionStateInvalid
	}
	if len(state.AppliedEventIDs) == 0 && state.LastAppliedEventOrder.known() {
		return ErrAvailabilitySelectionStateInvalid
	}
	return nil
}

func availabilitySelectionStateHasAggregateV1(state AvailabilitySelectionStateV1) bool {
	return state.SelectionEventMessageID != "" ||
		state.SelectionProjectionMessageID != "" ||
		state.AvailabilityPromptSourceMessageID != "" ||
		state.SelectedOptionIndex != 0 ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{})
}

func availabilitySelectionSnapshotCompleteV1(snapshot AvailabilitySelectionSnapshotV1) bool {
	return snapshot.SelectedOptionIndex > 0 &&
		hasCompleteSelectedTripFacts(snapshot.TripID, snapshot.BoardStopID, snapshot.AlightStopID)
}

func availabilitySelectionTombstoneFromEventV1(event AvailabilitySelectionEventV1) AvailabilitySelectionTombstoneV1 {
	return normalizeAvailabilitySelectionTombstoneV1(AvailabilitySelectionTombstoneV1{
		EventID:                           event.EventID,
		EventMessageID:                    event.MessageID,
		EventOrder:                        event.Order,
		AvailabilityPromptSourceMessageID: event.AvailabilityPromptSourceMessageID,
		RejectedOptionIndexes:             append([]int(nil), event.RejectedOptionIndexes...),
		RejectedTripDates:                 append([]string(nil), event.RejectedTripDates...),
		RejectedWholeContext:              event.RejectedWholeContext || event.Type == AvailabilitySelectionEventInvalidated,
		ReasonCode:                        event.ReasonCode,
	})
}

func appendAvailabilitySelectionTombstoneV1(
	items []AvailabilitySelectionTombstoneV1,
	tombstone AvailabilitySelectionTombstoneV1,
) []AvailabilitySelectionTombstoneV1 {
	for _, item := range items {
		if item.EventID == tombstone.EventID {
			return items
		}
	}
	return append(items, tombstone)
}

func availabilitySelectionRejectionTargetsStateV1(
	state AvailabilitySelectionStateV1,
	tombstone AvailabilitySelectionTombstoneV1,
) bool {
	if state.Status != AvailabilitySelectionStatusBookable ||
		strings.TrimSpace(state.AvailabilityPromptSourceMessageID) == "" ||
		state.AvailabilityPromptSourceMessageID != tombstone.AvailabilityPromptSourceMessageID {
		return false
	}
	return tombstone.RejectedWholeContext ||
		availabilityOptionIndexRejected(tombstone.RejectedOptionIndexes, state.SelectedOptionIndex) ||
		availabilityTripDateRejected(tombstone.RejectedTripDates, state.Snapshot.TripDate)
}

func (state AvailabilitySelectionStateV1) rejectsPromptOption(
	promptSourceMessageID string,
	index int,
	tripDate string,
) bool {
	promptSourceMessageID = strings.TrimSpace(promptSourceMessageID)
	if promptSourceMessageID == "" || index <= 0 {
		return false
	}
	for _, rejection := range state.Rejections {
		if rejection.AvailabilityPromptSourceMessageID != promptSourceMessageID {
			continue
		}
		if rejection.RejectedWholeContext ||
			availabilityOptionIndexRejected(rejection.RejectedOptionIndexes, index) ||
			availabilityTripDateRejected(rejection.RejectedTripDates, tripDate) {
			return true
		}
	}
	if state.Tombstone != nil &&
		state.Tombstone.AvailabilityPromptSourceMessageID == promptSourceMessageID {
		return state.Tombstone.RejectedWholeContext ||
			availabilityOptionIndexRejected(state.Tombstone.RejectedOptionIndexes, index) ||
			availabilityTripDateRejected(state.Tombstone.RejectedTripDates, tripDate)
	}
	return false
}

func clearAvailabilitySelectionAuthorityV1(state *AvailabilitySelectionStateV1) {
	if state == nil {
		return
	}
	state.SelectionEventMessageID = ""
	state.SelectionProjectionMessageID = ""
	state.AvailabilityPromptSourceMessageID = ""
	state.SelectedOptionIndex = 0
	state.Snapshot = AvailabilitySelectionSnapshotV1{}
}

func bootstrapAvailabilitySelectionStateV1(
	existing *AvailabilitySelectionStateV1,
	events []AvailabilitySelectionEventV1,
) AvailabilitySelectionStateV1 {
	if existing != nil {
		state := normalizeAvailabilitySelectionStateV1(*existing)
		state.BootstrapCompleted = true
		if validateAvailabilitySelectionStateV1(state) != nil {
			return invalidAvailabilitySelectionStateV1("INVALID_PERSISTED_STATE")
		}
		return state
	}
	state := newAvailabilitySelectionStateV1()
	events = NormalizeAvailabilitySelectionEventsV1(state, events)
	for _, event := range events {
		if !availabilitySelectionEventValidV1(event) {
			return invalidAvailabilitySelectionStateV1("INVALID_BOOTSTRAP_EVENT")
		}
	}
	state = ReduceAvailabilitySelectionEventsV1(state, events)
	state.BootstrapCompleted = true
	if validateAvailabilitySelectionStateV1(state) != nil {
		return invalidAvailabilitySelectionStateV1("INVALID_BOOTSTRAP_STATE")
	}
	return state
}

func invalidAvailabilitySelectionStateV1(reason string) AvailabilitySelectionStateV1 {
	recordedAt := time.Unix(0, 0).UTC()
	event := AvailabilitySelectionEventV1{
		Type:                 AvailabilitySelectionEventInvalidated,
		MessageID:            "availability-selection-bootstrap",
		ReasonCode:           strings.TrimSpace(reason),
		RejectedWholeContext: true,
		Order: AvailabilitySelectionEventOrderV1{
			ReceivedAt: recordedAt,
			CreatedAt:  recordedAt,
			MessageID:  "availability-selection-bootstrap",
		},
	}
	state := ReduceAvailabilitySelectionEventsV1(newAvailabilitySelectionStateV1(), []AvailabilitySelectionEventV1{event})
	state.BootstrapCompleted = true
	return state
}

func availabilitySelectionSnapshotV1FromPayload(payload map[string]interface{}) (AvailabilitySelectionSnapshotV1, bool) {
	snapshot, _, ok := availabilitySelectionSnapshotV1FromPayloadWithPresence(payload)
	return snapshot, ok
}

func availabilitySelectionSnapshotV1FromPayloadWithPresence(
	payload map[string]interface{},
) (AvailabilitySelectionSnapshotV1, availabilitySelectionSnapshotPresenceV1, bool) {
	if len(payload) == 0 {
		return AvailabilitySelectionSnapshotV1{}, 0, false
	}
	presence, validTypes := availabilitySelectionSnapshotPresenceFromPayloadV1(payload)
	if !validTypes {
		return AvailabilitySelectionSnapshotV1{}, 0, false
	}
	snapshot := normalizeAvailabilitySelectionSnapshotV1(AvailabilitySelectionSnapshotV1{
		SelectedOptionIndex:    asInt(payload["selected_option_index"]),
		SegmentID:              asString(payload["segment_id"]),
		TripID:                 asString(payload["trip_id"]),
		RouteID:                asString(payload["route_id"]),
		BoardStopID:            asString(payload["board_stop_id"]),
		AlightStopID:           asString(payload["alight_stop_id"]),
		OriginStopID:           asString(payload["origin_stop_id"]),
		DestinationStopID:      asString(payload["destination_stop_id"]),
		Origin:                 asString(payload["origin"]),
		Destination:            asString(payload["destination"]),
		PackageName:            asString(payload["package_name"]),
		OriginDisplayName:      asString(payload["origin_display_name"]),
		DestinationDisplayName: asString(payload["destination_display_name"]),
		OriginDepartTime:       asString(payload["origin_depart_time"]),
		TripDate:               asString(payload["trip_date"]),
		SeatsAvailable:         asInt(payload["seats_available"]),
		Price:                  asFloat64(payload["price"]),
		Currency:               asString(payload["currency"]),
		Status:                 asString(payload["status"]),
		TripStatus:             asString(payload["trip_status"]),
	})
	return snapshot,
		presence,
		availabilitySelectionSnapshotCompleteV1(snapshot)
}

func availabilitySelectionSnapshotPresenceFromPayloadV1(
	payload map[string]interface{},
) (availabilitySelectionSnapshotPresenceV1, bool) {
	var presence availabilitySelectionSnapshotPresenceV1
	for _, field := range availabilitySelectionSnapshotFieldsV1 {
		value, present := payload[field.key]
		if !present {
			continue
		}
		if !availabilitySelectionSnapshotFieldValueValidV1(value, field.kind) {
			return 0, false
		}
		presence |= field.presence
	}
	return presence, true
}

func availabilitySelectionSnapshotFieldValueValidV1(
	value interface{},
	kind availabilitySelectionSnapshotFieldKindV1,
) bool {
	switch kind {
	case availabilitySelectionSnapshotFieldStringV1:
		_, ok := value.(string)
		return ok
	case availabilitySelectionSnapshotFieldIntV1:
		_, ok := strictAvailabilitySelectionSnapshotIntV1(value)
		return ok
	case availabilitySelectionSnapshotFieldFloatV1:
		_, ok := strictAvailabilitySelectionSnapshotFloatV1(value)
		return ok
	default:
		return false
	}
}

func strictAvailabilitySelectionSnapshotIntV1(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		if strconv.IntSize == 32 && (typed < math.MinInt32 || typed > math.MaxInt32) {
			return 0, false
		}
		return int(typed), true
	case float32:
		return strictAvailabilitySelectionSnapshotIntV1(float64(typed))
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed {
			return 0, false
		}
		parsed, err := strconv.ParseInt(strconv.FormatFloat(typed, 'f', -1, 64), 10, strconv.IntSize)
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, strconv.IntSize)
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func strictAvailabilitySelectionSnapshotFloatV1(value interface{}) (float64, bool) {
	var result float64
	switch typed := value.(type) {
	case float64:
		result = typed
	case float32:
		result = float64(typed)
	case int:
		result = float64(typed)
	case int32:
		result = float64(typed)
	case int64:
		result = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		result = parsed
	default:
		return 0, false
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, false
	}
	return result, true
}

func availabilitySelectionSnapshotPresenceFromEventJSONV1(
	raw json.RawMessage,
) (availabilitySelectionSnapshotPresenceV1, bool) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(raw, &event); err != nil {
		return 0, false
	}
	snapshotRaw, present := event["snapshot"]
	if !present || string(snapshotRaw) == "null" {
		return 0, true
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(snapshotRaw, &snapshot); err != nil {
		return 0, false
	}
	return availabilitySelectionSnapshotPresenceFromPayloadV1(snapshot)
}

func inferredAvailabilitySelectionSnapshotPresenceV1(
	snapshot AvailabilitySelectionSnapshotV1,
) availabilitySelectionSnapshotPresenceV1 {
	var presence availabilitySelectionSnapshotPresenceV1
	if snapshot.SelectedOptionIndex != 0 {
		presence |= availabilitySelectionSnapshotSelectedOptionIndexPresentV1
	}
	if snapshot.SegmentID != "" {
		presence |= availabilitySelectionSnapshotSegmentIDPresentV1
	}
	if snapshot.TripID != "" {
		presence |= availabilitySelectionSnapshotTripIDPresentV1
	}
	if snapshot.RouteID != "" {
		presence |= availabilitySelectionSnapshotRouteIDPresentV1
	}
	if snapshot.BoardStopID != "" {
		presence |= availabilitySelectionSnapshotBoardStopIDPresentV1
	}
	if snapshot.AlightStopID != "" {
		presence |= availabilitySelectionSnapshotAlightStopIDPresentV1
	}
	if snapshot.OriginStopID != "" {
		presence |= availabilitySelectionSnapshotOriginStopIDPresentV1
	}
	if snapshot.DestinationStopID != "" {
		presence |= availabilitySelectionSnapshotDestinationStopIDPresentV1
	}
	if snapshot.Origin != "" {
		presence |= availabilitySelectionSnapshotOriginPresentV1
	}
	if snapshot.Destination != "" {
		presence |= availabilitySelectionSnapshotDestinationPresentV1
	}
	if snapshot.PackageName != "" {
		presence |= availabilitySelectionSnapshotPackageNamePresentV1
	}
	if snapshot.OriginDisplayName != "" {
		presence |= availabilitySelectionSnapshotOriginDisplayNamePresentV1
	}
	if snapshot.DestinationDisplayName != "" {
		presence |= availabilitySelectionSnapshotDestinationDisplayNamePresentV1
	}
	if snapshot.OriginDepartTime != "" {
		presence |= availabilitySelectionSnapshotOriginDepartTimePresentV1
	}
	if snapshot.TripDate != "" {
		presence |= availabilitySelectionSnapshotTripDatePresentV1
	}
	if snapshot.SeatsAvailable != 0 {
		presence |= availabilitySelectionSnapshotSeatsAvailablePresentV1
	}
	if snapshot.Price != 0 {
		presence |= availabilitySelectionSnapshotPricePresentV1
	}
	if snapshot.Currency != "" {
		presence |= availabilitySelectionSnapshotCurrencyPresentV1
	}
	if snapshot.Status != "" {
		presence |= availabilitySelectionSnapshotStatusPresentV1
	}
	if snapshot.TripStatus != "" {
		presence |= availabilitySelectionSnapshotTripStatusPresentV1
	}
	return presence
}

func availabilitySelectionEventSnapshotPresenceV1(
	event AvailabilitySelectionEventV1,
) availabilitySelectionSnapshotPresenceV1 {
	if event.snapshotPresenceKnown {
		return event.snapshotPresence
	}
	return inferredAvailabilitySelectionSnapshotPresenceV1(event.Snapshot)
}

func availabilitySelectionSnapshotV1FromAvailabilityWithPresence(
	availability *AvailabilitySearchResult,
	index int,
) (AvailabilitySelectionSnapshotV1, availabilitySelectionSnapshotPresenceV1, bool) {
	payload := selectedAvailabilityResultPayloadFromAvailability(availability, index)
	return availabilitySelectionSnapshotV1FromPayloadWithPresence(payload)
}

func (snapshot AvailabilitySelectionSnapshotV1) payload() map[string]interface{} {
	return map[string]interface{}{
		"selected_option_index":    snapshot.SelectedOptionIndex,
		"segment_id":               snapshot.SegmentID,
		"trip_id":                  snapshot.TripID,
		"route_id":                 snapshot.RouteID,
		"board_stop_id":            snapshot.BoardStopID,
		"alight_stop_id":           snapshot.AlightStopID,
		"origin_stop_id":           snapshot.OriginStopID,
		"destination_stop_id":      snapshot.DestinationStopID,
		"origin":                   snapshot.Origin,
		"destination":              snapshot.Destination,
		"package_name":             snapshot.PackageName,
		"origin_display_name":      snapshot.OriginDisplayName,
		"destination_display_name": snapshot.DestinationDisplayName,
		"origin_depart_time":       snapshot.OriginDepartTime,
		"trip_date":                snapshot.TripDate,
		"seats_available":          snapshot.SeatsAvailable,
		"price":                    snapshot.Price,
		"currency":                 snapshot.Currency,
		"status":                   snapshot.Status,
		"trip_status":              snapshot.TripStatus,
	}
}

func (snapshot AvailabilitySelectionSnapshotV1) availabilityItem() (AvailabilitySearchItem, bool) {
	item := AvailabilitySearchItem{
		SegmentID:              snapshot.SegmentID,
		TripID:                 snapshot.TripID,
		RouteID:                snapshot.RouteID,
		BoardStopID:            snapshot.BoardStopID,
		AlightStopID:           snapshot.AlightStopID,
		OriginStopID:           snapshot.OriginStopID,
		DestinationStopID:      snapshot.DestinationStopID,
		OriginDisplayName:      firstNonEmpty(snapshot.OriginDisplayName, snapshot.Origin),
		DestinationDisplayName: firstNonEmpty(snapshot.DestinationDisplayName, snapshot.Destination),
		PackageName:            snapshot.PackageName,
		OriginDepartTime:       snapshot.OriginDepartTime,
		TripDate:               snapshot.TripDate,
		SeatsAvailable:         snapshot.SeatsAvailable,
		Price:                  snapshot.Price,
		Currency:               snapshot.Currency,
		Status:                 snapshot.Status,
		TripStatus:             snapshot.TripStatus,
	}
	return item, availabilitySelectionSnapshotCompleteV1(snapshot)
}

func hydrateAvailabilitySelectionEventsV1(
	events []AvailabilitySelectionEventV1,
	messages []Message,
	requireInbound bool,
) ([]AvailabilitySelectionEventV1, error) {
	if len(events) == 0 {
		return nil, nil
	}
	messagesByID := make(map[string]Message, len(messages))
	for _, message := range messages {
		messageID := strings.TrimSpace(message.ID)
		if messageID != "" {
			messagesByID[messageID] = message
		}
	}
	eventsByMessageID := make(map[string][]AvailabilitySelectionEventV1)
	messageIDs := make([]string, 0)
	for _, event := range events {
		messageID := strings.TrimSpace(event.MessageID)
		message, ok := messagesByID[messageID]
		if !ok || (requireInbound &&
			!strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND")) {
			return nil, ErrAvailabilitySelectionStateInvalid
		}
		if _, exists := eventsByMessageID[messageID]; !exists {
			messageIDs = append(messageIDs, messageID)
		}
		event.MessageID = messageID
		event.EventID = ""
		event.Order = AvailabilitySelectionEventOrderV1{}
		eventsByMessageID[messageID] = append(eventsByMessageID[messageID], event)
	}
	sort.Strings(messageIDs)
	hydrated := make([]AvailabilitySelectionEventV1, 0, len(events))
	for _, messageID := range messageIDs {
		message := messagesByID[messageID]
		messageEvents := deduplicateAvailabilitySelectionEventsV1(eventsByMessageID[messageID])
		sort.SliceStable(messageEvents, func(i, j int) bool {
			leftRank := availabilitySelectionEventTypeRankV1(messageEvents[i].Type)
			rightRank := availabilitySelectionEventTypeRankV1(messageEvents[j].Type)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			return availabilitySelectionEventCanonicalKeyV1(messageEvents[i]) <
				availabilitySelectionEventCanonicalKeyV1(messageEvents[j])
		})
		for ordinal, event := range messageEvents {
			event.Order = availabilitySelectionEventOrderFromMessageV1(message, ordinal)
			event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
			hydrated = append(hydrated, event)
		}
	}
	return sortAvailabilitySelectionEventsV1(hydrated), nil
}

func deduplicateAvailabilitySelectionEventsV1(
	events []AvailabilitySelectionEventV1,
) []AvailabilitySelectionEventV1 {
	seen := make(map[string]struct{}, len(events))
	deduplicated := make([]AvailabilitySelectionEventV1, 0, len(events))
	for _, event := range events {
		event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
		event.EventID = ""
		event.Order = AvailabilitySelectionEventOrderV1{}
		key := availabilitySelectionEventCanonicalKeyV1(event)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deduplicated = append(deduplicated, event)
	}
	return deduplicated
}

func availabilitySelectionEventTypeRankV1(eventType AvailabilitySelectionEventTypeV1) int {
	switch eventType {
	case AvailabilitySelectionEventMaterialized:
		return 0
	case AvailabilitySelectionEventRejected:
		return 1
	case AvailabilitySelectionEventInvalidated:
		return 2
	default:
		return 3
	}
}

func availabilitySelectionEventCanonicalKeyV1(event AvailabilitySelectionEventV1) string {
	event.EventID = ""
	event.Order = AvailabilitySelectionEventOrderV1{}
	raw, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	return string(raw)
}

func availabilitySelectionEventOrderFromMessageV1(
	message Message,
	eventOrdinal int,
) AvailabilitySelectionEventOrderV1 {
	receivedAt := message.ReceivedAt
	createdAt := message.CreatedAt
	if receivedAt.IsZero() {
		receivedAt = createdAt
	}
	if createdAt.IsZero() {
		createdAt = receivedAt
	}
	return normalizeAvailabilitySelectionEventOrderV1(AvailabilitySelectionEventOrderV1{
		ReceivedAt:   receivedAt,
		CreatedAt:    createdAt,
		MessageID:    strings.TrimSpace(message.ID),
		EventOrdinal: eventOrdinal,
	})
}

func availabilitySelectionStructuredEventsV1(messages []Message) []AvailabilitySelectionEventV1 {
	events := make([]AvailabilitySelectionEventV1, 0, len(messages))
	startIndex := latestDeliveredInvalidAvailabilityPromptIndexV1(messages, len(messages)) + 1
	for index := startIndex; index < len(messages); index++ {
		message := messages[index]
		if strings.TrimSpace(message.ID) == "" {
			message.ID = availabilitySelectionLegacyMessageIDV1(message, index)
		}
		direct := availabilitySelectionEventsV1FromMessage(message)
		if len(direct) > 0 {
			events = append(events, direct...)
			continue
		}
		if legacy, ok := legacyAvailabilitySelectionEventV1FromMessage(message, index, messages); ok {
			if !legacy.Order.known() {
				legacy.Order = availabilitySelectionEventOrderFromMessageV1(message, 0)
			}
			legacy = NormalizeAvailabilitySelectionEventsV1WithoutState(legacy)
			legacy = enrichLegacyAvailabilitySelectionEventSnapshotV1(legacy, messages)
			events = append(events, legacy)
		}
	}
	return sortAvailabilitySelectionEventsV1(events)
}

func availabilitySelectionBootstrapEventsWithInvalidationV1(
	session Session,
	events []AvailabilitySelectionEventV1,
	messages []Message,
) []AvailabilitySelectionEventV1 {
	if !canonicalAvailabilityFactsInvalidatedInMetadata(session.Metadata) {
		return events
	}
	boundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(session.Metadata)
	order, messageID := availabilitySelectionInvalidationOrderV1(boundary, events, messages)
	invalidation := NormalizeAvailabilitySelectionEventsV1WithoutState(AvailabilitySelectionEventV1{
		Type:                 AvailabilitySelectionEventInvalidated,
		MessageID:            messageID,
		ReasonCode:           "LEGACY_CANONICAL_AVAILABILITY_INVALIDATED",
		RejectedWholeContext: true,
		Order:                order,
	})
	return sortAvailabilitySelectionEventsV1(append(events, invalidation))
}

func availabilitySelectionInvalidationOrderV1(
	boundary canonicalAvailabilityFactsInvalidationBoundary,
	events []AvailabilitySelectionEventV1,
	messages []Message,
) (AvailabilitySelectionEventOrderV1, string) {
	const boundaryOrdinal = 1 << 30
	if boundaryMessageID := strings.TrimSpace(boundary.AfterMessageID); boundaryMessageID != "" {
		for _, message := range messages {
			if strings.TrimSpace(message.ID) != boundaryMessageID {
				continue
			}
			order := availabilitySelectionEventOrderFromMessageV1(message, boundaryOrdinal)
			return order, boundaryMessageID
		}
	}
	if !boundary.AfterCreatedAt.IsZero() {
		var latest AvailabilitySelectionEventOrderV1
		for index, message := range messages {
			if !canonicalAvailabilityHistoryMessageAtOrBeforeBoundary(
				message,
				index,
				-1,
				boundary,
			) {
				continue
			}
			order := availabilitySelectionEventOrderFromMessageV1(message, boundaryOrdinal)
			if !latest.known() || compareAvailabilitySelectionEventOrderV1(order, latest) > 0 {
				latest = order
			}
		}
		if latest.known() {
			return latest, latest.MessageID
		}
		recordedAt := boundary.AfterCreatedAt.UTC()
		messageID := "availability-selection-invalidation-boundary"
		return AvailabilitySelectionEventOrderV1{
			ReceivedAt: recordedAt,
			CreatedAt:  recordedAt,
			MessageID:  messageID,
		}, messageID
	}

	var latest AvailabilitySelectionEventOrderV1
	for _, event := range events {
		if !event.Order.known() {
			continue
		}
		if !latest.known() || compareAvailabilitySelectionEventOrderV1(event.Order, latest) > 0 {
			latest = event.Order
		}
	}
	if latest.known() {
		latest.EventOrdinal++
		return latest, latest.MessageID
	}
	recordedAt := time.Unix(0, 0).UTC()
	messageID := "availability-selection-legacy-marker"
	return AvailabilitySelectionEventOrderV1{
		ReceivedAt: recordedAt,
		CreatedAt:  recordedAt,
		MessageID:  messageID,
	}, messageID
}

func enrichLegacyAvailabilitySelectionEventSnapshotV1(
	event AvailabilitySelectionEventV1,
	messages []Message,
) AvailabilitySelectionEventV1 {
	if event.Type != AvailabilitySelectionEventMaterialized || !event.MaterializesAuthority {
		return event
	}
	projection, projectionIndex, projectionOK := exactAvailabilitySelectionEnrichmentMessageV1(
		messages,
		event.ProjectionMessageID,
	)
	if !projectionOK {
		event.MaterializesAuthority = false
		return event
	}
	projectionAuthority := classifyAvailabilityPromptCandidateV1(projection)
	if projectionAuthority.Class == availabilityPromptAuthorityAbsentLegacyV1 {
		if !legacyAvailabilitySelectionProjectionFactsV1(projection) ||
			projectionAuthority.Selection == nil ||
			!projectionAuthority.Selection.SelectedResultPresent ||
			!legacyAvailabilitySelectionEnrichmentIdentityMatchesV1(projection, event) {
			event.MaterializesAuthority = false
			return event
		}
		candidate, candidatePresence, candidateOK := availabilitySelectionSnapshotV1FromPayloadWithPresence(
			projectionAuthority.Selection.SelectedResult,
		)
		if !candidateOK {
			event.MaterializesAuthority = false
			return event
		}
		if _, sourceOK := exactLegacyAvailabilitySelectionPromptSourceV1(
			messages,
			projectionIndex,
			projectionIndex,
			event.AvailabilityPromptSourceMessageID,
			event.SelectedOptionIndex,
			candidate,
		); !sourceOK || !availabilitySelectionSnapshotCompatibleForEnrichmentV1(
			event.Snapshot,
			availabilitySelectionEventSnapshotPresenceV1(event),
			candidate,
			candidatePresence,
		) {
			event.MaterializesAuthority = false
			return event
		}
		event.Snapshot, event.snapshotPresence = mergeMissingAvailabilitySelectionSnapshotV1(
			event.Snapshot,
			availabilitySelectionEventSnapshotPresenceV1(event),
			candidate,
			candidatePresence,
		)
		event.snapshotPresenceKnown = true
		return event
	}
	if projectionAuthority.Class != availabilityPromptAuthorityValidStructuralV1 {
		event.MaterializesAuthority = false
		return event
	}
	eventSnapshotPresence := availabilitySelectionEventSnapshotPresenceV1(event)
	for _, sourceMessageID := range []string{
		event.ProjectionMessageID,
		event.AvailabilityPromptSourceMessageID,
	} {
		sourceMessageID = strings.TrimSpace(sourceMessageID)
		if sourceMessageID == "" {
			continue
		}
		for _, message := range messages {
			if strings.TrimSpace(message.ID) != sourceMessageID {
				continue
			}
			if !legacyAvailabilitySelectionEnrichmentIdentityMatchesV1(message, event) {
				continue
			}
			for _, candidate := range availabilitySelectionSnapshotCandidatesWithPresenceFromMessageV1(
				message,
				event.SelectedOptionIndex,
			) {
				if candidate.Snapshot.TripID != event.Snapshot.TripID ||
					candidate.Snapshot.BoardStopID != event.Snapshot.BoardStopID ||
					candidate.Snapshot.AlightStopID != event.Snapshot.AlightStopID {
					continue
				}
				event.Snapshot, eventSnapshotPresence = mergeMissingAvailabilitySelectionSnapshotV1(
					event.Snapshot,
					eventSnapshotPresence,
					candidate.Snapshot,
					candidate.Presence,
				)
				event.snapshotPresence = eventSnapshotPresence
				event.snapshotPresenceKnown = true
			}
		}
	}
	return event
}

func exactAvailabilitySelectionEnrichmentMessageV1(
	messages []Message,
	messageID string,
) (Message, int, bool) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return Message{}, -1, false
	}
	foundIndex := -1
	for index, message := range messages {
		if strings.TrimSpace(message.ID) != messageID {
			continue
		}
		if foundIndex >= 0 {
			return Message{}, -1, false
		}
		foundIndex = index
	}
	if foundIndex < 0 {
		return Message{}, -1, false
	}
	return messages[foundIndex], foundIndex, true
}

func legacyAvailabilitySelectionEnrichmentIdentityMatchesV1(
	message Message,
	event AvailabilitySelectionEventV1,
) bool {
	authority := classifyAvailabilityPromptCandidateV1(message)
	if authority.Class != availabilityPromptAuthorityAbsentLegacyV1 ||
		authority.Selection == nil || !authority.Selection.SelectedResultPresent {
		return true
	}
	selected := authority.Selection.SelectedResult
	selectionMessageID := strings.TrimSpace(asString(
		selected[selectedAvailabilitySelectionMessageIDPayloadKey],
	))
	promptSourceMessageID := strings.TrimSpace(asString(
		selected[availabilityPromptSourceMessageIDPayloadKey],
	))
	expectedPromptSourceMessageID := strings.TrimSpace(event.AvailabilityPromptSourceMessageID)
	return strings.TrimSpace(message.ID) == strings.TrimSpace(event.ProjectionMessageID) &&
		asInt(selected["selected_option_index"]) == event.SelectedOptionIndex &&
		selectionMessageID != "" && selectionMessageID == strings.TrimSpace(event.MessageID) &&
		promptSourceMessageID != "" && expectedPromptSourceMessageID != "" &&
		promptSourceMessageID == expectedPromptSourceMessageID
}

type availabilitySelectionSnapshotCandidateV1 struct {
	Snapshot AvailabilitySelectionSnapshotV1
	Presence availabilitySelectionSnapshotPresenceV1
}

func availabilitySelectionSnapshotCandidatesFromMessageV1(
	message Message,
	index int,
) []AvailabilitySelectionSnapshotV1 {
	detailed := availabilitySelectionSnapshotCandidatesWithPresenceFromMessageV1(message, index)
	candidates := make([]AvailabilitySelectionSnapshotV1, 0, len(detailed))
	for _, candidate := range detailed {
		candidates = append(candidates, candidate.Snapshot)
	}
	return candidates
}

func availabilitySelectionSnapshotCandidatesWithPresenceFromMessageV1(
	message Message,
	index int,
) []availabilitySelectionSnapshotCandidateV1 {
	authority := classifyAvailabilityPromptCandidateV1(message)
	var facts *AvailabilitySearchResult
	var selected map[string]interface{}
	switch authority.Class {
	case availabilityPromptAuthorityValidStructuralV1:
		if authority.Presented == nil {
			return nil
		}
		snapshot, presence, ok := availabilitySelectionSnapshotV1FromAvailabilityWithPresence(authority.Presented, index)
		if !ok {
			return nil
		}
		return []availabilitySelectionSnapshotCandidateV1{{Snapshot: snapshot, Presence: presence}}
	case availabilityPromptAuthorityAbsentLegacyV1:
		if legacyAvailabilityPromptFactsV1(message) {
			facts = authority.Facts
		} else if legacyAvailabilitySelectionProjectionFactsV1(message) &&
			authority.Selection != nil && authority.Selection.SelectedResultPresent {
			selected = authority.Selection.SelectedResult
		} else {
			return nil
		}
	default:
		return nil
	}

	candidates := make([]availabilitySelectionSnapshotCandidateV1, 0, 3)
	if len(selected) > 0 {
		if snapshot, presence, ok := availabilitySelectionSnapshotV1FromPayloadWithPresence(selected); ok {
			candidates = append(candidates, availabilitySelectionSnapshotCandidateV1{
				Snapshot: snapshot,
				Presence: presence,
			})
		}
	}
	if facts != nil {
		if snapshot, presence, ok := availabilitySelectionSnapshotV1FromAvailabilityWithPresence(facts, index); ok {
			if index > 0 && index <= len(facts.Results) {
				snapshot.PackageName = strings.TrimSpace(facts.Results[index-1].PackageName)
			}
			candidates = append(candidates, availabilitySelectionSnapshotCandidateV1{
				Snapshot: snapshot,
				Presence: presence,
			})
		}
	}
	return candidates
}

func mergeMissingAvailabilitySelectionSnapshotV1(
	current AvailabilitySelectionSnapshotV1,
	currentPresence availabilitySelectionSnapshotPresenceV1,
	candidate AvailabilitySelectionSnapshotV1,
	candidatePresence availabilitySelectionSnapshotPresenceV1,
) (AvailabilitySelectionSnapshotV1, availabilitySelectionSnapshotPresenceV1) {
	merge := func(field availabilitySelectionSnapshotPresenceV1, assign func()) {
		if currentPresence&field == 0 && candidatePresence&field != 0 {
			assign()
			currentPresence |= field
		}
	}
	merge(availabilitySelectionSnapshotSelectedOptionIndexPresentV1, func() { current.SelectedOptionIndex = candidate.SelectedOptionIndex })
	merge(availabilitySelectionSnapshotSegmentIDPresentV1, func() { current.SegmentID = candidate.SegmentID })
	merge(availabilitySelectionSnapshotTripIDPresentV1, func() { current.TripID = candidate.TripID })
	merge(availabilitySelectionSnapshotRouteIDPresentV1, func() { current.RouteID = candidate.RouteID })
	merge(availabilitySelectionSnapshotBoardStopIDPresentV1, func() { current.BoardStopID = candidate.BoardStopID })
	merge(availabilitySelectionSnapshotAlightStopIDPresentV1, func() { current.AlightStopID = candidate.AlightStopID })
	merge(availabilitySelectionSnapshotOriginStopIDPresentV1, func() { current.OriginStopID = candidate.OriginStopID })
	merge(availabilitySelectionSnapshotDestinationStopIDPresentV1, func() { current.DestinationStopID = candidate.DestinationStopID })
	merge(availabilitySelectionSnapshotOriginPresentV1, func() { current.Origin = candidate.Origin })
	merge(availabilitySelectionSnapshotDestinationPresentV1, func() { current.Destination = candidate.Destination })
	merge(availabilitySelectionSnapshotPackageNamePresentV1, func() { current.PackageName = candidate.PackageName })
	merge(availabilitySelectionSnapshotOriginDisplayNamePresentV1, func() { current.OriginDisplayName = candidate.OriginDisplayName })
	merge(availabilitySelectionSnapshotDestinationDisplayNamePresentV1, func() { current.DestinationDisplayName = candidate.DestinationDisplayName })
	merge(availabilitySelectionSnapshotOriginDepartTimePresentV1, func() { current.OriginDepartTime = candidate.OriginDepartTime })
	merge(availabilitySelectionSnapshotTripDatePresentV1, func() { current.TripDate = candidate.TripDate })
	merge(availabilitySelectionSnapshotSeatsAvailablePresentV1, func() { current.SeatsAvailable = candidate.SeatsAvailable })
	merge(availabilitySelectionSnapshotPricePresentV1, func() { current.Price = candidate.Price })
	merge(availabilitySelectionSnapshotCurrencyPresentV1, func() { current.Currency = candidate.Currency })
	merge(availabilitySelectionSnapshotStatusPresentV1, func() { current.Status = candidate.Status })
	merge(availabilitySelectionSnapshotTripStatusPresentV1, func() { current.TripStatus = candidate.TripStatus })
	return normalizeAvailabilitySelectionSnapshotV1(current), currentPresence
}

func availabilitySelectionSnapshotCompatibleForEnrichmentV1(
	current AvailabilitySelectionSnapshotV1,
	currentPresence availabilitySelectionSnapshotPresenceV1,
	candidate AvailabilitySelectionSnapshotV1,
	candidatePresence availabilitySelectionSnapshotPresenceV1,
) bool {
	current = normalizeAvailabilitySelectionSnapshotV1(current)
	candidate = normalizeAvailabilitySelectionSnapshotV1(candidate)
	compatible := func(field availabilitySelectionSnapshotPresenceV1, equal bool) bool {
		return currentPresence&field == 0 || candidatePresence&field != 0 && equal
	}
	return compatible(availabilitySelectionSnapshotSelectedOptionIndexPresentV1, current.SelectedOptionIndex == candidate.SelectedOptionIndex) &&
		compatible(availabilitySelectionSnapshotSegmentIDPresentV1, current.SegmentID == candidate.SegmentID) &&
		compatible(availabilitySelectionSnapshotTripIDPresentV1, current.TripID == candidate.TripID) &&
		compatible(availabilitySelectionSnapshotRouteIDPresentV1, current.RouteID == candidate.RouteID) &&
		compatible(availabilitySelectionSnapshotBoardStopIDPresentV1, current.BoardStopID == candidate.BoardStopID) &&
		compatible(availabilitySelectionSnapshotAlightStopIDPresentV1, current.AlightStopID == candidate.AlightStopID) &&
		compatible(availabilitySelectionSnapshotOriginStopIDPresentV1, current.OriginStopID == candidate.OriginStopID) &&
		compatible(availabilitySelectionSnapshotDestinationStopIDPresentV1, current.DestinationStopID == candidate.DestinationStopID) &&
		compatible(availabilitySelectionSnapshotOriginPresentV1, current.Origin == candidate.Origin) &&
		compatible(availabilitySelectionSnapshotDestinationPresentV1, current.Destination == candidate.Destination) &&
		compatible(availabilitySelectionSnapshotPackageNamePresentV1, current.PackageName == candidate.PackageName) &&
		compatible(availabilitySelectionSnapshotOriginDisplayNamePresentV1, current.OriginDisplayName == candidate.OriginDisplayName) &&
		compatible(availabilitySelectionSnapshotDestinationDisplayNamePresentV1, current.DestinationDisplayName == candidate.DestinationDisplayName) &&
		compatible(availabilitySelectionSnapshotOriginDepartTimePresentV1, current.OriginDepartTime == candidate.OriginDepartTime) &&
		compatible(availabilitySelectionSnapshotTripDatePresentV1, current.TripDate == candidate.TripDate) &&
		compatible(availabilitySelectionSnapshotSeatsAvailablePresentV1, current.SeatsAvailable == candidate.SeatsAvailable) &&
		compatible(availabilitySelectionSnapshotPricePresentV1, current.Price == candidate.Price) &&
		compatible(availabilitySelectionSnapshotCurrencyPresentV1, current.Currency == candidate.Currency) &&
		compatible(availabilitySelectionSnapshotStatusPresentV1, current.Status == candidate.Status) &&
		compatible(availabilitySelectionSnapshotTripStatusPresentV1, current.TripStatus == candidate.TripStatus)
}

func availabilitySelectionEventsV1FromMessage(message Message) []AvailabilitySelectionEventV1 {
	rawEvents := message.NormalizedPayload[availabilitySelectionEventsV1MessageKey]
	if rawEvents == nil {
		return nil
	}
	raw, err := json.Marshal(rawEvents)
	if err != nil {
		return nil
	}
	var decoded []AvailabilitySelectionEventV1
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	var rawDecoded []json.RawMessage
	if err := json.Unmarshal(raw, &rawDecoded); err != nil || len(rawDecoded) != len(decoded) {
		return nil
	}
	for index := range decoded {
		presence, ok := availabilitySelectionSnapshotPresenceFromEventJSONV1(rawDecoded[index])
		if !ok {
			return nil
		}
		decoded[index].snapshotPresence = presence
		decoded[index].snapshotPresenceKnown = true
		decoded[index].MessageID = strings.TrimSpace(message.ID)
		decoded[index].EventID = ""
		decoded[index].Order = AvailabilitySelectionEventOrderV1{}
	}
	hydrated, err := hydrateAvailabilitySelectionEventsV1(decoded, []Message{message}, true)
	if err != nil {
		return nil
	}
	events := make([]AvailabilitySelectionEventV1, 0, len(hydrated))
	for _, event := range hydrated {
		events = append(events, event)
	}
	return events
}

func legacyAvailabilitySelectionEventV1FromMessage(
	message Message,
	historyIndex int,
	messages []Message,
) (AvailabilitySelectionEventV1, bool) {
	if rejection := availabilityRejectionEvidenceFromMessageMetadata(message); rejection.Found {
		event := AvailabilitySelectionEventV1{
			Type:                              AvailabilitySelectionEventRejected,
			MessageID:                         availabilitySelectionLegacyMessageIDV1(message, historyIndex),
			AvailabilityPromptSourceMessageID: rejection.AvailabilityPromptSourceMessageID,
			RejectedOptionIndexes:             append([]int(nil), rejection.OptionIndexes...),
			RejectedTripDates:                 append([]string(nil), rejection.TripDates...),
			RejectedWholeContext:              rejection.WholeContext,
			ReasonCode:                        "LEGACY_STRUCTURED_REJECTION",
		}
		event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
		return event, true
	}
	evidence := messageAvailabilitySelectionEvidence(message)
	if !evidence.found() || evidence.Status == availabilitySelectionRejected {
		return AvailabilitySelectionEventV1{}, false
	}
	messageID := availabilitySelectionLegacyMessageIDV1(message, historyIndex)
	markerValue, markerPresent := availabilitySelectionMessageAuthorityMarker(message)
	structuralSelection := availabilitySelectionMessageHasStructuralSelectIntentV1(message)
	if markerPresent && !markerValue {
		return AvailabilitySelectionEventV1{}, false
	}
	if evidence.Status != availabilitySelectionBookable {
		if structuralSelection {
			event := AvailabilitySelectionEventV1{
				Type:                 AvailabilitySelectionEventInvalidated,
				MessageID:            messageID,
				ReasonCode:           "LEGACY_INCOMPLETE_SELECTION",
				RejectedWholeContext: true,
			}
			event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
			return event, true
		}
		return AvailabilitySelectionEventV1{}, false
	}
	selected := selectedAvailabilityResultFromMessage(message)
	snapshot, snapshotPresence, ok := availabilitySelectionSnapshotV1FromPayloadWithPresence(selected)
	if !ok {
		for _, candidate := range availabilitySelectionSnapshotCandidatesWithPresenceFromMessageV1(
			message,
			evidence.SelectedOptionIndex,
		) {
			if candidate.Snapshot.SelectedOptionIndex == evidence.SelectedOptionIndex {
				snapshot = candidate.Snapshot
				snapshotPresence = candidate.Presence
				ok = true
				break
			}
		}
		if !ok {
			return AvailabilitySelectionEventV1{}, false
		}
	}
	selectionEventMessageID := strings.TrimSpace(evidence.SelectionMessageID)
	selectionMessage, selectionHistoryIndex, selectionMessageOK := exactLegacyAvailabilitySelectionMessageV1(
		messages,
		historyIndex,
		selectionEventMessageID,
	)
	if !selectionMessageOK {
		return AvailabilitySelectionEventV1{}, false
	}
	promptSource, sourceOK := exactLegacyAvailabilitySelectionPromptSourceV1(
		messages,
		selectionHistoryIndex,
		historyIndex,
		evidence.AvailabilityPromptSourceMessageID,
		evidence.SelectedOptionIndex,
		snapshot,
	)
	if !structuralSelection || !sourceOK {
		return AvailabilitySelectionEventV1{}, false
	}
	event := AvailabilitySelectionEventV1{
		Type:                              AvailabilitySelectionEventMaterialized,
		MessageID:                         selectionEventMessageID,
		ProjectionMessageID:               messageID,
		AvailabilityPromptSourceMessageID: promptSource,
		SelectedOptionIndex:               evidence.SelectedOptionIndex,
		Snapshot:                          snapshot,
		MaterializesAuthority:             true,
		ReasonCode:                        "LEGACY_STRUCTURED_SELECTION",
		Order:                             availabilitySelectionEventOrderFromMessageV1(selectionMessage, 0),
		snapshotPresence:                  snapshotPresence,
		snapshotPresenceKnown:             true,
	}
	event = NormalizeAvailabilitySelectionEventsV1WithoutState(event)
	return event, true
}

func exactLegacyAvailabilitySelectionMessageV1(
	messages []Message,
	beforeIndex int,
	selectionMessageID string,
) (Message, int, bool) {
	selectionMessageID = strings.TrimSpace(selectionMessageID)
	if selectionMessageID == "" {
		return Message{}, -1, false
	}
	if beforeIndex > len(messages) {
		beforeIndex = len(messages)
	}
	var selected Message
	selectedIndex := -1
	for index := 0; index < beforeIndex; index++ {
		message := messages[index]
		if strings.TrimSpace(message.ID) != selectionMessageID {
			continue
		}
		if selectedIndex >= 0 || !strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			return Message{}, -1, false
		}
		selected = message
		selectedIndex = index
	}
	return selected, selectedIndex, selectedIndex >= 0
}

func availabilitySelectionMessageHasStructuralSelectIntentV1(message Message) bool {
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if strings.EqualFold(
			payloadMetadataString(payload, "intent"),
			string(IntentSelectAvailabilityOption),
		) {
			return true
		}
	}
	return false
}

func exactLegacyAvailabilitySelectionPromptSourceV1(
	messages []Message,
	sourceBeforeIndex int,
	materializationIndex int,
	expectedSourceMessageID string,
	selectedOptionIndex int,
	snapshot AvailabilitySelectionSnapshotV1,
) (string, bool) {
	if sourceBeforeIndex > len(messages) {
		sourceBeforeIndex = len(messages)
	}
	if materializationIndex > len(messages) {
		materializationIndex = len(messages)
	}
	if sourceBeforeIndex < 0 || materializationIndex < 0 {
		return "", false
	}
	expectedSourceMessageID = strings.TrimSpace(expectedSourceMessageID)
	if expectedSourceMessageID == "" {
		return "", false
	}
	source, sourceIndex, ok := resolveAvailabilityPromptEffectiveSourceByIDV1(
		messages,
		expectedSourceMessageID,
		sourceBeforeIndex,
	)
	if !ok || availabilityPromptSourceMessageIDFromMessage(source) != expectedSourceMessageID {
		return "", false
	}
	if barrierIndex := latestDeliveredInvalidAvailabilityPromptIndexV1(messages, materializationIndex); barrierIndex > sourceIndex {
		return "", false
	}
	availability := visibleAvailabilitySelectionContextFromHistoryMessage(source)
	selected, selectedOK := selectedAvailabilityItemForMaterialization(
		availability,
		selectedOptionIndex,
	)
	if !selectedOK ||
		strings.TrimSpace(selected.TripID) != snapshot.TripID ||
		strings.TrimSpace(selected.BoardStopID) != snapshot.BoardStopID ||
		strings.TrimSpace(selected.AlightStopID) != snapshot.AlightStopID {
		return "", false
	}
	return expectedSourceMessageID, true
}

func availabilitySelectionLegacyMessageIDV1(message Message, historyIndex int) string {
	if messageID := strings.TrimSpace(message.ID); messageID != "" {
		return messageID
	}
	return "legacy-history:" + strconv.Itoa(historyIndex+1)
}

func uniqueAvailabilitySelectionIndexesV1(values []int) []int {
	unique := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || availabilityOptionIndexRejected(unique, value) {
			continue
		}
		unique = append(unique, value)
	}
	return unique
}

func uniqueAvailabilitySelectionTripDatesV1(values []string) []string {
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = canonicalRejectedTripDate(value)
		if value == "" || availabilityTripDateRejected(unique, value) {
			continue
		}
		unique = append(unique, value)
	}
	return unique
}

func uniqueTrimmedStrings(values []string) []string {
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || availabilitySelectionStringSliceContains(unique, value) {
			continue
		}
		unique = append(unique, value)
	}
	return unique
}

func availabilitySelectionStringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
