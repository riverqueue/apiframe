package apitest

import (
	"context"
	"fmt"

	"github.com/riverqueue/apiframe/apiendpoint"
	"github.com/riverqueue/apiframe/apierror"
	"github.com/riverqueue/apiframe/internal/validate"
)

// InvokeHandler invokes a service handler and returns its results.
//
// Service handlers are normal functions and can be invoked directly, but it's
// preferable to invoke them with this function because a few extra niceties are
// observed that are normally only available from the API framework:
//
//   - Incoming request structs are validated and an API error is emitted in case
//     they're invalid (any `validate` tags are checked).
//   - Outgoing response structs are validated.
//
// Sample invocation:
//
//	endpoint := &testEndpoint{}
//	resp, err := apitest.InvokeHandler(ctx, endpoint.Execute, nil, &testRequest{ReqField: "string"})
//	require.NoError(t, err)
func InvokeHandler[TReq any, TResp any](ctx context.Context, handler func(context.Context, *TReq) (*TResp, error), opts *apiendpoint.MountOpts, req *TReq) (*TResp, error) {
	if opts == nil {
		opts = &apiendpoint.MountOpts{}
	}

	validator := opts.Validator
	if validator == nil {
		validator = validate.Default
	}

	if err := validator.StructCtx(ctx, req); err != nil {
		return nil, apierror.NewBadRequest(validate.PublicFacingMessage(validator, err))
	}

	resp, err := handler(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := validator.StructCtx(ctx, resp); err != nil {
		return nil, fmt.Errorf("apitest: error validating response API resource: %w", err)
	}

	return resp, nil
}
