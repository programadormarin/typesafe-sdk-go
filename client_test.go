package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
)

// ─── New() config resolution ──────────────────────────────────────────────────

func TestNew_missingAPIKeyReturnsError(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	_, err := New()
	if err == nil {
		t.Fatal("expected error when no API key")
	}
	var tsErr *TypeSafeError
	if !errors.As(err, &tsErr) {
		t.Errorf("expected *TypeSafeError, got %T", err)
	}
}

func TestNew_invalidAPIKeyReturnsError(t *testing.T) {
	_, err := New(WithAPIKey("key with spaces"))
	if err == nil {
		t.Fatal("expected error for API key with spaces")
	}
}

func TestNew_envAPIKeyUsed(t *testing.T) {
	t.Setenv(EnvAPIKey, "env-key-abc")
	c, err := New()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.apiKey != "env-key-abc" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "env-key-abc")
	}
}

func TestNew_optionOverridesEnv(t *testing.T) {
	t.Setenv(EnvAPIKey, "env-key")
	c, err := New(WithAPIKey("option-key"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.apiKey != "option-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "option-key")
	}
}

func TestNew_defaultModelUsed(t *testing.T) {
	c := mustNewClient(t)
	if c.defaultModel != DefaultModel {
		t.Errorf("defaultModel = %q, want %q", c.defaultModel, DefaultModel)
	}
}

func TestNew_withModelOption(t *testing.T) {
	c := mustNewClient(t, WithModel("jev-1.13.0"))
	if c.defaultModel != "jev-1.13.0" {
		t.Errorf("defaultModel = %q, want %q", c.defaultModel, "jev-1.13.0")
	}
}

func TestNew_defaultBaseURL(t *testing.T) {
	c := mustNewClient(t)
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
}

func TestNew_trailingSlashStripped(t *testing.T) {
	c := mustNewClient(t, WithBaseURL("https://example.com/api/"))
	if strings.HasSuffix(c.baseURL, "/") {
		t.Errorf("baseURL %q should not have trailing slash", c.baseURL)
	}
}

func TestNew_defaultTimeout(t *testing.T) {
	c := mustNewClient(t)
	if c.timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", c.timeout, DefaultTimeout)
	}
}

func TestNew_customTimeout(t *testing.T) {
	c := mustNewClient(t, WithTimeout(30*time.Second))
	if c.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.timeout)
	}
}

func TestNew_defaultHeaders(t *testing.T) {
	c := mustNewClient(t, WithHeader("X-Org", "acme"))
	if c.defaultHeaders["X-Org"] != "acme" {
		t.Errorf("default header X-Org = %q, want %q", c.defaultHeaders["X-Org"], "acme")
	}
}

// ─── SystemOne() against a mock server ───────────────────────────────────────

func TestSystemOne_sendsCorrectRequestBody(t *testing.T) {
	var capturedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		writeSystemOneResponse(w, `{
			"model": "jev-latest",
			"answers": {"is_urgent": {"type": "noul", "noul": 0.9}},
			"usage": {}
		}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	_, err := c.SystemOne(context.Background(),
		"Help, my payouts are failing!",
		map[string]Question{
			"is_urgent": Noul{Instructions: "Is this urgent?"},
		},
	)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}

	if capturedBody["state"] != "Help, my payouts are failing!" {
		t.Errorf("state = %v", capturedBody["state"])
	}
	if capturedBody["model"] != DefaultModel {
		t.Errorf("model = %v, want %q", capturedBody["model"], DefaultModel)
	}
	questions, ok := capturedBody["questions"].(map[string]any)
	if !ok {
		t.Fatalf("questions is %T", capturedBody["questions"])
	}
	q := questions["is_urgent"].(map[string]any)
	if q["type"] != "noul" {
		t.Errorf("question type = %v, want noul", q["type"])
	}
}

func TestSystemOne_sendsAuthHeader(t *testing.T) {
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		writeSystemOneResponse(w, `{"model":"jev-latest","answers":{},"usage":{}}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	_, _ = c.SystemOne(context.Background(), "test", map[string]Question{
		"q": Noul{Instructions: "x"},
	})

	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("Authorization header = %q, want Bearer prefix", authHeader)
	}
}

