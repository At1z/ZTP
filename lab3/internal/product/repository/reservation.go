package repository

import (
	"context"
	"errors"
	"fmt"

	"lab3/internal/product/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Locking strategy
//
// Every operation that changes the inventory first locks the affected product
// rows with SELECT ... FOR UPDATE (several products in ascending id order) and
// only then touches reservations. A single, consistent order means two
// transactions can never wait for each other in a cycle. Concurrent requests
// for the same product are serialized by its row lock, so the available stock
// computed inside the transaction cannot change before the commit.
//
// Time comes from the database (now()) everywhere, so several service
// instances never disagree about whether a reservation has expired.
//
// Release only deletes rows. That can only increase availability, so it needs
// no product lock.

type ReservationRepository struct {
	pool *pgxpool.Pool
}

func NewReservationRepository(pool *pgxpool.Pool) *ReservationRepository {
	return &ReservationRepository{pool: pool}
}

func (r *ReservationRepository) Reserve(ctx context.Context, p domain.ReserveParams) (domain.Reservation, error) {
	res := domain.Reservation{
		ProductID: p.ProductID,
		CartID:    p.CartID,
		UserID:    p.UserID,
	}

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var stock int
		err := tx.QueryRow(ctx, `SELECT stock FROM products WHERE id = $1 FOR UPDATE`, p.ProductID).Scan(&stock)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock product: %w", err)
		}

		var reserved int
		err = tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(quantity), 0)::int FROM reservations
			WHERE product_id = $1 AND expires_at > now()`, p.ProductID).Scan(&reserved)
		if err != nil {
			return fmt.Errorf("sum reservations: %w", err)
		}

		if p.Quantity > stock-reserved {
			return domain.ErrInsufficientStock
		}

		// The reservation lasts TTL from its creation: adding more of the same
		// product to an active reservation keeps the original expiry. An expired
		// reservation of the same cart is replaced by a fresh one.
		err = tx.QueryRow(ctx, `
			INSERT INTO reservations (product_id, cart_id, user_id, quantity, expires_at)
			VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))
			ON CONFLICT (cart_id, product_id) DO UPDATE SET
				quantity   = CASE WHEN reservations.expires_at > now()
				                  THEN reservations.quantity + EXCLUDED.quantity
				                  ELSE EXCLUDED.quantity END,
				expires_at = CASE WHEN reservations.expires_at > now()
				                  THEN reservations.expires_at
				                  ELSE EXCLUDED.expires_at END
			RETURNING id, quantity, expires_at`,
			p.ProductID, p.CartID, p.UserID, p.Quantity, p.TTL.Seconds(),
		).Scan(&res.ID, &res.Quantity, &res.ExpiresAt)
		if err != nil {
			return fmt.Errorf("upsert reservation: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Reservation{}, err
	}
	return res, nil
}

func (r *ReservationRepository) Release(ctx context.Context, cartID, productID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM reservations WHERE cart_id = $1 AND product_id = $2`, cartID, productID)
	if err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}
	return nil
}

func (r *ReservationRepository) ReleaseCart(ctx context.Context, cartID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM reservations WHERE cart_id = $1`, cartID)
	if err != nil {
		return fmt.Errorf("release cart reservations: %w", err)
	}
	return nil
}

func (r *ReservationRepository) Confirm(ctx context.Context, cartID uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// Products first, in id order, then the cart's reservations. The cart
		// service serializes operations on one cart, so the set of reserved
		// products cannot change between the two statements.
		_, err := tx.Exec(ctx, `
			SELECT 1 FROM products
			WHERE id IN (SELECT product_id FROM reservations WHERE cart_id = $1)
			ORDER BY id FOR UPDATE`, cartID)
		if err != nil {
			return fmt.Errorf("lock products: %w", err)
		}

		type line struct {
			ProductID uuid.UUID
			Quantity  int
			Expired   bool
		}
		rows, err := tx.Query(ctx, `
			SELECT product_id, quantity, expires_at <= now() AS expired
			FROM reservations WHERE cart_id = $1 ORDER BY product_id FOR UPDATE`, cartID)
		if err != nil {
			return fmt.Errorf("load reservations: %w", err)
		}
		lines, err := pgx.CollectRows(rows, pgx.RowToStructByPos[line])
		if err != nil {
			return fmt.Errorf("scan reservations: %w", err)
		}

		if len(lines) == 0 {
			return domain.ErrReservationNotFound
		}
		for _, l := range lines {
			if l.Expired {
				return domain.ErrReservationExpired
			}
		}

		for _, l := range lines {
			tag, err := tx.Exec(ctx,
				`UPDATE products SET stock = stock - $2 WHERE id = $1 AND stock >= $2`, l.ProductID, l.Quantity)
			if err != nil {
				return fmt.Errorf("decrement stock: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return domain.ErrInsufficientStock
			}
		}

		if _, err := tx.Exec(ctx, `DELETE FROM reservations WHERE cart_id = $1`, cartID); err != nil {
			return fmt.Errorf("delete reservations: %w", err)
		}
		return nil
	})
}
