package automation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/chat"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidEvolutionPayload          = errors.New("invalid evolution payload")
	ErrMissingEvolutionChatKey          = errors.New("missing evolution remoteJid")
	ErrMissingEvolutionMessageID        = errors.New("missing evolution message id")
	ErrMissingEvolutionStatus           = errors.New("missing evolution status")
	ErrMissingEvolutionPresence         = errors.New("missing evolution presence status")
	ErrMissingJobName                   = errors.New("missing automation job name")
	ErrInvalidJobRunLimit               = errors.New("invalid automation job run limit")
	ErrInvalidChatBufferFlushLimit      = errors.New("invalid chat buffer flush limit")
	ErrInvalidChatAutoSendRetryLimit    = errors.New("invalid chat auto-send retry limit")
	ErrInvalidChatAutoSendRetryCooldown = errors.New("invalid chat auto-send retry cooldown_seconds")
	ErrInvalidBookingsExpireLimit       = errors.New("invalid bookings expire limit")
	ErrInvalidBookingsExpireHold        = errors.New("invalid bookings expire hold_minutes")
	ErrPaymentNotificationTarget        = errors.New("payment_id or booking_id is required")
)

type ChatIngestor interface {
	Ingest(ctx context.Context, input chat.IngestMessageInput) (chat.IngestMessageResult, error)
	UpdateMessage(ctx context.Context, input chat.UpdateMessageInput) (chat.Message, error)
	ApplyPresenceSignal(ctx context.Context, input chat.ApplyPresenceSignalInput) (chat.ApplyPresenceSignalResult, error)
	ListSessions(ctx context.Context, filter chat.ListSessionsFilter) ([]chat.Session, error)
	Reprocess(ctx context.Context, input chat.ReprocessInput) (chat.ReprocessResult, error)
	RetryDraftAutoSend(ctx context.Context, input chat.RetryDraftAutoSendInput) (chat.RetryDraftAutoSendResult, error)
	GetSessionsSummary(ctx context.Context, filter chat.ListSessionsFilter) (chat.SessionsSummary, error)
	QueueAutomationDraft(ctx context.Context, input chat.QueueAutomationDraftInput) (chat.QueueAutomationDraftResult, error)
}

type chatReviewAlertNotifier interface {
	Enabled() bool
	NotifyReviewAlert(ctx context.Context, payload ChatReviewAlertNotificationPayload) error
}

type paymentNotificationSource interface {
	Get(ctx context.Context, id string) (payments.Payment, error)
	List(ctx context.Context, filter payments.PaymentListFilter) ([]payments.Payment, error)
	GetBookingNotificationContext(ctx context.Context, bookingID string) (payments.PaymentNotificationContext, error)
}

type bookingExpirationSource interface {
	List(ctx context.Context, filter bookings.ListFilter) ([]bookings.BookingListItem, error)
	UpdateStatus(ctx context.Context, id string, status string) (bookings.BookingDetails, error)
}

type Service struct {
	store               Store
	chat                ChatIngestor
	cfg                 config.Config
	reviewAlertNotifier chatReviewAlertNotifier
	paymentSource       paymentNotificationSource
	bookingSource       bookingExpirationSource
}

func NewService(store Store, chatSvc ChatIngestor, cfg config.Config, deps ...interface{}) *Service {
	notifier := newChatReviewAlertNotifier(cfg)
	var paymentSource paymentNotificationSource
	var bookingSource bookingExpirationSource
	for _, dep := range deps {
		switch typed := dep.(type) {
		case chatReviewAlertNotifier:
			if typed != nil {
				notifier = typed
			}
		case paymentNotificationSource:
			if typed != nil {
				paymentSource = typed
			}
		case bookingExpirationSource:
			if typed != nil {
				bookingSource = typed
			}
		}
	}
	return &Service{
		store:               store,
		chat:                chatSvc,
		cfg:                 cfg,
		reviewAlertNotifier: notifier,
		paymentSource:       paymentSource,
		bookingSource:       bookingSource,
	}
}

const (
	maxEvolutionInlineImageBytes = 8 * 1024 * 1024
	maxEvolutionInlineAudioBytes = 25 * 1024 * 1024
	maxEvolutionInlinePDFBytes   = 10 * 1024 * 1024
)

func (s *Service) fetchEvolutionMediaDataURL(
	ctx context.Context,
	messageID string,
	remoteJID string,
	fromMe bool,
	mimeType string,
	maxBytes int,
) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.EvolutionBaseURL), "/")
	instance := strings.TrimSpace(s.cfg.EvolutionInstance)
	apiKey := strings.TrimSpace(s.cfg.EvolutionAPIKey)
	messageID = strings.TrimSpace(messageID)
	remoteJID = strings.TrimSpace(remoteJID)

	if baseURL == "" || instance == "" || apiKey == "" || messageID == "" || remoteJID == "" {
		return "", errors.New("missing evolution media config")
	}

	endpoint := baseURL + "/chat/getBase64FromMediaMessage/" + instance

	requestPayload := map[string]interface{}{
		"message": map[string]interface{}{
			"key": map[string]interface{}{
				"id":        messageID,
				"remoteJid": remoteJID,
				"fromMe":    fromMe,
			},
		},
		"convertToMp4": false,
	}

	requestBody, err := json.Marshal(requestPayload)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 45 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	limit := maxEvolutionInlineImageBytes * 2
	if maxBytes > 0 {
		limit = maxBytes * 2
	}
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf(
			"evolution get base64 failed status=%d body=%s",
			resp.StatusCode,
			truncateForLog(string(rawBody), 500),
		)
	}

	parsed := map[string]interface{}{}
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return "", err
	}

	base64Text := firstNonEmptyString(
		asStringPath(parsed, "base64"),
		asStringPath(parsed, "data", "base64"),
		asStringPath(parsed, "message", "base64"),
		asStringPath(parsed, "data", "message", "base64"),
		asStringPath(parsed, "data", "message", "base64Message"),
	)

	base64Text = stripDataURLPrefix(base64Text)
	if strings.TrimSpace(base64Text) == "" {
		return "", errors.New("evolution response missing base64")
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return "", fmt.Errorf("invalid evolution base64: %w", err)
	}
	if len(decoded) == 0 {
		return "", errors.New("empty evolution media")
	}
	if maxBytes <= 0 {
		maxBytes = maxEvolutionInlineImageBytes
	}
	if len(decoded) > maxBytes {
		return "", errors.New("evolution media too large")
	}

	resolvedMime := strings.TrimSpace(mimeType)
	if resolvedMime == "" {
		resolvedMime = http.DetectContentType(decoded)
	}

	return "data:" + resolvedMime + ";base64," + base64.StdEncoding.EncodeToString(decoded), nil
}

func (s *Service) transcribeOpenAIAudioFromDataURL(ctx context.Context, dataURL string, mimeType string, model string) (string, error) {
	dataURL = strings.TrimSpace(dataURL)
	if dataURL == "" {
		return "", errors.New("missing audio data url")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gpt-4o-mini-transcribe"
	}
	apiKey := strings.TrimSpace(s.cfg.OpenAIAPIKey)
	if apiKey == "" {
		return "", errors.New("missing openai api key")
	}

	base64Text := stripDataURLPrefix(dataURL)
	if strings.TrimSpace(base64Text) == "" {
		return "", errors.New("missing audio base64")
	}
	decoded, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return "", fmt.Errorf("invalid audio base64: %w", err)
	}
	if len(decoded) == 0 {
		return "", errors.New("empty audio media")
	}

	normalizedMime := normalizeAudioMimeType(mimeType)
	tmpFile, err := os.CreateTemp("", "schumacher-audio-*"+audioFileExtensionForMimeType(normalizedMime))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.Write(decoded); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	log.Printf("audio_transcription_openai_file_ready transcription_status=file_ready")
	text, err := s.transcribeOpenAIAudioFile(ctx, tmpFile.Name(), normalizedMime, model)
	if err == nil {
		return text, nil
	}
	if !isOpenAIUnsupportedAudioFormatError(err) || !canUseFFmpegFallback() {
		return "", err
	}

	fallbackPath := strings.TrimSuffix(tmpFile.Name(), filepath.Ext(tmpFile.Name())) + ".wav"
	if convertErr := convertAudioFileWithFFmpeg(ctx, tmpFile.Name(), fallbackPath); convertErr != nil {
		return "", err
	}
	defer func() {
		_ = os.Remove(fallbackPath)
	}()

	log.Printf("audio_transcription_openai_file_ready transcription_status=file_ready")
	return s.transcribeOpenAIAudioFile(ctx, fallbackPath, "audio/wav", model)
}

func (s *Service) transcribeOpenAIAudioFile(ctx context.Context, filePath string, mimeType string, model string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.OpenAIBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := strings.TrimSpace(s.cfg.OpenAIAPIKey)
	if apiKey == "" {
		return "", errors.New("missing openai api key")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gpt-4o-mini-transcribe"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		_ = writer.Close()
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.WriteField("model", model); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.WriteField("language", "pt"); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.WriteField("prompt", "Português do Brasil. Atendimento Schumacher Tur. O cliente pode responder perguntas de reserva. Destinos comuns em Santa Catarina: Fraiburgo, Monte Carlo, Videira, Campos Novos, Chapecó, Concórdia, Ipumirim, Petrolândia, Ituporanga, Seara. Origens comuns no Maranhão: Santa Inês, Monção, Igarapé do Meio. Preserve de forma literal respostas curtas sobre passageiros e crianças, como 'só eu', 'só pra mim', 'é só pra mim', 'a passagem é só pra mim', 'vou sozinho', 'sou só eu', 'não tem criança', 'sem criança', 'não vai criança' e 'tem uma criança'. Não troque 'só pra mim' por uma frase ambígua. Transcreva nomes de cidades conforme essa lista."); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if strings.TrimSpace(mimeType) != "" {
		req.Header.Set("X-Audio-Mime-Type", normalizeAudioMimeType(mimeType))
	}

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai audio transcription failed status=%d body=%s", resp.StatusCode, truncateForLog(string(responseBody), 500))
	}

	payload := map[string]interface{}{}
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &payload)
	}
	text := strings.TrimSpace(firstNonEmptyString(
		asStringPath(payload, "text"),
		asStringPath(payload, "data", "text"),
		asStringPath(payload, "output_text"),
	))
	if text == "" {
		return "", errors.New("openai transcription missing text")
	}
	return text, nil
}

