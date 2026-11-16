package unit

import (
	"context"
	"errors"
	"testing"

	orderdomain "lab3/internal/order/domain"
	orderservice "lab3/internal/order/service"

	"github.com/google/uuid"
)

// fakeOrderRepo stores orders and notifications in memory, idempotent per cart.
type fakeOrderRepo struct {
	orders        map[uuid.UUID]orderdomain.Order // by cart id
	notifications []orderdomain.Notification
}

func newFakeOrderRepo() *fakeOrderRepo {
	return &fakeOrderRepo{orders: map[uuid.UUID]orderdomain.Order{}}
}

func (r *fakeOrderRepo) Create(_ context.Context, o orderdomain.Order, n orderdomain.Notification) (orderdomain.Order, bool, error) {
	if existing, ok := r.orders[o.CartID]; ok {
		return existing, false, nil
	}
	r.orders[o.CartID] = o
	r.notifications = append(r.notifications, n)
	return o, true, nil
}

func (r *fakeOrderRepo) Get(_ context.Context, userID, orderID uuid.UUID) (orderdomain.Order, error) {
	for _, o := range r.orders {
		if o.ID == orderID && o.UserID == userID {
			return o, nil
		}
	}
	return orderdomain.Order{}, orderdomain.ErrOrderNotFound
}

func (r *fakeOrderRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]orderdomain.Notification, error) {
	var out []orderdomain.Notification
	for _, n := range r.notifications {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	return out, nil
}

func validOrderCommand() orderservice.CreateOrderCommand {
	return orderservice.CreateOrderCommand{
		UserID: uuid.New(),
		CartID: uuid.New(),
		Items: []orderdomain.Item{
			{ProductID: uuid.New(), ProductName: "Kubek", UnitPriceCents: 2450, Quantity: 2},
			{ProductID: uuid.New(), ProductName: "Koszulka", UnitPriceCents: 5990, Quantity: 1},
		},
	}
}

func TestCreateOrderComputesTotalAndNotifiesTheUser(t *testing.T) {
	repo := newFakeOrderRepo()
	svc := orderservice.NewOrderCommandService(repo)
	cmd := validOrderCommand()

	order, created, err := svc.CreateOrder(context.Background(), cmd)

	if err != nil || !created {
		t.Fatalf("created = %v, err = %v", created, err)
	}
	if want := int64(2*2450 + 5990); order.TotalCents != want {
		t.Fatalf("total = %d, want %d", order.TotalCents, want)
	}
	if order.Status != orderdomain.StatusProcessing {
		t.Fatalf("status = %s, want processing", order.Status)
	}
	if len(repo.notifications) != 1 || repo.notifications[0].UserID != cmd.UserID || repo.notifications[0].OrderID != order.ID {
		t.Fatalf("notifications = %+v", repo.notifications)
	}
}

func TestCreateOrderIsIdempotentPerCart(t *testing.T) {
	repo := newFakeOrderRepo()
	svc := orderservice.NewOrderCommandService(repo)
	cmd := validOrderCommand()

	first, _, err := svc.CreateOrder(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := svc.CreateOrder(context.Background(), cmd)

	if err != nil || created {
		t.Fatalf("created = %v, err = %v", created, err)
	}
	if first.ID != second.ID {
		t.Fatal("a retry must return the existing order")
	}
	if len(repo.notifications) != 1 {
		t.Fatalf("%d notifications, want 1", len(repo.notifications))
	}
}

func TestCreateOrderRejectsInvalidInput(t *testing.T) {
	tests := map[string]func(*orderservice.CreateOrderCommand){
		"no user":       func(c *orderservice.CreateOrderCommand) { c.UserID = uuid.Nil },
		"no cart":       func(c *orderservice.CreateOrderCommand) { c.CartID = uuid.Nil },
		"no items":      func(c *orderservice.CreateOrderCommand) { c.Items = nil },
		"zero quantity": func(c *orderservice.CreateOrderCommand) { c.Items[0].Quantity = 0 },
		"no product":    func(c *orderservice.CreateOrderCommand) { c.Items[0].ProductID = uuid.Nil },
		"negative cost": func(c *orderservice.CreateOrderCommand) { c.Items[0].UnitPriceCents = -1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			repo := newFakeOrderRepo()
			svc := orderservice.NewOrderCommandService(repo)

			cmd := validOrderCommand()
			mutate(&cmd)
			_, _, err := svc.CreateOrder(context.Background(), cmd)

			if !errors.Is(err, orderdomain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			if len(repo.orders) != 0 {
				t.Fatal("nothing may be stored for invalid input")
			}
		})
	}
}

func TestOrderQueriesAreScopedToTheUser(t *testing.T) {
	repo := newFakeOrderRepo()
	commands := orderservice.NewOrderCommandService(repo)
	queries := orderservice.NewOrderQueryService(repo, repo)
	cmd := validOrderCommand()
	order, _, err := commands.CreateOrder(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := queries.GetOrder(context.Background(), orderservice.GetOrderQuery{UserID: cmd.UserID, OrderID: order.ID}); err != nil {
		t.Fatalf("owner: %v", err)
	}
	stranger := uuid.New()
	if _, err := queries.GetOrder(context.Background(), orderservice.GetOrderQuery{UserID: stranger, OrderID: order.ID}); !errors.Is(err, orderdomain.ErrOrderNotFound) {
		t.Fatalf("stranger: err = %v, want ErrOrderNotFound", err)
	}
	if list, _ := queries.ListNotifications(context.Background(), orderservice.ListNotificationsQuery{UserID: stranger}); len(list) != 0 {
		t.Fatalf("stranger sees notifications: %+v", list)
	}
	if list, _ := queries.ListNotifications(context.Background(), orderservice.ListNotificationsQuery{UserID: cmd.UserID}); len(list) != 1 {
		t.Fatalf("owner sees %d notifications, want 1", len(list))
	}
}
