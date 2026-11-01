package handler

import (
	"context"
	"net/http"
	"strconv"

	"lab2/internal/domain"
)

// APIPrefix is the versioned base path of all resource endpoints.
const APIPrefix = "/api/v1"

type NotificationService interface {
	Create(ctx context.Context, in domain.NotificationInput) (domain.Notification, error)
	Get(ctx context.Context, id int64) (domain.Notification, error)
	List(ctx context.Context) ([]domain.Notification, error)
	Cancel(ctx context.Context, id int64) (domain.Notification, error)
	SendNow(ctx context.Context, id int64) (domain.Notification, error)
}

type notificationHandler struct {
	service NotificationService
}

func (h notificationHandler) create(w http.ResponseWriter, r *http.Request) {
	var in domain.NotificationInput
	if !decodeBody(w, r, &in) {
		return
	}
	n, err := h.service.Create(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", APIPrefix+"/notifications/"+strconv.FormatInt(n.ID, 10))
	writeJSON(w, http.StatusCreated, n)
}

func (h notificationHandler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h notificationHandler) get(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.service.Get)
}

func (h notificationHandler) cancel(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.service.Cancel)
}

func (h notificationHandler) sendNow(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.service.SendNow)
}

// byID runs an operation on the notification named by the {id} path parameter.
func (h notificationHandler) byID(w http.ResponseWriter, r *http.Request,
	op func(context.Context, int64) (domain.Notification, error)) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := op(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}
