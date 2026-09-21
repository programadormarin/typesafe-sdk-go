package typesafe

import (
	"net/http"
	"time"
)

// ─── Client options ───────────────────────────────────────────────────────────

// ClientOption configures a [Client] at construction time.
// Pass one or more options to [New].
type ClientOption func(*clientConfig)

// clientConfig holds resolved client-level settings.
type clientConfig struct {
	apiKey         string
	baseURL        string
	defaultModel   string
	timeout        time.Duration
	retry          RetryPolicy
	retrySet       bool // true if caller passed WithRetry
	defaultHeaders map[string]string
	httpClient     *http.Client
}

// WithAPIKey sets the API key used for all requests.
// When not set, the client reads TYPESAFE_API_KEY from the environment.
func WithAPIKey(key string) ClientOption {
	return func(c *clientConfig) { c.apiKey = key }
}

// WithBaseURL overrides the API base URL (default: https://api.typesafe.ai).
// When not set, the client reads TYPESAFE_BASE_URL from the environment.
func WithBaseURL(u string) ClientOption {
	return func(c *clientConfig) { c.baseURL = u }
}

// WithModel sets the default model name used when no per-request model is specified.
// When not set, the client reads TYPESAFE_DEFAULT_MODEL from the environment,
// then falls back to "jev-latest".
func WithModel(model string) ClientOption {
	return func(c *clientConfig) { c.defaultModel = model }
}

// WithTimeout sets the per-request HTTP timeout (default: 10s).
// This is the timeout for each individual HTTP operation, not the total retry budget.
// Use WithRetry to control the total budget across retries.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.timeout = d }
}

// WithRetry replaces the default [RetryPolicy].
// Pass RetryPolicy{MaxRetries: 0} to disable retries entirely.
func WithRetry(p RetryPolicy) ClientOption {
	return func(c *clientConfig) {
		c.retry = p
		c.retrySet = true
	}
}

// WithHTTPClient uses an existing *http.Client instead of the SDK's default.
// The provided client's Timeout field is used as the per-request timeout unless
// WithTimeout is also provided.
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *clientConfig) { c.httpClient = h }
}

// WithHeader adds a default header sent with every request.
// Use WithRequestHeader for per-call headers.
func WithHeader(key, value string) ClientOption {
	return func(c *clientConfig) {
		if c.defaultHeaders == nil {
			c.defaultHeaders = make(map[string]string)
		}
		c.defaultHeaders[key] = value
	}
}

// ─── Per-request options ──────────────────────────────────────────────────────

// RequestOption configures a single [Client.SystemOne] or [Client.Models] call.
type RequestOption func(*requestOptions)

// requestOptions holds per-call overrides merged on top of client-level defaults.
type requestOptions struct {
	model        string
	timeout      time.Duration // zero = use client default
	timeoutSet   bool
	retry        RetryPolicy
	retrySet     bool
	extraHeaders map[string]string
}

// WithRequestModel overrides the model for a single call.
func WithRequestModel(model string) RequestOption {
	return func(o *requestOptions) { o.model = model }
}

// WithRequestTimeout overrides the HTTP timeout for a single call.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(o *requestOptions) {
		o.timeout = d
		o.timeoutSet = true
	}
}

// WithRequestRetry overrides the [RetryPolicy] for a single call.
func WithRequestRetry(p RetryPolicy) RequestOption {
	return func(o *requestOptions) {
		o.retry = p
		o.retrySet = true
	}
}

// WithRequestHeader adds or overrides a header for a single call.
func WithRequestHeader(key, value string) RequestOption {
	return func(o *requestOptions) {
		if o.extraHeaders == nil {
			o.extraHeaders = make(map[string]string)
		}
		o.extraHeaders[key] = value
	}
}
