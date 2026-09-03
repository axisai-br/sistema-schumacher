package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

var (
	ErrOpenAIPassengerMeaningV1Disabled      = errors.New("openai passenger meaning v1 disabled")
	ErrOpenAIPassengerMeaningV1RequestFailed = errors.New("openai passenger meaning v1 request failed")
	ErrOpenAIPassengerMeaningV1EmptyOutput   = errors.New("openai passenger meaning v1 returned empty output")
	ErrOpenAIPassengerMeaningV1InvalidOutput = errors.New("openai passenger meaning v1 returned invalid output")
	ErrOpenAIPassengerMeaningV1Refusal       = errors.New("openai passenger meaning v1 refused output")
)

type OpenAIPassengerMeaningV1Interpreter interface {
	Enabled() bool
	InterpretPassengerMeaningV1(ctx context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error)
}

type OpenAIPassengerMeaningV1RunInput struct {
	CurrentTurn         string
	State               PassengerClarificationStateV1
	PromptEvent         PassengerClarificationEventV1
	SourceMessageID     string
	SourcePromptEventID string
	IdempotencyKey      string
}

type OpenAIPassengerMeaningV1RunResult struct {
	Proposal           PassengerClarificationMeaningV1
	ProposalParseable  bool
	SchemaValid        bool
	SchemaReasonCodes  []string
	Model              string
	ProviderResponseID string
}

type OpenAIPassengerMeaningV1Runner struct {
	baseURL string
	apiKey  string
	model   string
	client  OpenAIResponsesDoer
}

func NewOpenAIPassengerMeaningV1Runner(cfg config.Config) *OpenAIPassengerMeaningV1Runner {
	return &OpenAIPassengerMeaningV1Runner{
		baseURL: strings.TrimSpace(cfg.OpenAIBaseURL),
		apiKey:  strings.TrimSpace(cfg.OpenAIAPIKey),
		model:   strings.TrimSpace(cfg.OpenAIModel),
		client:  &http.Client{Timeout: 45 * time.Second},
	}
}

func (r *OpenAIPassengerMeaningV1Runner) Enabled() bool {
	return r != nil && r.apiKey != "" && r.model != "" && r.baseURL != ""
}

