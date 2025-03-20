package apitest

import (
	"context"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"

	"github.com/riverqueue/apiframe/apiendpoint"
	"github.com/riverqueue/apiframe/apierror"
)

func TestInvokeHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	type testRequest struct {
		RequiredReqField string `json:"req_field" validate:"required"`
	}
	type testResponse struct {
		RequiredRespField string `json:"resp_field" validate:"required"`
	}

	handler := func(_ context.Context, req *testRequest) (*testResponse, error) {
		return &testResponse{RequiredRespField: "response value"}, nil
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		resp, err := InvokeHandler(ctx, handler, nil, &testRequest{RequiredReqField: "string"})
		require.NoError(t, err)
		require.Equal(t, &testResponse{RequiredRespField: "response value"}, resp)
	})

	t.Run("ValidatesRequest", func(t *testing.T) {
		t.Parallel()

		_, err := InvokeHandler(ctx, handler, nil, &testRequest{RequiredReqField: ""})
		require.Equal(t, apierror.NewBadRequestf("Field `req_field` is required."), err)
	})

	t.Run("ValidatesResponse", func(t *testing.T) {
		t.Parallel()

		handler := func(_ context.Context, _ *testRequest) (*testResponse, error) {
			return &testResponse{RequiredRespField: ""}, nil
		}

		_, err := InvokeHandler(ctx, handler, nil, &testRequest{RequiredReqField: "string"})
		require.EqualError(t, err, "apitest: error validating response API resource: Key: 'testResponse.resp_field' Error:Field validation for 'resp_field' failed on the 'required' tag")
	})

	t.Run("CustomValidator", func(t *testing.T) {
		t.Parallel()

		customValidator := validator.New()
		opts := &apiendpoint.MountOpts{Validator: customValidator}

		resp, err := InvokeHandler(ctx, handler, opts, &testRequest{RequiredReqField: "string"})
		require.NoError(t, err)
		require.Equal(t, &testResponse{RequiredRespField: "response value"}, resp)
	})
}
