package chat

import (
	"context"
	"time"
)

type Session struct {
	ID                      string                 `json:"id"`
	Channel                 string                 `json:"channel"`
	ContactKey              string                 `json:"contact_key"`
	CustomerPhone           string                 `json:"customer_phone,omitempty"`
	CustomerName            string                 `json:"customer_name,omitempty"`
	Status                  string                 `json:"status"`
	HandoffStatus           string                 `json:"handoff_status"`
	CurrentOwnerUserID      string                 `json:"current_owner_user_id,omitempty"`
	LastMessageAt           *time.Time             `json:"last_message_at,omitempty"`
	LastInboundAt           *time.Time             `json:"last_inbound_at,omitempty"`
	LastOutboundAt          *time.Time             `json:"last_outbound_at,omitempty"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
	AgentStatus             string                 `json:"agent_status,omitempty"`
	HasAutomationDraft      bool                   `json:"has_automation_draft,omitempty"`
	DraftReviewStatus       string                 `json:"draft_review_status,omitempty"`
	DraftIdempotencyKey     string                 `json:"draft_idempotency_key,omitempty"`
	DraftGeneratedAt        *time.Time             `json:"draft_generated_at,omitempty"`
	DraftReviewedAt         *time.Time             `json:"draft_reviewed_at,omitempty"`
	DraftReviewedByUserID   string                 `json:"draft_reviewed_by_user_id,omitempty"`
	DraftReviewAction       string                 `json:"draft_review_action,omitempty"`
	DraftToolNames          []string               `json:"draft_tool_names,omitempty"`
	DraftToolCallCount      int                    `json:"draft_tool_call_count,omitempty"`
	DraftModel              string                 `json:"draft_model,omitempty"`
	DraftProviderResponseID string                 `json:"draft_provider_response_id,omitempty"`
	DraftReviewSLASeconds   int                    `json:"draft_review_sla_seconds,omitempty"`
	DraftPendingAgeSeconds  int                    `json:"draft_pending_age_seconds,omitempty"`
	DraftPendingAgeBucket   string                 `json:"draft_pending_age_bucket,omitempty"`
	DraftReviewPriority     string                 `json:"draft_review_priority,omitempty"`
	DraftAutoSendStatus     string                 `json:"draft_auto_send_status,omitempty"`
	DraftAutoSendReasons    []string               `json:"draft_auto_send_reasons,omitempty"`
	DraftReviewAlertActive  bool                   `json:"draft_review_alert_active,omitempty"`
	DraftReviewAlertLevel   string                 `json:"draft_review_alert_level,omitempty"`
	DraftReviewAlertCode    string                 `json:"draft_review_alert_code,omitempty"`
	DraftReviewAlertMessage string                 `json:"draft_review_alert_message,omitempty"`
	DraftReviewOverdue      bool                   `json:"draft_review_overdue,omitempty"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
}

