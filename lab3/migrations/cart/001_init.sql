-- user_id and product_id belong to other domains, hence no foreign keys.
CREATE TABLE carts (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'checked_out', 'expired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- At most one open cart per user, so requests from several devices of the
-- same user always land in the same cart.
CREATE UNIQUE INDEX carts_one_open_per_user_idx ON carts (user_id) WHERE status = 'open';

-- Used by the job that expires carts.
CREATE INDEX carts_open_expires_idx ON carts (expires_at) WHERE status = 'open';

CREATE TABLE cart_items (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id      UUID          NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    product_id   UUID          NOT NULL,
    product_name TEXT          NOT NULL,
    unit_price   NUMERIC(10,2) NOT NULL CHECK (unit_price >= 0),
    quantity     INTEGER       NOT NULL CHECK (quantity > 0),
    UNIQUE (cart_id, product_id)
);
