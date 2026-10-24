package service

import (
	"context"

	"lab1/internal/domain"
)

type ProductRepository interface {
	Create(ctx context.Context, p *domain.Product) error
	GetByID(ctx context.Context, id int64) (domain.Product, error)
	List(ctx context.Context) ([]domain.Product, error)
	Update(ctx context.Context, p *domain.Product) error
	Delete(ctx context.Context, id int64) error
	History(ctx context.Context, id int64) ([]domain.HistoryEntry, error)
}

type BannedPhraseRepository interface {
	List(ctx context.Context) ([]domain.BannedPhrase, error)
	Create(ctx context.Context, p *domain.BannedPhrase) error
	Delete(ctx context.Context, id int64) error
}
