package typesafe

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
)

// ─── isRetryable ──────────────────────────────────────────────────────────────

func TestRetryPolicy_isRetryable_429(t *testing.T) {
	p := DefaultRetryPolicy()
	err := newAPIError(429, nil, make(http.Header), "")
	if !p.isRetryable(err) {
		t.Error("429 should be retryable")
	}
}

func TestRetryPolicy_isRetryable_500(t *testing.T) {
	p := DefaultRetryPolicy()
	err := newAPIError(500, nil, make(http.Header), "")
	if !p.isRetryable(err) {
		t.Error("500 should be retryable")
	}
}

func TestRetryPolicy_isRetryable_401(t *testing.T) {
	p := DefaultRetryPolicy()
	err := newAPIError(401, nil, make(http.Header), "")
	if p.isRetryable(err) {
		t.Error("401 should NOT be retryable")
	}
}

func TestRetryPolicy_isRetryable_422(t *testing.T) {
	p := DefaultRetryPolicy()
	err := newAPIError(422, nil, make(http.Header), "")
	if p.isRetryable(err) {
		t.Error("422 should NOT be retryable")
	}
}

func TestRetryPolicy_isRetryable_timeoutError(t *testing.T) {
	p := DefaultRetryPolicy()
	err := &TypeSafeTimeoutError{TypeSafeConnectionError: TypeSafeConnectionError{Cause: errors.New("deadline exceeded")}}
	if !p.isRetryable(err) {
		t.Error("timeout error should be retryable by default")
	}
}

func TestRetryPolicy_isRetryable_timeoutDisabled(t *testing.T) {
	p := DefaultRetryPolicy()
	p.RetryOnTimeout = false
	err := &TypeSafeTimeoutError{}
	if p.isRetryable(err) {
		t.Error("timeout error should NOT be retryable when RetryOnTimeout=false")
	}
}

func TestRetryPolicy_isRetryable_connectionError(t *testing.T) {
	p := DefaultRetryPolicy()
	err := &TypeSafeConnectionError{Cause: errors.New("connection refused")}
	if !p.isRetryable(err) {
		t.Error("connection error should be retryable by default")
	}
}

func TestRetryPolicy_isRetryable_connErrDisabled(t *testing.T) {
	p := DefaultRetryPolicy()
	p.RetryOnConnErr = false
	err := &TypeSafeConnectionError{}
	if p.isRetryable(err) {
		t.Error("connection error should NOT be retryable when RetryOnConnErr=false")
	}
}

func TestRetryPolicy_isRetryable_nonAPIError(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.isRetryable(errors.New("some random error")) {
		t.Error("arbitrary errors should NOT be retryable")
	}
}

func TestRetryPolicy_isRetryable_nil(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.isRetryable(nil) {
		t.Error("nil should not be retryable")
	}
}

// ─── backoffDelay ─────────────────────────────────────────────────────────────

func TestRetryPolicy_backoffDelay_firstAttempt(t *testing.T) {
	p := RetryPolicy{
		BackoffInitial: 500 * time.Millisecond,
		BackoffMax:     5 * time.Second,
		BackoffJitter:  0, // no jitter for deterministic test
	}
	d := p.backoffDelay(0)
	// With zero jitter: delay = initial * 2^0 = 500ms
	if d != 500*time.Millisecond {
		t.Errorf("backoffDelay(0) = %v, want 500ms", d)
	}
}

func TestRetryPolicy_backoffDelay_secondAttempt(t *testing.T) {
	p := RetryPolicy{
		BackoffInitial: 500 * time.Millisecond,
		BackoffMax:     5 * time.Second,
		BackoffJitter:  0,
	}
	d := p.backoffDelay(1)
	// 500ms * 2^1 = 1s
	if d != 1*time.Second {
		t.Errorf("backoffDelay(1) = %v, want 1s", d)
	}
}

func TestRetryPolicy_backoffDelay_cappedAtMax(t *testing.T) {
	p := RetryPolicy{
		BackoffInitial: 500 * time.Millisecond,
		BackoffMax:     1 * time.Second,
		BackoffJitter:  0,
	}
	// 500ms * 2^4 = 8s, but capped at 1s
	d := p.backoffDelay(4)
	if d > 1*time.Second {
		t.Errorf("backoffDelay(4) = %v, exceeds max 1s", d)
	}
}

func TestRetryPolicy_backoffDelay_zeroInitial(t *testing.T) {
	p := RetryPolicy{BackoffInitial: 0, BackoffMax: 5 * time.Second}
	if p.backoffDelay(0) != 0 {
		t.Error("zero initial backoff should produce zero delay")
	}
}