func audioFileExtensionForMimeType(mimeType string) string {
	switch normalizeAudioMimeType(mimeType) {
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/webm":
		return ".webm"
	case "audio/flac":
		return ".flac"
	default:
		return ".bin"
	}
}

func normalizeAudioMimeType(mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		mediaType = mimeType
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func isOpenAIUnsupportedAudioFormatError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unsupported file format audio") ||
		strings.Contains(msg, "unsupported file format")
}

func canUseFFmpegFallback() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func convertAudioFileWithFFmpeg(ctx context.Context, inputPath string, outputPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputPath, "-ac", "1", "-ar", "16000", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg convert failed: %w: %s", err, truncateForLog(string(output), 500))
	}
	return nil
}

func (s *Service) HandleEvolutionMessages(ctx context.Context, body []byte) (EvolutionWebhookResult, error) {
	payload, payloadMap, err := decodeEvolutionPayload(body)
	if err != nil {
		return EvolutionWebhookResult{}, err
	}

	event := strings.TrimSpace(payload.Event)
	if event != "messages.upsert" {
		return EvolutionWebhookResult{
			Status: "skipped",
			Event:  event,
			Reason: "unsupported_event",
		}, nil
	}

	contactKey := strings.TrimSpace(payload.Data.Key.RemoteJID)
	if contactKey == "" {
		contactKey = strings.TrimSpace(payload.Data.Key.RemoteJIDAlt)
	}
	if contactKey == "" {
		return EvolutionWebhookResult{}, ErrMissingEvolutionChatKey
	}

	if payload.Data.Key.FromMe {
		return EvolutionWebhookResult{
			Status:            "skipped",
			Event:             event,
			Direction:         "OUTBOUND",
			ContactKey:        contactKey,
			ProviderMessageID: strings.TrimSpace(payload.Data.Key.ID),
			MessageType:       strings.TrimSpace(payload.Data.MessageType),
			Reason:            "from_me",
		}, nil
	}

	receivedAt := resolveEvolutionTimestamp(payload)
	messageType := strings.TrimSpace(payload.Data.MessageType)
	kind := mapEvolutionKind(messageType)
	textBody := extractEvolutionText(payload.Data)
	normalized := map[string]interface{}{
		"provider":                 "EVOLUTION",
		"event":                    event,
		"instance":                 strings.TrimSpace(payload.Instance),
		"message_type":             messageType,
		"original_message_kind":    kind,
		"message_text":             textBody,
		"from_me":                  payload.Data.Key.FromMe,
		"remote_jid":               contactKey,
		"customer_phone":           normalizePhone(contactKey),
		"push_name":                strings.TrimSpace(payload.Data.PushName),
		"chatwoot_message_id":      payload.Data.ChatwootMessageID,
		"chatwoot_inbox_id":        payload.Data.ChatwootInboxID,
		"chatwoot_conversation_id": payload.Data.ChatwootConversationID,
	}
	processingStatus := "RECEIVED"
	for key, value := range extractEvolutionMessageMetadata(payload.Data) {
		normalized[key] = value
	}

	if strings.EqualFold(messageType, "imageMessage") {
		keyID := strings.TrimSpace(payload.Data.Key.ID)
		remoteJID := strings.TrimSpace(payload.Data.Key.RemoteJID)
		if remoteJID == "" {
			remoteJID = strings.TrimSpace(payload.Data.Key.RemoteJIDAlt)
		}

		imageMime := strings.TrimSpace(stringValue(normalized["image_mime_type"]))
		if imageMime == "" {
			imageMime = "image/jpeg"
		}

		normalized["evolution_message_id"] = keyID
		normalized["evolution_remote_jid"] = remoteJID
		normalized["evolution_from_me"] = payload.Data.Key.FromMe

		dataURL, mediaErr := s.fetchEvolutionMediaDataURL(ctx, keyID, remoteJID, payload.Data.Key.FromMe, imageMime, maxEvolutionInlineImageBytes)
		if mediaErr == nil && strings.TrimSpace(dataURL) != "" {
			normalized["image_data_url"] = dataURL
			normalized["image_source"] = "evolution_get_base64"
		} else {
			normalized["image_source"] = "evolution_url_fallback"
			if mediaErr != nil {
				normalized["image_base64_error"] = mediaErr.Error()
			}
		}
	}

	if strings.EqualFold(messageType, "documentMessage") &&
		strings.EqualFold(strings.TrimSpace(stringValue(normalized["document_mime_type"])), "application/pdf") {

		keyID := strings.TrimSpace(payload.Data.Key.ID)
		remoteJID := strings.TrimSpace(payload.Data.Key.RemoteJID)
		if remoteJID == "" {
			remoteJID = strings.TrimSpace(payload.Data.Key.RemoteJIDAlt)
		}

		normalized["evolution_message_id"] = keyID
		normalized["evolution_remote_jid"] = remoteJID
		normalized["evolution_from_me"] = payload.Data.Key.FromMe

		dataURL, mediaErr := s.fetchEvolutionMediaDataURL(
			ctx,
			keyID,
			remoteJID,
			payload.Data.Key.FromMe,
			"application/pdf",
			maxEvolutionInlinePDFBytes,
		)

		if mediaErr == nil && strings.TrimSpace(dataURL) != "" {
			normalized["document_data_url"] = dataURL
			normalized["document_source"] = "evolution_get_base64"
		} else {
			normalized["document_source"] = "evolution_url_fallback"
			if mediaErr != nil {
				normalized["document_base64_error"] = mediaErr.Error()
			}
		}
	}

	if isEvolutionAudioMessage(messageType, kind, payload.Data.Message.AudioMessage) {
		keyID := strings.TrimSpace(payload.Data.Key.ID)
		remoteJID := strings.TrimSpace(payload.Data.Key.RemoteJID)
		if remoteJID == "" {
			remoteJID = strings.TrimSpace(payload.Data.Key.RemoteJIDAlt)
		}

		audioMetadata := extractEvolutionAudioMetadata(payload.Data.Message.AudioMessage)
		for key, value := range audioMetadata {
			normalized[key] = value
		}

		audioMime := normalizeAudioMimeType(stringValue(normalized["audio_mimetype"]))
		if audioMime == "" {
			audioMime = "audio/ogg"
		}
		if audioSeconds := intValue(normalized["audio_seconds"]); audioSeconds > 0 {
			normalized["audio_seconds"] = audioSeconds
		}

		normalized["media_message_id"] = keyID
		normalized["evolution_message_id"] = keyID
		normalized["evolution_remote_jid"] = remoteJID
		normalized["evolution_from_me"] = payload.Data.Key.FromMe
		normalized["audio_mimetype"] = audioMime

		log.Printf("audio_transcription_start transcription_status=STARTED")
		log.Printf("audio_transcription_media_fetch_start transcription_status=STARTED")
		dataURL, mediaErr := s.fetchEvolutionMediaDataURL(ctx, keyID, remoteJID, payload.Data.Key.FromMe, audioMime, maxEvolutionInlineAudioBytes)
		mediaFetchStatus := "COMPLETED"
		if mediaErr != nil {
			mediaFetchStatus = "FAILED"
		}
		log.Printf("audio_transcription_media_fetch_done transcription_status=%s", mediaFetchStatus)
		if mediaErr != nil {
			normalized["audio_source"] = "evolution_url_fallback"
			normalized["audio_base64_error"] = mediaErr.Error()
			normalized["transcription_status"] = "FAILED"
			normalized["transcription_error"] = mediaErr.Error()
			processingStatus = "REVIEW_REQUIRED"
			log.Printf("audio_transcription_failed transcription_status=FAILED")
		} else {
			normalized["audio_source"] = "evolution_get_base64"
			normalized["audio_data_url"] = dataURL
			normalized["audio_base64"] = stripDataURLPrefix(dataURL)

			model := strings.TrimSpace(s.cfg.OpenAITranscriptionModel)
			if model == "" {
				model = "gpt-4o-mini-transcribe"
			}

			log.Printf("audio_transcription_openai_start transcription_status=STARTED")
			transcriptionText, transcribeErr := s.transcribeOpenAIAudioFromDataURL(ctx, dataURL, audioMime, model)
			if transcribeErr != nil {
				normalized["transcription_status"] = "FAILED"
				normalized["transcription_error"] = transcribeErr.Error()
				log.Printf("audio_transcription_failed transcription_status=FAILED")
			} else {
				transcriptionText = chat.NormalizeIncomingCustomerText(transcriptionText)
				if transcriptionText == "" {
					emptyErr := errors.New("empty transcription text")
					normalized["transcription_status"] = "FAILED"
					normalized["transcription_error"] = emptyErr.Error()
					processingStatus = "REVIEW_REQUIRED"
					log.Printf("audio_transcription_failed transcription_status=FAILED")
				} else {
					textBody = transcriptionText
					kind = "TEXT"
					normalized["message_text"] = transcriptionText
					normalized["transcription_status"] = "COMPLETED"
					normalized["transcription_text"] = transcriptionText
					normalized["transcription_model"] = model
					processingStatus = "READY_FOR_AUTOMATION"
					log.Printf("audio_transcription_openai_done transcription_status=COMPLETED text_len=%d", len(transcriptionText))
				}
			}
		}
	}
	normalized["processing_status"] = processingStatus

	result, err := s.chat.Ingest(ctx, chat.IngestMessageInput{
		Channel:       "WHATSAPP",
		ContactKey:    contactKey,
		CustomerPhone: normalizePhone(contactKey),
		CustomerName:  strings.TrimSpace(payload.Data.PushName),
		Metadata: map[string]interface{}{
			"provider":   "EVOLUTION",
			"instance":   strings.TrimSpace(payload.Instance),
			"server_url": strings.TrimSpace(payload.ServerURL),
			"source":     strings.TrimSpace(payload.Data.Source),
		},
		Message: chat.IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              kind,
			ProviderMessageID: strings.TrimSpace(payload.Data.Key.ID),
			IdempotencyKey:    buildEvolutionIdempotencyKey(contactKey, payload.Data.Key.ID),
			SenderName:        strings.TrimSpace(payload.Data.PushName),
			SenderPhone:       normalizePhone(contactKey),
			Body:              textBody,
			Payload:           payloadMap,
			NormalizedPayload: normalized,
			ProcessingStatus:  processingStatus,
			ReceivedAt:        &receivedAt,
		},
	})
	if err != nil {
		return EvolutionWebhookResult{}, err
	}

	if strings.EqualFold(processingStatus, "READY_FOR_AUTOMATION") && strings.TrimSpace(textBody) != "" {
		log.Printf("audio_transcription_message_update_start transcription_status=COMPLETED")
		updatedMessage, updateErr := s.chat.UpdateMessage(ctx, chat.UpdateMessageInput{
			MessageID: result.Message.ID,
			Body:      textBody,
			NormalizedPayload: map[string]interface{}{
				"transcription_status":     "COMPLETED",
				"transcription_text":       textBody,
				"message_text":             textBody,
				"processing_status":        processingStatus,
				"current_turn_message_ids": []string{result.Message.ID},
			},
			ProcessingStatus: processingStatus,
		})
		if updateErr != nil {
			log.Printf("audio_transcription_message_update_failed transcription_status=FAILED")
			return EvolutionWebhookResult{}, updateErr
		}
		result.Message = updatedMessage
		log.Printf("audio_transcription_message_update_done transcription_status=COMPLETED text_len=%d", len(textBody))
	}

	return EvolutionWebhookResult{
		Status:            "accepted",
		Event:             event,
		Direction:         "INBOUND",
		ContactKey:        contactKey,
		CustomerPhone:     normalizePhone(contactKey),
		ProviderMessageID: strings.TrimSpace(payload.Data.Key.ID),
		MessageType:       strings.TrimSpace(payload.Data.MessageType),
		SessionID:         result.Session.ID,
		MessageID:         result.Message.ID,
		Idempotent:        result.Idempotent,
	}, nil
}

