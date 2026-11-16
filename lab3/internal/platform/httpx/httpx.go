package httpx

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error string `json:"error"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, errorBody{Error: message})
}

// DecodeJSON reads a request body into dst, rejecting unknown fields and
// oversized bodies.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

const UserHeader = "X-User-ID"

// UserID identifies the caller by the UUID in the X-User-ID header and writes
// 401 when it is missing or malformed.
func UserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.Header.Get(UserHeader))
	if err != nil || id == uuid.Nil {
		Error(w, http.StatusUnauthorized, UserHeader+" header with a user UUID is required")
		return uuid.Nil, false
	}
	return id, true
}
