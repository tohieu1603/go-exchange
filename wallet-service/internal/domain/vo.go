package domain

import "strings"

// Currency is a value object — an uppercase, non-empty asset symbol (USDT, VND,
// BTC…). Construct via NewCurrency so an invalid symbol can never enter the
// domain. It is stored as a plain string at the persistence/transport boundary.
type Currency string

// NewCurrency validates and normalises a currency symbol.
func NewCurrency(s string) (Currency, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || len(s) > 10 {
		return "", ErrInvalidCurrency
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return "", ErrInvalidCurrency
		}
	}
	return Currency(s), nil
}

func (c Currency) String() string { return string(c) }

// requirePositive guards an amount that must be strictly greater than zero.
func requirePositive(amount float64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}