type Message struct {
	ID                string                 `json:"id"`
	SessionID         string                 `json:"session_id"`
	Direction         string                 `json:"direction"`
	Kind              string                 `json:"kind"`
	ProviderMessageID string                 `json:"provider_message_id,omitempty"`
	IdempotencyKey    string                 `json:"idempotency_key,omitempty"`
	SenderName        string                 `json:"sender_name,omitempty"`
	SenderPhone       string                 `json:"sender_phone,omitempty"`
	Body              string                 `json:"body,omitempty"`
	Payload           map[string]interface{} `json:"payload,omitempty"`
	NormalizedPayload map[string]interface{} `json:"normalized_payload,omitempty"`
	ProcessingStatus  string                 `json:"processing_status"`
	ReceivedAt        time.Time              `json:"received_at"`
	SentAt            *time.Time             `json:"sent_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

type ToolCall struct {
	ID              string                 `json:"id"`
	SessionID       string                 `json:"session_id"`
	MessageID       string                 `json:"message_id,omitempty"`
	ToolName        string                 `json:"tool_name"`
	RequestPayload  map[string]interface{} `json:"request_payload,omitempty"`
	ResponsePayload map[string]interface{} `json:"response_payload,omitempty"`
	Status          string                 `json:"status"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	StartedAt       time.Time              `json:"started_at"`
	FinishedAt      *time.Time             `json:"finished_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

type Handoff struct {
	ID             string                 `json:"id"`
	SessionID      string                 `json:"session_id"`
	RequestedBy    string                 `json:"requested_by"`
	Reason         string                 `json:"reason,omitempty"`
	Status         string                 `json:"status"`
	AssignedUserID string                 `json:"assigned_user_id,omitempty"`
	RequestedAt    time.Time              `json:"requested_at"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type IngestMessageInput struct {
	Channel       string                 `json:"channel"`
	ContactKey    string                 `json:"contact_key"`
	CustomerPhone string                 `json:"customer_phone"`
	CustomerName  string                 `json:"customer_name"`
	Metadata      map[string]interface{} `json:"metadata"`
	Message       IngestMessagePayload   `json:"message"`
}

type IngestMessagePayload struct {
	Direction         string                 `json:"direction"`
	Kind              string                 `json:"kind"`
	ProviderMessageID string                 `json:"provider_message_id"`
	IdempotencyKey    string                 `json:"idempotency_key"`
	SenderName        string                 `json:"sender_name"`
	SenderPhone       string                 `json:"sender_phone"`
	Body              string                 `json:"body"`
	Payload           map[string]interface{} `json:"payload"`
	NormalizedPayload map[string]interface{} `json:"normalized_payload"`
	ProcessingStatus  string                 `json:"processing_status"`
	ReceivedAt        *time.Time             `json:"received_at"`
	SentAt            *time.Time             `json:"sent_at"`
}

type UpdateMessageInput struct {
	MessageID         string                 `json:"message_id"`
	Body              string                 `json:"body"`
	NormalizedPayload map[string]interface{} `json:"normalized_payload"`
	ProcessingStatus  string                 `json:"processing_status"`
}

type IngestMessageResult struct {
	Session    Session `json:"session"`
	Message    Message `json:"message"`
	Idempotent bool    `json:"idempotent"`
}

type QueueAutomationDraftInput struct {
	Channel        string                 `json:"channel"`
	ContactKey     string                 `json:"contact_key"`
	CustomerPhone  string                 `json:"customer_phone"`
	CustomerName   string                 `json:"customer_name"`
	Body           string                 `json:"body"`
	SenderName     string                 `json:"sender_name"`
	IdempotencyKey string                 `json:"idempotency_key"`
	Metadata       map[string]interface{} `json:"metadata"`
}

type QueueAutomationDraftResult struct {
	Session    Session `json:"session"`
	Message    Message `json:"message"`
	Idempotent bool    `json:"idempotent"`
}

type ListSessionsFilter struct {
	Limit               int
	Offset              int
	Channel             string
	Status              string
	HandoffStatus       string
	ContactKey          string
	AgentStatus         string
	DraftReviewStatus   string
	DraftAutoSendStatus string
	OrderBy             string
	ReviewSLASeconds    int
}

type SessionsSummary struct {
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
	AutoSendRetryPendingCount int    `json:"auto_send_retry_pending_count"`
	AutoSendBlockedHumanCount int    `json:"auto_send_blocked_human_count"`
	AutoSendIssueCount        int    `json:"auto_send_issue_count"`
	ReviewSLASeconds          int    `json:"review_sla_seconds"`
	HasReviewAlert            bool   `json:"has_review_alert"`
	ReviewAlertLevel          string `json:"review_alert_level,omitempty"`
	ReviewAlertCode           string `json:"review_alert_code,omitempty"`
	ReviewAlertMessage        string `json:"review_alert_message,omitempty"`
	ReviewAlertSessionCount   int    `json:"review_alert_session_count"`
}

type ListMessagesFilter struct {
	Limit  int
	Offset int
}

type StructuredInterpreterShadowReportFilter struct {
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	SessionID string `json:"session_id,omitempty"`
}

type StructuredInterpreterShadowReportResponse struct {
	Filter                StructuredInterpreterShadowReportFilter `json:"filter"`
	LoadedMessageCount    int                                     `json:"loaded_message_count"`
	ReportItemCount       int                                     `json:"report_item_count"`
	AssistReportItemCount int                                     `json:"assist_report_item_count"`
	Report                StructuredInterpreterShadowReport       `json:"report"`
}

type UpsertSessionInput struct {
	Channel        string
	ContactKey     string
	CustomerPhone  string
	CustomerName   string
	LastMessageAt  *time.Time
	LastInboundAt  *time.Time
	LastOutboundAt *time.Time
	Metadata       map[string]interface{}
}

type CreateMessageInput struct {
	SessionID         string
	Direction         string
	Kind              string
	ProviderMessageID string
	IdempotencyKey    string
	SenderName        string
	SenderPhone       string
	Body              string
	Payload           map[string]interface{}
	NormalizedPayload map[string]interface{}
	ProcessingStatus  string
	ReceivedAt        time.Time
	SentAt            *time.Time
}

type UpdateSessionBufferStateInput struct {
	SessionID string
	Buffer    map[string]interface{}
}

type UpdateSessionMetadataInput struct {
	SessionID string
	Metadata  map[string]interface{}
}

type RequestHandoffInput struct {
	SessionID      string                 `json:"-"`
	RequestedBy    string                 `json:"requested_by"`
	Reason         string                 `json:"reason"`
	AssignedUserID string                 `json:"assigned_user_id"`
	Metadata       map[string]interface{} `json:"metadata"`
}

type RequestHandoffResult struct {
	Session Session `json:"session"`
	Handoff Handoff `json:"handoff"`
}

type ResumeSessionInput struct {
	SessionID string                 `json:"-"`
	ResumedBy string                 `json:"resumed_by"`
	Reason    string                 `json:"reason"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type ResumeSessionResult struct {
	Session Session `json:"session"`
	Handoff Handoff `json:"handoff"`
}

type ResolveSessionInput struct {
	SessionID        string                 `json:"-"`
	ResolvedByUserID string                 `json:"resolved_by_user_id"`
	ResolveReason    string                 `json:"reason"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type ResolveSessionResult struct {
	Session Session `json:"session"`
	Status  string  `json:"status"`
	Reason  string  `json:"reason,omitempty"`
}

type ReplyOutbound struct {
	ID                string                 `json:"id"`
	SessionID         string                 `json:"session_id,omitempty"`
	Channel           string                 `json:"channel"`
	Recipient         string                 `json:"recipient"`
	Payload           map[string]interface{} `json:"payload,omitempty"`
	Provider          string                 `json:"provider"`
	ProviderMessageID string                 `json:"provider_message_id,omitempty"`
	IdempotencyKey    string                 `json:"idempotency_key"`
	Status            string                 `json:"status"`
	SentAt            *time.Time             `json:"sent_at,omitempty"`
	DeliveredAt       *time.Time             `json:"delivered_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type ReplyInput struct {
	SessionID      string                 `json:"-"`
	OwnerUserID    string                 `json:"owner_user_id"`
	DraftMessageID string                 `json:"draft_message_id"`
	Body           string                 `json:"body"`
	SenderName     string                 `json:"sender_name"`
	IdempotencyKey string                 `json:"idempotency_key"`
	Metadata       map[string]interface{} `json:"metadata"`
}

type ReplyMediaInput struct {
	SessionID      string
	OwnerUserID    string
	SenderName     string
	IdempotencyKey string
	Caption        string
	MediaType      string
	FileName       string
	MimeType       string
	FileContent    []byte
	Metadata       map[string]interface{}
}

type ReplyMediaResult = ReplyResult

type CreateAutomationReplyInput struct {
	SessionID      string
	DraftMessageID string
	IdempotencyKey string
	SenderName     string
	Metadata       map[string]interface{}
}

type UpdateDraftAutoSendStateInput struct {
	SessionID       string
	DraftMessageID  string
	AutoSendStatus  string
	AutoSendReasons []string
	Payload         map[string]interface{}
	Agent           map[string]interface{}
}

type ReplyResult struct {
	Session    Session       `json:"session"`
	Message    Message       `json:"message"`
	Outbound   ReplyOutbound `json:"outbound"`
	Draft      *Message      `json:"draft,omitempty"`
	Idempotent bool          `json:"idempotent"`
}

type ReprocessInput struct {
	SessionID string                 `json:"-"`
	Trigger   string                 `json:"trigger"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type ReprocessResult struct {
	Session    Session                `json:"session"`
	Status     string                 `json:"status"`
	Reason     string                 `json:"reason,omitempty"`
	Memory     map[string]interface{} `json:"memory,omitempty"`
	Messages   []Message              `json:"messages,omitempty"`
	ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`
	Draft      *Message               `json:"draft,omitempty"`
	Idempotent bool                   `json:"idempotent,omitempty"`
}

type RetryDraftAutoSendInput struct {
	SessionID   string                 `json:"-"`
	RequestedBy string                 `json:"requested_by"`
	Reason      string                 `json:"reason"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type RetryDraftAutoSendResult struct {
	Session    Session        `json:"session"`
	Status     string         `json:"status"`
	Reason     string         `json:"reason,omitempty"`
	Draft      *Message       `json:"draft,omitempty"`
	Message    *Message       `json:"message,omitempty"`
	Outbound   *ReplyOutbound `json:"outbound,omitempty"`
	Idempotent bool           `json:"idempotent,omitempty"`
}

type CurrentDraftResult struct {
	Session                    Session                  `json:"session"`
	Draft                      Message                  `json:"draft"`
	LinkedReply                *Message                 `json:"linked_reply,omitempty"`
	DraftStatus                string                   `json:"draft_status"`
	AgentStatus                string                   `json:"agent_status,omitempty"`
	DraftIdempotencyKey        string                   `json:"draft_idempotency_key,omitempty"`
	GeneratedAt                *time.Time               `json:"generated_at,omitempty"`
	ReviewedAt                 *time.Time               `json:"reviewed_at,omitempty"`
	ReviewedByUserID           string                   `json:"reviewed_by_user_id,omitempty"`
	ReviewMode                 string                   `json:"review_mode,omitempty"`
	ReviewAction               string                   `json:"review_action,omitempty"`
	Model                      string                   `json:"model,omitempty"`
	ProviderResponseID         string                   `json:"provider_response_id,omitempty"`
	AutoSendStatus             string                   `json:"auto_send_status,omitempty"`
	AutoSendReasons            []string                 `json:"auto_send_reasons,omitempty"`
	AutoSendIssueActive        bool                     `json:"auto_send_issue_active,omitempty"`
	AutoSendLastAttemptAt      *time.Time               `json:"auto_send_last_attempt_at,omitempty"`
	AutoSendRetryAt            *time.Time               `json:"auto_send_retry_at,omitempty"`
	AutoSendRetryRequestedAt   *time.Time               `json:"auto_send_retry_requested_at,omitempty"`
	AutoSendRetryRequestedBy   string                   `json:"auto_send_retry_requested_by,omitempty"`
	AutoSendRetryRequestCount  int                      `json:"auto_send_retry_request_count,omitempty"`
	AutoSendRetryRequestReason string                   `json:"auto_send_retry_request_reason,omitempty"`
	AutoSendBlockedAt          *time.Time               `json:"auto_send_blocked_at,omitempty"`
	AutoSendLastErrorText      string                   `json:"auto_send_last_error_text,omitempty"`
	AutoSendBlockReason        string                   `json:"auto_send_block_reason,omitempty"`
	AutoSendLastReplyID        string                   `json:"auto_send_last_reply_message_id,omitempty"`
	AutoSendLastOutboundID     string                   `json:"auto_send_last_outbound_id,omitempty"`
	CurrentTurnMessageIDs      []string                 `json:"current_turn_message_ids,omitempty"`
	ToolNames                  []string                 `json:"tool_names,omitempty"`
	ToolCallCount              int                      `json:"tool_call_count,omitempty"`
	ToolCalls                  []map[string]interface{} `json:"tool_calls,omitempty"`
	ToolContext                map[string]interface{}   `json:"tool_context,omitempty"`
}

type SaveReprocessSnapshotInput struct {
	SessionID       string
	MessageIDs      []string
	MessageStatus   string
	MessageMetadata map[string]interface{}
	Memory          map[string]interface{}
	Agent           map[string]interface{}
	Buffer          map[string]interface{}
}

type SaveReprocessSnapshotResult struct {
	Session  Session
	Messages []Message
}

type SaveAgentDraftInput struct {
	SessionID         string
	IdempotencyKey    string
	Body              string
	SenderName        string
	ProcessingStatus  string
	Payload           map[string]interface{}
	NormalizedPayload map[string]interface{}
	Agent             map[string]interface{}
	Buffer            map[string]interface{}
	RecordedAt        time.Time
}

type SaveAgentDraftResult struct {
	Session Session
	Message Message
}

type CreateToolCallInput struct {
	SessionID       string
	MessageID       string
	ToolName        string
	RequestPayload  map[string]interface{}
	ResponsePayload map[string]interface{}
	Status          string
	ErrorCode       string
	ErrorMessage    string
	StartedAt       time.Time
	FinishedAt      *time.Time
}

type MarkReplyDeliverySentInput struct {
	SessionID         string
	MessageID         string
	OutboundID        string
	ProviderMessageID string
	ProviderStatus    string
	Payload           map[string]interface{}
	SentAt            time.Time
}

type MarkReplyDeliveryFailureInput struct {
	SessionID  string
	MessageID  string
	OutboundID string
	ErrorText  string
}

type ReplySender interface {
	Enabled() bool
	SendReply(ctx context.Context, input SendReplyInput) (SendReplyResult, error)
}

type SendReplyInput struct {
	Session  Session
	Message  Message
	Outbound ReplyOutbound
}

type SendReplyResult struct {
	ProviderMessageID string
	ProviderStatus    string
	Payload           map[string]interface{}
	SentAt            time.Time
}

type AgentRunner interface {
	Enabled() bool
	Run(ctx context.Context, input RunAgentInput) (RunAgentResult, error)
}

type AgentMediaInput struct {
	Kind      string
	URL       string
	MimeType  string
	MessageID string
	FileName  string
}

type RunAgentInput struct {
	Session          Session
	CurrentTurnIDs   []string
	CurrentTurnMedia []AgentMediaInput
	SystemPrompt     string
	UserPrompt       string
	IdempotencyKey   string
	TextFormat       map[string]interface{}
}

type RunAgentResult struct {
	ReplyText              string
	Model                  string
	ProviderResponseID     string
	ProviderConversationID string
	RequestPayload         map[string]interface{}
	ResponsePayload        map[string]interface{}
}

type AvailabilitySearcher interface {
	Enabled() bool
	Search(ctx context.Context, input AvailabilitySearchInput) (AvailabilitySearchResult, error)
}

type AvailabilitySearchInput struct {
	Origin            string
	Destination       string
	OriginStopID      string
	DestinationStopID string
	RouteID           string
	PackageName       string
	TripDate          *time.Time
	DateFrom          *time.Time
	DateTo            *time.Time
	Qty               int
	Limit             int
}

type AvailabilitySearchResult struct {
	Filter  AvailabilitySearchInput
	Results []AvailabilitySearchItem
}

type AvailabilitySearchItem struct {
	SegmentID              string  `json:"segment_id"`
	TripID                 string  `json:"trip_id"`
	RouteID                string  `json:"route_id"`
	BoardStopID            string  `json:"board_stop_id"`
	AlightStopID           string  `json:"alight_stop_id"`
	OriginStopID           string  `json:"origin_stop_id"`
	DestinationStopID      string  `json:"destination_stop_id"`
	OriginDisplayName      string  `json:"origin_display_name"`
	DestinationDisplayName string  `json:"destination_display_name"`
	OriginDepartTime       string  `json:"origin_depart_time"`
	TripDate               string  `json:"trip_date"`
	SeatsAvailable         int     `json:"seats_available"`
	Price                  float64 `json:"price"`
	Currency               string  `json:"currency"`
	Status                 string  `json:"status"`
	TripStatus             string  `json:"trip_status"`
	PackageName            string  `json:"package_name,omitempty"`
}

type PricingQuoteSearcher interface {
	Enabled() bool
	Search(ctx context.Context, input PricingQuoteInput) (PricingQuoteResult, error)
}

type PricingQuoteInput struct {
	FareMode   string
	Candidates []PricingQuoteCandidate
}

type PricingQuoteCandidate struct {
	TripID                 string `json:"trip_id"`
	RouteID                string `json:"route_id,omitempty"`
	BoardStopID            string `json:"board_stop_id"`
	AlightStopID           string `json:"alight_stop_id"`
	OriginStopID           string `json:"origin_stop_id,omitempty"`
	DestinationStopID      string `json:"destination_stop_id,omitempty"`
	OriginDisplayName      string `json:"origin_display_name,omitempty"`
	DestinationDisplayName string `json:"destination_display_name,omitempty"`
	OriginDepartTime       string `json:"origin_depart_time,omitempty"`
	TripDate               string `json:"trip_date,omitempty"`
}

type PricingQuoteResult struct {
	Filter  PricingQuoteInput
	Results []PricingQuoteItem
}

type PricingQuoteItem struct {
	TripID                 string  `json:"trip_id"`
	RouteID                string  `json:"route_id"`
	BoardStopID            string  `json:"board_stop_id"`
	AlightStopID           string  `json:"alight_stop_id"`
	OriginStopID           string  `json:"origin_stop_id"`
	DestinationStopID      string  `json:"destination_stop_id"`
	OriginDisplayName      string  `json:"origin_display_name,omitempty"`
	DestinationDisplayName string  `json:"destination_display_name,omitempty"`
	OriginDepartTime       string  `json:"origin_depart_time,omitempty"`
	TripDate               string  `json:"trip_date,omitempty"`
	BaseAmount             float64 `json:"base_amount"`
	CalcAmount             float64 `json:"calc_amount"`
	FinalAmount            float64 `json:"final_amount"`
	Currency               string  `json:"currency"`
	FareMode               string  `json:"fare_mode"`
}

type BookingLookupSearcher interface {
	Enabled() bool
	Search(ctx context.Context, input BookingLookupInput) (BookingLookupResult, error)
}

type BookingLookupInput struct {
	BookingID       string
	ReservationCode string
	Limit           int
}

type BookingLookupResult struct {
	Filter  BookingLookupInput
	Results []BookingLookupItem
}

type BookingLookupItem struct {
	ID              string     `json:"id"`
	TripID          string     `json:"trip_id"`
	Status          string     `json:"status"`
	ReservationCode string     `json:"reservation_code"`
	TotalAmount     float64    `json:"total_amount"`
	DepositAmount   float64    `json:"deposit_amount"`
	RemainderAmount float64    `json:"remainder_amount"`
	PassengerName   string     `json:"passenger_name"`
	PassengerPhone  string     `json:"passenger_phone"`
	SeatNumber      int        `json:"seat_number"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type BookingCreator interface {
	Enabled() bool
	Create(ctx context.Context, input BookingCreateInput) (BookingCreateResult, error)
}

type BookingCreateInput struct {
	SelectedOptionIndex    int
	TripID                 string
	BoardStopID            string
	AlightStopID           string
	OriginDisplayName      string
	DestinationDisplayName string
	TripDate               string
	DepartureTime          string
	Qty                    int
	CustomerName           string
	CustomerPhone          string
	IdempotencyKey         string
	Passengers             []BookingCreatePassengerInput
}

type BookingCreatePassengerInput struct {
	Name                   string `json:"name"`
	Document               string `json:"document"`
	DocumentType           string `json:"document_type"`
	CPF                    string `json:"cpf,omitempty"`
	RG                     string `json:"rg,omitempty"`
	CNH                    string `json:"cnh,omitempty"`
	BirthDate              string `json:"birth_date,omitempty"`
	BirthCertificateNumber string `json:"birth_certificate_number,omitempty"`
	BirthCity              string `json:"birth_city,omitempty"`
	Phone                  string `json:"phone,omitempty"`
	Email                  string `json:"email,omitempty"`
	Notes                  string `json:"notes,omitempty"`
	IsLapChild             bool   `json:"is_lap_child,omitempty"`
}

type BookingCreateResult struct {
	Filter          BookingCreateInput             `json:"filter"`
	Mode            string                         `json:"mode"`
	BookingID       string                         `json:"booking_id,omitempty"`
	ReservationCode string                         `json:"reservation_code,omitempty"`
	Status          string                         `json:"status,omitempty"`
	ReservedUntil   *time.Time                     `json:"reserved_until,omitempty"`
	TotalAmount     float64                        `json:"total_amount,omitempty"`
	DepositAmount   float64                        `json:"deposit_amount,omitempty"`
	RemainderAmount float64                        `json:"remainder_amount,omitempty"`
	Passengers      []BookingCreatePassengerResult `json:"passengers,omitempty"`
	Errors          []string                       `json:"errors,omitempty"`
	MessageForAgent string                         `json:"message_for_agent,omitempty"`
}

type BookingCreatePassengerResult struct {
	Name                   string `json:"name"`
	Document               string `json:"document,omitempty"`
	DocumentType           string `json:"document_type,omitempty"`
	CPF                    string `json:"cpf,omitempty"`
	RG                     string `json:"rg,omitempty"`
	CNH                    string `json:"cnh,omitempty"`
	BirthDate              string `json:"birth_date,omitempty"`
	BirthCertificateNumber string `json:"birth_certificate_number,omitempty"`
	BirthCity              string `json:"birth_city,omitempty"`
	Phone                  string `json:"phone,omitempty"`
	SeatID                 string `json:"seat_id,omitempty"`
	IsLapChild             bool   `json:"is_lap_child,omitempty"`
}

type RescheduleAssistSearcher interface {
	Enabled() bool
	Search(ctx context.Context, input RescheduleAssistInput) (RescheduleAssistResult, error)
}

type RescheduleAssistInput struct {
	BookingID            string
	ReservationCode      string
	RequestedTripDate    *time.Time
	RequestedOrigin      string
	RequestedDestination string
	RequestedQty         int
}

type RescheduleAssistResult struct {
	Filter                            RescheduleAssistInput
	Mode                              string                   `json:"mode"`
	Booking                           *RescheduleAssistBooking `json:"booking,omitempty"`
	Current                           RescheduleAssistRoute    `json:"current,omitempty"`
	Requested                         RescheduleAssistRequest  `json:"requested,omitempty"`
	Options                           []RescheduleAssistOption `json:"options,omitempty"`
	Errors                            []string                 `json:"errors,omitempty"`
	NextStep                          string                   `json:"next_step,omitempty"`
	HumanReviewRequired               bool                     `json:"human_review_required"`
	CanAutoReschedule                 bool                     `json:"can_auto_reschedule"`
	FieldsRequiredForManualCompletion []string                 `json:"fields_required_for_manual_completion,omitempty"`
	MessageForAgent                   string                   `json:"message_for_agent,omitempty"`
}

type RescheduleAssistBooking struct {
	ID              string `json:"id"`
	TripID          string `json:"trip_id,omitempty"`
	Status          string `json:"status,omitempty"`
	ReservationCode string `json:"reservation_code,omitempty"`
}

type RescheduleAssistRoute struct {
	Origin         string `json:"origin,omitempty"`
	Destination    string `json:"destination,omitempty"`
	TripDate       string `json:"trip_date,omitempty"`
	PassengerCount int    `json:"passenger_count,omitempty"`
}

type RescheduleAssistRequest struct {
	Origin      string `json:"origin,omitempty"`
	Destination string `json:"destination,omitempty"`
	TripDate    string `json:"trip_date,omitempty"`
	Qty         int    `json:"qtd_passagens,omitempty"`
}

type RescheduleAssistOption struct {
	TripID         string  `json:"trip_id,omitempty"`
	TripDate       string  `json:"trip_date,omitempty"`
	DepartureTime  string  `json:"hora,omitempty"`
	Origin         string  `json:"origin,omitempty"`
	Destination    string  `json:"destination,omitempty"`
	BoardStopID    string  `json:"board_stop_id,omitempty"`
	AlightStopID   string  `json:"alight_stop_id,omitempty"`
	SeatsAvailable int     `json:"seats_available,omitempty"`
	Price          float64 `json:"price,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	PackageName    string  `json:"package_name,omitempty"`
}

type PaymentStatusSearcher interface {
	Enabled() bool
	Search(ctx context.Context, input PaymentStatusInput) (PaymentStatusResult, error)
}

type PaymentStatusInput struct {
	BookingID       string
	ReservationCode string
	Limit           int
}

type PaymentStatusResult struct {
	Filter  PaymentStatusInput
	Results []PaymentStatusItem
}

type PaymentStatusItem struct {
	ID          string     `json:"id"`
	BookingID   string     `json:"booking_id"`
	Amount      float64    `json:"amount"`
	Method      string     `json:"method"`
	Status      string     `json:"status"`
	Provider    string     `json:"provider,omitempty"`
	ProviderRef string     `json:"provider_ref,omitempty"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type PaymentCreator interface {
	Enabled() bool
	Create(ctx context.Context, input PaymentCreateInput) (PaymentCreateResult, error)
}

type PaymentCreateInput struct {
	BookingID              string
	ReservationCode        string
	PaymentType            string
	ConfirmPaid            bool
	PaidAmount             float64
	DepositPerPerson       float64
	CustomerName           string
	CustomerPhone          string
	CustomerDocument       string
	CustomerDocumentSource string
	CustomerEmail          string
	Note                   string
}

type PaymentCreateResult struct {
	Filter          PaymentCreateInput `json:"filter"`
	Mode            string             `json:"mode"`
	BookingID       string             `json:"booking_id,omitempty"`
	ReservationCode string             `json:"reservation_code,omitempty"`
	BookingStatus   string             `json:"booking_status,omitempty"`
	PaymentType     string             `json:"payment_type,omitempty"`
	Stage           string             `json:"stage,omitempty"`
	AmountTotal     float64            `json:"amount_total,omitempty"`
	AmountPaid      float64            `json:"amount_paid,omitempty"`
	AmountDue       float64            `json:"amount_due,omitempty"`
	PaymentID       string             `json:"payment_id,omitempty"`
	PaymentStatus   string             `json:"payment_status,omitempty"`
	Provider        string             `json:"provider,omitempty"`
	ProviderRef     string             `json:"provider_ref,omitempty"`
	PixCode         string             `json:"pix_code,omitempty"`
	CheckoutURL     string             `json:"checkout_url,omitempty"`
	Errors          []string           `json:"errors,omitempty"`
	MessageForAgent string             `json:"message_for_agent,omitempty"`
}

type BookingCanceler interface {
	Enabled() bool
	Cancel(ctx context.Context, input BookingCancelInput) (BookingCancelResult, error)
}

type BookingCancelInput struct {
	BookingID       string
	ReservationCode string
	Reason          string
	Actor           string
}

type BookingCancelResult struct {
	Filter          BookingCancelInput `json:"filter"`
	Mode            string             `json:"mode"`
	BookingID       string             `json:"booking_id,omitempty"`
	ReservationCode string             `json:"reservation_code,omitempty"`
	TripID          string             `json:"trip_id,omitempty"`
	BookingStatus   string             `json:"booking_status,omitempty"`
	PreviousStatus  string             `json:"previous_status,omitempty"`
	Reason          string             `json:"reason,omitempty"`
	Actor           string             `json:"actor,omitempty"`
	PassengerCount  int                `json:"passenger_count,omitempty"`
	Idempotent      bool               `json:"idempotent,omitempty"`
	Errors          []string           `json:"errors,omitempty"`
	MessageForAgent string             `json:"message_for_agent,omitempty"`
}

type ApplyPresenceSignalInput struct {
	Channel        string
	ContactKey     string
	PresenceStatus string
	ObservedAt     *time.Time
	Metadata       map[string]interface{}
}

type ApplyPresenceSignalResult struct {
	Session       Session `json:"session"`
	Status        string  `json:"status"`
	PresenceState string  `json:"presence_state,omitempty"`
	Reason        string  `json:"reason,omitempty"`
}
