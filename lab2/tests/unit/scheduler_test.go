package unit

import (
	"errors"
	"testing"
	"time"

	"lab2/internal/domain"
	"lab2/internal/scheduler"
)

func TestSchedulerPublishesDueNotificationsToTheirChannel(t *testing.T) {
	store := newFakeStore(noonUTC)
	email := store.add(domain.Notification{Channel: domain.ChannelEmail, Priority: domain.PriorityLow, Status: domain.StatusScheduled, ScheduledAt: noonUTC})
	push := store.add(domain.Notification{Channel: domain.ChannelPush, Priority: domain.PriorityHigh, Status: domain.StatusScheduled, ScheduledAt: noonUTC.Add(-time.Minute)})
	broker := &fakeBroker{}

	if err := scheduler.New(store, broker, time.Second).Tick(ctx); err != nil {
		t.Fatal(err)
	}

	want := map[int64]published{
		email.ID: {Channel: "email", ID: email.ID, Priority: domain.PriorityLow},
		push.ID:  {Channel: "push", ID: push.ID, Priority: domain.PriorityHigh},
	}
	if len(broker.published) != 2 {
		t.Fatalf("published = %+v, want 2 messages", broker.published)
	}
	for _, p := range broker.published {
		if p != want[p.ID] {
			t.Errorf("published %+v, want %+v", p, want[p.ID])
		}
	}
	if store.status(email.ID) != domain.StatusQueued || store.status(push.ID) != domain.StatusQueued {
		t.Fatal("published notifications must be marked queued")
	}
}

func TestSchedulerLeavesFutureAndFinalNotifications(t *testing.T) {
	store := newFakeStore(noonUTC)
	store.add(domain.Notification{Channel: domain.ChannelEmail, Status: domain.StatusScheduled, ScheduledAt: noonUTC.Add(time.Second)})
	store.add(domain.Notification{Channel: domain.ChannelEmail, Status: domain.StatusCancelled, ScheduledAt: noonUTC})
	store.add(domain.Notification{Channel: domain.ChannelEmail, Status: domain.StatusSent, ScheduledAt: noonUTC})
	broker := &fakeBroker{}

	if err := scheduler.New(store, broker, time.Second).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(broker.published) != 0 {
		t.Fatalf("published = %+v, want nothing", broker.published)
	}
}

func TestSchedulerPublishesEachNotificationOnce(t *testing.T) {
	store := newFakeStore(noonUTC)
	store.add(domain.Notification{Channel: domain.ChannelEmail, Status: domain.StatusScheduled, ScheduledAt: noonUTC})
	broker := &fakeBroker{}
	s := scheduler.New(store, broker, time.Second)

	for range 3 {
		if err := s.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(broker.published) != 1 {
		t.Fatalf("published %d times, want once", len(broker.published))
	}
}

func TestSchedulerReleasesNotificationWhenPublishFails(t *testing.T) {
	store := newFakeStore(noonUTC)
	n := store.add(domain.Notification{Channel: domain.ChannelEmail, Status: domain.StatusScheduled, ScheduledAt: noonUTC})
	broker := &fakeBroker{err: errors.New("broker down")}
	s := scheduler.New(store, broker, time.Second)

	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if store.status(n.ID) != domain.StatusScheduled {
		t.Fatalf("status = %s, want scheduled so that it is retried", store.status(n.ID))
	}

	broker.err = nil
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(broker.published) != 1 || store.status(n.ID) != domain.StatusQueued {
		t.Fatalf("after recovery: published %+v, status %s", broker.published, store.status(n.ID))
	}
}
