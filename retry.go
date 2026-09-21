package typesafe

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
)

// RetryPolicy controls how the SDK retries failed requests.
// Use [DefaultRetryPolicy] to get sensible defaults, then modify fields as needed.
type RetryPolicy struct {
	// MaxRetries is the maximum number of additional attempts after the first.
	// 0 disables retries.
	MaxRetries int

	// BackoffInitial is the delay before the first retry.
	// It doubles each attempt up to BackoffMax.
	BackoffInitial time.Duration

	// BackoffMax caps the per-attempt delay.
	BackoffMax time.Duration

	// BackoffJitter is the fraction of each backoff delay randomly subtracted (0–1).
	BackoffJitter float64

	// HTTPStatuses is the set of HTTP status codes that trigger a retry.
	// Defaults to 408, 429, and all 5xx.
	HTTPStatuses map[int]bool

	// RespectRetryAfter, when true, honours the Retry-After / Retry-After-Ms
	// response headers and uses their value as the retry delay.
	RespectRetryAfter bool

	// RetryOnTimeout retries requests that failed due to a timeout.
	RetryOnTimeout bool

	// RetryOnConnErr retries requests that failed due to a connection error.
	RetryOnConnErr bool

	// Timeout is the total time budget for all attempts including delays.
	// Zero means no total budget (individual request timeouts still apply).
	Timeout time.Duration
}

// DefaultRetryPolicy returns a RetryPolicy with sensible defaults:
//   - 2 retries
//   - 500ms initial backoff, 5s cap, 25% jitter
//   - Retries on 408, 429 and all 5xx
//   - Respects Retry-After header
//   - Retries on timeout and connection errors
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        2,
		BackoffInitial:    500 * time.Millisecond,
		BackoffMax:        5 * time.Second,
		BackoffJitter:     0.25,
		HTTPStatuses:      defaultRetryStatuses(),
		RespectRetryAfter: true,
		RetryOnTimeout:    true,
		RetryOnConnErr:    true,
		Timeout:           30 * time.Second,
	}
}

func defaultRetryStatuses() map[int]bool {
	m := map[int]bool{408: true, 429: true}
	for s := 500; s < 600; s++ {
		m[s] = true
	}
	return m
}

// isRetryable reports whether err should trigger another attempt.
func (p RetryPolicy) isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *TypeSafeAPIError
	if errors.As(err, &apiErr) {
		return p.HTTPStatuses[apiErr.StatusCode]
	}
	var timeoutErr *TypeSafeTimeoutError
	if errors.As(err, &timeoutErr) {
		return p.RetryOnTimeout
	}
	var connErr *TypeSafeConnectionError
	if errors.As(err, &connErr) {
		return p.RetryOnConnErr
	}
	return false
}

// backoffDelay returns the delay for a given attempt number (0-indexed after first).
func (p RetryPolicy) backoffDelay(attempt int) time.Duration {
	if p.BackoffInitial == 0 || p.BackoffMax == 0 {
		return 0
	}
	// Exponential: initial * 2^attempt, capped at max.
	exp := float64(p.BackoffInitial) * math.Pow(2, float64(attempt))
	if exp > float64(p.BackoffMax) {
		exp = float64(p.BackoffMax)
	}
	// Subtract a random jitter fraction.
	jitter := rand.Float64() * p.BackoffJitter //nolint:gosec // backoff jitter, not crypto
	delay := time.Duration(exp * (1 - jitter))
	if delay < 0 {
		return 0
	}
	return delay
}

// retryDelay returns the wait time before the next attempt.
// It prefers the server's Retry-After header when RespectRetryAfter is set.
func (p RetryPolicy) retryDelay(attempt int, headers http.Header) time.Duration {
	if p.RespectRetryAfter && headers != nil {
		if ms := parseRetryAfterMs(headers); ms != nil {
			return time.Duration(*ms) * time.Millisecond
		}
	}
	return p.backoffDelay(attempt)
}

// parseRetryAfterMs reads the Retry-After-Ms or Retry-After header and returns
// the requested wait in milliseconds, or nil if the header is absent/unparseable.
func parseRetryAfterMs(headers http.Header) *int64 {
	// Prefer the ms-precision header.
	if raw := headers.Get(consts.HeaderRetryAfterMs); raw != "" {
		if ms, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && ms >= 0 {
			return &ms
		}
	}
	// Fall back to the standard Retry-After header (integer seconds).
	if raw := headers.Get(consts.HeaderRetryAfter); raw != "" {
		if secs, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && secs >= 0 {
			ms := secs * 1000
			return &ms
		}
	}
	return nil
}

// retryLoop executes fn with retry logic according to p.
// attempt is 0 for the first call, incremented for each retry.
// The context deadline/cancellation is respected on every sleep.
func (p RetryPolicy) retryLoop(ctx context.Context, fn func(attempt int) (http.Header, error)) error {
	var budgetDeadline time.Time
	if p.Timeout > 0 {
		budgetDeadline = time.Now().Add(p.Timeout)
	}

	var lastErr error
	var lastHeaders http.Header

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return &TypeSafeConnectionError{TypeSafeError: TypeSafeError{Message: "request cancelled"}, Cause: err}
		}

		headers, err := fn(attempt)
		if err == nil {
			return nil
		}
		lastErr = err
		lastHeaders = headers

		if attempt >= p.MaxRetries {
			return lastErr
		}
		if !p.isRetryable(err) {
			return lastErr
		}

		delay := p.retryDelay(attempt, lastHeaders)

		// Stop if sleeping would exceed the total budget.
		if !budgetDeadline.IsZero() && time.Now().Add(delay).After(budgetDeadline) {
			return lastErr
		}

		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(delay):
		}
	}
}
