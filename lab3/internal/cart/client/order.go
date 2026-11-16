package client

import (
	"context"
	"fmt"
	"net/http"

	"lab3/internal/cart/domain"

	"github.com/google/uuid"
)

// OrderClient calls the order service over HTTP.
type OrderClient struct {
	jsonClient
}

func NewOrderClient(baseURL string, httpClient *http.Client) *OrderClient {
	return &OrderClient{jsonClient{baseURL: baseURL, http: httpClient}}
}

func (c *OrderClient) CreateOrder(ctx context.Context, req domain.CreateOrderRequest) (domain.Order, error) {
	type line struct {
		ProductID      uuid.UUID `json:"product_id"`
		ProductName    string    `json:"product_name"`
		UnitPriceCents int64     `json:"unit_price_cents"`
		Quantity       int       `json:"quantity"`
	}
	lines := make([]line, len(req.Items))
	for i, it := range req.Items {
		lines[i] = line{ProductID: it.ProductID, ProductName: it.ProductName, UnitPriceCents: it.UnitPriceCents, Quantity: it.Quantity}
	}
	body := map[string]any{"cart_id": req.CartID, "items": lines}

	var out struct {
		ID         uuid.UUID `json:"id"`
		Status     string    `json:"status"`
		TotalCents int64     `json:"total_cents"`
	}
	status, err := c.do(ctx, http.MethodPost, "/orders", req.UserID, body, &out)
	if err != nil {
		return domain.Order{}, fmt.Errorf("%w: %v", domain.ErrOrderServiceUnavailable, err)
	}
	switch {
	case isSuccess(status):
		return domain.Order{ID: out.ID, Status: out.Status, TotalCents: out.TotalCents}, nil
	case status == http.StatusBadRequest:
		return domain.Order{}, domain.ErrInvalidInput
	default:
		return domain.Order{}, fmt.Errorf("%w: status %d", domain.ErrOrderServiceUnavailable, status)
	}
}
