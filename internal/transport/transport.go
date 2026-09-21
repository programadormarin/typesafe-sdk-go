// Package transport implements the low-level HTTP layer for the TypeSafe SDK.
// It builds requests, sends them, and returns raw responses without any
// knowledge of the SDK's public types.
package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
)

// sdkVersion is embedded in the User-Agent and X-TypeSafe-SDK headers. It
// defaults to the current release version and can be overridden at build time
// via -ldflags "-X github.com/programadormarin/typesafe-sdk-go/internal/transport.sdkVersion=dev".
var sdkVersion = consts.Version

// runtimeHeader is the X-TypeSafe-Runtime header value, computed once at init.
var runtimeHeader = fmt.Sprintf("go/%s (%s; %s)", runtime.Version()[2:], runtime.GOOS, runtime.GOARCH)

// Request describes a single HTTP request to build and send.
type Request struct {
	// Method is the HTTP method, e.g. "GET" or "POST".
	Method string

	// URL is the full request URL.
	URL string

	// Body is the request body, or nil when there is no body.
	Body []byte

	// APIKey is the bearer token used for authorization.
	APIKey string

	// DefaultHeaders are client-level headers applied to every request.
	DefaultHeaders map[string]string

	// ExtraHeaders are per-request headers with the highest precedence.
	ExtraHeaders map[string]string

	// Attempt is the 0-indexed retry attempt; the retry-count header is
	// injected from attempt 1 onwards.
	Attempt int
}

// Response is a raw HTTP response.
type Response struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Header contains the response headers.
	Header http.Header

	// Body is the raw response body bytes.
	Body []byte

	// Endpoint is "METHOD https://..." without credentials or query params.
	Endpoint string
}

// ErrorKind classifies a transport-level failure.
type ErrorKind int

const (
	// ErrorConnection indicates a generic network failure.
	ErrorConnection ErrorKind = iota

	// ErrorTimeout indicates the request exceeded its deadline.
	ErrorTimeout
)

// Error is returned when a request fails before an HTTP response is received.
type Error struct {
	// Kind classifies the failure.
	Kind ErrorKind

	// Cause is the underlying error.
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("transport error: %v", e.Cause)
	}
	return "transport error"
}

func (e *Error) Unwrap() error { return e.Cause }

// BuildRequest constructs an *http.Request from a Request.
// attempt is the 0-indexed retry attempt; the retry-count header is injected
// from attempt 1.
func BuildRequest(ctx context.Context, req Request) (*http.Request, error) {
	var bodyReader io.Reader
	if req.Body != nil {
		bodyReader = bytes.NewReader(req.Body)
	}

	hreq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("could not create HTTP request: %w", err)
	}

	// Required headers.
	hreq.Header.Set(consts.HeaderAuthorization, "Bearer "+req.APIKey)
	hreq.Header.Set(consts.HeaderAccept, consts.ContentTypeJSON)
	hreq.Header.Set(consts.HeaderUserAgent, consts.SDKName+"/"+sdkVersion)
	hreq.Header.Set(consts.HeaderSDK, consts.SDKName+"/"+sdkVersion)
	hreq.Header.Set(consts.HeaderRuntime, runtimeHeader)

	if req.Body != nil {
		hreq.Header.Set(consts.HeaderContentType, consts.ContentTypeJSON)
		hreq.ContentLength = int64(len(req.Body))
	}

	// Retry count header (omitted on first attempt).
	if req.Attempt > 0 {
		hreq.Header.Set(consts.HeaderRetryCount, strconv.Itoa(req.Attempt))
	}

	// Client-level default headers (may be overridden by per-request headers).
	for k, v := range req.DefaultHeaders {
		hreq.Header.Set(k, v)
	}
	// Per-request extra headers (highest precedence).
	for k, v := range req.ExtraHeaders {
		hreq.Header.Set(k, v)
	}
	// Never allow callers to override the auth header via extra headers.
	hreq.Header.Set(consts.HeaderAuthorization, "Bearer "+req.APIKey)

	return hreq, nil
}

// Do sends the request and returns the raw response. It returns a *Error when
// no HTTP response is received.
func Do(httpClient *http.Client, req *http.Request) (*Response, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		// Distinguish timeouts from generic connection failures.
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return nil, &Error{Kind: ErrorTimeout, Cause: err}
		}
		return nil, &Error{Kind: ErrorConnection, Cause: err}
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Kind: ErrorConnection, Cause: err}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
		Endpoint:   endpointDesc(req),
	}, nil
}

// isTimeoutError reports whether err (from http.Client.Do) represents a timeout.
func isTimeoutError(err error) bool {
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}

// endpointDesc formats "METHOD https://host/path" without credentials for error messages.
func endpointDesc(req *http.Request) string {
	if req == nil {
		return ""
	}
	u := *req.URL
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return req.Method + " " + u.String()
}
