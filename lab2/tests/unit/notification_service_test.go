package unit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"lab2/internal/domain"
	"lab2/internal/service"
)

// 10:00 UTC is noon in Warsaw (CEST, +2) in July.
var noonUTC = time.Date(2030, 7, 1, 10, 0, 0, 0, time.UTC)

func newService(now time.Time) (*service.NotificationService, *fakeStore) {
	store := newFakeStore(now)
	return service.NewNotificationService(store, func() time.Time { return now }), store
}

func validInput() domain.NotificationInput {
	return domain.NotificationInput{
		Channel:   domain.ChannelEmail,
		Content:   "Hello",
		Recipient: "ann@example.com",
		Timezone:  "Europe/Warsaw",
	}
}

// expectInvalidFields checks that err is a validation error naming exactly the
// given fields.
func expectInvalidFields(t *testing.T, err error, fields ...string) {
	t.Helper()
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	got := map[string]bool{}
	for _, fe := range ve.Errors {
		if fe.Message == "" {
			t.Errorf("error for %q has no message", fe.Field)
		}
		got[fe.Field] = true
	}
	if len(got) != len(fields) {
		t.Fatalf("invalid fields = %v, want %v", ve.Errors, fields)
	}
	for _, f := range fields {
		if !got[f] {
			t.Fatalf("invalid fields = %v, want %v", ve.Errors, fields)
		}
	}
}

func TestCreateAppliesDefaults(t *testing.T) {
	svc, _ := newService(noonUTC)
	n, err := svc.Create(ctx, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if n.Priority != domain.PriorityLow || n.Status != domain.StatusScheduled || !n.ScheduledAt.Equal(noonUTC) {
		t.Fatalf("notification = %+v, want low priority, scheduled for now", n)
	}
}

func TestCreateKeepsGivenSendTime(t *testing.T) {
	svc, _ := newService(noonUTC)
	at := noonUTC.Add(2 * time.Hour)
	in := validInput()
	in.SendAt = &at
	in.Priority = domain.PriorityHigh

	n, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !n.ScheduledAt.Equal(at) || n.Priority != domain.PriorityHigh {
		t.Fatalf("notification = %+v", n)
	}
}

func TestCreateWithPastSendTimeIsDueNow(t *testing.T) {
	svc, _ := newService(noonUTC)
	past := noonUTC.Add(-time.Hour)
	in := validInput()
	in.SendAt = &past

	n, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !n.ScheduledAt.Equal(noonUTC) {
		t.Fatalf("scheduled_at = %v, want now (%v)", n.ScheduledAt, noonUTC)
	}
}

func TestInvalidInput(t *testing.T) {
	tests := map[string]struct {
		change func(*domain.NotificationInput)
		field  string
	}{
		"unknown channel":     {func(in *domain.NotificationInput) { in.Channel = "sms" }, "channel"},
		"missing channel":     {func(in *domain.NotificationInput) { in.Channel = "" }, "channel"},
		"empty content":       {func(in *domain.NotificationInput) { in.Content = "  " }, "content"},
		"content too long":    {func(in *domain.NotificationInput) { in.Content = strings.Repeat("a", 501) }, "content"},
		"empty recipient":     {func(in *domain.NotificationInput) { in.Recipient = "" }, "recipient"},
		"email without @":     {func(in *domain.NotificationInput) { in.Recipient = "ann" }, "recipient"},
		"missing timezone":    {func(in *domain.NotificationInput) { in.Timezone = "" }, "timezone"},
		"unknown timezone":    {func(in *domain.NotificationInput) { in.Timezone = "Mars/Base" }, "timezone"},
		"Local is not a zone": {func(in *domain.NotificationInput) { in.Timezone = "Local" }, "timezone"},
		"unknown priority":    {func(in *domain.NotificationInput) { in.Priority = "urgent" }, "priority"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc, store := newService(noonUTC)
			in := validInput()
			tc.change(&in)
			_, err := svc.Create(ctx, in)
			expectInvalidFields(t, err, tc.field)
			if len(store.items) != 0 {
				t.Fatal("an invalid notification was stored")
			}
		})
	}
}

func TestContentLengthCountsCharactersNotBytes(t *testing.T) {
	svc, _ := newService(noonUTC)
	in := validInput()
	in.Content = strings.Repeat("ż", 500)
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatalf("500 characters should be accepted: %v", err)
	}
}

func TestPushRecipientNeedsNoAtSign(t *testing.T) {
	svc, _ := newService(noonUTC)
	in := validInput()
	in.Channel = domain.ChannelPush
	in.Recipient = "device-42"
	if _, err := svc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
}

