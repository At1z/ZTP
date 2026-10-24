package integ

import (
	"context"
	"net/http"
	"testing"
)

func getHistory(t *testing.T, id int64) []historyEntry {
	t.Helper()
	var entries []historyEntry
	send(t, http.MethodGet, productPath(id)+"/history", nil).
		expectStatus(t, http.StatusOK).decode(t, &entries)
	return entries
}

func TestHistoryRecordsChangedFields(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())

	send(t, http.MethodPut, productPath(created.ID),
		productWith(map[string]any{"price": 2500, "quantity": 3})).
		expectStatus(t, http.StatusOK)

	got := map[string]historyEntry{}
	for _, e := range getHistory(t, created.ID) {
		got[e.Field] = e
	}
	if len(got) != 2 {
		t.Fatalf("history fields = %v, want only price and quantity", got)
	}
	if e := got["price"]; e.OldValue != "3000.00" || e.NewValue != "2500.00" {
		t.Errorf("price entry = %+v", e)
	}
	if e := got["quantity"]; e.OldValue != "5" || e.NewValue != "3" {
		t.Errorf("quantity entry = %+v", e)
	}
}

func TestHistoryAccumulatesAcrossUpdates(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())

	for _, qty := range []int{4, 3, 2} {
		send(t, http.MethodPut, productPath(created.ID), productWith(map[string]any{"quantity": qty})).
			expectStatus(t, http.StatusOK)
	}

	entries := getHistory(t, created.ID)
	if len(entries) != 3 {
		t.Fatalf("history length = %d, want 3", len(entries))
	}
	if entries[0].OldValue != "5" || entries[2].NewValue != "2" {
		t.Fatalf("unexpected order or values: %+v", entries)
	}
}

func TestDeletingProductRemovesItsHistory(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())
	send(t, http.MethodPut, productPath(created.ID), productWith(map[string]any{"price": 2500})).
		expectStatus(t, http.StatusOK)
	send(t, http.MethodDelete, productPath(created.ID), nil).expectStatus(t, http.StatusNoContent)

	var rows int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM product_history WHERE product_id = $1", created.ID).Scan(&rows)
	if err != nil || rows != 0 {
		t.Fatalf("orphaned history rows = %d, err = %v", rows, err)
	}
}

func TestHistoryIgnoresUnchangedUpdateAndFailedUpdate(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())

	send(t, http.MethodPut, productPath(created.ID), validProduct()).expectStatus(t, http.StatusOK)
	send(t, http.MethodPut, productPath(created.ID), productWith(map[string]any{"price": 1})).
		expectStatus(t, http.StatusUnprocessableEntity)

	if entries := getHistory(t, created.ID); len(entries) != 0 {
		t.Fatalf("history = %+v, want empty", entries)
	}
}

func TestHistoryTracksNameAndCategory(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())

	send(t, http.MethodPut, productPath(created.ID),
		productWith(map[string]any{"name": "Renamed1", "category": "clothing", "price": 100})).
		expectStatus(t, http.StatusOK)

	fields := map[string]bool{}
	for _, e := range getHistory(t, created.ID) {
		fields[e.Field] = true
	}
	for _, want := range []string{"name", "category", "price"} {
		if !fields[want] {
			t.Errorf("missing history entry for %q", want)
		}
	}
}

func TestHistoryForUnknownProduct(t *testing.T) {
	reset(t)
	send(t, http.MethodGet, "/api/v1/products/9999/history", nil).expectErrorField(t, http.StatusNotFound, "id")
}
