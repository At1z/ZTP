package handler

import (
	"context"
	"net/http"

	"lab1/internal/domain"
)

type ProductService interface {
	Create(ctx context.Context, in domain.ProductInput) (domain.Product, error)
	Get(ctx context.Context, id int64) (domain.Product, error)
	List(ctx context.Context) ([]domain.Product, error)
	Update(ctx context.Context, id int64, in domain.ProductInput) (domain.Product, error)
	Delete(ctx context.Context, id int64) error
	History(ctx context.Context, id int64) ([]domain.HistoryEntry, error)
}

type productHandler struct {
	service ProductService
}

func (h productHandler) create(w http.ResponseWriter, r *http.Request) {
	var in domain.ProductInput
	if !decodeBody(w, r, &in) {
		return
	}
	product, err := h.service.Create(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", APIPrefix+"/products/"+itoa(product.ID))
	writeJSON(w, http.StatusCreated, product)
}

func (h productHandler) list(w http.ResponseWriter, r *http.Request) {
	products, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (h productHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	product, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h productHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in domain.ProductInput
	if !decodeBody(w, r, &in) {
		return
	}
	product, err := h.service.Update(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h productHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h productHandler) history(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	entries, err := h.service.History(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
