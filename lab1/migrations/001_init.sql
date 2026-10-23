CREATE TABLE products (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(20)    NOT NULL UNIQUE,
    category   VARCHAR(20)    NOT NULL,
    price      NUMERIC(12, 2) NOT NULL,
    quantity   INTEGER        NOT NULL CHECK (quantity >= 0),
    created_at TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE TABLE product_history (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT       NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    field      VARCHAR(50)  NOT NULL,
    old_value  TEXT,
    new_value  TEXT,
    changed_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_product_history_product_id ON product_history (product_id);

CREATE TABLE banned_phrases (
    id     BIGSERIAL PRIMARY KEY,
    phrase VARCHAR(50) NOT NULL UNIQUE
);
