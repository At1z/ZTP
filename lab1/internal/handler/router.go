package handler

import (
	"context"
	"net/http"
	"strconv"
)

// APIPrefix is the versioned base path of all resource endpoints.
const APIPrefix = "/api/v1"

type Pinger interface {
	Ping(ctx context.Context) error
}

func NewRouter(products ProductService, phrases BannedPhraseService, db Pinger) http.Handler {
	ph := productHandler{service: products}
	bh := bannedPhraseHandler{service: phrases}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(db))

	mux.HandleFunc("POST "+APIPrefix+"/products", ph.create)
	mux.HandleFunc("GET "+APIPrefix+"/products", ph.list)
	mux.HandleFunc("GET "+APIPrefix+"/products/{id}", ph.get)
	mux.HandleFunc("PUT "+APIPrefix+"/products/{id}", ph.update)
	mux.HandleFunc("DELETE "+APIPrefix+"/products/{id}", ph.delete)
	mux.HandleFunc("GET "+APIPrefix+"/products/{id}/history", ph.history)

	mux.HandleFunc("GET "+APIPrefix+"/banned-phrases", bh.list)
	mux.HandleFunc("POST "+APIPrefix+"/banned-phrases", bh.add)
	mux.HandleFunc("DELETE "+APIPrefix+"/banned-phrases/{id}", bh.delete)
	return mux
}

func healthHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
