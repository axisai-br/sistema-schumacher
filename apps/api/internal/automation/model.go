package automation

import (
	"encoding/json"
	"time"

	"schumacher-tur/api/internal/chat"
)

type JobRun struct {
	ID                string                 `json:"id"`
	JobName           string                 `json:"job_name"`
	TriggerSource     string                 `json:"trigger_source"`
	RequestedByUserID string                 `json:"requested_by_user_id,omitempty"`
	Status            string                 `json:"status"`
	InputPayload      map[string]interface{} `json:"input_payload,omitempty"`
	ResultPayload     map[string]interface{} `json:"result_payload,omitempty"`
	ErrorText         string                 `json:"error_text,omitempty"`
	StartedAt         time.Time              `json:"started_at"`
	FinishedAt        *time.Time             `json:"finished_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

type ListJobRunsInput struct {
	JobName       string `json:"job_name"`
	Status        string `json:"status,omitempty"`
	TriggerSource string `json:"trigger_source,omitempty"`
	Limit         int    `json:"limit"`
}

type ListJobRunsResult struct {
	JobName string           `json:"job_name"`
	Filter  ListJobRunsInput `json:"filter"`
	Count   int              `json:"count"`
	Runs    []JobRun         `json:"runs"`
}

type CutoverReadinessCheck struct {
	Key      string `json:"key"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Critical bool   `json:"critical"`
}

type CutoverReadinessJob struct {
	JobName       string     `json:"job_name"`
	Status        string     `json:"status"`
	JobRunID      string     `json:"job_run_id,omitempty"`
	TriggerSource string     `json:"trigger_source,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	ErrorText     string     `json:"error_text,omitempty"`
}

type CutoverReadinessResult struct {
	Status          string                  `json:"status"`
	CheckedAt       time.Time               `json:"checked_at"`
	Issues          []string                `json:"issues,omitempty"`
	Checks          []CutoverReadinessCheck `json:"checks"`
	SessionsSummary chat.SessionsSummary    `json:"sessions_summary"`
	LatestJobs      []CutoverReadinessJob   `json:"latest_jobs"`
}

type OutboundMessage struct {
	ID                string                 `json:"id"`
	SessionID         string                 `json:"session_id,omitempty"`
	JobRunID          string                 `json:"job_run_id,omitempty"`
	Channel           string                 `json:"channel"`
	Recipient         string                 `json:"recipient"`
	TemplateName      string                 `json:"template_name,omitempty"`
	Payload           map[string]interface{} `json:"payload,omitempty"`
	Provider          string                 `json:"provider"`
	ProviderMessageID string                 `json:"provider_message_id,omitempty"`
	IdempotencyKey    string                 `json:"idempotency_key"`
	Status            string                 `json:"status"`
	ErrorText         string                 `json:"error_text,omitempty"`
	ScheduledAt       *time.Time             `json:"scheduled_at,omitempty"`
	SentAt            *time.Time             `json:"sent_at,omitempty"`
	DeliveredAt       *time.Time             `json:"delivered_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type RunChatReviewAlertsInput struct {
	Channel       string `json:"channel"`
	Status        string `json:"status"`
	HandoffStatus string `json:"handoff_status"`
	ContactKey    string `json:"contact_key"`
}

type RunChatReviewAlertsResult struct {
	Status                string                         `json:"status"`
	Reason                string                         `json:"reason,omitempty"`
	NotificationTriggered bool                           `json:"notification_triggered"`
	Deduplicated          bool                           `json:"deduplicated"`
	WebhookConfigured     bool                           `json:"webhook_configured"`
	JobRunID              string                         `json:"job_run_id,omitempty"`
	ObservedAt            time.Time                      `json:"observed_at"`
	Filter                RunChatReviewAlertsInput       `json:"filter"`
	Summary               ChatReviewAlertSummarySnapshot `json:"summary"`
}

type RunBookingsExpireInput struct {
	Limit       int `json:"limit"`
	HoldMinutes int `json:"hold_minutes"`
}

