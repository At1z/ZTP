package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"lab3/internal/platform/httpx"

	"github.com/google/uuid"
)

// jsonClient sends JSON requests to one service. It is shared by the clients
// of the product and order services.
type jsonClient struct {
	baseURL string
	http    *http.Client
}

// do sends one request and returns the response status. The body is decoded
// into out only for 2xx responses. The error covers transport and decoding
// failures only; mapping a status to a domain error is up to the caller.
func (c *jsonClient) do(ctx context.Context, method, path string, userID uuid.UUID, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != uuid.Nil {
		req.Header.Set(httpx.UserHeader, userID.String())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func isSuccess(status int) bool {
	return status >= 200 && status < 300
}
