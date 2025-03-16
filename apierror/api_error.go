// Package apierror contains a variety of marshalable API errors that adhere to
// a unified error response convention.
package apierror

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// APIError is a struct that's embedded on a more specific API error struct (as
// seen below), and which provides a JSON serialization and a wait to
// conveniently write itself to an HTTP response.
//
// APIErrorInterface should be used with errors.As instead of this struct.
type APIError struct {
	// InternalError is an additional error that might be associated with the
	// API error. It's not returned in the API error response, but is logged in
	// API endpoint execution to provide extra information for operators.
	InternalError error `json:"-"`

	// Message is a descriptive, human-friendly message indicating what went
	// wrong. Try to make error messages as actionable as possible to help the
	// caller easily fix what went wrong.
	Message string `json:"message"`

	// StatusCode is the API error's HTTP status code. It's not marshaled to
	// JSON, but determines how the error is written to a response.
	StatusCode int `json:"-"`
}

func (e *APIError) Error() string                      { return e.Message }
func (e *APIError) GetInternalError() error            { return e.InternalError }
func (e *APIError) SetInternalError(internalErr error) { e.InternalError = internalErr }

// Write writes the API error to an HTTP response, writing to the given logger
// in case of a problem.
func (e *APIError) Write(ctx context.Context, logger *slog.Logger, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.StatusCode)

	respData, err := json.Marshal(e)
	if err != nil {
		logger.ErrorContext(ctx, "error marshaling API error", slog.String("error", err.Error()))
	}

	if _, err := w.Write(respData); err != nil {
		logger.ErrorContext(ctx, "error writing API error", slog.String("error", err.Error()))
	}
}

// Interface is an interface to an API error. This is needed for use with
// errors.As because APIError itself is embedded on another error struct, and
// won't be usable as an errors.As target.
type Interface interface {
	Error() string
	GetInternalError() error
	SetInternalError(internalErr error)
	Write(ctx context.Context, logger *slog.Logger, w http.ResponseWriter)
}

// WithInternalError is a convenience function for assigning an internal error
// to the given API error and returning it.
func WithInternalError[TAPIError Interface](apiErr TAPIError, internalErr error) TAPIError {
	apiErr.SetInternalError(internalErr)
	return apiErr
}

//
// BadRequest
//

type BadRequest struct { //nolint:errname
	APIError
}

func NewBadRequest(message string) *BadRequest {
	return &BadRequest{
		APIError: APIError{
			Message:    message,
			StatusCode: http.StatusBadRequest,
		},
	}
}

func NewBadRequestf(format string, a ...any) *BadRequest {
	return NewBadRequest(fmt.Sprintf(format, a...))
}

//
// InternalServerError
//

type InternalServerError struct {
	APIError
}

func NewInternalServerError(message string) *InternalServerError {
	return &InternalServerError{
		APIError: APIError{
			Message:    message,
			StatusCode: http.StatusInternalServerError,
		},
	}
}

func NewInternalServerErrorf(format string, a ...any) *InternalServerError {
	return NewInternalServerError(fmt.Sprintf(format, a...))
}

//
// NotFound
//

type NotFound struct { //nolint:errname
	APIError
}

func NewNotFound(message string) *NotFound {
	return &NotFound{
		APIError: APIError{
			Message:    message,
			StatusCode: http.StatusNotFound,
		},
	}
}

func NewNotFoundf(format string, a ...any) *NotFound {
	return NewNotFound(fmt.Sprintf(format, a...))
}

//
// RequestEntityTooLarge
//

type RequestEntityTooLarge struct { //nolint:errname
	APIError
}

func NewRequestEntityTooLarge(message string) *RequestEntityTooLarge {
	return &RequestEntityTooLarge{
		APIError: APIError{
			Message:    message,
			StatusCode: http.StatusRequestEntityTooLarge,
		},
	}
}

//
// ServiceUnavailable
//

type ServiceUnavailable struct { //nolint:errname
	APIError
}

func NewServiceUnavailable(message string) *ServiceUnavailable {
	return &ServiceUnavailable{
		APIError: APIError{
			Message:    message,
			StatusCode: http.StatusServiceUnavailable,
		},
	}
}

func NewServiceUnavailablef(format string, a ...any) *ServiceUnavailable {
	return NewServiceUnavailable(fmt.Sprintf(format, a...))
}

//
// Unauthorized
//

type Unauthorized struct { //nolint:errname
	APIError
}

func NewUnauthorized(format string, a ...any) *Unauthorized {
	return &Unauthorized{
		APIError: APIError{
			Message:    fmt.Sprintf(format, a...),
			StatusCode: http.StatusUnauthorized,
		},
	}
}