type RunChatBufferFlushInput struct {
	Limit         int    `json:"limit"`
	Channel       string `json:"channel"`
	Status        string `json:"status"`
	HandoffStatus string `json:"handoff_status"`
}

type RunChatAutoSendRetryInput struct {
	Limit           int    `json:"limit"`
	CooldownSeconds int    `json:"cooldown_seconds"`
	Channel         string `json:"channel"`
	Status          string `json:"status"`
	HandoffStatus   string `json:"handoff_status"`
}

type ChatBufferFlushSessionResult struct {
	SessionID      string     `json:"session_id"`
	ContactKey     string     `json:"contact_key,omitempty"`
	PendingUntil   *time.Time `json:"pending_until,omitempty"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason,omitempty"`
	Idempotent     bool       `json:"idempotent"`
	DraftMessageID string     `json:"draft_message_id,omitempty"`
	ToolCallCount  int        `json:"tool_call_count"`
	ErrorText      string     `json:"error_text,omitempty"`
}

type RunChatBufferFlushResult struct {
	Status          string                         `json:"status"`
	Reason          string                         `json:"reason,omitempty"`
	ObservedAt      time.Time                      `json:"observed_at"`
	JobRunID        string                         `json:"job_run_id,omitempty"`
	Filter          RunChatBufferFlushInput        `json:"filter"`
	CheckedCount    int                            `json:"checked_count"`
	DueCount        int                            `json:"due_count"`
	FlushedCount    int                            `json:"flushed_count"`
	FailedCount     int                            `json:"failed_count"`
	FlushedSessions []ChatBufferFlushSessionResult `json:"flushed_sessions,omitempty"`
}

type ChatAutoSendRetrySessionResult struct {
	SessionID        string     `json:"session_id"`
	ContactKey       string     `json:"contact_key,omitempty"`
	LastAttemptAt    *time.Time `json:"last_attempt_at,omitempty"`
	RetryRequestedAt *time.Time `json:"retry_requested_at,omitempty"`
	Status           string     `json:"status"`
	Reason           string     `json:"reason,omitempty"`
	Idempotent       bool       `json:"idempotent"`
	DraftMessageID   string     `json:"draft_message_id,omitempty"`
	OutboundID       string     `json:"outbound_id,omitempty"`
	ErrorText        string     `json:"error_text,omitempty"`
}

type RunChatAutoSendRetryResult struct {
	Status          string                           `json:"status"`
	Reason          string                           `json:"reason,omitempty"`
	ObservedAt      time.Time                        `json:"observed_at"`
	JobRunID        string                           `json:"job_run_id,omitempty"`
	Filter          RunChatAutoSendRetryInput        `json:"filter"`
	CheckedCount    int                              `json:"checked_count"`
	DueCount        int                              `json:"due_count"`
	RetriedCount    int                              `json:"retried_count"`
	BlockedCount    int                              `json:"blocked_count"`
	SkippedCount    int                              `json:"skipped_count"`
	FailedCount     int                              `json:"failed_count"`
	RetriedSessions []ChatAutoSendRetrySessionResult `json:"retried_sessions,omitempty"`
}

type ExpiredBookingResult struct {
	BookingID          string    `json:"booking_id"`
	ReservationCode    string    `json:"reservation_code,omitempty"`
	TripID             string    `json:"trip_id,omitempty"`
	PreviousStatus     string    `json:"previous_status,omitempty"`
	UpdatedStatus      string    `json:"updated_status,omitempty"`
	EffectiveExpiresAt time.Time `json:"effective_expires_at"`
	ExpirationSource   string    `json:"expiration_source"`
}

