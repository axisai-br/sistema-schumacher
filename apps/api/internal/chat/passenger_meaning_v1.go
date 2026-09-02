package chat

import (
	"math"
	"reflect"
	"sort"
	"strings"
)

const passengerClarificationMeaningV1Version = 1

type PassengerMeaningStatusV1 string

const (
	PassengerMeaningStatusKnown       PassengerMeaningStatusV1 = "KNOWN"
	PassengerMeaningStatusUnknown     PassengerMeaningStatusV1 = "UNKNOWN"
	PassengerMeaningStatusConflicting PassengerMeaningStatusV1 = "CONFLICTING"
)

type PassengerChildRelationV1 string

const (
	PassengerChildRelationChild   PassengerChildRelationV1 = "CHILD"
	PassengerChildRelationOther   PassengerChildRelationV1 = "OTHER"
	PassengerChildRelationUnknown PassengerChildRelationV1 = "UNKNOWN"
)

type PassengerChildAgeUnitV1 string

const (
	PassengerChildAgeUnitYears   PassengerChildAgeUnitV1 = "YEARS"
	PassengerChildAgeUnitMonths  PassengerChildAgeUnitV1 = "MONTHS"
	PassengerChildAgeUnitUnknown PassengerChildAgeUnitV1 = "UNKNOWN"
)

type PassengerMeaningCorrectionTargetV1 string

const (
	PassengerMeaningCorrectionNone               PassengerMeaningCorrectionTargetV1 = "NONE"
	PassengerMeaningCorrectionPassengerAggregate PassengerMeaningCorrectionTargetV1 = "PASSENGER_AGGREGATE"
	PassengerMeaningCorrectionChildAggregate     PassengerMeaningCorrectionTargetV1 = "CHILD_AGGREGATE"
	PassengerMeaningCorrectionFullAggregate      PassengerMeaningCorrectionTargetV1 = "FULL_AGGREGATE"
)

type PassengerMeaningMissingFieldV1 string

const (
	PassengerMeaningMissingPassengerCount PassengerMeaningMissingFieldV1 = "passenger_count"
	PassengerMeaningMissingChildUnder5    PassengerMeaningMissingFieldV1 = "child_under_5"
	PassengerMeaningMissingChildAges      PassengerMeaningMissingFieldV1 = "child_ages"
	PassengerMeaningMissingCorrection     PassengerMeaningMissingFieldV1 = "correction_target"
)

type PassengerMeaningReasonCodeV1 string

const (
	PassengerMeaningReasonPassengerCountStated       PassengerMeaningReasonCodeV1 = "PASSENGER_COUNT_STATED"
	PassengerMeaningReasonIncludesSpeakerComposition PassengerMeaningReasonCodeV1 = "INCLUDES_SPEAKER_COMPOSITION"
	PassengerMeaningReasonPassengerSubgroupOnly      PassengerMeaningReasonCodeV1 = "PASSENGER_SUBGROUP_ONLY"
	PassengerMeaningReasonChildAgeStated             PassengerMeaningReasonCodeV1 = "CHILD_AGE_STATED"
	PassengerMeaningReasonChildUnder5Stated          PassengerMeaningReasonCodeV1 = "CHILD_UNDER_5_STATED"
	PassengerMeaningReasonChildUnder5NotStated       PassengerMeaningReasonCodeV1 = "CHILD_UNDER_5_NOT_STATED"
	PassengerMeaningReasonCorrectionStated           PassengerMeaningReasonCodeV1 = "CORRECTION_STATED"
	PassengerMeaningReasonAmbiguousReference         PassengerMeaningReasonCodeV1 = "AMBIGUOUS_REFERENCE"
	PassengerMeaningReasonConflictingValues          PassengerMeaningReasonCodeV1 = "CONFLICTING_VALUES"
	PassengerMeaningReasonMissingAge                 PassengerMeaningReasonCodeV1 = "MISSING_AGE"
	PassengerMeaningReasonUnrelatedNumberIgnored     PassengerMeaningReasonCodeV1 = "UNRELATED_NUMBER_IGNORED"
)

