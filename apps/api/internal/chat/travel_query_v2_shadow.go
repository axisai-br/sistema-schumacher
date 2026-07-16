package chat

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	travelQueryV2ShadowKey                         = "travel_query_v2_shadow"
	travelQueryV2ShadowUnallowlistedLocationMarker = "__redacted_unallowlisted"
	travelQueryV2ShadowSchemaInvalidReason         = "openai_schema_invalid"
)

type TravelQueryV2ShadowStatus string

const (
	TravelQueryV2ShadowDisabled       TravelQueryV2ShadowStatus = "disabled"
	TravelQueryV2ShadowOpenAIDisabled TravelQueryV2ShadowStatus = "openai_disabled"
	TravelQueryV2ShadowInProgress     TravelQueryV2ShadowStatus = "in_progress"
	TravelQueryV2ShadowValid          TravelQueryV2ShadowStatus = "valid"
	TravelQueryV2ShadowError          TravelQueryV2ShadowStatus = "error"
	TravelQueryV2ShadowRefused        TravelQueryV2ShadowStatus = "refused"
)

type TravelQueryV2ShadowValidationStatus string

const (
	TravelQueryV2ShadowValidationSkipped  TravelQueryV2ShadowValidationStatus = "skipped"
	TravelQueryV2ShadowValidationAccepted TravelQueryV2ShadowValidationStatus = "accepted"
	TravelQueryV2ShadowValidationRejected TravelQueryV2ShadowValidationStatus = "rejected"
)

type TravelQueryV2ShadowSummary struct {
	OpenAI     TravelQueryV2ShadowOpenAISummary `json:"openai"`
	Validation TravelQueryV2ShadowValidation    `json:"validation"`
}

type TravelQueryV2ShadowOpenAISummary struct {
	Status             string                               `json:"status"`
	Intent             string                               `json:"intent,omitempty"`
	TurnMeaning        string                               `json:"turn_meaning,omitempty"`
	Origin             *TravelQueryV2ShadowLocationSummary  `json:"origin,omitempty"`
	Destination        *TravelQueryV2ShadowLocationSummary  `json:"destination,omitempty"`
	MentionedLocations []TravelQueryV2ShadowLocationSummary `json:"mentioned_locations,omitempty"`
	DatePreference     TravelQueryV2ShadowDateSummary       `json:"date_preference,omitempty"`
	OptionReference    TravelQueryV2ShadowOptionSummary     `json:"option_reference,omitempty"`
	RouteCoverage      TravelQueryV2ShadowCoverageSummary   `json:"route_coverage,omitempty"`
	SeatRequest        string                               `json:"seat_request,omitempty"`
	InstitutionalTopic string                               `json:"institutional_topic,omitempty"`
	NeedsClarification bool                                 `json:"needs_clarification"`
	MissingFields      []string                             `json:"missing_fields,omitempty"`
	Confidence         float64                              `json:"confidence,omitempty"`
	ProviderResponseID string                               `json:"provider_response_id,omitempty"`
	LatencyMs          int64                                `json:"latency_ms"`
	ErrorCode          string                               `json:"error_code,omitempty"`
}

type TravelQueryV2ShadowLocationSummary struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type TravelQueryV2ShadowDateSummary struct {
	Mode      string `json:"mode,omitempty"`
	ExactDate string `json:"exact_date,omitempty"`
}

type TravelQueryV2ShadowOptionSummary struct {
	Kind  string `json:"kind,omitempty"`
	Index int    `json:"index,omitempty"`
	Date  string `json:"date,omitempty"`
}

type TravelQueryV2ShadowCoverageSummary struct {
	QueryLocation string `json:"query_location,omitempty"`
	Mode          string `json:"mode,omitempty"`
}

