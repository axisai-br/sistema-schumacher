package chat

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"time"
)

const (
	availabilityPromptEventV1MessageKey = "availability_prompt_event_v1"
	availabilityPromptEventV1Version    = 1
	availabilityPromptMaxOptionsV1      = 5
)

// AvailabilityPromptPresentationV1 keeps the raw tool result separate from
// the indexes that were actually rendered to the customer. ResultIndexes are
// zero-based indexes into Raw.Results; the public display index is assigned
// only when the durable event is built.
type AvailabilityPromptPresentationV1 struct {
	Raw           AvailabilitySearchResult
	ResultIndexes []int
}

type AvailabilityPromptPresentedOptionV1 struct {
	DisplayIndex int    `json:"display_index"`
	ResultIndex  int    `json:"result_index"`
	TripID       string `json:"trip_id"`
	BoardStopID  string `json:"board_stop_id"`
	AlightStopID string `json:"alight_stop_id"`
	TripDate     string `json:"trip_date"`
}

type AvailabilityPromptEventV1 struct {
	Version              int                                   `json:"version"`
	Kind                 ActivePromptKind                      `json:"kind"`
	SourceMessageID      string                                `json:"source_message_id"`
	PresentedOptionCount int                                   `json:"presented_option_count"`
	PresentedOptions     []AvailabilityPromptPresentedOptionV1 `json:"presented_options"`
}

type availabilityPromptAuthorityClassV1 string

const (
	availabilityPromptAuthorityUndeliveredV1     availabilityPromptAuthorityClassV1 = "UNDELIVERED"
	availabilityPromptAuthorityAbsentLegacyV1    availabilityPromptAuthorityClassV1 = "ABSENT_LEGACY"
	availabilityPromptAuthorityValidStructuralV1 availabilityPromptAuthorityClassV1 = "VALID_STRUCTURAL"
	availabilityPromptAuthorityInvalidV1         availabilityPromptAuthorityClassV1 = "INVALID"
)

type availabilityPromptAuthorityV1 struct {
	Class       availabilityPromptAuthorityClassV1
	Candidate   bool
	Prompt      bool
	Event       AvailabilityPromptEventV1
	Facts       *AvailabilitySearchResult
	Selection   *availabilityPromptSelectionDomainV1
	Presented   *AvailabilitySearchResult
	Kind        ActivePromptKind
	OptionCount int
}

type availabilityPromptReconciliationV1 struct {
	Candidate   bool
	Prompt      bool
	Invalid     bool
	Event       *AvailabilityPromptEventV1
	Facts       *AvailabilitySearchResult
	Selection   *availabilityPromptSelectionDomainV1
	Kind        ActivePromptKind
	OptionCount int
}

func availabilityPromptPresentationV1(
	raw AvailabilitySearchResult,
	resultIndexes []int,
) *AvailabilityPromptPresentationV1 {
	if len(resultIndexes) == 0 || len(resultIndexes) > availabilityPromptMaxOptionsV1 {
		return nil
	}
	seen := make(map[int]struct{}, len(resultIndexes))
	indexes := make([]int, 0, len(resultIndexes))
	for _, resultIndex := range resultIndexes {
		if resultIndex < 0 || resultIndex >= len(raw.Results) {
			return nil
		}
		if _, duplicate := seen[resultIndex]; duplicate {
			return nil
		}
		item := raw.Results[resultIndex]
		if !availabilityPromptItemValidV1(item) {
			return nil
		}
		seen[resultIndex] = struct{}{}
		indexes = append(indexes, resultIndex)
	}
	cloned := raw
	cloned.Results = append([]AvailabilitySearchItem(nil), raw.Results...)
	return &AvailabilityPromptPresentationV1{
		Raw:           cloned,
		ResultIndexes: indexes,
	}
}

func availabilityPromptEventForRunV1(
	run RunAgentResult,
	sourceMessageID string,
) (AvailabilityPromptEventV1, bool) {
	presentation := run.AvailabilityPresentation
	if presentation == nil {
		return AvailabilityPromptEventV1{}, false
	}
	event, ok := availabilityPromptEventFromPresentationV1(*presentation, sourceMessageID)
	if !ok {
		return AvailabilityPromptEventV1{}, false
	}
	return event, true
}

func availabilityPromptEventFromPresentationV1(
	presentation AvailabilityPromptPresentationV1,
	sourceMessageID string,
) (AvailabilityPromptEventV1, bool) {
	sourceMessageID = strings.TrimSpace(sourceMessageID)
	if sourceMessageID == "" ||
		len(presentation.ResultIndexes) == 0 ||
		len(presentation.ResultIndexes) > availabilityPromptMaxOptionsV1 {
		return AvailabilityPromptEventV1{}, false
	}

	event := AvailabilityPromptEventV1{
		Version:              availabilityPromptEventV1Version,
		Kind:                 ActivePromptAvailabilityOptionChoice,
		SourceMessageID:      sourceMessageID,
		PresentedOptionCount: len(presentation.ResultIndexes),
		PresentedOptions:     make([]AvailabilityPromptPresentedOptionV1, 0, len(presentation.ResultIndexes)),
	}
	seen := make(map[int]struct{}, len(presentation.ResultIndexes))
	for displayOffset, resultIndex := range presentation.ResultIndexes {
		if resultIndex < 0 || resultIndex >= len(presentation.Raw.Results) {
			return AvailabilityPromptEventV1{}, false
		}
		if _, duplicate := seen[resultIndex]; duplicate {
			return AvailabilityPromptEventV1{}, false
		}
		item := presentation.Raw.Results[resultIndex]
		if !availabilityPromptItemValidV1(item) {
			return AvailabilityPromptEventV1{}, false
		}
		seen[resultIndex] = struct{}{}
		event.PresentedOptions = append(event.PresentedOptions, AvailabilityPromptPresentedOptionV1{
			DisplayIndex: displayOffset + 1,
			ResultIndex:  resultIndex,
			TripID:       strings.TrimSpace(item.TripID),
			BoardStopID:  strings.TrimSpace(item.BoardStopID),
			AlightStopID: strings.TrimSpace(item.AlightStopID),
			TripDate:     strings.TrimSpace(item.TripDate),
		})
	}
	if !validateAvailabilityPromptEventV1(event) ||
		!availabilityPromptEventMatchesRawV1(event, presentation.Raw) {
		return AvailabilityPromptEventV1{}, false
	}
	return event, true
}

func availabilityPromptEventForOutboundV1(
	source Message,
	outboundMessageID string,
	contentUnchanged bool,
) (AvailabilityPromptEventV1, bool) {
	if !contentUnchanged || strings.TrimSpace(outboundMessageID) == "" {
		return AvailabilityPromptEventV1{}, false
	}
	event, ok := availabilityPromptEventFromMessageV1(source)
	if !ok || strings.TrimSpace(source.ID) == "" || event.SourceMessageID != strings.TrimSpace(source.ID) {
		return AvailabilityPromptEventV1{}, false
	}
	if _, factsPresent, factsValid := availabilityPromptPresentedContextV1(source, event); !factsPresent || !factsValid {
		return AvailabilityPromptEventV1{}, false
	}
	event.SourceMessageID = strings.TrimSpace(outboundMessageID)
	if !validateAvailabilityPromptEventV1(event) {
		return AvailabilityPromptEventV1{}, false
	}
	return event, true
}