type PassengerCountMeaningV1 struct {
	Status     PassengerMeaningStatusV1 `json:"status"`
	Value      *int                     `json:"value"`
	Provenance PassengerCountProvenance `json:"provenance"`
}

type PassengerChildReferenceMeaningV1 struct {
	ReferenceID string                   `json:"reference_id"`
	Relation    PassengerChildRelationV1 `json:"relation"`
	AgeValue    *int                     `json:"age_value"`
	AgeUnit     PassengerChildAgeUnitV1  `json:"age_unit"`
	Under5      *bool                    `json:"under_5"`
}

type PassengerChildUnder5MeaningV1 struct {
	Status     PassengerMeaningStatusV1           `json:"status"`
	Count      *int                               `json:"count"`
	References []PassengerChildReferenceMeaningV1 `json:"references"`
}

type PassengerMeaningCorrectionV1 struct {
	Present  bool                               `json:"present"`
	Replaces PassengerMeaningCorrectionTargetV1 `json:"replaces"`
}

// PassengerClarificationMeaningV1 is a semantic proposal only. It deliberately
// contains no runtime event, action, tool, booking, payment, document, or price.
type PassengerClarificationMeaningV1 struct {
	Version             int                              `json:"version"`
	SourceMessageID     string                           `json:"source_message_id"`
	SourcePromptEventID string                           `json:"source_prompt_event_id"`
	PassengerCount      PassengerCountMeaningV1          `json:"passenger_count"`
	ChildUnder5         PassengerChildUnder5MeaningV1    `json:"child_under_5"`
	Correction          PassengerMeaningCorrectionV1     `json:"correction"`
	NeedsClarification  bool                             `json:"needs_clarification"`
	MissingFields       []PassengerMeaningMissingFieldV1 `json:"missing_fields"`
	Confidence          float64                          `json:"confidence"`
	ReasonCodes         []PassengerMeaningReasonCodeV1   `json:"reason_codes"`
}

type PassengerClarificationMeaningV1ValidationInput struct {
	Proposal                    PassengerClarificationMeaningV1
	State                       PassengerClarificationStateV1
	PromptEvent                 PassengerClarificationEventV1
	ExpectedSourceMessageID     string
	ExpectedSourcePromptEventID string
}

type PassengerClarificationMeaningV1ValidationResult struct {
	ReasonCodes []string
}

func (r PassengerClarificationMeaningV1ValidationResult) Accepted() bool {
	return len(r.ReasonCodes) == 0
}

