package service

import "time"

// Notifications are only delivered between these local hours of the recipient.
const (
	quietEndHour   = 8  // first allowed hour
	quietStartHour = 22 // first forbidden hour
)

// nextAllowedTime returns t if it falls into the recipient's allowed window,
// otherwise the nearest 08:00 in the recipient's time zone.
func nextAllowedTime(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	y, m, d := local.Date()
	switch h := local.Hour(); {
	case h >= quietEndHour && h < quietStartHour:
		return t
	case h >= quietStartHour:
		return time.Date(y, m, d+1, quietEndHour, 0, 0, 0, loc).UTC()
	default:
		return time.Date(y, m, d, quietEndHour, 0, 0, 0, loc).UTC()
	}
}
