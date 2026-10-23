package handler

import (
	"context"
	"net/http"

	"lab1/internal/domain"
)

type BannedPhraseService interface {
	List(ctx context.Context) ([]domain.BannedPhrase, error)
	Add(ctx context.Context, in domain.BannedPhraseInput) (domain.BannedPhrase, error)
	Delete(ctx context.Context, id int64) error
}

type bannedPhraseHandler struct {
	service BannedPhraseService
}

func (h bannedPhraseHandler) list(w http.ResponseWriter, r *http.Request) {
	phrases, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, phrases)
}

func (h bannedPhraseHandler) add(w http.ResponseWriter, r *http.Request) {
	var in domain.BannedPhraseInput
	if !decodeBody(w, r, &in) {
		return
	}
	phrase, err := h.service.Add(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, phrase)
}

func (h bannedPhraseHandler) delete(w http.ResponseWriter, r *http.Request) {
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
