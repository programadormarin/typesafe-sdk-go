package typesafe

import (
	"fmt"
	"net/http"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
	"github.com/programadormarin/typesafe-sdk-go/internal/jsonutil"
)

// TypeSafeError is the base type for all errors returned by this SDK.
// Use errors.As to unwrap to a more specific type.
type TypeSafeError struct {
	Message string
}

func (e *TypeSafeError) Error() string { return e.Message }

// TypeSafeAPIError is returned for any HTTP response with a non-success status code.
type TypeSafeAPIError struct {
	TypeSafeError

	// StatusCode is the HTTP response status code.
	StatusCode int

	// Body is the raw response body bytes, or nil for an empty body.
	Body []byte

	// Headers are the HTTP response headers.
	Headers http.Header

	// RequestID is the value of the x-typesafe-request-id response header, if present.
	RequestID string

	// Endpoint is "METHOD https://..." without credentials or query params.
	Endpoint string
}

func (e *TypeSafeAPIError) Error() string {
	msg := fmt.Sprintf("%d %s", e.StatusCode, e.Message)
	if e.Endpoint != "" {
		msg = e.Endpoint + ": " + msg
	}
	if e.RequestID != "" {
		msg += fmt.Sprintf(" (request_id=%s)", e.RequestID)
	}
	return msg
}

func (e *TypeSafeAPIError) Unwrap() error { return &e.TypeSafeError }

// TypeSafeBadRequestError is returned for HTTP 400 responses.
type TypeSafeBadRequestError struct{ TypeSafeAPIError }

func (e *TypeSafeBadRequestError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafeAuthenticationError is returned for HTTP 401 responses.
type TypeSafeAuthenticationError struct{ TypeSafeAPIError }

func (e *TypeSafeAuthenticationError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafePermissionDeniedError is returned for HTTP 403 responses.
type TypeSafePermissionDeniedError struct{ TypeSafeAPIError }

func (e *TypeSafePermissionDeniedError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafeNotFoundError is returned for HTTP 404 responses.
type TypeSafeNotFoundError struct{ TypeSafeAPIError }

func (e *TypeSafeNotFoundError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafeUnprocessableEntityError is returned for HTTP 422 responses.
// This typically means a required field was missing or a question was malformed.
type TypeSafeUnprocessableEntityError struct{ TypeSafeAPIError }

func (e *TypeSafeUnprocessableEntityError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafeRateLimitError is returned for HTTP 429 responses.
type TypeSafeRateLimitError struct {
	TypeSafeAPIError

	// RetryAfterMs is the server-requested wait in milliseconds, or nil if not provided.
	RetryAfterMs *int64
}

func (e *TypeSafeRateLimitError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafeInternalServerError is returned for HTTP 5xx responses.
type TypeSafeInternalServerError struct{ TypeSafeAPIError }

func (e *TypeSafeInternalServerError) Unwrap() error { return &e.TypeSafeAPIError }

// TypeSafeConnectionError is returned when a request fails before receiving an HTTP response.
type TypeSafeConnectionError struct {
	TypeSafeError

	// Cause is the underlying network error.
	Cause error
}

func (e *TypeSafeConnectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("connection error: %v", e.Cause)
	}
	return "connection error"
}

func (e *TypeSafeConnectionError) Unwrap() error { return e.Cause }

// TypeSafeTimeoutError is returned when a request exceeds its configured timeout.
type TypeSafeTimeoutError struct {
	TypeSafeConnectionError
}

func (e *TypeSafeTimeoutError) Error() string {
	return fmt.Sprintf("request timed out: %v", e.Cause)
}

// TypeSafeResponseValidationError is returned when the server returned a 2xx status but
// the response body was missing or structurally invalid at the named field path.
type TypeSafeResponseValidationError struct {
	TypeSafeAPIError

	// FieldPath is the dotted path to the offending field, e.g. "answers.tone.confidence".
	FieldPath string
}

func (e *TypeSafeResponseValidationError) Error() string {
	return fmt.Sprintf("invalid response data at %q: %s", e.FieldPath, e.TypeSafeAPIError.Error())
}

func (e *TypeSafeResponseValidationError) Unwrap() error { return &e.TypeSafeAPIError }

// newAPIError builds the right error subtype from an HTTP status code.
func newAPIError(status int, body []byte, headers http.Header, endpoint string) error {
	msg := extractErrorMessage(body)
	if msg == "" {
		if len(body) == 0 {
			msg = "status code (no body)"
		} else {
			const maxLen = 200
			raw := string(body)
			if len(raw) > maxLen {
				raw = raw[:maxLen] + "…"
			}
			msg = raw
		}
	}

	base := TypeSafeAPIError{
		TypeSafeError: TypeSafeError{Message: msg},
		StatusCode:    status,
		Body:          body,
		Headers:       headers,
		RequestID:     headers.Get(consts.HeaderRequestID),
		Endpoint:      endpoint,
	}

	switch status {
	case http.StatusBadRequest:
		return &TypeSafeBadRequestError{TypeSafeAPIError: base}
	case http.StatusUnauthorized:
		return &TypeSafeAuthenticationError{TypeSafeAPIError: base}
	case http.StatusForbidden:
		return &TypeSafePermissionDeniedError{TypeSafeAPIError: base}
	case http.StatusNotFound:
		return &TypeSafeNotFoundError{TypeSafeAPIError: base}
	case http.StatusUnprocessableEntity:
		return &TypeSafeUnprocessableEntityError{TypeSafeAPIError: base}
	case http.StatusTooManyRequests:
		raMs := parseRetryAfterMs(headers)
		return &TypeSafeRateLimitError{TypeSafeAPIError: base, RetryAfterMs: raMs}
	default:
		if status >= 500 {
			return &TypeSafeInternalServerError{TypeSafeAPIError: base}
		}
		return &base
	}
}

// extractErrorMessage tries common error body shapes.
func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	// Try to decode as a JSON object and look for common message fields.
	// We do a minimal hand-rolled scan to avoid import cycles; the full
	// json package is used in internal/jsonutil where appropriate.
	var obj map[string]any
	if err := jsonutil.Unmarshal(body, &obj); err != nil {
		return string(body)
	}
	for _, key := range []string{"message", "error", "detail"} {
		if v, ok := obj[key]; ok {
			switch t := v.(type) {
			case string:
				return t
			case map[string]any:
				if msg, ok := t["message"].(string); ok {
					return msg
				}
			}
		}
	}
	return ""
}
