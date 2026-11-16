-- A cart is confirmed at most once. Confirming it again (a retried checkout)
-- finds its row here and does nothing, so the stock is never decremented twice.
CREATE TABLE confirmed_carts (
    cart_id      UUID        PRIMARY KEY,
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
