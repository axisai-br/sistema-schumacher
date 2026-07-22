package chat

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	passengerClarificationStateV1Version   = 1
	passengerClarificationStateV1MemoryKey = "passenger_clarification_state_v1"
)

var ErrPassengerClarificationStateInvalid = errors.New("passenger clarification state invalid")

type PassengerCountProvenance string

const (
	PassengerCountProvenanceUnknown                    PassengerCountProvenance = "UNKNOWN"
	PassengerCountProvenanceSoloSpeaker                PassengerCountProvenance = "SOLO_SPEAKER"
	PassengerCountProvenanceAbsoluteTotal              PassengerCountProvenance = "ABSOLUTE_TOTAL"
	PassengerCountProvenanceIncludesSpeakerComposition PassengerCountProvenance = "INCLUDES_SPEAKER_COMPOSITION"
	PassengerCountProvenanceSubgroupOnly               PassengerCountProvenance = "SUBGROUP_ONLY"
)

type PassengerClarificationAuthorityV1 string

const (
	PassengerClarificationAuthorityPreBooking  PassengerClarificationAuthorityV1 = "PRE_BOOKING_STATE"
	PassengerClarificationAuthorityPostBooking PassengerClarificationAuthorityV1 = "POST_BOOKING_PERSISTED"
)

type PassengerClarificationSlotStatusV1 string

const (
	PassengerClarificationSlotOpen        PassengerClarificationSlotStatusV1 = "OPEN"
	PassengerClarificationSlotPending     PassengerClarificationSlotStatusV1 = "PENDING"
	PassengerClarificationSlotAnswered    PassengerClarificationSlotStatusV1 = "ANSWERED"
	PassengerClarificationSlotConflicting PassengerClarificationSlotStatusV1 = "CONFLICTING"
)

type PassengerClarificationEventTypeV1 string

const (
	PassengerClarificationEventPassengerPromptOpened PassengerClarificationEventTypeV1 = "PASSENGER_PROMPT_OPENED"
	PassengerClarificationEventChildPromptOpened     PassengerClarificationEventTypeV1 = "CHILD_PROMPT_OPENED"
	PassengerClarificationEventPassengerCountSet     PassengerClarificationEventTypeV1 = "PASSENGER_COUNT_SET"
	PassengerClarificationEventChildCountSet         PassengerClarificationEventTypeV1 = "CHILD_COUNT_SET"
	PassengerClarificationEventSlotCorrected         PassengerClarificationEventTypeV1 = "SLOT_CORRECTED"
	PassengerClarificationEventSlotInvalidated       PassengerClarificationEventTypeV1 = "SLOT_INVALIDATED"
)

type PassengerClarificationSlotV1 string

const (
	PassengerClarificationSlotPassenger PassengerClarificationSlotV1 = "PASSENGER"
	PassengerClarificationSlotChild     PassengerClarificationSlotV1 = "CHILD"
)

const (
	passengerClarificationReasonPassengerPromptOpened        = "PASSENGER_PROMPT_OPENED"
	passengerClarificationReasonChildPromptOpened            = "CHILD_PROMPT_OPENED"
	passengerClarificationReasonPassengerCountAccepted       = "PASSENGER_COUNT_ACCEPTED"
	passengerClarificationReasonChildCountAccepted           = "CHILD_COUNT_ACCEPTED"
	passengerClarificationReasonSlotCorrected                = "SLOT_CORRECTED"
	passengerClarificationReasonUnsupportedFamilyIdentity    = "UNSUPPORTED_FAMILY_IDENTITY"
	passengerClarificationReasonAmbiguousPassengerMeaning    = "AMBIGUOUS_PASSENGER_MEANING"
	passengerClarificationReasonPassengerTotalsConflict      = "PASSENGER_TOTALS_CONFLICT"
	passengerClarificationReasonChildrenExceedPassengerTotal = "CHILDREN_EXCEED_PASSENGER_TOTAL"
	passengerClarificationReasonAbsoluteTotalChildConflict   = "ABSOLUTE_TOTAL_CHILD_CONFLICT"
	passengerClarificationReasonInvalidEvent                 = "INVALID_EVENT"
	passengerClarificationReasonInvalidState                 = "INVALID_STATE"
	passengerClarificationReasonSoloChildAddsTraveler        = "SOLO_CHILD_PROMPT_ADDS_TRAVELER"
)

