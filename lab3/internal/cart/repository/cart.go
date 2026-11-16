package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"lab3/internal/cart/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A cart is live while status = 'open' AND expires_at > now(). Expiry is
// decided by the database clock, so correctness does not depend on the job
// that deletes expired carts.

type CartRepository struct {
	pool *pgxpool.Pool
}

func NewCartRepository(pool *pgxpool.Pool) *CartRepository {
	return &CartRepository{pool: pool}
}

func (r *CartRepository) OpenCart(ctx context.Context, userID uuid.UUID, ttl time.Duration) (domain.Cart, bool, error) {
	cart := domain.Cart{UserID: userID, Status: domain.StatusOpen}

	// The partial unique index allows one open cart per user, so concurrent
	// requests from several devices end up with the same cart.
	err := r.pool.QueryRow(ctx, `
		INSERT INTO carts (user_id, expires_at) VALUES ($1, now() + make_interval(secs => $2))
		ON CONFLICT (user_id) WHERE status = 'open' DO NOTHING
		RETURNING id, created_at, expires_at`, userID, ttl.Seconds(),
	).Scan(&cart.ID, &cart.CreatedAt, &cart.ExpiresAt)
	if err == nil {
		return cart, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Cart{}, false, fmt.Errorf("insert cart: %w", err)
	}

	existing, err := r.GetOpenCart(ctx, userID)
	return existing, false, err
}

func (r *CartRepository) GetOpenCart(ctx context.Context, userID uuid.UUID) (domain.Cart, error) {
	// One statement, so the cart and its items come from the same snapshot.
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.status, c.created_at, c.expires_at,
		       i.product_id, i.product_name, round(i.unit_price * 100)::bigint, i.quantity
		FROM carts c
		LEFT JOIN cart_items i ON i.cart_id = c.id
		WHERE c.user_id = $1 AND c.status = 'open' AND c.expires_at > now()
		ORDER BY i.product_name, i.product_id`, userID)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("get cart: %w", err)
	}
	defer rows.Close()

	cart := domain.Cart{UserID: userID, Items: []domain.Item{}}
	found := false
	for rows.Next() {
		var (
			productID *uuid.UUID
			name      *string
			price     *int64
			quantity  *int
		)
		if err := rows.Scan(&cart.ID, &cart.Status, &cart.CreatedAt, &cart.ExpiresAt,
			&productID, &name, &price, &quantity); err != nil {
			return domain.Cart{}, fmt.Errorf("scan cart: %w", err)
		}
		found = true
		if productID != nil {
			cart.Items = append(cart.Items, domain.Item{
				ProductID:      *productID,
				ProductName:    *name,
				UnitPriceCents: *price,
				Quantity:       *quantity,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Cart{}, fmt.Errorf("read cart: %w", err)
	}
	if !found {
		return domain.Cart{}, domain.ErrCartNotFound
	}
	return cart, nil
}

func (r *CartRepository) Mutate(ctx context.Context, userID uuid.UUID, fn func(ctx context.Context, tx domain.CartTx) error) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		cart := domain.Cart{UserID: userID, Status: domain.StatusOpen}

		// Exclusive row lock: concurrent Mutate calls for this cart wait here
		// until the transaction ends.
		err := tx.QueryRow(ctx, `
			SELECT id, created_at, expires_at FROM carts
			WHERE user_id = $1 AND status = 'open' AND expires_at > now()
			FOR UPDATE`, userID).Scan(&cart.ID, &cart.CreatedAt, &cart.ExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrCartNotFound
		}
		if err != nil {
			return fmt.Errorf("lock cart: %w", err)
		}

		return fn(ctx, &cartTx{tx: tx, cart: cart})
	})
}

func (r *CartRepository) DeleteExpired(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx,
		`DELETE FROM carts WHERE status = 'open' AND expires_at <= now() RETURNING id`)
	if err != nil {
		return nil, fmt.Errorf("delete expired carts: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("scan expired carts: %w", err)
	}
	return ids, nil
}

type cartTx struct {
	tx   pgx.Tx
	cart domain.Cart
}

func (t *cartTx) Cart() domain.Cart { return t.cart }

func (t *cartTx) Items(ctx context.Context) ([]domain.Item, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT product_id, product_name, round(unit_price * 100)::bigint, quantity
		FROM cart_items WHERE cart_id = $1 ORDER BY product_name, product_id`, t.cart.ID)
	if err != nil {
		return nil, fmt.Errorf("load cart items: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[domain.Item])
	if err != nil {
		return nil, fmt.Errorf("scan cart items: %w", err)
	}
	return items, nil
}

func (t *cartTx) MarkCheckedOut(ctx context.Context) error {
	_, err := t.tx.Exec(ctx, `UPDATE carts SET status = 'checked_out' WHERE id = $1`, t.cart.ID)
	if err != nil {
		return fmt.Errorf("close cart: %w", err)
	}
	return nil
}

func (t *cartTx) UpsertItem(ctx context.Context, item domain.Item) error {
	// One atomic statement instead of read-modify-write. The price of an
	// existing line is kept.
	_, err := t.tx.Exec(ctx, `
		INSERT INTO cart_items (cart_id, product_id, product_name, unit_price, quantity)
		VALUES ($1, $2, $3, $4::bigint::numeric / 100, $5)
		ON CONFLICT (cart_id, product_id) DO UPDATE
		SET quantity = cart_items.quantity + EXCLUDED.quantity`,
		t.cart.ID, item.ProductID, item.ProductName, item.UnitPriceCents, item.Quantity)
	if err != nil {
		return fmt.Errorf("upsert cart item: %w", err)
	}
	return nil
}

func (t *cartTx) DeleteItem(ctx context.Context, productID uuid.UUID) (bool, error) {
	tag, err := t.tx.Exec(ctx,
		`DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2`, t.cart.ID, productID)
	if err != nil {
		return false, fmt.Errorf("delete cart item: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
