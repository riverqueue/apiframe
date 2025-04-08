package apitype

import (
	"github.com/go-playground/validator/v10"
)

// NewValidator creates a new validator with ExplicitNullable validation configured.
//
// This function is exported so that it can be used by other packages that need
// to validate Patch fields.
func NewValidator() *validator.Validate {
	// WithRequiredStructEnabled can be removed once validator/v11 is released.
	val := validator.New(validator.WithRequiredStructEnabled())
	return WithExplicitNullableValidation[string](val)
}

// WithExplicitNullableValidation registers the validator with the
// ExplicitNullable type.
func WithExplicitNullableValidation[T any](val *validator.Validate) *validator.Validate {
	val.RegisterCustomTypeFunc(ExtractExplicitNullableValueForValidation[T], ExplicitNullable[T]{})
	return val
}
