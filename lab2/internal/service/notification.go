package service

import (
	"context"
	"errors"
	"strings"
	"time"
	_ "time/tzdata" // time zone database for minimal container images
	"unicode/utf8"

	"lab2/internal/domain"
)

type NotificationRepository interface {
	Create(ctx context.Context, n *domain.Notification) error
	GetByID(ctx context.Context, id int64) (domain.Notification, error)
	List(ctx context.Context) ([]domain.Notification, error)
	Cancel(ctx context.Context, id int64) (domain.Notification, error)
	SendNow(ctx context.Context, id int64) (domain.Notification, error)
}

type NotificationService struct {
	repo NotificationRepository
	now  func() time.Time
}

func NewNotificationService(repo NotificationRepository, now func() time.Time) *NotificationService {
	return &NotificationService{repo: repo, now: now}
}

func (s *NotificationService) Create(ctx context.Context, in domain.NotificationInput) (domain.Notification, error) {
	loc, errs := validateNotification(in)
	if len(errs) > 0 {
		return domain.Notification{}, &domain.ValidationError{Errors: errs}
	}

	priority := in.Priority
	if priority == "" {
		priority = domain.PriorityLow
	}
	sendAt := s.now()
	if in.SendAt != nil && in.SendAt.After(sendAt) {
		sendAt = *in.SendAt
	}

	n := domain.Notification{
		Channel:     in.Channel,
		Content:     in.Content,
		Recipient:   in.Recipient,
		Timezone:    in.Timezone,
		Priority:    priority,
		ScheduledAt: nextAllowedTime(sendAt, loc).UTC(),
	}
	if err := s.repo.Create(ctx, &n); err != nil {
		return domain.Notification{}, err
	}
	return n, nil
}

func (s *NotificationService) Get(ctx context.Context, id int64) (domain.Notification, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *NotificationService) List(ctx context.Context) ([]domain.Notification, error) {
	return s.repo.List(ctx)
}

// Cancel stops a scheduled or queued notification.
func (s *NotificationService) Cancel(ctx context.Context, id int64) (domain.Notification, error) {
	n, err := s.repo.Cancel(ctx, id)
	return n, stateConflict(err, "only scheduled or queued notifications can be cancelled")
}

// SendNow skips the planned time and the quiet hours of a scheduled notification.
func (s *NotificationService) SendNow(ctx context.Context, id int64) (domain.Notification, error) {
	n, err := s.repo.SendNow(ctx, id)
	return n, stateConflict(err, "only scheduled notifications can be sent now")
}

func stateConflict(err error, msg string) error {
	if errors.Is(err, domain.ErrInvalidState) {
		return &domain.ConflictError{FieldError: domain.FieldError{Field: "status", Message: msg}}
	}
	return err
}

// validateNotification applies all rules and reports every invalid field at
// once. The returned location is only valid when there are no errors.
func validateNotification(in domain.NotificationInput) (*time.Location, []domain.FieldError) {
	var errs []domain.FieldError

	switch in.Channel {
	case domain.ChannelEmail, domain.ChannelPush:
	default:
		errs = append(errs, domain.FieldError{Field: "channel", Message: "must be one of: email, push"})
	}

	switch {
	case strings.TrimSpace(in.Content) == "":
		errs = append(errs, domain.FieldError{Field: "content", Message: "is required"})
	case utf8.RuneCountInString(in.Content) > domain.ContentMaxLength:
		errs = append(errs, domain.FieldError{Field: "content", Message: "must be at most 500 characters"})
	}

	switch {
	case strings.TrimSpace(in.Recipient) == "":
		errs = append(errs, domain.FieldError{Field: "recipient", Message: "is required"})
	case in.Channel == domain.ChannelEmail && !strings.Contains(in.Recipient, "@"):
		errs = append(errs, domain.FieldError{Field: "recipient", Message: "must be an email address"})
	}

	loc, err := time.LoadLocation(in.Timezone)
	if in.Timezone == "" || in.Timezone == "Local" || err != nil {
		errs = append(errs, domain.FieldError{Field: "timezone", Message: "must be an IANA time zone, e.g. Europe/Warsaw"})
	}

	switch in.Priority {
	case "", domain.PriorityLow, domain.PriorityHigh:
	default:
		errs = append(errs, domain.FieldError{Field: "priority", Message: "must be one of: low, high"})
	}
	return loc, errs
}