func (s *Service) HandleEvolutionStatus(ctx context.Context, body []byte) (EvolutionStatusWebhookResult, error) {
	payload, payloadMap, err := decodeEvolutionPayload(body)
	if err != nil {
		return EvolutionStatusWebhookResult{}, err
	}

	providerMessageID := strings.TrimSpace(payload.Data.Key.ID)
	if providerMessageID == "" {
		return EvolutionStatusWebhookResult{}, ErrMissingEvolutionMessageID
	}

	providerStatus := normalizeProviderStatus(payload.Data.Status)
	if providerStatus == "" {
		return EvolutionStatusWebhookResult{}, ErrMissingEvolutionStatus
	}

	record, err := s.store.RecordEvolutionStatus(ctx, RecordEvolutionStatusInput{
		ProviderMessageID: providerMessageID,
		ProviderStatus:    providerStatus,
		Event:             strings.TrimSpace(payload.Event),
		ObservedAt:        resolveEvolutionTimestamp(payload),
		Payload:           payloadMap,
	})
	if err != nil {
		return EvolutionStatusWebhookResult{}, err
	}

	return EvolutionStatusWebhookResult{
		Status:                  "accepted",
		Event:                   strings.TrimSpace(payload.Event),
		ProviderMessageID:       providerMessageID,
		ProviderStatus:          providerStatus,
		MessageType:             strings.TrimSpace(payload.Data.MessageType),
		MatchedChatMessages:     record.MatchedChatMessages,
		MatchedOutboundMessages: record.MatchedOutboundMessages,
	}, nil
}

func (s *Service) HandleEvolutionPresence(ctx context.Context, body []byte) (EvolutionPresenceWebhookResult, error) {
	payload, payloadMap, err := decodeEvolutionPayload(body)
	if err != nil {
		return EvolutionPresenceWebhookResult{}, err
	}

	event := strings.TrimSpace(payload.Event)
	if event != "presence.update" && event != "presence-update" {
		return EvolutionPresenceWebhookResult{
			Status: "skipped",
			Event:  event,
			Reason: "unsupported_event",
		}, nil
	}

	contactKey := resolveEvolutionPresenceContactKey(payload.Data)
	if contactKey == "" {
		return EvolutionPresenceWebhookResult{}, ErrMissingEvolutionChatKey
	}

	presenceStatus := resolveEvolutionPresenceStatus(payload.Data)
	if presenceStatus == "" {
		return EvolutionPresenceWebhookResult{}, ErrMissingEvolutionPresence
	}

	result, err := s.chat.ApplyPresenceSignal(ctx, chat.ApplyPresenceSignalInput{
		Channel:        "WHATSAPP",
		ContactKey:     contactKey,
		PresenceStatus: presenceStatus,
		ObservedAt:     timePointer(resolveEvolutionTimestamp(payload)),
		Metadata: map[string]interface{}{
			"provider":   "EVOLUTION",
			"instance":   strings.TrimSpace(payload.Instance),
			"event":      event,
			"source":     strings.TrimSpace(payload.Data.Source),
			"payload":    payloadMap,
			"server_url": strings.TrimSpace(payload.ServerURL),
		},
	})
	if err != nil {
		return EvolutionPresenceWebhookResult{}, err
	}

	return EvolutionPresenceWebhookResult{
		Status:        result.Status,
		Event:         event,
		ContactKey:    contactKey,
		CustomerPhone: normalizePhone(contactKey),
		PresenceState: presenceStatus,
		SessionID:     result.Session.ID,
		Reason:        result.Reason,
	}, nil
}

func (s *Service) RunChatReviewAlerts(ctx context.Context, input RunChatReviewAlertsInput) (RunChatReviewAlertsResult, error) {
	filter := chat.ListSessionsFilter{
		Channel:       strings.ToUpper(strings.TrimSpace(input.Channel)),
		Status:        strings.ToUpper(strings.TrimSpace(input.Status)),
		HandoffStatus: strings.ToUpper(strings.TrimSpace(input.HandoffStatus)),
		ContactKey:    strings.TrimSpace(input.ContactKey),
	}
	observedAt := time.Now().UTC()
	summary, err := s.chat.GetSessionsSummary(ctx, filter)
	if err != nil {
		return RunChatReviewAlertsResult{}, err
	}

	result := RunChatReviewAlertsResult{
		Status:            "skipped",
		Reason:            "no_active_alert",
		WebhookConfigured: s.reviewAlertNotifier != nil && s.reviewAlertNotifier.Enabled(),
		ObservedAt:        observedAt,
		Filter: RunChatReviewAlertsInput{
			Channel:       filter.Channel,
			Status:        filter.Status,
			HandoffStatus: filter.HandoffStatus,
			ContactKey:    filter.ContactKey,
		},
		Summary: buildChatReviewAlertSummarySnapshot(summary),
	}
	if !summary.HasReviewAlert {
		return result, nil
	}
	if s.reviewAlertNotifier == nil || !s.reviewAlertNotifier.Enabled() {
		result.Reason = "notifier_not_configured"
		return result, nil
	}

	alertSignature := buildChatReviewAlertSignature(result.Filter, summary)
	if existing, err := s.store.FindLatestJobRunByKey(ctx, "CHAT_REVIEW_ALERTS", alertSignature); err != nil {
		return RunChatReviewAlertsResult{}, err
	} else if existing != nil {
		result.Reason = "duplicate_alert_state"
		result.Deduplicated = true
		result.JobRunID = existing.ID
		return result, nil
	}

	jobRun, err := s.store.CreateJobRun(ctx, CreateJobRunInput{
		JobName:       "CHAT_REVIEW_ALERTS",
		TriggerSource: "MANUAL",
		Status:        "RUNNING",
		InputPayload: map[string]interface{}{
			"filter":          result.Filter,
			"summary":         result.Summary,
			"alert_signature": alertSignature,
		},
		StartedAt: observedAt,
	})
	if err != nil {
		return RunChatReviewAlertsResult{}, err
	}
	result.JobRunID = jobRun.ID

	payload := ChatReviewAlertNotificationPayload{
		Source:            "chat_review_queue",
		ObservedAt:        observedAt,
		AlertLevel:        summary.ReviewAlertLevel,
		AlertCode:         summary.ReviewAlertCode,
		AlertMessage:      summary.ReviewAlertMessage,
		AlertSessionCount: summary.ReviewAlertSessionCount,
		Filter:            result.Filter,
		Summary:           result.Summary,
	}
	if err := s.reviewAlertNotifier.NotifyReviewAlert(ctx, payload); err != nil {
		_, updateErr := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "FAILED",
			ErrorText:  err.Error(),
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"filter":          result.Filter,
				"summary":         result.Summary,
				"alert_signature": alertSignature,
				"delivery_status": "FAILED",
			},
		})
		if updateErr != nil {
			return RunChatReviewAlertsResult{}, fmt.Errorf("notify review alert: %w; update job run: %v", err, updateErr)
		}
		return RunChatReviewAlertsResult{}, err
	}

	if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
		ID:         jobRun.ID,
		Status:     "SENT",
		FinishedAt: time.Now().UTC(),
		ResultPayload: map[string]interface{}{
			"filter":          result.Filter,
			"summary":         result.Summary,
			"alert_signature": alertSignature,
			"delivery_status": "SENT",
		},
	}); err != nil {
		return RunChatReviewAlertsResult{}, err
	}

	result.Status = "sent"
	result.Reason = "alert_sent"
	result.NotificationTriggered = true
	return result, nil
}

