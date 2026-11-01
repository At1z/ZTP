package unit

import (
	"context"
	"errors"
	"sort"
	"time"

	"lab2/internal/domain"
)

var ctx = context.Background()

// fakeStore is an in-memory implementation of the repository interfaces used
// by the service, the scheduler and the worker. It follows the same status
// rules as the SQL queries.
type fakeStore struct {
	items  map[int64]domain.Notification
	nextID int64
	now    time.Time
}

func newFakeStore(now time.Time) *fakeStore {
	return &fakeStore{items: map[int64]domain.Notification{}, nextID: 1, now: now}
}

// add stores a notification directly, bypassing the service.
func (f *fakeStore) add(n domain.Notification) domain.Notification {
	n.ID = f.nextID
	f.nextID++
	f.items[n.ID] = n
	return n
}

func (f *fakeStore) status(id int64) domain.Status { return f.items[id].Status }

func (f *fakeStore) Create(_ context.Context, n *domain.Notification) error {
	n.ID = f.nextID
	f.nextID++
	n.Status = domain.StatusScheduled
	n.CreatedAt = f.now
	f.items[n.ID] = *n
	return nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (domain.Notification, error) {
	n, ok := f.items[id]
	if !ok {
		return domain.Notification{}, domain.ErrNotFound
	}
	return n, nil
}

func (f *fakeStore) List(context.Context) ([]domain.Notification, error) {
	list := []domain.Notification{}
	for _, n := range f.items {
		list = append(list, n)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list, nil
}

// transition changes the status of a notification that is in one of the from
// states, with the same ErrNotFound / ErrInvalidState results as the SQL.
func (f *fakeStore) transition(id int64, to domain.Status, from ...domain.Status) (domain.Notification, error) {
	n, err := f.GetByID(ctx, id)
	if err != nil {
		return n, err
	}
	for _, s := range from {
		if n.Status == s {
			n.Status = to
			f.items[id] = n
			return n, nil
		}
	}
	return n, domain.ErrInvalidState
}

func (f *fakeStore) Cancel(_ context.Context, id int64) (domain.Notification, error) {
	return f.transition(id, domain.StatusCancelled, domain.StatusScheduled, domain.StatusQueued)
}

func (f *fakeStore) SendNow(_ context.Context, id int64) (domain.Notification, error) {
	n, err := f.transition(id, domain.StatusScheduled, domain.StatusScheduled)
	if err == nil {
		n.ScheduledAt = f.now
		f.items[id] = n
	}
	return n, err
}

func (f *fakeStore) ClaimDue(_ context.Context, limit int) ([]domain.Notification, error) {
	var due []domain.Notification
	for _, n := range f.items {
		if n.Status == domain.StatusScheduled && !n.ScheduledAt.After(f.now) {
			due = append(due, n)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].ID < due[j].ID })
	if len(due) > limit {
		due = due[:limit]
	}
	for i := range due {
		due[i].Status = domain.StatusQueued
		f.items[due[i].ID] = due[i]
	}
	return due, nil
}

func (f *fakeStore) Release(_ context.Context, id int64) error {
	_, err := f.transition(id, domain.StatusScheduled, domain.StatusQueued)
	if errors.Is(err, domain.ErrInvalidState) {
		return nil
	}
	return err
}

func (f *fakeStore) StartAttempt(_ context.Context, id int64) (domain.Notification, bool, error) {
	n, ok := f.items[id]
	if !ok || n.Status != domain.StatusQueued {
		return domain.Notification{}, false, nil
	}
	n.Attempts++
	f.items[id] = n
	return n, true, nil
}

func (f *fakeStore) finish(id int64, to domain.Status, msg string) error {
	n := f.items[id]
	if n.Status == domain.StatusQueued {
		n.Status = to
		n.LastError = msg
		f.items[id] = n
	}
	return nil
}

func (f *fakeStore) MarkSent(_ context.Context, id int64) error {
	return f.finish(id, domain.StatusSent, "")
}

func (f *fakeStore) MarkFailed(_ context.Context, id int64, msg string) error {
	return f.finish(id, domain.StatusFailed, msg)
}

func (f *fakeStore) RecordFailure(_ context.Context, id int64, msg string) error {
	n := f.items[id]
	n.LastError = msg
	f.items[id] = n
	return nil
}

// published records one call to the broker.
type published struct {
	Channel  string
	ID       int64
	Priority domain.Priority
	Delay    time.Duration // set for retries
}

// fakeBroker implements scheduler.Publisher and worker.Retrier.
type fakeBroker struct {
	published []published
	retries   []published
	err       error
}

func (b *fakeBroker) Publish(_ context.Context, channel string, id int64, p domain.Priority) error {
	if b.err != nil {
		return b.err
	}
	b.published = append(b.published, published{Channel: channel, ID: id, Priority: p})
	return nil
}

func (b *fakeBroker) Retry(_ context.Context, channel string, id int64, p domain.Priority, d time.Duration) error {
	if b.err != nil {
		return b.err
	}
	b.retries = append(b.retries, published{Channel: channel, ID: id, Priority: p, Delay: d})
	return nil
}

// fakeSender implements worker.Sender; it fails for the first failures calls.
type fakeSender struct {
	failures int
	sent     []int64
}

func (s *fakeSender) Send(_ context.Context, n domain.Notification) error {
	if s.failures > 0 {
		s.failures--
		return errors.New("delivery failed")
	}
	s.sent = append(s.sent, n.ID)
	return nil
}
