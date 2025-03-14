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
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/riverqueue/apiframe/apierror"
	"github.com/riverqueue/river/rivershared/riversharedtest"
)

func TestMountAndServe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	type testBundle struct {
		logger   *slog.Logger
		recorder *httptest.ResponseRecorder
	}

	setup := func(t *testing.T) (*http.ServeMux, *testBundle) {
		t.Helper()

		var (
			logger = riversharedtest.Logger(t)
			mux    = http.NewServeMux()
			opts   = &MountOpts{Logger: logger}
		)

		Mount(mux, &getEndpoint{}, opts)
		Mount(mux, &postEndpoint{}, opts)

		return mux, &testBundle{
			logger:   logger,
			recorder: httptest.NewRecorder(),
		}
	}

	t.Run("GetEndpoint", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodGet, "/api/get-endpoint", nil)
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusOK, &getResponse{Message: "Hello."}, bundle.recorder)
	})

	t.Run("BodyIgnoredOnGet", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodGet, "/api/get-endpoint",
			bytes.NewBuffer(mustMarshalJSON(t, &getRequest{IgnoredJSONMessage: "Ignored hello."})))
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusOK, &getResponse{Message: "Hello."}, bundle.recorder)
	})

	t.Run("MaxBytesErrorHandling", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		payload := mustMarshalJSON(t, &postRequest{Message: "Hello."})

		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123", bytes.NewBuffer(payload))
		req.Body = http.MaxBytesReader(bundle.recorder, io.NopCloser(bytes.NewReader(payload)), int64(len(payload)-1))
		mux.ServeHTTP(bundle.recorder, req)
		requireStatusAndJSONResponse(t, http.StatusRequestEntityTooLarge, &apierror.APIError{Message: "Request entity too large."}, bundle.recorder)
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodPost, "/api/get-endpoint", nil)
		mux.ServeHTTP(bundle.recorder, req)

		// This error comes from net/http.
		requireStatusAndResponse(t, http.StatusMethodNotAllowed, "Method Not Allowed\n", bundle.recorder)
	})

	t.Run("NilOptions", func(t *testing.T) {
		t.Parallel()

		_, bundle := setup(t)

		mux := http.NewServeMux()
		Mount(mux, &postEndpoint{}, nil)

		reqPayload := mustMarshalJSON(t, &postRequest{Message: "Hello."})
		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123", bytes.NewBuffer(reqPayload))
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusCreated, &postResponse{ID: "123", Message: "Hello.", RawPayload: reqPayload}, bundle.recorder)
	})

	t.Run("OptionsWithCustomLogger", func(t *testing.T) {
		t.Parallel()

		_, bundle := setup(t)

		mux := http.NewServeMux()
		Mount(mux, &getEndpoint{}, &MountOpts{Logger: bundle.logger})

		req := httptest.NewRequest(http.MethodGet, "/api/get-endpoint", nil)
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusOK, &getResponse{Message: "Hello."}, bundle.recorder)
	})

	t.Run("PostEndpointAndExtractRaw", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		reqPayload := mustMarshalJSON(t, &postRequest{Message: "Hello."})
		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123", bytes.NewBuffer(reqPayload))
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusCreated, &postResponse{ID: "123", Message: "Hello.", RawPayload: reqPayload}, bundle.recorder)
	})

	t.Run("ValidationError", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123", nil)
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusBadRequest, &apierror.APIError{Message: "Field `message` is required."}, bundle.recorder)
	})

	t.Run("APIError", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123",
			bytes.NewBuffer(mustMarshalJSON(t, &postRequest{MakeAPIError: true, Message: "Hello."})))
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusBadRequest, &apierror.APIError{Message: "Bad request."}, bundle.recorder)
	})

	t.Run("InterpretedError", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123",
			bytes.NewBuffer(mustMarshalJSON(t, &postRequest{MakePostgresError: true, Message: "Hello."})))
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusBadRequest, &apierror.APIError{Message: "Insufficient database privilege to perform this operation."}, bundle.recorder)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		ctx, cancel := context.WithDeadline(ctx, time.Now())
		t.Cleanup(cancel)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/post-endpoint/123",
			bytes.NewBuffer(mustMarshalJSON(t, &postRequest{Message: "Hello."})))
		require.NoError(t, err)
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusServiceUnavailable, &apierror.APIError{Message: "Request timed out. Retrying the request might work."}, bundle.recorder)
	})

	t.Run("InternalServerError", func(t *testing.T) {
		t.Parallel()

		mux, bundle := setup(t)

		req := httptest.NewRequest(http.MethodPost, "/api/post-endpoint/123",
			bytes.NewBuffer(mustMarshalJSON(t, &postRequest{MakeInternalError: true, Message: "Hello."})))
		mux.ServeHTTP(bundle.recorder, req)

		requireStatusAndJSONResponse(t, http.StatusInternalServerError, &apierror.APIError{Message: "Internal server error. Check logs for more information."}, bundle.recorder)
	})
}

