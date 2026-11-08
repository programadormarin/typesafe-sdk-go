package typesafe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientConfigAndAccessors(t *testing.T) {
	t.Setenv(EnvAPIKey, "env-key")
	c, err := NewClient(Config{APIKey: "k", BaseURL: "https://example.test", DefaultModel: "m", Compression: CompressionNone})
	if err != nil { t.Fatal(err) }
	if c.BaseURL() != "https://example.test" || c.DefaultModel() != "m" { t.Fatalf("accessors: %q %q", c.BaseURL(), c.DefaultModel()) }
	if c.compress { t.Fatal("compression should be off") }
	if _, err := NewClient(Config{}, Config{}); err == nil { t.Fatal("expected too many configs") }
	if _, err := NewClient(Config{APIKey: "k", BaseURL: "not-a-url"}); err == nil { t.Fatal("expected bad url") }
	if _, err := NewClient(Config{APIKey: "k", Timeout: -time.Second}); err == nil { t.Fatal("expected bad timeout") }
	if _, err := NewClient(Config{APIKey: "k", Compression: Compression("bad")}); err == nil { t.Fatal("expected bad compression") }
	bad := DefaultRetryPolicy(); bad.MaxRetries = -1
	if _, err := NewClient(Config{APIKey: "k", Retry: &bad}); err == nil { t.Fatal("expected bad retry") }
	if _, err := NewClient(); err != nil { t.Fatalf("env config: %v", err) }
}

func TestModelsServiceAndMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"m","description":"d","release_date":"2026-01-01"}]}`))
	}))
	defer srv.Close()
	c, _ := New(WithAPIKey("k"), WithBaseURL(srv.URL))
	if resp, err := c.Models(context.Background()); err != nil || resp.Models[0].Name != "m" { t.Fatalf("Models: %v %+v", err, resp) }
	if resp, err := c.Models.List(context.Background()); err != nil || resp.Models[0].Name != "m" { t.Fatalf("Models.List: %v %+v", err, resp) }
}

func TestImageBase64AndString(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	enc := base64.StdEncoding.EncodeToString(png)
	img, err := ImageFromBase64("  " + enc[:4] + "\n" + enc[4:] + " ")
	if err != nil { t.Fatal(err) }
	if !strings.HasPrefix(img.String(), "<image:") { t.Fatalf("ref %q", img.String()) }
	b, _ := json.Marshal(img)
	var ref string
	_ = json.Unmarshal(b, &ref)
	if ref != img.String() { t.Fatalf("marshal %s", b) }
	if _, err := ImageFromBase64(""); err == nil { t.Fatal("expected empty") }
	if _, err := ImageFromBase64("!!!!"); err == nil { t.Fatal("expected bad base64") }
	if _, err := ImageFromBase64("é"); err == nil { t.Fatal("expected non-ascii") }
	if _, err := ImageFromFile("/definitely/missing/file.png").resolve(); err == nil { t.Fatal("expected file error") }
	if _, err := NewImage(nil).resolve(); err == nil { t.Fatal("expected empty image") }
	if _, err := NewImage([]byte{1}).resolve(); err != nil { t.Fatal(err) }
}

func TestContentTypes(t *testing.T) {
	if contentType([]byte("\x89PNG\r\n\x1a\n")) != contentTypePNG { t.Fatal("png") }
	if contentType([]byte("\xff\xd8\xff")) != contentTypeJPEG { t.Fatal("jpeg") }
	if contentType([]byte("xx")) != contentTypeUnknown { t.Fatal("unknown") }
}

func TestPassedAssetsValidation(t *testing.T) {
	if _, _, err := passedAssets(map[string]*Image{" ": NewImage([]byte{1})}, map[string]*Image{}); err == nil { t.Fatal("expected empty alias") }
	if _, _, err := passedAssets(map[string]*Image{"a": nil}, map[string]*Image{}); err == nil { t.Fatal("expected nil image") }
}

func TestExtraBodyAssetsSkipsHandling(t *testing.T) {
	c, _ := New(WithAPIKey("k"))
	body := []byte(`{"x":1}`)
	out, ct, err := c.withAssets(body, SystemOneRequest{ExtraBody: map[string]any{"assets": nil}})
	if err != nil || string(out) != string(body) || ct != jsonContentType { t.Fatalf("%s %s %v", out, ct, err) }
}

func TestRawQuestionValidationAndMarshal(t *testing.T) {
	if err := (RawQuestion{}).validate("q"); err == nil { t.Fatal("expected empty type") }
	if err := (RawQuestion{"type": "choice", "criteria": map[string]any{}}).validate("q"); err == nil { t.Fatal("expected empty choice") }
	if err := (RawQuestion{"type": "score", "criteria": []any{}}).validate("q"); err == nil { t.Fatal("expected empty score") }
	if err := (RawQuestion{"type": "custom", "x": 1}).validate("q"); err != nil { t.Fatal(err) }
	b, err := json.Marshal(RawQuestion{"type": "noul", "v": 1.0})
	if err != nil || !strings.Contains(string(b), `"type":"noul"`) { t.Fatalf("%s %v", b, err) }
	if !emptyContainer(nil) || !emptyContainer([]any{}) || emptyContainer("x") { t.Fatal("emptyContainer") }
}

func TestBoxAndRequestOptions(t *testing.T) {
	var gotHeader, gotModel, gotExtra string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Test")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		gotExtra, _ = body["extra"].(string)
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{}}`))
	}))
	defer srv.Close()
	c, _ := New(WithAPIKey("k"), WithBaseURL(srv.URL), WithHeaders(http.Header{"X-Base": {"1"}}), WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	_, err := c.SystemOne(context.Background(), SystemOneRequest{State: "x", Questions: map[string]Question{"q": Box{}}, Model: "m"}, RequestOptions{Headers: http.Header{"X-Test": {"yes"}}})
	if err != nil { t.Fatal(err) }
	if gotHeader != "yes" || gotModel != "m" { t.Fatalf("%q %q", gotHeader, gotModel) }
	_, err = c.SystemOne(context.Background(), "x", map[string]Question{"q": Noul{Instructions: "i"}}, WithRequestHeader("X-Test", "yes2"), WithRequestModel("m2"), WithRequestExtraBody(map[string]any{"extra": "e"}))
	if err != nil { t.Fatal(err) }
	if gotHeader != "yes2" || gotModel != "m2" || gotExtra != "e" { t.Fatalf("%q %q %q", gotHeader, gotModel, gotExtra) }
}

