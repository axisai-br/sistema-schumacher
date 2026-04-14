package affiliate

import "time"

type BalanceResponse struct {
	AvailableAmount     int64             `json:"available_amount"`
	WaitingFundsAmount  int64             `json:"waiting_funds_amount"`
	TransferredAmount   int64             `json:"transferred_amount"`
	Currency            string            `json:"currency"`
	CanWithdraw         bool              `json:"can_withdraw"`
	WithdrawBlockReason string            `json:"withdraw_block_reason,omitempty"`
	AnticipationInfo    *AnticipationInfo `json:"anticipation_info,omitempty"`
}

type AnticipationInfo struct {
	Enabled          bool   `json:"enabled"`
	Type             string `json:"type,omitempty"`
	Delay            int64  `json:"delay,omitempty"`
	VolumePercentage int64  `json:"volume_percentage,omitempty"`
}

type WithdrawRequest struct {
	Amount int64 `json:"amount"`
}

type WithdrawResponse struct {
	WithdrawalID    string `json:"withdrawal_id"`
	TransferID      string `json:"transfer_id,omitempty"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	RequestedAmount int64  `json:"requested_amount"`
	Currency        string `json:"currency"`
}

type WithdrawalHistoryItem struct {
	ID          string     `json:"id"`
	Amount      int64      `json:"amount"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"`
	TransferID  *string    `json:"transfer_id,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

type WithdrawalsHistoryResponse struct {
	Items      []WithdrawalHistoryItem `json:"items"`
	Pagination Pagination              `json:"pagination"`
}

type Pagination struct {
	Limit         int `json:"limit"`
	Offset        int `json:"offset"`
	TotalEstimate int `json:"total_estimate"`
}

type AuthorizationContext struct {
	UserID       string
	HasRole      bool
	RecipientID  string
	HasRecipient bool
}

type ListFilter struct {
	Limit  int
	Offset int
}

type WithdrawalRecord struct {
	ID                  string
	UserID              string
	RecipientID         string
	AmountCents         int64
	Currency            string
	Status              string
	TransferID          *string
	IdempotencyKey      string
	ProviderRequestRaw  []byte
	ProviderResponseRaw []byte
	ProviderError       *string
	RequestedAt         time.Time
	ProcessedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type WebhookEventRecord struct {
	ID            string
	EventID       *string
	EventType     *string
	TransferID    *string
	Signature     *string
	PayloadHash   string
	ProcessStatus string
	ReceivedAt    time.Time
}
