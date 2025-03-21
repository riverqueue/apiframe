// Package apiendpoint provides a lightweight API framework extracted from its
// original use in River projects.  It lets API endpoints be defined, then
// mounted into an http.ServeMux.
package apiendpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/riverqueue/apiframe/apierror"
	"github.com/riverqueue/apiframe/apimiddleware"
	"github.com/riverqueue/apiframe/internal/validate"
)

// Endpoint is a struct that should be embedded on an API endpoint, and which
// provides a partial implementation for EndpointInterface.
type Endpoint[TReq any, TResp any] struct {
	// Logger used to log information about endpoint execution.
	logger *slog.Logger

	// Metadata about the endpoint. This is not available until SetMeta is
	// invoked on the endpoint, which is usually done in Mount.
	meta *EndpointMeta
}

func (e *Endpoint[TReq, TResp]) SetLogger(logger *slog.Logger) { e.logger = logger }
func (e *Endpoint[TReq, TResp]) SetMeta(meta *EndpointMeta)    { e.meta = meta }

type EndpointInterface interface {
	// Meta returns metadata about an API endpoint, like the path it should be
	// mounted at, and the status code it returns on success.
	//
	// This should be implemented by each specific API endpoint.
	Meta() *EndpointMeta

	// SetLogger sets a logger on the endpoint.
	//
	// Implementation inherited from an embedded Endpoint struct.
	SetLogger(logger *slog.Logger)

	// SetMeta sets metadata on an Endpoint struct after it's extracted from a
	// call to an endpoint's Meta function.
	//
	// Implementation inherited from an embedded Endpoint struct.
	SetMeta(meta *EndpointMeta)
}

// EndpointExecuteInterface is an interface to an API endpoint. Some of it is
// implemented by an embedded Endpoint struct, and some of it should be
// implemented by the endpoint itself.
type EndpointExecuteInterface[TReq any, TResp any] interface {
	EndpointInterface

	// Execute executes the API endpoint.
	//
	// This should be implemented by each specific API endpoint.
	Execute(ctx context.Context, req *TReq) (*TResp, error)
}

// EndpointMeta is metadata about an API endpoint.
type EndpointMeta struct {
	// Pattern is the API endpoint's HTTP method and path where it should be
	// mounted, which is passed to http.ServeMux by Mount. It should start with
	// a verb like `GET` or `POST`, and may contain Go 1.22 path variables like
	// `{name}`, whose values should be extracted by an endpoint request
	// struct's custom ExtractRaw implementation.
	Pattern string

	// StatusCode is the status code to be set on a successful response.
	StatusCode int
}

func (m *EndpointMeta) validate() {
	if m.Pattern == "" {
		panic("Endpoint.Path is required")
	}
	if m.StatusCode == 0 {
		panic("Endpoint.StatusCode is required")
	}
}

type MountOpts struct {
	Logger *slog.Logger
	// MiddlewareStack is a stack of middleware that will be mounted in front of
	// the API endpoint handler. If not specified, no middleware will be used.
	MiddlewareStack *apimiddleware.MiddlewareStack
	// Validator is the validator to use for this endpoint. If not specified,
	// the default validator will be used.
	Validator *validator.Validate
}