func TestErrorTypesAndUserAbort(t *testing.T) {
	if _, ok := newAPIError(409, []byte(`{"message":"conflict"}`), http.Header{}, "ep").(*TypeSafeConflictError); !ok { t.Fatal("conflict") }
	rl, ok := newAPIError(429, []byte(`{}`), http.Header{"Retry-After": {"2"}}, "ep").(*TypeSafeRateLimitError)
	if !ok || rl.RetryAfterMs == nil || *rl.RetryAfterMs != 2000 { t.Fatalf("rate limit %+v", rl) }
	if _, ok := newAPIError(401, nil, http.Header{}, "ep").(*TypeSafeAuthenticationError); !ok { t.Fatal("auth") }
	abort := &TypeSafeUserAbortError{Cause: context.Canceled}
	if abort.Error() == "" || !errors.Is(abort, context.Canceled) { t.Fatal("abort") }
	if (&TypeSafeError{Message: "m"}).Error() != "m" { t.Fatal("plain error") }
}

func TestSystemOneAsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","answers":{"q":{"no_type":true}},"usage":{}}`))
	}))
	defer srv.Close()
	c, _ := New(WithAPIKey("k"), WithBaseURL(srv.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	type R struct{ Model string `json:"model"` }
	if _, err := SystemOneAs[R](context.Background(), c, SystemOneRequest{State: "x", Questions: map[string]Question{"q": Noul{Instructions: "i"}}}); err == nil {
		t.Fatal("expected lift error")
	}
	// type mismatch path
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","answers":{"q":{"type":"noul","noul":1}},"usage":{}}`))
	}))
	defer srv2.Close()
	c2, _ := New(WithAPIKey("k"), WithBaseURL(srv2.URL), WithRetry(RetryPolicy{MaxRetries: 0}))
	type Bad struct{ Q string `json:"q"` }
	if _, err := SystemOneAs[Bad](context.Background(), c2, SystemOneRequest{State: "x", Questions: map[string]Question{"q": Noul{Instructions: "i"}}}); err == nil {
		t.Fatal("expected type error")
	}
}

func TestAttachAssetsPanicsOnNil(t *testing.T) {
	c, _ := New(WithAPIKey("k"))
	defer func() { if recover() == nil { t.Fatal("expected panic") } }()
	c.AttachAssets(nil)
}

func TestCompressionNoneNoHeader(t *testing.T) {
	var ce string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ce = r.Header.Get("Content-Encoding")
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{}}`))
	}))
	defer srv.Close()
	c, _ := New(WithAPIKey("k"), WithBaseURL(srv.URL), WithCompression(CompressionNone), WithRetry(RetryPolicy{MaxRetries: 0}))
	_, _ = c.SystemOne(context.Background(), strings.Repeat("x", 2000), map[string]Question{"q": Noul{Instructions: strings.Repeat("y", 2000)}})
	if ce != "" { t.Fatalf("content-encoding=%q", ce) }
}
