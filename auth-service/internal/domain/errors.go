package domain

import "errors"

// ErrNotFound is returned by the persistence adapters when a row does not exist.
// It replaces the gorm.ErrRecordNotFound the application layer used to compare
// against, keeping gorm out of the use-case code.
var ErrNotFound = errors.New("record not found")