func TestRetryPolicy_backoffDelay_zeroMax(t *testing.T) {
	p := RetryPolicy{BackoffInitial: 500 * time.Millisecond, BackoffMax: 0}
	if p.backoffDelay(0) != 0 {
		t.Error("zero max backoff should produce zero delay")
	}
}

// ─── parseRetryAfterMs ────────────────────────────────────────────────────────

func TestParseRetryAfterMs_msHeader(t *testing.T) {
	h := make(http.Header)
	h.Set(consts.HeaderRetryAfterMs, "2500")
	ms := parseRetryAfterMs(h)
	if ms == nil || *ms != 2500 {
		t.Errorf("parseRetryAfterMs = %v, want 2500", ms)
	}
}

func TestParseRetryAfterMs_secondsHeader(t *testing.T) {
	h := make(http.Header)
	h.Set(consts.HeaderRetryAfter, "3")
	ms := parseRetryAfterMs(h)
	if ms == nil || *ms != 3000 {
		t.Errorf("parseRetryAfterMs = %v, want 3000", ms)
	}
}

func TestParseRetryAfterMs_msHeaderTakesPrecedence(t *testing.T) {
	h := make(http.Header)
	h.Set(consts.HeaderRetryAfterMs, "1000")
	h.Set(consts.HeaderRetryAfter, "10")
	ms := parseRetryAfterMs(h)
	if ms == nil || *ms != 1000 {
		t.Errorf("parseRetryAfterMs = %v, want 1000 (ms header wins)", ms)
	}
}

func TestParseRetryAfterMs_absent(t *testing.T) {
	ms := parseRetryAfterMs(make(http.Header))
	if ms != nil {
		t.Errorf("parseRetryAfterMs = %v, want nil when header absent", ms)
	}
}

func TestParseRetryAfterMs_negative(t *testing.T) {
	h := make(http.Header)
	h.Set(consts.HeaderRetryAfterMs, "-1")
	ms := parseRetryAfterMs(h)
	if ms != nil {
		t.Errorf("parseRetryAfterMs = %v, want nil for negative value", ms)
	}
}

// ─── retryLoop ────────────────────────────────────────────────────────────────

func TestRetryLoop_succeedsFirstAttempt(t *testing.T) {
	p := RetryPolicy{MaxRetries: 2, BackoffInitial: 0}
	calls := 0
	err := p.retryLoop(context.Background(), func(attempt int) (http.Header, error) {
		calls++
		return nil, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1", calls)
	}
}

func TestRetryLoop_retriesOnRetryableError(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     2,
		BackoffInitial: 0,
		HTTPStatuses:   map[int]bool{500: true},
	}
	calls := 0
	err := p.retryLoop(context.Background(), func(attempt int) (http.Header, error) {
		calls++
		if calls < 3 {
			return make(http.Header), newAPIError(500, nil, make(http.Header), "")
		}
		return nil, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("fn called %d times, want 3", calls)
	}
}

func TestRetryLoop_doesNotRetryNonRetryable(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     2,
		BackoffInitial: 0,
		HTTPStatuses:   map[int]bool{500: true},
	}
	calls := 0
	err := p.retryLoop(context.Background(), func(attempt int) (http.Header, error) {
		calls++
		return make(http.Header), newAPIError(401, nil, make(http.Header), "")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("fn called %d times, want 1 (no retries for 401)", calls)
	}
}

func TestRetryLoop_stopsAfterMaxRetries(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     3,
		BackoffInitial: 0,
		HTTPStatuses:   map[int]bool{500: true},
	}
	calls := 0
	err := p.retryLoop(context.Background(), func(attempt int) (http.Header, error) {
		calls++
		return make(http.Header), newAPIError(500, nil, make(http.Header), "")
	})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	// 1 initial + 3 retries = 4 total calls
	if calls != 4 {
		t.Errorf("fn called %d times, want 4 (1 + MaxRetries)", calls)
	}
}

func TestRetryLoop_respectsContextCancellation(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     100,
		BackoffInitial: 20 * time.Millisecond,
		BackoffMax:     20 * time.Millisecond,
		BackoffJitter:  0,
		HTTPStatuses:   map[int]bool{500: true},
	}
	// Cancel after ~55ms; with 20ms backoff between attempts we expect
	// at most 3 fn calls (attempt 0, sleep 20ms, attempt 1, sleep 20ms, attempt 2,
	// then the context deadline fires before the third sleep completes).
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()

	calls := 0
	_ = p.retryLoop(ctx, func(attempt int) (http.Header, error) {
		calls++
		return make(http.Header), newAPIError(500, nil, make(http.Header), "")
	})

	if calls > 4 {
		t.Errorf("fn called %d times; context cancellation didn't stop retries (expected ≤4)", calls)
	}
	if calls == 0 {
		t.Error("fn was never called")
	}
}
