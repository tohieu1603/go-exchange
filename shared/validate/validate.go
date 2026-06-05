// Package validate wraps go-playground/validator and returns apperr.Invalid on
// failure (mirrors the appios platform/validation convention), so usecases get a
// transport-mappable error for free.
package validate

import (
	"github.com/cryptox/shared/apperr"
	"github.com/go-playground/validator/v10"
)

var v = validator.New(validator.WithRequiredStructEnabled())

// Struct validates s against its `validate:"..."` tags. A validation failure is
// returned as apperr.Invalid (→ gRPC InvalidArgument / HTTP 400).
func Struct(s any) error {
	if err := v.Struct(s); err != nil {
		return apperr.Wrap(apperr.KindInvalid, err, "validation failed")
	}
	return nil
}

// Var validates a single value against a tag expression (e.g. "required,email").
func Var(field any, tag string) error {
	if err := v.Var(field, tag); err != nil {
		return apperr.Wrap(apperr.KindInvalid, err, "validation failed")
	}
	return nil
}
