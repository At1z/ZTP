package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"lab1/internal/domain"
)

type BannedPhraseRepository struct {
	pool *pgxpool.Pool
}

func NewBannedPhraseRepository(pool *pgxpool.Pool) *BannedPhraseRepository {
	return &BannedPhraseRepository{pool: pool}
}

func (r *BannedPhraseRepository) List(ctx context.Context) ([]domain.BannedPhrase, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, phrase FROM banned_phrases ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	phrases := []domain.BannedPhrase{}
	for rows.Next() {
		var p domain.BannedPhrase
		if err := rows.Scan(&p.ID, &p.Phrase); err != nil {
			return nil, err
		}
		phrases = append(phrases, p)
	}
	return phrases, rows.Err()
}

func (r *BannedPhraseRepository) Create(ctx context.Context, p *domain.BannedPhrase) error {
	err := r.pool.QueryRow(ctx,
		"INSERT INTO banned_phrases (phrase) VALUES ($1) RETURNING id", p.Phrase,
	).Scan(&p.ID)
	if isUniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *BannedPhraseRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM banned_phrases WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
