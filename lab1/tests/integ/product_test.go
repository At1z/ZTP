package integ

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateProduct(t *testing.T) {
	reset(t)

	res := send(t, http.MethodPost, "/api/v1/products", validProduct()).expectStatus(t, http.StatusCreated)
	var got product
	res.decode(t, &got)

	if got.ID == 0 || got.Name != "Laptop1" || got.Category != "electronics" || got.Price != 3000 || got.Quantity != 5 {
		t.Fatalf("unexpected product: %+v", got)
	}
}

func TestProductNameRules(t *testing.T) {
	tests := []struct {
		name        string
		productName string
		wantStatus  int
	}{
		{"minimum length 3", "abc", http.StatusCreated},
		{"maximum length 20", strings.Repeat("a", 20), http.StatusCreated},
		{"digits allowed", "abc123", http.StatusCreated},
		{"too short", "ab", http.StatusUnprocessableEntity},
		{"too long", strings.Repeat("a", 21), http.StatusUnprocessableEntity},
		{"empty", "", http.StatusUnprocessableEntity},
		{"space", "ab cd", http.StatusUnprocessableEntity},
		{"special character", "abc!", http.StatusUnprocessableEntity},
		{"underscore", "abc_def", http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reset(t)
			res := send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"name": tc.productName}))
			if tc.wantStatus == http.StatusCreated {
				res.expectStatus(t, tc.wantStatus)
				return
			}
			res.expectErrorField(t, tc.wantStatus, "name")
		})
	}
}

func TestProductNameMustBeUnique(t *testing.T) {
	reset(t)
	createProduct(t, validProduct())

	send(t, http.MethodPost, "/api/v1/products", validProduct()).
		expectErrorField(t, http.StatusConflict, "name")
}

func TestProductPriceLimitsPerCategory(t *testing.T) {
	tests := []struct {
		category string
		price    float64
		wantOK   bool
	}{
		{"electronics", 50, true},
		{"electronics", 50000, true},
		{"electronics", 49.99, false},
		{"electronics", 50000.01, false},
		{"books", 5, true},
		{"books", 500, true},
		{"books", 4.99, false},
		{"books", 500.01, false},
		{"clothing", 10, true},
		{"clothing", 5000, true},
		{"clothing", 9.99, false},
		{"clothing", 5000.01, false},
	}
	for _, tc := range tests {
		t.Run(tc.category, func(t *testing.T) {
			reset(t)
			res := send(t, http.MethodPost, "/api/v1/products",
				productWith(map[string]any{"category": tc.category, "price": tc.price}))
			if tc.wantOK {
				res.expectStatus(t, http.StatusCreated)
				return
			}
			res.expectErrorField(t, http.StatusUnprocessableEntity, "price")
		})
	}
}

func TestProductCategoryMustBeKnown(t *testing.T) {
	reset(t)
	send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"category": "toys"})).
		expectErrorField(t, http.StatusUnprocessableEntity, "category")
}

func TestProductQuantityRules(t *testing.T) {
	reset(t)

	send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"quantity": 0})).
		expectStatus(t, http.StatusCreated)

	send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"name": "Other1", "quantity": -1})).
		expectErrorField(t, http.StatusUnprocessableEntity, "quantity")

	send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"name": "Other2", "quantity": 1.5})).
		expectErrorField(t, http.StatusBadRequest, "quantity")
}

func TestMissingPriceAndQuantityAreRejected(t *testing.T) {
	reset(t)
	send(t, http.MethodPost, "/api/v1/products", map[string]any{"name": "Laptop1", "category": "books"}).
		expectErrorField(t, http.StatusUnprocessableEntity, "price")
}

func TestValidationReportsAllInvalidFields(t *testing.T) {
	reset(t)

	res := send(t, http.MethodPost, "/api/v1/products",
		map[string]any{"name": "a!", "category": "books", "price": 1000, "quantity": -5})
	res.expectStatus(t, http.StatusUnprocessableEntity)

	got := strings.Join(res.errorFields(t), ",")
	if got != "name,price,quantity" {
		t.Fatalf("error fields = %q, want name,price,quantity", got)
	}
}