func (s *Service) RunChatBufferFlush(ctx context.Context, input RunChatBufferFlushInput) (RunChatBufferFlushResult, error) {
	return s.runChatBufferFlush(ctx, input, "MANUAL")
}

func (s *Service) RunChatAutoSendRetry(ctx context.Context, input RunChatAutoSendRetryInput) (RunChatAutoSendRetryResult, error) {
	return s.runChatAutoSendRetry(ctx, input, "MANUAL")
}

func (s *Service) runChatBufferFlush(ctx context.Context, input RunChatBufferFlushInput, triggerSource string) (RunChatBufferFlushResult, error) {
	filter, err := normalizeRunChatBufferFlushInput(input)
	if err != nil {
		return RunChatBufferFlushResult{}, err
	}
	triggerSource = strings.ToUpper(strings.TrimSpace(triggerSource))
	if triggerSource == "" {
		triggerSource = "MANUAL"
	}

	observedAt := time.Now().UTC()
	jobRun, err := s.store.CreateJobRun(ctx, CreateJobRunInput{
		JobName:       "CHAT_BUFFER_FLUSH",
		TriggerSource: triggerSource,
		Status:        "RUNNING",
		InputPayload: map[string]interface{}{
			"filter": filter,
		},
		StartedAt: observedAt,
	})
	if err != nil {
		return RunChatBufferFlushResult{}, err
	}

	result := RunChatBufferFlushResult{
		Status:     "skipped",
		Reason:     "no_due_buffers",
		ObservedAt: observedAt,
		JobRunID:   jobRun.ID,
		Filter:     filter,
	}

	sessions, err := s.chat.ListSessions(ctx, chat.ListSessionsFilter{
		Limit:         filter.Limit,
		Channel:       filter.Channel,
		Status:        filter.Status,
		HandoffStatus: filter.HandoffStatus,
	})
	if err != nil {
		if _, updateErr := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "FAILED",
			ErrorText:  err.Error(),
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"reason": "list_sessions_failed",
				"filter": filter,
			},
		}); updateErr != nil {
			return RunChatBufferFlushResult{}, fmt.Errorf("list sessions: %w; update job run: %v", err, updateErr)
		}
		return RunChatBufferFlushResult{}, err
	}

	result.CheckedCount = len(sessions)
	results := make([]ChatBufferFlushSessionResult, 0)
	for _, session := range sessions {
		buffer := parseAutomationBufferState(session.Metadata)
		if !buffer.isDue(observedAt) {
			continue
		}

		result.DueCount++
		item := ChatBufferFlushSessionResult{
			SessionID:    session.ID,
			ContactKey:   strings.TrimSpace(session.ContactKey),
			PendingUntil: buffer.PendingUntil,
		}

		reprocessed, err := s.chat.Reprocess(ctx, chat.ReprocessInput{
			SessionID: session.ID,
			Trigger:   "SYSTEM_BUFFER_FLUSH",
			Metadata: map[string]interface{}{
				"source":        "CHAT_BUFFER_FLUSH",
				"job_run_id":    jobRun.ID,
				"observed_at":   observedAt.Format(time.RFC3339Nano),
				"pending_until": buffer.pendingUntilString(),
			},
		})
		if err != nil {
			item.Status = "failed"
			item.Reason = "reprocess_failed"
			item.ErrorText = err.Error()
			result.FailedCount++
			results = append(results, item)
			continue
		}

		item.Status = strings.ToLower(strings.TrimSpace(reprocessed.Status))
		if item.Status == "" {
			item.Status = "accepted"
		}
		item.Reason = strings.TrimSpace(reprocessed.Reason)
		item.Idempotent = reprocessed.Idempotent
		item.ToolCallCount = len(reprocessed.ToolCalls)
		if reprocessed.Draft != nil {
			item.DraftMessageID = strings.TrimSpace(reprocessed.Draft.ID)
		}
		result.FlushedCount++
		results = append(results, item)
	}
	result.FlushedSessions = results

	jobStatus := "SKIPPED"
	switch {
	case result.DueCount == 0:
		result.Status = "skipped"
		result.Reason = "no_due_buffers"
	case result.FlushedCount > 0 && result.FailedCount == 0:
		result.Status = "processed"
		result.Reason = "buffers_flushed"
		jobStatus = "COMPLETED"
	case result.FlushedCount > 0 && result.FailedCount > 0:
		result.Status = "partial"
		result.Reason = "some_flushes_failed"
		jobStatus = "PARTIAL"
	default:
		result.Status = "failed"
		result.Reason = "all_due_flushes_failed"
		jobStatus = "FAILED"
	}

	if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
		ID:         jobRun.ID,
		Status:     jobStatus,
		ErrorText:  buildChatBufferFlushJobError(result),
		FinishedAt: time.Now().UTC(),
		ResultPayload: map[string]interface{}{
			"reason":           result.Reason,
			"filter":           result.Filter,
			"checked_count":    result.CheckedCount,
			"due_count":        result.DueCount,
			"flushed_count":    result.FlushedCount,
			"failed_count":     result.FailedCount,
			"flushed_sessions": serializeChatBufferFlushSessions(result.FlushedSessions),
		},
	}); err != nil {
		return RunChatBufferFlushResult{}, err
	}

	return result, nil
}

func (s *Service) runChatAutoSendRetry(ctx context.Context, input RunChatAutoSendRetryInput, triggerSource string) (RunChatAutoSendRetryResult, error) {
	filter, err := normalizeRunChatAutoSendRetryInput(input, s.cfg)
	if err != nil {
		return RunChatAutoSendRetryResult{}, err
	}
	triggerSource = strings.ToUpper(strings.TrimSpace(triggerSource))
	if triggerSource == "" {
		triggerSource = "MANUAL"
	}

	observedAt := time.Now().UTC()
	jobRun, err := s.store.CreateJobRun(ctx, CreateJobRunInput{
		JobName:       "CHAT_AUTO_SEND_RETRY",
		TriggerSource: triggerSource,
		Status:        "RUNNING",
		InputPayload: map[string]interface{}{
			"filter": filter,
		},
		StartedAt: observedAt,
	})
	if err != nil {
		return RunChatAutoSendRetryResult{}, err
	}

	result := RunChatAutoSendRetryResult{
		Status:     "skipped",
		Reason:     "no_due_retries",
		ObservedAt: observedAt,
		JobRunID:   jobRun.ID,
		Filter:     filter,
	}

	sessions, err := s.chat.ListSessions(ctx, chat.ListSessionsFilter{
		Limit:               filter.Limit,
		Channel:             filter.Channel,
		Status:              filter.Status,
		HandoffStatus:       filter.HandoffStatus,
		DraftAutoSendStatus: "AUTO_SEND_RETRY_PENDING",
	})
	if err != nil {
		if _, updateErr := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "FAILED",
			ErrorText:  err.Error(),
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"reason": "list_sessions_failed",
				"filter": filter,
			},
		}); updateErr != nil {
			return RunChatAutoSendRetryResult{}, fmt.Errorf("list sessions: %w; update job run: %v", err, updateErr)
		}
		return RunChatAutoSendRetryResult{}, err
	}

	result.CheckedCount = len(sessions)
	items := make([]ChatAutoSendRetrySessionResult, 0)
	cooldown := time.Duration(filter.CooldownSeconds) * time.Second
	for _, session := range sessions {
		retryState := parseAutomationDraftRetryState(session.Metadata)
		if !retryState.isDue(observedAt, cooldown) {
			continue
		}

		result.DueCount++
		item := ChatAutoSendRetrySessionResult{
			SessionID:        session.ID,
			ContactKey:       strings.TrimSpace(session.ContactKey),
			LastAttemptAt:    retryState.LastAttemptAt,
			RetryRequestedAt: retryState.RetryRequestedAt,
		}

		retried, err := s.chat.RetryDraftAutoSend(ctx, chat.RetryDraftAutoSendInput{
			SessionID:   session.ID,
			RequestedBy: "system:auto-send-retry-loop",
			Reason:      "system_retry_loop",
			Metadata: map[string]interface{}{
				"source":           "CHAT_AUTO_SEND_RETRY",
				"job_run_id":       jobRun.ID,
				"observed_at":      observedAt.Format(time.RFC3339Nano),
				"cooldown_seconds": filter.CooldownSeconds,
			},
		})
		if err != nil {
			item.Status = "failed"
			item.Reason = "retry_failed"
			item.ErrorText = err.Error()
			result.FailedCount++
			items = append(items, item)
			continue
		}

		item.Status = strings.ToLower(strings.TrimSpace(retried.Status))
		if item.Status == "" {
			item.Status = "accepted"
		}
		item.Reason = strings.TrimSpace(retried.Reason)
		item.Idempotent = retried.Idempotent
		if retried.Draft != nil {
			item.DraftMessageID = strings.TrimSpace(retried.Draft.ID)
		}
		if retried.Outbound != nil {
			item.OutboundID = strings.TrimSpace(retried.Outbound.ID)
		}

		switch item.Status {
		case "accepted":
			result.RetriedCount++
		case "blocked":
			result.BlockedCount++
		case "skipped":
			result.SkippedCount++
		default:
			result.SkippedCount++
		}
		items = append(items, item)
	}
	result.RetriedSessions = items

	jobStatus := "SKIPPED"
	switch {
	case result.DueCount == 0:
		result.Status = "skipped"
		result.Reason = "no_due_retries"
	case result.FailedCount == 0 && result.RetriedCount > 0 && result.BlockedCount == 0 && result.SkippedCount == 0:
		result.Status = "processed"
		result.Reason = "retries_delivered"
		jobStatus = "COMPLETED"
	case result.FailedCount == 0 && result.RetriedCount == 0 && result.BlockedCount > 0 && result.SkippedCount == 0:
		result.Status = "blocked"
		result.Reason = "retries_blocked_human"
	case result.FailedCount == 0 && result.RetriedCount == 0 && result.SkippedCount > 0 && result.BlockedCount == 0:
		result.Status = "skipped"
		result.Reason = "no_retryable_retries"
	case result.FailedCount > 0 && result.RetriedCount == 0 && result.BlockedCount == 0 && result.SkippedCount == 0:
		result.Status = "failed"
		result.Reason = "all_retries_failed"
		jobStatus = "FAILED"
	default:
		result.Status = "partial"
		result.Reason = "some_retries_not_delivered"
		jobStatus = "PARTIAL"
	}

	if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
		ID:         jobRun.ID,
		Status:     jobStatus,
		ErrorText:  buildChatAutoSendRetryJobError(result),
		FinishedAt: time.Now().UTC(),
		ResultPayload: map[string]interface{}{
			"reason":           result.Reason,
			"filter":           result.Filter,
			"checked_count":    result.CheckedCount,
			"due_count":        result.DueCount,
			"retried_count":    result.RetriedCount,
			"blocked_count":    result.BlockedCount,
			"skipped_count":    result.SkippedCount,
			"failed_count":     result.FailedCount,
			"retried_sessions": serializeChatAutoSendRetrySessions(result.RetriedSessions),
		},
	}); err != nil {
		return RunChatAutoSendRetryResult{}, err
	}

	return result, nil
}

