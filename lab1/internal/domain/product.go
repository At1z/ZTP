package domain

import "time"

type Category string

const (
	CategoryElectronics Category = "electronics"
	CategoryBooks       Category = "books"
	CategoryClothing    Category = "clothing"
)

type PriceLimit struct {
	Min, Max float64
}

// PriceLimits defines the allowed price range (PLN) per category.
var PriceLimits = map[Category]PriceLimit{
	CategoryElectronics: {Min: 50, Max: 50000},
	CategoryBooks:       {Min: 5, Max: 500},
	CategoryClothing:    {Min: 10, Max: 5000},
}

const (
	NameMinLength = 3
	NameMaxLength = 20
)

type Product struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Category  Category  `json:"category"`
	Price     float64   `json:"price"`
	Quantity  int       `json:"quantity"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProductInput is the payload used to create or replace a product. Price and
// Quantity are pointers so that a missing field can be told apart from zero.
type ProductInput struct {
	Name     string   `json:"name"`
	Category Category `json:"category"`
	Price    *float64 `json:"price"`
	Quantity *int     `json:"quantity"`
}

type HistoryEntry struct {
	ID        int64     `json:"id"`
	ProductID int64     `json:"product_id"`
	Field     string    `json:"field"`
	OldValue  string    `json:"old_value"`
	NewValue  string    `json:"new_value"`
	ChangedAt time.Time `json:"changed_at"`
}