// PassengerClarificationChildReferenceV1 is intentionally opaque. H-B does not
// derive these identifiers from words or family labels; a future structured
// meaning contract may provide stable reference IDs.
type PassengerClarificationChildReferenceV1 struct {
	ID       string `json:"id"`
	Under5   bool   `json:"under_5"`
	AgeKnown bool   `json:"age_known"`
}

type PassengerClarificationAddsTravelerOriginV1 struct {
	SourceMessageID          string `json:"source_message_id"`
	SourcePromptMessageID    string `json:"source_prompt_message_id"`
	PassengerSourceMessageID string `json:"passenger_source_message_id"`
	ReasonCode               string `json:"reason_code"`
}

// PassengerClarificationStateV1 is the durable authority after its first
// persistence. Text and bounded history are not inputs to its reducer.
type PassengerClarificationStateV1 struct {
	Version   int                               `json:"version"`
	Authority PassengerClarificationAuthorityV1 `json:"authority"`
	BookingID string                            `json:"booking_id,omitempty"`

	PassengerCount           int                                `json:"passenger_count"`
	PassengerCountKnown      bool                               `json:"passenger_count_known"`
	PassengerCountProvenance PassengerCountProvenance           `json:"passenger_count_provenance"`
	PassengerSlotStatus      PassengerClarificationSlotStatusV1 `json:"passenger_slot_status"`
	PassengerPromptMessageID string                             `json:"passenger_prompt_message_id,omitempty"`
	PassengerLastMessageID   string                             `json:"passenger_last_message_id,omitempty"`
	PassengerReasonCodes     []string                           `json:"passenger_reason_codes,omitempty"`

	ChildUnder5Count              int                                        `json:"child_under_5_count"`
	ChildUnder5CountKnown         bool                                       `json:"child_under_5_count_known"`
	ChildUnder5AddsTraveler       bool                                       `json:"child_under_5_adds_traveler"`
	ChildUnder5AddsTravelerOrigin PassengerClarificationAddsTravelerOriginV1 `json:"child_under_5_adds_traveler_origin,omitempty"`
	ChildSlotStatus               PassengerClarificationSlotStatusV1         `json:"child_slot_status"`
	ChildPromptMessageID          string                                     `json:"child_prompt_message_id,omitempty"`
	ChildLastMessageID            string                                     `json:"child_last_message_id,omitempty"`
	ChildReasonCodes              []string                                   `json:"child_reason_codes,omitempty"`
	ChildReferences               []PassengerClarificationChildReferenceV1   `json:"child_references,omitempty"`

	AppliedEventIDs    []string `json:"applied_event_ids,omitempty"`
	AppliedMessageIDs  []string `json:"applied_message_ids,omitempty"`
	BootstrapCompleted bool     `json:"bootstrap_completed"`
	HasEvidence        bool     `json:"has_evidence"`
}

type PassengerClarificationEventV1 struct {
	Type                PassengerClarificationEventTypeV1        `json:"type"`
	Slot                PassengerClarificationSlotV1             `json:"slot"`
	EventID             string                                   `json:"event_id,omitempty"`
	MessageID           string                                   `json:"message_id"`
	PromptMessageID     string                                   `json:"prompt_message_id,omitempty"`
	Value               int                                      `json:"value,omitempty"`
	ValueKnown          bool                                     `json:"value_known,omitempty"`
	PassengerProvenance PassengerCountProvenance                 `json:"passenger_provenance,omitempty"`
	ChildReferences     []PassengerClarificationChildReferenceV1 `json:"child_references,omitempty"`
	ReasonCode          string                                   `json:"reason_code,omitempty"`
}

func newPassengerClarificationStateV1() PassengerClarificationStateV1 {
	return PassengerClarificationStateV1{
		Version:                  passengerClarificationStateV1Version,
		Authority:                PassengerClarificationAuthorityPreBooking,
		PassengerCountProvenance: PassengerCountProvenanceUnknown,
		PassengerSlotStatus:      PassengerClarificationSlotPending,
		ChildSlotStatus:          PassengerClarificationSlotPending,
	}
}

