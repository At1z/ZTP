package unit

import (
	"context"
	"errors"
	"testing"

	"lab1/internal/domain"
	"lab1/internal/service"
)

// fakeProducts is an in-memory service.ProductRepository.
type fakeProducts struct {
	items  map[int64]domain.Product
	nextID int64
}

func newFakeProducts() *fakeProducts {
	return &fakeProducts{items: map[int64]domain.Product{}, nextID: 1}
}

func (f *fakeProducts) nameTaken(name string, exceptID int64) bool {
	for id, p := range f.items {
		if p.Name == name && id != exceptID {
			return true
		}
	}
	return false
}

func (f *fakeProducts) Create(_ context.Context, p *domain.Product) error {
	if f.nameTaken(p.Name, 0) {
		return domain.ErrConflict
	}
	p.ID = f.nextID
	f.nextID++
	f.items[p.ID] = *p
	return nil
}

func (f *fakeProducts) GetByID(_ context.Context, id int64) (domain.Product, error) {
	p, ok := f.items[id]
	if !ok {
		return domain.Product{}, domain.ErrNotFound
	}
	return p, nil
}

func (f *fakeProducts) List(context.Context) ([]domain.Product, error) {
	products := []domain.Product{}
	for _, p := range f.items {
		products = append(products, p)
	}
	return products, nil
}

func (f *fakeProducts) Update(_ context.Context, p *domain.Product) error {
	if _, ok := f.items[p.ID]; !ok {
		return domain.ErrNotFound
	}
	if f.nameTaken(p.Name, p.ID) {
		return domain.ErrConflict
	}
	f.items[p.ID] = *p
	return nil
}

func (f *fakeProducts) Delete(_ context.Context, id int64) error {
	if _, ok := f.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.items, id)
	return nil
}

func (f *fakeProducts) History(context.Context, int64) ([]domain.HistoryEntry, error) {
	return []domain.HistoryEntry{}, nil
}

// fakePhrases is an in-memory service.BannedPhraseRepository.
type fakePhrases struct {
	items  []domain.BannedPhrase
	nextID int64
}

func newFakePhrases() *fakePhrases {
	return &fakePhrases{nextID: 1}
}

func (f *fakePhrases) List(context.Context) ([]domain.BannedPhrase, error) {
	return append([]domain.BannedPhrase{}, f.items...), nil
}

func (f *fakePhrases) Create(_ context.Context, p *domain.BannedPhrase) error {
	for _, existing := range f.items {
		if existing.Phrase == p.Phrase {
			return domain.ErrConflict
		}
	}
	p.ID = f.nextID
	f.nextID++
	f.items = append(f.items, *p)
	return nil
}

func (f *fakePhrases) Delete(_ context.Context, id int64) error {
	for i, p := range f.items {
		if p.ID == id {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

// --- helpers ---

type env struct {
	products *service.ProductService
	phrases  *service.BannedPhraseService
}

func newEnv() env {
	productRepo, phraseRepo := newFakeProducts(), newFakePhrases()
	return env{
		products: service.NewProductService(productRepo, phraseRepo),
		phrases:  service.NewBannedPhraseService(phraseRepo),
	}
}

func validInput() domain.ProductInput {
	return domain.ProductInput{Name: "Laptop1", Category: domain.CategoryElectronics, Price: ptr(3000.0), Quantity: ptr(5)}
}

func ptr[T any](v T) *T { return &v }

// invalidFields returns the fields of a *domain.ValidationError, failing the
// test when err is of another type.
func invalidFields(t *testing.T, err error) []string {
	t.Helper()
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want *domain.ValidationError", err)
	}
	fields := make([]string, len(ve.Errors))
	for i, e := range ve.Errors {
		fields[i] = e.Field
	}
	return fields
}

func expectInvalidField(t *testing.T, err error, field string) {
	t.Helper()
	for _, f := range invalidFields(t, err) {
		if f == field {
			return
		}
	}
	t.Fatalf("expected an error for field %q, got: %v", field, err)
}