// ValidatePassengerClarificationMeaningV1 validates only structured contract,
// source epoch, state, and invariants. Current-turn text is intentionally not
// part of the input, so this function cannot reinterpret provider language.
func ValidatePassengerClarificationMeaningV1(input PassengerClarificationMeaningV1ValidationInput) PassengerClarificationMeaningV1ValidationResult {
	reasons := make([]string, 0, 16)
	proposal := input.Proposal
	state := input.State
	prompt := input.PromptEvent

	if proposal.Version != passengerClarificationMeaningV1Version {
		reasons = append(reasons, "invalid_version")
	}
	expectedMessageID := strings.TrimSpace(input.ExpectedSourceMessageID)
	expectedPromptEventID := strings.TrimSpace(input.ExpectedSourcePromptEventID)
	if expectedMessageID == "" || strings.TrimSpace(proposal.SourceMessageID) != expectedMessageID {
		reasons = append(reasons, "source_message_mismatch")
	}
	if expectedPromptEventID == "" || strings.TrimSpace(proposal.SourcePromptEventID) != expectedPromptEventID {
		reasons = append(reasons, "source_prompt_event_mismatch")
	}
	if validatePassengerClarificationStateV1(state) != nil ||
		state.Authority != PassengerClarificationAuthorityPreBooking ||
		!state.HasEvidence || passengerClarificationStateConflictingV1(state) {
		reasons = append(reasons, "ineligible_passenger_state")
	}
	if !passengerClarificationEventIsPromptV1(prompt) || !passengerClarificationEventValidV1(prompt) ||
		strings.TrimSpace(prompt.EventID) == "" || strings.TrimSpace(prompt.EventID) != passengerClarificationEventIDV1(prompt) ||
		strings.TrimSpace(prompt.EventID) != expectedPromptEventID {
		reasons = append(reasons, "invalid_prompt_epoch")
	} else if !passengerMeaningV1PromptMatchesState(state, prompt) {
		reasons = append(reasons, "stale_prompt_epoch")
	}

	reasons = append(reasons, validatePassengerCountMeaningV1(proposal.PassengerCount)...)
	reasons = append(reasons, validatePassengerChildMeaningV1(proposal.ChildUnder5)...)
	if proposal.PassengerCount.Status == PassengerMeaningStatusKnown && proposal.PassengerCount.Value != nil &&
		proposal.ChildUnder5.Status == PassengerMeaningStatusKnown && proposal.ChildUnder5.Count != nil &&
		*proposal.ChildUnder5.Count > *proposal.PassengerCount.Value {
		reasons = append(reasons, "children_exceed_passenger_total")
	}
	if proposal.PassengerCount.Status == PassengerMeaningStatusKnown &&
		proposal.PassengerCount.Provenance == PassengerCountProvenanceAbsoluteTotal &&
		proposal.PassengerCount.Value != nil &&
		passengerMeaningV1DistinctChildReferenceCount(proposal.ChildUnder5.References) > *proposal.PassengerCount.Value {
		reasons = append(reasons, "children_exceed_passenger_total")
	}

	if proposal.Correction.Present == (proposal.Correction.Replaces == PassengerMeaningCorrectionNone) {
		reasons = append(reasons, "invalid_correction_shape")
	}
	if !passengerMeaningCorrectionTargetAllowedV1(proposal.Correction.Replaces) {
		reasons = append(reasons, "invalid_correction_target")
	}
	reasons = append(reasons, validatePassengerMeaningV1AgainstSnapshot(proposal, state)...)
	if proposal.Correction.Present {
		switch proposal.Correction.Replaces {
		case PassengerMeaningCorrectionPassengerAggregate:
			if proposal.PassengerCount.Status == PassengerMeaningStatusUnknown {
				reasons = append(reasons, "incomplete_passenger_correction")
			}
		case PassengerMeaningCorrectionChildAggregate:
			if proposal.ChildUnder5.Status == PassengerMeaningStatusUnknown {
				reasons = append(reasons, "incomplete_child_correction")
			}
		case PassengerMeaningCorrectionFullAggregate:
			if proposal.PassengerCount.Status == PassengerMeaningStatusUnknown || proposal.ChildUnder5.Status == PassengerMeaningStatusUnknown {
				reasons = append(reasons, "incomplete_full_correction")
			}
		}
		if !passengerMeaningV1ReasonCodeContains(proposal.ReasonCodes, PassengerMeaningReasonCorrectionStated) {
			reasons = append(reasons, "missing_correction_reason")
		}
	} else if passengerMeaningV1ReasonCodeContains(proposal.ReasonCodes, PassengerMeaningReasonCorrectionStated) {
		reasons = append(reasons, "unexpected_correction_reason")
	}

	requiredMissing := passengerMeaningV1RequiredMissingFields(proposal)
	if proposal.NeedsClarification != (len(requiredMissing) > 0) ||
		!samePassengerMeaningMissingFieldsV1(proposal.MissingFields, requiredMissing) {
		reasons = append(reasons, "incoherent_clarification_fields")
	}
	if math.IsNaN(proposal.Confidence) || math.IsInf(proposal.Confidence, 0) || proposal.Confidence < 0 || proposal.Confidence > 1 {
		reasons = append(reasons, "confidence_out_of_range")
	}
	if len(proposal.ReasonCodes) == 0 || len(proposal.ReasonCodes) > 16 || !passengerMeaningReasonCodesValidV1(proposal.ReasonCodes) {
		reasons = append(reasons, "invalid_reason_codes")
	}

	return PassengerClarificationMeaningV1ValidationResult{ReasonCodes: dedupePassengerMeaningV1Strings(reasons)}
}

