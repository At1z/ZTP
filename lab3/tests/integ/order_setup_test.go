package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	orderhandler "lab3/internal/order/handler"
	orderrepo "lab3/internal/order/repository"
	orderservice "lab3/internal/order/service"

	"github.com/google/uuid"
)

// orderEnv is the order service on its own fresh database, wired like cmd/order.
type orderEnv struct {
	router http.Handler
}

func newOrderEnv(t *testing.T) *orderEnv {
	t.Helper()
	pool := newDatabase(t, "order")
	orders := orderrepo.NewOrderRepository(pool)
	notifications := orderrepo.NewNotificationRepository(pool)
	return &orderEnv{router: orderhandler.NewRouter(
		orderservice.NewOrderQueryService(orders, notifications),
		orderservice.NewOrderCommandService(orders),
		pool.Ping,
	)}
}

func (o *orderEnv) do(user uuid.UUID, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != uuid.Nil {
		req.Header.Set("X-User-ID", user.String())
	}
	rec := httptest.NewRecorder()
	o.router.ServeHTTP(rec, req)
	return rec
}