func (s *Service) RunBookingsExpire(ctx context.Context, input RunBookingsExpireInput) (RunBookingsExpireResult, error) {
	if s.bookingSource == nil {
		return RunBookingsExpireResult{}, errors.New("booking expiration source is not configured")
	}

	filter, err := normalizeRunBookingsExpireInput(input)
	if err != nil {
		return RunBookingsExpireResult{}, err
	}

	observedAt := time.Now().UTC()
	jobRun, err := s.store.CreateJobRun(ctx, CreateJobRunInput{
		JobName:       "BOOKINGS_EXPIRE",
		TriggerSource: "MANUAL",
		Status:        "RUNNING",
		InputPayload: map[string]interface{}{
			"status":       "PENDING",
			"limit":        filter.Limit,
			"hold_minutes": filter.HoldMinutes,
		},
		StartedAt: observedAt,
	})
	if err != nil {
		return RunBookingsExpireResult{}, err
	}

	result := RunBookingsExpireResult{
		Status:      "skipped",
		Reason:      "no_expired_bookings",
		ObservedAt:  observedAt,
		JobRunID:    jobRun.ID,
		Limit:       filter.Limit,
		HoldMinutes: filter.HoldMinutes,
	}

	items, err := s.bookingSource.List(ctx, bookings.ListFilter{
		Status: "PENDING",
		Limit:  filter.Limit,
	})
	if err != nil {
		if _, updateErr := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "FAILED",
			ErrorText:  err.Error(),
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"reason": "list_pending_bookings_failed",
			},
		}); updateErr != nil {
			return RunBookingsExpireResult{}, fmt.Errorf("list pending bookings: %w; update job run: %v", err, updateErr)
		}
		return RunBookingsExpireResult{}, err
	}

	result.CheckedCount = len(items)
	expired := collectExpiredBookings(items, observedAt, filter.HoldMinutes)
	if len(expired) == 0 {
		if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "SKIPPED",
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"reason":        result.Reason,
				"checked_count": result.CheckedCount,
				"expired_count": 0,
			},
		}); err != nil {
			return RunBookingsExpireResult{}, err
		}
		return result, nil
	}

	expiredResults := make([]ExpiredBookingResult, 0, len(expired))
	for _, candidate := range expired {
		details, err := s.bookingSource.UpdateStatus(ctx, candidate.Booking.ID, "EXPIRED")
		if err != nil {
			if _, updateErr := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
				ID:         jobRun.ID,
				Status:     "FAILED",
				ErrorText:  err.Error(),
				FinishedAt: time.Now().UTC(),
				ResultPayload: map[string]interface{}{
					"reason":               "expire_booking_failed",
					"checked_count":        result.CheckedCount,
					"expired_count":        len(expiredResults),
					"failed_booking_id":    candidate.Booking.ID,
					"reservation_code":     candidate.Booking.ReservationCode,
					"effective_expires_at": candidate.EffectiveExpiresAt.Format(time.RFC3339Nano),
				},
			}); updateErr != nil {
				return RunBookingsExpireResult{}, fmt.Errorf("expire booking %s: %w; update job run: %v", candidate.Booking.ID, err, updateErr)
			}
			return RunBookingsExpireResult{}, err
		}

		expiredResults = append(expiredResults, ExpiredBookingResult{
			BookingID:          details.Booking.ID,
			ReservationCode:    details.Booking.ReservationCode,
			TripID:             details.Booking.TripID,
			PreviousStatus:     candidate.Booking.Status,
			UpdatedStatus:      details.Booking.Status,
			EffectiveExpiresAt: candidate.EffectiveExpiresAt,
			ExpirationSource:   candidate.ExpirationSource,
		})
	}

	result.Status = "processed"
	result.Reason = "expired_bookings_processed"
	result.ExpiredCount = len(expiredResults)
	result.ExpiredBookings = expiredResults

	if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
		ID:         jobRun.ID,
		Status:     "COMPLETED",
		FinishedAt: time.Now().UTC(),
		ResultPayload: map[string]interface{}{
			"reason":           result.Reason,
			"checked_count":    result.CheckedCount,
			"expired_count":    result.ExpiredCount,
			"expired_bookings": serializeExpiredBookings(expiredResults),
		},
	}); err != nil {
		return RunBookingsExpireResult{}, err
	}

	return result, nil
}

func (s *Service) RunPaymentNotifications(ctx context.Context, input RunPaymentNotificationsInput) (RunPaymentNotificationsResult, error) {
	if s.paymentSource == nil {
		return RunPaymentNotificationsResult{}, errors.New("payment notification source is not configured")
	}

	payment, err := s.resolvePaymentNotificationPayment(ctx, input)
	if err != nil {
		return RunPaymentNotificationsResult{}, err
	}

	notification, err := s.paymentSource.GetBookingNotificationContext(ctx, payment.BookingID)
	if err != nil {
		return RunPaymentNotificationsResult{}, err
	}

	observedAt := time.Now().UTC()
	channel := strings.ToUpper(strings.TrimSpace(input.Channel))
	if channel == "" {
		channel = "WHATSAPP"
	}

	payload := buildPaymentNotificationPayload(payment, notification, observedAt)
	preview := buildPaymentNotificationPreview(payload)
	draftKey := buildPaymentNotificationDraftKey(payment.ID)

	result := RunPaymentNotificationsResult{
		Status:          "skipped",
		ObservedAt:      observedAt,
		PaymentID:       payment.ID,
		BookingID:       payment.BookingID,
		ReservationCode: notification.ReservationCode,
		CustomerName:    notification.CustomerName,
		CustomerPhone:   preview.CustomerPhone,
		ContactKey:      preview.ContactKey,
		PaymentStatus:   notification.PaymentStatus,
		AmountPaid:      notification.AmountPaid,
		AmountDue:       notification.AmountDue,
		Messages:        preview.Messages,
	}

	jobRun, err := s.store.CreateJobRun(ctx, CreateJobRunInput{
		JobName:       "PAYMENT_NOTIFICATIONS",
		TriggerSource: "MANUAL",
		Status:        "RUNNING",
		InputPayload: map[string]interface{}{
			"payment_id":            payment.ID,
			"booking_id":            payment.BookingID,
			"channel":               channel,
			"reservation_code":      notification.ReservationCode,
			"customer_phone":        preview.CustomerPhone,
			"contact_key":           preview.ContactKey,
			"payment_status":        notification.PaymentStatus,
			"amount_paid":           notification.AmountPaid,
			"amount_due":            notification.AmountDue,
			"draft_idempotency_key": draftKey,
		},
		StartedAt: observedAt,
	})
	if err != nil {
		return RunPaymentNotificationsResult{}, err
	}
	result.JobRunID = jobRun.ID

	if preview.ContactKey == "" {
		if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "SKIPPED",
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"reason":     "missing_customer_phone",
				"payment_id": payment.ID,
				"booking_id": payment.BookingID,
			},
		}); err != nil {
			return RunPaymentNotificationsResult{}, err
		}
		result.Reason = "missing_customer_phone"
		return result, nil
	}

	draft, err := s.chat.QueueAutomationDraft(ctx, chat.QueueAutomationDraftInput{
		Channel:        channel,
		ContactKey:     preview.ContactKey,
		CustomerPhone:  preview.CustomerPhone,
		CustomerName:   notification.CustomerName,
		Body:           preview.Body,
		SenderName:     "AUTOMATION",
		IdempotencyKey: draftKey,
		Metadata: map[string]interface{}{
			"source":                "PAYMENT_NOTIFICATION",
			"automation_job_name":   "PAYMENT_NOTIFICATIONS",
			"automation_job_run_id": jobRun.ID,
			"draft_model":           "PAYMENT_NOTIFICATION_TEMPLATE",
			"shadow_mode":           true,
			"requires_human_review": true,
			"payment_id":            payment.ID,
			"booking_id":            payment.BookingID,
			"reservation_code":      notification.ReservationCode,
			"payment_status":        notification.PaymentStatus,
			"amount_paid":           notification.AmountPaid,
			"amount_due":            notification.AmountDue,
			"event":                 payload.Event,
			"messages":              preview.Messages,
		},
	})
	if err != nil {
		if _, updateErr := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
			ID:         jobRun.ID,
			Status:     "FAILED",
			ErrorText:  err.Error(),
			FinishedAt: time.Now().UTC(),
			ResultPayload: map[string]interface{}{
				"payment_id": payment.ID,
				"booking_id": payment.BookingID,
				"reason":     "draft_queue_failed",
			},
		}); updateErr != nil {
			return RunPaymentNotificationsResult{}, fmt.Errorf("queue payment notification draft: %w; update job run: %v", err, updateErr)
		}
		return RunPaymentNotificationsResult{}, err
	}

	jobStatus := "QUEUED"
	result.Status = "queued"
	result.Reason = "draft_created"
	result.DraftQueued = true
	result.SessionID = draft.Session.ID
	result.DraftMessageID = draft.Message.ID
	if draft.Idempotent {
		jobStatus = "SKIPPED"
		result.Status = "skipped"
		result.Reason = "draft_already_exists"
		result.DraftQueued = false
		result.Idempotent = true
	}

	if _, err := s.store.UpdateJobRun(ctx, UpdateJobRunInput{
		ID:         jobRun.ID,
		Status:     jobStatus,
		FinishedAt: time.Now().UTC(),
		ResultPayload: map[string]interface{}{
			"payment_id":            payment.ID,
			"booking_id":            payment.BookingID,
			"reservation_code":      notification.ReservationCode,
			"contact_key":           preview.ContactKey,
			"draft_idempotency_key": draftKey,
			"session_id":            draft.Session.ID,
			"draft_message_id":      draft.Message.ID,
			"idempotent":            draft.Idempotent,
			"messages_count":        len(preview.Messages),
			"reason":                result.Reason,
		},
	}); err != nil {
		return RunPaymentNotificationsResult{}, err
	}

	return result, nil
}

