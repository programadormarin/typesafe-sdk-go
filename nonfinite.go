package typesafe

import "math"

// nonFiniteString returns the stand-in string for a non-finite number.
func nonFiniteString(f float64) (string, bool) {
	switch {
	case math.IsNaN(f):
		return "NaN", true
	case math.IsInf(f, 1):
		return "Infinity", true
	case math.IsInf(f, -1):
		return "-Infinity", true
	}
	return "", false
}

// sanitizeNonFinite replaces NaN, +Inf, and -Inf with strings recursively.
func sanitizeNonFinite(value any) any {
	switch v := value.(type) {
	case float64:
		if s, ok := nonFiniteString(v); ok {
			return s
		}
	case float32:
		if s, ok := nonFiniteString(float64(v)); ok {
			return s
		}
	case []float64:
		out := make([]any, len(v))
		for i, f := range v {
			out[i] = sanitizeNonFinite(f)
		}
		return out
	case []float32:
		out := make([]any, len(v))
		for i, f := range v {
			out[i] = sanitizeNonFinite(f)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeNonFinite(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = sanitizeNonFinite(item)
		}
		return out
	}
	return value
}