type TravelQueryV2ShadowValidation struct {
	Status      string   `json:"status"`
	Accepted    bool     `json:"accepted"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

type TravelQueryV2ShadowInput struct {
	Enabled                  bool
	OpenAIInterpreter        OpenAITravelQueryV2Interpreter
	StructuredInput          StructuredInterpreterInput
	ActivePrompt             ActivePromptContext
	AvailabilityFacts        TravelQueryAvailabilityFactsV2
	LocationCatalog          []TravelQueryLocationEvidenceV2
	ExistingDecisionStrength DecisionStrength
	IdempotencyKey           string
}

func RunTravelQueryV2Shadow(ctx context.Context, input TravelQueryV2ShadowInput) TravelQueryV2ShadowSummary {
	summary := TravelQueryV2ShadowSummary{
		OpenAI:     TravelQueryV2ShadowOpenAISummary{Status: string(TravelQueryV2ShadowDisabled)},
		Validation: skippedTravelQueryV2ShadowValidation("shadow_disabled"),
	}
	if !input.Enabled {
		return summary
	}
	if input.OpenAIInterpreter == nil || !input.OpenAIInterpreter.Enabled() {
		summary.OpenAI.Status = string(TravelQueryV2ShadowOpenAIDisabled)
		summary.Validation = skippedTravelQueryV2ShadowValidation("openai_disabled")
		return summary
	}

	startedAt := time.Now()
	result, err := input.OpenAIInterpreter.InterpretTravelQueryV2(ctx, OpenAITravelQueryV2RunInput{
		StructuredInput:          input.StructuredInput,
		ActivePrompt:             input.ActivePrompt,
		AvailabilityFacts:        input.AvailabilityFacts,
		LocationCatalog:          append([]TravelQueryLocationEvidenceV2(nil), input.LocationCatalog...),
		ExistingDecisionStrength: input.ExistingDecisionStrength,
		IdempotencyKey:           input.IdempotencyKey,
	})
	latencyMs := time.Since(startedAt).Milliseconds()

	if result.ProposalParseable {
		summary.OpenAI = summarizeTravelQueryV2ShadowProposal(result.Proposal, input.LocationCatalog)
		summary.OpenAI.ProviderResponseID = sanitizeTravelQueryV2ShadowProviderID(result.ProviderResponseID)
		summary.OpenAI.LatencyMs = latencyMs
		summary.Validation = validateTravelQueryV2ShadowProposal(input, result.Proposal, result.SchemaValid && err == nil)
	}
	if err != nil {
		summary.OpenAI.Status = string(TravelQueryV2ShadowError)
		if errors.Is(err, ErrOpenAITravelQueryV2Refusal) {
			summary.OpenAI.Status = string(TravelQueryV2ShadowRefused)
		}
		summary.OpenAI.ErrorCode = sanitizeTravelQueryV2ShadowError(err)
		summary.OpenAI.LatencyMs = latencyMs
		if !result.ProposalParseable {
			summary.Validation = skippedTravelQueryV2ShadowValidation(travelQueryV2ShadowValidationSkipReason(err))
		}
		return summary
	}

	summary.OpenAI = summarizeTravelQueryV2ShadowProposal(result.Proposal, input.LocationCatalog)
	summary.OpenAI.Status = string(TravelQueryV2ShadowValid)
	summary.OpenAI.ProviderResponseID = sanitizeTravelQueryV2ShadowProviderID(result.ProviderResponseID)
	summary.OpenAI.LatencyMs = latencyMs
	summary.Validation = validateTravelQueryV2ShadowProposal(input, result.Proposal, result.SchemaValid)
	return summary
}

func validateTravelQueryV2ShadowProposal(input TravelQueryV2ShadowInput, proposal TravelQueryMeaningV2, schemaValid bool) TravelQueryV2ShadowValidation {
	activePrompt := input.ActivePrompt
	if activePrompt.Kind == "" {
		activePrompt = InferActivePromptContext(input.StructuredInput.History, input.StructuredInput.State)
	}
	result := ValidateTravelQueryMeaningV2(TravelQueryValidationInputV2{
		Proposal:                 proposal,
		CurrentTurn:              input.StructuredInput.CurrentTurn,
		State:                    input.StructuredInput.State,
		ActivePrompt:             activePrompt,
		History:                  input.StructuredInput.History,
		ObservedAt:               input.StructuredInput.ObservedAt,
		AvailabilityFacts:        input.AvailabilityFacts,
		LocationCatalog:          append([]TravelQueryLocationEvidenceV2(nil), input.LocationCatalog...),
		ExistingDecisionStrength: input.ExistingDecisionStrength,
	})
	if schemaValid && result.Accepted() {
		return TravelQueryV2ShadowValidation{
			Status:   string(TravelQueryV2ShadowValidationAccepted),
			Accepted: true,
		}
	}
	reasons := make([]string, 0, len(result.ReasonCodes)+1)
	if !schemaValid {
		reasons = append(reasons, travelQueryV2ShadowSchemaInvalidReason)
	}
	for _, reason := range result.ReasonCodes {
		reasons = append(reasons, string(reason))
	}
	return TravelQueryV2ShadowValidation{
		Status:      string(TravelQueryV2ShadowValidationRejected),
		ReasonCodes: reasons,
	}
}

func skippedTravelQueryV2ShadowValidation(reason string) TravelQueryV2ShadowValidation {
	reason = strings.TrimSpace(reason)
	validation := TravelQueryV2ShadowValidation{Status: string(TravelQueryV2ShadowValidationSkipped)}
	if reason != "" {
		validation.ReasonCodes = []string{reason}
	}
	return validation
}

func summarizeTravelQueryV2ShadowProposal(proposal TravelQueryMeaningV2, catalog []TravelQueryLocationEvidenceV2) TravelQueryV2ShadowOpenAISummary {
	canonicalLocations := travelQueryV2ShadowCanonicalLocationAllowlist(catalog)
	summary := TravelQueryV2ShadowOpenAISummary{
		Intent:             sanitizeTravelQueryV2ShadowIntent(proposal.Intent),
		TurnMeaning:        sanitizeTravelQueryV2ShadowTurnMeaning(proposal.TurnMeaning),
		DatePreference:     TravelQueryV2ShadowDateSummary{Mode: sanitizeTravelQueryV2ShadowDateMode(proposal.DatePreference.Mode), ExactDate: sanitizeTravelQueryV2ShadowDate(proposal.DatePreference.ExactDate)},
		OptionReference:    TravelQueryV2ShadowOptionSummary{Kind: sanitizeTravelQueryV2ShadowOptionKind(proposal.OptionReference.Kind), Index: proposal.OptionReference.Index, Date: sanitizeTravelQueryV2ShadowDate(proposal.OptionReference.Date)},
		RouteCoverage:      TravelQueryV2ShadowCoverageSummary{QueryLocation: sanitizeTravelQueryV2ShadowCanonicalLocation(proposal.RouteCoverage.QueryLocation, canonicalLocations), Mode: sanitizeTravelQueryV2ShadowCoverageMode(proposal.RouteCoverage.Mode)},
		SeatRequest:        sanitizeTravelQueryV2ShadowSeatMode(proposal.SeatRequest),
		InstitutionalTopic: sanitizeTravelQueryV2ShadowInstitutionalTopic(proposal.InstitutionalTopic),
		NeedsClarification: proposal.NeedsClarification,
		MissingFields:      sanitizeTravelQueryV2ShadowMissingFields(proposal.MissingFields),
		Confidence:         sanitizeTravelQueryV2ShadowConfidence(proposal.Confidence),
	}
	if proposal.Origin != nil {
		summary.Origin = summarizeTravelQueryV2ShadowLocation(*proposal.Origin, canonicalLocations)
	}
	if proposal.Destination != nil {
		summary.Destination = summarizeTravelQueryV2ShadowLocation(*proposal.Destination, canonicalLocations)
	}
	limit := len(proposal.MentionedLocations)
	if limit > 8 {
		limit = 8
	}
	for _, location := range proposal.MentionedLocations[:limit] {
		if sanitized := summarizeTravelQueryV2ShadowLocation(location, canonicalLocations); sanitized != nil {
			summary.MentionedLocations = append(summary.MentionedLocations, *sanitized)
		}
	}
	return summary
}

func summarizeTravelQueryV2ShadowLocation(location LocationMeaning, canonicalLocations map[string]struct{}) *TravelQueryV2ShadowLocationSummary {
	name := sanitizeTravelQueryV2ShadowCanonicalLocation(location.Name, canonicalLocations)
	if name == "" {
		return nil
	}
	role := "__redacted_invalid"
	if travelQueryLocationRoleAllowed(location.Role) {
		role = string(location.Role)
	}
	return &TravelQueryV2ShadowLocationSummary{Name: name, Role: role}
}

func sanitizeTravelQueryV2ShadowIntent(value TravelQueryIntent) string {
	if travelQueryIntentAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowTurnMeaning(value TurnMeaning) string {
	if travelQueryTurnMeaningAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowDateMode(value DateMode) string {
	if travelQueryDateModeAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowOptionKind(value OptionReferenceKind) string {
	if travelQueryOptionReferenceKindAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowCoverageMode(value RouteCoverageMode) string {
	if travelQueryRouteCoverageModeAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowSeatMode(value SeatRequestMode) string {
	if travelQuerySeatRequestModeAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowInstitutionalTopic(value InstitutionalTopic) string {
	if travelQueryInstitutionalTopicAllowed(value) {
		return string(value)
	}
	return "__redacted_invalid"
}

func sanitizeTravelQueryV2ShadowConfidence(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return 0
	}
	return value
}

func travelQueryV2ShadowCanonicalLocationAllowlist(catalog []TravelQueryLocationEvidenceV2) map[string]struct{} {
	allowed := make(map[string]struct{}, len(catalog))
	for _, evidence := range catalog {
		name := strings.TrimSpace(evidence.CanonicalName)
		if name == "" || redactOpenAITravelQueryV2SensitiveText(name) != name || len(name) > 120 {
			continue
		}
		allowed[name] = struct{}{}
	}
	return allowed
}

func sanitizeTravelQueryV2ShadowCanonicalLocation(value string, allowed map[string]struct{}) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, ok := allowed[value]; !ok {
		return travelQueryV2ShadowUnallowlistedLocationMarker
	}
	return value
}

func sanitizeTravelQueryV2ShadowDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, ok := parseTravelQueryISODate(value); !ok {
		return "__redacted_invalid"
	}
	return value
}

func sanitizeTravelQueryV2ShadowMissingFields(fields []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if !travelQueryMissingFieldAllowed(field) {
			field = "__redacted_invalid"
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func sanitizeTravelQueryV2ShadowProviderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return ""
	}
	return value
}

func sanitizeTravelQueryV2ShadowError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrOpenAITravelQueryV2Disabled):
		return "openai_travel_query_v2_disabled"
	case errors.Is(err, ErrOpenAITravelQueryV2RequestFailed):
		return "openai_travel_query_v2_request_failed"
	case errors.Is(err, ErrOpenAITravelQueryV2EmptyOutput):
		return "openai_travel_query_v2_empty_output"
	case errors.Is(err, ErrOpenAITravelQueryV2InvalidOutput):
		return "openai_travel_query_v2_invalid_output"
	case errors.Is(err, ErrOpenAITravelQueryV2Refusal):
		return "openai_travel_query_v2_refusal"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline_exceeded"
	default:
		return "openai_travel_query_v2_error"
	}
}

func travelQueryV2ShadowValidationSkipReason(err error) string {
	switch {
	case errors.Is(err, ErrOpenAITravelQueryV2Disabled):
		return "openai_disabled"
	case errors.Is(err, ErrOpenAITravelQueryV2InvalidOutput):
		return "openai_schema_invalid"
	case errors.Is(err, ErrOpenAITravelQueryV2EmptyOutput):
		return "openai_missing_result"
	case errors.Is(err, ErrOpenAITravelQueryV2Refusal):
		return "openai_refusal"
	default:
		return "openai_error"
	}
}

func buildTravelQueryV2ShadowAvailabilityFacts(input StructuredInterpreterInput, activePrompt ActivePromptContext) TravelQueryAvailabilityFactsV2 {
	if activePrompt.Kind != ActivePromptAvailabilityOptionChoice || !activePrompt.HasAvailabilityList || activePrompt.AvailabilityOptionCount <= 0 {
		return TravelQueryAvailabilityFactsV2{}
	}
	message, ok := latestReliableAssistantMessage(input.History)
	if !ok || strings.TrimSpace(message.ID) != strings.TrimSpace(activePrompt.SourceMessageID) ||
		message.ReceivedAt.IsZero() || !message.ReceivedAt.Equal(activePrompt.SourceMessageReceivedAt) {
		return TravelQueryAvailabilityFactsV2{}
	}
	visible := visibleAvailabilityContextFromPromptMessageAt(message, input.ObservedAt)
	if visible == nil || len(visible.Results) != activePrompt.AvailabilityOptionCount {
		return TravelQueryAvailabilityFactsV2{}
	}
	return TravelQueryAvailabilityFactsV2{
		SourceMessageID:         strings.TrimSpace(message.ID),
		SourceMessageReceivedAt: message.ReceivedAt,
		VisibleOptions:          append([]AvailabilitySearchItem(nil), visible.Results...),
	}
}

func buildTravelQueryV2ShadowLocationCatalog(facts TravelQueryAvailabilityFactsV2) []TravelQueryLocationEvidenceV2 {
	byName := map[string]TravelQueryLocationEvidenceV2{}
	for _, destinations := range []map[string]string{scPackageDestinations, maPackageDestinations} {
		for _, canonicalName := range destinations {
			key := travelQueryCanonicalLocationKey(canonicalName)
			byName[key] = TravelQueryLocationEvidenceV2{
				CanonicalName: canonicalName,
				StopID:        strings.TrimSpace(canonicalStopIDsByLocation[key]),
			}
		}
	}
	for _, option := range facts.VisibleOptions {
		mergeTravelQueryV2ShadowLocationEvidence(byName, option.OriginDisplayName, option.OriginStopID)
		mergeTravelQueryV2ShadowLocationEvidence(byName, option.DestinationDisplayName, option.DestinationStopID)
	}
	keys := make([]string, 0, len(byName))
	for key := range byName {
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]TravelQueryLocationEvidenceV2, 0, len(keys))
	for _, key := range keys {
		out = append(out, byName[key])
	}
	return out
}

func mergeTravelQueryV2ShadowLocationEvidence(catalog map[string]TravelQueryLocationEvidenceV2, canonicalName string, stopID string) {
	stopID = strings.TrimSpace(stopID)
	if strings.TrimSpace(canonicalName) == "" || stopID == "" {
		return
	}
	matches := 0
	canonical := TravelQueryLocationEvidenceV2{}
	for _, evidence := range catalog {
		if strings.EqualFold(strings.TrimSpace(evidence.StopID), stopID) {
			matches++
			canonical = evidence
		}
	}
	// A display name is presentation-only. A unique known StopID already owns
	// its canonical identity; unknown or ambiguous IDs must not create one.
	if matches == 1 {
		catalog[travelQueryCanonicalLocationKey(canonical.CanonicalName)] = canonical
	}
}

func travelQueryV2ShadowDecisionStrength(decision IntentDecision, documentMediaGuardrail bool) DecisionStrength {
	if documentMediaGuardrail {
		return DecisionStrengthStrong
	}
	if decision.Intent == IntentUnknown || strings.EqualFold(strings.TrimSpace(decision.Action), "safe_fallback") {
		return DecisionStrengthFallback
	}
	if strings.EqualFold(strings.TrimSpace(decision.Source), "deterministic_broad_state") ||
		decision.Intent == IntentUnsupportedCargo || decision.Intent == IntentUnsupportedPackage {
		return DecisionStrengthWeak
	}
	switch decision.Intent {
	case IntentHumanSupport,
		IntentBookingCancel,
		IntentPassengerCountReply,
		IntentPassengerDocumentsProvided,
		IntentLapChildAssignmentAnswer,
		IntentDocumentConfirmation,
		IntentBookingCreateConfirmation,
		IntentPaymentPreference,
		IntentPaymentCreate:
		return DecisionStrengthStrong
	case IntentSelectAvailabilityOption:
		if decision.SelectedOptionIndex > 0 {
			return DecisionStrengthStrong
		}
	}
	return DecisionStrengthWeak
}
