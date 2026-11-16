package unit

import (
	"context"
	"errors"
	"testing"
	"time"

	"lab3/internal/product/domain"
	"lab3/internal/product/service"

	"github.com/google/uuid"
)

type fakeReservations struct {
	got     domain.ReserveParams
	calls   int
	err     error
	release []uuid.UUID
}

func (f *fakeReservations) Reserve(_ context.Context, p domain.ReserveParams) (domain.Reservation, error) {
	f.calls++
	f.got = p
	return domain.Reservation{ProductID: p.ProductID, CartID: p.CartID, Quantity: p.Quantity}, f.err
}

func (f *fakeReservations) Release(_ context.Context, cartID, _ uuid.UUID) error {
	f.release = append(f.release, cartID)
	return f.err
}

func (f *fakeReservations) ReleaseCart(_ context.Context, cartID uuid.UUID) error {
	f.release = append(f.release, cartID)
	return f.err
}

func (f *fakeReservations) Confirm(context.Context, uuid.UUID, []domain.ConfirmItem) error {
	return f.err
}

type fakeProducts struct {
	products []domain.Product
}

func (f fakeProducts) List(context.Context) ([]domain.Product, error) { return f.products, nil }

func (f fakeProducts) Get(_ context.Context, id uuid.UUID) (domain.Product, error) {
	for _, p := range f.products {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Product{}, domain.ErrNotFound
}

func validCommand() service.ReserveProductCommand {
	return service.ReserveProductCommand{
		ProductID: uuid.New(),
		CartID:    uuid.New(),
		UserID:    uuid.New(),
		Quantity:  2,
	}
}

func TestReserveRejectsInvalidInput(t *testing.T) {
	tests := map[string]func(*service.ReserveProductCommand){
		"missing product": func(c *service.ReserveProductCommand) { c.ProductID = uuid.Nil },
		"missing cart":    func(c *service.ReserveProductCommand) { c.CartID = uuid.Nil },
		"missing user":    func(c *service.ReserveProductCommand) { c.UserID = uuid.Nil },
		"zero quantity":   func(c *service.ReserveProductCommand) { c.Quantity = 0 },
		"negative":        func(c *service.ReserveProductCommand) { c.Quantity = -1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			repo := &fakeReservations{}
			svc := service.NewReservationService(repo, time.Minute)

			cmd := validCommand()
			mutate(&cmd)
			_, err := svc.Reserve(context.Background(), cmd)

			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			if repo.calls != 0 {
				t.Fatal("repository must not be called for invalid input")
			}
		})
	}
}

func TestReservePassesConfiguredTTL(t *testing.T) {
	repo := &fakeReservations{}
	svc := service.NewReservationService(repo, 15*time.Minute)

	cmd := validCommand()
	if _, err := svc.Reserve(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}

	if repo.got.TTL != 15*time.Minute || repo.got.Quantity != cmd.Quantity || repo.got.CartID != cmd.CartID {
		t.Fatalf("unexpected params: %+v", repo.got)
	}
}

func TestReservePropagatesInsufficientStock(t *testing.T) {
	repo := &fakeReservations{err: domain.ErrInsufficientStock}
	svc := service.NewReservationService(repo, time.Minute)

	_, err := svc.Reserve(context.Background(), validCommand())

	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
}

func TestReleaseReachesRepository(t *testing.T) {
	repo := &fakeReservations{}
	svc := service.NewReservationService(repo, time.Minute)
	cartID := uuid.New()

	if err := svc.Release(context.Background(), service.ReleaseReservationCommand{CartID: cartID, ProductID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReleaseCart(context.Background(), service.ReleaseCartCommand{CartID: cartID}); err != nil {
		t.Fatal(err)
	}
	if len(repo.release) != 2 {
		t.Fatalf("release calls = %d, want 2", len(repo.release))
	}
}

func TestProductQueries(t *testing.T) {
	known := domain.Product{ID: uuid.New(), Name: "Kubek", Available: 3}
	svc := service.NewProductService(fakeProducts{products: []domain.Product{known}})

	list, err := svc.ListProducts(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, err = %v", list, err)
	}

	got, err := svc.GetProduct(context.Background(), service.GetProductQuery{ID: known.ID})
	if err != nil || got.Name != "Kubek" {
		t.Fatalf("get = %v, err = %v", got, err)
	}

	_, err = svc.GetProduct(context.Background(), service.GetProductQuery{ID: uuid.New()})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
