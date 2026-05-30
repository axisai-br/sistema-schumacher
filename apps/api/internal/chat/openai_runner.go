package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"time"
)

var (
	ErrOpenAIRunnerNotConfigured = errors.New("openai runner not configured")
	ErrOpenAIRunFailed           = errors.New("openai agent run failed")
	ErrOpenAIEmptyReply          = errors.New("openai agent returned empty reply")
)

const maxInlineImageBytes = 8 * 1024 * 1024

type OpenAIRunner struct {
	baseURL           string
	apiKey            string
	model             string
	visionModel       string
	continuityEnabled bool
	client            *http.Client
}

func compactOpenAIErrorBody(body []byte) string {
	/* Sanitize the message error that comes from OpenAI */

	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}

	parsed := map[string]interface{}{}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if errMap, ok := parsed["error"].(map[string]interface{}); ok {
			message := strings.TrimSpace(asString(errMap["message"]))
			errType := strings.TrimSpace(asString(errMap["type"]))
			code := strings.TrimSpace(asString(errMap["code"]))

			parts := []string{}
			if errType != "" {
				parts = append(parts, "type="+errType)
			}
			if code != "" {
				parts = append(parts, "code="+code)
			}
			if message != "" {
				parts = append(parts, "message="+message)
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}

		}
	}

	if len(text) > 1500 {
		return text[:1500]
	}
	return text
}