func passengerClarificationStateV1FromSession(session Session) (PassengerClarificationStateV1, bool) {
	memory := asMap(session.Metadata["memory"])
	return decodePassengerClarificationStateV1(memory[passengerClarificationStateV1MemoryKey])
}

func passengerClarificationStateV1ValueFromSession(session Session) (interface{}, bool) {
	memory := asMap(session.Metadata["memory"])
	value, ok := memory[passengerClarificationStateV1MemoryKey]
	return value, ok
}

func passengerClarificationStateV1OrDefault(session Session) PassengerClarificationStateV1 {
	if state, ok := passengerClarificationStateV1FromSession(session); ok {
		return state
	}
	return newPassengerClarificationStateV1()
}

func decodePassengerClarificationStateV1(value interface{}) (PassengerClarificationStateV1, bool) {
	if value == nil {
		return PassengerClarificationStateV1{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return PassengerClarificationStateV1{}, false
	}
	var state PassengerClarificationStateV1
	if err := json.Unmarshal(raw, &state); err != nil || state.Version != passengerClarificationStateV1Version {
		return PassengerClarificationStateV1{}, false
	}
	state = normalizePassengerClarificationStateV1(state)
	if validatePassengerClarificationStateV1(state) != nil {
		return PassengerClarificationStateV1{}, false
	}
	return state, true
}

func normalizePassengerClarificationStateV1(state PassengerClarificationStateV1) PassengerClarificationStateV1 {
	state.Version = passengerClarificationStateV1Version
	if state.Authority == "" {
		state.Authority = PassengerClarificationAuthorityPreBooking
	}
	if state.PassengerCountProvenance == "" {
		state.PassengerCountProvenance = PassengerCountProvenanceUnknown
	}
	if state.PassengerSlotStatus == "" {
		state.PassengerSlotStatus = PassengerClarificationSlotPending
	}
	if state.ChildSlotStatus == "" {
		state.ChildSlotStatus = PassengerClarificationSlotPending
	}
	return state
}

// ReducePassengerClarificationEventsV1 is pure with respect to text: its only
// inputs are a typed state and typed events. Duplicate event IDs are no-ops.
func ReducePassengerClarificationEventsV1(
	state PassengerClarificationStateV1,
	events []PassengerClarificationEventV1,
) PassengerClarificationStateV1 {
	state = normalizePassengerClarificationStateV1(state)
	if state.Authority == PassengerClarificationAuthorityPostBooking {
		return state
	}
	for _, event := range events {
		event.EventID = passengerClarificationEventIDV1(event)
		if event.EventID == "" || containsPassengerClarificationID(state.AppliedEventIDs, event.EventID) {
			continue
		}
		if !applyPassengerClarificationEventV1(&state, event) {
			continue
		}
		state.AppliedEventIDs = append(state.AppliedEventIDs, event.EventID)
		if event.MessageID != "" && !containsPassengerClarificationID(state.AppliedMessageIDs, event.MessageID) {
			state.AppliedMessageIDs = append(state.AppliedMessageIDs, event.MessageID)
		}
		state.HasEvidence = true
	}
	return reconcilePassengerClarificationStateV1(state)
}

func passengerClarificationEventIDV1(event PassengerClarificationEventV1) string {
	messageID := strings.TrimSpace(event.MessageID)
	if messageID == "" || event.Type == "" {
		return ""
	}
	return messageID + ":" + string(event.Type) + ":" + string(event.Slot)
}

func containsPassengerClarificationID(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func applyPassengerClarificationEventV1(state *PassengerClarificationStateV1, event PassengerClarificationEventV1) bool {
	if state == nil {
		return false
	}
	switch event.Type {
	case PassengerClarificationEventPassengerPromptOpened:
		if state.PassengerPromptMessageID == strings.TrimSpace(event.MessageID) &&
			state.PassengerSlotStatus == PassengerClarificationSlotAnswered {
			return true
		}
		resetPassengerClarificationPassengerSlotV1(state, event.MessageID, passengerClarificationEventReason(event, passengerClarificationReasonPassengerPromptOpened))
		return true
	case PassengerClarificationEventChildPromptOpened:
		if state.ChildPromptMessageID == strings.TrimSpace(event.MessageID) &&
			state.ChildSlotStatus == PassengerClarificationSlotAnswered {
			return true
		}
		resetPassengerClarificationChildSlotV1(state, event.MessageID, passengerClarificationEventReason(event, passengerClarificationReasonChildPromptOpened))
		return true
	case PassengerClarificationEventPassengerCountSet:
		return setPassengerClarificationPassengerSlotV1(state, event, false)
	case PassengerClarificationEventChildCountSet:
		return setPassengerClarificationChildSlotV1(state, event, false)
	case PassengerClarificationEventSlotCorrected:
		switch event.Slot {
		case PassengerClarificationSlotPassenger:
			return setPassengerClarificationPassengerSlotV1(state, event, true)
		case PassengerClarificationSlotChild:
			return setPassengerClarificationChildSlotV1(state, event, true)
		default:
			return false
		}
	case PassengerClarificationEventSlotInvalidated:
		invalidatePassengerClarificationSlotV1(state, event)
		return true
	default:
		return false
	}
}

func resetPassengerClarificationPassengerSlotV1(state *PassengerClarificationStateV1, promptMessageID string, reason string) {
	previousStatus := state.PassengerSlotStatus
	previousReasons := append([]string(nil), state.PassengerReasonCodes...)
	state.PassengerCount = 0
	state.PassengerCountKnown = false
	state.PassengerCountProvenance = PassengerCountProvenanceUnknown
	state.PassengerSlotStatus = PassengerClarificationSlotOpen
	state.PassengerPromptMessageID = strings.TrimSpace(promptMessageID)
	state.PassengerLastMessageID = strings.TrimSpace(promptMessageID)
	state.PassengerReasonCodes = []string{reason}
	if previousStatus == PassengerClarificationSlotPending || previousStatus == PassengerClarificationSlotConflicting {
		state.PassengerReasonCodes = mergeDistinctStrings(previousReasons, reason)
	}
}

func resetPassengerClarificationChildSlotV1(state *PassengerClarificationStateV1, promptMessageID string, reason string) {
	previousStatus := state.ChildSlotStatus
	previousReasons := append([]string(nil), state.ChildReasonCodes...)
	state.ChildUnder5Count = 0
	state.ChildUnder5CountKnown = false
	state.ChildUnder5AddsTraveler = false
	state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{}
	state.ChildSlotStatus = PassengerClarificationSlotOpen
	state.ChildPromptMessageID = strings.TrimSpace(promptMessageID)
	state.ChildLastMessageID = strings.TrimSpace(promptMessageID)
	state.ChildReasonCodes = []string{reason}
	if previousStatus == PassengerClarificationSlotPending || previousStatus == PassengerClarificationSlotConflicting {
		state.ChildReasonCodes = mergeDistinctStrings(previousReasons, reason)
	}
	state.ChildReferences = nil
}

func setPassengerClarificationPassengerSlotV1(state *PassengerClarificationStateV1, event PassengerClarificationEventV1, correction bool) bool {
	if !correction && state.PassengerPromptMessageID != "" &&
		strings.TrimSpace(event.PromptMessageID) != state.PassengerPromptMessageID {
		return false
	}
	if !event.ValueKnown || event.Value <= 0 {
		invalidatePassengerClarificationSlotV1(state, PassengerClarificationEventV1{
			Slot: PassengerClarificationSlotPassenger, MessageID: event.MessageID,
			ReasonCode: passengerClarificationEventReason(event, passengerClarificationReasonInvalidEvent),
		})
		return true
	}
	state.PassengerCount = event.Value
	state.PassengerCountKnown = true
	state.PassengerCountProvenance = event.PassengerProvenance
	if state.PassengerCountProvenance == "" {
		state.PassengerCountProvenance = PassengerCountProvenanceUnknown
	}
	state.PassengerSlotStatus = PassengerClarificationSlotAnswered
	if promptMessageID := strings.TrimSpace(event.PromptMessageID); promptMessageID != "" {
		state.PassengerPromptMessageID = promptMessageID
	}
	state.PassengerLastMessageID = strings.TrimSpace(event.MessageID)
	reason := passengerClarificationReasonPassengerCountAccepted
	if correction {
		reason = passengerClarificationReasonSlotCorrected
	}
	state.PassengerReasonCodes = []string{passengerClarificationEventReason(event, reason)}
	if correction && state.PassengerCountProvenance != PassengerCountProvenanceSoloSpeaker {
		state.ChildUnder5AddsTraveler = false
		state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{}
	}
	return true
}

func setPassengerClarificationChildSlotV1(state *PassengerClarificationStateV1, event PassengerClarificationEventV1, correction bool) bool {
	if !correction && state.ChildPromptMessageID != "" &&
		strings.TrimSpace(event.PromptMessageID) != state.ChildPromptMessageID {
		return false
	}
	if !event.ValueKnown || event.Value < 0 {
		invalidatePassengerClarificationSlotV1(state, PassengerClarificationEventV1{
			Slot: PassengerClarificationSlotChild, MessageID: event.MessageID,
			ReasonCode: passengerClarificationEventReason(event, passengerClarificationReasonInvalidEvent),
		})
		return true
	}
	state.ChildUnder5Count = event.Value
	state.ChildUnder5CountKnown = true
	state.ChildSlotStatus = PassengerClarificationSlotAnswered
	if promptMessageID := strings.TrimSpace(event.PromptMessageID); promptMessageID != "" {
		state.ChildPromptMessageID = promptMessageID
	}
	state.ChildLastMessageID = strings.TrimSpace(event.MessageID)
	reason := passengerClarificationReasonChildCountAccepted
	if correction {
		reason = passengerClarificationReasonSlotCorrected
	}
	state.ChildReasonCodes = []string{passengerClarificationEventReason(event, reason)}
	state.ChildReferences = clonePassengerClarificationChildReferencesV1(event.ChildReferences)
	if event.Value == 0 {
		state.ChildReferences = nil
		state.ChildUnder5AddsTraveler = false
		state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{}
		return true
	}
	answersPersistedChildPrompt := state.ChildPromptMessageID != "" &&
		strings.TrimSpace(event.PromptMessageID) == state.ChildPromptMessageID
	state.ChildUnder5AddsTraveler = answersPersistedChildPrompt &&
		state.PassengerCountKnown &&
		state.PassengerCountProvenance == PassengerCountProvenanceSoloSpeaker
	if state.ChildUnder5AddsTraveler {
		state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{
			SourceMessageID:          strings.TrimSpace(event.MessageID),
			SourcePromptMessageID:    state.ChildPromptMessageID,
			PassengerSourceMessageID: state.PassengerLastMessageID,
			ReasonCode:               passengerClarificationReasonSoloChildAddsTraveler,
		}
	} else {
		state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{}
	}
	return true
}

func invalidatePassengerClarificationSlotV1(state *PassengerClarificationStateV1, event PassengerClarificationEventV1) {
	reason := passengerClarificationEventReason(event, passengerClarificationReasonAmbiguousPassengerMeaning)
	status := PassengerClarificationSlotPending
	if reason == passengerClarificationReasonChildrenExceedPassengerTotal ||
		reason == passengerClarificationReasonAbsoluteTotalChildConflict ||
		reason == passengerClarificationReasonPassengerTotalsConflict {
		status = PassengerClarificationSlotConflicting
	}
	switch event.Slot {
	case PassengerClarificationSlotPassenger:
		state.PassengerCount = 0
		state.PassengerCountKnown = false
		state.PassengerCountProvenance = PassengerCountProvenanceUnknown
		state.PassengerSlotStatus = status
		state.PassengerLastMessageID = strings.TrimSpace(event.MessageID)
		state.PassengerReasonCodes = []string{reason}
	case PassengerClarificationSlotChild:
		state.ChildUnder5Count = 0
		state.ChildUnder5CountKnown = false
		state.ChildUnder5AddsTraveler = false
		state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{}
		state.ChildSlotStatus = status
		state.ChildLastMessageID = strings.TrimSpace(event.MessageID)
		state.ChildReasonCodes = []string{reason}
		state.ChildReferences = nil
	}
}

func reconcilePassengerClarificationStateV1(state PassengerClarificationStateV1) PassengerClarificationStateV1 {
	state = normalizePassengerClarificationStateV1(state)
	if !state.PassengerCountKnown || !state.ChildUnder5CountKnown {
		return state
	}
	reason := ""
	if state.ChildUnder5Count > state.PassengerCount {
		reason = passengerClarificationReasonChildrenExceedPassengerTotal
	} else if state.PassengerCount == 1 &&
		state.PassengerCountProvenance == PassengerCountProvenanceAbsoluteTotal &&
		state.ChildUnder5Count > 0 &&
		state.ChildPromptMessageID != "" {
		reason = passengerClarificationReasonAbsoluteTotalChildConflict
	}
	if reason == "" {
		return state
	}
	invalidatePassengerClarificationSlotV1(&state, PassengerClarificationEventV1{
		Slot: PassengerClarificationSlotPassenger, MessageID: state.PassengerLastMessageID, ReasonCode: reason,
	})
	invalidatePassengerClarificationSlotV1(&state, PassengerClarificationEventV1{
		Slot: PassengerClarificationSlotChild, MessageID: state.ChildLastMessageID, ReasonCode: reason,
	})
	return state
}

func validatePassengerClarificationStateV1(state PassengerClarificationStateV1) error {
	state = normalizePassengerClarificationStateV1(state)
	if state.Version != passengerClarificationStateV1Version {
		return ErrPassengerClarificationStateInvalid
	}
	if state.Authority != PassengerClarificationAuthorityPreBooking &&
		state.Authority != PassengerClarificationAuthorityPostBooking {
		return ErrPassengerClarificationStateInvalid
	}
	if !passengerClarificationSlotStatusValidV1(state.PassengerSlotStatus) ||
		!passengerClarificationSlotStatusValidV1(state.ChildSlotStatus) {
		return ErrPassengerClarificationStateInvalid
	}
	if state.PassengerCountKnown != (state.PassengerSlotStatus == PassengerClarificationSlotAnswered) ||
		state.ChildUnder5CountKnown != (state.ChildSlotStatus == PassengerClarificationSlotAnswered) {
		return ErrPassengerClarificationStateInvalid
	}
	if (state.PassengerCountKnown && state.PassengerCount <= 0) ||
		(!state.PassengerCountKnown && state.PassengerCount != 0) ||
		(state.ChildUnder5CountKnown && state.ChildUnder5Count < 0) ||
		(!state.ChildUnder5CountKnown && state.ChildUnder5Count != 0) {
		return ErrPassengerClarificationStateInvalid
	}
	if state.PassengerCountKnown {
		if !passengerCountProvenanceActionableV1(state.PassengerCountProvenance) {
			return ErrPassengerClarificationStateInvalid
		}
	} else if state.PassengerCountProvenance != PassengerCountProvenanceUnknown {
		return ErrPassengerClarificationStateInvalid
	}
	if state.PassengerSlotStatus == PassengerClarificationSlotOpen && state.PassengerPromptMessageID == "" {
		return ErrPassengerClarificationStateInvalid
	}
	if state.ChildSlotStatus == PassengerClarificationSlotOpen && state.ChildPromptMessageID == "" {
		return ErrPassengerClarificationStateInvalid
	}
	if len(state.ChildReferences) > state.ChildUnder5Count ||
		(len(state.ChildReferences) > 0 && !state.ChildUnder5CountKnown) {
		return ErrPassengerClarificationStateInvalid
	}
	if state.ChildUnder5AddsTraveler {
		origin := state.ChildUnder5AddsTravelerOrigin
		if !state.ChildUnder5CountKnown || state.ChildUnder5Count <= 0 ||
			!state.PassengerCountKnown || state.PassengerCountProvenance != PassengerCountProvenanceSoloSpeaker ||
			origin.SourceMessageID == "" || origin.SourcePromptMessageID == "" ||
			origin.SourcePromptMessageID != state.ChildPromptMessageID {
			return ErrPassengerClarificationStateInvalid
		}
	} else if state.ChildUnder5AddsTravelerOrigin != (PassengerClarificationAddsTravelerOriginV1{}) {
		return ErrPassengerClarificationStateInvalid
	}
	if state.Authority == PassengerClarificationAuthorityPostBooking {
		if strings.TrimSpace(state.BookingID) == "" || !state.PassengerCountKnown || !state.ChildUnder5CountKnown || state.ChildUnder5AddsTraveler {
			return ErrPassengerClarificationStateInvalid
		}
	}
	return nil
}

func passengerCountProvenanceActionableV1(provenance PassengerCountProvenance) bool {
	switch provenance {
	case PassengerCountProvenanceSoloSpeaker,
		PassengerCountProvenanceAbsoluteTotal,
		PassengerCountProvenanceIncludesSpeakerComposition:
		return true
	default:
		return false
	}
}

func passengerClarificationSlotStatusValidV1(status PassengerClarificationSlotStatusV1) bool {
	switch status {
	case PassengerClarificationSlotOpen, PassengerClarificationSlotPending,
		PassengerClarificationSlotAnswered, PassengerClarificationSlotConflicting:
		return true
	default:
		return false
	}
}

func invalidPassengerClarificationStateV1() PassengerClarificationStateV1 {
	state := newPassengerClarificationStateV1()
	state.BootstrapCompleted = true
	state.HasEvidence = true
	state.PassengerSlotStatus = PassengerClarificationSlotConflicting
	state.ChildSlotStatus = PassengerClarificationSlotConflicting
	state.PassengerReasonCodes = []string{passengerClarificationReasonInvalidState}
	state.ChildReasonCodes = []string{passengerClarificationReasonInvalidState}
	return state
}

type PassengerClarificationBookingAuthorityV1 struct {
	BookingID      string
	PassengerCount int
	LapChildCount  int
}

func bootstrapPassengerClarificationStateV1(
	existing *PassengerClarificationStateV1,
	booking *PassengerClarificationBookingAuthorityV1,
	events []PassengerClarificationEventV1,
) PassengerClarificationStateV1 {
	if existing != nil {
		state := normalizePassengerClarificationStateV1(*existing)
		state.BootstrapCompleted = true
		return state
	}
	if booking != nil && strings.TrimSpace(booking.BookingID) != "" && booking.PassengerCount > 0 &&
		booking.LapChildCount >= 0 && booking.LapChildCount <= booking.PassengerCount {
		messageID := "booking:" + strings.TrimSpace(booking.BookingID)
		return PassengerClarificationStateV1{
			Version:                  passengerClarificationStateV1Version,
			Authority:                PassengerClarificationAuthorityPostBooking,
			BookingID:                strings.TrimSpace(booking.BookingID),
			PassengerCount:           booking.PassengerCount,
			PassengerCountKnown:      true,
			PassengerCountProvenance: PassengerCountProvenanceAbsoluteTotal,
			PassengerSlotStatus:      PassengerClarificationSlotAnswered,
			PassengerLastMessageID:   messageID,
			PassengerReasonCodes:     []string{passengerClarificationReasonPassengerCountAccepted},
			ChildUnder5Count:         booking.LapChildCount,
			ChildUnder5CountKnown:    true,
			ChildSlotStatus:          PassengerClarificationSlotAnswered,
			ChildLastMessageID:       messageID,
			ChildReasonCodes:         []string{passengerClarificationReasonChildCountAccepted},
			BootstrapCompleted:       true,
			HasEvidence:              true,
		}
	}
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), events)
	state.BootstrapCompleted = true
	return state
}

func passengerClarificationStateConflictingV1(state PassengerClarificationStateV1) bool {
	return state.PassengerSlotStatus == PassengerClarificationSlotConflicting ||
		state.ChildSlotStatus == PassengerClarificationSlotConflicting
}

func passengerClarificationStateRequiresClarificationV1(state PassengerClarificationStateV1) bool {
	return passengerClarificationStateUnsafeV1(state)
}

func passengerClarificationStateUnsafeV1(state PassengerClarificationStateV1) bool {
	return validatePassengerClarificationStateV1(state) != nil ||
		passengerClarificationStateConflictingV1(state) ||
		!state.PassengerCountKnown || !state.ChildUnder5CountKnown
}

func passengerClarificationEventReason(event PassengerClarificationEventV1, fallback string) string {
	if reason := strings.TrimSpace(event.ReasonCode); reason != "" {
		return reason
	}
	return fallback
}

func clonePassengerClarificationChildReferencesV1(input []PassengerClarificationChildReferenceV1) []PassengerClarificationChildReferenceV1 {
	if len(input) == 0 {
		return nil
	}
	return append([]PassengerClarificationChildReferenceV1(nil), input...)
}