func validatePassengerMeaningV1AgainstSnapshot(proposal PassengerClarificationMeaningV1, state PassengerClarificationStateV1) []string {
	reasons := make([]string, 0, 4)
	coverage := passengerMeaningV1CorrectionCoverage(proposal.Correction)
	if state.PassengerCountKnown &&
		(proposal.PassengerCount.Status != PassengerMeaningStatusKnown ||
			proposal.PassengerCount.Value == nil ||
			*proposal.PassengerCount.Value != state.PassengerCount ||
			proposal.PassengerCount.Provenance != state.PassengerCountProvenance) &&
		!coverage.Passenger {
		reasons = append(reasons, "passenger_count_conflicts_with_snapshot")
	}
	if state.ChildUnder5CountKnown &&
		(proposal.ChildUnder5.Status != PassengerMeaningStatusKnown ||
			proposal.ChildUnder5.Count == nil || *proposal.ChildUnder5.Count != state.ChildUnder5Count) &&
		!coverage.Child {
		reasons = append(reasons, "child_count_conflicts_with_snapshot")
	}
	if len(state.ChildReferences) > 0 && !coverage.Child &&
		!passengerMeaningV1ChildReferencesMatchSnapshot(state.ChildReferences, proposal.ChildUnder5.References) {
		reasons = append(reasons, "child_references_conflict_with_snapshot")
	}
	if state.ChildUnder5AddsTraveler && coverage.Passenger && !coverage.Child &&
		!passengerMeaningV1PreservesSoloSpeaker(proposal.PassengerCount) {
		reasons = append(reasons, "passenger_correction_conflicts_with_child_dependency")
	}
	return reasons
}

type passengerMeaningV1CorrectionCoverageV1 struct {
	Passenger bool
	Child     bool
}

var passengerMeaningV1CorrectionCoverageMatrix = map[PassengerMeaningCorrectionTargetV1]passengerMeaningV1CorrectionCoverageV1{
	PassengerMeaningCorrectionNone:               {},
	PassengerMeaningCorrectionPassengerAggregate: {Passenger: true},
	PassengerMeaningCorrectionChildAggregate:     {Child: true},
	PassengerMeaningCorrectionFullAggregate:      {Passenger: true, Child: true},
}

func passengerMeaningV1CorrectionCoverage(correction PassengerMeaningCorrectionV1) passengerMeaningV1CorrectionCoverageV1 {
	if !correction.Present {
		return passengerMeaningV1CorrectionCoverageV1{}
	}
	return passengerMeaningV1CorrectionCoverageMatrix[correction.Replaces]
}

func passengerMeaningV1ChildReferencesMatchSnapshot(snapshot []PassengerClarificationChildReferenceV1, proposal []PassengerChildReferenceMeaningV1) bool {
	proposalsByID := make(map[string][]PassengerChildReferenceMeaningV1, len(proposal))
	for _, reference := range proposal {
		id := strings.TrimSpace(reference.ReferenceID)
		proposalsByID[id] = append(proposalsByID[id], reference)
	}
	seenSnapshot := make(map[string]struct{}, len(snapshot))
	for _, existing := range snapshot {
		id := strings.TrimSpace(existing.ID)
		if _, duplicate := seenSnapshot[id]; duplicate {
			return false
		}
		seenSnapshot[id] = struct{}{}
		matches := proposalsByID[id]
		if len(matches) != 1 || matches[0].Relation != PassengerChildRelationChild ||
			matches[0].Under5 == nil || *matches[0].Under5 != existing.Under5 {
			return false
		}
	}
	return true
}

