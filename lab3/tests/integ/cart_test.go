package integ

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

type cartJSON struct {
	ID         uuid.UUID `json:"id"`
	TotalCents int64     `json:"total_cents"`
	Items      []struct {
		ProductID uuid.UUID `json:"product_id"`
		Quantity  int       `json:"quantity"`
	} `json:"items"`
}

func addBody(product uuid.UUID, qty int) string {
	return fmt.Sprintf(`{"product_id":%q,"quantity":%d}`, product, qty)
}

func decodeCart(t *testing.T, rec *httptest.ResponseRecorder) cartJSON {
	t.Helper()
	var c cartJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatalf("decode cart: %v (body %s)", err, rec.Body)
	}
	return c
}

func (c cartJSON) quantity(product uuid.UUID) int {
	for _, it := range c.Items {
		if it.ProductID == product {
			return it.Quantity
		}
	}
	return 0
}

func TestRequestsWithoutUserAreRejected(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)

	if rec := c.do(uuid.Nil, "GET", "/cart", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /cart without user: status = %d, want 401", rec.Code)
	}
	if rec := c.do(uuid.Nil, "POST", "/cart", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /cart without user: status = %d, want 401", rec.Code)
	}
}

func TestCartLifecycle(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	user := uuid.New()
	mug := c.product.addProduct(t, 10)

	if rec := c.do(user, "GET", "/cart", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cart before creation: status = %d, want 404", rec.Code)
	}
	if rec := c.do(user, "POST", "/cart", ""); rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201", rec.Code)
	}
	if rec := c.do(user, "POST", "/cart", ""); rec.Code != http.StatusOK {
		t.Fatalf("create again: status = %d, want 200 with the same cart", rec.Code)
	}

	rec := c.add(user, mug, 3)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d, body = %s", rec.Code, rec.Body)
	}
	cart := decodeCart(t, rec)
	if cart.quantity(mug) != 3 || cart.TotalCents != 3*1000 {
		t.Fatalf("cart = %+v, want 3 pieces worth 3000", cart)
	}
	if got := c.product.available(t, mug); got != 7 {
		t.Fatalf("available = %d, want 7", got)
	}

	rec = c.remove(user, mug)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: status = %d", rec.Code)
	}
	if got := decodeCart(t, rec); len(got.Items) != 0 || got.TotalCents != 0 {
		t.Fatalf("cart after remove = %+v", got)
	}
	if got := c.product.available(t, mug); got != 10 {
		t.Fatalf("available after remove = %d, want 10", got)
	}
	if rec := c.remove(user, mug); rec.Code != http.StatusNotFound {
		t.Fatalf("remove again: status = %d, want 404", rec.Code)
	}
}