type RunBookingsExpireResult struct {
	Status          string                 `json:"status"`
	Reason          string                 `json:"reason,omitempty"`
	ObservedAt      time.Time              `json:"observed_at"`
	JobRunID        string                 `json:"job_run_id,omitempty"`
	Limit           int                    `json:"limit"`
	HoldMinutes     int                    `json:"hold_minutes"`
	CheckedCount    int                    `json:"checked_count"`
	ExpiredCount    int                    `json:"expired_count"`
	ExpiredBookings []ExpiredBookingResult `json:"expired_bookings,omitempty"`
}

type RunPaymentNotificationsInput struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Channel   string `json:"channel"`
}

type RunPaymentNotificationsResult struct {
	Status          string    `json:"status"`
	Reason          string    `json:"reason,omitempty"`
	DraftQueued     bool      `json:"draft_queued"`
	Idempotent      bool      `json:"idempotent"`
	ObservedAt      time.Time `json:"observed_at"`
	JobRunID        string    `json:"job_run_id,omitempty"`
	PaymentID       string    `json:"payment_id,omitempty"`
	BookingID       string    `json:"booking_id,omitempty"`
	ReservationCode string    `json:"reservation_code,omitempty"`
	CustomerName    string    `json:"customer_name,omitempty"`
	CustomerPhone   string    `json:"customer_phone,omitempty"`
	ContactKey      string    `json:"contact_key,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	DraftMessageID  string    `json:"draft_message_id,omitempty"`
	PaymentStatus   string    `json:"payment_status,omitempty"`
	AmountPaid      float64   `json:"amount_paid,omitempty"`
	AmountDue       float64   `json:"amount_due,omitempty"`
	Messages        []string  `json:"messages,omitempty"`
}

type ChatReviewAlertNotificationPayload struct {
	Source            string                         `json:"source"`
	ObservedAt        time.Time                      `json:"observed_at"`
	AlertLevel        string                         `json:"alert_level"`
	AlertCode         string                         `json:"alert_code"`
	AlertMessage      string                         `json:"alert_message,omitempty"`
	AlertSessionCount int                            `json:"alert_session_count"`
	Filter            RunChatReviewAlertsInput       `json:"filter"`
	Summary           ChatReviewAlertSummarySnapshot `json:"summary"`
}

type ChatReviewAlertSummarySnapshot struct {
	TotalCount                int    `json:"total_count"`
	PendingReviewCount        int    `json:"pending_review_count"`
	ReviewedCount             int    `json:"reviewed_count"`
	NoDraftCount              int    `json:"no_draft_count"`
	HumanOwnedCount           int    `json:"human_owned_count"`
	BotOwnedCount             int    `json:"bot_owned_count"`
	DueSoonReviewCount        int    `json:"due_soon_review_count"`
	OverdueReviewCount        int    `json:"overdue_review_count"`
	HighPriorityReviewCount   int    `json:"high_priority_review_count"`
	MediumPriorityReviewCount int    `json:"medium_priority_review_count"`
	LowPriorityReviewCount    int    `json:"low_priority_review_count"`
	OldestPendingAgeSeconds   int    `json:"oldest_pending_age_seconds"`
	ReviewSLASeconds          int    `json:"review_sla_seconds"`
	HasReviewAlert            bool   `json:"has_review_alert"`
	ReviewAlertLevel          string `json:"review_alert_level,omitempty"`
	ReviewAlertCode           string `json:"review_alert_code,omitempty"`
	ReviewAlertMessage        string `json:"review_alert_message,omitempty"`
	ReviewAlertSessionCount   int    `json:"review_alert_session_count"`
}

type CreateJobRunInput struct {
	JobName       string
	TriggerSource string
	Status        string
	InputPayload  map[string]interface{}
	ResultPayload map[string]interface{}
	ErrorText     string
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type UpdateJobRunInput struct {
	ID            string
	Status        string
	ResultPayload map[string]interface{}
	ErrorText     string
	FinishedAt    time.Time
}

type EvolutionEnvelope struct {
	Body json.RawMessage `json:"body"`
}

type EvolutionWebhookPayload struct {
	Event       string               `json:"event"`
	Instance    string               `json:"instance"`
	Data        EvolutionMessageData `json:"data"`
	Destination string               `json:"destination"`
	DateTime    string               `json:"date_time"`
	Sender      string               `json:"sender"`
	ServerURL   string               `json:"server_url"`
	APIKey      string               `json:"apikey"`
}

type EvolutionMessageData struct {
	Key                    EvolutionMessageKey     `json:"key"`
	ID                     string                  `json:"id"`
	KeyID                  string                  `json:"keyId"`
	PushName               string                  `json:"pushName"`
	Status                 string                  `json:"status"`
	Presences              map[string]interface{}  `json:"presences"`
	Message                EvolutionMessageContent `json:"message"`
	MessageType            string                  `json:"messageType"`
	MessageTimestamp       int64                   `json:"messageTimestamp"`
	InstanceID             string                  `json:"instanceId"`
	Source                 string                  `json:"source"`
	ChatwootMessageID      int64                   `json:"chatwootMessageId"`
	ChatwootInboxID        int64                   `json:"chatwootInboxId"`
	ChatwootConversationID int64                   `json:"chatwootConversationId"`
}

type EvolutionMessageKey struct {
	RemoteJID      string `json:"remoteJid"`
	RemoteJIDAlt   string `json:"remoteJidAlt"`
	FromMe         bool   `json:"fromMe"`
	ID             string `json:"id"`
	Participant    string `json:"participant"`
	AddressingMode string `json:"addressingMode"`
}

type EvolutionMessageContent struct {
	Conversation        string                        `json:"conversation"`
	ExtendedTextMessage *EvolutionExtendedTextMessage `json:"extendedTextMessage"`
	ImageMessage        *EvolutionMediaMessage        `json:"imageMessage"`
	VideoMessage        *EvolutionMediaMessage        `json:"videoMessage"`
	AudioMessage        map[string]interface{}        `json:"audioMessage"`
	DocumentMessage     map[string]interface{}        `json:"documentMessage"`
}

type EvolutionExtendedTextMessage struct {
	Text string `json:"text"`
}

type EvolutionMediaMessage struct {
	Caption    string      `json:"caption"`
	URL        string      `json:"url"`
	DirectPath string      `json:"directPath"`
	MimeType   string      `json:"mimetype"`
	FileLength interface{} `json:"fileLength"`
}

type EvolutionWebhookResult struct {
	Status            string `json:"status"`
	Event             string `json:"event,omitempty"`
	Direction         string `json:"direction,omitempty"`
	ContactKey        string `json:"contact_key,omitempty"`
	CustomerPhone     string `json:"customer_phone,omitempty"`
	ProviderMessageID string `json:"provider_message_id,omitempty"`
	MessageType       string `json:"message_type,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	MessageID         string `json:"message_id,omitempty"`
	Idempotent        bool   `json:"idempotent,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type EvolutionStatusWebhookResult struct {
	Status                  string `json:"status"`
	Event                   string `json:"event,omitempty"`
	ProviderMessageID       string `json:"provider_message_id,omitempty"`
	ProviderStatus          string `json:"provider_status,omitempty"`
	MessageType             string `json:"message_type,omitempty"`
	MatchedChatMessages     int64  `json:"matched_chat_messages,omitempty"`
	MatchedOutboundMessages int64  `json:"matched_outbound_messages,omitempty"`
	Reason                  string `json:"reason,omitempty"`
}

type EvolutionPresenceWebhookResult struct {
	Status        string `json:"status"`
	Event         string `json:"event,omitempty"`
	ContactKey    string `json:"contact_key,omitempty"`
	CustomerPhone string `json:"customer_phone,omitempty"`
	PresenceState string `json:"presence_state,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type RecordEvolutionStatusInput struct {
	ProviderMessageID string
	ProviderStatus    string
	Event             string
	ObservedAt        time.Time
	Payload           map[string]interface{}
}

type RecordEvolutionStatusResult struct {
	MatchedChatMessages     int64
	MatchedOutboundMessages int64
}
