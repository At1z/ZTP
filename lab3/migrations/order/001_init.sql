-- cart_id and product_id are plain references to other domains.
CREATE TABLE orders (
    id         UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID          NOT NULL,
    cart_id    UUID          NOT NULL UNIQUE,
    total      NUMERIC(12,2) NOT NULL CHECK (total >= 0),
    status     TEXT          NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'completed', 'cancelled')),
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_idx ON orders (user_id);

CREATE TABLE order_items (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID          NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id   UUID          NOT NULL,
    product_name TEXT          NOT NULL,
    unit_price   NUMERIC(10,2) NOT NULL CHECK (unit_price >= 0),
    quantity     INTEGER       NOT NULL CHECK (quantity > 0)
);

CREATE INDEX order_items_order_idx ON order_items (order_id);

CREATE TABLE notifications (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    order_id   UUID        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    message    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
