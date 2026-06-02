package webhooks

import "time"

const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"

	MaxAttempts = 5
	BaseBackoff = time.Second
	MaxBackoff  = time.Hour
)

type Subscription struct {
	ID          string
	ProjectID   string
	URL         string
	Secret      string
	FilterEvent *string
	Active      bool
	CreatedAt   time.Time
}

type Delivery struct {
	ID             string
	SubscriptionID string
	EventID        string
	Status         string
	Attempts       int
	NextRetryAt    time.Time
	LastError      *string
	Payload        []byte
	CreatedAt      time.Time
}

type DeliveryWithSub struct {
	Delivery
	URL    string
	Secret string
}

// nextBackoff returns the delay before the next attempt using capped exponential backoff.
// attempts=1 → 1s, attempts=2 → 2s, attempts=3 → 4s, …, capped at MaxBackoff.
func nextBackoff(attempts int) time.Duration {
	d := BaseBackoff
	for i := 1; i < attempts && d < MaxBackoff; i++ {
		d *= 2
		if d > MaxBackoff {
			d = MaxBackoff
		}
	}
	return d
}