func passengerMeaningV1DistinctChildReferenceCount(references []PassengerChildReferenceMeaningV1) int {
	distinct := make(map[string]struct{}, len(references))
	for _, reference := range references {
		id := strings.TrimSpace(reference.ReferenceID)
		if reference.Relation != PassengerChildRelationChild || !passengerMeaningOpaqueReferenceIDValidV1(id) {
			continue
		}
		distinct[id] = struct{}{}
	}
	return len(distinct)
}

func passengerMeaningV1PreservesSoloSpeaker(passenger PassengerCountMeaningV1) bool {
	return passenger.Status == PassengerMeaningStatusKnown &&
		passenger.Provenance == PassengerCountProvenanceSoloSpeaker &&
		passenger.Value != nil && *passenger.Value == 1
}

func passengerMeaningV1PromptMatchesState(state PassengerClarificationStateV1, prompt PassengerClarificationEventV1) bool {
	switch prompt.Type {
	case PassengerClarificationEventPassengerPromptOpened:
		return prompt.Slot == PassengerClarificationSlotPassenger &&
			state.PassengerSlotStatus == PassengerClarificationSlotOpen &&
			strings.TrimSpace(state.PassengerPromptMessageID) == strings.TrimSpace(prompt.MessageID)
	case PassengerClarificationEventChildPromptOpened:
		return prompt.Slot == PassengerClarificationSlotChild &&
			state.ChildSlotStatus == PassengerClarificationSlotOpen &&
			strings.TrimSpace(state.ChildPromptMessageID) == strings.TrimSpace(prompt.MessageID)
	default:
		return false
	}
}

type passengerCountMeaningRuleV1 struct {
	Status        PassengerMeaningStatusV1
	Provenance    PassengerCountProvenance
	ValueRequired bool
	Minimum       int
	Maximum       int
}

var passengerCountMeaningRulesV1 = []passengerCountMeaningRuleV1{
	{Status: PassengerMeaningStatusKnown, Provenance: PassengerCountProvenanceSoloSpeaker, ValueRequired: true, Minimum: 1, Maximum: 1},
	{Status: PassengerMeaningStatusKnown, Provenance: PassengerCountProvenanceAbsoluteTotal, ValueRequired: true, Minimum: 1, Maximum: 99},
	{Status: PassengerMeaningStatusKnown, Provenance: PassengerCountProvenanceIncludesSpeakerComposition, ValueRequired: true, Minimum: 2, Maximum: 99},
	{Status: PassengerMeaningStatusUnknown, Provenance: PassengerCountProvenanceUnknown},
	{Status: PassengerMeaningStatusUnknown, Provenance: PassengerCountProvenanceSubgroupOnly},
	{Status: PassengerMeaningStatusConflicting, Provenance: PassengerCountProvenanceUnknown},
}

func validatePassengerCountMeaningV1(value PassengerCountMeaningV1) []string {
	if !passengerMeaningStatusAllowedV1(value.Status) {
		return []string{"invalid_passenger_status"}
	}
	rule, ok := passengerCountMeaningRule(value.Status, value.Provenance)
	if !ok {
		return []string{passengerCountMeaningRuleViolation(value.Status)}
	}
	if rule.ValueRequired {
		if value.Value == nil || *value.Value < rule.Minimum || *value.Value > rule.Maximum {
			return []string{"invalid_passenger_count"}
		}
		return nil
	}
	if value.Value != nil {
		return []string{passengerCountMeaningRuleViolation(value.Status)}
	}
	return nil
}

func passengerCountMeaningRule(status PassengerMeaningStatusV1, provenance PassengerCountProvenance) (passengerCountMeaningRuleV1, bool) {
	for _, rule := range passengerCountMeaningRulesV1 {
		if rule.Status == status && rule.Provenance == provenance {
			return rule, true
		}
	}
	return passengerCountMeaningRuleV1{}, false
}

func passengerCountMeaningRuleViolation(status PassengerMeaningStatusV1) string {
	switch status {
	case PassengerMeaningStatusKnown:
		return "invalid_passenger_provenance"
	case PassengerMeaningStatusUnknown:
		return "invalid_unknown_passenger_count"
	default:
		return "invalid_conflicting_passenger_count"
	}
}

