package unit

import (
	"context"
	"errors"
	"testing"
	"time"

	cartdomain "lab3/internal/cart/domain"
	cartservice "lab3/internal/cart/service"

	"github.com/google/uuid"
)

type cartFixture struct {
	repo     *fakeCartRepo
	products *fakeProductClient
	commands *cartservice.CartCommandService
	queries  *cartservice.CartQueryService
}

func newCartFixture() *cartFixture {
	repo := newFakeCartRepo()
	products := newFakeProductClient()
	return &cartFixture{
		repo:     repo,
		products: products,
		commands: cartservice.NewCartCommandService(repo, products, 15*time.Minute),
		queries:  cartservice.NewCartQueryService(repo),
	}
}

// userWithCart returns a user who already has an open cart.
func (f *cartFixture) userWithCart(t *testing.T) uuid.UUID {
	t.Helper()
	user := uuid.New()
	if _, _, err := f.commands.OpenCart(context.Background(), cartservice.OpenCartCommand{UserID: user}); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestOpenCartIsIdempotent(t *testing.T) {
	f := newCartFixture()
	user := uuid.New()

	first, created, err := f.commands.OpenCart(context.Background(), cartservice.OpenCartCommand{UserID: user})
	if err != nil || !created {
		t.Fatalf("first: created = %v, err = %v", created, err)
	}
	second, created, err := f.commands.OpenCart(context.Background(), cartservice.OpenCartCommand{UserID: user})
	if err != nil || created {
		t.Fatalf("second: created = %v, err = %v", created, err)
	}
	if first.ID != second.ID {
		t.Fatal("a user must keep one open cart")
	}
}

func TestOpenCartRequiresUser(t *testing.T) {
	f := newCartFixture()

	_, _, err := f.commands.OpenCart(context.Background(), cartservice.OpenCartCommand{})

	if !errors.Is(err, cartdomain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestOpenCartReleasesReservationsOfExpiredCarts(t *testing.T) {
	f := newCartFixture()
	staleCart := uuid.New()
	f.repo.expired = []uuid.UUID{staleCart}

	if _, _, err := f.commands.OpenCart(context.Background(), cartservice.OpenCartCommand{UserID: uuid.New()}); err != nil {
		t.Fatal(err)
	}

	if len(f.products.released) != 1 || f.products.released[0] != staleCart {
		t.Fatalf("released = %v, want the expired cart", f.products.released)
	}
}

func TestAddItemReservesStockAndStoresPriceSnapshot(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)
	product := f.products.addProduct("Kubek", 2450, 10)

	cart, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: user, ProductID: product, Quantity: 2})

	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 1 || cart.Items[0].ProductName != "Kubek" || cart.Items[0].UnitPriceCents != 2450 || cart.Items[0].Quantity != 2 {
		t.Fatalf("unexpected items: %+v", cart.Items)
	}
	if got := f.products.reserved[product]; got != 2 {
		t.Fatalf("reserved = %d, want 2", got)
	}
	if call := f.products.reserveCall[0]; call.CartID != cart.ID || call.UserID != user {
		t.Fatalf("reservation was not made for this cart and user: %+v", call)
	}
}

func TestAddItemTwiceSumsQuantities(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)
	product := f.products.addProduct("Koszulka", 5990, 10)
	cmd := cartservice.AddItemCommand{UserID: user, ProductID: product, Quantity: 1}

	if _, err := f.commands.AddItem(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	cart, err := f.commands.AddItem(context.Background(), cmd)

	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("items = %+v, want one line with quantity 2", cart.Items)
	}
}

func TestAddItemRejectsInvalidInput(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)
	product := f.products.addProduct("Kubek", 100, 1)

	tests := map[string]cartservice.AddItemCommand{
		"no user":       {ProductID: product, Quantity: 1},
		"no product":    {UserID: user, Quantity: 1},
		"zero quantity": {UserID: user, ProductID: product},
		"negative":      {UserID: user, ProductID: product, Quantity: -3},
	}
	for name, cmd := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := f.commands.AddItem(context.Background(), cmd)
			if !errors.Is(err, cartdomain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
	if len(f.products.reserveCall) != 0 {
		t.Fatal("invalid input must not reach the product service")
	}
}

func TestAddItemUnknownProduct(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)

	_, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: user, ProductID: uuid.New(), Quantity: 1})

	if !errors.Is(err, cartdomain.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

func TestAddItemWithoutCart(t *testing.T) {
	f := newCartFixture()
	product := f.products.addProduct("Kubek", 100, 1)

	_, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: uuid.New(), ProductID: product, Quantity: 1})

	if !errors.Is(err, cartdomain.ErrCartNotFound) {
		t.Fatalf("err = %v, want ErrCartNotFound", err)
	}
}

