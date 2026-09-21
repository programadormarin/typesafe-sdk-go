package typesafe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
	"github.com/programadormarin/typesafe-sdk-go/internal/jsonutil"
	"github.com/programadormarin/typesafe-sdk-go/internal/transport"
)

// Client is the entry point for interacting with the TypeSafe System One API.
// Create one with [New] and reuse it across requests; it is safe for concurrent use.
type Client struct {
	apiKey         string
	baseURL        string
	defaultModel   string
	timeout        time.Duration
	retry          RetryPolicy
	defaultHeaders map[string]string
	httpClient     *http.Client
}

// New creates a [Client] configured by the provided options.
//
// The API key is resolved in order:
//  1. [WithAPIKey] option
//  2. TYPESAFE_API_KEY environment variable
//
// Similarly, [WithBaseURL] / TYPESAFE_BASE_URL and [WithModel] / TYPESAFE_DEFAULT_MODEL
// follow the same precedence. Sensible defaults are used when neither is supplied.
//
// Returns an error if the API key cannot be resolved or is invalid.
func New(opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Resolve API key.
	apiKey := cfg.apiKey
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(EnvAPIKey))
	}
	if apiKey == "" {
		return nil, &TypeSafeError{
			Message: fmt.Sprintf(
				"no API key provided; pass WithAPIKey or set the %s environment variable",
				EnvAPIKey,
			),
		}
	}
	if !isValidAPIKey(apiKey) {
		return nil, &TypeSafeError{
			Message: "API key must contain only printable ASCII characters without whitespace",
		}
	}

	// Resolve base URL.
	baseURL := cfg.baseURL
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv(EnvBaseURL))
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Resolve default model.
	defaultModel := cfg.defaultModel
	if defaultModel == "" {
		defaultModel = strings.TrimSpace(os.Getenv(EnvDefaultModel))
	}
	if defaultModel == "" {
		defaultModel = DefaultModel
	}

	// Resolve timeout.
	timeout := cfg.timeout
	if timeout == 0 && cfg.httpClient != nil {
		timeout = cfg.httpClient.Timeout
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	// Resolve retry policy.
	retry := cfg.retry
	if !cfg.retrySet {
		retry = DefaultRetryPolicy()
	}

	// Build HTTP client.
	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else {
		// Clone so we can set Timeout without mutating the caller's client.
		clone := *httpClient
		if cfg.timeout > 0 {
			clone.Timeout = cfg.timeout
		}
		httpClient = &clone
	}

	return &Client{
		apiKey:         apiKey,
		baseURL:        baseURL,
		defaultModel:   defaultModel,
		timeout:        timeout,
		retry:          retry,
		defaultHeaders: cfg.defaultHeaders,
		httpClient:     httpClient,
	}, nil
}

