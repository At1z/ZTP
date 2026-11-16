package integ

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"lab3/internal/product/domain"
	"lab3/internal/product/service"

	"github.com/google/uuid"
)

// runConcurrently starts n goroutines at once and counts how many returned nil.
func runConcurrently(n int, fn func(i int) error) (ok int, errs []error) {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		start   = make(chan struct{})
		success atomic.Int32
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			err := fn(i)
			if err == nil {
				success.Add(1)
				return
			}
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()
	return int(success.Load()), errs
}

// The key test: many carts fight for the last piece, exactly one wins.
func TestOnlyOneCartReservesTheLastItem(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 1)

	ok, errs := runConcurrently(50, func(int) error {
		return e.reserve(product, uuid.New(), 1)
	})

	if ok != 1 {
		t.Fatalf("%d reservations succeeded, want exactly 1", ok)
	}
	for _, err := range errs {
		if !errors.Is(err, domain.ErrInsufficientStock) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if got := e.available(t, product); got != 0 {
		t.Fatalf("available = %d, want 0", got)
	}
}

func TestReservationsNeverExceedStock(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 10)

	ok, _ := runConcurrently(40, func(int) error {
		return e.reserve(product, uuid.New(), 1)
	})

	if ok != 10 {
		t.Fatalf("%d reservations succeeded, want 10", ok)
	}
	if got := e.available(t, product); got != 0 {
		t.Fatalf("available = %d, want 0", got)
	}
}

// Parallel additions to the same cart must all be counted (no lost update).
func TestConcurrentReservationsInOneCartAreAllCounted(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 100)
	cart := uuid.New()

	ok, errs := runConcurrently(20, func(int) error {
		return e.reserve(product, cart, 1)
	})

	if ok != 20 {
		t.Fatalf("%d succeeded, errors: %v", ok, errs)
	}
	if got := e.available(t, product); got != 80 {
		t.Fatalf("available = %d, want 80", got)
	}
}

func TestReservationExpiresAndFreesStock(t *testing.T) {
	e := newEnv(t, time.Second)
	product := e.addProduct(t, 1)

	if err := e.reserve(product, uuid.New(), 1); err != nil {
		t.Fatal(err)
	}
	if err := e.reserve(product, uuid.New(), 1); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock while locked", err)
	}

	time.Sleep(1200 * time.Millisecond)

	if got := e.available(t, product); got != 1 {
		t.Fatalf("available after expiry = %d, want 1", got)
	}
	if err := e.reserve(product, uuid.New(), 1); err != nil {
		t.Fatalf("reserve after expiry: %v", err)
	}
}

func TestReleaseFreesStockImmediately(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 1)
	cart := uuid.New()

	if err := e.reserve(product, cart, 1); err != nil {
		t.Fatal(err)
	}
	err := e.reservations.Release(context.Background(), service.ReleaseReservationCommand{CartID: cart, ProductID: product})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.reserve(product, uuid.New(), 1); err != nil {
		t.Fatalf("other cart after release: %v", err)
	}
}

func TestReleaseCartFreesAllItems(t *testing.T) {
	e := newEnv(t, time.Minute)
	a, b := e.addProduct(t, 1), e.addProduct(t, 1)
	cart := uuid.New()
	if err := e.reserve(a, cart, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.reserve(b, cart, 1); err != nil {
		t.Fatal(err)
	}

	if err := e.reservations.ReleaseCart(context.Background(), service.ReleaseCartCommand{CartID: cart}); err != nil {
		t.Fatal(err)
	}

	if e.available(t, a) != 1 || e.available(t, b) != 1 {
		t.Fatal("stock was not released")
	}
}

func TestReservingAgainDoesNotExtendExpiry(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 5)
	cart := uuid.New()
	cmd := service.ReserveProductCommand{ProductID: product, CartID: cart, UserID: uuid.New(), Quantity: 1}

	first, err := e.reservations.Reserve(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	second, err := e.reservations.Reserve(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}

	if second.Quantity != 2 {
		t.Fatalf("quantity = %d, want 2", second.Quantity)
	}
	if !second.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("expiry moved from %v to %v", first.ExpiresAt, second.ExpiresAt)
	}
}

func (e *env) confirm(cart uuid.UUID, items ...domain.ConfirmItem) error {
	return e.reservations.Confirm(context.Background(), service.ConfirmReservationsCommand{CartID: cart, Items: items})
}

