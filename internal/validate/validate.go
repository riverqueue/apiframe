// Package validate internalizes Go Playground's Validator framework, setting
// some common options that we use everywhere, providing some useful helpers,
// and exporting a simplified API.
package validate

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Default is the package's default validator instance. WithRequiredStructEnabled
// can be removed once validator/v11 is released.
var Default = validator.New(validator.WithRequiredStructEnabled()) //nolint:gochecknoglobals

func init() { //nolint:gochecknoinits
	Default.RegisterTagNameFunc(preferPublicName)
}

// PublicFacingMessage builds a complete error message from a validator error
// that's suitable for public-facing consumption.
//
// I only added a few possible validations to start. We'll probably need to add
// more as we go and expand our usage.
func PublicFacingMessage(v *validator.Validate, validatorErr error) string {
	var message strings.Builder

	//nolint:errorlint
	if validationErrs, ok := validatorErr.(validator.ValidationErrors); ok {
		for _, fieldErr := range validationErrs {
			switch fieldErr.Tag() {
			case "lte":
				fallthrough // lte and max are synonyms
			case "max":
				kind := fieldErr.Kind()
				if kind == reflect.Pointer {
					kind = fieldErr.Type().Elem().Kind()
				}

				switch kind { //nolint:exhaustive
				case reflect.Float32, reflect.Float64, reflect.Int, reflect.Int32, reflect.Int64:
					fmt.Fprintf(&message, " Field `%s` must be less than or equal to %s.",
						fieldErr.Field(), fieldErr.Param())

				case reflect.Slice, reflect.Map:
					fmt.Fprintf(&message, " Field `%s` must contain at most %s element(s).",
						fieldErr.Field(), fieldErr.Param())

				case reflect.String:
					fmt.Fprintf(&message, " Field `%s` must be at most %s character(s) long.",
						fieldErr.Field(), fieldErr.Param())

				default:
					message.WriteString(fieldErr.Error())
				}

			case "gte":
				fallthrough // gte and min are synonyms
			case "min":
				kind := fieldErr.Kind()
				if kind == reflect.Pointer {
					kind = fieldErr.Type().Elem().Kind()
				}

				switch kind { //nolint:exhaustive
				case reflect.Float32, reflect.Float64, reflect.Int, reflect.Int32, reflect.Int64:
					fmt.Fprintf(&message, " Field `%s` must be greater or equal to %s.",
						fieldErr.Field(), fieldErr.Param())

				case reflect.Slice, reflect.Map:
					fmt.Fprintf(&message, " Field `%s` must contain at least %s element(s).",
						fieldErr.Field(), fieldErr.Param())

				case reflect.String:
					fmt.Fprintf(&message, " Field `%s` must be at least %s character(s) long.",
						fieldErr.Field(), fieldErr.Param())

				default:
					message.WriteString(fieldErr.Error())
				}

			case "oneof":
				fmt.Fprintf(&message, " Field `%s` should be one of the following values: %s.",
					fieldErr.Field(), fieldErr.Param())

			case "required":
				fmt.Fprintf(&message, " Field `%s` is required.", fieldErr.Field())

			default:
				fmt.Fprintf(&message, " Validation on field `%s` failed on the `%s` tag.", fieldErr.Field(), fieldErr.Tag())
			}
		}
	}

	return strings.TrimSpace(message.String())
}

// preferPublicName is a validator tag naming function that uses public names
// like a field's JSON tag instead of actual field names in structs.
// This is important because we sent these back as user-facing errors (and the
// users submitted them as JSON/path parameters).
func preferPublicName(fld reflect.StructField) string {
	name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
	if name != "" && name != "-" {
		return name
	}

	return fld.Name
}
