package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"time"
)

var (
	ErrOpenAIJSONRunnerNotConfigured = errors.New("openai json runner not configured")
	ErrOpenAIJSONEmptyOutput         = errors.New("openai json runner returned empty output")
	ErrOpenAIJSONInvalidOutput       = errors.New("openai json runner returned invalid output")
)

type AgentJSONDecisionRunner interface {
	Enabled() bool
	RunIntentDecision(ctx context.Context, input RunJSONDecisionInput) (IntentDecisionJSON, OpenAIJSONRunResult, error)
}

type RunJSONDecisionInput struct {
	SystemPrompt   string
	CompactInput   string
	SchemaName     string
	Schema         map[string]interface{}
	IdempotencyKey string
	Session        Session
}

type OpenAIJSONRunResult struct {
	Model                  string
	ProviderResponseID     string
	ProviderConversationID string
	RequestPayload         map[string]interface{}
	ResponsePayload        map[string]interface{}
	RawOutput              string
}

type OpenAIJSONRunner struct {
	baseURL           string
	apiKey            string
	model             string
	continuityEnabled bool
	client            *http.Client
}

func NewOpenAIJSONRunner(cfg config.Config) *OpenAIJSONRunner {
	return &OpenAIJSONRunner{
		baseURL:           "https://api.openai.com/v1",
		apiKey:            strings.TrimSpace(cfg.OpenAIAPIKey),
		model:             strings.TrimSpace(cfg.OpenAIModel),
		continuityEnabled: cfg.ChatOpenAIContinuityEnabled,
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (r *OpenAIJSONRunner) Enabled() bool {
	return r != nil && r.apiKey != "" && r.model != ""
}

func (r *OpenAIJSONRunner) RunIntentDecision(ctx context.Context, input RunJSONDecisionInput) (IntentDecisionJSON, OpenAIJSONRunResult, error) {
	if strings.TrimSpace(input.SchemaName) == "" {
		input.SchemaName = "intent_decision"
	}
	if input.Schema == nil {
		input.Schema = intentDecisionJSONSchema()
	}
	var decision IntentDecisionJSON
	result, err := r.Run(ctx, input, &decision)
	if err != nil {
		return IntentDecisionJSON{}, result, err
	}
	return decision, result, nil
}

func (r *OpenAIJSONRunner) Run(ctx context.Context, input RunJSONDecisionInput, output interface{}) (OpenAIJSONRunResult, error) {
	if !r.Enabled() {
		return OpenAIJSONRunResult{}, ErrOpenAIJSONRunnerNotConfigured
	}
	if output == nil {
		return OpenAIJSONRunResult{}, fmt.Errorf("%w: nil output", ErrOpenAIJSONInvalidOutput)
	}

	schemaName := strings.TrimSpace(input.SchemaName)
	if schemaName == "" {
		schemaName = "json_decision"
	}
	requestPayload := r.buildRequestPayload(input, schemaName, true)
	firstAttemptUsedPreviousResponseID := openAIRequestUsesPreviousResponseID(requestPayload)
	result, err := r.runRequest(ctx, requestPayload, input.IdempotencyKey, output)
	if err == nil {
		return r.finalizeResult(result, requestPayload, firstAttemptUsedPreviousResponseID, false, false), nil
	}

	if r.shouldRetryWithoutContinuity(input.Session, requestPayload, err) {
		retryPayload := r.buildRequestPayload(input, schemaName, false)
		retryResult, retryErr := r.runRequest(ctx, retryPayload, openAIRetryIdempotencyKey(input.IdempotencyKey), output)
		if retryErr == nil {
			return r.finalizeResult(retryResult, retryPayload, firstAttemptUsedPreviousResponseID, firstAttemptUsedPreviousResponseID, true), nil
		}
		return OpenAIJSONRunResult{}, retryErr
	}

	return OpenAIJSONRunResult{}, err
}

func (r *OpenAIJSONRunner) buildRequestPayload(input RunJSONDecisionInput, schemaName string, allowContinuity bool) map[string]interface{} {
	payload := map[string]interface{}{
		"model":        r.model,
		"instructions": input.SystemPrompt,
		"input":        input.CompactInput,
		"temperature":  0,
		"text": map[string]interface{}{
			"format": map[string]interface{}{
				"type":   "json_schema",
				"name":   schemaName,
				"strict": true,
				"schema": input.Schema,
			},
		},
	}

	continuity := readOpenAIContinuityMetadata(input.Session.Metadata)
	if allowContinuity && r.continuityEnabled {
		if continuity.ProviderResponseID != "" && openAIModelsCompatible(r.model, continuity.ProviderModel) {
			payload["previous_response_id"] = continuity.ProviderResponseID
			return payload
		}
		if continuity.ProviderConversationID != "" && openAIModelsCompatible(r.model, continuity.ProviderModel) {
			payload["conversation"] = map[string]interface{}{"id": continuity.ProviderConversationID}
			return payload
		}
	}
	return payload
}

func (r *OpenAIJSONRunner) shouldRetryWithoutContinuity(session Session, requestPayload map[string]interface{}, err error) bool {
	if !r.continuityEnabled || !shouldRetryOpenAIWithoutContinuity(err) {
		return false
	}
	return openAIRequestUsesContinuity(requestPayload)
}

func (r *OpenAIJSONRunner) finalizeResult(result OpenAIJSONRunResult, requestPayload map[string]interface{}, previousResponseIDUsed bool, previousResponseIDFailed bool, retryWithoutPreviousResponseID bool) OpenAIJSONRunResult {
	if result.RequestPayload == nil {
		result.RequestPayload = map[string]interface{}{}
	}
	if result.ResponsePayload == nil {
		result.ResponsePayload = map[string]interface{}{}
	}
	for key, value := range requestPayload {
		result.RequestPayload[key] = value
	}
	result.RequestPayload["previous_response_id_used"] = previousResponseIDUsed
	result.RequestPayload["previous_response_id_failed"] = previousResponseIDFailed
	result.RequestPayload["retry_without_previous_response_id"] = retryWithoutPreviousResponseID
	result.ResponsePayload["previous_response_id_used"] = previousResponseIDUsed
	result.ResponsePayload["previous_response_id_failed"] = previousResponseIDFailed
	result.ResponsePayload["retry_without_previous_response_id"] = retryWithoutPreviousResponseID
	result.ProviderConversationID = firstNonEmpty(
		result.ProviderConversationID,
		extractOpenAIConversationID(result.ResponsePayload),
	)
	if result.ProviderConversationID != "" {
		result.RequestPayload["provider_conversation_id"] = result.ProviderConversationID
		result.ResponsePayload["provider_conversation_id"] = result.ProviderConversationID
	}
	if result.ProviderResponseID != "" {
		result.RequestPayload["provider_response_id"] = result.ProviderResponseID
		result.ResponsePayload["provider_response_id"] = result.ProviderResponseID
	}
	return result
}

func (r *OpenAIJSONRunner) runRequest(ctx context.Context, requestPayload map[string]interface{}, idempotencyKey string, output interface{}) (OpenAIJSONRunResult, error) {
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return OpenAIJSONRunResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return OpenAIJSONRunResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if trimmed := strings.TrimSpace(idempotencyKey); trimmed != "" {
		req.Header.Set("X-Client-Request-Id", trimmed)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return OpenAIJSONRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIRunFailed, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return OpenAIJSONRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIRunFailed, err)
	}

	responsePayload := map[string]interface{}{}
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &responsePayload)
	}
	if resp.StatusCode >= 300 {
		errorBody := compactOpenAIErrorBody(responseBody)
		if errorBody != "" {
			return OpenAIJSONRunResult{}, fmt.Errorf("%w: status %d %s", ErrOpenAIRunFailed, resp.StatusCode, errorBody)
		}
		return OpenAIJSONRunResult{}, fmt.Errorf("%w: status %d", ErrOpenAIRunFailed, resp.StatusCode)
	}

	rawOutput := strings.TrimSpace(extractOpenAIResponseText(responsePayload))
	result := OpenAIJSONRunResult{
		Model:              strings.TrimSpace(asString(requestPayload["model"])),
		ProviderResponseID: strings.TrimSpace(asString(responsePayload["id"])),
		RequestPayload:     requestPayload,
		ResponsePayload:    responsePayload,
		RawOutput:          rawOutput,
	}
	if rawOutput == "" {
		return result, ErrOpenAIJSONEmptyOutput
	}
	if err := json.Unmarshal([]byte(rawOutput), output); err != nil {
		return result, fmt.Errorf("%w: %v", ErrOpenAIJSONInvalidOutput, err)
	}
	return result, nil
}
