package typesafe

import (
	"log/slog"
	"net/http"
	"time"
)

// ClientOption functional option.
type ClientOption func(*clientConfig)

type clientConfig struct {
	apiKey         string
	baseURL        string
	defaultModel   string
	timeout        time.Duration
	retry          RetryPolicy
	retrySet       bool
	defaultHeaders map[string]string
	headers        http.Header
	httpClient     *http.Client
	logger         *slog.Logger
	compression    Compression
}

// Config fields covered above.

// Functional options.
func WithAPIKey(key string) ClientOption { return func(c *clientConfig) { c.apiKey = key } }
func WithBaseURL(u string) ClientOption { return func(c *clientConfig) { c.baseURL = u } }
func WithModel(model string) ClientOption { return func(c *clientConfig) { c.defaultModel = model } }
func WithTimeout(d time.Duration) ClientOption { return func(c *clientConfig) { c.timeout = d } }
func WithRetry(p RetryPolicy) ClientOption {
	return func(c *clientConfig) { c.retry = p; c.retrySet = true }
}
func WithHTTPClient(h *http.Client) ClientOption { return func(c *clientConfig) { c.httpClient = h } }
func WithHeader(key, value string) ClientOption {
	return func(c *clientConfig) {
		if c.defaultHeaders == nil { c.defaultHeaders = make(map[string]string) }
		c.defaultHeaders[key] = value
	}
}
func WithHeaders(h http.Header) ClientOption {
	return func(c *clientConfig) {
		if c.headers == nil { c.headers = make(http.Header) }
		for k, vs := range h {
			for _, v := range vs { c.headers.Add(k, v) }
		}
	}
}
func WithLogger(l *slog.Logger) ClientOption { return func(c *clientConfig) { c.logger = l } }
func WithCompression(comp Compression) ClientOption { return func(c *clientConfig) { c.compression = comp } }

// WithRequest* options.
type RequestOption func(*requestOptions)

type requestOptions struct {
	model        string
	modelSet     bool
	timeout      time.Duration
	timeoutSet   bool
	retry        RetryPolicy
	retrySet     bool
	headers      http.Header
	assets       map[string]*Image
	extraBody    map[string]any
}

func WithRequestModel(model string) RequestOption {
	return func(o *requestOptions) { o.model = model; o.modelSet = true }
}
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(o *requestOptions) { o.timeout = d; o.timeoutSet = true }
}
func WithRequestRetry(p RetryPolicy) RequestOption {
	return func(o *requestOptions) { o.retry = p; o.retrySet = true }
}
func WithRequestHeader(key, value string) RequestOption {
	return func(o *requestOptions) {
		if o.headers == nil { o.headers = make(http.Header) }
		o.headers.Set(key, value)
	}
}
func WithRequestHeaders(h http.Header) RequestOption {
	return func(o *requestOptions) {
		if o.headers == nil { o.headers = make(http.Header) }
		for k, vs := range h {
			for _, v := range vs { o.headers.Add(k, v) }
		}
	}
}
func WithRequestAssets(a map[string]*Image) RequestOption { return func(o *requestOptions) { o.assets = a } }
func WithRequestExtraBody(b map[string]any) RequestOption { return func(o *requestOptions) { o.extraBody = b } }

// normalizeRequestOptions handles legacy RequestOption funcs and valksor RequestOptions structs.
func normalizeRequestOptions(c *Client, opts []any) requestOptions {
	ro := requestOptions{retry: c.retry}
	for _, o := range opts {
		switch v := o.(type) {
		case RequestOption:
			v(&ro)
		case func(*requestOptions):
			v(&ro)
		case RequestOptions:
			if v.Timeout != 0 { ro.timeout = v.Timeout; ro.timeoutSet = true }
			if v.Retry != nil { ro.retry = *v.Retry; ro.retrySet = true }
			if v.Headers != nil {
				if ro.headers == nil { ro.headers = make(http.Header) }
				for k, vs := range v.Headers {
					for _, val := range vs { ro.headers.Add(k, val) }
				}
			}
		case *RequestOptions:
			if v != nil {
				// recurse via value
				ro2 := normalizeRequestOptions(c, []any{*v})
				// merge
				if ro2.timeoutSet { ro.timeout = ro2.timeout; ro.timeoutSet = true }
				if ro2.retrySet { ro.retry = ro2.retry; ro.retrySet = true }
				if ro2.headers != nil {
					if ro.headers == nil { ro.headers = make(http.Header) }
					for k, vs := range ro2.headers { for _, val := range vs { ro.headers.Add(k, val) } }
				}
			}
		default:
			// ignore
		}
	}
	return ro
}