func availabilityPromptMessageFromDeliveredSourceV1(
	source Message,
	delivered Message,
) (Message, bool) {
	sourceEvent, sourceOK := availabilityPromptEventFromMessageV1(source)
	deliveredEvent, deliveredOK := availabilityPromptEventFromMessageV1(delivered)
	if !sourceOK || !deliveredOK ||
		sourceEvent.SourceMessageID != strings.TrimSpace(source.ID) ||
		deliveredEvent.SourceMessageID != strings.TrimSpace(delivered.ID) ||
		!availabilityPromptEventsEquivalentProjectionV1(sourceEvent, deliveredEvent) {
		return Message{}, false
	}
	if _, factsPresent, factsValid := availabilityPromptPresentedContextV1(source, sourceEvent); !factsPresent || !factsValid {
		return Message{}, false
	}

	resolved := effectiveDeliveredPromptMessageV1(source, delivered)
	resolved.Payload = cloneMap(resolved.Payload)
	resolved.NormalizedPayload = cloneMap(resolved.NormalizedPayload)
	delete(resolved.Payload, "mode")
	delete(resolved.NormalizedPayload, "mode")
	resolved.Payload[availabilityPromptEventV1MessageKey] = deliveredEvent
	resolved.NormalizedPayload[availabilityPromptEventV1MessageKey] = deliveredEvent
	authority := classifyAvailabilityPromptCandidateV1(resolved)
	if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
		return Message{}, false
	}
	return resolved, true
}

func availabilityPromptEventsEquivalentProjectionV1(
	left AvailabilityPromptEventV1,
	right AvailabilityPromptEventV1,
) bool {
	left.SourceMessageID = ""
	right.SourceMessageID = ""
	return reflect.DeepEqual(left, right)
}

func availabilityPromptEventFromMessageV1(message Message) (AvailabilityPromptEventV1, bool) {
	leftRaw, leftPresent := message.Payload[availabilityPromptEventV1MessageKey]
	rightRaw, rightPresent := message.NormalizedPayload[availabilityPromptEventV1MessageKey]
	if !leftPresent || !rightPresent {
		return AvailabilityPromptEventV1{}, false
	}
	left, leftOK := decodeAvailabilityPromptEventV1(leftRaw)
	right, rightOK := decodeAvailabilityPromptEventV1(rightRaw)
	if !leftOK || !rightOK || !reflect.DeepEqual(left, right) {
		return AvailabilityPromptEventV1{}, false
	}
	return left, true
}

func decodeAvailabilityPromptEventV1(raw interface{}) (AvailabilityPromptEventV1, bool) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return AvailabilityPromptEventV1{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var event AvailabilityPromptEventV1
	if err := decoder.Decode(&event); err != nil {
		return AvailabilityPromptEventV1{}, false
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return AvailabilityPromptEventV1{}, false
	}
	if !validateAvailabilityPromptEventV1(event) {
		return AvailabilityPromptEventV1{}, false
	}
	return event, true
}

func validateAvailabilityPromptEventV1(event AvailabilityPromptEventV1) bool {
	if event.Version != availabilityPromptEventV1Version ||
		event.Kind != ActivePromptAvailabilityOptionChoice ||
		strings.TrimSpace(event.SourceMessageID) == "" ||
		event.PresentedOptionCount <= 0 ||
		event.PresentedOptionCount > availabilityPromptMaxOptionsV1 ||
		len(event.PresentedOptions) != event.PresentedOptionCount {
		return false
	}
	seenResults := make(map[int]struct{}, len(event.PresentedOptions))
	for offset, option := range event.PresentedOptions {
		if option.DisplayIndex != offset+1 || option.ResultIndex < 0 ||
			strings.TrimSpace(option.TripID) == "" ||
			strings.TrimSpace(option.BoardStopID) == "" ||
			strings.TrimSpace(option.AlightStopID) == "" ||
			!availabilityPromptTripDateValidV1(option.TripDate) {
			return false
		}
		if _, duplicate := seenResults[option.ResultIndex]; duplicate {
			return false
		}
		seenResults[option.ResultIndex] = struct{}{}
	}
	return true
}

func availabilityPromptTripDateValidV1(value string) bool {
	if value == "" {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func availabilityPromptItemValidV1(item AvailabilitySearchItem) bool {
	return hasCompleteAvailabilitySearchItemFacts(item) &&
		availabilityPromptTripDateValidV1(item.TripDate)
}

func availabilityPromptEventMatchesRawV1(
	event AvailabilityPromptEventV1,
	raw AvailabilitySearchResult,
) bool {
	if !validateAvailabilityPromptEventV1(event) {
		return false
	}
	for _, option := range event.PresentedOptions {
		if option.ResultIndex < 0 || option.ResultIndex >= len(raw.Results) {
			return false
		}
		item := raw.Results[option.ResultIndex]
		if !availabilityPromptItemValidV1(item) ||
			strings.TrimSpace(item.TripID) != strings.TrimSpace(option.TripID) ||
			strings.TrimSpace(item.BoardStopID) != strings.TrimSpace(option.BoardStopID) ||
			strings.TrimSpace(item.AlightStopID) != strings.TrimSpace(option.AlightStopID) ||
			strings.TrimSpace(item.TripDate) != strings.TrimSpace(option.TripDate) {
			return false
		}
	}
	return true
}

func classifyAvailabilityPromptCandidateV1(message Message) availabilityPromptAuthorityV1 {
	reconciled := reconcileAvailabilityPromptMessageV1(message)
	authority := availabilityPromptAuthorityV1{
		Class:       availabilityPromptAuthorityUndeliveredV1,
		Candidate:   reconciled.Candidate,
		Prompt:      reconciled.Prompt,
		Kind:        reconciled.Kind,
		OptionCount: reconciled.OptionCount,
	}
	if !confirmedOutboundDeliveryV1(message) {
		return authority
	}
	if availabilityDraftReviewMetadataInvalidV1(message) || reconciled.Invalid {
		authority.Class = availabilityPromptAuthorityInvalidV1
		return authority
	}

	if reconciled.Event == nil {
		authority.Class = availabilityPromptAuthorityAbsentLegacyV1
		authority.Facts = reconciled.Facts
		authority.Selection = reconciled.Selection
		return authority
	}
	payloadEvent := *reconciled.Event
	if isDeliveredPromptProjectionMessageV1(message) {
		authority.Class = availabilityPromptAuthorityInvalidV1
		return authority
	}
	presented, factsPresent, factsValid := availabilityPromptPresentedContextV1(message, payloadEvent)
	if !factsPresent || !factsValid || presented == nil {
		authority.Class = availabilityPromptAuthorityInvalidV1
		return authority
	}
	authority.Class = availabilityPromptAuthorityValidStructuralV1
	authority.Event = payloadEvent
	authority.Facts = reconciled.Facts
	authority.Selection = reconciled.Selection
	authority.Presented = presented
	authority.Kind = payloadEvent.Kind
	authority.OptionCount = payloadEvent.PresentedOptionCount
	return authority
}

func availabilityDraftReviewMetadataInvalidV1(message Message) bool {
	modePresent := false
	mode := ""
	actionPresent := false
	action := ""
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if payload == nil {
			continue
		}
		if raw, exists := payload["mode"]; exists {
			candidate := strings.ToUpper(strings.TrimSpace(asString(raw)))
			if candidate == string(deliveredPromptProjectionDraftReviewV1) {
				modePresent = true
			}
			if mode != "" && candidate != mode {
				return modePresent || candidate == string(deliveredPromptProjectionDraftReviewV1)
			}
			mode = candidate
		}
		if raw, exists := payload["review_action"]; exists {
			candidate := strings.ToUpper(strings.TrimSpace(asString(raw)))
			if actionPresent && candidate != action {
				return modePresent
			}
			action = candidate
			actionPresent = true
		}
	}
	if !modePresent {
		return false
	}
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		if !strings.EqualFold(strings.TrimSpace(asString(payload["mode"])), string(deliveredPromptProjectionDraftReviewV1)) ||
			!strings.EqualFold(strings.TrimSpace(asString(payload["review_action"])), "APPROVED_AS_IS") {
			return true
		}
	}
	return !actionPresent || action != "APPROVED_AS_IS"
}

