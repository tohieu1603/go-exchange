package postgres

import "fmt"

// wrap annotates a non-nil error with the operation name, returning nil when err
// is nil. Used by the :exec adapter methods to keep them one-liners.
func wrap(op string, err error) error {
	if err != nil {
		return fmt.Errorf("postgres: %s: %w", op, err)
	}
	return nil
}

// uptrToI64 / i64ptrToUint bridge the domain's *uint optional ids and sqlc's
// nullable *int64 columns (e.g. refresh_tokens.parent_id).
func uptrToI64(p *uint) *int64 {
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}

func i64ptrToUint(p *int64) *uint {
	if p == nil {
		return nil
	}
	v := uint(*p)
	return &v
}
