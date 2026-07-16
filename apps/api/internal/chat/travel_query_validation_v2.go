package chat

import (
	"math"
	"strings"
	"time"
)

var travelQueryCanonicalLocationAccentReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c",
)

const minTravelQueryMeaningV2Confidence = 0.70

// TravelQueryValidationStatus is the closed status returned by the V2
// semantic validator.
type TravelQueryValidationStatus string

const (
	TravelQueryValidationStatusAccepted TravelQueryValidationStatus = "ACCEPTED"
	TravelQueryValidationStatusRejected TravelQueryValidationStatus = "REJECTED"
)

// TravelQueryValidationReasonCode is a closed, local reason emitted when a
// TravelQueryMeaningV2 proposal is rejected.
type TravelQueryValidationReasonCode string

const (
	TravelQueryValidationReasonEmptyCurrentTurn                   TravelQueryValidationReasonCode = "EMPTY_CURRENT_TURN"
	TravelQueryValidationReasonUnknownIntent                      TravelQueryValidationReasonCode = "UNKNOWN_INTENT"
	TravelQueryValidationReasonInvalidEnumValue                   TravelQueryValidationReasonCode = "INVALID_ENUM_VALUE"
	TravelQueryValidationReasonInvalidConfidence                  TravelQueryValidationReasonCode = "INVALID_CONFIDENCE"
	TravelQueryValidationReasonLowConfidenceRequiresClarification TravelQueryValidationReasonCode = "LOW_CONFIDENCE_REQUIRES_CLARIFICATION"
	TravelQueryValidationReasonIncoherentClarification            TravelQueryValidationReasonCode = "INCOHERENT_CLARIFICATION"
	TravelQueryValidationReasonInconsistentSemanticCombination    TravelQueryValidationReasonCode = "INCONSISTENT_SEMANTIC_COMBINATION"
	TravelQueryValidationReasonContradictoryLocationRole          TravelQueryValidationReasonCode = "CONTRADICTORY_LOCATION_ROLE"
	TravelQueryValidationReasonLocationNotInCatalog               TravelQueryValidationReasonCode = "LOCATION_NOT_IN_CATALOG"
	TravelQueryValidationReasonLocationNotGrounded                TravelQueryValidationReasonCode = "LOCATION_NOT_GROUNDED"
	TravelQueryValidationReasonObservedAtRequired                 TravelQueryValidationReasonCode = "OBSERVED_AT_REQUIRED"
	TravelQueryValidationReasonInvalidExactDate                   TravelQueryValidationReasonCode = "INVALID_EXACT_DATE"
	TravelQueryValidationReasonExactDateInPast                    TravelQueryValidationReasonCode = "EXACT_DATE_IN_PAST"
	TravelQueryValidationReasonEarliestAvailableRouteRequired     TravelQueryValidationReasonCode = "EARLIEST_AVAILABLE_ROUTE_REQUIRED"
	TravelQueryValidationReasonCurrentAvailabilityFactsRequired   TravelQueryValidationReasonCode = "CURRENT_AVAILABILITY_FACTS_REQUIRED"
	TravelQueryValidationReasonCurrentAvailabilityFactsInvalid    TravelQueryValidationReasonCode = "CURRENT_AVAILABILITY_FACTS_INVALID"
	TravelQueryValidationReasonInvalidOptionReference             TravelQueryValidationReasonCode = "INVALID_OPTION_REFERENCE"
	TravelQueryValidationReasonOptionReferenceMismatch            TravelQueryValidationReasonCode = "OPTION_REFERENCE_MISMATCH"
	TravelQueryValidationReasonOptionIndexOutOfRange              TravelQueryValidationReasonCode = "OPTION_INDEX_OUT_OF_RANGE"
	TravelQueryValidationReasonOptionDateNotVisible               TravelQueryValidationReasonCode = "OPTION_DATE_NOT_VISIBLE"
	TravelQueryValidationReasonAmbiguousOptionReference           TravelQueryValidationReasonCode = "AMBIGUOUS_OPTION_REFERENCE"
	TravelQueryValidationReasonInvalidRouteCoverage               TravelQueryValidationReasonCode = "INVALID_ROUTE_COVERAGE"
	TravelQueryValidationReasonRouteCoverageEvidenceRequired      TravelQueryValidationReasonCode = "ROUTE_COVERAGE_EVIDENCE_REQUIRED"
	TravelQueryValidationReasonInvalidSeatRequest                 TravelQueryValidationReasonCode = "INVALID_SEAT_REQUEST"
	TravelQueryValidationReasonInvalidInstitutionalTopic          TravelQueryValidationReasonCode = "INVALID_INSTITUTIONAL_TOPIC"
	TravelQueryValidationReasonStrongDecisionProtected            TravelQueryValidationReasonCode = "STRONG_DECISION_PROTECTED"
)

// TravelQueryLocationEvidenceV2 is caller-provided, read-only catalog
// evidence. StopID may be empty for a known nearby reference that is not an
// exact operational stop.
type TravelQueryLocationEvidenceV2 struct {
	CanonicalName string
	StopID        string
}

// TravelQueryAvailabilityFactsV2 contains only the options visibly attached
// to one reliable active prompt. Source identity is checked against both the
// ActivePromptContext and the latest reliable outbound message.
type TravelQueryAvailabilityFactsV2 struct {
	SourceMessageID         string
	SourceMessageReceivedAt time.Time
	VisibleOptions          []AvailabilitySearchItem
}

type travelQueryAvailabilityFactsState uint8

const (
	travelQueryAvailabilityFactsNotApplicable travelQueryAvailabilityFactsState = iota
	travelQueryAvailabilityFactsValid
	travelQueryAvailabilityFactsInvalid
)

