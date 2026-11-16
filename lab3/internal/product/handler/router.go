package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"lab3/internal/platform/httpx"
	"lab3/internal/product/domain"
	"lab3/internal/product/service"

	"github.com/google/uuid"
)

// handler translates HTTP to service calls and service errors to status codes.
type handler struct {
	products     *service.ProductService
	reservations *service.ReservationService
	ping         func(context.Context) error
}

func NewRouter(products *service.ProductService, reservations *service.ReservationService, ping func(context.Context) error) http.Handler {
	s := &handler{products: products, reservations: reservations, ping: ping}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /products", s.listProducts)
	mux.HandleFunc("GET /products/{id}", s.getProduct)
	mux.HandleFunc("POST /reservations", s.reserve)
	mux.HandleFunc("DELETE /reservations/{cartId}", s.releaseCart)
	mux.HandleFunc("DELETE /reservations/{cartId}/{productId}", s.releaseReservation)
	mux.HandleFunc("POST /reservations/{cartId}/confirm", s.confirm)
	return mux
}

type productResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceCents  int64     `json:"price_cents"`
	Stock       int       `json:"stock"`
	Available   int       `json:"available"`
}

func toProductResponse(p domain.Product) productResponse {
	return productResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		PriceCents:  p.PriceCents,
		Stock:       p.Stock,
		Available:   p.Available,
	}
}

type reserveRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	CartID    uuid.UUID `json:"cart_id"`
	UserID    uuid.UUID `json:"user_id"`
	Quantity  int       `json:"quantity"`
}

type reservationResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	CartID    uuid.UUID `json:"cart_id"`
	Quantity  int       `json:"quantity"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *handler) health(w http.ResponseWriter, r *http.Request) {
	if err := s.ping(r.Context()); err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *handler) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := s.products.ListProducts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]productResponse, len(products))
	for i, p := range products {
		out[i] = toProductResponse(p)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	p, err := s.products.GetProduct(r.Context(), service.GetProductQuery{ID: id})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toProductResponse(p))
}

func (s *handler) reserve(w http.ResponseWriter, r *http.Request) {
	var req reserveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := s.reservations.Reserve(r.Context(), service.ReserveProductCommand{
		ProductID: req.ProductID,
		CartID:    req.CartID,
		UserID:    req.UserID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, reservationResponse{
		ID:        res.ID,
		ProductID: res.ProductID,
		CartID:    res.CartID,
		Quantity:  res.Quantity,
		ExpiresAt: res.ExpiresAt,
	})
}

func (s *handler) releaseReservation(w http.ResponseWriter, r *http.Request) {
	cartID, ok := pathUUID(w, r, "cartId")
	if !ok {
		return
	}
	productID, ok := pathUUID(w, r, "productId")
	if !ok {
		return
	}
	err := s.reservations.Release(r.Context(), service.ReleaseReservationCommand{CartID: cartID, ProductID: productID})
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *handler) releaseCart(w http.ResponseWriter, r *http.Request) {
	cartID, ok := pathUUID(w, r, "cartId")
	if !ok {
		return
	}
	if err := s.reservations.ReleaseCart(r.Context(), service.ReleaseCartCommand{CartID: cartID}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *handler) confirm(w http.ResponseWriter, r *http.Request) {
	cartID, ok := pathUUID(w, r, "cartId")
	if !ok {
		return
	}
	if err := s.reservations.Confirm(r.Context(), service.ConfirmReservationsCommand{CartID: cartID}); err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, name+" must be a UUID")
		return uuid.Nil, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrReservationNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInsufficientStock), errors.Is(err, domain.ErrReservationExpired):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		log.Printf("internal error: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal error")
	}
}
