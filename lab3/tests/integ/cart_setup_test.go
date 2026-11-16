package integ

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cartclient "lab3/internal/cart/client"
	carthandler "lab3/internal/cart/handler"
	cartrepo "lab3/internal/cart/repository"
	cartservice "lab3/internal/cart/service"

	"github.com/google/uuid"
)

// cartEnv is the cart service talking over HTTP to a real product service,
// each on its own database.
type cartEnv struct {
	product  *env
	order    *orderEnv
	commands *cartservice.CartCommandService
	router   http.Handler
}

func newCartEnv(t *testing.T, reservationTTL, cartTTL time.Duration) *cartEnv {
	t.Helper()
	product := newEnv(t, reservationTTL)
	return newCartEnvFor(t, product, product.router, newOrderEnv(t), nil, cartTTL)
}

// newCartEnvFor wires a cart service to the given product service handler and
// order service. wrapOrder, when set, sits in front of the order service so a
// test can make it fail.
func newCartEnvFor(t *testing.T, product *env, productHandler http.Handler, order *orderEnv, wrapOrder func(http.Handler) http.Handler, cartTTL time.Duration) *cartEnv {
	t.Helper()
	productServer := httptest.NewServer(productHandler)
	t.Cleanup(productServer.Close)

	orderHandler := order.router
	if wrapOrder != nil {
		orderHandler = wrapOrder(orderHandler)
	}
	orderServer := httptest.NewServer(orderHandler)
	t.Cleanup(orderServer.Close)

	pool := newDatabase(t, "cart")
	carts := cartrepo.NewCartRepository(pool)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	products := cartclient.NewProductClient(productServer.URL, httpClient)
	orders := cartclient.NewOrderClient(orderServer.URL, httpClient)
	commands := cartservice.NewCartCommandService(carts, products, orders, cartTTL)

	return &cartEnv{
		product:  product,
		order:    order,
		commands: commands,
		router:   carthandler.NewRouter(cartservice.NewCartQueryService(carts), commands, pool.Ping),
	}
}

func (c *cartEnv) do(user uuid.UUID, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != uuid.Nil {
		req.Header.Set("X-User-ID", user.String())
	}
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	return rec
}

func (c *cartEnv) openCart(t *testing.T, user uuid.UUID) {
	t.Helper()
	if rec := c.do(user, "POST", "/cart", ""); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("open cart: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func (c *cartEnv) add(user, product uuid.UUID, qty int) *httptest.ResponseRecorder {
	return c.do(user, "POST", "/cart/items", addBody(product, qty))
}

func (c *cartEnv) remove(user, product uuid.UUID) *httptest.ResponseRecorder {
	return c.do(user, "DELETE", "/cart/items/"+product.String(), "")
}

// reservedQuantity is what the product service has locked for the product.
func (c *cartEnv) reservedQuantity(t *testing.T, product uuid.UUID) int {
	t.Helper()
	var n int
	err := c.product.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(quantity), 0)::int FROM reservations WHERE product_id = $1 AND expires_at > now()`, product).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (c *cartEnv) checkout(user uuid.UUID) *httptest.ResponseRecorder {
	return c.do(user, "POST", "/cart/checkout", "")
}