func TestAllInvalidFieldsAreReportedAtOnce(t *testing.T) {
	svc, _ := newService(noonUTC)
	_, err := svc.Create(ctx, domain.NotificationInput{Priority: "urgent"})
	expectInvalidFields(t, err, "channel", "content", "recipient", "timezone", "priority")
}

func TestQuietHours(t *testing.T) {
	warsaw := func(h, m int) time.Time { // local Warsaw time on 1 July 2030 (UTC+2)
		return time.Date(2030, 7, 1, h-2, m, 0, 0, time.UTC)
	}
	tests := map[string]struct {
		sendAt time.Time
		tz     string
		want   time.Time
	}{
		"daytime is kept":          {warsaw(12, 0), "Europe/Warsaw", warsaw(12, 0)},
		"08:00 is allowed":         {warsaw(8, 0), "Europe/Warsaw", warsaw(8, 0)},
		"21:59 is allowed":         {warsaw(21, 59), "Europe/Warsaw", warsaw(21, 59)},
		"22:00 moves to next 8:00": {warsaw(22, 0), "Europe/Warsaw", warsaw(8, 0).AddDate(0, 0, 1)},
		"23:30 moves to next 8:00": {warsaw(23, 30), "Europe/Warsaw", warsaw(8, 0).AddDate(0, 0, 1)},
		"03:00 moves to same 8:00": {warsaw(3, 0), "Europe/Warsaw", warsaw(8, 0)},
		// 10:00 UTC is 03:00 in New York (UTC-4): night there, day in Warsaw.
		"zone of the recipient counts": {noonUTC, "America/New_York", time.Date(2030, 7, 1, 12, 0, 0, 0, time.UTC)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc, _ := newService(tc.sendAt.Add(-time.Hour))
			in := validInput()
			in.Timezone = tc.tz
			in.SendAt = &tc.sendAt
			n, err := svc.Create(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			if !n.ScheduledAt.Equal(tc.want) {
				t.Fatalf("scheduled_at = %v, want %v", n.ScheduledAt, tc.want)
			}
		})
	}
}

func TestQuietHoursApplyToNotificationsCreatedWithoutSendTime(t *testing.T) {
	night := time.Date(2030, 7, 1, 23, 0, 0, 0, time.UTC) // 01:00 in Warsaw
	svc, _ := newService(night)
	n, err := svc.Create(ctx, validInput())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2030, 7, 2, 6, 0, 0, 0, time.UTC) // 08:00 in Warsaw
	if !n.ScheduledAt.Equal(want) {
		t.Fatalf("scheduled_at = %v, want %v", n.ScheduledAt, want)
	}
}

func TestCancel(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusScheduled, domain.StatusQueued} {
		t.Run(string(status), func(t *testing.T) {
			svc, store := newService(noonUTC)
			n := store.add(domain.Notification{Status: status})
			got, err := svc.Cancel(ctx, n.ID)
			if err != nil || got.Status != domain.StatusCancelled {
				t.Fatalf("got %+v, err %v; want cancelled", got, err)
			}
		})
	}
}

func TestCancelFinalNotificationIsConflict(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusSent, domain.StatusFailed, domain.StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			svc, store := newService(noonUTC)
			n := store.add(domain.Notification{Status: status})
			_, err := svc.Cancel(ctx, n.ID)
			var ce *domain.ConflictError
			if !errors.As(err, &ce) || ce.Field != "status" {
				t.Fatalf("err = %v, want a conflict on status", err)
			}
			if store.status(n.ID) != status {
				t.Fatalf("status changed to %s", store.status(n.ID))
			}
		})
	}
}

func TestSendNowMakesNotificationDue(t *testing.T) {
	svc, store := newService(noonUTC)
	n := store.add(domain.Notification{Status: domain.StatusScheduled, ScheduledAt: noonUTC.Add(48 * time.Hour)})

	got, err := svc.SendNow(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ScheduledAt.Equal(noonUTC) {
		t.Fatalf("scheduled_at = %v, want now", got.ScheduledAt)
	}
}

func TestSendNowOnlyForScheduled(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusQueued, domain.StatusSent, domain.StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			svc, store := newService(noonUTC)
			n := store.add(domain.Notification{Status: status})
			_, err := svc.SendNow(ctx, n.ID)
			var ce *domain.ConflictError
			if !errors.As(err, &ce) {
				t.Fatalf("err = %v, want a conflict", err)
			}
		})
	}
}

func TestUnknownNotificationIsNotFound(t *testing.T) {
	svc, _ := newService(noonUTC)
	if _, err := svc.Get(ctx, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get: %v", err)
	}
	if _, err := svc.Cancel(ctx, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Cancel: %v", err)
	}
	if _, err := svc.SendNow(ctx, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SendNow: %v", err)
	}
}