func (s *Service) ListJobRuns(ctx context.Context, input ListJobRunsInput) (ListJobRunsResult, error) {
	jobName := normalizeJobName(input.JobName)
	if jobName == "" {
		return ListJobRunsResult{}, ErrMissingJobName
	}

	limit := input.Limit
	switch {
	case limit == 0:
		limit = 20
	case limit < 0:
		return ListJobRunsResult{}, ErrInvalidJobRunLimit
	case limit > 100:
		limit = 100
	}

	filter := ListJobRunsInput{
		JobName:       jobName,
		Status:        strings.ToUpper(strings.TrimSpace(input.Status)),
		TriggerSource: strings.ToUpper(strings.TrimSpace(input.TriggerSource)),
		Limit:         limit,
	}
	runs, err := s.store.ListJobRuns(ctx, filter)
	if err != nil {
		return ListJobRunsResult{}, err
	}

	return ListJobRunsResult{
		JobName: filter.JobName,
		Filter:  filter,
		Count:   len(runs),
		Runs:    runs,
	}, nil
}

func (s *Service) GetCutoverReadiness(ctx context.Context) (CutoverReadinessResult, error) {
	checkedAt := time.Now().UTC()

	summary, err := s.chat.GetSessionsSummary(ctx, chat.ListSessionsFilter{})
	if err != nil {
		return CutoverReadinessResult{}, err
	}

	checks := make([]CutoverReadinessCheck, 0, 10)
	addCheck := func(key string, ready bool, critical bool, readyMessage string, attentionMessage string) {
		status := "READY"
		message := readyMessage
		if !ready {
			status = "ATTENTION_REQUIRED"
			message = attentionMessage
		}
		checks = append(checks, CutoverReadinessCheck{
			Key:      key,
			Status:   status,
			Message:  message,
			Critical: critical,
		})
	}

	openAIReady := strings.TrimSpace(s.cfg.OpenAIAPIKey) != "" && strings.TrimSpace(s.cfg.OpenAIModel) != ""
	evolutionSenderReady := strings.TrimSpace(s.cfg.EvolutionBaseURL) != "" &&
		strings.TrimSpace(s.cfg.EvolutionAPIKey) != "" &&
		strings.TrimSpace(s.cfg.EvolutionInstance) != ""
	evolutionSecretReady := strings.TrimSpace(s.cfg.EvolutionWebhookSecret) != ""

	addCheck("openai_runner", openAIReady, true,
		"OPENAI_API_KEY e OPENAI_MODEL configurados para o loop do agente.",
		"Falta configurar OPENAI_API_KEY e/ou OPENAI_MODEL para operar o atendimento sem o n8n.")
	addCheck("evolution_sender", evolutionSenderReady, true,
		"Evolution sender configurado para envio e reply da API.",
		"Falta configurar EVOLUTION_BASE_URL, EVOLUTION_API_KEY e/ou EVOLUTION_INSTANCE.")
	addCheck("evolution_webhook_secret", evolutionSecretReady, true,
		"EVOLUTION_WEBHOOK_SECRET configurado para validar os webhooks publicos.",
		"EVOLUTION_WEBHOOK_SECRET ainda nao esta configurado; o cutover fica sem validacao explicita de origem.")
	addCheck("chat_buffer_auto_flush", s.cfg.ChatBufferAutoFlushEnabled, true,
		"Loop server-side de chat-buffer-flush esta habilitado.",
		"CHAT_BUFFER_AUTO_FLUSH_ENABLED esta desligado.")
	addCheck("chat_auto_send_retry", s.cfg.ChatAutoSendRetryEnabled, true,
		"Loop server-side de chat-auto-send-retry esta habilitado.",
		"CHAT_AUTO_SEND_RETRY_ENABLED esta desligado.")
	addCheck("payment_notifications", s.paymentSource != nil, true,
		"Fonte interna para notificacao pos-pagamento esta conectada.",
		"Fonte interna de notificacao pos-pagamento nao foi conectada no bootstrap.")
	addCheck("bookings_expire", s.bookingSource != nil, true,
		"Fonte interna para expiracao de reservas esta conectada.",
		"Fonte interna de expiracao de reservas nao foi conectada no bootstrap.")
	addCheck("session_observability", true, false,
		"Observabilidade operacional disponivel em /chat/sessions, /chat/sessions/summary e /chat/sessions/{sessionId}/draft.",
		"Observabilidade operacional indisponivel.")
	addCheck("review_queue", !summary.HasReviewAlert, false,
		"Fila de revisao sem alerta ativo no momento.",
		firstNonEmptyString(
			summary.ReviewAlertMessage,
			fmt.Sprintf("Fila de revisao com alerta ativo (%s).", strings.TrimSpace(summary.ReviewAlertCode)),
		))
	addCheck("auto_send_issue_queue", summary.AutoSendIssueCount == 0, false,
		"Nao ha problemas pendentes na fila de auto-send.",
		fmt.Sprintf("Existem %d sessoes com problema de auto-send pendente.", summary.AutoSendIssueCount))

	latestJobs := make([]CutoverReadinessJob, 0, 4)
	for _, jobName := range []string{"CHAT_BUFFER_FLUSH", "CHAT_AUTO_SEND_RETRY", "BOOKINGS_EXPIRE", "PAYMENT_NOTIFICATIONS"} {
		item, err := s.latestCutoverJob(ctx, jobName)
		if err != nil {
			return CutoverReadinessResult{}, err
		}
		latestJobs = append(latestJobs, item)
	}

	issues := make([]string, 0, len(checks))
	overallStatus := "READY"
	for _, check := range checks {
		if check.Status == "READY" {
			continue
		}
		issues = append(issues, check.Message)
		if check.Critical {
			overallStatus = "ATTENTION_REQUIRED"
		}
	}
	if overallStatus == "READY" && (summary.HasReviewAlert || summary.AutoSendIssueCount > 0) {
		overallStatus = "ATTENTION_REQUIRED"
	}

	return CutoverReadinessResult{
		Status:          overallStatus,
		CheckedAt:       checkedAt,
		Issues:          issues,
		Checks:          checks,
		SessionsSummary: summary,
		LatestJobs:      latestJobs,
	}, nil
}

func (s *Service) latestCutoverJob(ctx context.Context, jobName string) (CutoverReadinessJob, error) {
	runs, err := s.store.ListJobRuns(ctx, ListJobRunsInput{
		JobName: jobName,
		Limit:   1,
	})
	if err != nil {
		return CutoverReadinessJob{}, err
	}
	if len(runs) == 0 {
		return CutoverReadinessJob{
			JobName: jobName,
			Status:  "NEVER_RUN",
		}, nil
	}
	run := runs[0]
	return CutoverReadinessJob{
		JobName:       jobName,
		Status:        strings.TrimSpace(run.Status),
		JobRunID:      strings.TrimSpace(run.ID),
		TriggerSource: strings.TrimSpace(run.TriggerSource),
		StartedAt:     timePointer(run.StartedAt),
		FinishedAt:    run.FinishedAt,
		ErrorText:     strings.TrimSpace(run.ErrorText),
	}, nil
}