// Second user wants the last piece: the product service says no and the cart
// stays empty.
func TestAddItemWithoutStockLeavesCartUntouched(t *testing.T) {
	f := newCartFixture()
	owner, other := f.userWithCart(t), f.userWithCart(t)
	ticket := f.products.addProduct("Bilet", 19999, 1)

	if _, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: owner, ProductID: ticket, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	_, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: other, ProductID: ticket, Quantity: 1})

	if !errors.Is(err, cartdomain.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
	cart, _ := f.queries.GetCart(context.Background(), cartservice.GetCartQuery{UserID: other})
	if len(cart.Items) != 0 {
		t.Fatalf("items = %+v, want none", cart.Items)
	}
}

func TestRemoveItemReleasesReservation(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)
	product := f.products.addProduct("Kubek", 100, 5)
	added, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: user, ProductID: product, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}

	cart, err := f.commands.RemoveItem(context.Background(), cartservice.RemoveItemCommand{UserID: user, ProductID: product})

	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 0 {
		t.Fatalf("items = %+v, want none", cart.Items)
	}
	if len(f.products.released) != 1 || f.products.released[0] != added.ID {
		t.Fatalf("released = %v, want the cart's reservation", f.products.released)
	}
}

func TestRemoveItemKeepsItemWhenReleaseFails(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)
	product := f.products.addProduct("Kubek", 100, 5)
	if _, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: user, ProductID: product, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	f.products.releaseErr = errBoom

	_, err := f.commands.RemoveItem(context.Background(), cartservice.RemoveItemCommand{UserID: user, ProductID: product})

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the release error", err)
	}
	cart, _ := f.queries.GetCart(context.Background(), cartservice.GetCartQuery{UserID: user})
	if len(cart.Items) != 1 {
		t.Fatalf("items = %+v, the line must survive a failed release", cart.Items)
	}
}

func TestRemoveUnknownItem(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)

	_, err := f.commands.RemoveItem(context.Background(), cartservice.RemoveItemCommand{UserID: user, ProductID: uuid.New()})

	if !errors.Is(err, cartdomain.ErrItemNotFound) {
		t.Fatalf("err = %v, want ErrItemNotFound", err)
	}
}

func TestGetCartTotalsTheValue(t *testing.T) {
	f := newCartFixture()
	user := f.userWithCart(t)
	mug := f.products.addProduct("Kubek", 2450, 10)
	shirt := f.products.addProduct("Koszulka", 5990, 10)
	for _, cmd := range []cartservice.AddItemCommand{
		{UserID: user, ProductID: mug, Quantity: 2},
		{UserID: user, ProductID: shirt, Quantity: 1},
	} {
		if _, err := f.commands.AddItem(context.Background(), cmd); err != nil {
			t.Fatal(err)
		}
	}

	cart, err := f.queries.GetCart(context.Background(), cartservice.GetCartQuery{UserID: user})

	if err != nil {
		t.Fatal(err)
	}
	if want := int64(2*2450 + 5990); cart.TotalCents() != want {
		t.Fatalf("total = %d, want %d", cart.TotalCents(), want)
	}
}

func TestUsersDoNotSeeEachOthersCarts(t *testing.T) {
	f := newCartFixture()
	alice, bob := f.userWithCart(t), f.userWithCart(t)
	product := f.products.addProduct("Kubek", 100, 5)
	if _, err := f.commands.AddItem(context.Background(), cartservice.AddItemCommand{UserID: alice, ProductID: product, Quantity: 1}); err != nil {
		t.Fatal(err)
	}

	cart, err := f.queries.GetCart(context.Background(), cartservice.GetCartQuery{UserID: bob})

	if err != nil || len(cart.Items) != 0 {
		t.Fatalf("bob sees %+v (err %v), want an empty cart", cart.Items, err)
	}
	if _, err := f.commands.RemoveItem(context.Background(), cartservice.RemoveItemCommand{UserID: bob, ProductID: product}); !errors.Is(err, cartdomain.ErrItemNotFound) {
		t.Fatalf("bob removing alice's item: err = %v, want ErrItemNotFound", err)
	}
}

func TestExpireCartsReleasesEachCart(t *testing.T) {
	f := newCartFixture()
	a, b := uuid.New(), uuid.New()
	f.repo.expired = []uuid.UUID{a, b}

	n, err := f.commands.ExpireCarts(context.Background())

	if err != nil || n != 2 || len(f.products.released) != 2 {
		t.Fatalf("n = %d, released = %v, err = %v", n, f.products.released, err)
	}
}

func TestExpireCartsContinuesWhenReleaseFails(t *testing.T) {
	f := newCartFixture()
	f.repo.expired = []uuid.UUID{uuid.New(), uuid.New()}
	f.products.releaseErr = errBoom

	n, err := f.commands.ExpireCarts(context.Background())

	if err != nil || n != 2 {
		t.Fatalf("n = %d, err = %v; release failures must not abort the sweep", n, err)
	}
}