func availabilityPromptPresentedContextV1(
	message Message,
	event AvailabilityPromptEventV1,
) (*AvailabilitySearchResult, bool, bool) {
	contexts, factsPresent, factsValid := availabilityPromptRawContextsV1(message)
	if !factsPresent {
		return nil, false, false
	}
	if !factsValid || len(contexts) != 2 || !reflect.DeepEqual(contexts[0], contexts[1]) {
		return nil, true, false
	}
	raw := contexts[0]
	if !availabilityPromptEventMatchesRawV1(event, raw) {
		return nil, true, false
	}
	presented := raw
	presented.Results = make([]AvailabilitySearchItem, 0, event.PresentedOptionCount)
	for _, option := range event.PresentedOptions {
		presented.Results = append(presented.Results, raw.Results[option.ResultIndex])
	}
	presented.Filter.Limit = len(presented.Results)
	return &presented, true, true
}

func availabilityPromptRawContextsV1(message Message) ([]AvailabilitySearchResult, bool, bool) {
	payload := availabilityPromptDomainAvailabilityFactsV1(message.Payload)
	normalized := availabilityPromptDomainAvailabilityFactsV1(message.NormalizedPayload)
	present := payload.Present || normalized.Present
	if !present {
		return nil, false, true
	}
	if !payload.Present || !normalized.Present || !payload.Valid || !normalized.Valid ||
		!reflect.DeepEqual(payload.Value, normalized.Value) {
		return nil, true, false
	}
	left, leftOK := payload.Value.(AvailabilitySearchResult)
	right, rightOK := normalized.Value.(AvailabilitySearchResult)
	if !leftOK || !rightOK {
		return nil, true, false
	}
	return []AvailabilitySearchResult{left, right}, true, true
}

func availabilityPromptClassifiedContextV1(message Message) (*AvailabilitySearchResult, bool) {
	authority := classifyAvailabilityPromptCandidateV1(message)
	switch authority.Class {
	case availabilityPromptAuthorityValidStructuralV1:
		return authority.Presented, authority.Presented != nil
	case availabilityPromptAuthorityAbsentLegacyV1:
		if !authority.Candidate || authority.Facts == nil {
			return nil, false
		}
		result := *authority.Facts
		return &result, true
	default:
		return nil, false
	}
}

func messageHasAvailabilityPromptEventV1(message Message) bool {
	_, payloadPresent := message.Payload[availabilityPromptEventV1MessageKey]
	_, normalizedPresent := message.NormalizedPayload[availabilityPromptEventV1MessageKey]
	return payloadPresent || normalizedPresent
}

type availabilityPromptDomainClassV1 string

const (
	availabilityPromptDomainAbsentV1          availabilityPromptDomainClassV1 = "ABSENT"
	availabilityPromptDomainAvailabilityV1    availabilityPromptDomainClassV1 = "AVAILABILITY"
	availabilityPromptDomainNonAvailabilityV1 availabilityPromptDomainClassV1 = "NON_AVAILABILITY"
	availabilityPromptDomainInvalidV1         availabilityPromptDomainClassV1 = "INVALID"
)

type availabilityPromptDomainMetadataV1 struct {
	Present bool
	Valid   bool
	Value   string
	Domain  availabilityPromptDomainClassV1
}

type availabilityPromptDomainArtifactV1 struct {
	Present bool
	Valid   bool
	Value   interface{}
}

type availabilityPromptSelectionDomainV1 struct {
	SelectedOptionIndexPresent bool
	SelectedOptionIndex        int
	SelectedResultPresent      bool
	SelectedResult             map[string]interface{}
	AuthorityMarkerPresent     bool
	AuthorityMarker            bool
}

type availabilityPromptDomainCopyV1 struct {
	Class                 availabilityPromptDomainClassV1
	ToolContextMalformed  bool
	Intent                availabilityPromptDomainMetadataV1
	Template              availabilityPromptDomainMetadataV1
	PromptKind            availabilityPromptDomainMetadataV1
	AvailabilityEvent     availabilityPromptDomainArtifactV1
	AvailabilityFacts     availabilityPromptDomainArtifactV1
	AvailabilitySelection availabilityPromptDomainArtifactV1
	PassengerPromptEvent  availabilityPromptDomainArtifactV1
	PassengerPendingEvent availabilityPromptDomainArtifactV1
}

func messageMayCarryAvailabilityPromptV1(message Message) bool {
	return reconcileAvailabilityPromptMessageV1(message).Candidate
}

