package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

// Pinger is implemented by anything that can report its own availability.
type Pinger interface {
	Ping(ctx context.Context) error
}

func NewRouter(db, broker Pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(db, broker))
	return mux
}

func healthHandler(db, broker Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := broker.Ping(r.Context()); err != nil {
			http.Error(w, "broker unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
