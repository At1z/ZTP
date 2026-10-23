package service

import (
	"context"
	"errors"

	"lab1/internal/domain"
)

type ProductService struct {
	products ProductRepository
	phrases  BannedPhraseRepository
}

func NewProductService(products ProductRepository, phrases BannedPhraseRepository) *ProductService {
	return &ProductService{products: products, phrases: phrases}
}

func (s *ProductService) Create(ctx context.Context, in domain.ProductInput) (domain.Product, error) {
	if err := s.validate(ctx, in); err != nil {
		return domain.Product{}, err
	}
	p := productFromInput(0, in)
	if err := s.products.Create(ctx, &p); err != nil {
		return domain.Product{}, mapNameConflict(err)
	}
	return p, nil
}

func (s *ProductService) Get(ctx context.Context, id int64) (domain.Product, error) {
	return s.products.GetByID(ctx, id)
}

func (s *ProductService) List(ctx context.Context) ([]domain.Product, error) {
	return s.products.List(ctx)
}

func (s *ProductService) Update(ctx context.Context, id int64, in domain.ProductInput) (domain.Product, error) {
	if _, err := s.products.GetByID(ctx, id); err != nil {
		return domain.Product{}, err
	}
	if err := s.validate(ctx, in); err != nil {
		return domain.Product{}, err
	}
	p := productFromInput(id, in)
	if err := s.products.Update(ctx, &p); err != nil {
		return domain.Product{}, mapNameConflict(err)
	}
	return p, nil
}

func (s *ProductService) Delete(ctx context.Context, id int64) error {
	return s.products.Delete(ctx, id)
}

func (s *ProductService) History(ctx context.Context, id int64) ([]domain.HistoryEntry, error) {
	if _, err := s.products.GetByID(ctx, id); err != nil {
		return nil, err
	}
	return s.products.History(ctx, id)
}

// validate applies all domain rules and reports every invalid field at once.
func (s *ProductService) validate(ctx context.Context, in domain.ProductInput) error {
	var errs []domain.FieldError

	if msg := nameFormatError(in.Name); msg != "" {
		errs = append(errs, domain.FieldError{Field: "name", Message: msg})
	} else {
		phrases, err := s.phrases.List(ctx)
		if err != nil {
			return err
		}
		if phrase, found := findBannedPhrase(in.Name, phrases); found {
			errs = append(errs, domain.FieldError{Field: "name", Message: "contains forbidden phrase: " + phrase})
		}
	}
	errs = append(errs, categoryAndPriceErrors(in)...)
	errs = append(errs, quantityErrors(in.Quantity)...)

	if len(errs) > 0 {
		return &domain.ValidationError{Errors: errs}
	}
	return nil
}

func productFromInput(id int64, in domain.ProductInput) domain.Product {
	// validate has already guaranteed that Price and Quantity are set.
	return domain.Product{ID: id, Name: in.Name, Category: in.Category, Price: *in.Price, Quantity: *in.Quantity}
}

func mapNameConflict(err error) error {
	if errors.Is(err, domain.ErrConflict) {
		return &domain.ConflictError{FieldError: domain.FieldError{
			Field: "name", Message: "product with this name already exists",
		}}
	}
	return err
}
