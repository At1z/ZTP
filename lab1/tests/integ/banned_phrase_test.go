package integ

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBannedPhraseBlocksCreate(t *testing.T) {
	reset(t)
	addBannedPhrase(t, "spam")

	send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"name": "MegaSpamDeal"})).
		expectErrorField(t, http.StatusUnprocessableEntity, "name")
	send(t, http.MethodPost, "/api/v1/products", productWith(map[string]any{"name": "Fine123"})).
		expectStatus(t, http.StatusCreated)
}

func TestBannedPhraseBlocksUpdate(t *testing.T) {
	reset(t)
	created := createProduct(t, validProduct())
	addBannedPhrase(t, "spam")

	send(t, http.MethodPut, productPath(created.ID), productWith(map[string]any{"name": "xSPAMx"})).
		expectErrorField(t, http.StatusUnprocessableEntity, "name")
}

func TestBannedPhraseListIsDynamic(t *testing.T) {
	reset(t)
	payload := productWith(map[string]any{"name": "BadWord1"})

	send(t, http.MethodPost, "/api/v1/products", payload).expectStatus(t, http.StatusCreated)
	reset(t)

	phrase := addBannedPhrase(t, "badword")
	send(t, http.MethodPost, "/api/v1/products", payload).
		expectErrorField(t, http.StatusUnprocessableEntity, "name")

	send(t, http.MethodDelete, fmt.Sprintf("/api/v1/banned-phrases/%d", phrase.ID), nil).
		expectStatus(t, http.StatusNoContent)
	send(t, http.MethodPost, "/api/v1/products", payload).expectStatus(t, http.StatusCreated)
}

func TestBannedPhraseManagement(t *testing.T) {
	reset(t)

	phrase := addBannedPhrase(t, "  Spam ")
	if phrase.Phrase != "spam" {
		t.Fatalf("phrase = %q, want normalized %q", phrase.Phrase, "spam")
	}

	send(t, http.MethodPost, "/api/v1/banned-phrases", map[string]any{"phrase": "SPAM"}).
		expectErrorField(t, http.StatusConflict, "phrase")
	send(t, http.MethodPost, "/api/v1/banned-phrases", map[string]any{"phrase": "   "}).
		expectErrorField(t, http.StatusUnprocessableEntity, "phrase")

	var list []bannedPhrase
	send(t, http.MethodGet, "/api/v1/banned-phrases", nil).expectStatus(t, http.StatusOK).decode(t, &list)
	if len(list) != 1 || list[0].Phrase != "spam" {
		t.Fatalf("unexpected list: %+v", list)
	}

	send(t, http.MethodDelete, "/api/v1/banned-phrases/9999", nil).expectErrorField(t, http.StatusNotFound, "id")
}
