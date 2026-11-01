package handler

import (
	"context"
	"net/http"
)

// Pinger is implemented by anything that can report its own availability.
type Pinger interface {
	Ping(ctx context.Context) error
}

func NewRouter(notifications NotificationService, db, broker Pinger) http.Handler {
	nh := notificationHandler{service: notifications}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(db, broker))

	mux.HandleFunc("POST "+APIPrefix+"/notifications", nh.create)
	mux.HandleFunc("GET "+APIPrefix+"/notifications", nh.list)
	mux.HandleFunc("GET "+APIPrefix+"/notifications/{id}", nh.get)
	mux.HandleFunc("POST "+APIPrefix+"/notifications/{id}/send-now", nh.sendNow)
	mux.HandleFunc("POST "+APIPrefix+"/notifications/{id}/cancel", nh.cancel)
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
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
