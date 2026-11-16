package integ

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"lab3/internal/platform/config"
	"lab3/internal/platform/db"
	"lab3/internal/product/handler"
	"lab3/internal/product/repository"
	"lab3/internal/product/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// env is a fresh products database with the real migrations applied, wired
// the same way as in cmd/product.
type env struct {
	pool         *pgxpool.Pool
	products     *service.ProductService
	reservations *service.ReservationService
	repo         *repository.ReservationRepository
	router       http.Handler
}

// newDatabase creates a temporary database with the migrations of one service
// applied, so tests never touch each other's data or the development database.
// It is dropped when the test ends.
func newDatabase(t *testing.T, service string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	admin, err := db.Connect(ctx, config.Load("postgres", "").DatabaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL (is `make up` running?): %v", err)
	}
	t.Cleanup(admin.Close)

	name := service + "_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}

	pool, err := db.Connect(ctx, config.Load(name, "").DatabaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop database: %v", err)
		}
	})

	applyMigrations(t, pool, service)
	return pool
}

// newEnv is the product service on a fresh database, wired like cmd/product.
func newEnv(t *testing.T, ttl time.Duration) *env {
	t.Helper()
	pool := newDatabase(t, "product")

	products := repository.NewProductRepository(pool)
	reservations := repository.NewReservationRepository(pool)
	e := &env{
		pool:         pool,
		products:     service.NewProductService(products),
		reservations: service.NewReservationService(reservations, ttl),
		repo:         reservations,
	}
	e.router = handler.NewRouter(e.products, e.reservations, pool.Ping)
	return e
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool, service string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", service, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
}

// addProduct inserts a product with the given stock and returns its id.
func (e *env) addProduct(t *testing.T, stock int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.pool.QueryRow(context.Background(),
		`INSERT INTO products (name, price, stock) VALUES ('test', 10.00, $1) RETURNING id`, stock).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) stock(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT stock FROM products WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) available(t *testing.T, id uuid.UUID) int {
	t.Helper()
	p, err := e.products.GetProduct(context.Background(), service.GetProductQuery{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return p.Available
}

func (e *env) reserve(productID, cartID uuid.UUID, qty int) error {
	_, err := e.reservations.Reserve(context.Background(), service.ReserveProductCommand{
		ProductID: productID,
		CartID:    cartID,
		UserID:    uuid.New(),
		Quantity:  qty,
	})
	return err
}
