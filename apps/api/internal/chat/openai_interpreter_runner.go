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
	ErrOpenAIStructuredInterpreterDisabled      = errors.New("openai structured interpreter disabled")
	ErrOpenAIStructuredInterpreterRequestFailed = errors.New("openai structured interpreter request failed")
	ErrOpenAIStructuredInterpreterEmptyOutput   = errors.New("openai structured interpreter returned empty output")
	ErrOpenAIStructuredInterpreterInvalidOutput = errors.New("openai structured interpreter returned invalid output")
)

type OpenAIStructuredInterpreter interface {
	Enabled() bool
	InterpretStructuredTurn(ctx context.Context, input OpenAIStructuredInterpreterRunInput) (OpenAIStructuredInterpreterRunResult, error)
}

type OpenAIResponsesDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type OpenAIStructuredInterpreterRunInput struct {
	StructuredInput     StructuredInterpreterInput
	LocalInterpretation StructuredInterpretation
	IdempotencyKey      string
}

type OpenAIStructuredInterpreterRunResult struct {
	Interpretation     StructuredInterpretation
	Validation         OpenAIStructuredInterpretationValidationResult
	Model              string
	ProviderResponseID string
	RawOutput          string
}

type OpenAIStructuredInterpreterRunner struct {
	baseURL string
	apiKey  string
	model   string
	client  OpenAIResponsesDoer
}

func NewOpenAIStructuredInterpreterRunner(cfg config.Config) *OpenAIStructuredInterpreterRunner {
	return &OpenAIStructuredInterpreterRunner{
		baseURL: strings.TrimSpace(cfg.OpenAIBaseURL),
		apiKey:  strings.TrimSpace(cfg.OpenAIAPIKey),
		model:   strings.TrimSpace(cfg.OpenAIModel),
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (r *OpenAIStructuredInterpreterRunner) Enabled() bool {
	return r != nil && r.apiKey != "" && r.model != "" && r.baseURL != ""
}

func (r *OpenAIStructuredInterpreterRunner) InterpretStructuredTurn(ctx context.Context, input OpenAIStructuredInterpreterRunInput) (OpenAIStructuredInterpreterRunResult, error) {
	if !r.Enabled() {
		return OpenAIStructuredInterpreterRunResult{}, ErrOpenAIStructuredInterpreterDisabled
	}

	requestPayload := r.buildRequestPayload(input)
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIStructuredInterpreterRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIStructuredInterpreterRequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if trimmed := strings.TrimSpace(input.IdempotencyKey); trimmed != "" {
		req.Header.Set("X-Client-Request-Id", trimmed)
	}

	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIStructuredInterpreterRequestFailed, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIStructuredInterpreterRequestFailed, err)
	}

	if resp.StatusCode >= 300 {
		errorBody := compactOpenAIErrorBody(responseBody)
		if errorBody != "" {
			return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: status %d %s", ErrOpenAIStructuredInterpreterRequestFailed, resp.StatusCode, errorBody)
		}
		return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: status %d", ErrOpenAIStructuredInterpreterRequestFailed, resp.StatusCode)
	}

	responsePayload := map[string]interface{}{}
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &responsePayload); err != nil {
			return OpenAIStructuredInterpreterRunResult{}, fmt.Errorf("%w: %v", ErrOpenAIStructuredInterpreterInvalidOutput, err)
		}
	}

	rawOutput := strings.TrimSpace(extractOpenAIResponseText(responsePayload))
	result := OpenAIStructuredInterpreterRunResult{
		Model:              r.model,
		ProviderResponseID: strings.TrimSpace(asString(responsePayload["id"])),
		RawOutput:          rawOutput,
	}
	if rawOutput == "" {
		return result, ErrOpenAIStructuredInterpreterEmptyOutput
	}

	validation := validateOpenAIStructuredInterpretationPayload(input.StructuredInput, []byte(rawOutput))
	result.Validation = validation
	if !validation.Valid {
		return result, fmt.Errorf("%w: %s", ErrOpenAIStructuredInterpreterInvalidOutput, strings.Join(validation.Reasons, ","))
	}

	result.Interpretation = validation.Interpretation
	return result, nil
}

func (r *OpenAIStructuredInterpreterRunner) buildRequestPayload(input OpenAIStructuredInterpreterRunInput) map[string]interface{} {
	return map[string]interface{}{
		"model":        r.model,
		"instructions": buildOpenAIStructuredInterpreterSystemPrompt(),
		"input": buildOpenAIStructuredInterpreterCompactInput(
			input.StructuredInput,
			input.LocalInterpretation,
		),
		"store": false,
		"tools": []interface{}{},
		"text": map[string]interface{}{
			"format": openAIStructuredInterpretationJSONSchema(),
		},
	}
}