func buildChatReviewAlertSummarySnapshot(summary chat.SessionsSummary) ChatReviewAlertSummarySnapshot {
	return ChatReviewAlertSummarySnapshot{
		TotalCount:                summary.TotalCount,
		PendingReviewCount:        summary.PendingReviewCount,
		ReviewedCount:             summary.ReviewedCount,
		NoDraftCount:              summary.NoDraftCount,
		HumanOwnedCount:           summary.HumanOwnedCount,
		BotOwnedCount:             summary.BotOwnedCount,
		DueSoonReviewCount:        summary.DueSoonReviewCount,
		OverdueReviewCount:        summary.OverdueReviewCount,
		HighPriorityReviewCount:   summary.HighPriorityReviewCount,
		MediumPriorityReviewCount: summary.MediumPriorityReviewCount,
		LowPriorityReviewCount:    summary.LowPriorityReviewCount,
		OldestPendingAgeSeconds:   summary.OldestPendingAgeSeconds,
		ReviewSLASeconds:          summary.ReviewSLASeconds,
		HasReviewAlert:            summary.HasReviewAlert,
		ReviewAlertLevel:          summary.ReviewAlertLevel,
		ReviewAlertCode:           summary.ReviewAlertCode,
		ReviewAlertMessage:        summary.ReviewAlertMessage,
		ReviewAlertSessionCount:   summary.ReviewAlertSessionCount,
	}
}

func buildChatReviewAlertSignature(filter RunChatReviewAlertsInput, summary chat.SessionsSummary) string {
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(summary.ReviewAlertCode)),
		strings.ToUpper(strings.TrimSpace(summary.ReviewAlertLevel)),
		fmt.Sprintf("%d", summary.ReviewAlertSessionCount),
		fmt.Sprintf("%d", summary.OverdueReviewCount),
		fmt.Sprintf("%d", summary.DueSoonReviewCount),
		fmt.Sprintf("%d", summary.HighPriorityReviewCount),
		fmt.Sprintf("%d", summary.MediumPriorityReviewCount),
		fmt.Sprintf("%d", summary.LowPriorityReviewCount),
		fmt.Sprintf("%d", summary.ReviewSLASeconds),
		strings.ToUpper(strings.TrimSpace(filter.Channel)),
		strings.ToUpper(strings.TrimSpace(filter.Status)),
		strings.ToUpper(strings.TrimSpace(filter.HandoffStatus)),
		strings.TrimSpace(filter.ContactKey),
	}, "|")
}

func normalizeJobName(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "-", "_")
	trimmed = strings.ReplaceAll(trimmed, " ", "_")
	return strings.ToUpper(trimmed)
}

func (s *Service) resolvePaymentNotificationPayment(ctx context.Context, input RunPaymentNotificationsInput) (payments.Payment, error) {
	paymentID := strings.TrimSpace(input.PaymentID)
	if paymentID != "" {
		return s.paymentSource.Get(ctx, paymentID)
	}

	bookingID := strings.TrimSpace(input.BookingID)
	if bookingID == "" {
		return payments.Payment{}, ErrPaymentNotificationTarget
	}

	items, err := s.paymentSource.List(ctx, payments.PaymentListFilter{
		BookingID: bookingID,
		Status:    "PAID",
		Limit:     1,
	})
	if err != nil {
		return payments.Payment{}, err
	}
	if len(items) == 0 {
		return payments.Payment{}, pgx.ErrNoRows
	}
	return items[0], nil
}

func decodeEvolutionPayload(body []byte) (EvolutionWebhookPayload, map[string]interface{}, error) {
	var payload EvolutionWebhookPayload
	if err := json.Unmarshal(body, &payload); err == nil && payload.Event != "" {
		rawMap := map[string]interface{}{}
		if err := json.Unmarshal(body, &rawMap); err != nil {
			return EvolutionWebhookPayload{}, nil, ErrInvalidEvolutionPayload
		}
		return payload, rawMap, nil
	}

	var envelope EvolutionEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Body) == 0 {
		return EvolutionWebhookPayload{}, nil, ErrInvalidEvolutionPayload
	}
	if err := json.Unmarshal(envelope.Body, &payload); err != nil || payload.Event == "" {
		return EvolutionWebhookPayload{}, nil, ErrInvalidEvolutionPayload
	}
	rawMap := map[string]interface{}{}
	if err := json.Unmarshal(envelope.Body, &rawMap); err != nil {
		return EvolutionWebhookPayload{}, nil, ErrInvalidEvolutionPayload
	}
	return payload, rawMap, nil
}

func resolveEvolutionTimestamp(payload EvolutionWebhookPayload) time.Time {
	if payload.Data.MessageTimestamp > 0 {
		return time.Unix(payload.Data.MessageTimestamp, 0).UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.DateTime)); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}

func extractEvolutionText(data EvolutionMessageData) string {
	switch strings.TrimSpace(data.MessageType) {
	case "conversation":
		return strings.TrimSpace(data.Message.Conversation)
	case "extendedTextMessage":
		if data.Message.ExtendedTextMessage != nil {
			return strings.TrimSpace(data.Message.ExtendedTextMessage.Text)
		}
	case "imageMessage":
		if data.Message.ImageMessage != nil {
			return strings.TrimSpace(data.Message.ImageMessage.Caption)
		}
	case "videoMessage":
		if data.Message.VideoMessage != nil {
			return strings.TrimSpace(data.Message.VideoMessage.Caption)
		}
	case "documentMessage":
		if text := extractEvolutionDocumentCaption(data.Message.DocumentMessage); text != "" {
			return text
		}
		if text := extractEvolutionDocumentFileName(data.Message.DocumentMessage); text != "" {
			return text
		}
	}

	if text := strings.TrimSpace(data.Message.Conversation); text != "" {
		return text
	}
	if data.Message.ExtendedTextMessage != nil {
		if text := strings.TrimSpace(data.Message.ExtendedTextMessage.Text); text != "" {
			return text
		}
	}
	if data.Message.ImageMessage != nil {
		if text := strings.TrimSpace(data.Message.ImageMessage.Caption); text != "" {
			return text
		}
	}
	if data.Message.VideoMessage != nil {
		return strings.TrimSpace(data.Message.VideoMessage.Caption)
	}
	if text := extractEvolutionDocumentCaption(data.Message.DocumentMessage); text != "" {
		return text
	}
	if text := extractEvolutionDocumentFileName(data.Message.DocumentMessage); text != "" {
		return text
	}
	return ""
}

func extractEvolutionMessageMetadata(data EvolutionMessageData) map[string]interface{} {
	switch strings.TrimSpace(data.MessageType) {
	case "imageMessage":
		return extractEvolutionImageMetadata(data.Message.ImageMessage)
	case "audioMessage", "audio", "ptt", "voice":
		return extractEvolutionAudioMetadata(data.Message.AudioMessage)
	case "documentMessage":
		return extractEvolutionDocumentMetadata(data.Message.DocumentMessage)
	default:
		return nil
	}
}