func TestAddItemValidationAndMissingCart(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	user := uuid.New()
	mug := c.product.addProduct(t, 10)

	if rec := c.add(user, mug, 1); rec.Code != http.StatusNotFound {
		t.Fatalf("add without cart: status = %d, want 404", rec.Code)
	}
	c.openCart(t, user)
	if rec := c.add(user, mug, 0); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero quantity: status = %d, want 400", rec.Code)
	}
	if rec := c.add(user, uuid.New(), 1); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown product: status = %d, want 404", rec.Code)
	}
	if rec := c.do(user, "POST", "/cart/items", `{"bad":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: status = %d, want 400", rec.Code)
	}
}

// Two users want the one ticket: the second is turned away until the first
// gives it back.
func TestSecondUserCannotTakeLockedProduct(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	alice, bob := uuid.New(), uuid.New()
	ticket := c.product.addProduct(t, 1)
	c.openCart(t, alice)
	c.openCart(t, bob)

	if rec := c.add(alice, ticket, 1); rec.Code != http.StatusCreated {
		t.Fatalf("alice: status = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := c.add(bob, ticket, 1); rec.Code != http.StatusConflict {
		t.Fatalf("bob: status = %d, want 409", rec.Code)
	}
	if got := decodeCart(t, c.do(bob, "GET", "/cart", "")); len(got.Items) != 0 {
		t.Fatalf("bob's cart = %+v, must stay empty", got)
	}

	c.remove(alice, ticket)

	if rec := c.add(bob, ticket, 1); rec.Code != http.StatusCreated {
		t.Fatalf("bob after alice removed it: status = %d", rec.Code)
	}
}

func TestLockExpiresWithTheCart(t *testing.T) {
	c := newCartEnv(t, time.Second, time.Second)
	alice, bob := uuid.New(), uuid.New()
	ticket := c.product.addProduct(t, 1)
	c.openCart(t, alice)
	c.openCart(t, bob)
	if rec := c.add(alice, ticket, 1); rec.Code != http.StatusCreated {
		t.Fatalf("alice: status = %d", rec.Code)
	}

	time.Sleep(2300 * time.Millisecond)

	if rec := c.do(alice, "GET", "/cart", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expired cart: status = %d, want 404", rec.Code)
	}
	// Bob's cart expired too, so he starts a new one and can take the ticket.
	c.openCart(t, bob)
	if rec := c.add(bob, ticket, 1); rec.Code != http.StatusCreated {
		t.Fatalf("bob after expiry: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestExpiredCartIsReplacedByANewOne(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Second)
	user := uuid.New()
	mug := c.product.addProduct(t, 1)
	c.openCart(t, user)
	oldID := decodeCart(t, c.do(user, "GET", "/cart", "")).ID
	if rec := c.add(user, mug, 1); rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d", rec.Code)
	}

	time.Sleep(1300 * time.Millisecond)

	rec := c.do(user, "POST", "/cart", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("new cart: status = %d, want 201", rec.Code)
	}
	if decodeCart(t, rec).ID == oldID {
		t.Fatal("expected a different cart")
	}
	// The old cart's reservation was released, so the same user can reserve
	// the only piece again in the new cart.
	if rec := c.add(user, mug, 1); rec.Code != http.StatusCreated {
		t.Fatalf("add in new cart: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestExpireCartsReleasesStock(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Second)
	user := uuid.New()
	mug := c.product.addProduct(t, 1)
	c.openCart(t, user)
	if rec := c.add(user, mug, 1); rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d", rec.Code)
	}

	time.Sleep(1300 * time.Millisecond)
	n, err := c.commands.ExpireCarts(t.Context())

	if err != nil || n != 1 {
		t.Fatalf("expired %d carts, err = %v", n, err)
	}
	if got := c.product.available(t, mug); got != 1 {
		t.Fatalf("available = %d, want 1 (reservation released by the sweep)", got)
	}
}

func TestUsersAreIsolated(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	alice, bob := uuid.New(), uuid.New()
	mug := c.product.addProduct(t, 10)
	c.openCart(t, alice)
	c.openCart(t, bob)
	c.add(alice, mug, 2)

	if got := decodeCart(t, c.do(bob, "GET", "/cart", "")); len(got.Items) != 0 {
		t.Fatalf("bob sees %+v", got)
	}
	if rec := c.remove(bob, mug); rec.Code != http.StatusNotFound {
		t.Fatalf("bob removing alice's item: status = %d, want 404", rec.Code)
	}
	if got := decodeCart(t, c.do(alice, "GET", "/cart", "")); got.quantity(mug) != 2 {
		t.Fatalf("alice's cart changed: %+v", got)
	}
}

func TestProductServiceDownGivesBadGateway(t *testing.T) {
	product := newEnv(t, time.Minute)
	down := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newCartEnvFor(t, product, down, time.Minute)
	user := uuid.New()
	c.openCart(t, user)

	rec := c.add(user, product.addProduct(t, 1), 1)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// The same user adds from many devices at once: every addition is counted and
// the reservation matches the cart line.
func TestConcurrentAddsFromManyDevicesAreAllCounted(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	user := uuid.New()
	mug := c.product.addProduct(t, 100)
	c.openCart(t, user)

	ok, errs := runConcurrently(20, func(int) error {
		if rec := c.add(user, mug, 1); rec.Code != http.StatusCreated {
			return fmt.Errorf("status %d: %s", rec.Code, rec.Body)
		}
		return nil
	})

	if ok != 20 {
		t.Fatalf("%d adds succeeded, errors: %v", ok, errs)
	}
	cart := decodeCart(t, c.do(user, "GET", "/cart", ""))
	if cart.quantity(mug) != 20 {
		t.Fatalf("cart quantity = %d, want 20", cart.quantity(mug))
	}
	if got := c.reservedQuantity(t, mug); got != 20 {
		t.Fatalf("reserved = %d, want 20", got)
	}
}

// Adds and removes race on one cart. Whatever order they run in, the cart line
// and the product reservation must end up identical (no lost update, no
// reservation left without a line or the other way round).
func TestConcurrentAddsAndRemovesStayConsistent(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	user := uuid.New()
	mug := c.product.addProduct(t, 1000)
	c.openCart(t, user)

	for round := 0; round < 5; round++ {
		runConcurrently(30, func(i int) error {
			if i%3 == 0 {
				c.remove(user, mug) // 404 when the line is missing is fine
				return nil
			}
			if rec := c.add(user, mug, 1); rec.Code != http.StatusCreated {
				return fmt.Errorf("status %d", rec.Code)
			}
			return nil
		})

		cart := decodeCart(t, c.do(user, "GET", "/cart", ""))
		if inCart, reserved := cart.quantity(mug), c.reservedQuantity(t, mug); inCart != reserved {
			t.Fatalf("round %d: cart holds %d, product reserved %d", round, inCart, reserved)
		}
	}
}

// Different users hammering the last pieces through the whole stack: never
// more reservations than stock.
func TestManyUsersNeverOversellThroughTheCart(t *testing.T) {
	c := newCartEnv(t, time.Minute, time.Minute)
	ticket := c.product.addProduct(t, 1)
	users := make([]uuid.UUID, 30)
	for i := range users {
		users[i] = uuid.New()
		c.openCart(t, users[i])
	}

	ok, _ := runConcurrently(len(users), func(i int) error {
		if rec := c.add(users[i], ticket, 1); rec.Code != http.StatusCreated {
			return fmt.Errorf("status %d", rec.Code)
		}
		return nil
	})

	if ok != 1 {
		t.Fatalf("%d users got the last ticket, want exactly 1", ok)
	}
}