func reconcileAvailabilityPromptMessageV1(message Message) availabilityPromptReconciliationV1 {
	payload := availabilityPromptDomainCopyFromPayloadV1(message.Payload, message.ID)
	normalized := availabilityPromptDomainCopyFromPayloadV1(message.NormalizedPayload, message.ID)
	reconciled := availabilityPromptReconciliationV1{}

	availabilityArtifacts := [][2]availabilityPromptDomainArtifactV1{
		{payload.AvailabilityEvent, normalized.AvailabilityEvent},
		{payload.AvailabilityFacts, normalized.AvailabilityFacts},
		{payload.AvailabilitySelection, normalized.AvailabilitySelection},
	}
	passengerArtifacts := [][2]availabilityPromptDomainArtifactV1{
		{payload.PassengerPromptEvent, normalized.PassengerPromptEvent},
		{payload.PassengerPendingEvent, normalized.PassengerPendingEvent},
	}
	hasAvailabilityArtifact := false
	hasPassengerArtifact := false
	validAvailabilityArtifact := false
	validPassengerArtifact := false
	invalidAvailabilityArtifact := false
	invalidPassengerArtifact := false
	malformedToolContext := payload.ToolContextMalformed || normalized.ToolContextMalformed
	for _, pair := range availabilityArtifacts {
		if !pair[0].Present && !pair[1].Present {
			continue
		}
		hasAvailabilityArtifact = true
		if !availabilityPromptDomainArtifactPairValidV1(pair[0], pair[1]) {
			invalidAvailabilityArtifact = true
			continue
		}
		validAvailabilityArtifact = true
	}
	for _, pair := range passengerArtifacts {
		if !pair[0].Present && !pair[1].Present {
			continue
		}
		hasPassengerArtifact = true
		if !availabilityPromptDomainArtifactPairValidV1(pair[0], pair[1]) {
			invalidPassengerArtifact = true
			continue
		}
		validPassengerArtifact = true
	}
	_, _, bodyCarriesAvailability := legacyAvailabilityPromptBodyV1(messageTurnText(message))
	reconciled.Candidate = hasAvailabilityArtifact ||
		availabilityPromptCopyHasAvailabilityMetadataV1(payload) ||
		availabilityPromptCopyHasAvailabilityMetadataV1(normalized) ||
		bodyCarriesAvailability
	if availabilityPromptDomainArtifactPairValidV1(payload.AvailabilityEvent, normalized.AvailabilityEvent) {
		event := payload.AvailabilityEvent.Value.(AvailabilityPromptEventV1)
		reconciled.Event = &event
	}
	if availabilityPromptDomainArtifactPairValidV1(payload.AvailabilityFacts, normalized.AvailabilityFacts) {
		facts := payload.AvailabilityFacts.Value.(AvailabilitySearchResult)
		reconciled.Facts = &facts
	}
	if availabilityPromptDomainArtifactPairValidV1(payload.AvailabilitySelection, normalized.AvailabilitySelection) {
		selection := payload.AvailabilitySelection.Value.(availabilityPromptSelectionDomainV1)
		reconciled.Selection = &selection
	}

	metadataDomain := availabilityPromptDomainAbsentV1
	metadataInvalid := false
	for _, pair := range [][2]availabilityPromptDomainMetadataV1{
		{payload.Intent, normalized.Intent},
		{payload.Template, normalized.Template},
		{payload.PromptKind, normalized.PromptKind},
	} {
		if !pair[0].Present && !pair[1].Present {
			continue
		}
		if !availabilityPromptDomainMetadataPairValidV1(pair[0], pair[1]) {
			metadataInvalid = true
			continue
		}
		if metadataDomain != availabilityPromptDomainAbsentV1 && metadataDomain != pair[0].Domain {
			metadataInvalid = true
			continue
		}
		metadataDomain = pair[0].Domain
	}

	if metadataInvalid || invalidPassengerArtifact ||
		(validAvailabilityArtifact && validPassengerArtifact) ||
		(hasAvailabilityArtifact && hasPassengerArtifact) ||
		(metadataDomain == availabilityPromptDomainAvailabilityV1 && hasPassengerArtifact) ||
		(metadataDomain == availabilityPromptDomainNonAvailabilityV1 && hasAvailabilityArtifact) ||
		(bodyCarriesAvailability &&
			(hasPassengerArtifact || metadataDomain == availabilityPromptDomainNonAvailabilityV1)) {
		reconciled.Invalid = true
		return reconciled
	}
	if malformedToolContext && !hasAvailabilityArtifact {
		reconciled.Invalid = true
		return reconciled
	}
	if malformedToolContext {
		invalidAvailabilityArtifact = true
	}
	if invalidAvailabilityArtifact {
		reconciled.Invalid = true
		return reconciled
	}

	if validAvailabilityArtifact {
		reconciled.Candidate = true
	} else if validPassengerArtifact || metadataDomain == availabilityPromptDomainNonAvailabilityV1 {
		return reconciled
	} else if metadataDomain == availabilityPromptDomainAvailabilityV1 {
		reconciled.Candidate = true
	}

	if reconciled.Event != nil {
		event := *reconciled.Event
		reconciled.Prompt = true
		reconciled.Kind = event.Kind
		reconciled.OptionCount = event.PresentedOptionCount
	}
	if metadataDomain == availabilityPromptDomainAvailabilityV1 {
		reconciled.Prompt = true
		if kind, ok := reconciledAvailabilityPromptMetadataKindV1(payload); ok {
			reconciled.Kind = kind
		}
	}

	if !hasAvailabilityArtifact && metadataDomain == availabilityPromptDomainAbsentV1 {
		kind, count, ok := legacyAvailabilityPromptBodyV1(messageTurnText(message))
		if ok {
			reconciled.Candidate = true
			reconciled.Prompt = true
			reconciled.Kind = kind
			reconciled.OptionCount = count
		}
	} else if reconciled.Candidate {
		kind, count, ok := legacyAvailabilityPromptBodyV1(messageTurnText(message))
		if ok {
			if reconciled.Kind != "" && reconciled.Kind != ActivePromptUnknown && reconciled.Kind != kind {
				reconciled.Invalid = true
				return reconciled
			}
			reconciled.Prompt = true
			reconciled.Kind = kind
			reconciled.OptionCount = count
		}
	}
	return reconciled
}

func availabilityPromptCopyHasAvailabilityMetadataV1(copy availabilityPromptDomainCopyV1) bool {
	for _, metadata := range []availabilityPromptDomainMetadataV1{copy.Intent, copy.Template, copy.PromptKind} {
		if metadata.Present && metadata.Valid && metadata.Domain == availabilityPromptDomainAvailabilityV1 {
			return true
		}
	}
	return false
}

func availabilityPromptCopyHasNonAvailabilityMetadataV1(copy availabilityPromptDomainCopyV1) bool {
	for _, metadata := range []availabilityPromptDomainMetadataV1{copy.Intent, copy.Template, copy.PromptKind} {
		if metadata.Present && metadata.Valid && metadata.Domain == availabilityPromptDomainNonAvailabilityV1 {
			return true
		}
	}
	return false
}

func availabilityPromptDomainArtifactPairValidV1(
	payload availabilityPromptDomainArtifactV1,
	normalized availabilityPromptDomainArtifactV1,
) bool {
	return payload.Present && normalized.Present && payload.Valid && normalized.Valid &&
		reflect.DeepEqual(payload.Value, normalized.Value)
}

func availabilityPromptDomainMetadataPairValidV1(
	payload availabilityPromptDomainMetadataV1,
	normalized availabilityPromptDomainMetadataV1,
) bool {
	if payload.Present != normalized.Present {
		return false
	}
	if !payload.Present {
		return true
	}
	return payload.Valid && normalized.Valid && reflect.DeepEqual(payload, normalized)
}

