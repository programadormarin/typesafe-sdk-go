package typesafe

import (
	"net/http"
	"time"
)

// JSONValue is a value accepted by encoding/json.
type JSONValue = any

// SystemOneRequest evaluates state against named typed questions.
type SystemOneRequest struct {
	State     any
	Questions map[string]Question
	Model     string
	ExtraBody map[string]any
	Assets    map[string]*Image
}

// RequestOptions overrides client settings for one API call (valksor-compatible).
type RequestOptions struct {
	Timeout time.Duration
	Retry   *RetryPolicy
	Headers http.Header
}

