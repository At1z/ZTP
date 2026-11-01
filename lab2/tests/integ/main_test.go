package integ

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"lab2/internal/config"
	"lab2/internal/domain"
	"lab2/internal/handler"
	"lab2/internal/queue"
	"lab2/internal/repository"
	"lab2/internal/scheduler"
	"lab2/internal/service"
	"lab2/internal/worker"
)

const (
	migrationFile = "../../migrations/001_init.sql"
	// Own queue names, so that the tests never compete with the running
	// workers of docker compose for messages.
	queuePrefix = "lab2test"
	retryDelay  = 200 * time.Millisecond
)

var (
	baseURL string
	pool    *pgxpool.Pool
	broker  *queue.Broker
	amqpURL string
)

// TestMain starts the real HTTP API and scheduler on top of a dedicated, freshly
// migrated PostgreSQL database and dedicated RabbitMQ queues. Workers are
// started by the tests that need them.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	cfg := config.Load()
	amqpURL = cfg.AMQPURL

	var err error
	pool, err = setupTestDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Printf("cannot prepare test database (is `make up` running?): %v", err)
		return 1
	}
	defer pool.Close()

	broker, err = queue.Connect(cfg.AMQPURL, queuePrefix)
	if err != nil {
		log.Printf("cannot connect to RabbitMQ (is `make up` running?): %v", err)
		return 1
	}
	defer broker.Close()

	repo := repository.NewNotificationRepository(pool)
	srv := httptest.NewServer(handler.NewRouter(service.NewNotificationService(repo, time.Now), pool, broker))
	defer srv.Close()
	baseURL = srv.URL

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.New(repo, broker, 100*time.Millisecond).Run(ctx)
	}()
	defer wg.Wait()
	defer stop()

	return m.Run()
}

func setupTestDatabase(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	testDB := os.Getenv("TEST_DB_NAME")
	if testDB == "" {
		testDB = "lab2_test"
	}

	admin, err := pgx.ConnectConfig(ctx, poolCfg.ConnConfig.Copy())
	if err != nil {
		return nil, err
	}
	defer admin.Close(ctx)

	var exists bool
	err = admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", testDB).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{testDB}.Sanitize()); err != nil {
			return nil, err
		}
	}

	poolCfg.ConnConfig.Database = testDB
	testPool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	schema, err := os.ReadFile(migrationFile)
	if err != nil {
		testPool.Close()
		return nil, err
	}
	if _, err := testPool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;\n"+string(schema)); err != nil {
		testPool.Close()
		return nil, err
	}
	return testPool, nil
}

// --- test sender and workers ---

type delivery struct {
	ID      int64
	Channel domain.Channel
}

// testSender is a worker.Sender that records deliveries and can be told to
// fail or to block, so that tests control what the workers do.
type testSender struct {
	mu        sync.Mutex
	delivered []delivery
	failNext  int
	gate      chan struct{}
	blocked   atomic.Int32
}

func (s *testSender) Send(ctx context.Context, n domain.Notification) error {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate != nil {
		s.blocked.Add(1)
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext > 0 {
		s.failNext--
		return fmt.Errorf("simulated failure")
	}
	s.delivered = append(s.delivered, delivery{ID: n.ID, Channel: n.Channel})
	return nil
}

func (s *testSender) failTimes(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = n
}

// block makes every Send wait until the returned function is called.
func (s *testSender) block() (release func()) {
	gate := make(chan struct{})
	s.mu.Lock()
	s.gate = gate
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

func (s *testSender) ids() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int64, len(s.delivered))
	for i, d := range s.delivered {
		ids[i] = d.ID
	}
	return ids
}

// startWorkers runs count worker instances of one channel until the test ends.
func startWorkers(t *testing.T, channel string, count int, sender *testSender) {
	t.Helper()
	processor := worker.NewProcessor(repository.NewNotificationRepository(pool), sender, broker, retryDelay)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := broker.Consume(ctx, channel, processor.Process); err != nil && ctx.Err() == nil {
				t.Errorf("worker %s: %v", channel, err)
			}
		}()
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
}

// --- helpers ---

type response struct {
	status int
	body   []byte
}

