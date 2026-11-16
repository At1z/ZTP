CREATE TABLE notifications (
    id           BIGSERIAL PRIMARY KEY,
    channel      TEXT        NOT NULL CHECK (channel IN ('push', 'email')),
    content      TEXT        NOT NULL,
    recipient    TEXT        NOT NULL,
    timezone     TEXT        NOT NULL,
    priority     TEXT        NOT NULL DEFAULT 'low' CHECK (priority IN ('low', 'high')),
    status       TEXT        NOT NULL DEFAULT 'scheduled'
                 CHECK (status IN ('scheduled', 'queued', 'sent', 'failed', 'cancelled')),
    scheduled_at TIMESTAMPTZ NOT NULL,
    attempts     INT         NOT NULL DEFAULT 0,
    last_error   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at      TIMESTAMPTZ
);

-- The scheduler polls for due notifications.
CREATE INDEX notifications_due_idx ON notifications (scheduled_at) WHERE status = 'scheduled';
