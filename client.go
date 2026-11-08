package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultTimeout = 10 * time.Second

// Config configures a Client for NewClient. Empty values fall back to
// environment variables and then SDK defaults.
type Config struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
	Retry        *RetryPolicy
	Headers      http.Header
	HTTPClient   *http.Client
	Logger       *slog.Logger
	Compression  Compression
}

// SystemOneRequest is defined in request.go.

// ModelsMethod is a callable type that also supports the valksor-style .List method.
type ModelsMethod func(context.Context, ...any) (*ListModelsResponse, error)

// List delegates to the underlying client method.
func (m ModelsMethod) List(ctx context.Context, opts ...any) (*ListModelsResponse, error) {
	return m(ctx, opts...)
}

// Client is the entry point.
type Client struct {
	apiKey       string
	baseURL      string
	defaultModel string
	timeout      time.Duration
	retry        RetryPolicy
	headers          http.Header
	defaultHeaders  map[string]string
	httpClient       *http.Client
	logger       *slog.Logger
	compress     bool

	attachedMu sync.Mutex
	attached   map[string]*attachedImage

	Models ModelsMethod
}

func newClient(config Config) (*Client, error) {
	apiKey := firstNonBlank(config.APIKey, os.Getenv(EnvAPIKey))
	if apiKey == "" {
		return nil, &TypeSafeError{Message: fmt.Sprintf("no API key provided; set Config.APIKey or %s", EnvAPIKey), Cause: ErrInvalidRequest}
	}
	if err := validateAPIKey(apiKey); err != nil {
		return nil, err
	}
	baseURL := firstNonBlank(config.BaseURL, os.Getenv(EnvBaseURL), DefaultBaseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid TypeSafe base URL %q: %w", baseURL, ErrInvalidRequest)
	}
	model := firstNonBlank(config.DefaultModel, os.Getenv(EnvDefaultModel), DefaultModel)
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("timeout must be positive: %w", ErrInvalidRequest)
	}
	retry := DefaultRetryPolicy()
	if config.Retry != nil {
		retry = cloneRetryPolicy(*config.Retry)
	}
	if err := retry.validate(); err != nil {
		return nil, err
	}
	if err := config.Compression.validate(); err != nil {
		return nil, err
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		transport := pooledTransport()
		httpClient = &http.Client{Transport: transport, Timeout: timeout}
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	client := &Client{
		apiKey:       apiKey,
		baseURL:      baseURL,
		defaultModel: model,
		timeout:      timeout,
		retry:        retry,
		headers:      config.Headers.Clone(),
		httpClient:   httpClient,
		logger:       logger,
		compress:     config.Compression != CompressionNone,
		attached:     make(map[string]*attachedImage),
	}
	client.Models = ModelsMethod(func(ctx context.Context, opts ...any) (*ListModelsResponse, error) {
		return client.models(ctx, opts...)
	})
	return client, nil
}

// NewClient constructs from optional Config values (at most one).
func NewClient(configs ...Config) (*Client, error) {
	if len(configs) > 1 {
		return nil, fmt.Errorf("NewClient accepts at most one Config: %w", ErrInvalidRequest)
	}
	var config Config
	if len(configs) == 1 {
		config = configs[0]
	}
	client, err := newClient(config)
	return client, err
}

// New is the legacy functional constructor.
func New(opts ...ClientOption) (*Client, error) {
	cfg := clientConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	config := Config{
		APIKey:       cfg.apiKey,
		BaseURL:      cfg.baseURL,
		DefaultModel: cfg.defaultModel,
		Timeout:      cfg.timeout,
		Headers:      cfg.headers,
		HTTPClient:   cfg.httpClient,
		Logger:       cfg.logger,
		Compression:  cfg.compression,
	}
	if cfg.retrySet {
		p := cfg.retry
		config.Retry = &p
	}
	// merge map default headers into Headers
	if cfg.defaultHeaders != nil {
		if config.Headers == nil {
			config.Headers = http.Header{}
		}
		for k, v := range cfg.defaultHeaders {
			config.Headers.Set(k, v)
		}
	}
	client, err := newClient(config)
	if err == nil && client != nil {
		client.defaultHeaders = cfg.defaultHeaders
	}
	return client, err
}

// ModelsService defined in models.go? Add here.
type ModelsService struct {
	client *Client
}

func (m *ModelsService) List(ctx context.Context, opts ...any) (*ListModelsResponse, error) {
	return m.client.Models(ctx, opts...)
}

func pooledTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil
	}
	transport := base.Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 100
	transport.MaxConnsPerHost = 100
	transport.IdleConnTimeout = 30 * time.Second
	return transport
}

