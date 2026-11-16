package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"lab3/internal/cart/domain"

	"github.com/google/uuid"
)

// ProductClient calls the product service over HTTP.
type ProductClient struct {
	baseURL string
	http    *http.Client
}

func NewProductClient(baseURL string, httpClient *http.Client) *ProductClient {
	return &ProductClient{baseURL: baseURL, http: httpClient}
}

func (c *ProductClient) GetProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	var out struct {
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
		PriceCents int64     `json:"price_cents"`
	}
	if err := c.call(ctx, http.MethodGet, "/products/"+id.String(), nil, &out); err != nil {
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
	return c.call(ctx, http.MethodPost, "/reservations", body, nil)
}

func (c *ProductClient) Release(ctx context.Context, cartID, productID uuid.UUID) error {
	return c.call(ctx, http.MethodDelete, fmt.Sprintf("/reservations/%s/%s", cartID, productID), nil, nil)
}

func (c *ProductClient) ReleaseCart(ctx context.Context, cartID uuid.UUID) error {
	return c.call(ctx, http.MethodDelete, "/reservations/"+cartID.String(), nil, nil)
}

// call sends one request and maps the response status to a domain error.
func (c *ProductClient) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrProductServiceUnavailable, err)
	}
	defer resp.Body.Close()

	if err := statusError(resp.StatusCode); err != nil {
		return err
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("%w: decode response: %v", domain.ErrProductServiceUnavailable, err)
		}
	}
	return nil
}

func statusError(status int) error {
	switch {
	case status >= 200 && status < 300:
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