func availabilityPromptMetadataDomainV1(copy availabilityPromptDomainCopyV1) availabilityPromptDomainClassV1 {
	hasAvailability := false
	hasNonAvailability := false
	for _, metadata := range []availabilityPromptDomainMetadataV1{copy.Intent, copy.Template, copy.PromptKind} {
		if !metadata.Present {
			continue
		}
		if !metadata.Valid {
			return availabilityPromptDomainInvalidV1
		}
		switch metadata.Domain {
		case availabilityPromptDomainAvailabilityV1:
			hasAvailability = true
		case availabilityPromptDomainNonAvailabilityV1:
			hasNonAvailability = true
		default:
			return availabilityPromptDomainInvalidV1
		}
	}
	if hasAvailability && hasNonAvailability {
		return availabilityPromptDomainInvalidV1
	}
	if hasAvailability {
		return availabilityPromptDomainAvailabilityV1
	}
	if hasNonAvailability {
		return availabilityPromptDomainNonAvailabilityV1
	}
	return availabilityPromptDomainAbsentV1
}

func availabilityPromptMetadataEqualV1(left availabilityPromptDomainCopyV1, right availabilityPromptDomainCopyV1) bool {
	return reflect.DeepEqual(left.Intent, right.Intent) &&
		reflect.DeepEqual(left.Template, right.Template) &&
		reflect.DeepEqual(left.PromptKind, right.PromptKind)
}

func availabilityPromptHasAnyDomainArtifactV1(copy availabilityPromptDomainCopyV1) bool {
	for _, artifact := range []availabilityPromptDomainArtifactV1{
		copy.AvailabilityEvent,
		copy.AvailabilityFacts,
		copy.AvailabilitySelection,
		copy.PassengerPromptEvent,
		copy.PassengerPendingEvent,
	} {
		if artifact.Present {
			return true
		}
	}
	return false
}

func availabilityPromptDomainCopyFromPayloadV1(payload map[string]interface{}, messageID string) availabilityPromptDomainCopyV1 {
	copy := availabilityPromptDomainCopyV1{
		Intent:                availabilityPromptDomainMetadataFromPayloadV1(payload, "intent", availabilityPromptIntentDomainV1),
		Template:              availabilityPromptDomainMetadataFromPayloadV1(payload, "template_name", availabilityPromptTemplateDomainV1),
		PromptKind:            availabilityPromptDomainMetadataFromPayloadV1(payload, outOfTurnActivePromptTemplateDataKey, availabilityPromptKindDomainV1),
		AvailabilityEvent:     availabilityPromptDomainAvailabilityEventV1(payload, messageID),
		AvailabilityFacts:     availabilityPromptDomainAvailabilityFactsV1(payload),
		AvailabilitySelection: availabilityPromptDomainAvailabilitySelectionV1(payload),
		PassengerPromptEvent:  availabilityPromptDomainPassengerEventV1(payload, passengerPromptEventV1MessageKey, messageID, false),
		PassengerPendingEvent: availabilityPromptDomainPassengerEventV1(payload, passengerPendingPromptEventV1MessageKey, messageID, true),
	}
	if availabilityPromptHasOutOfTurnSourceLinkV1(payload) {
		// active_prompt_kind describes the linked earlier prompt, not this
		// message's own domain. Availability artifacts remain classified below.
		copy.PromptKind = availabilityPromptDomainMetadataV1{}
	}
	if toolContext, present := payload["tool_context"]; present && asMap(toolContext) == nil {
		copy.ToolContextMalformed = true
	}
	copy.Class = availabilityPromptDomainClassifyCopyV1(copy)
	return copy
}

func availabilityPromptHasOutOfTurnSourceLinkV1(payload map[string]interface{}) bool {
	data := asMap(payload["template_data"])
	if !readBool(data[outOfTurnTemplateDataKey]) ||
		strings.TrimSpace(asString(data[outOfTurnActivePromptSourceIDDataKey])) == "" {
		return false
	}
	kind := strings.ToUpper(strings.TrimSpace(asString(data[outOfTurnActivePromptTemplateDataKey])))
	domain, valid := availabilityPromptKindDomainV1(kind)
	return valid && domain == availabilityPromptDomainAvailabilityV1
}

func availabilityPromptDomainMetadataFromPayloadV1(
	payload map[string]interface{},
	key string,
	classify func(string) (availabilityPromptDomainClassV1, bool),
) availabilityPromptDomainMetadataV1 {
	evidence := availabilityPromptDomainMetadataV1{}
	containers := []map[string]interface{}{payload}
	for _, nestedKey := range []string{"request_payload", "response_payload", "template_data"} {
		if nested := asMap(payload[nestedKey]); nested != nil {
			containers = append(containers, nested)
		}
	}
	for _, container := range containers {
		raw, present := container[key]
		if !present {
			continue
		}
		evidence.Present = true
		value, stringValue := raw.(string)
		value = strings.ToUpper(strings.TrimSpace(value))
		domain, recognized := classify(value)
		if !stringValue || value == "" || !recognized {
			evidence.Valid = false
			return evidence
		}
		if evidence.Value != "" && (evidence.Value != value || evidence.Domain != domain) {
			evidence.Valid = false
			return evidence
		}
		evidence.Valid = true
		evidence.Value = value
		evidence.Domain = domain
	}
	return evidence
}

func availabilityPromptIntentDomainV1(value string) (availabilityPromptDomainClassV1, bool) {
	switch Intent(value) {
	case IntentAvailabilitySearch, IntentSelectAvailabilityOption:
		return availabilityPromptDomainAvailabilityV1, true
	case IntentPassengerCountReply,
		IntentPassengerDocumentsProvided,
		IntentLapChildAssignmentAnswer,
		IntentDocumentConfirmation,
		IntentBookingCreateConfirmation,
		IntentPaymentPreference,
		IntentPaymentInfoQuestion,
		IntentPaymentMethodQuestion,
		IntentPayingPassengerInfoQuestion,
		IntentDocumentRequirementsInfoQuestion,
		IntentChildPolicyInfoQuestion,
		IntentBaggageInfoQuestion,
		IntentBoardingInfoQuestion,
		IntentHumanSupportInfoQuestion,
		IntentPaymentStatusQuery,
		IntentPaymentCreate,
		IntentBookingCancel,
		IntentReschedule,
		IntentUnsupportedCargo,
		IntentUnsupportedPackage,
		IntentHumanSupport:
		return availabilityPromptDomainNonAvailabilityV1, true
	default:
		return availabilityPromptDomainInvalidV1, false
	}
}

