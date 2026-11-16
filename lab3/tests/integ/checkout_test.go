package integ

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type checkoutJSON struct {
	OrderID    uuid.UUID `json:"order_id"`
	Status     string    `json:"status"`
	TotalCents int64     `json:"total_cents"`
}

type orderJSON struct {
	ID         uuid.UUID `json:"id"`
	Status     string    `json:"status"`
	TotalCents int64     `json:"total_cents"`
	Items      []struct {
		ProductName string `json:"product_name"`
		Quantity    int    `json:"quantity"`
	} `json:"items"`
}

type notificationJSON struct {
	OrderID uuid.UUID `json:"order_id"`
	Message string    `json:"message"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body)
	}
	return v
}

func (o *orderEnv) ordersCount(t *testing.T, user uuid.UUID) int {
	t.Helper()
	return len(decode[[]notificationJSON](t, o.do(user, "GET", "/notifications", "")))
}

func TestCheckoutBuysTheCartAndStartsAnOrder(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	user, stranger := uuid.New(), uuid.New()
	mug := c.product.addProduct(t, 10) // costs 10.00
	c.openCart(t, user)
	if rec := c.add(user, mug, 3); rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d", rec.Code)
	}

	rec := c.checkout(user)

	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout: status = %d, body = %s", rec.Code, rec.Body)
	}
	done := decode[checkoutJSON](t, rec)
	if done.Status != "processing" || done.TotalCents != 3000 {
		t.Fatalf("checkout = %+v, want a processing order worth 3000", done)
	}

	// The stock was decremented for good and nothing stays locked.
	if got := c.product.stock(t, mug); got != 7 {
		t.Fatalf("stock = %d, want 7", got)
	}
	if got := c.product.available(t, mug); got != 7 {
		t.Fatalf("available = %d, want 7", got)
	}
	// The cart is closed.
	if rec := c.do(user, "GET", "/cart", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cart after checkout: status = %d, want 404", rec.Code)
	}

	// The order exists in the order service, for its owner only.
	order := decode[orderJSON](t, c.order.do(user, "GET", "/orders/"+done.OrderID.String(), ""))
	if order.TotalCents != 3000 || len(order.Items) != 1 || order.Items[0].Quantity != 3 {
		t.Fatalf("order = %+v", order)
	}
	if rec := c.order.do(stranger, "GET", "/orders/"+done.OrderID.String(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("stranger reading the order: status = %d, want 404", rec.Code)
	}

	// The user was notified, the stranger was not.
	notes := decode[[]notificationJSON](t, c.order.do(user, "GET", "/notifications", ""))
	if len(notes) != 1 || notes[0].OrderID != done.OrderID || notes[0].Message == "" {
		t.Fatalf("notifications = %+v", notes)
	}
	if got := c.order.ordersCount(t, stranger); got != 0 {
		t.Fatalf("stranger has %d notifications", got)
	}

	// A new cart can be opened afterwards.
	if rec := c.do(user, "POST", "/cart", ""); rec.Code != http.StatusCreated {
		t.Fatalf("new cart: status = %d, want 201", rec.Code)
	}
}

func TestCheckoutOfAnEmptyCartIsRejected(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	user := uuid.New()
	c.openCart(t, user)

	if rec := c.checkout(user); rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if rec := c.checkout(uuid.New()); rec.Code != http.StatusNotFound {
		t.Fatalf("no cart: status = %d, want 404", rec.Code)
	}
	if rec := c.checkout(uuid.Nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no user: status = %d, want 401", rec.Code)
	}
}

// The cart outlives the reservation here (long cart ttl, short lock), so the
// checkout meets an expired reservation and must fail safely.
func TestCheckoutFailsSafelyAfterTheReservationExpired(t *testing.T) {
	c := newCartEnv(t, time.Second, time.Minute)
	user := uuid.New()
	ticket := c.product.addProduct(t, 1)
	c.openCart(t, user)
	if rec := c.add(user, ticket, 1); rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d", rec.Code)
	}

	time.Sleep(1300 * time.Millisecond)
	rec := c.checkout(user)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body)
	}
	if got := c.product.stock(t, ticket); got != 1 {
		t.Fatalf("stock = %d, want 1 (nothing bought)", got)
	}
	if got := c.order.ordersCount(t, user); got != 0 {
		t.Fatalf("%d notifications, want none (no order)", got)
	}
	if rec := c.do(user, "GET", "/cart", ""); rec.Code != http.StatusOK {
		t.Fatalf("cart after failed checkout: status = %d, want it to stay open", rec.Code)
	}
}

// The order service is down when the stock is bought. The cart stays open, the
// retry succeeds, and neither the stock nor the order is taken twice.
func TestCheckoutCanBeRetriedAfterTheOrderServiceFailed(t *testing.T) {
	product := newEnv(t, time.Minute)
	order := newOrderEnv(t)
	var failures atomic.Int32
	failures.Store(1)
	flaky := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && failures.Add(-1) >= 0 {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	c := newCartEnvFor(t, product, product.router, order, flaky, time.Minute)
	user := uuid.New()
	mug := product.addProduct(t, 5)
	c.openCart(t, user)
	if rec := c.add(user, mug, 2); rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d", rec.Code)
	}

	if rec := c.checkout(user); rec.Code != http.StatusBadGateway {
		t.Fatalf("first checkout: status = %d, want 502", rec.Code)
	}
	if rec := c.do(user, "GET", "/cart", ""); rec.Code != http.StatusOK {
		t.Fatalf("cart after the failure: status = %d, want it to stay open", rec.Code)
	}

	rec := c.checkout(user)

	if rec.Code != http.StatusCreated {
		t.Fatalf("retry: status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := product.stock(t, mug); got != 3 {
		t.Fatalf("stock = %d, want 3 (bought once)", got)
	}
	if got := order.ordersCount(t, user); got != 1 {
		t.Fatalf("%d notifications, want exactly 1", got)
	}
}

// The same cart ordered twice (two requests at once) still yields one order.
func TestOrderServiceCreatesOneOrderPerCart(t *testing.T) {
	o := newOrderEnv(t)
	user, cart := uuid.New(), uuid.New()
	body := `{"cart_id":"` + cart.String() + `","items":[{"product_id":"` + uuid.NewString() + `","product_name":"x","unit_price_cents":100,"quantity":2}]}`

	ok, errs := runConcurrently(10, func(int) error {
		if rec := o.do(user, "POST", "/orders", body); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			return &statusError{rec.Code}
		}
		return nil
	})

	if ok != 10 {
		t.Fatalf("%d requests succeeded, errors: %v", ok, errs)
	}
	if got := o.ordersCount(t, user); got != 1 {
		t.Fatalf("%d notifications, want 1", got)
	}
}

type statusError struct{ code int }

func (e *statusError) Error() string { return http.StatusText(e.code) }

func TestOrderServiceValidatesInput(t *testing.T) {
	o := newOrderEnv(t)
	user := uuid.New()

	if rec := o.do(user, "POST", "/orders", `{"cart_id":"`+uuid.NewString()+`","items":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no items: status = %d, want 400", rec.Code)
	}
	if rec := o.do(uuid.Nil, "POST", "/orders", `{}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no user: status = %d, want 401", rec.Code)
	}
	if rec := o.do(user, "GET", "/orders/"+uuid.NewString(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown order: status = %d, want 404", rec.Code)
	}
}