func TestMaybeInterpretInternalError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("ConnectError", func(t *testing.T) {
		t.Parallel()

		_, err := pgconn.Connect(ctx, "postgres://user@127.0.0.1:37283/does_not_exist")

		require.Equal(t, apierror.WithInternalError(apierror.NewBadRequest("There was a problem connecting to the configured database. Check logs for details."), err), maybeInterpretInternalError(err))
	})

	t.Run("ConnectError", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: pgerrcode.InsufficientPrivilege}

		require.Equal(t, apierror.WithInternalError(apierror.NewBadRequest("Insufficient database privilege to perform this operation."), err), maybeInterpretInternalError(err))
	})

	t.Run("OtherPGError", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: pgerrcode.CardinalityViolation}

		require.Equal(t, err, maybeInterpretInternalError(err))
	})

	t.Run("ConnectError", func(t *testing.T) {
		t.Parallel()

		err := errors.New("other error")

		require.Equal(t, err, maybeInterpretInternalError(err))
	})
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func mustUnmarshalJSON[T any](t *testing.T, data []byte) *T {
	t.Helper()

	var val T
	err := json.Unmarshal(data, &val)
	require.NoError(t, err)
	return &val
}

// Shortcut for requiring an HTTP status code and a JSON-marshaled response
// equivalent to expectedResp. The important thing that is does is that in the
// event of a failure on status code, it prints the response body as additional
// context to help debug the problem.
func requireStatusAndJSONResponse[T any](t *testing.T, expectedStatusCode int, expectedResp *T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, expectedStatusCode, recorder.Result().StatusCode, "Unexpected status code; response body: %s", recorder.Body.String())
	require.Equal(t, expectedResp, mustUnmarshalJSON[T](t, recorder.Body.Bytes()))
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
}

// Same as the above, but for a non-JSON response.
func requireStatusAndResponse(t *testing.T, expectedStatusCode int, expectedResp string, recorder *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, expectedStatusCode, recorder.Result().StatusCode, "Unexpected status code; response body: %s", recorder.Body.String())
	require.Equal(t, expectedResp, recorder.Body.String())
}

//
// getEndpoint
//

type getEndpoint struct {
	Endpoint[getRequest, getResponse]
}

func (*getEndpoint) Meta() *EndpointMeta {
	return &EndpointMeta{
		Pattern:    "GET /api/get-endpoint",
		StatusCode: http.StatusOK,
	}
}

type getRequest struct {
	IgnoredJSONMessage string `json:"ignored_json" validate:"-"`
}

type getResponse struct {
	Message string `json:"message" validate:"required"`
}

func (a *getEndpoint) Execute(_ context.Context, req *getRequest) (*getResponse, error) {
	// This branch never gets taken because request bodies are ignored on GET.
	if req.IgnoredJSONMessage != "" {
		return &getResponse{Message: req.IgnoredJSONMessage}, nil
	}

	return &getResponse{Message: "Hello."}, nil
}

//
// postEndpoint
//

type postEndpoint struct {
	Endpoint[postRequest, postResponse]
	MaxBodyBytes int64
}

func (a *postEndpoint) Meta() *EndpointMeta {
	return &EndpointMeta{
		Pattern:    "POST /api/post-endpoint/{id}",
		StatusCode: http.StatusCreated,
	}
}

type postRequest struct {
	ID                string `json:"-"                   validate:"-"`
	MakeAPIError      bool   `json:"make_api_error"      validate:"-"`
	MakeInternalError bool   `json:"make_internal_error" validate:"-"`
	MakePostgresError bool   `json:"make_postgres_error" validate:"-"`
	Message           string `json:"message"             validate:"required"`
	RawPayload        []byte `json:"-"                   validate:"-"`
}

func (req *postRequest) ExtractRaw(r *http.Request) error {
	var err error
	if req.RawPayload, err = io.ReadAll(r.Body); err != nil {
		return err
	}

	req.ID = r.PathValue("id")
	return nil
}

type postResponse struct {
	ID         string          `json:"id"`
	Message    string          `json:"message"`
	RawPayload json.RawMessage `json:"raw_payload"`
}

func (a *postEndpoint) Execute(ctx context.Context, req *postRequest) (*postResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req.MakeAPIError {
		return nil, apierror.NewBadRequest("Bad request.")
	}

	if req.MakeInternalError {
		return nil, errors.New("an internal error occurred")
	}

	if req.MakePostgresError {
		// Wrap the error to make it more realistic.
		return nil, fmt.Errorf("error running Postgres query: %w", &pgconn.PgError{Code: pgerrcode.InsufficientPrivilege})
	}

	return &postResponse{ID: req.ID, Message: req.Message, RawPayload: req.RawPayload}, nil
}