func availabilityPromptTemplateDomainV1(value string) (availabilityPromptDomainClassV1, bool) {
	switch ResponseTemplateName(value) {
	case TemplateAvailabilityList,
		TemplateAvailabilityEarliest,
		TemplateContextFallbackAvailabilityOption,
		TemplateContextFallbackAvailabilityDate:
		return availabilityPromptDomainAvailabilityV1, true
	case TemplateAskPassengerCount,
		TemplateAskChildUnder5,
		TemplateAskDocuments,
		TemplateAskLapChildAssignment,
		TemplateAskPaymentChoice,
		TemplateAskMAOrigin,
		TemplateAskMADestination,
		TemplateAskSCOrigin,
		TemplateAskSCOriginForMA,
		TemplateAskReservationRouteSC,
		TemplatePublicSCTable,
		TemplateUnsupportedCargo,
		TemplateUnsupportedPackage,
		TemplateHumanHandoff,
		TemplateBookingCreated,
		TemplateConfirmDocument,
		TemplatePaymentCreate,
		TemplatePaymentMethods,
		TemplatePaymentOptionsInfo,
		TemplatePayingPassengerInfo,
		TemplateDocumentRequirementsInfo,
		TemplateChildPolicyInfo,
		TemplateBaggageInfo,
		TemplateBoardingInfo,
		TemplateHumanSupportInfo,
		TemplateContextFallbackPassengerCount,
		TemplateContextFallbackChildUnder5,
		TemplateContextFallbackLapChildAssignment,
		TemplateContextFallbackPassengerDocuments,
		TemplateContextFallbackDocumentConfirmation,
		TemplateContextFallbackPaymentPreference,
		TemplateContextFallbackPayerCPF:
		return availabilityPromptDomainNonAvailabilityV1, true
	default:
		return availabilityPromptDomainInvalidV1, false
	}
}

func availabilityPromptKindDomainV1(value string) (availabilityPromptDomainClassV1, bool) {
	switch ActivePromptKind(value) {
	case ActivePromptAvailabilityDateChoice, ActivePromptAvailabilityOptionChoice:
		return availabilityPromptDomainAvailabilityV1, true
	case ActivePromptReservationRoute,
		ActivePromptPassengerCount,
		ActivePromptLapChildQuestion,
		ActivePromptLapChildAssignment,
		ActivePromptPassengerDocuments,
		ActivePromptDocumentConfirmation,
		ActivePromptPaymentPreference,
		ActivePromptPayerCPF:
		return availabilityPromptDomainNonAvailabilityV1, true
	default:
		return availabilityPromptDomainInvalidV1, false
	}
}

func availabilityPromptDomainAvailabilityEventV1(payload map[string]interface{}, messageID string) availabilityPromptDomainArtifactV1 {
	raw, present := payload[availabilityPromptEventV1MessageKey]
	if !present {
		return availabilityPromptDomainArtifactV1{}
	}
	artifact := availabilityPromptDomainArtifactV1{Present: true}
	event, valid := decodeAvailabilityPromptEventV1(raw)
	if !valid || event.SourceMessageID != strings.TrimSpace(messageID) {
		return artifact
	}
	artifact.Valid = true
	artifact.Value = event
	return artifact
}

func availabilityPromptDomainAvailabilityFactsV1(payload map[string]interface{}) availabilityPromptDomainArtifactV1 {
	toolContextRaw, toolContextPresent := payload["tool_context"]
	if !toolContextPresent {
		return availabilityPromptDomainArtifactV1{}
	}
	toolContext := asMap(toolContextRaw)
	if toolContext == nil {
		// A malformed generic tool_context does not identify a functional domain
		// by itself. When the peer copy contains availability_search, bilateral
		// reconciliation still sees the missing copy and fails closed.
		return availabilityPromptDomainArtifactV1{}
	}
	availabilityRaw, present := toolContext[toolNameAvailabilitySearch]
	if !present {
		return availabilityPromptDomainArtifactV1{}
	}
	artifact := availabilityPromptDomainArtifactV1{Present: true}
	availabilityPayload := asMap(availabilityRaw)
	if availabilityPayload == nil {
		return artifact
	}
	result, valid := parseAvailabilityContextPayloadWithValidityV1(availabilityPayload)
	if !valid {
		return artifact
	}
	artifact.Valid = true
	artifact.Value = result
	return artifact
}

func availabilityPromptDomainAvailabilitySelectionV1(payload map[string]interface{}) availabilityPromptDomainArtifactV1 {
	value := availabilityPromptSelectionDomainV1{}
	artifact := availabilityPromptDomainArtifactV1{}
	containers := []map[string]interface{}{payload}
	for _, nestedKey := range []string{"request_payload", "response_payload"} {
		if nested := asMap(payload[nestedKey]); nested != nil {
			containers = append(containers, nested)
		}
	}
	for _, container := range containers {
		if raw, present := container["selected_option_index"]; present {
			artifact.Present = true
			index, valid := availabilityPromptStrictPositiveIndexV1(raw)
			if !valid || (value.SelectedOptionIndexPresent && value.SelectedOptionIndex != index) {
				return artifact
			}
			value.SelectedOptionIndexPresent = true
			value.SelectedOptionIndex = index
		}
		if raw, present := container[selectedAvailabilityResultPayloadKey]; present {
			artifact.Present = true
			snapshot := asMap(raw)
			canonicalSnapshot, canonical := canonicalAvailabilityPromptMapV1(snapshot)
			if len(snapshot) == 0 || !canonical ||
				(value.SelectedResultPresent && !reflect.DeepEqual(value.SelectedResult, canonicalSnapshot)) {
				return artifact
			}
			value.SelectedResultPresent = true
			value.SelectedResult = canonicalSnapshot
		}
		if raw, present := container[availabilitySelectionMaterializesAuthorityPayloadKey]; present {
			artifact.Present = true
			marker, valid := raw.(bool)
			if !valid || (value.AuthorityMarkerPresent && value.AuthorityMarker != marker) {
				return artifact
			}
			value.AuthorityMarkerPresent = true
			value.AuthorityMarker = marker
		}
	}
	if !artifact.Present {
		return artifact
	}
	if value.SelectedResultPresent {
		index := value.SelectedOptionIndex
		if !value.SelectedOptionIndexPresent {
			var valid bool
			index, valid = availabilityPromptStrictPositiveIndexV1(value.SelectedResult["selected_option_index"])
			if !valid {
				return artifact
			}
		}
		if !selectedAvailabilitySnapshotMaterializesIndex(value.SelectedResult, index) {
			return artifact
		}
	}
	artifact.Valid = true
	artifact.Value = value
	return artifact
}

func canonicalAvailabilityPromptMapV1(value map[string]interface{}) (map[string]interface{}, bool) {
	if len(value) == 0 {
		return nil, false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	canonical := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return nil, false
	}
	return canonical, true
}

func availabilityPromptStrictPositiveIndexV1(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, value > 0
	case int32:
		return int(value), value > 0
	case int64:
		return int(value), value > 0
	case float32:
		index := int(value)
		return index, index > 0 && float32(index) == value
	case float64:
		index := int(value)
		return index, index > 0 && float64(index) == value
	default:
		return 0, false
	}
}

func availabilityPromptDomainPassengerEventV1(
	payload map[string]interface{},
	key string,
	messageID string,
	pending bool,
) availabilityPromptDomainArtifactV1 {
	raw, present := payload[key]
	if !present {
		return availabilityPromptDomainArtifactV1{}
	}
	artifact := availabilityPromptDomainArtifactV1{Present: true}
	event, valid := decodePassengerClarificationEventV1(raw)
	if !valid {
		return artifact
	}
	if pending {
		if !passengerClarificationEventIsPromptV1(event) {
			return artifact
		}
	} else {
		var canonical bool
		event, canonical = passengerClarificationEventForMessageV1(event, messageID)
		if !canonical || !passengerClarificationEventIsPromptV1(event) {
			return artifact
		}
	}
	artifact.Valid = true
	artifact.Value = event
	return artifact
}

