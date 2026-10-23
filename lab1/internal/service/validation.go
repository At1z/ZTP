package service

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"lab1/internal/domain"
)

// nameFormatError checks the length and character rules for a product name.
func nameFormatError(name string) string {
	if n := utf8.RuneCountInString(name); n < domain.NameMinLength || n > domain.NameMaxLength {
		return fmt.Sprintf("must be between %d and %d characters long", domain.NameMinLength, domain.NameMaxLength)
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "may contain only letters and digits"
		}
	}
	return ""
}

// findBannedPhrase returns the first banned phrase contained in name (case-insensitive).
func findBannedPhrase(name string, phrases []domain.BannedPhrase) (string, bool) {
	lower := strings.ToLower(name)
	for _, p := range phrases {
		if strings.Contains(lower, p.Phrase) {
			return p.Phrase, true
		}
	}
	return "", false
}

// categoryAndPriceErrors validates the category and, if it is known, the price range.
func categoryAndPriceErrors(in domain.ProductInput) []domain.FieldError {
	var errs []domain.FieldError
	limit, known := domain.PriceLimits[in.Category]
	if !known {
		errs = append(errs, domain.FieldError{Field: "category", Message: "must be one of: electronics, books, clothing"})
	}
	switch {
	case in.Price == nil:
		errs = append(errs, requiredError("price"))
	case known && (*in.Price < limit.Min || *in.Price > limit.Max):
		errs = append(errs, domain.FieldError{
			Field:   "price",
			Message: fmt.Sprintf("must be between %.2f and %.2f for category %s", limit.Min, limit.Max, in.Category),
		})
	}
	return errs
}

func quantityErrors(quantity *int) []domain.FieldError {
	switch {
	case quantity == nil:
		return []domain.FieldError{requiredError("quantity")}
	case *quantity < 0:
		return []domain.FieldError{{Field: "quantity", Message: "must not be negative"}}
	}
	return nil
}

func requiredError(field string) domain.FieldError {
	return domain.FieldError{Field: field, Message: "is required"}
}