// Mount mounts an endpoint to a Go http.ServeMux. The logger is used to log
// information about endpoint execution.
func Mount[TReq any, TResp any](mux *http.ServeMux, apiEndpoint EndpointExecuteInterface[TReq, TResp], opts *MountOpts) EndpointInterface {
	if opts == nil {
		opts = &MountOpts{}
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	validator := opts.Validator
	if validator == nil {
		validator = validate.Default
	}

	apiEndpoint.SetLogger(logger)

	meta := apiEndpoint.Meta()
	meta.validate() // panic on problem
	apiEndpoint.SetMeta(meta)

	innerHandler := func(w http.ResponseWriter, r *http.Request) {
		executeAPIEndpoint(w, r, opts.Logger, meta, validator, apiEndpoint.Execute)
	}

	if opts.MiddlewareStack != nil {
		mux.Handle(meta.Pattern, opts.MiddlewareStack.Mount(http.HandlerFunc(innerHandler)))
	} else {
		mux.HandleFunc(meta.Pattern, innerHandler)
	}

	return apiEndpoint
}

func executeAPIEndpoint[TReq any, TResp any](w http.ResponseWriter, r *http.Request, logger *slog.Logger, meta *EndpointMeta, validator *validator.Validate, execute func(ctx context.Context, req *TReq) (*TResp, error)) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err := func() error {
		var req TReq
		if r.Method != http.MethodGet {
			reqData, err := io.ReadAll(r.Body)
			if err != nil {
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					return apierror.NewRequestEntityTooLarge("Request entity too large.")
				}
				return fmt.Errorf("error reading request body: %w", err)
			}

			if len(reqData) > 0 {
				if err := json.Unmarshal(reqData, &req); err != nil {
					return apierror.NewBadRequestf("Error unmarshaling request body: %s.", err)
				}
			}

			r.Body = io.NopCloser(bytes.NewReader(reqData))
		}

		if rawExtractor, ok := any(&req).(RawExtractor); ok {
			if err := rawExtractor.ExtractRaw(r); err != nil {
				return err
			}
		}

		if err := validator.StructCtx(ctx, &req); err != nil {
			return apierror.NewBadRequest(validate.PublicFacingMessage(validator, err))
		}

		resp, err := execute(ctx, &req)
		if err != nil {
			return err
		}

		if rawExtractor, ok := any(resp).(RawResponder); ok {
			return rawExtractor.RespondRaw(w)
		}

		respData, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("error marshaling response JSON: %w", err)
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(meta.StatusCode)

		if _, err := w.Write(respData); err != nil {
			return fmt.Errorf("error writing response: %w", err)
		}

		return nil
	}()
	if err != nil {
		// Convert certain types of Postgres errors into something more
		// user-friendly than an internal server error.
		err = maybeInterpretInternalError(err)

		var apiErr apierror.Interface
		if errors.As(err, &apiErr) {
			logAttrs := []any{
				slog.String("error", apiErr.Error()),
			}

			if internalErr := apiErr.GetInternalError(); internalErr != nil {
				logAttrs = append(logAttrs, slog.String("internal_error", internalErr.Error()))
			}

			// Logged at info level because API errors are normal.
			logger.InfoContext(ctx, "API error response", logAttrs...)

			apiErr.Write(ctx, logger, w)
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			logger.ErrorContext(ctx, "request timeout", slog.String("error", err.Error()))
			apierror.NewServiceUnavailable("Request timed out. Retrying the request might work.").Write(ctx, logger, w)
			return
		}

		// Internal server error. The error goes to logs but should not be
		// included in the response in case there's something sensitive in
		// the error string.
		logger.ErrorContext(ctx, "error running API route", slog.String("error", err.Error()))
		apierror.NewInternalServerError("Internal server error. Check logs for more information.").Write(ctx, logger, w)
	}
}

// RawExtractor is an interface that can be implemented by request structs that
// allows them to extract information from a raw request, like path values.
type RawExtractor interface {
	ExtractRaw(r *http.Request) error
}

// RawResponder is an interface that can be implemented by response structs that
// allow them to respond directly to a ResponseWriter instead of emitting the
// normal JSON format.
type RawResponder interface {
	RespondRaw(w http.ResponseWriter) error
}

// Make some broad categories of internal error back into something public
// facing because in some cases they can be a vast help for debugging.
func maybeInterpretInternalError(err error) error {
	var (
		apiErr     apierror.Interface
		connectErr *pgconn.ConnectError
		pgErr      *pgconn.PgError
	)

	switch {
	case errors.As(err, &connectErr):
		apiErr = apierror.NewBadRequest("There was a problem connecting to the configured database. Check logs for details.")

	case errors.As(err, &pgErr):
		if pgErr.Code == pgerrcode.InsufficientPrivilege {
			apiErr = apierror.NewBadRequest("Insufficient database privilege to perform this operation.")
		} else {
			return err
		}

	default:
		return err
	}

	return apierror.WithInternalError(apiErr, err)
}
