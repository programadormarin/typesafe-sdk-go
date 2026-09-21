// Package jsonutil provides safe JSON encode/decode helpers for the TypeSafe SDK.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Marshal encodes v to JSON. Returns a [TypeSafeError]-compatible error message on failure.
func Marshal(v any) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("could not encode request body as JSON: %w", err)
	}
	// Encoder appends a trailing newline; trim it so Content-Length is exact.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Unmarshal decodes JSON data into v.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
