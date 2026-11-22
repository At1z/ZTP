package repository

import (
	"context"
	"errors"
	"fmt"

	"lab3/internal/order/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const selectOrder = `
SELECT id, user_id, cart_id, round(total * 100)::bigint, status, created_at FROM orders`

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) Create(ctx context.Context, order domain.Order, n domain.Notification) (domain.Order, bool, error) {
	created := false
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// cart_id is unique, so a retry (or two concurrent requests) for the
		// same cart inserts nothing and gets the existing order below.
		tag, err := tx.Exec(ctx, `
			INSERT INTO orders (id, user_id, cart_id, total, status)
			VALUES ($1, $2, $3, $4::bigint::numeric / 100, $5)
			ON CONFLICT (cart_id) DO NOTHING`,
			order.ID, order.UserID, order.CartID, order.TotalCents, order.Status)
		if err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		created = true

		for _, it := range order.Items {
			_, err := tx.Exec(ctx, `
				INSERT INTO order_items (order_id, product_id, product_name, unit_price, quantity)
				VALUES ($1, $2, $3, $4::bigint::numeric / 100, $5)`,
				order.ID, it.ProductID, it.ProductName, it.UnitPriceCents, it.Quantity)
			if err != nil {
				return fmt.Errorf("insert order item: %w", err)
			}
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO notifications (id, user_id, order_id, message) VALUES ($1, $2, $3, $4)`,
			n.ID, n.UserID, n.OrderID, n.Message)
		if err != nil {
			return fmt.Errorf("insert notification: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Order{}, false, err
	}

	stored, err := r.load(ctx, selectOrder+` WHERE cart_id = $1`, order.CartID)
	return stored, created, err
}

func (r *OrderRepository) Get(ctx context.Context, userID, orderID uuid.UUID) (domain.Order, error) {
	return r.load(ctx, selectOrder+` WHERE id = $1 AND user_id = $2`, orderID, userID)
}

func (r *OrderRepository) load(ctx context.Context, query string, args ...any) (domain.Order, error) {
	var o domain.Order
	err := r.pool.QueryRow(ctx, query, args...).Scan(&o.ID, &o.UserID, &o.CartID, &o.TotalCents, &o.Status, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("load order: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT product_id, product_name, round(unit_price * 100)::bigint, quantity
		FROM order_items WHERE order_id = $1 ORDER BY product_name, product_id`, o.ID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("load order items: %w", err)
	}
	o.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[domain.Item])
	if err != nil {
		return domain.Order{}, fmt.Errorf("scan order items: %w", err)
	}
	return o, nil
}