func NewOpenAIRunner(cfg config.Config) *OpenAIRunner {
	return &OpenAIRunner{
		baseURL:           "https://api.openai.com/v1",
		apiKey:            strings.TrimSpace(cfg.OpenAIAPIKey),
		model:             strings.TrimSpace(cfg.OpenAIModel),
		visionModel:       strings.TrimSpace(cfg.OpenAIVisionModel),
		continuityEnabled: cfg.ChatOpenAIContinuityEnabled,
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (r *OpenAIRunner) Enabled() bool {
	return r != nil && r.apiKey != "" && r.model != ""
}

func (r *OpenAIRunner) Run(ctx context.Context, input RunAgentInput) (RunAgentResult, error) {
	if !r.Enabled() {
		return RunAgentResult{}, ErrOpenAIRunnerNotConfigured
	}

	requestPayload := r.buildRequestPayload(ctx, input, true)
	firstAttemptUsedPreviousResponseID := openAIRequestUsesPreviousResponseID(requestPayload)
	result, err := r.runRequest(ctx, requestPayload, input.IdempotencyKey)
	if err == nil {
		return r.finalizeRunAgentResult(result, requestPayload, firstAttemptUsedPreviousResponseID, false, false), nil
	}

	if r.shouldRetryWithoutContinuity(input.Session, requestPayload, err) {
		retryPayload := r.buildRequestPayload(ctx, input, false)
		retryResult, retryErr := r.runRequest(ctx, retryPayload, openAIRetryIdempotencyKey(input.IdempotencyKey))
		if retryErr == nil {
			return r.finalizeRunAgentResult(retryResult, retryPayload, firstAttemptUsedPreviousResponseID, firstAttemptUsedPreviousResponseID, true), nil
		}
		return RunAgentResult{}, retryErr
	}

	if len(input.CurrentTurnMedia) == 0 {
		return RunAgentResult{}, err
	}

	fallbackPayload := map[string]interface{}{
		"model":        r.model,
		"instructions": input.SystemPrompt,
		"input":        input.UserPrompt,
	}
	fallbackResult, fallbackErr := r.runRequest(ctx, fallbackPayload, input.IdempotencyKey)
	if fallbackErr != nil {
		return RunAgentResult{}, fallbackErr
	}
	return r.finalizeRunAgentResult(fallbackResult, fallbackPayload, false, false, false), nil
}

func (r *OpenAIRunner) requestModel(input RunAgentInput) string {
	if len(input.CurrentTurnMedia) > 0 {
		if model := strings.TrimSpace(r.visionModel); model != "" {
			return model
		}
	}
	return r.model
}

func (r *OpenAIRunner) buildRequestPayload(ctx context.Context, input RunAgentInput, allowContinuity bool) map[string]interface{} {
	model := r.requestModel(input)
	payload := map[string]interface{}{
		"model":        model,
		"instructions": input.SystemPrompt,
		"input":        buildOpenAIInputContent(ctx, r.client, input),
	}

	if len(input.TextFormat) > 0 {
		payload["text"] = map[string]interface{}{
			"format": input.TextFormat,
		}
	}

	continuity := readOpenAIContinuityMetadata(input.Session.Metadata)
	if allowContinuity && r.continuityEnabled {
		if continuity.ProviderResponseID != "" && openAIModelsCompatible(model, continuity.ProviderModel) {
			payload["previous_response_id"] = continuity.ProviderResponseID
			return payload
		}
		if continuity.ProviderConversationID != "" && openAIModelsCompatible(model, continuity.ProviderModel) {
			payload["conversation"] = map[string]interface{}{"id": continuity.ProviderConversationID}
			return payload
		}
	}
	return payload
}

func (r *OpenAIRunner) shouldRetryWithoutContinuity(session Session, requestPayload map[string]interface{}, err error) bool {
	if !r.continuityEnabled || !shouldRetryOpenAIWithoutContinuity(err) {
		return false
	}
	return openAIRequestUsesContinuity(requestPayload)
}

func (r *OpenAIRunner) finalizeRunAgentResult(result RunAgentResult, requestPayload map[string]interface{}, previousResponseIDUsed bool, previousResponseIDFailed bool, retryWithoutPreviousResponseID bool) RunAgentResult {
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

func (r *OpenAIRunner) runRequest(ctx context.Context, requestPayload map[string]interface{}, idempotencyKey string) (RunAgentResult, error) {
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return RunAgentResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.baseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return RunAgentResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if trimmed := strings.TrimSpace(idempotencyKey); trimmed != "" {
		req.Header.Set("X-Client-Request-Id", trimmed)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return RunAgentResult{}, fmt.Errorf("%w: %v", ErrOpenAIRunFailed, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return RunAgentResult{}, fmt.Errorf("%w: %v", ErrOpenAIRunFailed, err)
	}

	responsePayload := map[string]interface{}{}
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &responsePayload)
	}
	if resp.StatusCode >= 300 {
		errorBody := compactOpenAIErrorBody(responseBody)
		if errorBody != "" {
			return RunAgentResult{}, fmt.Errorf(
				"%w: status %d %s",
				ErrOpenAIRunFailed,
				resp.StatusCode,
				errorBody,
			)
		}
		return RunAgentResult{}, fmt.Errorf("%w: status %d", ErrOpenAIRunFailed, resp.StatusCode)
	}

	replyText := strings.TrimSpace(extractOpenAIResponseText(responsePayload))
	if replyText == "" {
		return RunAgentResult{}, ErrOpenAIEmptyReply
	}

	return RunAgentResult{
		ReplyText:          replyText,
		Model:              strings.TrimSpace(asString(requestPayload["model"])),
		ProviderResponseID: strings.TrimSpace(asString(responsePayload["id"])),
		RequestPayload:     requestPayload,
		ResponsePayload:    responsePayload,
	}, nil
}

func buildOpenAIInputContent(ctx context.Context, client *http.Client, input RunAgentInput) interface{} {
	content := []map[string]interface{}{
		{
			"type": "input_text",
			"text": input.UserPrompt,
		},
	}
	for _, item := range input.CurrentTurnMedia {
		switch strings.ToUpper(strings.TrimSpace(item.Kind)) {
		case "PDF":
			fileInput := buildOpenAIFileInput(item)
			if fileInput != nil {
				content = append(content, fileInput)
			}
		case "IMAGE":
			imageURL := resolveOpenAIImageURL(ctx, client, item)
			if imageURL == "" {
				continue
			}
			content = append(content, map[string]interface{}{
				"type":      "input_image",
				"image_url": imageURL,
				"detail":    "high",
			})
		}
	}
	if len(content) == 1 {
		return input.UserPrompt
	}
	return []map[string]interface{}{
		{
			"role":    "user",
			"content": content,
		},
	}
}

func buildOpenAIFileInput(item AgentMediaInput) map[string]interface{} {
	url := strings.TrimSpace(item.URL)
	if url == "" {
		return nil
	}

	filename := strings.TrimSpace(item.FileName)
	if filename == "" {
		filename = "documento.pdf"
	}

	if strings.HasPrefix(strings.ToLower(url), "data:application/pdf") {
		return map[string]interface{}{
			"type":      "input_file",
			"filename":  filename,
			"file_data": url,
		}
	}

	if isHTTPURL(url) {
		return map[string]interface{}{
			"type":     "input_file",
			"filename": filename,
			"file_url": url,
		}
	}

	return nil
}

func resolveOpenAIImageURL(ctx context.Context, client *http.Client, item AgentMediaInput) string {
	url := strings.TrimSpace(item.URL)
	if url == "" {
		log.Printf("openai image resolve skipped reason=empty_url kind=%s mime=%s", item.Kind, item.MimeType)
		return ""
	}
	if strings.HasPrefix(strings.ToLower(url), "data:") {
		log.Printf("openai image resolve ok source=data_url kind=%s mime=%s", item.Kind, item.MimeType)
		return url
	}
	if client == nil || !isHTTPURL(url) {
		log.Printf("openai image resolve passthrough reason=no_http_client_or_non_http kind=%s mime=%s url_present=%t", item.Kind, item.MimeType, url != "")
		return url
	}
	if dataURL, err := fetchImageAsDataURL(ctx, client, url, item.MimeType); err == nil {
		log.Printf("openai image resolve ok source=fetched_data_url kind=%s mime=%s", item.Kind, item.MimeType)
		return dataURL
	} else {
		log.Printf("openai image resolve fetch_failed kind=%s mime=%s error=%v", item.Kind, item.MimeType, err)
	}
	return url
}

func fetchImageAsDataURL(ctx context.Context, client *http.Client, url string, mimeType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxInlineImageBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", errors.New("empty image body")
	}
	if len(body) > maxInlineImageBytes {
		return "", errors.New("image too large to inline")
	}

	resolvedMime := strings.TrimSpace(mimeType)
	if resolvedMime == "" {
		resolvedMime = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	}
	if resolvedMime == "" {
		resolvedMime = http.DetectContentType(body)
	}
	log.Printf(
		"openai image fetch ok mime=%s bytes=%d",
		resolvedMime,
		len(body),
	)
	return "data:" + resolvedMime + ";base64," + base64.StdEncoding.EncodeToString(body), nil
}

func extractOpenAIResponseText(payload map[string]interface{}) string {
	if text := strings.TrimSpace(asString(payload["output_text"])); text != "" {
		return text
	}

	rawOutput, ok := payload["output"].([]interface{})
	if !ok {
		return ""
	}
	parts := make([]string, 0, 2)
	for _, entry := range rawOutput {
		message, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		contentItems, ok := message["content"].([]interface{})
		if !ok {
			continue
		}
		for _, rawContent := range contentItems {
			content, ok := rawContent.(map[string]interface{})
			if !ok {
				continue
			}
			if text := strings.TrimSpace(asString(content["text"])); text != "" {
				parts = append(parts, text)
				continue
			}
			if text := strings.TrimSpace(asString(content["output_text"])); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
