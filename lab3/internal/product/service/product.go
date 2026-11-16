package service

import (
	"context"

	"lab3/internal/product/domain"

	"github.com/google/uuid"
)

// ProductService is the query side: it only reads.
type ProductService struct {
	products domain.ProductRepository
}

func NewProductService(products domain.ProductRepository) *ProductService {
	return &ProductService{products: products}
}

func (s *ProductService) ListProducts(ctx context.Context) ([]domain.Product, error) {
	return s.products.List(ctx)
}

type GetProductQuery struct {
	ID uuid.UUID
}

func (s *ProductService) GetProduct(ctx context.Context, q GetProductQuery) (domain.Product, error) {
	return s.products.Get(ctx, q.ID)
}
