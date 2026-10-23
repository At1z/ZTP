package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lab1/internal/domain"
)

const productColumns = "id, name, category, price, quantity, created_at, updated_at"

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(row scanner) (domain.Product, error) {
	var p domain.Product
	err := row.Scan(&p.ID, &p.Name, &p.Category, &p.Price, &p.Quantity, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	return p, err
}

func (r *ProductRepository) Create(ctx context.Context, p *domain.Product) error {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO products (name, category, price, quantity)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, updated_at`,
		p.Name, p.Category, p.Price, p.Quantity,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *ProductRepository) GetByID(ctx context.Context, id int64) (domain.Product, error) {
	return scanProduct(r.pool.QueryRow(ctx,
		"SELECT "+productColumns+" FROM products WHERE id = $1", id))
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+productColumns+" FROM products ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []domain.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

// Update replaces the product and records every changed field in product_history,
// all in one transaction.
func (r *ProductRepository) Update(ctx context.Context, p *domain.Product) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	old, err := scanProduct(tx.QueryRow(ctx,
		"SELECT "+productColumns+" FROM products WHERE id = $1 FOR UPDATE", p.ID))
	if err != nil {
		return err
	}

	err = tx.QueryRow(ctx,
		`UPDATE products
		 SET name = $2, category = $3, price = $4, quantity = $5, updated_at = now()
		 WHERE id = $1
		 RETURNING created_at, updated_at`,
		p.ID, p.Name, p.Category, p.Price, p.Quantity,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}

	for _, c := range diffProducts(old, *p) {
		_, err := tx.Exec(ctx,
			`INSERT INTO product_history (product_id, field, old_value, new_value)
			 VALUES ($1, $2, $3, $4)`,
			p.ID, c.field, c.oldValue, c.newValue)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *ProductRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM products WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ProductRepository) History(ctx context.Context, id int64) ([]domain.HistoryEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, product_id, field, old_value, new_value, changed_at
		 FROM product_history WHERE product_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []domain.HistoryEntry{}
	for rows.Next() {
		var e domain.HistoryEntry
		if err := rows.Scan(&e.ID, &e.ProductID, &e.Field, &e.OldValue, &e.NewValue, &e.ChangedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
