package unit

import (
	"context"
	"errors"
	"time"

	cartdomain "lab3/internal/cart/domain"

	"github.com/google/uuid"
)

// fakeCartRepo keeps carts in memory and mimics Mutate: when fn fails, the
// changes made inside are discarded.
type fakeCartRepo struct {
	carts   map[uuid.UUID]*cartdomain.Cart // by user id
	expired []uuid.UUID                    // returned (once) by DeleteExpired
}

func newFakeCartRepo() *fakeCartRepo {
	return &fakeCartRepo{carts: map[uuid.UUID]*cartdomain.Cart{}}
}

func (r *fakeCartRepo) OpenCart(_ context.Context, userID uuid.UUID, ttl time.Duration) (cartdomain.Cart, bool, error) {
	if c, ok := r.carts[userID]; ok {
		return *c, false, nil
	}
	c := &cartdomain.Cart{
		ID:        uuid.New(),
		UserID:    userID,
		Status:    cartdomain.StatusOpen,
		ExpiresAt: time.Now().Add(ttl),
	}
	r.carts[userID] = c
	return *c, true, nil
}

func (r *fakeCartRepo) GetOpenCart(_ context.Context, userID uuid.UUID) (cartdomain.Cart, error) {
	c, ok := r.carts[userID]
	if !ok {
		return cartdomain.Cart{}, cartdomain.ErrCartNotFound
	}
	out := *c
	out.Items = append([]cartdomain.Item{}, c.Items...)
	return out, nil
}

func (r *fakeCartRepo) Mutate(ctx context.Context, userID uuid.UUID, fn func(context.Context, cartdomain.CartTx) error) error {
	c, ok := r.carts[userID]
	if !ok {
		return cartdomain.ErrCartNotFound
	}
	snapshot := append([]cartdomain.Item{}, c.Items...)
	if err := fn(ctx, &fakeCartTx{cart: c}); err != nil {
		c.Items = snapshot
		return err
	}
	return nil
}

func (r *fakeCartRepo) DeleteExpired(context.Context) ([]uuid.UUID, error) {
	ids := r.expired
	r.expired = nil
	return ids, nil
}

type fakeCartTx struct{ cart *cartdomain.Cart }

func (t *fakeCartTx) Cart() cartdomain.Cart {
	c := *t.cart
	c.Items = nil
	return c
}

func (t *fakeCartTx) UpsertItem(_ context.Context, item cartdomain.Item) error {
	for i := range t.cart.Items {
		if t.cart.Items[i].ProductID == item.ProductID {
			t.cart.Items[i].Quantity += item.Quantity
			return nil
		}
	}
	t.cart.Items = append(t.cart.Items, item)
	return nil
}

func (t *fakeCartTx) DeleteItem(_ context.Context, productID uuid.UUID) (bool, error) {
	for i := range t.cart.Items {
		if t.cart.Items[i].ProductID == productID {
			t.cart.Items = append(t.cart.Items[:i], t.cart.Items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// fakeProductClient plays the product service: a catalogue plus stock.
type fakeProductClient struct {
	products    map[uuid.UUID]cartdomain.Product
	available   map[uuid.UUID]int
	reserved    map[uuid.UUID]int // by product, quantity reserved so far
	released    []uuid.UUID       // cart ids passed to Release/ReleaseCart
	releaseErr  error
	reserveCall []cartdomain.ReserveRequest
}

func newFakeProductClient() *fakeProductClient {
	return &fakeProductClient{
		products:  map[uuid.UUID]cartdomain.Product{},
		available: map[uuid.UUID]int{},
		reserved:  map[uuid.UUID]int{},
	}
}

func (p *fakeProductClient) addProduct(name string, priceCents int64, stock int) uuid.UUID {
	id := uuid.New()
	p.products[id] = cartdomain.Product{ID: id, Name: name, PriceCents: priceCents}
	p.available[id] = stock
	return id
}

func (p *fakeProductClient) GetProduct(_ context.Context, id uuid.UUID) (cartdomain.Product, error) {
	product, ok := p.products[id]
	if !ok {
		return cartdomain.Product{}, cartdomain.ErrProductNotFound
	}
	return product, nil
}

func (p *fakeProductClient) Reserve(_ context.Context, req cartdomain.ReserveRequest) error {
	p.reserveCall = append(p.reserveCall, req)
	if p.available[req.ProductID] < req.Quantity {
		return cartdomain.ErrInsufficientStock
	}
	p.available[req.ProductID] -= req.Quantity
	p.reserved[req.ProductID] += req.Quantity
	return nil
}

func (p *fakeProductClient) Release(_ context.Context, cartID, _ uuid.UUID) error {
	if p.releaseErr != nil {
		return p.releaseErr
	}
	p.released = append(p.released, cartID)
	return nil
}

func (p *fakeProductClient) ReleaseCart(_ context.Context, cartID uuid.UUID) error {
	if p.releaseErr != nil {
		return p.releaseErr
	}
	p.released = append(p.released, cartID)
	return nil
}

var errBoom = errors.New("boom")
