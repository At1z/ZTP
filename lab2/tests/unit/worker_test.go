package unit

import (
	"errors"
	"testing"
	"time"

	"lab2/internal/domain"
	"lab2/internal/worker"
)

const retryDelay = 5 * time.Second

func newWorkerEnv(failures int) (*worker.Processor, *fakeStore, *fakeSender, *fakeBroker) {
	store := newFakeStore(noonUTC)
	sender := &fakeSender{failures: failures}
	broker := &fakeBroker{}
	return worker.NewProcessor(store, sender, broker, retryDelay), store, sender, broker
}

func queuedNotification(store *fakeStore) domain.Notification {
	return store.add(domain.Notification{
		Channel: domain.ChannelEmail, Priority: domain.PriorityHigh, Status: domain.StatusQueued,
	})
}

func TestWorkerDeliversQueuedNotification(t *testing.T) {
	p, store, sender, broker := newWorkerEnv(0)
	n := queuedNotification(store)

	if err := p.Process(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	got := store.items[n.ID]
	if got.Status != domain.StatusSent || got.Attempts != 1 {
		t.Fatalf("notification = %+v, want sent after 1 attempt", got)
	}
	if len(sender.sent) != 1 || len(broker.retries) != 0 {
		t.Fatalf("sent %v, retries %v", sender.sent, broker.retries)
	}
}

func TestWorkerDropsDuplicateMessages(t *testing.T) {
	p, store, sender, _ := newWorkerEnv(0)
	n := queuedNotification(store)

	for range 3 { // the same message delivered three times
		if err := p.Process(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.sent) != 1 {
		t.Fatalf("delivered %d times, want exactly once", len(sender.sent))
	}
	if store.items[n.ID].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", store.items[n.ID].Attempts)
	}
}

func TestWorkerSkipsNotificationsThatAreNotQueued(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusScheduled, domain.StatusCancelled, domain.StatusSent, domain.StatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			p, store, sender, broker := newWorkerEnv(0)
			n := store.add(domain.Notification{Channel: domain.ChannelEmail, Status: status})

			if err := p.Process(ctx, n.ID); err != nil {
				t.Fatal(err)
			}
			if len(sender.sent) != 0 || len(broker.retries) != 0 || store.status(n.ID) != status {
				t.Fatalf("a %s notification was touched", status)
			}
		})
	}
}

func TestWorkerSkipsUnknownNotification(t *testing.T) {
	p, _, sender, _ := newWorkerEnv(0)
	if err := p.Process(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatal("something was sent")
	}
}

func TestWorkerSchedulesRetryAfterFailure(t *testing.T) {
	p, store, _, broker := newWorkerEnv(1)
	n := queuedNotification(store)

	if err := p.Process(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	got := store.items[n.ID]
	if got.Status != domain.StatusQueued || got.Attempts != 1 || got.LastError == "" {
		t.Fatalf("notification = %+v, want queued with 1 attempt and an error", got)
	}
	want := published{Channel: "email", ID: n.ID, Priority: domain.PriorityHigh, Delay: retryDelay}
	if len(broker.retries) != 1 || broker.retries[0] != want {
		t.Fatalf("retries = %+v, want %+v", broker.retries, want)
	}
}

func TestWorkerSucceedsOnThirdAttempt(t *testing.T) {
	p, store, sender, broker := newWorkerEnv(2)
	n := queuedNotification(store)

	for range domain.MaxAttempts { // the broker would redeliver after each retry
		if err := p.Process(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
	}
	got := store.items[n.ID]
	if got.Status != domain.StatusSent || got.Attempts != 3 || got.LastError != "" {
		t.Fatalf("notification = %+v, want sent after 3 attempts", got)
	}
	if len(broker.retries) != 2 || len(sender.sent) != 1 {
		t.Fatalf("retries %d, deliveries %d; want 2 and 1", len(broker.retries), len(sender.sent))
	}
}

func TestWorkerGivesUpAfterThreeAttempts(t *testing.T) {
	p, store, sender, broker := newWorkerEnv(100)
	n := queuedNotification(store)

	for range 5 { // more messages than attempts: the extra ones must be ignored
		if err := p.Process(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
	}
	got := store.items[n.ID]
	if got.Status != domain.StatusFailed || got.Attempts != domain.MaxAttempts || got.LastError == "" {
		t.Fatalf("notification = %+v, want failed after %d attempts", got, domain.MaxAttempts)
	}
	if len(broker.retries) != domain.MaxAttempts-1 || len(sender.sent) != 0 {
		t.Fatalf("retries %d, deliveries %d", len(broker.retries), len(sender.sent))
	}
}

func TestWorkerReturnsErrorWhenRetryCannotBePublished(t *testing.T) {
	p, store, _, broker := newWorkerEnv(1)
	broker.err = errors.New("broker down")
	n := queuedNotification(store)

	if err := p.Process(ctx, n.ID); err == nil {
		t.Fatal("expected an error so that the message is redelivered")
	}
}