// SystemOne evaluates state against a map of questions and returns typed answers.
//
// state may be a string, map, or slice — anything that encodes to a valid JSON value.
// questions must contain at least one entry; each value must be a [Noul], [Choice], or [Score].
//
// Per-call overrides (model, timeout, retry policy, extra headers) can be provided
// via [WithRequestModel], [WithRequestTimeout], [WithRequestRetry], [WithRequestHeader].
func (c *Client) SystemOne(
	ctx context.Context,
	state any,
	questions map[string]Question,
	opts ...RequestOption,
) (*SystemOneResponse, error) {
	if err := validateQuestions(questions); err != nil {
		return nil, err
	}

	ro := c.applyRequestOptions(opts)

	model := ro.model
	if model == "" {
		model = c.defaultModel
	}

	body, err := jsonutil.Marshal(map[string]any{
		"state":     state,
		"model":     model,
		"questions": questions,
	})
	if err != nil {
		return nil, &TypeSafeError{Message: fmt.Sprintf("could not encode request: %v", err)}
	}

	raw, err := c.send(ctx, requestConfig{
		method:         "POST",
		url:            c.baseURL + consts.PathSystemOne,
		body:           body,
		extraHeaders:   ro.extraHeaders,
		defaultHeaders: c.defaultHeaders,
		apiKey:         c.apiKey,
		retry:          ro.retry,
	})
	if err != nil {
		return nil, err
	}

	var resp SystemOneResponse
	if err := decodeResponse(raw, 200, nil, "POST "+c.baseURL+consts.PathSystemOne, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Models lists the models available to your account.
func (c *Client) Models(ctx context.Context, opts ...RequestOption) (*ListModelsResponse, error) {
	ro := c.applyRequestOptions(opts)

	raw, err := c.send(ctx, requestConfig{
		method:         "GET",
		url:            c.baseURL + consts.PathModels,
		extraHeaders:   ro.extraHeaders,
		defaultHeaders: c.defaultHeaders,
		apiKey:         c.apiKey,
		retry:          ro.retry,
	})
	if err != nil {
		return nil, err
	}

	var resp ListModelsResponse
	if err := decodeResponse(raw, 200, nil, "GET "+c.baseURL+consts.PathModels, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// requestConfig holds everything needed to build and send one HTTP request.
type requestConfig struct {
	method         string
	url            string
	body           []byte // nil for GET
	extraHeaders   map[string]string
	defaultHeaders map[string]string
	apiKey         string
	retry          RetryPolicy
}

// send executes one request through the retry loop and returns the raw response
// bytes, mapping transport failures and non-2xx statuses to typed errors.
func (c *Client) send(ctx context.Context, cfg requestConfig) ([]byte, error) {
	var result []byte

	err := cfg.retry.retryLoop(ctx, func(attempt int) (http.Header, error) {
		req, err := transport.BuildRequest(ctx, transport.Request{
			Method:         cfg.method,
			URL:            cfg.url,
			Body:           cfg.body,
			APIKey:         cfg.apiKey,
			DefaultHeaders: cfg.defaultHeaders,
			ExtraHeaders:   cfg.extraHeaders,
			Attempt:        attempt,
		})
		if err != nil {
			// Non-retryable: request construction failure.
			return nil, err
		}

		resp, err := transport.Do(c.httpClient, req)
		if err != nil {
			return nil, mapTransportError(err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return resp.Header, newAPIError(resp.StatusCode, resp.Body, resp.Header, resp.Endpoint)
		}

		result = resp.Body
		return resp.Header, nil
	})
	return result, err
}

// mapTransportError converts a transport-level failure into a typed SDK error.
func mapTransportError(err error) error {
	var terr *transport.Error
	if !errors.As(err, &terr) {
		return err
	}
	base := TypeSafeConnectionError{
		TypeSafeError: TypeSafeError{},
		Cause:         terr.Cause,
	}
	if terr.Kind == transport.ErrorTimeout {
		return &TypeSafeTimeoutError{TypeSafeConnectionError: base}
	}
	return &base
}

// decodeResponse unmarshals raw JSON bytes into v, wrapping decode failures in a
// TypeSafeResponseValidationError at the given fieldPath.
func decodeResponse(raw []byte, statusCode int, headers http.Header, endpoint string, v any) error {
	if err := jsonutil.Unmarshal(raw, v); err != nil {
		return &TypeSafeResponseValidationError{
			TypeSafeAPIError: TypeSafeAPIError{
				TypeSafeError: TypeSafeError{Message: fmt.Sprintf("invalid response data: %v", err)},
				StatusCode:    statusCode,
				Body:          raw,
				Headers:       headers,
				Endpoint:      endpoint,
			},
			FieldPath: ".",
		}
	}
	return nil
}

// applyRequestOptions merges per-call options on top of client defaults.
func (c *Client) applyRequestOptions(opts []RequestOption) requestOptions {
	ro := requestOptions{
		retry: c.retry,
	}
	for _, o := range opts {
		o(&ro)
	}
	// ro.retry is already set by the WithRequestRetry option when retrySet is true.
	return ro
}

// isValidAPIKey reports whether the key contains only printable ASCII without whitespace.
func isValidAPIKey(key string) bool {
	for _, r := range key {
		if r > 127 || r < 32 || r == ' ' {
			return false
		}
	}
	return true
}