func extractEvolutionImageMetadata(payload *EvolutionMediaMessage) map[string]interface{} {
	if payload == nil {
		return nil
	}

	metadata := map[string]interface{}{}
	if caption := strings.TrimSpace(payload.Caption); caption != "" {
		metadata["image_caption"] = caption
	}
	if mimeType := strings.TrimSpace(payload.MimeType); mimeType != "" {
		metadata["image_mime_type"] = mimeType
	}
	if fileLength := intValue(payload.FileLength); fileLength > 0 {
		metadata["image_file_length"] = fileLength
	}
	if url := firstNonEmptyString(payload.URL, payload.DirectPath); url != "" {
		metadata["image_url"] = url
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func extractEvolutionDocumentMetadata(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}

	metadata := map[string]interface{}{}
	if caption := extractEvolutionDocumentCaption(payload); caption != "" {
		metadata["document_caption"] = caption
	}
	if fileName := extractEvolutionDocumentFileName(payload); fileName != "" {
		metadata["document_file_name"] = fileName
	}
	if mimeType := firstNonEmptyString(
		payload["mimetype"],
		payload["mimeType"],
		payload["mime_type"],
	); mimeType != "" {
		metadata["document_mime_type"] = mimeType
		if strings.EqualFold(mimeType, "application/pdf") {
			metadata["document_is_pdf"] = true
			metadata["document_format"] = "PDF"
		}
	}
	if pageCount := intValue(payload["pageCount"]); pageCount > 0 {
		metadata["document_page_count"] = pageCount
	}
	if fileLength := intValue(payload["fileLength"]); fileLength > 0 {
		metadata["document_file_length"] = fileLength
	}
	if url := firstNonEmptyString(
		payload["url"],
		payload["directPath"],
		payload["direct_path"],
	); url != "" {
		metadata["document_url"] = url
	}
	if title := firstNonEmptyString(payload["title"]); title != "" {
		metadata["document_title"] = title
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func extractEvolutionDocumentCaption(payload map[string]interface{}) string {
	return firstNonEmptyString(payload["caption"])
}

func extractEvolutionDocumentFileName(payload map[string]interface{}) string {
	return firstNonEmptyString(
		payload["fileName"],
		payload["file_name"],
		payload["title"],
	)
}

func extractEvolutionAudioMetadata(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}

	metadata := map[string]interface{}{}
	if mimeType := firstNonEmptyString(
		payload["mimetype"],
		payload["mimeType"],
		payload["mime_type"],
	); mimeType != "" {
		metadata["audio_mimetype"] = mimeType
	}
	seconds := intValue(payload["seconds"])
	if seconds <= 0 {
		seconds = intValue(payload["duration"])
	}
	if seconds <= 0 {
		seconds = intValue(payload["durationSeconds"])
	}
	if seconds <= 0 {
		seconds = intValue(payload["duration_seconds"])
	}
	if seconds > 0 {
		metadata["audio_seconds"] = seconds
	}
	if url := firstNonEmptyString(
		payload["url"],
		payload["directPath"],
		payload["direct_path"],
	); url != "" {
		metadata["audio_url"] = url
	}
	if isPTT(payload["ptt"]) {
		metadata["audio_is_ptt"] = true
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func isEvolutionAudioMessage(messageType string, kind string, payload map[string]interface{}) bool {
	folded := strings.ToLower(strings.TrimSpace(messageType))
	switch folded {
	case "audiomessage", "audio", "voice", "ptt":
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "AUDIO", "VOICE", "PTT":
		return true
	}
	return len(payload) > 0
}

func isPTT(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y", "on":
			return true
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case int32:
		return typed != 0
	case float64:
		return typed != 0
	case float32:
		return typed != 0
	}
	return false
}

func normalizePhone(contactKey string) string {
	trimmed := strings.TrimSpace(contactKey)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, "@"); idx >= 0 {
		trimmed = trimmed[:idx]
	}

	builder := strings.Builder{}
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func mapEvolutionKind(messageType string) string {
	switch strings.TrimSpace(messageType) {
	case "conversation", "extendedTextMessage":
		return "TEXT"
	case "imageMessage":
		return "IMAGE"
	case "videoMessage":
		return "VIDEO"
	case "audioMessage":
		return "AUDIO"
	case "documentMessage":
		return "DOCUMENT"
	default:
		return strings.ToUpper(strings.TrimSpace(messageType))
	}
}

func buildEvolutionIdempotencyKey(contactKey string, providerMessageID string) string {
	contactKey = strings.TrimSpace(contactKey)
	providerMessageID = strings.TrimSpace(providerMessageID)
	if contactKey == "" && providerMessageID == "" {
		return ""
	}
	return "evolution:" + contactKey + ":" + providerMessageID
}

func normalizeProviderStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

type automationBufferState struct {
	Status       string
	PendingUntil *time.Time
}

func (b automationBufferState) isDue(observedAt time.Time) bool {
	if strings.ToUpper(strings.TrimSpace(b.Status)) != "PENDING" || b.PendingUntil == nil {
		return false
	}
	return !b.PendingUntil.After(observedAt)
}

func (b automationBufferState) pendingUntilString() string {
	if b.PendingUntil == nil {
		return ""
	}
	return b.PendingUntil.UTC().Format(time.RFC3339Nano)
}

func parseAutomationBufferState(metadata map[string]interface{}) automationBufferState {
	raw, ok := metadata["buffer"].(map[string]interface{})
	if !ok || len(raw) == 0 {
		return automationBufferState{}
	}
	return automationBufferState{
		Status:       strings.ToUpper(strings.TrimSpace(stringValue(raw["status"]))),
		PendingUntil: parseAutomationTime(raw["pending_until"]),
	}
}

type automationDraftRetryState struct {
	Status           string
	LastAttemptAt    *time.Time
	RetryRequestedAt *time.Time
}

func (r automationDraftRetryState) isDue(observedAt time.Time, cooldown time.Duration) bool {
	if strings.ToUpper(strings.TrimSpace(r.Status)) != "AUTO_SEND_RETRY_PENDING" {
		return false
	}
	base := r.referenceTime()
	if base == nil {
		return true
	}
	if cooldown <= 0 {
		return !base.After(observedAt)
	}
	return !base.Add(cooldown).After(observedAt)
}

func (r automationDraftRetryState) referenceTime() *time.Time {
	switch {
	case r.LastAttemptAt != nil && r.RetryRequestedAt != nil:
		if r.LastAttemptAt.After(*r.RetryRequestedAt) {
			return r.LastAttemptAt
		}
		return r.RetryRequestedAt
	case r.LastAttemptAt != nil:
		return r.LastAttemptAt
	case r.RetryRequestedAt != nil:
		return r.RetryRequestedAt
	default:
		return nil
	}
}

func parseAutomationDraftRetryState(metadata map[string]interface{}) automationDraftRetryState {
	raw, ok := metadata["agent"].(map[string]interface{})
	if !ok || len(raw) == 0 {
		return automationDraftRetryState{}
	}
	return automationDraftRetryState{
		Status:           strings.ToUpper(strings.TrimSpace(stringValue(raw["auto_send_status"]))),
		LastAttemptAt:    parseAutomationTime(raw["auto_send_last_attempt_at"]),
		RetryRequestedAt: parseAutomationTime(raw["auto_send_retry_requested_at"]),
	}
}

func parseAutomationTime(value interface{}) *time.Time {
	raw := strings.TrimSpace(stringValue(value))
	if raw == "" {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		utc := parsed.UTC()
		return &utc
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		utc := parsed.UTC()
		return &utc
	}
	return nil
}

func normalizeRunChatBufferFlushInput(input RunChatBufferFlushInput) (RunChatBufferFlushInput, error) {
	if input.Limit < 0 || input.Limit > 500 {
		return RunChatBufferFlushInput{}, ErrInvalidChatBufferFlushLimit
	}
	if input.Limit == 0 {
		input.Limit = 100
	}
	input.Channel = strings.ToUpper(strings.TrimSpace(input.Channel))
	if input.Channel == "" {
		input.Channel = "WHATSAPP"
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "ACTIVE"
	}
	input.HandoffStatus = strings.ToUpper(strings.TrimSpace(input.HandoffStatus))
	if input.HandoffStatus == "" {
		input.HandoffStatus = "BOT"
	}
	return input, nil
}

func normalizeRunChatAutoSendRetryInput(input RunChatAutoSendRetryInput, cfg config.Config) (RunChatAutoSendRetryInput, error) {
	if input.Limit < 0 || input.Limit > 500 {
		return RunChatAutoSendRetryInput{}, ErrInvalidChatAutoSendRetryLimit
	}
	if input.CooldownSeconds < 0 || input.CooldownSeconds > 86400 {
		return RunChatAutoSendRetryInput{}, ErrInvalidChatAutoSendRetryCooldown
	}
	if input.Limit == 0 {
		input.Limit = cfg.ChatAutoSendRetryLimit
		if input.Limit <= 0 {
			input.Limit = 20
		}
	}
	if input.CooldownSeconds == 0 {
		input.CooldownSeconds = cfg.ChatAutoSendRetryCooldownSeconds
		if input.CooldownSeconds <= 0 {
			input.CooldownSeconds = 30
		}
	}
	input.Channel = strings.ToUpper(strings.TrimSpace(input.Channel))
	if input.Channel == "" {
		input.Channel = "WHATSAPP"
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "ACTIVE"
	}
	input.HandoffStatus = strings.ToUpper(strings.TrimSpace(input.HandoffStatus))
	if input.HandoffStatus == "" {
		input.HandoffStatus = "BOT"
	}
	return input, nil
}

func serializeChatBufferFlushSessions(items []ChatBufferFlushSessionResult) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		payload := map[string]interface{}{
			"session_id":      item.SessionID,
			"contact_key":     item.ContactKey,
			"status":          item.Status,
			"reason":          item.Reason,
			"idempotent":      item.Idempotent,
			"tool_call_count": item.ToolCallCount,
		}
		if item.PendingUntil != nil {
			payload["pending_until"] = item.PendingUntil.UTC().Format(time.RFC3339Nano)
		}
		if strings.TrimSpace(item.DraftMessageID) != "" {
			payload["draft_message_id"] = item.DraftMessageID
		}
		if strings.TrimSpace(item.ErrorText) != "" {
			payload["error_text"] = item.ErrorText
		}
		result = append(result, payload)
	}
	return result
}

func serializeChatAutoSendRetrySessions(items []ChatAutoSendRetrySessionResult) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		payload := map[string]interface{}{
			"session_id":  item.SessionID,
			"contact_key": item.ContactKey,
			"status":      item.Status,
			"reason":      item.Reason,
			"idempotent":  item.Idempotent,
		}
		if item.LastAttemptAt != nil {
			payload["last_attempt_at"] = item.LastAttemptAt.UTC().Format(time.RFC3339Nano)
		}
		if item.RetryRequestedAt != nil {
			payload["retry_requested_at"] = item.RetryRequestedAt.UTC().Format(time.RFC3339Nano)
		}
		if strings.TrimSpace(item.DraftMessageID) != "" {
			payload["draft_message_id"] = item.DraftMessageID
		}
		if strings.TrimSpace(item.OutboundID) != "" {
			payload["outbound_id"] = item.OutboundID
		}
		if strings.TrimSpace(item.ErrorText) != "" {
			payload["error_text"] = item.ErrorText
		}
		result = append(result, payload)
	}
	return result
}

func buildChatBufferFlushJobError(result RunChatBufferFlushResult) string {
	if result.FailedCount == 0 {
		return ""
	}
	if result.FlushedCount == 0 {
		return "all due chat buffer flushes failed"
	}
	return "some chat buffer flushes failed"
}

func buildChatAutoSendRetryJobError(result RunChatAutoSendRetryResult) string {
	if result.FailedCount == 0 {
		return ""
	}
	if result.RetriedCount == 0 && result.BlockedCount == 0 && result.SkippedCount == 0 {
		return "all due chat auto-send retries failed"
	}
	return "some chat auto-send retries failed"
}

func resolveEvolutionPresenceContactKey(data EvolutionMessageData) string {
	if value := strings.TrimSpace(data.ID); value != "" {
		return value
	}
	if value := strings.TrimSpace(data.Key.RemoteJID); value != "" {
		return value
	}
	for key := range data.Presences {
		if strings.TrimSpace(key) != "" {
			return key
		}
	}
	return ""
}

func resolveEvolutionPresenceStatus(data EvolutionMessageData) string {
	for _, raw := range data.Presences {
		payload, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range []string{"lastKnownPresence", "presence", "status"} {
			if value := normalizeProviderStatus(stringValue(payload[key])); value != "" {
				return value
			}
		}
	}
	return normalizeProviderStatus(data.Status)
}

func timePointer(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}

func stringValue(value interface{}) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func asStringPath(root map[string]interface{}, path ...string) string {
	var current interface{} = root
	for _, key := range path {
		item, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = item[key]
	}
	return strings.TrimSpace(stringValue(current))
}

func stripDataURLPrefix(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		if index := strings.Index(value, ","); index >= 0 {
			return strings.TrimSpace(value[index+1:])
		}
	}
	return value
}

func truncateForLog(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func intValue(value interface{}) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	}
	return 0
}