func (r *OpenAIPassengerMeaningV1Runner) InterpretPassengerMeaningV1(ctx context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
	if !r.Enabled() {
		return OpenAIPassengerMeaningV1RunResult{}, ErrOpenAIPassengerMeaningV1Disabled
	}
	body, err := json.Marshal(r.buildRequestPayload(input))
	if err != nil {
		return OpenAIPassengerMeaningV1RunResult{}, fmt.Errorf("%w: %v", ErrOpenAIPassengerMeaningV1RequestFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return OpenAIPassengerMeaningV1RunResult{}, fmt.Errorf("%w: %v", ErrOpenAIPassengerMeaningV1RequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		req.Header.Set("X-Client-Request-Id", key)
	}

	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return OpenAIPassengerMeaningV1RunResult{}, fmt.Errorf("%w: %v", ErrOpenAIPassengerMeaningV1RequestFailed, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return OpenAIPassengerMeaningV1RunResult{}, fmt.Errorf("%w: %v", ErrOpenAIPassengerMeaningV1RequestFailed, err)
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		if compact := compactOpenAIErrorBody(responseBody); compact != "" {
			return OpenAIPassengerMeaningV1RunResult{}, fmt.Errorf("%w: status %d %s", ErrOpenAIPassengerMeaningV1RequestFailed, response.StatusCode, compact)
		}
		return OpenAIPassengerMeaningV1RunResult{}, fmt.Errorf("%w: status %d", ErrOpenAIPassengerMeaningV1RequestFailed, response.StatusCode)
	}

	responsePayload := map[string]interface{}{}
	if len(responseBody) == 0 || json.Unmarshal(responseBody, &responsePayload) != nil {
		return OpenAIPassengerMeaningV1RunResult{}, ErrOpenAIPassengerMeaningV1InvalidOutput
	}
	result := OpenAIPassengerMeaningV1RunResult{
		Model:              r.model,
		ProviderResponseID: strings.TrimSpace(asString(responsePayload["id"])),
	}
	if openAITravelQueryV2ResponseRefused(responsePayload) {
		return result, ErrOpenAIPassengerMeaningV1Refusal
	}
	rawOutput := strings.TrimSpace(extractOpenAIResponseText(responsePayload))
	if rawOutput == "" {
		return result, ErrOpenAIPassengerMeaningV1EmptyOutput
	}
	validation := validateOpenAIPassengerMeaningV1Payload([]byte(rawOutput))
	result.Proposal = validation.Proposal
	result.ProposalParseable = validation.ProposalParseable
	result.SchemaValid = validation.Valid
	result.SchemaReasonCodes = append([]string(nil), validation.ReasonCodes...)
	if !validation.Valid {
		return result, fmt.Errorf("%w: %s", ErrOpenAIPassengerMeaningV1InvalidOutput, strings.Join(validation.ReasonCodes, ","))
	}
	return result, nil
}

func (r *OpenAIPassengerMeaningV1Runner) buildRequestPayload(input OpenAIPassengerMeaningV1RunInput) map[string]interface{} {
	return map[string]interface{}{
		"model":        r.model,
		"instructions": buildOpenAIPassengerMeaningV1SystemPrompt(),
		"input":        buildOpenAIPassengerMeaningV1CompactInput(input),
		"store":        false,
		"tools":        []interface{}{},
		"tool_choice":  "none",
		"text": map[string]interface{}{
			"format": openAIPassengerClarificationMeaningV1JSONSchema(),
		},
	}
}

type openAIPassengerMeaningV1PayloadValidation struct {
	Proposal          PassengerClarificationMeaningV1
	ProposalParseable bool
	Valid             bool
	ReasonCodes       []string
}

func validateOpenAIPassengerMeaningV1Payload(payload []byte) openAIPassengerMeaningV1PayloadValidation {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(payload, &root); err != nil || root == nil {
		return openAIPassengerMeaningV1PayloadValidation{ReasonCodes: []string{"invalid_json_payload"}}
	}
	var proposal PassengerClarificationMeaningV1
	lenientErr := json.Unmarshal(payload, &proposal)
	parseable := lenientErr == nil && openAIPassengerMeaningV1RootHasProposal(root)
	reasons := validateOpenAIPassengerMeaningV1RequiredFields(root)

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		reasons = append(reasons, "invalid_json_shape")
	} else if err := decoder.Decode(&struct{}{}); err != io.EOF {
		reasons = append(reasons, "invalid_json_payload")
	}
	if !passengerMeaningV1PayloadEnumsAllowed(proposal) {
		reasons = append(reasons, "invalid_enum_value")
	}
	if proposal.Version != passengerClarificationMeaningV1Version {
		reasons = append(reasons, "invalid_version")
	}
	if len(strings.TrimSpace(proposal.SourceMessageID)) == 0 || len(proposal.SourceMessageID) > 128 ||
		len(strings.TrimSpace(proposal.SourcePromptEventID)) == 0 || len(proposal.SourcePromptEventID) > 256 {
		reasons = append(reasons, "invalid_source_identifier")
	}
	if math.IsNaN(proposal.Confidence) || math.IsInf(proposal.Confidence, 0) || proposal.Confidence < 0 || proposal.Confidence > 1 {
		reasons = append(reasons, "confidence_out_of_range")
	}
	if len(proposal.ChildUnder5.References) > 16 || len(proposal.MissingFields) > 4 ||
		len(proposal.ReasonCodes) == 0 || len(proposal.ReasonCodes) > 16 {
		reasons = append(reasons, "array_size_out_of_range")
	}
	if proposal.PassengerCount.Value != nil && (*proposal.PassengerCount.Value < 1 || *proposal.PassengerCount.Value > 99) {
		reasons = append(reasons, "passenger_count_out_of_range")
	}
	if proposal.ChildUnder5.Count != nil && (*proposal.ChildUnder5.Count < 0 || *proposal.ChildUnder5.Count > 99) {
		reasons = append(reasons, "child_count_out_of_range")
	}
	for _, reference := range proposal.ChildUnder5.References {
		if len(strings.TrimSpace(reference.ReferenceID)) == 0 || len(reference.ReferenceID) > 64 {
			reasons = append(reasons, "child_reference_out_of_range")
		}
		if reference.AgeValue != nil && (*reference.AgeValue < 0 || *reference.AgeValue > 1440) {
			reasons = append(reasons, "child_age_out_of_range")
		}
	}
	reasons = dedupeOpenAIStructuredReasons(reasons)
	return openAIPassengerMeaningV1PayloadValidation{
		Proposal:          proposal,
		ProposalParseable: parseable,
		Valid:             len(reasons) == 0,
		ReasonCodes:       reasons,
	}
}

func openAIPassengerMeaningV1RootHasProposal(root map[string]json.RawMessage) bool {
	for _, field := range []string{"version", "source_message_id", "source_prompt_event_id", "passenger_count", "child_under_5"} {
		value, ok := root[field]
		if !ok || openAIRawJSONIsNull(value) {
			return false
		}
	}
	return true
}

func validateOpenAIPassengerMeaningV1RequiredFields(root map[string]json.RawMessage) []string {
	reasons := validateOpenAIRawRequiredFields(root, "root", []string{
		"version", "source_message_id", "source_prompt_event_id", "passenger_count", "child_under_5",
		"correction", "needs_clarification", "missing_fields", "confidence", "reason_codes",
	})
	for _, object := range []struct {
		field    string
		required []string
		nullable []string
	}{
		{field: "passenger_count", required: []string{"status", "provenance"}, nullable: []string{"value"}},
		{field: "child_under_5", required: []string{"status", "references"}, nullable: []string{"count"}},
		{field: "correction", required: []string{"present", "replaces"}},
	} {
		raw, objectReasons := openAIRawRequiredObject(root, object.field)
		reasons = append(reasons, objectReasons...)
		if raw == nil {
			continue
		}
		reasons = append(reasons, validateOpenAIRawRequiredFields(raw, object.field, object.required)...)
		for _, field := range object.nullable {
			if _, ok := raw[field]; !ok {
				reasons = append(reasons, "missing_required_field:"+object.field+"."+field)
			}
		}
	}
	child, _ := openAIRawRequiredObject(root, "child_under_5")
	if child != nil {
		if value, ok := child["references"]; ok && !openAIRawJSONIsNull(value) {
			var references []map[string]json.RawMessage
			if err := json.Unmarshal(value, &references); err != nil {
				reasons = append(reasons, "invalid_required_array:child_under_5.references")
			} else {
				for index, reference := range references {
					path := fmt.Sprintf("child_under_5.references[%d]", index)
					reasons = append(reasons, validateOpenAIRawRequiredFields(reference, path, []string{"reference_id", "relation", "age_unit"})...)
					for _, nullable := range []string{"age_value", "under_5"} {
						if _, ok := reference[nullable]; !ok {
							reasons = append(reasons, "missing_required_field:"+path+"."+nullable)
						}
					}
				}
			}
		}
	}
	return dedupeOpenAIStructuredReasons(reasons)
}

func passengerMeaningV1PayloadEnumsAllowed(proposal PassengerClarificationMeaningV1) bool {
	if !passengerMeaningStatusAllowedV1(proposal.PassengerCount.Status) ||
		!passengerMeaningStatusAllowedV1(proposal.ChildUnder5.Status) ||
		!passengerMeaningCorrectionTargetAllowedV1(proposal.Correction.Replaces) {
		return false
	}
	switch proposal.PassengerCount.Provenance {
	case PassengerCountProvenanceUnknown,
		PassengerCountProvenanceSoloSpeaker,
		PassengerCountProvenanceAbsoluteTotal,
		PassengerCountProvenanceIncludesSpeakerComposition,
		PassengerCountProvenanceSubgroupOnly:
	default:
		return false
	}
	for _, field := range proposal.MissingFields {
		if !passengerMeaningMissingFieldAllowedV1(field) {
			return false
		}
	}
	if !passengerMeaningReasonCodesValidV1(proposal.ReasonCodes) {
		return false
	}
	for _, reference := range proposal.ChildUnder5.References {
		if !passengerChildRelationAllowedV1(reference.Relation) || !passengerChildAgeUnitAllowedV1(reference.AgeUnit) {
			return false
		}
	}
	return true
}
