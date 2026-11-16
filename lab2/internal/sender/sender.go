package sender

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"

	"lab2/internal/domain"
)

// Simulated pretends to deliver a notification by logging it. FailRate (0..1)
// makes a share of the deliveries fail so that retries can be observed.
type Simulated struct {
	FailRate float64
}

func (s Simulated) Send(_ context.Context, n domain.Notification) error {
	if rand.Float64() < s.FailRate {
		return fmt.Errorf("simulated %s delivery failure", n.Channel)
	}
	log.Printf("[%s] delivered notification %d to %s (priority %s, attempt %d): %q",
		n.Channel, n.ID, n.Recipient, n.Priority, n.Attempts, n.Content)
	return nil
}
