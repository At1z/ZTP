package repository

import (
	"strconv"

	"lab1/internal/domain"
)

type fieldChange struct {
	field, oldValue, newValue string
}

// diffProducts lists the tracked fields whose values differ between two versions.
func diffProducts(old, next domain.Product) []fieldChange {
	var changes []fieldChange
	add := func(field, o, n string) {
		if o != n {
			changes = append(changes, fieldChange{field, o, n})
		}
	}
	add("name", old.Name, next.Name)
	add("category", string(old.Category), string(next.Category))
	add("price", formatPrice(old.Price), formatPrice(next.Price))
	add("quantity", strconv.Itoa(old.Quantity), strconv.Itoa(next.Quantity))
	return changes
}

func formatPrice(price float64) string {
	return strconv.FormatFloat(price, 'f', 2, 64)
}