func (c *Client) BaseURL() string     { return c.baseURL }
func (c *Client) DefaultModel() string { return c.defaultModel }

// AttachAssets defined in assets.go.

// SystemOne flexible entrypoint.
func (c *Client) SystemOne(ctx context.Context, args ...any) (*SystemOneResponse, error) {
	var request SystemOneRequest
	var opts []any
	if len(args) == 0 {
		return nil, fmt.Errorf("SystemOne requires state or request: %w", ErrInvalidRequest)
	}
	if r, ok := args[0].(SystemOneRequest); ok {
		request = r
		opts = args[1:]
	} else {
		if len(args) < 2 {
			return nil, fmt.Errorf("SystemOne requires state and questions: %w", ErrInvalidRequest)
		}
		request.State = args[0]
		q, ok := args[1].(map[string]Question)
		if !ok {
			return nil, fmt.Errorf("questions must be map[string]Question: %w", ErrInvalidRequest)
		}
		request.Questions = q
		opts = args[2:]
	}
	body, meta, err := c.systemOneRaw(ctx, request, opts...)
	if err != nil {
		return nil, err
	}
	resp, err := decodeSystemOne(body)
	if err != nil {
		return nil, &TypeSafeResponseValidationError{
			TypeSafeAPIError: TypeSafeAPIError{
				TypeSafeError: TypeSafeError{Message: fmt.Sprintf("invalid response data: %v", err)},
				StatusCode:    meta.StatusCode,
				Body:          body,
				Headers:       meta.Headers,
				Endpoint:      c.baseURL + "/v1/systemone",
				RequestID:     meta.RequestID,
			},
			FieldPath: ".",
		}
	}
	resp.Meta = meta
	return resp, nil
}

// Models lists models.
func (c *Client) models(ctx context.Context, opts ...any) (*ListModelsResponse, error) {
	raw, meta, err := c.request(ctx, http.MethodGet, "/v1/models", nil, "", opts...)
	if err != nil {
		return nil, err
	}
	var resp ListModelsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, &TypeSafeResponseValidationError{TypeSafeAPIError: TypeSafeAPIError{TypeSafeError: TypeSafeError{Message: err.Error()}, StatusCode: meta.StatusCode, Body: raw, Headers: meta.Headers, RequestID: meta.RequestID}, FieldPath: "."}
	}
	return &resp, nil
}

func (c *Client) systemOneRaw(ctx context.Context, request SystemOneRequest, opts ...any) ([]byte, ResponseMeta, error) {
	if request.State == nil {
		return nil, ResponseMeta{}, fmt.Errorf("state is required: %w", ErrInvalidRequest)
	}
	if err := validateQuestions(request.Questions); err != nil {
		return nil, ResponseMeta{}, err
	}
	ro := normalizeRequestOptions(c, opts)
	if ro.modelSet && strings.TrimSpace(ro.model) != "" {
		request.Model = ro.model
	}
	if ro.assets != nil {
		if request.Assets == nil { request.Assets = map[string]*Image{} }
		for k, v := range ro.assets { request.Assets[k] = v }
	}
	if ro.extraBody != nil {
		if request.ExtraBody == nil { request.ExtraBody = map[string]any{} }
		for k, v := range ro.extraBody { request.ExtraBody[k] = v }
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = c.defaultModel
	}
	payload := make(map[string]any, len(request.ExtraBody)+3)
	for key, value := range request.ExtraBody {
		payload[key] = sanitizeNonFinite(value)
	}
	payload["state"] = sanitizeNonFinite(request.State)
	payload["questions"] = request.Questions
	payload["model"] = model
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("encode request body: %w", err)
	}
	body, contentType, err := c.withAssets(encoded, request)
	if err != nil {
		return nil, ResponseMeta{}, err
	}
	return c.request(ctx, http.MethodPost, "/v1/systemone", body, contentType, opts...)
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, contentType string, opts ...any) ([]byte, ResponseMeta, error) {
	ro := normalizeRequestOptions(c, opts)
	if ro.timeout < 0 {
		return nil, ResponseMeta{}, fmt.Errorf("timeout must be positive: %w", ErrInvalidRequest)
	}
	timeout := c.timeout
	if ro.timeout != 0 {
		timeout = ro.timeout
	}
	retry := cloneRetryPolicy(c.retry)
	if ro.retrySet {
		retry = cloneRetryPolicy(ro.retry)
	}
	if err := retry.validate(); err != nil {
		return nil, ResponseMeta{}, err
	}
	headers := c.headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	if ro.headers != nil {
		for k, vs := range ro.headers {
			headers.Del(k)
			for _, v := range vs {
				headers.Add(k, v)
			}
		}
	}
	// ro.extraHeaders stored as map[string]string in legacy; reconcile? We'll support ro.extraHeadersMap
	headers.Set("Authorization", "Bearer "+c.apiKey)
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", "typesafe-sdk-go/"+Version)
	headers.Set("X-TypeSafe-SDK", "typesafe-sdk-go/"+Version)
	headers.Set("X-TypeSafe-Runtime", fmt.Sprintf("go/%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH))
	if body != nil {
		if contentType != "" {
			headers.Set("Content-Type", contentType)
		}
		if c.compress && len(body) >= minCompressedBodyLength {
			body = zstdCompress(body)
			headers.Set("Content-Encoding", string(CompressionZstd))
		}
	}
	headers.Del("X-TypeSafe-Retry-Count")

	var result []byte
	var meta ResponseMeta
	err := retry.retryLoop(ctx, func(attempt int) (http.Header, error) {
		attemptHeaders := headers.Clone()
		if attempt > 0 {
			attemptHeaders.Set("X-TypeSafe-Retry-Count", strconv.Itoa(attempt))
		}
		data, m, rerr := c.attempt(ctx, timeout, method, c.baseURL+path, body, attemptHeaders)
		meta = m
		if rerr == nil {
			result = data
			return m.Headers, nil
		}
		return m.Headers, rerr
	})
	if err != nil {
		// context cancellation override
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, meta, &TypeSafeUserAbortError{Cause: ctx.Err()}
			}
		}
		return nil, meta, err
	}
	return result, meta, nil
}

