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
	ErrOpenAITravelQueryV2Disabled      = errors.New("openai travel query v2 disabled")
	ErrOpenAITravelQueryV2RequestFailed = errors.New("openai travel query v2 request failed")
	ErrOpenAITravelQueryV2EmptyOutput   = errors.New("openai travel query v2 returned empty output")
	ErrOpenAITravelQueryV2InvalidOutput = errors.New("openai travel query v2 returned invalid output")
	ErrOpenAITravelQueryV2Refusal       = errors.New("openai travel query v2 refused output")
)

type OpenAITravelQueryV2Interpreter interface {
	Enabled() bool
	InterpretTravelQueryV2(ctx context.Context, input OpenAITravelQueryV2RunInput) (OpenAITravelQueryV2RunResult, error)
}

type OpenAITravelQueryV2RunInput struct {
	StructuredInput          StructuredInterpreterInput
	ActivePrompt             ActivePromptContext
	AvailabilityFacts        TravelQueryAvailabilityFactsV2
	LocationCatalog          []TravelQueryLocationEvidenceV2
	ExistingDecisionStrength DecisionStrength
	IdempotencyKey           string
}

type OpenAITravelQueryV2RunResult struct {
	Proposal           TravelQueryMeaningV2
	ProposalParseable  bool
	SchemaValid        bool
	SchemaReasonCodes  []string
	Model              string
	ProviderResponseID string
}

type OpenAITravelQueryV2Runner struct {
	baseURL string
	apiKey  string
	model   string
	client  OpenAIResponsesDoer
}