func TestMalformedJSONIsRejected(t *testing.T) {
	reset(t)
	send(t, http.MethodPost, "/api/v1/products", "not an object").
		expectErrorField(t, http.StatusBadRequest, "body")
}

func TestGetAndListProducts(t *testing.T) {
	reset(t)
	first := createProduct(t, validProduct())
	createProduct(t, productWith(map[string]any{"name": "Book123", "category": "books", "price": 40}))

	var got product
	send(t, http.MethodGet, productPath(first.ID), nil).expectStatus(t, http.StatusOK).decode(t, &got)
	if got.ID != first.ID || got.Name != "Laptop1" {
		t.Fatalf("unexpected product: %+v", got)
	}

	var list []product
	send(t, http.MethodGet, "/api/v1/products", nil).expectStatus(t, http.StatusOK).decode(t, &list)
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2", len(list))
	}

	send(t, http.MethodGet, "/api/v1/products/9999", nil).expectErrorField(t, http.StatusNotFound, "id")
	send(t, http.MethodGet, "/api/v1/products/abc", nil).expectErrorField(t, http.StatusBadRequest, "id")
}

func TestListProductsWhenEmpty(t *testing.T) {
	reset(t)
	res := send(t, http.MethodGet, "/api/v1/products", nil).expectStatus(t, http.StatusOK)
	if strings.TrimSpace(string(res.body)) != "[]" {
		t.Fatalf("body = %s, want []", res.body)
	}
}

func TestUpdateProduct(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())

	var updated product
	send(t, http.MethodPut, productPath(created.ID),
		productWith(map[string]any{"name": "Laptop2", "price": 2500, "quantity": 3})).
		expectStatus(t, http.StatusOK).decode(t, &updated)
	if updated.ID != created.ID || updated.Name != "Laptop2" || updated.Price != 2500 || updated.Quantity != 3 {
		t.Fatalf("unexpected product: %+v", updated)
	}

	var stored product
	send(t, http.MethodGet, productPath(created.ID), nil).expectStatus(t, http.StatusOK).decode(t, &stored)
	if stored.Name != "Laptop2" {
		t.Fatalf("update was not persisted: %+v", stored)
	}
}

func TestUpdateProductValidation(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())
	other := createProduct(t, productWith(map[string]any{"name": "Other1"}))
	path := productPath(created.ID)

	send(t, http.MethodPut, path, productWith(map[string]any{"name": "ab"})).
		expectErrorField(t, http.StatusUnprocessableEntity, "name")
	send(t, http.MethodPut, path, productWith(map[string]any{"price": 1})).
		expectErrorField(t, http.StatusUnprocessableEntity, "price")
	send(t, http.MethodPut, path, productWith(map[string]any{"quantity": -1})).
		expectErrorField(t, http.StatusUnprocessableEntity, "quantity")
	send(t, http.MethodPut, path, productWith(map[string]any{"name": "Other1"})).
		expectErrorField(t, http.StatusConflict, "name")

	// keeping the product's own name is not a conflict
	send(t, http.MethodPut, path, productWith(map[string]any{"price": 2000})).
		expectStatus(t, http.StatusOK)

	send(t, http.MethodPut, productPath(other.ID+100), validProduct()).
		expectErrorField(t, http.StatusNotFound, "id")
}

func TestDeleteProduct(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())

	send(t, http.MethodDelete, productPath(created.ID), nil).expectStatus(t, http.StatusNoContent)
	send(t, http.MethodGet, productPath(created.ID), nil).expectErrorField(t, http.StatusNotFound, "id")
	send(t, http.MethodDelete, productPath(created.ID), nil).expectErrorField(t, http.StatusNotFound, "id")
}