func TestSystemOne_perRequestModelOverride(t *testing.T) {
	var capturedModel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		capturedModel, _ = body["model"].(string)
		writeSystemOneResponse(w, `{"model":"jev-1.13.0","answers":{},"usage":{}}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	_, _ = c.SystemOne(context.Background(), "x", map[string]Question{
		"q": Noul{Instructions: "x"},
	}, WithRequestModel("jev-1.13.0"))

	if capturedModel != "jev-1.13.0" {
		t.Errorf("model = %q, want %q", capturedModel, "jev-1.13.0")
	}
}

func TestSystemOne_returnsAuthenticationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	_, err := c.SystemOne(context.Background(), "x", map[string]Question{"q": Noul{}})
	if err == nil {
		t.Fatal("expected error")
	}
	var authErr *TypeSafeAuthenticationError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *TypeSafeAuthenticationError, got %T: %v", err, err)
	}
}

func TestSystemOne_retriesOn500(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, `{"message":"server error"}`, http.StatusInternalServerError)
			return
		}
		writeSystemOneResponse(w, `{"model":"jev-latest","answers":{"q":{"type":"noul","noul":0.5}},"usage":{}}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{
		MaxRetries:     3,
		BackoffInitial: 0,
		HTTPStatuses:   map[int]bool{500: true},
	}))
	_, err := c.SystemOne(context.Background(), "x", map[string]Question{"q": Noul{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestSystemOne_emptyQuestionsReturnsError(t *testing.T) {
	c := mustNewClient(t)
	_, err := c.SystemOne(context.Background(), "x", map[string]Question{})
	if err == nil {
		t.Fatal("expected error for empty questions")
	}
}

// ─── Models() ─────────────────────────────────────────────────────────────────

func TestModels_returnsModelList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"models": [
				{"name": "jev-latest", "description": "Latest Jev", "release_date": "2025-01-01"},
				{"name": "jev-1.13.0", "description": "Jev 1.13", "release_date": "2024-11-01"}
			]
		}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	resp, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(resp.Models) != 2 {
		t.Errorf("Models len = %d, want 2", len(resp.Models))
	}
	if resp.Models[0].Name != "jev-latest" {
		t.Errorf("Models[0].Name = %q", resp.Models[0].Name)
	}
}

// ─── Request headers ──────────────────────────────────────────────────────────

func TestSystemOne_sdkHeaderPresent(t *testing.T) {
	var sdkHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sdkHeader = r.Header.Get(consts.HeaderSDK)
		writeSystemOneResponse(w, `{"model":"jev-latest","answers":{},"usage":{}}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	_, _ = c.SystemOne(context.Background(), "x", map[string]Question{"q": Noul{}})
	if !strings.HasPrefix(sdkHeader, "typesafe-sdk-go/") {
		t.Errorf("X-TypeSafe-SDK = %q, want typesafe-sdk-go/ prefix", sdkHeader)
	}
}

func TestSystemOne_defaultHeaderForwarded(t *testing.T) {
	var orgHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgHeader = r.Header.Get("X-Org")
		writeSystemOneResponse(w, `{"model":"jev-latest","answers":{},"usage":{}}`)
	}))
	defer srv.Close()

	c := mustNewClient(t, WithBaseURL(srv.URL),
		WithHeader("X-Org", "acme"),
		WithRetry(RetryPolicy{MaxRetries: 0}),
	)
	_, _ = c.SystemOne(context.Background(), "x", map[string]Question{"q": Noul{}})
	if orgHeader != "acme" {
		t.Errorf("X-Org = %q, want acme", orgHeader)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustNewClient(t *testing.T, opts ...ClientOption) *Client {
	t.Helper()
	t.Setenv(EnvAPIKey, "test-key-abc")
	opts = append([]ClientOption{WithAPIKey("test-key-abc")}, opts...)
	c, err := New(opts...)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return c
}

func writeSystemOneResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}