type travelQueryAvailabilityFactsEvaluation struct {
	state   travelQueryAvailabilityFactsState
	options []AvailabilitySearchItem
}

type travelQueryFactualRoute struct {
	origin      TravelQueryLocationEvidenceV2
	destination TravelQueryLocationEvidenceV2
}

type travelQueryValidationContext struct {
	availabilityFacts travelQueryAvailabilityFactsEvaluation
	currentRoute      travelQueryFactualRoute
	hasCurrentRoute   bool
}

// TravelQueryValidationInputV2 contains semantic input and factual evidence.
// The validator reads this value but never mutates it.
type TravelQueryValidationInputV2 struct {
	Proposal                 TravelQueryMeaningV2
	CurrentTurn              string
	State                    CanonicalConversationState
	ActivePrompt             ActivePromptContext
	History                  []Message
	ObservedAt               time.Time
	AvailabilityFacts        TravelQueryAvailabilityFactsV2
	LocationCatalog          []TravelQueryLocationEvidenceV2
	ExistingDecisionStrength DecisionStrength
}

// TravelQueryValidationResultV2 reports semantic acceptance only. It contains
// no operation, tool, template, state mutation, message, booking, selection,
// payment, or handoff authorization.
type TravelQueryValidationResultV2 struct {
	Status      TravelQueryValidationStatus
	ReasonCodes []TravelQueryValidationReasonCode
}

func (result TravelQueryValidationResultV2) Accepted() bool {
	return result.Status == TravelQueryValidationStatusAccepted
}

// ValidateTravelQueryMeaningV2 is a pure, deterministic validator for the V2
// semantic proposal. Acceptance never authorizes an operational side effect.
func ValidateTravelQueryMeaningV2(input TravelQueryValidationInputV2) TravelQueryValidationResultV2 {
	if strings.TrimSpace(input.CurrentTurn) == "" {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonEmptyCurrentTurn)
	}
	if input.ExistingDecisionStrength == DecisionStrengthStrong {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonStrongDecisionProtected)
	}
	context := newTravelQueryValidationContext(input)
	if context.availabilityFacts.state == travelQueryAvailabilityFactsInvalid {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	}
	if !travelQueryDecisionStrengthAllowed(input.ExistingDecisionStrength) {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonInvalidEnumValue)
	}
	if input.Proposal.Intent == "" ||
		(input.Proposal.Intent == TravelQueryIntentUnknown && input.Proposal.TurnMeaning != TurnMeaningAcknowledgement) {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonUnknownIntent)
	}
	if !travelQueryMeaningV2EnumsAllowed(input.Proposal) {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonInvalidEnumValue)
	}
	if reason := validateTravelQueryOptionReferenceShape(input.Proposal.OptionReference); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}
	if math.IsNaN(input.Proposal.Confidence) || math.IsInf(input.Proposal.Confidence, 0) ||
		input.Proposal.Confidence < 0 || input.Proposal.Confidence > 1 {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonInvalidConfidence)
	}
	if !travelQueryClarificationCoherent(input, context) {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonIncoherentClarification)
	}
	if input.Proposal.Confidence < minTravelQueryMeaningV2Confidence && !input.Proposal.NeedsClarification {
		return rejectTravelQueryMeaningV2(TravelQueryValidationReasonLowConfidenceRequiresClarification)
	}
	if reason := validateTravelQuerySemanticCombination(input); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}
	if reason := validateTravelQueryLocations(input, context); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}
	if reason := validateTravelQueryDatePreference(input); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}
	if reason := validateTravelQueryEarliestAvailable(input, context); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}
	if reason := validateTravelQueryOptionReference(input, context); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}
	if reason := validateTravelQueryRouteCoverage(input, context); reason != "" {
		return rejectTravelQueryMeaningV2(reason)
	}

	return TravelQueryValidationResultV2{
		Status:      TravelQueryValidationStatusAccepted,
		ReasonCodes: []TravelQueryValidationReasonCode{},
	}
}

func rejectTravelQueryMeaningV2(reason TravelQueryValidationReasonCode) TravelQueryValidationResultV2 {
	if !travelQueryValidationReasonCodeAllowed(reason) {
		reason = TravelQueryValidationReasonInvalidEnumValue
	}
	return TravelQueryValidationResultV2{
		Status:      TravelQueryValidationStatusRejected,
		ReasonCodes: []TravelQueryValidationReasonCode{reason},
	}
}

func travelQueryValidationReasonCodeAllowed(reason TravelQueryValidationReasonCode) bool {
	switch reason {
	case TravelQueryValidationReasonEmptyCurrentTurn,
		TravelQueryValidationReasonUnknownIntent,
		TravelQueryValidationReasonInvalidEnumValue,
		TravelQueryValidationReasonInvalidConfidence,
		TravelQueryValidationReasonLowConfidenceRequiresClarification,
		TravelQueryValidationReasonIncoherentClarification,
		TravelQueryValidationReasonInconsistentSemanticCombination,
		TravelQueryValidationReasonContradictoryLocationRole,
		TravelQueryValidationReasonLocationNotInCatalog,
		TravelQueryValidationReasonLocationNotGrounded,
		TravelQueryValidationReasonObservedAtRequired,
		TravelQueryValidationReasonInvalidExactDate,
		TravelQueryValidationReasonExactDateInPast,
		TravelQueryValidationReasonEarliestAvailableRouteRequired,
		TravelQueryValidationReasonCurrentAvailabilityFactsRequired,
		TravelQueryValidationReasonCurrentAvailabilityFactsInvalid,
		TravelQueryValidationReasonInvalidOptionReference,
		TravelQueryValidationReasonOptionReferenceMismatch,
		TravelQueryValidationReasonOptionIndexOutOfRange,
		TravelQueryValidationReasonOptionDateNotVisible,
		TravelQueryValidationReasonAmbiguousOptionReference,
		TravelQueryValidationReasonInvalidRouteCoverage,
		TravelQueryValidationReasonRouteCoverageEvidenceRequired,
		TravelQueryValidationReasonInvalidSeatRequest,
		TravelQueryValidationReasonInvalidInstitutionalTopic,
		TravelQueryValidationReasonStrongDecisionProtected:
		return true
	default:
		return false
	}
}

