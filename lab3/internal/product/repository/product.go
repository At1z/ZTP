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

// available = stock minus active reservations; expired ones are ignored by
// the expires_at > now() check, so no cleanup job is needed for correctness.
const selectProduct = `
SELECT p.id, p.name, p.description, round(p.price * 100)::bigint, p.stock,
       p.stock - COALESCE((SELECT SUM(r.quantity) FROM reservations r
                           WHERE r.product_id = p.id AND r.expires_at > now()), 0)::int
FROM products p`

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

func scanProduct(row pgx.Row) (domain.Product, error) {
	var p domain.Product
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.Stock, &p.Available)
	return p, err
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, selectProduct+` ORDER BY p.created_at, p.name`)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	products := []domain.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (r *ProductRepository) Get(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	p, err := scanProduct(r.pool.QueryRow(ctx, selectProduct+` WHERE p.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Product{}, fmt.Errorf("get product: %w", err)
	}
	return p, nil
}