func availabilityPromptDomainClassifyCopyV1(copy availabilityPromptDomainCopyV1) availabilityPromptDomainClassV1 {
	hasAvailability := false
	hasNonAvailability := false
	for _, metadata := range []availabilityPromptDomainMetadataV1{copy.Intent, copy.Template, copy.PromptKind} {
		if !metadata.Present {
			continue
		}
		if !metadata.Valid {
			return availabilityPromptDomainInvalidV1
		}
		switch metadata.Domain {
		case availabilityPromptDomainAvailabilityV1:
			hasAvailability = true
		case availabilityPromptDomainNonAvailabilityV1:
			hasNonAvailability = true
		default:
			return availabilityPromptDomainInvalidV1
		}
	}
	for _, artifact := range []availabilityPromptDomainArtifactV1{
		copy.AvailabilityEvent,
		copy.AvailabilityFacts,
		copy.AvailabilitySelection,
	} {
		if artifact.Present && !artifact.Valid {
			return availabilityPromptDomainInvalidV1
		}
		hasAvailability = hasAvailability || artifact.Present
	}
	for _, artifact := range []availabilityPromptDomainArtifactV1{
		copy.PassengerPromptEvent,
		copy.PassengerPendingEvent,
	} {
		if artifact.Present && !artifact.Valid {
			return availabilityPromptDomainInvalidV1
		}
		hasNonAvailability = hasNonAvailability || artifact.Present
	}
	if hasAvailability && hasNonAvailability {
		return availabilityPromptDomainInvalidV1
	}
	if hasAvailability {
		return availabilityPromptDomainAvailabilityV1
	}
	if hasNonAvailability {
		return availabilityPromptDomainNonAvailabilityV1
	}
	return availabilityPromptDomainAbsentV1
}

func availabilityPromptHasBilateralStructuralDomainV1(
	payload availabilityPromptDomainCopyV1,
	normalized availabilityPromptDomainCopyV1,
) bool {
	for _, artifacts := range [][2]availabilityPromptDomainArtifactV1{
		{payload.AvailabilityEvent, normalized.AvailabilityEvent},
		{payload.AvailabilityFacts, normalized.AvailabilityFacts},
		{payload.AvailabilitySelection, normalized.AvailabilitySelection},
	} {
		if artifacts[0].Present && artifacts[0].Valid &&
			artifacts[1].Present && artifacts[1].Valid &&
			reflect.DeepEqual(artifacts[0].Value, artifacts[1].Value) {
			return true
		}
	}
	return false
}

func reconciledAvailabilityPromptKindV1(message Message) (ActivePromptKind, bool) {
	reconciled := reconcileAvailabilityPromptMessageV1(message)
	if !reconciled.Candidate || reconciled.Invalid || !reconciled.Prompt ||
		reconciled.Kind == ActivePromptUnknown {
		return ActivePromptUnknown, false
	}
	return reconciled.Kind, true
}

func reconciledAvailabilityPromptMetadataKindV1(payload availabilityPromptDomainCopyV1) (ActivePromptKind, bool) {
	if payload.PromptKind.Present {
		return ActivePromptKind(payload.PromptKind.Value), true
	}
	if payload.Template.Present {
		switch ResponseTemplateName(payload.Template.Value) {
		case TemplateContextFallbackAvailabilityDate:
			return ActivePromptAvailabilityDateChoice, true
		case TemplateAvailabilityList,
			TemplateAvailabilityEarliest,
			TemplateContextFallbackAvailabilityOption:
			return ActivePromptAvailabilityOptionChoice, true
		}
	}
	if payload.Intent.Present && Intent(payload.Intent.Value) == IntentSelectAvailabilityOption {
		return ActivePromptAvailabilityOptionChoice, true
	}
	return ActivePromptUnknown, false
}

func legacyAvailabilityPromptBodyV1(body string) (ActivePromptKind, int, bool) {
	if body == "" || body != strings.TrimSpace(body) {
		return ActivePromptUnknown, 0, false
	}
	if strings.HasPrefix(body, availabilityListReplyHeader) &&
		strings.HasSuffix(body, availabilityListReplyQuestion) {
		options := strings.TrimSuffix(strings.TrimPrefix(body, availabilityListReplyHeader), availabilityListReplyQuestion)
		lines := strings.Split(options, "\n")
		if len(lines) > 0 && len(lines) <= availabilityPromptMaxOptionsV1 {
			valid := true
			for offset, line := range lines {
				if !legacyAvailabilityListOptionLineV1(line, offset+1) {
					valid = false
					break
				}
			}
			if valid {
				return ActivePromptAvailabilityOptionChoice, len(lines), true
			}
		}
	}
	if strings.HasPrefix(body, earliestAvailabilityReplyPrefix) &&
		strings.HasSuffix(body, earliestAvailabilityReplyQuestion) {
		details := strings.TrimSuffix(strings.TrimPrefix(body, earliestAvailabilityReplyPrefix), earliestAvailabilityReplyQuestion)
		if legacyEarliestAvailabilityDetailsV1(details) {
			return ActivePromptAvailabilityOptionChoice, 1, true
		}
	}
	return ActivePromptUnknown, 0, false
}

func legacyAvailabilityListOptionLineV1(line string, expectedIndex int) bool {
	prefix := string(rune('0'+expectedIndex)) + ". "
	if !strings.HasPrefix(line, prefix) || line != strings.TrimSpace(line) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(line, prefix), ", ")
	if len(parts) == 0 || len(parts) > 4 || !legacyAvailabilityRouteV1(parts[0]) {
		return false
	}
	position := 1
	if position < len(parts) && availabilityPromptTripDateValidV1(parts[position]) {
		position++
	}
	if position < len(parts) && strings.HasPrefix(parts[position], "saida ") {
		if !legacyAvailabilityClockV1(strings.TrimPrefix(parts[position], "saida ")) {
			return false
		}
		position++
	}
	if position < len(parts) {
		if !legacyAvailabilityPriceV1(parts[position]) {
			return false
		}
		position++
	}
	return position == len(parts)
}

func legacyEarliestAvailabilityDetailsV1(details string) bool {
	parts := strings.Split(details, ", ")
	if len(parts) == 0 || len(parts) > 4 || !legacyAvailabilityDisplayDateV1(parts[0]) {
		return false
	}
	position := 1
	if position < len(parts) && strings.HasPrefix(parts[position], "com saída às ") {
		if !legacyAvailabilityClockV1(strings.TrimPrefix(parts[position], "com saída às ")) {
			return false
		}
		position++
	}
	if position < len(parts) && strings.HasPrefix(parts[position], "de ") {
		if !legacyAvailabilityRouteV1(strings.TrimPrefix(parts[position], "de ")) {
			return false
		}
		position++
	}
	if position < len(parts) {
		if !strings.HasPrefix(parts[position], "por ") ||
			!legacyAvailabilityPriceV1(strings.TrimPrefix(parts[position], "por ")) {
			return false
		}
		position++
	}
	return position == len(parts)
}