func travelQueryMeaningV2EnumsAllowed(proposal TravelQueryMeaningV2) bool {
	if !travelQueryIntentAllowed(proposal.Intent) || !travelQueryTurnMeaningAllowed(proposal.TurnMeaning) {
		return false
	}
	if !travelQueryDateModeAllowed(proposal.DatePreference.Mode) ||
		!travelQueryOptionReferenceKindAllowed(proposal.OptionReference.Kind) ||
		!travelQueryRouteCoverageModeAllowed(proposal.RouteCoverage.Mode) ||
		!travelQuerySeatRequestModeAllowed(proposal.SeatRequest) ||
		!travelQueryInstitutionalTopicAllowed(proposal.InstitutionalTopic) {
		return false
	}
	if proposal.Origin != nil && !travelQueryLocationRoleAllowed(proposal.Origin.Role) {
		return false
	}
	if proposal.Destination != nil && !travelQueryLocationRoleAllowed(proposal.Destination.Role) {
		return false
	}
	for _, location := range proposal.MentionedLocations {
		if !travelQueryLocationRoleAllowed(location.Role) {
			return false
		}
	}
	return true
}

func travelQueryIntentAllowed(intent TravelQueryIntent) bool {
	switch intent {
	case TravelQueryIntentUnknown,
		TravelQueryIntentAvailabilitySearch,
		TravelQueryIntentSelectAvailabilityOption,
		TravelQueryIntentRouteCoverage,
		TravelQueryIntentSeatRequest,
		TravelQueryIntentInstitutionalQuestion:
		return true
	default:
		return false
	}
}

func travelQueryTurnMeaningAllowed(meaning TurnMeaning) bool {
	switch meaning {
	case TurnMeaningUnknown,
		TurnMeaningGreeting,
		TurnMeaningNewRequest,
		TurnMeaningAnswerToQuestion,
		TurnMeaningConfirmation,
		TurnMeaningAcknowledgement:
		return true
	default:
		return false
	}
}

func travelQueryDateModeAllowed(mode DateMode) bool {
	switch mode {
	case DateModeUnspecified, DateModeExact, DateModeEarliestAvailable, DateModeAnyAvailable:
		return true
	default:
		return false
	}
}

func travelQueryOptionReferenceKindAllowed(kind OptionReferenceKind) bool {
	switch kind {
	case OptionReferenceNone, OptionReferenceIndex, OptionReferenceDate, OptionReferenceDeictic:
		return true
	default:
		return false
	}
}

func travelQueryRouteCoverageModeAllowed(mode RouteCoverageMode) bool {
	switch mode {
	case RouteCoverageModeUnspecified, RouteCoverageModeExactStop, RouteCoverageModeNearbyReference:
		return true
	default:
		return false
	}
}

func travelQuerySeatRequestModeAllowed(mode SeatRequestMode) bool {
	switch mode {
	case SeatRequestNone, SeatRequestBookTravel, SeatRequestChooseSpecificSeat:
		return true
	default:
		return false
	}
}

func travelQueryInstitutionalTopicAllowed(topic InstitutionalTopic) bool {
	switch topic {
	case InstitutionalTopicNone, InstitutionalTopicCompanyInfo, InstitutionalTopicContact,
		InstitutionalTopicAddress, InstitutionalTopicBusinessHours:
		return true
	default:
		return false
	}
}

func travelQueryLocationRoleAllowed(role LocationRole) bool {
	switch role {
	case LocationRoleUnknown, LocationRoleOrigin, LocationRoleDestination, LocationRoleVia, LocationRoleNearbyReference:
		return true
	default:
		return false
	}
}

func travelQueryDecisionStrengthAllowed(strength DecisionStrength) bool {
	switch strength {
	case DecisionStrengthStrong, DecisionStrengthWeak, DecisionStrengthFallback:
		return true
	default:
		return false
	}
}

func travelQueryClarificationCoherent(input TravelQueryValidationInputV2, context travelQueryValidationContext) bool {
	missing := make(map[string]struct{}, len(input.Proposal.MissingFields))
	for _, raw := range input.Proposal.MissingFields {
		field := strings.TrimSpace(raw)
		if !travelQueryMissingFieldAllowed(field) {
			return false
		}
		if _, duplicate := missing[field]; duplicate {
			return false
		}
		missing[field] = struct{}{}
	}
	if input.Proposal.NeedsClarification != (len(missing) > 0) {
		return false
	}
	for field := range missing {
		switch field {
		case "origin":
			if effectiveTravelQueryOrigin(input, context) != "" {
				return false
			}
		case "destination":
			if effectiveTravelQueryDestination(input, context) != "" {
				return false
			}
		case "date":
			if !travelQueryDateModeUnset(input.Proposal.DatePreference.Mode) {
				return false
			}
		case "option_reference":
			if input.Proposal.OptionReference.Kind == OptionReferenceIndex {
				return false
			}
		case "route_coverage":
			if !travelQueryRouteCoverageUnset(input.Proposal.RouteCoverage) {
				return false
			}
		case "seat_request":
			if !travelQuerySeatRequestUnset(input.Proposal.SeatRequest) {
				return false
			}
		case "institutional_topic":
			if !travelQueryInstitutionalTopicUnset(input.Proposal.InstitutionalTopic) {
				return false
			}
		}
	}
	return true
}