func validatePassengerChildMeaningV1(value PassengerChildUnder5MeaningV1) []string {
	reasons := make([]string, 0, 8)
	if !passengerMeaningStatusAllowedV1(value.Status) {
		reasons = append(reasons, "invalid_child_status")
	}
	switch value.Status {
	case PassengerMeaningStatusKnown:
		if value.Count == nil || *value.Count < 0 || *value.Count > 99 {
			reasons = append(reasons, "invalid_child_count")
		}
	case PassengerMeaningStatusUnknown, PassengerMeaningStatusConflicting:
		if value.Count != nil {
			reasons = append(reasons, "invalid_unknown_child_count")
		}
	}
	if len(value.References) > 16 {
		reasons = append(reasons, "too_many_child_references")
	}
	seen := map[string]struct{}{}
	knownUnder5References := 0
	knownUnder5Classifications := 0
	for _, reference := range value.References {
		id := strings.TrimSpace(reference.ReferenceID)
		if !passengerMeaningOpaqueReferenceIDValidV1(id) {
			reasons = append(reasons, "invalid_child_reference_id")
		} else if _, ok := seen[id]; ok {
			reasons = append(reasons, "duplicate_child_reference_id")
		} else {
			seen[id] = struct{}{}
		}
		if !passengerChildRelationAllowedV1(reference.Relation) {
			reasons = append(reasons, "invalid_child_relation")
		}
		if !passengerChildAgeUnitAllowedV1(reference.AgeUnit) {
			reasons = append(reasons, "invalid_child_age_unit")
			continue
		}
		if reference.AgeUnit == PassengerChildAgeUnitUnknown {
			if reference.AgeValue != nil {
				reasons = append(reasons, "invalid_unknown_child_age")
			}
		} else {
			if reference.AgeValue == nil || !passengerChildAgeInRangeV1(reference.AgeUnit, reference.AgeValue) {
				reasons = append(reasons, "invalid_child_age")
			} else if reference.Under5 == nil || *reference.Under5 != passengerChildAgeUnder5V1(reference.AgeUnit, *reference.AgeValue) {
				reasons = append(reasons, "incoherent_child_age")
			}
		}
		if reference.Under5 != nil && *reference.Under5 {
			knownUnder5References++
		}
		if reference.Under5 != nil {
			knownUnder5Classifications++
		}
	}
	if value.Status == PassengerMeaningStatusKnown && value.Count != nil && knownUnder5References > *value.Count {
		reasons = append(reasons, "child_references_exceed_count")
	}
	if value.Status == PassengerMeaningStatusKnown && value.Count != nil && len(value.References) > 0 &&
		knownUnder5Classifications == len(value.References) && knownUnder5References != *value.Count {
		reasons = append(reasons, "incoherent_child_reference_count")
	}
	if value.Status != PassengerMeaningStatusKnown && knownUnder5Classifications > 0 {
		reasons = append(reasons, "known_child_reference_with_unknown_aggregate")
	}
	return reasons
}

func passengerMeaningV1RequiredMissingFields(proposal PassengerClarificationMeaningV1) []PassengerMeaningMissingFieldV1 {
	fields := make([]PassengerMeaningMissingFieldV1, 0, 2)
	if proposal.PassengerCount.Status != PassengerMeaningStatusKnown {
		fields = append(fields, PassengerMeaningMissingPassengerCount)
	}
	if proposal.ChildUnder5.Status != PassengerMeaningStatusKnown {
		fields = append(fields, PassengerMeaningMissingChildUnder5)
	}
	if passengerMeaningV1ReasonCodeContains(proposal.ReasonCodes, PassengerMeaningReasonMissingAge) {
		fields = append(fields, PassengerMeaningMissingChildAges)
	}
	return fields
}

