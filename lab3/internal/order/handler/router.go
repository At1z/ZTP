package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"lab3/internal/order/domain"
	"lab3/internal/order/service"
	"lab3/internal/platform/httpx"

	"github.com/google/uuid"
)

// handler translates HTTP to service calls and service errors to status codes.
type handler struct {
	queries  *service.OrderQueryService
	commands *service.OrderCommandService
	ping     func(context.Context) error
}

func NewRouter(queries *service.OrderQueryService, commands *service.OrderCommandService, ping func(context.Context) error) http.Handler {
	h := &handler{queries: queries, commands: commands, ping: ping}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /orders", h.createOrder)
	mux.HandleFunc("GET /orders/{id}", h.getOrder)
	mux.HandleFunc("GET /notifications", h.listNotifications)
	return mux
}

type itemDTO struct {
	ProductID      uuid.UUID `json:"product_id"`
	ProductName    string    `json:"product_name"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	Quantity       int       `json:"quantity"`
}

type createOrderRequest struct {
	CartID uuid.UUID `json:"cart_id"`
	Items  []itemDTO `json:"items"`
}

type orderResponse struct {
	ID         uuid.UUID     `json:"id"`
	CartID     uuid.UUID     `json:"cart_id"`
	Status     domain.Status `json:"status"`
	TotalCents int64         `json:"total_cents"`
	CreatedAt  time.Time     `json:"created_at"`
	Items      []itemDTO     `json:"items"`
}

type notificationResponse struct {
	ID        uuid.UUID `json:"id"`
	OrderID   uuid.UUID `json:"order_id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

func toOrderResponse(o domain.Order) orderResponse {
	items := make([]itemDTO, len(o.Items))
	for i, it := range o.Items {
		items[i] = itemDTO{ProductID: it.ProductID, ProductName: it.ProductName, UnitPriceCents: it.UnitPriceCents, Quantity: it.Quantity}
	}
	return orderResponse{
		ID:         o.ID,
		CartID:     o.CartID,
		Status:     o.Status,
		TotalCents: o.TotalCents,
		CreatedAt:  o.CreatedAt,
		Items:      items,
	}
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.ping(r.Context()); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// createOrder is called by the cart service. It answers 201 for a new order
// and 200 when the cart already had one (a retried request).
func (h *handler) createOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	var req createOrderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	items := make([]domain.Item, len(req.Items))
	for i, it := range req.Items {
		items[i] = domain.Item{ProductID: it.ProductID, ProductName: it.ProductName, UnitPriceCents: it.UnitPriceCents, Quantity: it.Quantity}
	}

	order, created, err := h.commands.CreateOrder(r.Context(), service.CreateOrderCommand{
		UserID: userID,
		CartID: req.CartID,
		Items:  items,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, toOrderResponse(order))
}

func (h *handler) getOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	order, err := h.queries.GetOrder(r.Context(), service.GetOrderQuery{UserID: userID, OrderID: id})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toOrderResponse(order))
}

func (h *handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	list, err := h.queries.ListNotifications(r.Context(), service.ListNotificationsQuery{UserID: userID})
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]notificationResponse, len(list))
	for i, n := range list {
		out[i] = notificationResponse{ID: n.ID, OrderID: n.OrderID, Message: n.Message, CreatedAt: n.CreatedAt}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrOrderNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	default:
		log.Printf("internal error: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal error")
	}
}
