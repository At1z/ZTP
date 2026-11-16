package integ

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (e *env) do(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func reserveBody(product, cart uuid.UUID, qty int) string {
	return fmt.Sprintf(`{"product_id":%q,"cart_id":%q,"user_id":%q,"quantity":%d}`, product, cart, uuid.New(), qty)
}

func TestListProducts(t *testing.T) {
	e := newEnv(t, time.Minute)

	rec := e.do("GET", "/products", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var products []struct {
		Name      string `json:"name"`
		Available int    `json:"available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &products); err != nil {
		t.Fatal(err)
	}
	if len(products) < 2 {
		t.Fatalf("got %d seeded products, want at least 2", len(products))
	}
}

func TestGetProductStatusCodes(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 4)

	if rec := e.do("GET", "/products/"+product.String(), ""); rec.Code != http.StatusOK {
		t.Fatalf("existing: status = %d", rec.Code)
	}
	if rec := e.do("GET", "/products/"+uuid.NewString(), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: status = %d", rec.Code)
	}
	if rec := e.do("GET", "/products/not-a-uuid", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed: status = %d", rec.Code)
	}
}

func TestReserveStatusCodes(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 1)

	if rec := e.do("POST", "/reservations", reserveBody(product, uuid.New(), 1)); rec.Code != http.StatusCreated {
		t.Fatalf("first: status = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/reservations", reserveBody(product, uuid.New(), 1)); rec.Code != http.StatusConflict {
		t.Fatalf("second cart: status = %d, want 409", rec.Code)
	}
	if rec := e.do("POST", "/reservations", reserveBody(uuid.New(), uuid.New(), 1)); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown product: status = %d, want 404", rec.Code)
	}
	if rec := e.do("POST", "/reservations", reserveBody(product, uuid.New(), 0)); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero quantity: status = %d, want 400", rec.Code)
	}
	if rec := e.do("POST", "/reservations", `{"nope":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: status = %d, want 400", rec.Code)
	}
}

func TestReleaseAndConfirmOverHTTP(t *testing.T) {
	e := newEnv(t, time.Minute)
	product := e.addProduct(t, 2)
	cart := uuid.New()
	confirmPath := fmt.Sprintf("/reservations/%s/confirm", cart)
	confirmBody := fmt.Sprintf(`{"items":[{"product_id":%q,"quantity":1}]}`, product)
	if rec := e.do("POST", "/reservations", reserveBody(product, cart, 1)); rec.Code != http.StatusCreated {
		t.Fatalf("reserve: status = %d", rec.Code)
	}

	if rec := e.do("DELETE", fmt.Sprintf("/reservations/%s/%s", cart, product), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("release: status = %d", rec.Code)
	}
	if rec := e.do("POST", confirmPath, confirmBody); rec.Code != http.StatusNotFound {
		t.Fatalf("confirm without reservation: status = %d, want 404", rec.Code)
	}

	if rec := e.do("POST", "/reservations", reserveBody(product, cart, 1)); rec.Code != http.StatusCreated {
		t.Fatalf("reserve again: status = %d", rec.Code)
	}
	if rec := e.do("POST", confirmPath, `{"items":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("confirm without items: status = %d, want 400", rec.Code)
	}
	if rec := e.do("POST", confirmPath, confirmBody); rec.Code != http.StatusOK {
		t.Fatalf("confirm: status = %d, body = %s", rec.Code, rec.Body)
	}
}