func samePassengerMeaningMissingFieldsV1(actual []PassengerMeaningMissingFieldV1, expected []PassengerMeaningMissingFieldV1) bool {
	if len(actual) != len(expected) {
		return false
	}
	a := append([]PassengerMeaningMissingFieldV1(nil), actual...)
	e := append([]PassengerMeaningMissingFieldV1(nil), expected...)
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(e, func(i, j int) bool { return e[i] < e[j] })
	for index := range a {
		if a[index] != e[index] || !passengerMeaningMissingFieldAllowedV1(a[index]) {
			return false
		}
	}
	return true
}

func passengerMeaningStatusAllowedV1(value PassengerMeaningStatusV1) bool {
	return value == PassengerMeaningStatusKnown || value == PassengerMeaningStatusUnknown || value == PassengerMeaningStatusConflicting
}

func passengerMeaningCorrectionTargetAllowedV1(value PassengerMeaningCorrectionTargetV1) bool {
	return value == PassengerMeaningCorrectionNone || value == PassengerMeaningCorrectionPassengerAggregate ||
		value == PassengerMeaningCorrectionChildAggregate || value == PassengerMeaningCorrectionFullAggregate
}

func passengerMeaningMissingFieldAllowedV1(value PassengerMeaningMissingFieldV1) bool {
	return value == PassengerMeaningMissingPassengerCount || value == PassengerMeaningMissingChildUnder5 ||
		value == PassengerMeaningMissingChildAges || value == PassengerMeaningMissingCorrection
}