func (c *Client) attempt(ctx context.Context, timeout time.Duration, method, target string, body []byte, headers http.Header) ([]byte, ResponseMeta, error) {
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx2, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("build HTTP request: %w", err)
	}
	req.Header = headers
	started := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ResponseMeta{}, &TypeSafeUserAbortError{Cause: ctx.Err()}
		}
		if errors.Is(ctx2.Err(), context.DeadlineExceeded) {
			return nil, ResponseMeta{}, &TypeSafeTimeoutError{TypeSafeConnectionError: TypeSafeConnectionError{TypeSafeError: TypeSafeError{}, Cause: err}}
		}
		return nil, ResponseMeta{}, &TypeSafeConnectionError{TypeSafeError: TypeSafeError{}, Cause: err}
	}
	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	meta := ResponseMeta{StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), RequestID: resp.Header.Get("x-typesafe-request-id")}
	c.logger.Info("request completed", "status", resp.StatusCode, "duration", time.Since(started), "request_id", meta.RequestID)
	if readErr != nil {
		return nil, meta, &TypeSafeConnectionError{TypeSafeError: TypeSafeError{}, Cause: readErr}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, meta, newAPIError(resp.StatusCode, data, resp.Header, req.Method+" "+target)
	}
	return data, meta, nil
}

func validateAPIKey(key string) error {
	for _, r := range key {
		if r > 127 || r == ' ' || r < 32 {
			return fmt.Errorf("API key must contain only printable ASCII characters without whitespace: %w", ErrInvalidRequest)
		}
	}
	return nil
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func cloneRetryPolicy(p RetryPolicy) RetryPolicy {
	// clone map
	m := make(map[int]bool, len(p.HTTPStatuses))
	for k, v := range p.HTTPStatuses {
		m[k] = v
	}
	p.HTTPStatuses = m
	return p
}

func decodeSystemOne(body []byte) (*SystemOneResponse, error) {
	var resp SystemOneResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func SystemOneAs[T any](ctx context.Context, client *Client, request SystemOneRequest, opts ...any) (*T, error) {
	data, meta, err := client.systemOneRaw(ctx, request, opts...)
	if err != nil {
		return nil, err
	}
	// lift answers
	lifted, err := liftAnswers(data)
	if err != nil {
		return nil, &TypeSafeResponseValidationError{TypeSafeAPIError: TypeSafeAPIError{TypeSafeError: TypeSafeError{Message: err.Error()}, StatusCode: meta.StatusCode, Body: data, Headers: meta.Headers, RequestID: meta.RequestID}, FieldPath: "."}
	}
	result := new(T)
	if err := json.Unmarshal(lifted, result); err != nil {
		return nil, &TypeSafeResponseValidationError{TypeSafeAPIError: TypeSafeAPIError{TypeSafeError: TypeSafeError{Message: validationCause(err).Error()}, StatusCode: meta.StatusCode, Body: data, Headers: meta.Headers, RequestID: meta.RequestID}, FieldPath: "."}
	}
	return result, nil
}
