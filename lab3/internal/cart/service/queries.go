package service

import (
	"context"

	"lab3/internal/cart/domain"

	"github.com/google/uuid"
)

// CartQueryService is the query side: it only reads.
type CartQueryService struct {
	carts domain.CartRepository
}

func NewCartQueryService(carts domain.CartRepository) *CartQueryService {
	return &CartQueryService{carts: carts}
}

type GetCartQuery struct {
	UserID uuid.UUID
}

func (s *CartQueryService) GetCart(ctx context.Context, q GetCartQuery) (domain.Cart, error) {
	return s.carts.GetOpenCart(ctx, q.UserID)
}