func NewOpenAITravelQueryV2Runner(cfg config.Config) *OpenAITravelQueryV2Runner {
	return &OpenAITravelQueryV2Runner{
		baseURL: strings.TrimSpace(cfg.OpenAIBaseURL),
		apiKey:  strings.TrimSpace(cfg.OpenAIAPIKey),
		model:   strings.TrimSpace(cfg.OpenAIModel),
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (r *OpenAITravelQueryV2Runner) Enabled() bool {
	return r != nil && r.apiKey != "" && r.model != "" && r.baseURL != ""
}

func (r *OpenAITravelQueryV2Runner) InterpretTravelQueryV2(ctx context.Context, input OpenAITravelQueryV2RunInput) (OpenAITravelQueryV2RunResult, error) {
	if !r.Enabled() {
		return OpenAITravelQueryV2RunResult{}, ErrOpenAITravelQueryV2Disabled
	}

	requestPayload := r.buildRequestPayload(input)
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return OpenAITravelQueryV2RunResult{}, fmt.Errorf("%w: %v", ErrOpenAITravelQueryV2RequestFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return OpenAITravelQueryV2RunResult{}, fmt.Errorf("%w: %v", ErrOpenAITravelQueryV2RequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey := strings.TrimSpace(input.IdempotencyKey); idempotencyKey != "" {
		req.Header.Set("X-Client-Request-Id", idempotencyKey)
	}

	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return OpenAITravelQueryV2RunResult{}, fmt.Errorf("%w: %v", ErrOpenAITravelQueryV2RequestFailed, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return OpenAITravelQueryV2RunResult{}, fmt.Errorf("%w: %v", ErrOpenAITravelQueryV2RequestFailed, err)
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		errorBody := compactOpenAIErrorBody(responseBody)
		if errorBody != "" {
			return OpenAITravelQueryV2RunResult{}, fmt.Errorf("%w: status %d %s", ErrOpenAITravelQueryV2RequestFailed, response.StatusCode, errorBody)
		}
		return OpenAITravelQueryV2RunResult{}, fmt.Errorf("%w: status %d", ErrOpenAITravelQueryV2RequestFailed, response.StatusCode)
	}

	responsePayload := map[string]interface{}{}
	if len(responseBody) == 0 || json.Unmarshal(responseBody, &responsePayload) != nil {
		return OpenAITravelQueryV2RunResult{}, ErrOpenAITravelQueryV2InvalidOutput
	}
	result := OpenAITravelQueryV2RunResult{
		Model:              r.model,
		ProviderResponseID: strings.TrimSpace(asString(responsePayload["id"])),
	}
	if openAITravelQueryV2ResponseRefused(responsePayload) {
		return result, ErrOpenAITravelQueryV2Refusal
	}
	rawOutput := strings.TrimSpace(extractOpenAIResponseText(responsePayload))
	if rawOutput == "" {
		return result, ErrOpenAITravelQueryV2EmptyOutput
	}

	validation := validateOpenAITravelQueryMeaningV2Payload([]byte(rawOutput))
	result.Proposal = validation.Proposal
	result.ProposalParseable = validation.ProposalParseable
	result.SchemaValid = validation.Valid
	result.SchemaReasonCodes = append([]string(nil), validation.ReasonCodes...)
	if !validation.Valid {
		return result, fmt.Errorf("%w: %s", ErrOpenAITravelQueryV2InvalidOutput, strings.Join(validation.ReasonCodes, ","))
	}
	return result, nil
}

func (r *OpenAITravelQueryV2Runner) buildRequestPayload(input OpenAITravelQueryV2RunInput) map[string]interface{} {
	return map[string]interface{}{
		"model":        r.model,
		"instructions": buildOpenAITravelQueryV2SystemPrompt(),
		"input":        buildOpenAITravelQueryV2CompactInput(input),
		"store":        false,
		"tools":        []interface{}{},
		"tool_choice":  "none",
		"text": map[string]interface{}{
			"format": openAITravelQueryMeaningV2JSONSchema(),
		},
	}
}

type openAITravelQueryV2PayloadValidation struct {
	Proposal          TravelQueryMeaningV2
	ProposalParseable bool
	Valid             bool
	ReasonCodes       []string
}

func validateOpenAITravelQueryMeaningV2Payload(payload []byte) openAITravelQueryV2PayloadValidation {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(payload, &root); err != nil || root == nil {
		return openAITravelQueryV2PayloadValidation{ReasonCodes: []string{"invalid_json_payload"}}
	}

	var candidate OpenAITravelQueryMeaningV2JSON
	lenientErr := json.Unmarshal(payload, &candidate)
	proposal := openAITravelQueryMeaningV2JSONToDomain(candidate)
	parseable := lenientErr == nil && openAITravelQueryV2RootHasProposal(root)
	reasons := validateOpenAITravelQueryV2RequiredFields(root)

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		reasons = append(reasons, "invalid_json_shape")
	} else if err := decoder.Decode(&struct{}{}); err != io.EOF {
		reasons = append(reasons, "invalid_json_payload")
	}
	proposal = openAITravelQueryMeaningV2JSONToDomain(candidate)
	if !travelQueryMeaningV2EnumsAllowed(proposal) {
		reasons = append(reasons, "invalid_enum_value")
	}
	for _, field := range proposal.MissingFields {
		if !travelQueryMissingFieldAllowed(strings.TrimSpace(field)) {
			reasons = append(reasons, "invalid_enum_value")
			break
		}
	}
	if math.IsNaN(proposal.Confidence) || math.IsInf(proposal.Confidence, 0) || proposal.Confidence < 0 || proposal.Confidence > 1 {
		reasons = append(reasons, "confidence_out_of_range")
	}
	if proposal.OptionReference.Index < 0 {
		reasons = append(reasons, "option_index_out_of_range")
	}
	reasons = dedupeOpenAIStructuredReasons(reasons)
	return openAITravelQueryV2PayloadValidation{
		Proposal:          proposal,
		ProposalParseable: parseable,
		Valid:             len(reasons) == 0,
		ReasonCodes:       reasons,
	}
}

func openAITravelQueryV2RootHasProposal(root map[string]json.RawMessage) bool {
	for _, field := range []string{"intent", "turn_meaning"} {
		value, ok := root[field]
		if !ok || openAIRawJSONIsNull(value) {
			return false
		}
	}
	return true
}

func validateOpenAITravelQueryV2RequiredFields(root map[string]json.RawMessage) []string {
	reasons := validateOpenAIRawRequiredFields(root, "root", []string{
		"intent",
		"turn_meaning",
		"mentioned_locations",
		"date_preference",
		"option_reference",
		"route_coverage",
		"seat_request",
		"institutional_topic",
		"needs_clarification",
		"missing_fields",
		"confidence",
		"reasons",
	})
	for _, nullable := range []string{"origin", "destination"} {
		if _, ok := root[nullable]; !ok {
			reasons = append(reasons, "missing_required_field:root."+nullable)
		}
	}

	for _, object := range []struct {
		field    string
		required []string
	}{
		{field: "date_preference", required: []string{"mode", "exact_date"}},
		{field: "option_reference", required: []string{"kind", "index", "date"}},
		{field: "route_coverage", required: []string{"query_location", "mode"}},
	} {
		raw, objectReasons := openAIRawRequiredObject(root, object.field)
		reasons = append(reasons, objectReasons...)
		if raw != nil {
			reasons = append(reasons, validateOpenAIRawRequiredFields(raw, object.field, object.required)...)
		}
	}
	for _, field := range []string{"origin", "destination"} {
		value, ok := root[field]
		if !ok || openAIRawJSONIsNull(value) {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(value, &raw); err != nil || raw == nil {
			reasons = append(reasons, "invalid_required_object:"+field)
			continue
		}
		reasons = append(reasons, validateOpenAIRawRequiredFields(raw, field, []string{"name", "role"})...)
	}
	if value, ok := root["mentioned_locations"]; ok && !openAIRawJSONIsNull(value) {
		var locations []map[string]json.RawMessage
		if err := json.Unmarshal(value, &locations); err != nil {
			reasons = append(reasons, "invalid_required_array:mentioned_locations")
		} else {
			for index, location := range locations {
				path := fmt.Sprintf("mentioned_locations[%d]", index)
				reasons = append(reasons, validateOpenAIRawRequiredFields(location, path, []string{"name", "role"})...)
			}
		}
	}
	return dedupeOpenAIStructuredReasons(reasons)
}

func openAITravelQueryMeaningV2JSONToDomain(candidate OpenAITravelQueryMeaningV2JSON) TravelQueryMeaningV2 {
	proposal := TravelQueryMeaningV2{
		Intent:             candidate.Intent,
		TurnMeaning:        candidate.TurnMeaning,
		MentionedLocations: make([]LocationMeaning, 0, len(candidate.MentionedLocations)),
		DatePreference: DatePreference{
			Mode:      candidate.DatePreference.Mode,
			ExactDate: strings.TrimSpace(candidate.DatePreference.ExactDate),
		},
		OptionReference: OptionReference{
			Kind:  candidate.OptionReference.Kind,
			Index: candidate.OptionReference.Index,
			Date:  strings.TrimSpace(candidate.OptionReference.Date),
		},
		RouteCoverage: RouteCoverageMeaning{
			QueryLocation: strings.TrimSpace(candidate.RouteCoverage.QueryLocation),
			Mode:          candidate.RouteCoverage.Mode,
		},
		SeatRequest:        candidate.SeatRequest,
		InstitutionalTopic: candidate.InstitutionalTopic,
		NeedsClarification: candidate.NeedsClarification,
		MissingFields:      normalizeOpenAIStructuredReasons(candidate.MissingFields),
		Confidence:         candidate.Confidence,
		Reasons:            normalizeOpenAIStructuredReasons(candidate.Reasons),
	}
	if candidate.Origin != nil {
		proposal.Origin = &LocationMeaning{Name: strings.TrimSpace(candidate.Origin.Name), Role: candidate.Origin.Role}
	}
	if candidate.Destination != nil {
		proposal.Destination = &LocationMeaning{Name: strings.TrimSpace(candidate.Destination.Name), Role: candidate.Destination.Role}
	}
	for _, location := range candidate.MentionedLocations {
		proposal.MentionedLocations = append(proposal.MentionedLocations, LocationMeaning{
			Name: strings.TrimSpace(location.Name),
			Role: location.Role,
		})
	}
	return proposal
}

func openAITravelQueryV2ResponseRefused(payload map[string]interface{}) bool {
	output, _ := payload["output"].([]interface{})
	for _, rawMessage := range output {
		message, _ := rawMessage.(map[string]interface{})
		content, _ := message["content"].([]interface{})
		for _, rawItem := range content {
			item, _ := rawItem.(map[string]interface{})
			if strings.EqualFold(strings.TrimSpace(asString(item["type"])), "refusal") || strings.TrimSpace(asString(item["refusal"])) != "" {
				return true
			}
		}
	}
	return false
}
