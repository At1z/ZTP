CREATE TABLE products (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT          NOT NULL CHECK (name <> ''),
    description TEXT          NOT NULL DEFAULT '',
    price       NUMERIC(10,2) NOT NULL CHECK (price >= 0),
    stock       INTEGER       NOT NULL CHECK (stock >= 0),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- stock is the physical inventory. A reservation is active while
-- expires_at > now(), so correctness never depends on a cleanup job.
-- cart_id and user_id point into the cart service, hence no foreign keys.
CREATE TABLE reservations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID        NOT NULL REFERENCES products (id),
    cart_id    UUID        NOT NULL,
    user_id    UUID        NOT NULL,
    quantity   INTEGER     NOT NULL CHECK (quantity > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cart_id, product_id)
);

-- Used when summing the active reservations of one product.
CREATE INDEX reservations_product_expires_idx ON reservations (product_id, expires_at);

INSERT INTO products (name, description, price, stock) VALUES
    ('Bilet koncertowy', 'Ostatnia sztuka, idealna do testu blokady.', 199.99, 1),
    ('Koszulka', 'Bawełniana koszulka z logo.', 59.90, 25),
    ('Kubek', 'Ceramiczny kubek 300 ml.', 24.50, 10);
