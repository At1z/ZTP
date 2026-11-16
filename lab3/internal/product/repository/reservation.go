package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

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

func (r *ReservationRepository) Confirm(ctx context.Context, cartID uuid.UUID, items []domain.ConfirmItem) error {
	// Always lock and process the products in id order (see the strategy above).
	sorted := append([]domain.ConfirmItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i].ProductID[:], sorted[j].ProductID[:]) < 0
	})

	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// Idempotency: a cart is confirmed once. A concurrent confirm of the
		// same cart waits here for the first one to commit and then sees the row.
		tag, err := tx.Exec(ctx, `INSERT INTO confirmed_carts (cart_id) VALUES ($1) ON CONFLICT DO NOTHING`, cartID)
		if err != nil {
			return fmt.Errorf("mark cart confirmed: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}

		for _, it := range sorted {
			if _, err := tx.Exec(ctx, `SELECT 1 FROM products WHERE id = $1 FOR UPDATE`, it.ProductID); err != nil {
				return fmt.Errorf("lock product: %w", err)
			}
		}

		for _, it := range sorted {
			var (
				quantity int
				expired  bool
			)
			err := tx.QueryRow(ctx, `
				SELECT quantity, expires_at <= now() FROM reservations
				WHERE cart_id = $1 AND product_id = $2 FOR UPDATE`, cartID, it.ProductID).Scan(&quantity, &expired)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrReservationNotFound
			}
			if err != nil {
				return fmt.Errorf("load reservation: %w", err)
			}
			if expired {
				return domain.ErrReservationExpired
			}
			if quantity != it.Quantity {
				return domain.ErrReservationMismatch
			}

			tag, err := tx.Exec(ctx,
				`UPDATE products SET stock = stock - $2 WHERE id = $1 AND stock >= $2`, it.ProductID, it.Quantity)
			if err != nil {
				return fmt.Errorf("decrement stock: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return domain.ErrInsufficientStock
			}
			if _, err := tx.Exec(ctx,
				`DELETE FROM reservations WHERE cart_id = $1 AND product_id = $2`, cartID, it.ProductID); err != nil {
				return fmt.Errorf("delete reservation: %w", err)
			}
		}
		return nil
	})
}