func passengerMeaningReasonCodesValidV1(values []PassengerMeaningReasonCodeV1) bool {
	seen := map[PassengerMeaningReasonCodeV1]struct{}{}
	for _, value := range values {
		if !passengerMeaningReasonCodeAllowedV1(value) {
			return false
		}
		if _, ok := seen[value]; ok {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func passengerMeaningV1ReasonCodeContains(values []PassengerMeaningReasonCodeV1, expected PassengerMeaningReasonCodeV1) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func passengerMeaningReasonCodeAllowedV1(value PassengerMeaningReasonCodeV1) bool {
	switch value {
	case PassengerMeaningReasonPassengerCountStated,
		PassengerMeaningReasonIncludesSpeakerComposition,
		PassengerMeaningReasonPassengerSubgroupOnly,
		PassengerMeaningReasonChildAgeStated,
		PassengerMeaningReasonChildUnder5Stated,
		PassengerMeaningReasonChildUnder5NotStated,
		PassengerMeaningReasonCorrectionStated,
		PassengerMeaningReasonAmbiguousReference,
		PassengerMeaningReasonConflictingValues,
		PassengerMeaningReasonMissingAge,
		PassengerMeaningReasonUnrelatedNumberIgnored:
		return true
	default:
		return false
	}
}

func passengerChildRelationAllowedV1(value PassengerChildRelationV1) bool {
	return value == PassengerChildRelationChild || value == PassengerChildRelationOther || value == PassengerChildRelationUnknown
}

func passengerChildAgeUnitAllowedV1(value PassengerChildAgeUnitV1) bool {
	return value == PassengerChildAgeUnitYears || value == PassengerChildAgeUnitMonths || value == PassengerChildAgeUnitUnknown
}

func passengerMeaningOpaqueReferenceIDValidV1(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func passengerChildAgeInRangeV1(unit PassengerChildAgeUnitV1, value *int) bool {
	if value == nil || *value < 0 {
		return false
	}
	if unit == PassengerChildAgeUnitYears {
		return *value <= 120
	}
	if unit == PassengerChildAgeUnitMonths {
		return *value <= 1440
	}
	return false
}

func passengerChildAgeUnder5V1(unit PassengerChildAgeUnitV1, value int) bool {
	if unit == PassengerChildAgeUnitMonths {
		return value < 60
	}
	return value < 5
}

func dedupePassengerMeaningV1Strings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type PassengerClarificationLocalProposalV1 struct {
	Version             int
	SourceMessageID     string
	SourcePromptEventID string
	PassengerCount      PassengerCountMeaningV1
	ChildUnder5         PassengerChildUnder5MeaningV1
	Correction          PassengerMeaningCorrectionV1
	NeedsClarification  bool
	MissingFields       []PassengerMeaningMissingFieldV1
	Confidence          float64
	ReasonCodes         []PassengerMeaningReasonCodeV1
}

// MapPassengerClarificationMeaningV1 creates a local proposal only. The return
// type cannot be applied by the B1 reducer and carries no runtime event.
func MapPassengerClarificationMeaningV1(meaning PassengerClarificationMeaningV1) PassengerClarificationLocalProposalV1 {
	return PassengerClarificationLocalProposalV1{
		Version:             meaning.Version,
		SourceMessageID:     strings.TrimSpace(meaning.SourceMessageID),
		SourcePromptEventID: strings.TrimSpace(meaning.SourcePromptEventID),
		PassengerCount:      clonePassengerCountMeaningV1(meaning.PassengerCount),
		ChildUnder5:         clonePassengerChildUnder5MeaningV1(meaning.ChildUnder5),
		Correction:          meaning.Correction,
		NeedsClarification:  meaning.NeedsClarification,
		MissingFields:       append([]PassengerMeaningMissingFieldV1(nil), meaning.MissingFields...),
		Confidence:          meaning.Confidence,
		ReasonCodes:         append([]PassengerMeaningReasonCodeV1(nil), meaning.ReasonCodes...),
	}
}

func clonePassengerCountMeaningV1(value PassengerCountMeaningV1) PassengerCountMeaningV1 {
	cloned := value
	if value.Value != nil {
		copyValue := *value.Value
		cloned.Value = &copyValue
	}
	return cloned
}

func clonePassengerChildUnder5MeaningV1(value PassengerChildUnder5MeaningV1) PassengerChildUnder5MeaningV1 {
	cloned := value
	if value.Count != nil {
		copyValue := *value.Count
		cloned.Count = &copyValue
	}
	cloned.References = make([]PassengerChildReferenceMeaningV1, 0, len(value.References))
	for _, reference := range value.References {
		copyReference := reference
		if reference.AgeValue != nil {
			copyValue := *reference.AgeValue
			copyReference.AgeValue = &copyValue
		}
		if reference.Under5 != nil {
			copyValue := *reference.Under5
			copyReference.Under5 = &copyValue
		}
		cloned.References = append(cloned.References, copyReference)
	}
	return cloned
}

type PassengerMeaningV1Evaluation struct {
	Matched        bool
	MismatchFields []string
}

// EvaluatePassengerClarificationMeaningV1 compares structured meaning only;
// it neither generates a proposal nor receives provider input.
func EvaluatePassengerClarificationMeaningV1(actual PassengerClarificationMeaningV1, expected PassengerClarificationMeaningV1) PassengerMeaningV1Evaluation {
	mismatches := make([]string, 0, 10)
	for _, comparison := range []struct {
		name  string
		match bool
	}{
		{"version", actual.Version == expected.Version},
		{"source_message_id", actual.SourceMessageID == expected.SourceMessageID},
		{"source_prompt_event_id", actual.SourcePromptEventID == expected.SourcePromptEventID},
		{"passenger_count", reflect.DeepEqual(actual.PassengerCount, expected.PassengerCount)},
		{"child_under_5", reflect.DeepEqual(actual.ChildUnder5, expected.ChildUnder5)},
		{"correction", reflect.DeepEqual(actual.Correction, expected.Correction)},
		{"needs_clarification", actual.NeedsClarification == expected.NeedsClarification},
		{"missing_fields", reflect.DeepEqual(actual.MissingFields, expected.MissingFields)},
		{"confidence", actual.Confidence == expected.Confidence},
		{"reason_codes", reflect.DeepEqual(actual.ReasonCodes, expected.ReasonCodes)},
	} {
		if !comparison.match {
			mismatches = append(mismatches, comparison.name)
		}
	}
	return PassengerMeaningV1Evaluation{Matched: len(mismatches) == 0, MismatchFields: mismatches}
}
