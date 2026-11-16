package domain

import "time"

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

type Priority string

const (
	PriorityLow  Priority = "low"
	PriorityHigh Priority = "high"
)

type Status string

const (
	StatusScheduled Status = "scheduled" // waiting for its send time
	StatusQueued    Status = "queued"    // published to the broker, waiting for a worker
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed" // all attempts used
	StatusCancelled Status = "cancelled"
)

// MaxAttempts is the number of delivery attempts before a notification fails.
const MaxAttempts = 3

const ContentMaxLength = 500

type Notification struct {
	ID          int64      `json:"id"`
	Channel     Channel    `json:"channel"`
	Content     string     `json:"content"`
	Recipient   string     `json:"recipient"`
	Timezone    string     `json:"timezone"`
	Priority    Priority   `json:"priority"`
	Status      Status     `json:"status"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

// NotificationInput is the payload used to create a notification. Priority
// defaults to low and SendAt to now.
type NotificationInput struct {
	Channel   Channel    `json:"channel"`
	Content   string     `json:"content"`
	Recipient string     `json:"recipient"`
	Timezone  string     `json:"timezone"`
	Priority  Priority   `json:"priority"`
	SendAt    *time.Time `json:"send_at"`
}
