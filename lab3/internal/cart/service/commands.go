package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"lab3/internal/cart/domain"

	"github.com/google/uuid"
)

// CartCommandService is the command side: every method changes state.
//
// # Consistency of concurrent operations on one cart
//
// A user may add and remove items from several devices at once. AddItem and
// RemoveItem run inside CartRepository.Mutate, which locks the cart row for
// the whole operation, including the call to the product service. Operations
// on one cart are thus serialized, so none is lost or overwritten and the
// product reservation always matches the cart line. Carts of other users are
// not affected. The lock lives in the database, so it works the same with any
// number of cart service instances.
//
// If the cart line cannot be saved after the stock was reserved, the
// transaction rolls back and the stray reservation simply expires on its own.
type CartCommandService struct {
	carts    domain.CartRepository
	products domain.ProductClient
	ttl      time.Duration
}

func NewCartCommandService(carts domain.CartRepository, products domain.ProductClient, ttl time.Duration) *CartCommandService {
	return &CartCommandService{carts: carts, products: products, ttl: ttl}
}

type OpenCartCommand struct {
	UserID uuid.UUID
}

// OpenCart is idempotent: it returns the user's open cart or creates one.
func (s *CartCommandService) OpenCart(ctx context.Context, cmd OpenCartCommand) (cart domain.Cart, created bool, err error) {
	if cmd.UserID == uuid.Nil {
		return domain.Cart{}, false, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	}

	// An expired cart that was not swept yet would block the new one, and its
	// reservations would still lock the stock for the same user.
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := s.ExpireCarts(ctx); err != nil {
			return domain.Cart{}, false, err
		}
		cart, created, err = s.carts.OpenCart(ctx, cmd.UserID, s.ttl)
		if !errors.Is(err, domain.ErrCartNotFound) {
			break
		}
	}
	if err != nil {
		return domain.Cart{}, false, err
	}

	cart, err = s.carts.GetOpenCart(ctx, cmd.UserID)
	return cart, created, err
}

type AddItemCommand struct {
	UserID    uuid.UUID
	ProductID uuid.UUID
	Quantity  int
}

func (s *CartCommandService) AddItem(ctx context.Context, cmd AddItemCommand) (domain.Cart, error) {
	switch {
	case cmd.UserID == uuid.Nil:
		return domain.Cart{}, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	case cmd.ProductID == uuid.Nil:
		return domain.Cart{}, fmt.Errorf("%w: product_id is required", domain.ErrInvalidInput)
	case cmd.Quantity <= 0:
		return domain.Cart{}, fmt.Errorf("%w: quantity must be positive", domain.ErrInvalidInput)
	}

	product, err := s.products.GetProduct(ctx, cmd.ProductID)
	if err != nil {
		return domain.Cart{}, err
	}

	err = s.carts.Mutate(ctx, cmd.UserID, func(ctx context.Context, tx domain.CartTx) error {
		err := s.products.Reserve(ctx, domain.ReserveRequest{
			CartID:    tx.Cart().ID,
			UserID:    cmd.UserID,
			ProductID: cmd.ProductID,
			Quantity:  cmd.Quantity,
		})
		if err != nil {
			return err
		}
		return tx.UpsertItem(ctx, domain.Item{
			ProductID:      product.ID,
			ProductName:    product.Name,
			UnitPriceCents: product.PriceCents,
			Quantity:       cmd.Quantity,
		})
	})
	if err != nil {
		return domain.Cart{}, err
	}
	return s.carts.GetOpenCart(ctx, cmd.UserID)
}

type RemoveItemCommand struct {
	UserID    uuid.UUID
	ProductID uuid.UUID
}

func (s *CartCommandService) RemoveItem(ctx context.Context, cmd RemoveItemCommand) (domain.Cart, error) {
	if cmd.UserID == uuid.Nil || cmd.ProductID == uuid.Nil {
		return domain.Cart{}, fmt.Errorf("%w: user_id and product_id are required", domain.ErrInvalidInput)
	}

	err := s.carts.Mutate(ctx, cmd.UserID, func(ctx context.Context, tx domain.CartTx) error {
		removed, err := tx.DeleteItem(ctx, cmd.ProductID)
		if err != nil {
			return err
		}
		// Release even when the line was missing, so a retry after a failed
		// release still frees the stock. A failure rolls the delete back.
		if err := s.products.Release(ctx, tx.Cart().ID, cmd.ProductID); err != nil {
			return err
		}
		if !removed {
			return domain.ErrItemNotFound
		}
		return nil
	})
	if err != nil {
		return domain.Cart{}, err
	}
	return s.carts.GetOpenCart(ctx, cmd.UserID)
}

// ExpireCarts deletes carts past their expiry and releases their reservations.
// Correctness does not depend on it: reservations expire by themselves. It only
// frees the stock earlier and keeps the tables small.
func (s *CartCommandService) ExpireCarts(ctx context.Context) (int, error) {
	ids, err := s.carts.DeleteExpired(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.products.ReleaseCart(ctx, id); err != nil {
			log.Printf("release reservations of expired cart %s: %v", id, err)
		}
	}
	return len(ids), nil
}
