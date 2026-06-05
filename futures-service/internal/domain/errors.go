package domain

import "errors"

var (
	ErrPriceUnavailable = errors.New("price unavailable")
	ErrPositionNotFound = errors.New("position not found or already closed")
	ErrInvalidSide      = errors.New("side must be LONG or SHORT")
	ErrInvalidLeverage  = errors.New("leverage must be between 1 and 125")
	ErrInvalidSize      = errors.New("size must be positive")
)
