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
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lab1/internal/config"
	"lab1/internal/handler"
	"lab1/internal/repository"
	"lab1/internal/service"
)

const migrationFile = "../../migrations/001_init.sql"

var (
	baseURL string
	pool    *pgxpool.Pool
)

// TestMain starts the real HTTP API on top of a dedicated, freshly migrated
// PostgreSQL database, so tests never touch development data.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	var err error
	pool, err = setupTestDatabase(ctx)
	if err != nil {
		log.Printf("cannot prepare test database (is `docker compose up -d` running?): %v", err)
		return 1
	}
	defer pool.Close()

	productRepo := repository.NewProductRepository(pool)
	phraseRepo := repository.NewBannedPhraseRepository(pool)
	srv := httptest.NewServer(handler.NewRouter(
		service.NewProductService(productRepo, phraseRepo),
		service.NewBannedPhraseService(phraseRepo),
		pool,
	))
	defer srv.Close()
	baseURL = srv.URL

	return m.Run()
}

func setupTestDatabase(ctx context.Context) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(config.Load().DatabaseURL)
	if err != nil {
		return nil, err
	}
	testDB := os.Getenv("TEST_DB_NAME")
	if testDB == "" {
		testDB = "lab1_test"
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

// --- helpers ---

type response struct {
	status int
	body   []byte
}

// reset empties all tables so every test starts from a clean state.
func reset(t *testing.T) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"TRUNCATE products, product_history, banned_phrases RESTART IDENTITY")
	if err != nil {
		t.Fatalf("reset database: %v", err)
	}
	t.Log("database reset: products, product_history, banned_phrases truncated")
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

// errorFields returns the field names reported in an error response.
func (r response) errorFields(t *testing.T) []string {
	t.Helper()
	var body struct {
		Errors []struct {
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	r.decode(t, &body)
	fields := make([]string, len(body.Errors))
	for i, e := range body.Errors {
		if e.Message == "" {
			t.Errorf("error for field %q has no message", e.Field)
		}
		fields[i] = e.Field
	}
	return fields
}

func (r response) expectErrorField(t *testing.T, status int, field string) {
	t.Helper()
	r.expectStatus(t, status)
	for _, f := range r.errorFields(t) {
		if f == field {
			return
		}
	}
	t.Fatalf("expected an error for field %q, got body: %s", field, r.body)
}

type product struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

type historyEntry struct {
	Field    string `json:"field"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

type bannedPhrase struct {
	ID     int64  `json:"id"`
	Phrase string `json:"phrase"`
}

func validProduct() map[string]any {
	return map[string]any{"name": "Laptop1", "category": "electronics", "price": 3000, "quantity": 5}
}

// productWith returns a valid product payload with the given fields overridden.
func productWith(overrides map[string]any) map[string]any {
	p := validProduct()
	for k, v := range overrides {
		p[k] = v
	}
	return p
}

func createProduct(t *testing.T, payload map[string]any) product {
	t.Helper()
	var p product
	send(t, http.MethodPost, "/api/v1/products", payload).expectStatus(t, http.StatusCreated).decode(t, &p)
	return p
}

func productPath(id int64) string {
	return fmt.Sprintf("/api/v1/products/%d", id)
}

func addBannedPhrase(t *testing.T, phrase string) bannedPhrase {
	t.Helper()
	var p bannedPhrase
	send(t, http.MethodPost, "/api/v1/banned-phrases", map[string]any{"phrase": phrase}).
		expectStatus(t, http.StatusCreated).decode(t, &p)
	return p
}
