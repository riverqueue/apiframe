package apitype

import (
	"encoding/json"
	"reflect"
)

// ExplicitNullable represents a nullable field that can distinguish between:
//
// - Field omitted from request (Set=false)
// - Field explicitly set to null (Set=true, Value=nil)
// - Field set to a value (Set=true, Value!=nil).
type ExplicitNullable[T any] struct {
	// Set is true if the field was present in the request.
	Set bool
	// Value is the string value if provided.
	Value *T
}

// UnmarshalJSON implements json.Unmarshaler to handle the three possible states
// of an ExplicitNullable[T] field in a JSON payload.
func (ps *ExplicitNullable[T]) UnmarshalJSON(data []byte) error {
	// Mark the field as present.
	ps.Set = true
	// If the JSON value is "null", mark as explicit null.
	if string(data) == "null" {
		return nil
	}
	// Otherwise, unmarshal into the value.
	return json.Unmarshal(data, &ps.Value)
}

// ExtractExplicitNullableValueForValidation extracts a value suitable for
// validation from an ExplicitNullable field. Returns nil if the field is
// omitted or explicitly null, which causes validation to be skipped. Returns
// the string value if the field was explicitly set, allowing validation of
// empty strings.
//
// This function is designed to be used with validator.RegisterCustomTypeFunc.
func ExtractExplicitNullableValueForValidation[T any](field reflect.Value) interface{} {
	ps, ok := field.Interface().(ExplicitNullable[T])
	if !ok || !ps.Set || ps.Value == nil {
		return nil
	}
	return ps.Value
}