func legacyAvailabilityRouteV1(route string) bool {
	if route == "" || route != strings.TrimSpace(route) {
		return false
	}
	separator := strings.Index(route, " para ")
	return separator > 0 && separator+len(" para ") < len(route) &&
		strings.TrimSpace(route[:separator]) == route[:separator] &&
		strings.TrimSpace(route[separator+len(" para "):]) == route[separator+len(" para "):]
}

func legacyAvailabilityDisplayDateV1(value string) bool {
	parsed, err := time.Parse("02/01/2006", value)
	return err == nil && parsed.Format("02/01/2006") == value
}

func legacyAvailabilityClockV1(value string) bool {
	parsed, err := time.Parse("15:04", value)
	return err == nil && parsed.Format("15:04") == value
}

func legacyAvailabilityPriceV1(value string) bool {
	if !strings.HasPrefix(value, "R$ ") {
		return false
	}
	number := strings.TrimPrefix(value, "R$ ")
	if number == "" {
		return false
	}
	decimal := strings.IndexByte(number, '.')
	integer := number
	fraction := ""
	if decimal >= 0 {
		if strings.Count(number, ".") != 1 {
			return false
		}
		integer = number[:decimal]
		fraction = number[decimal+1:]
		if len(fraction) != 2 || !legacyAvailabilityDigitsV1(fraction) {
			return false
		}
	}
	if integer == "" || !legacyAvailabilityDigitsV1(integer) ||
		(len(integer) > 1 && integer[0] == '0') {
		return false
	}
	return strings.Trim(number, "0.") != ""
}

func legacyAvailabilityDigitsV1(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func legacyAvailabilityPromptFactsV1(message Message) bool {
	authority := classifyAvailabilityPromptCandidateV1(message)
	return authority.Class == availabilityPromptAuthorityAbsentLegacyV1 &&
		authority.Candidate && authority.Facts != nil
}

func legacyAvailabilitySelectionProjectionFactsV1(message Message) bool {
	reconciled := reconcileAvailabilityPromptMessageV1(message)
	if reconciled.Selection == nil {
		return false
	}
	if reconciled.Facts != nil {
		return true
	}
	selected := reconciled.Selection.SelectedResult
	return reconciled.Selection.SelectedResultPresent &&
		strings.TrimSpace(asString(selected[selectedAvailabilitySelectionMessageIDPayloadKey])) != "" &&
		strings.TrimSpace(asString(selected[availabilityPromptSourceMessageIDPayloadKey])) != ""
}

func deliveredInvalidAvailabilityPromptBarrierV1(message Message) bool {
	if !messageMayCarryAvailabilityPromptV1(message) {
		return false
	}
	authority := classifyAvailabilityPromptCandidateV1(message)
	if authority.Class == availabilityPromptAuthorityInvalidV1 {
		return true
	}
	// A legacy projection that claims a selected option but carries neither a
	// complete snapshot nor availability facts is a pre-event fail-closed
	// boundary. It is not reclassified as a structural INVALID because both
	// event copies are genuinely absent, but it must still prevent an older
	// list from resolving a later raw index.
	if authority.Class != availabilityPromptAuthorityAbsentLegacyV1 {
		return false
	}
	for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
		claimsSelection := strings.EqualFold(
			payloadMetadataString(payload, "intent"),
			string(IntentSelectAvailabilityOption),
		) || payloadSelectedOptionIndex(payload) > 0
		if claimsSelection &&
			len(selectedAvailabilityResultFromPayload(payload)) == 0 &&
			availabilityPromptRawContextCountV1(message) == 0 {
			return true
		}
	}
	return false
}

func availabilityPromptRawContextCountV1(message Message) int {
	authority := classifyAvailabilityPromptCandidateV1(message)
	if authority.Facts == nil {
		return 0
	}
	return len(authority.Facts.Results)
}

func deliveredInvalidAvailabilityPromptBarrierAtV1(history []Message, index int) bool {
	if index < 0 || index >= len(history) {
		return false
	}
	message := history[index]
	if isDeliveredPromptProjectionMessageV1(message) {
		if classifyAvailabilityPromptCandidateV1(message).Class == availabilityPromptAuthorityUndeliveredV1 {
			return false
		}
		if resolved, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, index, message); ok {
			return deliveredInvalidAvailabilityPromptBarrierV1(resolved)
		}
		if messageMayCarryAvailabilityPromptV1(message) {
			return true
		}
	}
	return deliveredInvalidAvailabilityPromptBarrierV1(message)
}

func latestDeliveredInvalidAvailabilityPromptIndexV1(history []Message, beforeIndex int) int {
	if beforeIndex > len(history) {
		beforeIndex = len(history)
	}
	for index := beforeIndex - 1; index >= 0; index-- {
		if deliveredInvalidAvailabilityPromptBarrierAtV1(history, index) {
			return index
		}
	}
	return -1
}

func NormalizeDeliveryStatusV1(status string) string {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	switch normalized {
	case "SERVER_ACK", "SENT", messageStatusAutomationSent, "DELIVERY_ACK", "DELIVERED", "READ":
		return normalized
	default:
		return ""
	}
}

func DeliveryStatusRankV1(status string) (int, bool) {
	switch NormalizeDeliveryStatusV1(status) {
	case "SERVER_ACK":
		return 1, true
	case "SENT", messageStatusAutomationSent:
		return 2, true
	case "DELIVERY_ACK":
		return 3, true
	case "DELIVERED":
		return 4, true
	case "READ":
		return 5, true
	default:
		return 0, false
	}
}

func HighestDeliveryStatusV1(statuses ...string) string {
	highest := ""
	highestRank := 0
	for _, status := range statuses {
		normalized := NormalizeDeliveryStatusV1(status)
		rank, known := DeliveryStatusRankV1(normalized)
		if !known || rank <= highestRank {
			continue
		}
		highest = normalized
		highestRank = rank
	}
	return highest
}

func AdvanceDeliveryStatusV1(current string, candidate string) string {
	currentNormalized := NormalizeDeliveryStatusV1(current)
	candidateNormalized := NormalizeDeliveryStatusV1(candidate)
	currentRank, currentKnown := DeliveryStatusRankV1(currentNormalized)
	candidateRank, candidateKnown := DeliveryStatusRankV1(candidateNormalized)
	switch {
	case candidateKnown && (!currentKnown || candidateRank > currentRank):
		return candidateNormalized
	case currentKnown:
		return currentNormalized
	case candidateKnown:
		return candidateNormalized
	default:
		return strings.ToUpper(strings.TrimSpace(current))
	}
}

func DeliveryStatusConfirmsOutboundV1(status string) bool {
	rank, known := DeliveryStatusRankV1(status)
	return known && rank >= 2
}

func DeliveryStatusRecordsSentAtV1(status string) bool {
	return DeliveryStatusConfirmsOutboundV1(status)
}

func DeliveryStatusRecordsDeliveredAtV1(status string) bool {
	rank, known := DeliveryStatusRankV1(status)
	return known && rank >= 3
}