func travelQueryMissingFieldAllowed(field string) bool {
	switch field {
	case "origin", "destination", "date", "option_reference", "route_coverage", "seat_request", "institutional_topic":
		return true
	default:
		return false
	}
}

func validateTravelQuerySemanticCombination(input TravelQueryValidationInputV2) TravelQueryValidationReasonCode {
	proposal := input.Proposal
	if proposal.TurnMeaning == TurnMeaningAcknowledgement {
		if proposal.Intent != TravelQueryIntentUnknown || !travelQueryAcknowledgementPayloadEmpty(proposal) {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
		return ""
	}
	switch proposal.Intent {
	case TravelQueryIntentAvailabilitySearch:
		if !travelQueryOptionReferenceUnset(proposal.OptionReference) ||
			!travelQueryRouteCoverageUnset(proposal.RouteCoverage) ||
			!travelQuerySeatRequestUnset(proposal.SeatRequest) ||
			!travelQueryInstitutionalTopicUnset(proposal.InstitutionalTopic) {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
	case TravelQueryIntentSelectAvailabilityOption:
		if !travelQueryDatePreferenceUnset(proposal.DatePreference) ||
			!travelQueryRouteCoverageUnset(proposal.RouteCoverage) ||
			!travelQuerySeatRequestUnset(proposal.SeatRequest) ||
			!travelQueryInstitutionalTopicUnset(proposal.InstitutionalTopic) {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
	case TravelQueryIntentRouteCoverage:
		if travelQueryRouteCoverageUnset(proposal.RouteCoverage) &&
			!(proposal.NeedsClarification && travelQueryMissingFieldPresent(proposal, "route_coverage")) {
			return TravelQueryValidationReasonInvalidRouteCoverage
		}
		if !travelQueryDatePreferenceUnset(proposal.DatePreference) ||
			!travelQueryOptionReferenceUnset(proposal.OptionReference) ||
			!travelQuerySeatRequestUnset(proposal.SeatRequest) ||
			!travelQueryInstitutionalTopicUnset(proposal.InstitutionalTopic) {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
	case TravelQueryIntentSeatRequest:
		if travelQuerySeatRequestUnset(proposal.SeatRequest) &&
			!(proposal.NeedsClarification && travelQueryMissingFieldPresent(proposal, "seat_request")) {
			return TravelQueryValidationReasonInvalidSeatRequest
		}
		if !travelQueryOptionReferenceUnset(proposal.OptionReference) ||
			!travelQueryRouteCoverageUnset(proposal.RouteCoverage) ||
			!travelQueryInstitutionalTopicUnset(proposal.InstitutionalTopic) {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
	case TravelQueryIntentInstitutionalQuestion:
		if travelQueryInstitutionalTopicUnset(proposal.InstitutionalTopic) &&
			!(proposal.NeedsClarification && travelQueryMissingFieldPresent(proposal, "institutional_topic")) {
			return TravelQueryValidationReasonInvalidInstitutionalTopic
		}
		if !travelQueryDatePreferenceUnset(proposal.DatePreference) ||
			!travelQueryOptionReferenceUnset(proposal.OptionReference) ||
			!travelQueryRouteCoverageUnset(proposal.RouteCoverage) ||
			!travelQuerySeatRequestUnset(proposal.SeatRequest) ||
			proposal.Origin != nil || proposal.Destination != nil || len(proposal.MentionedLocations) > 0 {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
	}
	return ""
}

func travelQueryAcknowledgementPayloadEmpty(proposal TravelQueryMeaningV2) bool {
	return proposal.Origin == nil &&
		proposal.Destination == nil &&
		len(proposal.MentionedLocations) == 0 &&
		travelQueryDatePreferenceUnset(proposal.DatePreference) &&
		travelQueryOptionReferenceUnset(proposal.OptionReference) &&
		travelQueryRouteCoverageUnset(proposal.RouteCoverage) &&
		travelQuerySeatRequestUnset(proposal.SeatRequest) &&
		travelQueryInstitutionalTopicUnset(proposal.InstitutionalTopic) &&
		!proposal.NeedsClarification &&
		len(proposal.MissingFields) == 0
}

func travelQueryDateModeUnset(mode DateMode) bool {
	return mode == "" || mode == DateModeUnspecified
}

func travelQueryDatePreferenceUnset(preference DatePreference) bool {
	return travelQueryDateModeUnset(preference.Mode) && strings.TrimSpace(preference.ExactDate) == ""
}

func travelQueryOptionReferenceUnset(reference OptionReference) bool {
	return (reference.Kind == "" || reference.Kind == OptionReferenceNone) && reference.Index == 0 && strings.TrimSpace(reference.Date) == ""
}

func travelQueryRouteCoverageUnset(coverage RouteCoverageMeaning) bool {
	return (coverage.Mode == "" || coverage.Mode == RouteCoverageModeUnspecified) && strings.TrimSpace(coverage.QueryLocation) == ""
}

func travelQuerySeatRequestUnset(mode SeatRequestMode) bool {
	return mode == "" || mode == SeatRequestNone
}

func travelQueryInstitutionalTopicUnset(topic InstitutionalTopic) bool {
	return topic == "" || topic == InstitutionalTopicNone
}

type travelQueryLocationUse struct {
	name string
	role LocationRole
}

func validateTravelQueryLocations(input TravelQueryValidationInputV2, context travelQueryValidationContext) TravelQueryValidationReasonCode {
	uses := make([]travelQueryLocationUse, 0, len(input.Proposal.MentionedLocations)+2)
	if input.Proposal.Origin != nil {
		if input.Proposal.Origin.Role != LocationRoleOrigin {
			return TravelQueryValidationReasonContradictoryLocationRole
		}
		uses = append(uses, travelQueryLocationUse{name: input.Proposal.Origin.Name, role: input.Proposal.Origin.Role})
	}
	if input.Proposal.Destination != nil {
		if input.Proposal.Destination.Role != LocationRoleDestination {
			return TravelQueryValidationReasonContradictoryLocationRole
		}
		uses = append(uses, travelQueryLocationUse{name: input.Proposal.Destination.Name, role: input.Proposal.Destination.Role})
	}
	for _, location := range input.Proposal.MentionedLocations {
		if location.Role == "" || location.Role == LocationRoleUnknown {
			return TravelQueryValidationReasonContradictoryLocationRole
		}
		uses = append(uses, travelQueryLocationUse{name: location.Name, role: location.Role})
	}

	rolesByLocation := make(map[string]LocationRole, len(uses))
	for _, use := range uses {
		match, ok := findTravelQueryLocationEvidence(use.name, input.LocationCatalog)
		if !ok {
			return TravelQueryValidationReasonLocationNotInCatalog
		}
		key := travelQueryCanonicalLocationKey(match.CanonicalName)
		if previous, exists := rolesByLocation[key]; exists && previous != use.role {
			return TravelQueryValidationReasonContradictoryLocationRole
		}
		rolesByLocation[key] = use.role
		if !travelQueryLocationGrounded(input, context, match, use.role) {
			return TravelQueryValidationReasonLocationNotGrounded
		}
	}
	return ""
}

func findTravelQueryLocationEvidence(name string, catalog []TravelQueryLocationEvidenceV2) (TravelQueryLocationEvidenceV2, bool) {
	requested := travelQueryCanonicalLocationKey(name)
	if requested == "" {
		return TravelQueryLocationEvidenceV2{}, false
	}
	for _, evidence := range catalog {
		if travelQueryCanonicalLocationKey(evidence.CanonicalName) == requested {
			return evidence, true
		}
	}
	if strings.Contains(requested, "/") {
		return TravelQueryLocationEvidenceV2{}, false
	}
	requestedCity := travelQueryLocationCityKey(requested)
	var match TravelQueryLocationEvidenceV2
	count := 0
	for _, evidence := range catalog {
		canonical := travelQueryCanonicalLocationKey(evidence.CanonicalName)
		if canonical != "" && travelQueryLocationCityKey(canonical) == requestedCity {
			match = evidence
			count++
		}
	}
	return match, count == 1
}

// travelQueryCanonicalLocationKey normalizes only stable representation details.
// It intentionally does not apply ASR corrections, aliases, fuzzy matching, or
// city substitutions: those belong to the producer of the structured meaning.
func travelQueryCanonicalLocationKey(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	key = travelQueryCanonicalLocationAccentReplacer.Replace(key)
	key = strings.Trim(key, " .,:;!?")
	key = strings.ReplaceAll(key, "\\", "/")
	parts := strings.Split(key, "/")
	for index := range parts {
		parts[index] = strings.Join(strings.Fields(parts[index]), " ")
	}
	return strings.Join(parts, "/")
}

func travelQueryLocationCityKey(key string) string {
	if index := strings.LastIndex(key, "/"); index >= 0 {
		return strings.TrimSpace(key[:index])
	}
	return strings.TrimSpace(key)
}

func travelQueryLocationGrounded(input TravelQueryValidationInputV2, context travelQueryValidationContext, evidence TravelQueryLocationEvidenceV2, role LocationRole) bool {
	if role != LocationRoleOrigin && role != LocationRoleDestination {
		return !travelQueryCatalogCityAmbiguous(evidence, input.LocationCatalog)
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsValid {
		return travelQueryLocationGroundedByAvailabilityFacts(input.Proposal.OptionReference, context.availabilityFacts.options, evidence, role, input.LocationCatalog)
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsInvalid {
		return false
	}

	stateLocation := input.State.Route.Origin
	if role == LocationRoleDestination {
		stateLocation = input.State.Route.Destination
	}
	if strings.TrimSpace(stateLocation) != "" {
		return travelQueryStructuredLocationMatches(stateLocation, evidence, input.LocationCatalog)
	}
	return !travelQueryCatalogCityAmbiguous(evidence, input.LocationCatalog)
}

func travelQueryLocationGroundedByAvailabilityFacts(reference OptionReference, options []AvailabilitySearchItem, evidence TravelQueryLocationEvidenceV2, role LocationRole, catalog []TravelQueryLocationEvidenceV2) bool {
	if reference.Kind == OptionReferenceIndex && reference.Index > 0 && reference.Index <= len(options) {
		return travelQueryAvailabilityEndpointMatches(options[reference.Index-1], evidence, role, catalog)
	}
	collectiveStopID := ""
	for _, option := range options {
		endpoint, ok := travelQueryAvailabilityEndpointEvidence(option, role, catalog)
		if !ok || travelQueryCanonicalLocationKey(endpoint.CanonicalName) != travelQueryCanonicalLocationKey(evidence.CanonicalName) {
			return false
		}
		stopID := strings.ToUpper(strings.TrimSpace(endpoint.StopID))
		if collectiveStopID != "" && stopID != collectiveStopID {
			return false
		}
		collectiveStopID = stopID
	}
	return collectiveStopID != ""
}

func travelQueryAvailabilityEndpointMatches(option AvailabilitySearchItem, evidence TravelQueryLocationEvidenceV2, role LocationRole, catalog []TravelQueryLocationEvidenceV2) bool {
	endpoint, ok := travelQueryAvailabilityEndpointEvidence(option, role, catalog)
	return ok && travelQueryCanonicalLocationKey(endpoint.CanonicalName) == travelQueryCanonicalLocationKey(evidence.CanonicalName)
}

func travelQueryAvailabilityEndpointEvidence(option AvailabilitySearchItem, role LocationRole, catalog []TravelQueryLocationEvidenceV2) (TravelQueryLocationEvidenceV2, bool) {
	stopID := option.OriginStopID
	displayName := option.OriginDisplayName
	if role == LocationRoleDestination {
		stopID = option.DestinationStopID
		displayName = option.DestinationDisplayName
	}
	evidence, ok := findTravelQueryLocationEvidenceByStopID(stopID, catalog)
	if !ok || !travelQueryAvailabilityDisplayConsistent(displayName, evidence) {
		return TravelQueryLocationEvidenceV2{}, false
	}
	return evidence, true
}

func findTravelQueryLocationEvidenceByStopID(stopID string, catalog []TravelQueryLocationEvidenceV2) (TravelQueryLocationEvidenceV2, bool) {
	requested := strings.ToUpper(strings.TrimSpace(stopID))
	if requested == "" {
		return TravelQueryLocationEvidenceV2{}, false
	}
	var match TravelQueryLocationEvidenceV2
	count := 0
	for _, evidence := range catalog {
		if strings.ToUpper(strings.TrimSpace(evidence.StopID)) != requested || travelQueryCanonicalLocationKey(evidence.CanonicalName) == "" {
			continue
		}
		match = evidence
		count++
	}
	return match, count == 1
}

func travelQueryAvailabilityDisplayConsistent(displayName string, evidence TravelQueryLocationEvidenceV2) bool {
	displayKey := travelQueryCanonicalLocationKey(displayName)
	if displayKey == "" {
		return true
	}
	evidenceKey := travelQueryCanonicalLocationKey(evidence.CanonicalName)
	if displayKey == evidenceKey {
		return true
	}
	if strings.Contains(displayKey, "/") {
		return false
	}
	return displayKey == travelQueryLocationCityKey(evidenceKey)
}

func travelQueryStructuredLocationMatches(name string, evidence TravelQueryLocationEvidenceV2, catalog []TravelQueryLocationEvidenceV2) bool {
	match, ok := findTravelQueryLocationEvidence(name, catalog)
	return ok && travelQueryCanonicalLocationKey(match.CanonicalName) == travelQueryCanonicalLocationKey(evidence.CanonicalName)
}

func travelQueryCatalogCityAmbiguous(evidence TravelQueryLocationEvidenceV2, catalog []TravelQueryLocationEvidenceV2) bool {
	city := travelQueryLocationCityKey(travelQueryCanonicalLocationKey(evidence.CanonicalName))
	if city == "" {
		return false
	}
	variants := make(map[string]struct{})
	for _, candidate := range catalog {
		key := travelQueryCanonicalLocationKey(candidate.CanonicalName)
		if travelQueryLocationCityKey(key) != city {
			continue
		}
		variants[key] = struct{}{}
	}
	return len(variants) > 1
}

func travelQueryLocationNamesEqual(left string, right string) bool {
	leftKey := travelQueryCanonicalLocationKey(left)
	rightKey := travelQueryCanonicalLocationKey(right)
	if leftKey == "" || rightKey == "" {
		return false
	}
	if leftKey == rightKey {
		return true
	}
	if strings.Contains(leftKey, "/") && strings.Contains(rightKey, "/") {
		return false
	}
	return travelQueryLocationCityKey(leftKey) == travelQueryLocationCityKey(rightKey)
}

func validateTravelQueryDatePreference(input TravelQueryValidationInputV2) TravelQueryValidationReasonCode {
	preference := input.Proposal.DatePreference
	if preference.Mode != DateModeExact {
		if strings.TrimSpace(preference.ExactDate) != "" {
			return TravelQueryValidationReasonInconsistentSemanticCombination
		}
		return ""
	}
	if input.ObservedAt.IsZero() {
		return TravelQueryValidationReasonObservedAtRequired
	}
	date, ok := parseTravelQueryISODate(preference.ExactDate)
	if !ok {
		return TravelQueryValidationReasonInvalidExactDate
	}
	if date.Before(travelQueryObservedDate(input.ObservedAt)) {
		return TravelQueryValidationReasonExactDateInPast
	}
	return ""
}

func validateTravelQueryEarliestAvailable(input TravelQueryValidationInputV2, context travelQueryValidationContext) TravelQueryValidationReasonCode {
	if input.Proposal.DatePreference.Mode != DateModeEarliestAvailable {
		return ""
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsValid {
		if !context.hasCurrentRoute || strings.TrimSpace(context.currentRoute.origin.StopID) == "" ||
			strings.TrimSpace(context.currentRoute.destination.StopID) == "" ||
			travelQueryLocationNamesEqual(context.currentRoute.origin.CanonicalName, context.currentRoute.destination.CanonicalName) {
			return TravelQueryValidationReasonEarliestAvailableRouteRequired
		}
		return ""
	}
	origin, originOK := findTravelQueryLocationEvidence(effectiveTravelQueryOrigin(input, context), input.LocationCatalog)
	destination, destinationOK := findTravelQueryLocationEvidence(effectiveTravelQueryDestination(input, context), input.LocationCatalog)
	if !originOK || !destinationOK || strings.TrimSpace(origin.StopID) == "" || strings.TrimSpace(destination.StopID) == "" ||
		travelQueryLocationNamesEqual(origin.CanonicalName, destination.CanonicalName) {
		return TravelQueryValidationReasonEarliestAvailableRouteRequired
	}
	return ""
}

func effectiveTravelQueryOrigin(input TravelQueryValidationInputV2, context travelQueryValidationContext) string {
	if input.Proposal.Origin != nil && strings.TrimSpace(input.Proposal.Origin.Name) != "" {
		return strings.TrimSpace(input.Proposal.Origin.Name)
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsValid {
		if context.hasCurrentRoute {
			return context.currentRoute.origin.CanonicalName
		}
		return ""
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsInvalid {
		return ""
	}
	return strings.TrimSpace(input.State.Route.Origin)
}

func effectiveTravelQueryDestination(input TravelQueryValidationInputV2, context travelQueryValidationContext) string {
	if input.Proposal.Destination != nil && strings.TrimSpace(input.Proposal.Destination.Name) != "" {
		return strings.TrimSpace(input.Proposal.Destination.Name)
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsValid {
		if context.hasCurrentRoute {
			return context.currentRoute.destination.CanonicalName
		}
		return ""
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsInvalid {
		return ""
	}
	return strings.TrimSpace(input.State.Route.Destination)
}

func validateTravelQueryOptionReference(input TravelQueryValidationInputV2, context travelQueryValidationContext) TravelQueryValidationReasonCode {
	reference := input.Proposal.OptionReference
	isSelection := input.Proposal.Intent == TravelQueryIntentSelectAvailabilityOption
	if !isSelection && travelQueryOptionReferenceUnset(reference) {
		return ""
	}
	if input.ObservedAt.IsZero() {
		return TravelQueryValidationReasonObservedAtRequired
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsInvalid {
		return TravelQueryValidationReasonCurrentAvailabilityFactsInvalid
	}
	if context.availabilityFacts.state == travelQueryAvailabilityFactsNotApplicable {
		return TravelQueryValidationReasonCurrentAvailabilityFactsRequired
	}
	options := context.availabilityFacts.options

	switch reference.Kind {
	case OptionReferenceNone:
		if isSelection && input.Proposal.NeedsClarification && travelQueryMissingFieldPresent(input.Proposal, "option_reference") {
			return ""
		}
		return TravelQueryValidationReasonInvalidOptionReference
	case OptionReferenceIndex:
		if reference.Index > len(options) {
			return TravelQueryValidationReasonOptionIndexOutOfRange
		}
	case OptionReferenceDate:
		date, _ := parseTravelQueryISODate(reference.Date)
		matches := 0
		for _, option := range options {
			optionDate, optionValid := parseTravelQueryISODate(option.TripDate)
			if optionValid && optionDate.Equal(date) {
				matches++
			}
		}
		if matches == 0 {
			return TravelQueryValidationReasonOptionDateNotVisible
		}
		if matches > 1 && !travelQueryOptionAmbiguityClarified(input.Proposal) {
			return TravelQueryValidationReasonAmbiguousOptionReference
		}
		if matches == 1 && travelQueryOptionAmbiguityClarified(input.Proposal) {
			return TravelQueryValidationReasonIncoherentClarification
		}
	case OptionReferenceDeictic:
		if len(options) > 1 && !travelQueryOptionAmbiguityClarified(input.Proposal) {
			return TravelQueryValidationReasonAmbiguousOptionReference
		}
		if len(options) == 1 && travelQueryOptionAmbiguityClarified(input.Proposal) {
			return TravelQueryValidationReasonIncoherentClarification
		}
	}
	return ""
}

func validateTravelQueryOptionReferenceShape(reference OptionReference) TravelQueryValidationReasonCode {
	switch reference.Kind {
	case OptionReferenceNone:
		if reference.Index != 0 || strings.TrimSpace(reference.Date) != "" {
			return TravelQueryValidationReasonInvalidOptionReference
		}
	case OptionReferenceIndex:
		if reference.Index <= 0 || strings.TrimSpace(reference.Date) != "" {
			return TravelQueryValidationReasonInvalidOptionReference
		}
	case OptionReferenceDate:
		if reference.Index != 0 {
			return TravelQueryValidationReasonInvalidOptionReference
		}
		if _, valid := parseTravelQueryISODate(reference.Date); !valid {
			return TravelQueryValidationReasonInvalidOptionReference
		}
	case OptionReferenceDeictic:
		if reference.Index != 0 || strings.TrimSpace(reference.Date) != "" {
			return TravelQueryValidationReasonInvalidOptionReference
		}
	}
	return ""
}

func newTravelQueryValidationContext(input TravelQueryValidationInputV2) travelQueryValidationContext {
	evaluation := classifyCurrentTravelQueryAvailabilityFacts(input)
	context := travelQueryValidationContext{availabilityFacts: evaluation}
	if evaluation.state != travelQueryAvailabilityFactsValid {
		return context
	}
	context.currentRoute, context.hasCurrentRoute = travelQueryRouteFromAvailabilityFacts(
		input.Proposal.OptionReference,
		evaluation.options,
		input.LocationCatalog,
	)
	return context
}

func travelQueryRouteFromAvailabilityFacts(reference OptionReference, options []AvailabilitySearchItem, catalog []TravelQueryLocationEvidenceV2) (travelQueryFactualRoute, bool) {
	relevantOptions := options
	if reference.Kind == OptionReferenceIndex && reference.Index > 0 && reference.Index <= len(options) {
		relevantOptions = options[reference.Index-1 : reference.Index]
	}
	if len(relevantOptions) == 0 {
		return travelQueryFactualRoute{}, false
	}

	var route travelQueryFactualRoute
	var originStopID string
	var destinationStopID string
	for index, option := range relevantOptions {
		origin, originOK := travelQueryAvailabilityEndpointEvidence(option, LocationRoleOrigin, catalog)
		destination, destinationOK := travelQueryAvailabilityEndpointEvidence(option, LocationRoleDestination, catalog)
		if !originOK || !destinationOK {
			return travelQueryFactualRoute{}, false
		}
		optionOriginStopID := strings.ToUpper(strings.TrimSpace(origin.StopID))
		optionDestinationStopID := strings.ToUpper(strings.TrimSpace(destination.StopID))
		if optionOriginStopID == "" || optionDestinationStopID == "" {
			return travelQueryFactualRoute{}, false
		}
		if index == 0 {
			route = travelQueryFactualRoute{origin: origin, destination: destination}
			originStopID = optionOriginStopID
			destinationStopID = optionDestinationStopID
			continue
		}
		if optionOriginStopID != originStopID || optionDestinationStopID != destinationStopID {
			return travelQueryFactualRoute{}, false
		}
	}
	return route, true
}

func classifyCurrentTravelQueryAvailabilityFacts(input TravelQueryValidationInputV2) travelQueryAvailabilityFactsEvaluation {
	facts := input.AvailabilityFacts
	prompt := input.ActivePrompt
	if strings.TrimSpace(facts.SourceMessageID) == "" || facts.SourceMessageReceivedAt.IsZero() ||
		prompt.Kind != ActivePromptAvailabilityOptionChoice ||
		strings.TrimSpace(prompt.SourceMessageID) == "" || prompt.SourceMessageID != facts.SourceMessageID ||
		prompt.SourceMessageReceivedAt.IsZero() || !prompt.SourceMessageReceivedAt.Equal(facts.SourceMessageReceivedAt) {
		return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsNotApplicable}
	}
	if prompt.Phase != "" && input.State.Phase != "" && prompt.Phase != input.State.Phase {
		return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsNotApplicable}
	}
	message, ok := latestReliableAssistantMessage(input.History)
	if !ok || strings.TrimSpace(message.ID) != facts.SourceMessageID || !message.ReceivedAt.Equal(facts.SourceMessageReceivedAt) {
		return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsNotApplicable}
	}
	if strings.TrimSpace(prompt.SourceMessageBody) != "" &&
		!equivalentAssistantPromptBody(prompt.SourceMessageBody, messageTurnText(message)) {
		return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsNotApplicable}
	}
	if !prompt.HasAvailabilityList || prompt.AvailabilityOptionCount <= 0 ||
		len(facts.VisibleOptions) == 0 || prompt.AvailabilityOptionCount != len(facts.VisibleOptions) {
		return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsInvalid}
	}
	if input.ObservedAt.IsZero() {
		return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsInvalid}
	}
	today := travelQueryObservedDate(input.ObservedAt)
	for _, option := range facts.VisibleOptions {
		if !hasCompleteSelectedTripFacts(option.TripID, option.BoardStopID, option.AlightStopID) {
			return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsInvalid}
		}
		if _, ok := travelQueryAvailabilityEndpointEvidence(option, LocationRoleOrigin, input.LocationCatalog); !ok {
			return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsInvalid}
		}
		if _, ok := travelQueryAvailabilityEndpointEvidence(option, LocationRoleDestination, input.LocationCatalog); !ok {
			return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsInvalid}
		}
		date, valid := parseTravelQueryISODate(option.TripDate)
		if !valid || date.Before(today) {
			return travelQueryAvailabilityFactsEvaluation{state: travelQueryAvailabilityFactsInvalid}
		}
	}
	return travelQueryAvailabilityFactsEvaluation{
		state:   travelQueryAvailabilityFactsValid,
		options: facts.VisibleOptions,
	}
}

func travelQueryOptionAmbiguityClarified(proposal TravelQueryMeaningV2) bool {
	return proposal.NeedsClarification && travelQueryMissingFieldPresent(proposal, "option_reference")
}

func travelQueryMissingFieldPresent(proposal TravelQueryMeaningV2, field string) bool {
	for _, candidate := range proposal.MissingFields {
		if strings.TrimSpace(candidate) == field {
			return true
		}
	}
	return false
}

func validateTravelQueryRouteCoverage(input TravelQueryValidationInputV2, context travelQueryValidationContext) TravelQueryValidationReasonCode {
	if input.Proposal.Intent != TravelQueryIntentRouteCoverage {
		return ""
	}
	if input.Proposal.NeedsClarification && travelQueryMissingFieldPresent(input.Proposal, "route_coverage") {
		return ""
	}
	coverage := input.Proposal.RouteCoverage
	if strings.TrimSpace(coverage.QueryLocation) == "" ||
		(coverage.Mode != RouteCoverageModeExactStop && coverage.Mode != RouteCoverageModeNearbyReference) {
		return TravelQueryValidationReasonInvalidRouteCoverage
	}
	evidence, ok := findTravelQueryLocationEvidence(coverage.QueryLocation, input.LocationCatalog)
	if !ok {
		return TravelQueryValidationReasonLocationNotInCatalog
	}
	expectedRole := LocationRoleVia
	if coverage.Mode == RouteCoverageModeNearbyReference {
		expectedRole = LocationRoleNearbyReference
	}
	if !travelQueryCoverageLocationHasRole(input.Proposal.MentionedLocations, evidence, expectedRole, input.LocationCatalog) {
		return TravelQueryValidationReasonContradictoryLocationRole
	}
	if !travelQueryLocationGrounded(input, context, evidence, expectedRole) {
		return TravelQueryValidationReasonLocationNotGrounded
	}
	if coverage.Mode == RouteCoverageModeExactStop && strings.TrimSpace(evidence.StopID) == "" {
		return TravelQueryValidationReasonRouteCoverageEvidenceRequired
	}
	return ""
}

func travelQueryCoverageLocationHasRole(locations []LocationMeaning, evidence TravelQueryLocationEvidenceV2, expectedRole LocationRole, catalog []TravelQueryLocationEvidenceV2) bool {
	for _, location := range locations {
		match, ok := findTravelQueryLocationEvidence(location.Name, catalog)
		if ok && travelQueryCanonicalLocationKey(match.CanonicalName) == travelQueryCanonicalLocationKey(evidence.CanonicalName) {
			return location.Role == expectedRole
		}
	}
	return false
}

func parseTravelQueryISODate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func travelQueryObservedDate(observedAt time.Time) time.Time {
	return time.Date(observedAt.Year(), observedAt.Month(), observedAt.Day(), 0, 0, 0, 0, time.UTC)
}
