package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"lab1/internal/domain"
)

type errorResponse struct {
	Errors []domain.FieldError `json:"errors"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeFieldError(w http.ResponseWriter, status int, errs ...domain.FieldError) {
	writeJSON(w, status, errorResponse{Errors: errs})
}

// writeError maps service/domain errors to HTTP status codes.
func writeError(w http.ResponseWriter, err error) {
	var validation *domain.ValidationError
	var conflict *domain.ConflictError

	switch {
	case errors.As(err, &validation):
		writeFieldError(w, http.StatusUnprocessableEntity, validation.Errors...)
	case errors.As(err, &conflict):
		writeFieldError(w, http.StatusConflict, conflict.FieldError)
	case errors.Is(err, domain.ErrNotFound):
		writeFieldError(w, http.StatusNotFound, domain.FieldError{Field: "id", Message: "resource not found"})
	default:
		log.Printf("internal error: %v", err)
		writeFieldError(w, http.StatusInternalServerError, domain.FieldError{Field: "server", Message: "internal server error"})
	}
}

// maxBodyBytes caps the size of a request body.
const maxBodyBytes = 1 << 20

// decodeBody parses a JSON request body and writes a 400 (or 413 when the body
// is too large) response on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		field, msg := "body", "invalid JSON"
		var typeErr *json.UnmarshalTypeError
		var tooLarge *http.MaxBytesError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			field, msg = typeErr.Field, "must be of type "+typeErr.Type.String()
		}
		if errors.As(err, &tooLarge) {
			writeFieldError(w, http.StatusRequestEntityTooLarge, domain.FieldError{Field: "body", Message: "request body too large"})
			return false
		}
		writeFieldError(w, http.StatusBadRequest, domain.FieldError{Field: field, Message: msg})
		return false
	}
	return true
}

// pathID parses the {id} path parameter and writes a 400 response on failure.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeFieldError(w, http.StatusBadRequest, domain.FieldError{Field: "id", Message: "must be a positive integer"})
		return 0, false
	}
	return id, true
}
