package integ

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"lab2/internal/domain"
)

func TestHealth(t *testing.T) {
	send(t, http.MethodGet, "/health", nil).expectStatus(t, http.StatusOK)
}

func TestCreateAndGet(t *testing.T) {
	reset(t)
	created := createNotification(t, validNotification())
	if created.ID == 0 || created.Status != domain.StatusScheduled || created.Priority != domain.PriorityLow {
		t.Fatalf("created = %+v", created)
	}

	got := getNotification(t, created.ID)
	if got.Content != "Hello" || got.Recipient != "ann@example.com" || got.Channel != domain.ChannelEmail {
		t.Fatalf("got = %+v", got)
	}

	var list []domain.Notification
	send(t, http.MethodGet, "/api/v1/notifications", nil).expectStatus(t, http.StatusOK).decode(t, &list)
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestCreateReturnsLocation(t *testing.T) {
	reset(t)
	res, err := http.Post(baseURL+"/api/v1/notifications", "application/json",
		strings.NewReader(`{"channel":"push","content":"x","recipient":"d1","timezone":"UTC"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated || !strings.HasPrefix(res.Header.Get("Location"), "/api/v1/notifications/") {
		t.Fatalf("status %d, Location %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestValidationErrorsNameTheField(t *testing.T) {
	reset(t)
	tests := map[string]map[string]any{
		"channel":   notificationWith(map[string]any{"channel": "sms"}),
		"content":   notificationWith(map[string]any{"content": ""}),
		"recipient": notificationWith(map[string]any{"recipient": "not-an-email"}),
		"timezone":  notificationWith(map[string]any{"timezone": "Mars/Base"}),
		"priority":  notificationWith(map[string]any{"priority": "urgent"}),
	}
	for field, payload := range tests {
		t.Run(field, func(t *testing.T) {
			send(t, http.MethodPost, "/api/v1/notifications", payload).
				expectErrorField(t, http.StatusUnprocessableEntity, field)
		})
	}
}

func TestMalformedRequests(t *testing.T) {
	reset(t)
	res, err := http.Post(baseURL+"/api/v1/notifications", "application/json", strings.NewReader("{oops"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed JSON: status %d, want 400", res.StatusCode)
	}

	send(t, http.MethodPost, "/api/v1/notifications", notificationWith(map[string]any{"send_at": "yesterday"})).
		expectStatus(t, http.StatusBadRequest)
	send(t, http.MethodGet, "/api/v1/notifications/abc", nil).expectErrorField(t, http.StatusBadRequest, "id")
}

func TestUnknownNotificationIs404(t *testing.T) {
	reset(t)
	send(t, http.MethodGet, "/api/v1/notifications/999", nil).expectStatus(t, http.StatusNotFound)
	send(t, http.MethodPost, "/api/v1/notifications/999/cancel", nil).expectStatus(t, http.StatusNotFound)
	send(t, http.MethodPost, "/api/v1/notifications/999/send-now", nil).expectStatus(t, http.StatusNotFound)
}

func TestNotificationIsDeliveredByItsChannelWorker(t *testing.T) {
	reset(t)
	emailSender, pushSender := &testSender{}, &testSender{}
	startWorkers(t, "email", 1, emailSender)
	startWorkers(t, "push", 1, pushSender)

	email := createNotification(t, validNotification())
	push := createNotification(t, notificationWith(map[string]any{"channel": "push", "recipient": "device-1"}))

	sent := waitStatus(t, email.ID, domain.StatusSent, 5*time.Second)
	if sent.Attempts != 1 || sent.SentAt == nil {
		t.Fatalf("sent = %+v, want 1 attempt and sent_at", sent)
	}
	waitStatus(t, push.ID, domain.StatusSent, 5*time.Second)

	if got := emailSender.ids(); len(got) != 1 || got[0] != email.ID {
		t.Fatalf("email worker delivered %v, want only [%d]", got, email.ID)
	}
	if got := pushSender.ids(); len(got) != 1 || got[0] != push.ID {
		t.Fatalf("push worker delivered %v, want only [%d]", got, push.ID)
	}
}

func TestNotificationStaysQueuedUntilAWorkerStarts(t *testing.T) {
	reset(t)
	n := createNotification(t, validNotification())
	waitStatus(t, n.ID, domain.StatusQueued, 5*time.Second)

	sender := &testSender{}
	startWorkers(t, "email", 1, sender)
	waitStatus(t, n.ID, domain.StatusSent, 5*time.Second)
}

func TestNotificationIsSentAtTheScheduledTime(t *testing.T) {
	reset(t)
	sender := &testSender{}
	startWorkers(t, "email", 1, sender)

	at := time.Now().Add(1500 * time.Millisecond).UTC()
	n := createNotification(t, notificationWith(map[string]any{"send_at": at}))
	if !n.ScheduledAt.Equal(at) {
		t.Fatalf("scheduled_at = %v, want %v", n.ScheduledAt, at)
	}

	time.Sleep(700 * time.Millisecond)
	if got := getNotification(t, n.ID); got.Status != domain.StatusScheduled {
		t.Fatalf("status = %s before the send time, want scheduled", got.Status)
	}

	sent := waitStatus(t, n.ID, domain.StatusSent, 5*time.Second)
	if sent.SentAt.Before(at) {
		t.Fatalf("sent_at %v is before the scheduled time %v", sent.SentAt, at)
	}
}

func TestQuietHoursPostponeDeliveryUntilMorning(t *testing.T) {
	reset(t)
	sender := &testSender{}
	startWorkers(t, "email", 1, sender)

	zone := nightZone()
	n := createNotification(t, notificationWith(map[string]any{"timezone": zone}))

	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	local := n.ScheduledAt.In(loc)
	if local.Hour() != 8 || local.Minute() != 0 || !n.ScheduledAt.After(time.Now()) {
		t.Fatalf("scheduled_at = %v (%v in %s), want the next 08:00 local time", n.ScheduledAt, local, zone)
	}
	settle()
	if got := getNotification(t, n.ID); got.Status != domain.StatusScheduled || len(sender.ids()) != 0 {
		t.Fatalf("status %s, deliveries %v: nothing may be sent at night", got.Status, sender.ids())
	}

	// send-now overrides the quiet hours.
	send(t, http.MethodPost, notificationPath(n.ID)+"/send-now", nil).expectStatus(t, http.StatusOK)
	waitStatus(t, n.ID, domain.StatusSent, 5*time.Second)
}

func TestSendNowDeliversImmediately(t *testing.T) {
	reset(t)
	sender := &testSender{}
	startWorkers(t, "email", 1, sender)

	n := createNotification(t, notificationWith(map[string]any{"send_at": time.Now().Add(time.Hour).UTC()}))
	settle()
	if got := getNotification(t, n.ID); got.Status != domain.StatusScheduled {
		t.Fatalf("status = %s, want scheduled", got.Status)
	}

	send(t, http.MethodPost, notificationPath(n.ID)+"/send-now", nil).expectStatus(t, http.StatusOK)
	waitStatus(t, n.ID, domain.StatusSent, 5*time.Second)

	// Already sent: nothing more to push.
	send(t, http.MethodPost, notificationPath(n.ID)+"/send-now", nil).expectErrorField(t, http.StatusConflict, "status")
}

func TestCancelledNotificationIsNeverSent(t *testing.T) {
	reset(t)
	sender := &testSender{}
	startWorkers(t, "email", 1, sender)

	n := createNotification(t, notificationWith(map[string]any{"send_at": time.Now().Add(2 * time.Second).UTC()}))
	send(t, http.MethodPost, notificationPath(n.ID)+"/cancel", nil).expectStatus(t, http.StatusOK)

	time.Sleep(3 * time.Second) // past the original send time
	if got := getNotification(t, n.ID); got.Status != domain.StatusCancelled || len(sender.ids()) != 0 {
		t.Fatalf("status %s, deliveries %v; want cancelled and nothing sent", got.Status, sender.ids())
	}
	send(t, http.MethodPost, notificationPath(n.ID)+"/cancel", nil).expectErrorField(t, http.StatusConflict, "status")
	send(t, http.MethodPost, notificationPath(n.ID)+"/send-now", nil).expectErrorField(t, http.StatusConflict, "status")
}

func TestCancelQueuedNotificationBeforeDelivery(t *testing.T) {
	reset(t)
	n := createNotification(t, validNotification()) // no worker yet: it waits in the queue
	waitStatus(t, n.ID, domain.StatusQueued, 5*time.Second)

	send(t, http.MethodPost, notificationPath(n.ID)+"/cancel", nil).expectStatus(t, http.StatusOK)

	sender := &testSender{}
	startWorkers(t, "email", 1, sender) // receives the stale message and drops it
	settle()
	if got := getNotification(t, n.ID); got.Status != domain.StatusCancelled || len(sender.ids()) != 0 {
		t.Fatalf("status %s, deliveries %v; want cancelled and nothing sent", got.Status, sender.ids())
	}
}

func TestFailedDeliveryIsRetriedAndSucceeds(t *testing.T) {
	reset(t)
	sender := &testSender{}
	sender.failTimes(2)
	startWorkers(t, "email", 1, sender)

	n := createNotification(t, validNotification())
	sent := waitStatus(t, n.ID, domain.StatusSent, 10*time.Second)
	if sent.Attempts != 3 || sent.LastError != "" {
		t.Fatalf("sent = %+v, want 3 attempts and no error", sent)
	}
	if got := sender.ids(); len(got) != 1 {
		t.Fatalf("delivered %d times, want once", len(got))
	}
}

func TestNotificationFailsAfterThreeAttempts(t *testing.T) {
	reset(t)
	sender := &testSender{}
	sender.failTimes(1000)
	startWorkers(t, "email", 1, sender)

	n := createNotification(t, validNotification())
	failed := waitStatus(t, n.ID, domain.StatusFailed, 10*time.Second)
	if failed.Attempts != domain.MaxAttempts || failed.LastError == "" {
		t.Fatalf("failed = %+v, want %d attempts and last_error", failed, domain.MaxAttempts)
	}

	settle() // no further attempts
	if got := getNotification(t, n.ID); got.Attempts != domain.MaxAttempts {
		t.Fatalf("attempts = %d after failing, want %d", got.Attempts, domain.MaxAttempts)
	}
}

func TestEachNotificationIsDeliveredExactlyOnceByCompetingWorkers(t *testing.T) {
	reset(t)
	sender := &testSender{}
	startWorkers(t, "email", 3, sender)

	const count = 30
	ids := make([]int64, count)
	for i := range ids {
		ids[i] = createNotification(t, validNotification()).ID
	}
	for _, id := range ids {
		if n := waitStatus(t, id, domain.StatusSent, 15*time.Second); n.Attempts != 1 {
			t.Fatalf("notification %d: %d attempts, want 1", id, n.Attempts)
		}
	}

	settle()
	seen := map[int64]int{}
	for _, id := range sender.ids() {
		seen[id]++
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Errorf("notification %d delivered %d times, want exactly once", id, seen[id])
		}
	}
}

func TestHighPriorityOvertakesLowPriority(t *testing.T) {
	reset(t)
	sender := &testSender{}
	release := sender.block()
	startWorkers(t, "email", 1, sender)
	t.Cleanup(release) // runs before the worker stops

	// The single worker blocks on the first notification, so the next ones pile up.
	first := createNotification(t, validNotification())
	waitStatus(t, first.ID, domain.StatusQueued, 5*time.Second)
	for sender.blocked.Load() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	var low []int64
	for range 3 {
		n := createNotification(t, validNotification())
		waitStatus(t, n.ID, domain.StatusQueued, 5*time.Second)
		low = append(low, n.ID)
	}
	time.Sleep(300 * time.Millisecond) // let the scheduler finish publishing
	high := createNotification(t, notificationWith(map[string]any{"priority": "high"}))
	waitStatus(t, high.ID, domain.StatusQueued, 5*time.Second)
	time.Sleep(300 * time.Millisecond)

	release()
	for _, id := range append(low, high.ID) {
		waitStatus(t, id, domain.StatusSent, 10*time.Second)
	}

	want := append([]int64{first.ID, high.ID}, low...)
	got := sender.ids()
	if len(got) != len(want) {
		t.Fatalf("delivery order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delivery order = %v, want %v (high priority right after the blocked one)", got, want)
		}
	}
}

func TestSwaggerDocs(t *testing.T) {
	for path, want := range map[string]string{
		"/openapi.yaml": "openapi: 3.0.3",
		"/docs":         "swagger-ui",
	} {
		res := send(t, http.MethodGet, path, nil).expectStatus(t, http.StatusOK)
		if !strings.Contains(string(res.body), want) {
			t.Errorf("%s does not contain %q", path, want)
		}
	}
}
