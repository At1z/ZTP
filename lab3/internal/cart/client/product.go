package client

import (
	"context"
	"fmt"
	"net/http"

	"lab3/internal/cart/domain"

	"github.com/google/uuid"
)

// ProductClient calls the product service over HTTP.
type ProductClient struct {
	jsonClient
}

func NewProductClient(baseURL string, httpClient *http.Client) *ProductClient {
	return &ProductClient{jsonClient{baseURL: baseURL, http: httpClient}}
}

func (c *ProductClient) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	var out struct {
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
		PriceCents int64     `json:"price_cents"`
	}
	if err := c.call(ctx, http.MethodGet, "/products/"+id.String(), nil, &out, productStatusError); err != nil {
		return domain.Product{}, err
	}
	return domain.Product{ID: out.ID, Name: out.Name, PriceCents: out.PriceCents}, nil
}

func (c *ProductClient) Reserve(ctx context.Context, req domain.ReserveRequest) error {
	body := map[string]any{
		"product_id": req.ProductID,
		"cart_id":    req.CartID,
		"user_id":    req.UserID,
		"quantity":   req.Quantity,
	}
	return c.call(ctx, http.MethodPost, "/reservations", body, nil, productStatusError)
}

func (c *ProductClient) Release(ctx context.Context, cartID, productID uuid.UUID) error {
	path := fmt.Sprintf("/reservations/%s/%s", cartID, productID)
	return c.call(ctx, http.MethodDelete, path, nil, nil, productStatusError)
}

func (c *ProductClient) ReleaseCart(ctx context.Context, cartID uuid.UUID) error {
	return c.call(ctx, http.MethodDelete, "/reservations/"+cartID.String(), nil, nil, productStatusError)
}

func (c *ProductClient) Confirm(ctx context.Context, cartID uuid.UUID, items []domain.ConfirmItem) error {
	type line struct {
		ProductID uuid.UUID `json:"product_id"`
		Quantity  int       `json:"quantity"`
	}
	lines := make([]line, len(items))
	for i, it := range items {
		lines[i] = line{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	body := map[string]any{"items": lines}
	return c.call(ctx, http.MethodPost, fmt.Sprintf("/reservations/%s/confirm", cartID), body, nil, confirmStatusError)
}

func (c *ProductClient) call(ctx context.Context, method, path string, in, out any, mapStatus func(int) error) error {
	status, err := c.do(ctx, method, path, uuid.Nil, in, out)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrProductServiceUnavailable, err)
	}
	return mapStatus(status)
}

func productStatusError(status int) error {
	switch {
	case isSuccess(status):
		return nil
	case status == http.StatusNotFound:
		return domain.ErrProductNotFound
	case status == http.StatusConflict:
		return domain.ErrInsufficientStock
	case status == http.StatusBadRequest:
		return domain.ErrInvalidInput
	default:
		return fmt.Errorf("%w: status %d", domain.ErrProductServiceUnavailable, status)
	}
}

// For a confirmation, "not found" and "conflict" both mean that a reservation
// is gone, expired or does not match the cart.
func confirmStatusError(status int) error {
	switch {
	case isSuccess(status):
		return nil
	case status == http.StatusNotFound, status == http.StatusConflict:
		return domain.ErrReservationExpired
	case status == http.StatusBadRequest:
		return domain.ErrInvalidInput
	default:
		return fmt.Errorf("%w: status %d", domain.ErrProductServiceUnavailable, status)
	}
}