func TestConfirmDecrementsStockAndKeepsItFinal(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 3)
	cart := uuid.New()
	if err := e.reserve(product, cart, 2); err != nil {
		t.Fatal(err)
	}

	if err := e.confirm(cart, domain.ConfirmItem{ProductID: product, Quantity: 2}); err != nil {
		t.Fatal(err)
	}

	if got := e.stock(t, product); got != 1 {
		t.Fatalf("stock = %d, want 1", got)
	}
	if got := e.available(t, product); got != 1 {
		t.Fatalf("available = %d, want 1", got)
	}
}

// A retried checkout confirms the same cart again: it must succeed without
// taking the stock a second time.
func TestConfirmIsIdempotentPerCart(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 3)
	cart := uuid.New()
	if err := e.reserve(product, cart, 2); err != nil {
		t.Fatal(err)
	}
	item := domain.ConfirmItem{ProductID: product, Quantity: 2}

	for i := 0; i < 3; i++ {
		if err := e.confirm(cart, item); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
	}

	if got := e.stock(t, product); got != 1 {
		t.Fatalf("stock = %d, want 1 (decremented once)", got)
	}
}

func TestConcurrentConfirmsOfOneCartDecrementOnce(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 5)
	cart := uuid.New()
	if err := e.reserve(product, cart, 2); err != nil {
		t.Fatal(err)
	}

	ok, errs := runConcurrently(10, func(int) error {
		return e.confirm(cart, domain.ConfirmItem{ProductID: product, Quantity: 2})
	})

	if ok != 10 {
		t.Fatalf("%d confirms succeeded, errors: %v", ok, errs)
	}
	if got := e.stock(t, product); got != 3 {
		t.Fatalf("stock = %d, want 3", got)
	}
}

func TestConfirmFailsWithoutReservationOrWithWrongQuantity(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 5)
	cart := uuid.New()

	err := e.confirm(cart, domain.ConfirmItem{ProductID: product, Quantity: 1})
	if !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("no reservation: err = %v, want ErrReservationNotFound", err)
	}

	if err := e.reserve(product, cart, 2); err != nil {
		t.Fatal(err)
	}
	err = e.confirm(cart, domain.ConfirmItem{ProductID: product, Quantity: 1})
	if !errors.Is(err, domain.ErrReservationMismatch) {
		t.Fatalf("wrong quantity: err = %v, want ErrReservationMismatch", err)
	}
	if got := e.stock(t, product); got != 5 {
		t.Fatalf("stock = %d, want 5 (nothing confirmed)", got)
	}
}

func TestConfirmFailsAfterExpiryWithoutChangingStock(t *testing.T) {
	e := newEnv(t, time.Second)
	product := e.addProduct(t, 1)
	cart := uuid.New()
	if err := e.reserve(product, cart, 1); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1200 * time.Millisecond)

	err := e.confirm(cart, domain.ConfirmItem{ProductID: product, Quantity: 1})
	if !errors.Is(err, domain.ErrReservationExpired) {
		t.Fatalf("err = %v, want ErrReservationExpired", err)
	}
	if got := e.stock(t, product); got != 1 {
		t.Fatalf("stock = %d, want 1", got)
	}
	// The failed confirm must not mark the cart as confirmed.
	if err := e.reserve(product, cart, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.confirm(cart, domain.ConfirmItem{ProductID: product, Quantity: 1}); err != nil {
		t.Fatalf("confirm of a fresh reservation: %v", err)
	}
}

// Two carts holding the same products, confirmed at once, must not deadlock
// (products are always locked in id order).
func TestConcurrentConfirmsDoNotDeadlock(t *testing.T) {
	e := newEnv(t, time.Minute)
	a, b := e.addProduct(t, 10), e.addProduct(t, 10)
	cartX, cartY := uuid.New(), uuid.New()
	for _, step := range []struct{ product, cart uuid.UUID }{{a, cartX}, {b, cartX}, {b, cartY}, {a, cartY}} {
		if err := e.reserve(step.product, step.cart, 1); err != nil {
			t.Fatal(err)
		}
	}
	carts := []uuid.UUID{cartX, cartY}
	// Each cart lists the products in a different order.
	lists := [][]domain.ConfirmItem{
		{{ProductID: a, Quantity: 1}, {ProductID: b, Quantity: 1}},
		{{ProductID: b, Quantity: 1}, {ProductID: a, Quantity: 1}},
	}

	ok, errs := runConcurrently(2, func(i int) error {
		return e.confirm(carts[i], lists[i]...)
	})

	if ok != 2 {
		t.Fatalf("%d confirms succeeded, errors: %v", ok, errs)
	}
	if e.stock(t, a) != 8 || e.stock(t, b) != 8 {
		t.Fatal("stock was not decremented for both carts")
	}
}
