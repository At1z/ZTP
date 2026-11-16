package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"lab3/internal/cart/domain"
	"lab3/internal/cart/service"
	"lab3/internal/platform/httpx"

	"github.com/google/uuid"
)

// handler translates HTTP to service calls and service errors to status codes.
type handler struct {
	queries  *service.CartQueryService
	commands *service.CartCommandService
	ping     func(context.Context) error
}

func NewRouter(queries *service.CartQueryService, commands *service.CartCommandService, ping func(context.Context) error) http.Handler {
	h := &handler{queries: queries, commands: commands, ping: ping}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /cart", h.openCart)
	mux.HandleFunc("GET /cart", h.getCart)
	mux.HandleFunc("POST /cart/items", h.addItem)
	mux.HandleFunc("DELETE /cart/items/{productId}", h.removeItem)
	mux.HandleFunc("POST /cart/checkout", h.checkout)
	return mux
}

type itemResponse struct {
	ProductID      uuid.UUID `json:"product_id"`
	ProductName    string    `json:"product_name"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	Quantity       int       `json:"quantity"`
	LineTotalCents int64     `json:"line_total_cents"`
}

type cartResponse struct {
	ID         uuid.UUID      `json:"id"`
	Status     domain.Status  `json:"status"`
	ExpiresAt  time.Time      `json:"expires_at"`
	Items      []itemResponse `json:"items"`
	TotalCents int64          `json:"total_cents"`
}

func toCartResponse(c domain.Cart) cartResponse {
	items := make([]itemResponse, len(c.Items))
	for i, it := range c.Items {
		items[i] = itemResponse{
			ProductID:      it.ProductID,
			ProductName:    it.ProductName,
			UnitPriceCents: it.UnitPriceCents,
			Quantity:       it.Quantity,
			LineTotalCents: it.LineTotalCents(),
		}
	}
	return cartResponse{
		ID:         c.ID,
		Status:     c.Status,
		ExpiresAt:  c.ExpiresAt,
		Items:      items,
		TotalCents: c.TotalCents(),
	}
}

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.ping(r.Context()); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) openCart(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	cart, created, err := h.commands.OpenCart(r.Context(), service.OpenCartCommand{UserID: userID})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, toCartResponse(cart))
}

func (h *handler) getCart(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	cart, err := h.queries.GetCart(r.Context(), service.GetCartQuery{UserID: userID})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *handler) addItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	var req addItemRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cart, err := h.commands.AddItem(r.Context(), service.AddItemCommand{
		UserID:    userID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toCartResponse(cart))
}

func (h *handler) removeItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	productID, err := uuid.Parse(r.PathValue("productId"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "productId must be a UUID")
		return
	}
	cart, err := h.commands.RemoveItem(r.Context(), service.RemoveItemCommand{UserID: userID, ProductID: productID})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCartResponse(cart))
}

type orderResponse struct {
	OrderID    uuid.UUID `json:"order_id"`
	Status     string    `json:"status"`
	TotalCents int64     `json:"total_cents"`
}

func (h *handler) checkout(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	order, err := h.commands.Checkout(r.Context(), service.CheckoutCommand{UserID: userID})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, orderResponse{OrderID: order.ID, Status: order.Status, TotalCents: order.TotalCents})
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrCartNotFound),
		errors.Is(err, domain.ErrItemNotFound),
		errors.Is(err, domain.ErrProductNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInsufficientStock),
		errors.Is(err, domain.ErrEmptyCart),
		errors.Is(err, domain.ErrReservationExpired):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrProductServiceUnavailable):
		log.Printf("product service: %v", err)
		httpx.Error(w, http.StatusBadGateway, "product service unavailable")
	case errors.Is(err, domain.ErrOrderServiceUnavailable):
		log.Printf("order service: %v", err)
		httpx.Error(w, http.StatusBadGateway, "order service unavailable")
	default:
		log.Printf("internal error: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal error")
	}
}
