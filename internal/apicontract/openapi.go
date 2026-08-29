package apicontract

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

// Load loads the required data.
func Load(ctx context.Context, path string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI document: %w", err)
	}
	if err := document.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate OpenAPI document: %w", err)
	}
	return document, nil
}

type Validator struct {
	router  routers.Router
	options openapi3filter.Options
}

// NewValidator creates a validator.
func NewValidator(document *openapi3.T) (*Validator, error) {
	router, err := legacy.NewRouter(document)
	if err != nil {
		return nil, fmt.Errorf("build OpenAPI router: %w", err)
	}
	return &Validator{
		router: router,
		options: openapi3filter.Options{
			AuthenticationFunc:    openapi3filter.NoopAuthenticationFunc,
			IncludeResponseStatus: true,
		},
	}, nil
}

type Operation struct {
	input *openapi3filter.RequestValidationInput
}

// OperationID operations id.
func (operation *Operation) OperationID() string {
	return operation.input.Route.Operation.OperationID
}

// Match handles the match operation.
func (validator *Validator) Match(request *http.Request) (*Operation, error) {
	route, parameters, err := validator.router.FindRoute(request)
	if err != nil {
		return nil, fmt.Errorf("match OpenAPI operation: %w", err)
	}
	return &Operation{input: &openapi3filter.RequestValidationInput{
		Request: request, PathParams: parameters, Route: route, Options: &validator.options,
	}}, nil
}

// ValidateRequest validates request.
func (operation *Operation) ValidateRequest(ctx context.Context) error {
	if operation.input.Route.Operation.RequestBody == nil && operation.input.Request.ContentLength > 0 {
		return errors.New("validate OpenAPI request: request body is not documented")
	}
	if err := openapi3filter.ValidateRequest(ctx, operation.input); err != nil {
		return fmt.Errorf("validate OpenAPI request: %w", err)
	}
	return nil
}

// ValidateResponse validates response.
func (operation *Operation) ValidateResponse(
	ctx context.Context,
	status int,
	header http.Header,
	body []byte,
) error {
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: operation.input,
		Status:                 status,
		Header:                 header,
		Options:                operation.input.Options,
	}
	input.SetBodyBytes(body)
	if err := openapi3filter.ValidateResponse(ctx, input); err != nil {
		return fmt.Errorf("validate OpenAPI response: %w", err)
	}
	return nil
}
