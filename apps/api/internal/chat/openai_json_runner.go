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
}

type OpenAIJSONRunResult struct {
	Model              string
	ProviderResponseID string
	RequestPayload     map[string]interface{}
	ResponsePayload    map[string]interface{}
	RawOutput          string
}

type OpenAIJSONRunner struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAIJSONRunner(cfg config.Config) *OpenAIJSONRunner {
	return &OpenAIJSONRunner{
		baseURL: "https://api.openai.com/v1",
		apiKey:  strings.TrimSpace(cfg.OpenAIAPIKey),
		model:   strings.TrimSpace(cfg.OpenAIModel),
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
	requestPayload := map[string]interface{}{
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
	if trimmed := strings.TrimSpace(input.IdempotencyKey); trimmed != "" {
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
