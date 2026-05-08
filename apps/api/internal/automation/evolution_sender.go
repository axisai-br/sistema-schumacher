package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"schumacher-tur/api/internal/chat"
	"schumacher-tur/api/internal/shared/config"
)

var (
	ErrEvolutionSenderNotConfigured = errors.New("evolution sender is not configured")
	ErrEvolutionSendFailed          = errors.New("evolution send text request failed")
	ErrEvolutionMissingProviderKey  = errors.New("evolution send text response missing provider message id")
)

type EvolutionSender struct {
	baseURL  string
	apiKey   string
	instance string
	client   *http.Client
}

func NewEvolutionSender(cfg config.Config) *EvolutionSender {
	return &EvolutionSender{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.EvolutionBaseURL), "/"),
		apiKey:   strings.TrimSpace(cfg.EvolutionAPIKey),
		instance: strings.TrimSpace(cfg.EvolutionInstance),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *EvolutionSender) Enabled() bool {
	return s != nil && s.baseURL != "" && s.apiKey != "" && s.instance != ""
}

func (s *EvolutionSender) SendReply(ctx context.Context, input chat.SendReplyInput) (chat.SendReplyResult, error) {
	if !s.Enabled() {
		return chat.SendReplyResult{}, ErrEvolutionSenderNotConfigured
	}

	number := normalizeEvolutionRecipient(input.Outbound.Recipient)
	if number == "" {
		number = normalizeEvolutionRecipient(input.Session.ContactKey)
	}
	if number == "" {
		return chat.SendReplyResult{}, fmt.Errorf("%w: missing recipient", ErrEvolutionSendFailed)
	}

	requestPayload := map[string]interface{}{}
	isMediaMessage := false
	if outboundPayloadHasMedia(input.Outbound.Payload) {
		mediaType := normalizeEvolutionMediaType(asString(input.Outbound.Payload["media_kind"]))
		if mediaType == "" {
			mediaType = "document"
		}
		mimeType := strings.TrimSpace(asString(input.Outbound.Payload["media_mime_type"]))
		if mimeType == "" {
			mimeType = defaultEvolutionMediaMIMEType(mediaType)
		}
		fileName := strings.TrimSpace(asString(input.Outbound.Payload["media_file_name"]))
		if fileName == "" {
			fileName = defaultEvolutionMediaFileName(mediaType)
		}
		caption := strings.TrimSpace(asString(input.Outbound.Payload["media_caption"]))
		if caption == "" {
			caption = strings.TrimSpace(input.Message.Body)
		}

		requestPayload = map[string]interface{}{
			"number":    number,
			"mediatype": mediaType,
			"mimetype":  mimeType,
			"media":     strings.TrimSpace(asString(input.Outbound.Payload["media_base64"])),
			"fileName":  fileName,
		}
		if caption != "" && mediaType != "audio" {
			requestPayload["caption"] = caption
		}
		isMediaMessage = true
	} else {
		text := strings.TrimSpace(input.Message.Body)
		if text == "" {
			text = strings.TrimSpace(asString(input.Outbound.Payload["body"]))
		}
		if text == "" {
			return chat.SendReplyResult{}, fmt.Errorf("%w: empty reply text", ErrEvolutionSendFailed)
		}

		requestPayload = map[string]interface{}{
			"number": number,
			"text":   text,
			"textMessage": map[string]interface{}{
				"text": text,
			},
			"delay":       1200,
			"linkPreview": false,
		}
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return chat.SendReplyResult{}, err
	}

	endpoint := s.baseURL + "/message/sendText/" + s.instance
	if isMediaMessage {
		endpoint = s.baseURL + "/message/sendMedia/" + s.instance
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return chat.SendReplyResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return chat.SendReplyResult{}, fmt.Errorf("%w: %v", ErrEvolutionSendFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return chat.SendReplyResult{}, fmt.Errorf("%w: could not read response body: %v", ErrEvolutionSendFailed, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return chat.SendReplyResult{}, fmt.Errorf("%w: status=%d body=%s", ErrEvolutionSendFailed, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	payload := map[string]interface{}{}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &payload); err != nil {
			return chat.SendReplyResult{}, fmt.Errorf("%w: invalid response body", ErrEvolutionSendFailed)
		}
	}

	providerMessageID := extractEvolutionProviderMessageID(payload)
	if providerMessageID == "" {
		return chat.SendReplyResult{}, ErrEvolutionMissingProviderKey
	}

	return chat.SendReplyResult{
		ProviderMessageID: providerMessageID,
		ProviderStatus:    strings.ToUpper(strings.TrimSpace(asString(payload["status"]))),
		Payload: map[string]interface{}{
			"delivery_mode":     deliveryModeForOutbound(input.Outbound.Payload),
			"provider":          "EVOLUTION",
			"provider_request":  requestPayload,
			"provider_response": payload,
		},
		SentAt: time.Now().UTC(),
	}, nil
}

func normalizeEvolutionRecipient(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "@"); idx >= 0 {
		value = value[:idx]
	}
	digits := make([]rune, 0, len(value))
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	return string(digits)
}

func extractEvolutionProviderMessageID(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if key, ok := payload["key"].(map[string]interface{}); ok {
		if id := strings.TrimSpace(asString(key["id"])); id != "" {
			return id
		}
	}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		if key, ok := data["key"].(map[string]interface{}); ok {
			if id := strings.TrimSpace(asString(key["id"])); id != "" {
				return id
			}
		}
	}
	return strings.TrimSpace(asString(payload["id"]))
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func deliveryModeForOutbound(payload map[string]interface{}) string {
	if strings.EqualFold(strings.TrimSpace(asString(payload["mode"])), "BOT_AUTO_REPLY") {
		return "BOT_AUTO_SEND"
	}
	return "MANUAL_CONTROLLED"
}

func outboundPayloadHasMedia(payload map[string]interface{}) bool {
	return strings.TrimSpace(asString(payload["media_base64"])) != ""
}

func normalizeEvolutionMediaType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "image":
		return "image"
	case "audio":
		return "audio"
	case "document":
		return "document"
	default:
		return ""
	}
}

func defaultEvolutionMediaMIMEType(mediaType string) string {
	switch mediaType {
	case "image":
		return "image/jpeg"
	case "audio":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}

func defaultEvolutionMediaFileName(mediaType string) string {
	switch mediaType {
	case "image":
		return "media.jpg"
	case "audio":
		return "audio.ogg"
	default:
		return "document.bin"
	}
}
