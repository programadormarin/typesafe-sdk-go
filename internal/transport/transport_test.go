package transport

import (
	"context"
	"net/http"
	"testing"

	"github.com/programadormarin/typesafe-sdk-go/internal/consts"
)

// ─── BuildRequest ─────────────────────────────────────────────────────────────

func TestBuildRequest_authorizationHeader(t *testing.T) {
	req := Request{
		Method: "POST",
		URL:    "https://api.typesafe.ai/v1/systemone",
		Body:   []byte(`{}`),
		APIKey: "sk-test",
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderAuthorization); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer sk-test")
	}
}

func TestBuildRequest_contentTypeSetForBody(t *testing.T) {
	req := Request{
		Method: "POST",
		URL:    "https://api.typesafe.ai/v1/systemone",
		Body:   []byte(`{"key":"val"}`),
		APIKey: "sk-test",
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderContentType); got != consts.ContentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", got, consts.ContentTypeJSON)
	}
}

func TestBuildRequest_noContentTypeForGET(t *testing.T) {
	req := Request{
		Method: "GET",
		URL:    "https://api.typesafe.ai/v1/models",
		APIKey: "sk-test",
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderContentType); got != "" {
		t.Errorf("Content-Type should be empty for GET, got %q", got)
	}
}

func TestBuildRequest_retryCountOmittedOnFirstAttempt(t *testing.T) {
	req := Request{
		Method: "POST",
		URL:    "https://api.typesafe.ai/v1/systemone",
		Body:   []byte(`{}`),
		APIKey: "sk-test",
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderRetryCount); got != "" {
		t.Errorf("retry count header should be absent on attempt 0, got %q", got)
	}
}

func TestBuildRequest_retryCountSetOnRetry(t *testing.T) {
	req := Request{
		Method:  "POST",
		URL:     "https://api.typesafe.ai/v1/systemone",
		Body:    []byte(`{}`),
		APIKey:  "sk-test",
		Attempt: 2,
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderRetryCount); got != "2" {
		t.Errorf("retry count header = %q, want %q", got, "2")
	}
}

func TestBuildRequest_extraHeadersApplied(t *testing.T) {
	req := Request{
		Method:       "POST",
		URL:          "https://api.typesafe.ai/v1/systemone",
		Body:         []byte(`{}`),
		APIKey:       "sk-test",
		ExtraHeaders: map[string]string{"X-Custom": "value"},
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get("X-Custom"); got != "value" {
		t.Errorf("X-Custom = %q, want %q", got, "value")
	}
}

func TestBuildRequest_extraHeadersCannotOverrideAuth(t *testing.T) {
	req := Request{
		Method:       "POST",
		URL:          "https://api.typesafe.ai/v1/systemone",
		Body:         []byte(`{}`),
		APIKey:       "real-key",
		ExtraHeaders: map[string]string{consts.HeaderAuthorization: "Bearer injected"},
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderAuthorization); got != "Bearer real-key" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer real-key")
	}
}

func TestBuildRequest_sdkAndRuntimeHeaders(t *testing.T) {
	req := Request{
		Method: "GET",
		URL:    "https://api.typesafe.ai/v1/models",
		APIKey: "sk-test",
	}
	hreq, err := BuildRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := hreq.Header.Get(consts.HeaderSDK); got == "" {
		t.Error("X-TypeSafe-SDK header should be set")
	}
	if got := hreq.Header.Get(consts.HeaderRuntime); got == "" {
		t.Error("X-TypeSafe-Runtime header should be set")
	}
	if got := hreq.Header.Get(consts.HeaderUserAgent); got == "" {
		t.Error("User-Agent header should be set")
	}
}

// ─── endpointDesc ─────────────────────────────────────────────────────────────

func TestEndpointDesc_stripsCredentials(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://user:pass@api.typesafe.ai/v1/systemone?foo=bar", nil)
	desc := endpointDesc(req)
	if contains(desc, "user") || contains(desc, "pass") || contains(desc, "foo") {
		t.Errorf("endpointDesc %q should not contain credentials or query params", desc)
	}
	if !contains(desc, "POST") || !contains(desc, "/v1/systemone") {
		t.Errorf("endpointDesc %q should contain method and path", desc)
	}
}

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
