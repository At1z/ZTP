package unit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"lab1/internal/domain"
)

var ctx = context.Background()

func TestCreateValidProduct(t *testing.T) {
	e := newEnv()

	p, err := e.products.Create(ctx, validInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == 0 || p.Name != "Laptop1" {
		t.Fatalf("unexpected product: %+v", p)
	}
}

func TestNameRules(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"minimum length", "abc", false},
		{"maximum length", strings.Repeat("a", 20), false},
		{"letters and digits", "Abc123", false},
		{"polish letters", "żółć1", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 21), true},
		{"empty", "", true},
		{"space", "ab cd", true},
		{"special character", "abc!", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			in.Name = tc.input
			_, err := newEnv().products.Create(ctx, in)
			if tc.wantErr {
				expectInvalidField(t, err, "name")
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPriceLimitsPerCategory(t *testing.T) {
	tests := []struct {
		category domain.Category
		price    float64
		wantErr  bool
	}{
		{domain.CategoryElectronics, 50, false},
		{domain.CategoryElectronics, 50000, false},
		{domain.CategoryElectronics, 49.99, true},
		{domain.CategoryElectronics, 50000.01, true},
		{domain.CategoryBooks, 5, false},
		{domain.CategoryBooks, 500, false},
		{domain.CategoryBooks, 4.99, true},
		{domain.CategoryBooks, 500.01, true},
		{domain.CategoryClothing, 10, false},
		{domain.CategoryClothing, 5000, false},
		{domain.CategoryClothing, 9.99, true},
		{domain.CategoryClothing, 5000.01, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			in := validInput()
			in.Category, in.Price = tc.category, ptr(tc.price)
			_, err := newEnv().products.Create(ctx, in)
			if tc.wantErr {
				expectInvalidField(t, err, "price")
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUnknownCategory(t *testing.T) {
	in := validInput()
	in.Category = "toys"
	_, err := newEnv().products.Create(ctx, in)
	expectInvalidField(t, err, "category")
}

func TestQuantityMustNotBeNegative(t *testing.T) {
	e := newEnv()

	in := validInput()
	in.Quantity = ptr(0)
	if _, err := e.products.Create(ctx, in); err != nil {
		t.Fatalf("quantity 0 should be valid: %v", err)
	}

	in.Name, in.Quantity = "Other1", ptr(-1)
	_, err := e.products.Create(ctx, in)
	expectInvalidField(t, err, "quantity")
}

func TestMissingPriceAndQuantityAreRequired(t *testing.T) {
	in := validInput()
	in.Price, in.Quantity = nil, nil
	_, err := newEnv().products.Create(ctx, in)

	if got := strings.Join(invalidFields(t, err), ","); got != "price,quantity" {
		t.Fatalf("fields = %q, want price,quantity", got)
	}
}

func TestAllInvalidFieldsAreReported(t *testing.T) {
	in := domain.ProductInput{Name: "a!", Category: domain.CategoryBooks, Price: ptr(1000.0), Quantity: ptr(-5)}
	_, err := newEnv().products.Create(ctx, in)

	if got := strings.Join(invalidFields(t, err), ","); got != "name,price,quantity" {
		t.Fatalf("fields = %q, want name,price,quantity", got)
	}
}

func TestDuplicateNameIsConflict(t *testing.T) {
	e := newEnv()
	if _, err := e.products.Create(ctx, validInput()); err != nil {
		t.Fatal(err)
	}

	_, err := e.products.Create(ctx, validInput())
	var conflict *domain.ConflictError
	if !errors.As(err, &conflict) || conflict.Field != "name" {
		t.Fatalf("error = %v, want name conflict", err)
	}
}

func TestBannedPhraseBlocksCreateCaseInsensitive(t *testing.T) {
	e := newEnv()
	if _, err := e.phrases.Add(ctx, domain.BannedPhraseInput{Phrase: "spam"}); err != nil {
		t.Fatal(err)
	}

	in := validInput()
	in.Name = "MegaSPAMdeal"
	_, err := e.products.Create(ctx, in)
	expectInvalidField(t, err, "name")
}

func TestBannedPhraseBlocksUpdate(t *testing.T) {
	e := newEnv()
	created, _ := e.products.Create(ctx, validInput())
	if _, err := e.phrases.Add(ctx, domain.BannedPhraseInput{Phrase: "spam"}); err != nil {
		t.Fatal(err)
	}

	in := validInput()
	in.Name = "spamLaptop"
	_, err := e.products.Update(ctx, created.ID, in)
	expectInvalidField(t, err, "name")
}

func TestRemovedPhraseNoLongerBlocks(t *testing.T) {
	e := newEnv()
	phrase, _ := e.phrases.Add(ctx, domain.BannedPhraseInput{Phrase: "spam"})
	if err := e.phrases.Delete(ctx, phrase.ID); err != nil {
		t.Fatal(err)
	}

	in := validInput()
	in.Name = "spamLaptop"
	if _, err := e.products.Create(ctx, in); err != nil {
		t.Fatalf("unexpected error after removing phrase: %v", err)
	}
}

func TestUpdateProduct(t *testing.T) {
	e := newEnv()
	created, _ := e.products.Create(ctx, validInput())

	in := validInput()
	in.Price = ptr(2500.0)
	updated, err := e.products.Update(ctx, created.ID, in)
	if err != nil {
		t.Fatalf("Update with unchanged name: %v", err)
	}
	if updated.ID != created.ID || updated.Price != 2500 {
		t.Fatalf("unexpected product: %+v", updated)
	}
}

func TestUpdateToExistingNameIsConflict(t *testing.T) {
	e := newEnv()
	e.products.Create(ctx, validInput())
	other := validInput()
	other.Name = "Other1"
	created, _ := e.products.Create(ctx, other)

	_, err := e.products.Update(ctx, created.ID, validInput())
	var conflict *domain.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
}

func TestNotFound(t *testing.T) {
	e := newEnv()

	if _, err := e.products.Get(ctx, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get: %v, want ErrNotFound", err)
	}
	if _, err := e.products.Update(ctx, 42, validInput()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Update: %v, want ErrNotFound", err)
	}
	if err := e.products.Delete(ctx, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Delete: %v, want ErrNotFound", err)
	}
	if _, err := e.products.History(ctx, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("History: %v, want ErrNotFound", err)
	}
}
