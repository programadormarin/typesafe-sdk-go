package typesafe

import (
	"errors"
	"net/http"
	"testing"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
)

// ─── newAPIError dispatch ─────────────────────────────────────────────────────

func TestNewAPIError_400_isBadRequest(t *testing.T) {
	err := newAPIError(400, nil, make(http.Header), "POST /v1/systemone")
	var target *TypeSafeBadRequestError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeBadRequestError, got %T", err)
	}
}

func TestNewAPIError_401_isAuthentication(t *testing.T) {
	err := newAPIError(401, nil, make(http.Header), "")
	var target *TypeSafeAuthenticationError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeAuthenticationError, got %T", err)
	}
}

func TestNewAPIError_403_isPermissionDenied(t *testing.T) {
	err := newAPIError(403, nil, make(http.Header), "")
	var target *TypeSafePermissionDeniedError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafePermissionDeniedError, got %T", err)
	}
}

func TestNewAPIError_404_isNotFound(t *testing.T) {
	err := newAPIError(404, nil, make(http.Header), "")
	var target *TypeSafeNotFoundError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeNotFoundError, got %T", err)
	}
}

func TestNewAPIError_422_isUnprocessable(t *testing.T) {
	err := newAPIError(422, nil, make(http.Header), "")
	var target *TypeSafeUnprocessableEntityError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeUnprocessableEntityError, got %T", err)
	}
}

func TestNewAPIError_429_isRateLimit(t *testing.T) {
	err := newAPIError(429, nil, make(http.Header), "")
	var target *TypeSafeRateLimitError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeRateLimitError, got %T", err)
	}
}

func TestNewAPIError_500_isInternalServer(t *testing.T) {
	err := newAPIError(500, nil, make(http.Header), "")
	var target *TypeSafeInternalServerError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeInternalServerError, got %T", err)
	}
}

func TestNewAPIError_529_isInternalServer(t *testing.T) {
	err := newAPIError(529, nil, make(http.Header), "")
	var target *TypeSafeInternalServerError
	if !errors.As(err, &target) {
		t.Errorf("expected *TypeSafeInternalServerError for 529, got %T", err)
	}
}

// ─── Unwrap chain ─────────────────────────────────────────────────────────────

func TestAPIError_unwrapsToAPIError(t *testing.T) {
	err := newAPIError(429, nil, make(http.Header), "")
	var apiErr *TypeSafeAPIError
	if !errors.As(err, &apiErr) {
		t.Errorf("TypeSafeRateLimitError should unwrap to *TypeSafeAPIError, got %T", err)
	}
}

func TestAPIError_unwrapsToBaseError(t *testing.T) {
	err := newAPIError(500, nil, make(http.Header), "")
	var baseErr *TypeSafeError
	if !errors.As(err, &baseErr) {
		t.Errorf("TypeSafeInternalServerError should unwrap to *TypeSafeError, got %T", err)
	}
}

// ─── Error message formatting ─────────────────────────────────────────────────

func TestAPIError_includesStatusAndMessage(t *testing.T) {
	body := []byte(`{"message": "invalid API key"}`)
	err := newAPIError(401, body, make(http.Header), "POST /v1/systemone")
	msg := err.Error()
	for _, want := range []string{"401", "invalid API key", "POST /v1/systemone"} {
		if !contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestAPIError_includesRequestID(t *testing.T) {
	h := make(http.Header)
	h.Set(consts.HeaderRequestID, "req-abc123")
	err := newAPIError(500, nil, h, "")
	if !contains(err.Error(), "req-abc123") {
		t.Errorf("error message %q should contain request_id", err.Error())
	}
}

func TestRateLimitError_retryAfterMs(t *testing.T) {
	h := make(http.Header)
	h.Set(consts.HeaderRetryAfterMs, "2000")
	err := newAPIError(429, nil, h, "")
	var rl *TypeSafeRateLimitError
	if !errors.As(err, &rl) {
		t.Fatal("expected TypeSafeRateLimitError")
	}
	if rl.RetryAfterMs == nil || *rl.RetryAfterMs != 2000 {
		t.Errorf("RetryAfterMs = %v, want 2000", rl.RetryAfterMs)
	}
}

// ─── extractErrorMessage ─────────────────────────────────────────────────────

func TestExtractErrorMessage_messageField(t *testing.T) {
	body := []byte(`{"message": "something went wrong"}`)
	if got := extractErrorMessage(body); got != "something went wrong" {
		t.Errorf("got %q, want %q", got, "something went wrong")
	}
}

func TestExtractErrorMessage_errorStringField(t *testing.T) {
	body := []byte(`{"error": "bad request"}`)
	if got := extractErrorMessage(body); got != "bad request" {
		t.Errorf("got %q, want %q", got, "bad request")
	}
}

func TestExtractErrorMessage_errorObjectField(t *testing.T) {
	body := []byte(`{"error": {"message": "nested message"}}`)
	if got := extractErrorMessage(body); got != "nested message" {
		t.Errorf("got %q, want %q", got, "nested message")
	}
}

func TestExtractErrorMessage_emptyBody(t *testing.T) {
	if got := extractErrorMessage(nil); got != "" {
		t.Errorf("got %q, want empty for nil body", got)
	}
}

// ─── TypeSafeConnectionError ──────────────────────────────────────────────────

func TestConnectionError_errorMessage(t *testing.T) {
	err := &TypeSafeConnectionError{Cause: errors.New("dial tcp: connection refused")}
	if !contains(err.Error(), "connection refused") {
		t.Errorf("error message %q should mention cause", err.Error())
	}
}

func TestTimeoutError_errorMessage(t *testing.T) {
	err := &TypeSafeTimeoutError{
		TypeSafeConnectionError: TypeSafeConnectionError{Cause: errors.New("context deadline exceeded")},
	}
	if !contains(err.Error(), "timed out") {
		t.Errorf("error message %q should mention timed out", err.Error())
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