// reset empties the table and the queues so every test starts from a clean state.
func reset(t *testing.T) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE notifications RESTART IDENTITY"); err != nil {
		t.Fatalf("reset database: %v", err)
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("reset queues: %v", err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("reset queues: %v", err)
	}
	for _, c := range queue.Channels {
		for _, name := range []string{broker.QueueName(c), broker.RetryQueueName(c)} {
			if _, err := ch.QueuePurge(name, false); err != nil {
				t.Fatalf("purge %s: %v", name, err)
			}
		}
	}
	t.Log("reset: notifications truncated, queues purged")
}

func send(t *testing.T, method, path string, body any) response {
	t.Helper()
	var reader io.Reader
	reqBody := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
		reqBody = " " + string(raw)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	t.Logf("%s %s%s -> %d %s", method, path, reqBody, res.StatusCode, bytes.TrimSpace(raw))
	return response{status: res.StatusCode, body: raw}
}

func (r response) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
}

func (r response) expectStatus(t *testing.T, want int) response {
	t.Helper()
	if r.status != want {
		t.Fatalf("status = %d, want %d; body: %s", r.status, want, r.body)
	}
	return r
}

func (r response) expectErrorField(t *testing.T, status int, field string) {
	t.Helper()
	r.expectStatus(t, status)
	var body struct {
		Errors []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	r.decode(t, &body)
	for _, e := range body.Errors {
		if e.Field == field {
			if e.Message == "" {
				t.Errorf("error for field %q has no message", field)
			}
			return
		}
	}
	t.Fatalf("expected an error for field %q, got body: %s", field, r.body)
}

func notificationPath(id int64) string {
	return fmt.Sprintf("/api/v1/notifications/%d", id)
}

// dayZone returns a time zone in which it is 12:xx right now, so a notification
// created now is never held back by the quiet hours.
func dayZone() string { return zoneWithLocalHour(12) }

// nightZone returns a time zone in which it is 23:xx right now.
func nightZone() string { return zoneWithLocalHour(23) }

func zoneWithLocalHour(hour int) string {
	offset := hour - time.Now().UTC().Hour() // hours east of UTC
	if offset > 12 {
		offset -= 24
	} else if offset < -11 {
		offset += 24
	}
	switch {
	case offset == 0:
		return "UTC"
	case offset > 0:
		return fmt.Sprintf("Etc/GMT-%d", offset) // POSIX sign: GMT-5 is UTC+5
	default:
		return fmt.Sprintf("Etc/GMT+%d", -offset)
	}
}

func validNotification() map[string]any {
	return map[string]any{
		"channel": "email", "content": "Hello", "recipient": "ann@example.com", "timezone": dayZone(),
	}
}

// notificationWith returns a valid payload with the given fields overridden.
func notificationWith(overrides map[string]any) map[string]any {
	p := validNotification()
	for k, v := range overrides {
		p[k] = v
	}
	return p
}

func createNotification(t *testing.T, payload map[string]any) domain.Notification {
	t.Helper()
	var n domain.Notification
	send(t, http.MethodPost, "/api/v1/notifications", payload).
		expectStatus(t, http.StatusCreated).decode(t, &n)
	return n
}

func getNotification(t *testing.T, id int64) domain.Notification {
	t.Helper()
	var n domain.Notification
	send(t, http.MethodGet, notificationPath(id), nil).expectStatus(t, http.StatusOK).decode(t, &n)
	return n
}

// waitStatus polls until the notification has the given status.
func waitStatus(t *testing.T, id int64, want domain.Status, timeout time.Duration) domain.Notification {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var n domain.Notification
		res, err := http.Get(baseURL + notificationPath(id))
		if err == nil {
			_ = json.NewDecoder(res.Body).Decode(&n)
			res.Body.Close()
		}
		if n.Status == want {
			t.Logf("notification %d reached status %s (attempts %d)", id, want, n.Attempts)
			return n
		}
		if time.Now().After(deadline) {
			t.Fatalf("notification %d: status %q after %v, want %q", id, n.Status, timeout, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// settle gives the scheduler and the workers time to (wrongly) act.
func settle() { time.Sleep(700 * time.Millisecond) }
