package typesafe

import "time"

// Environment variable names.
const (
	// EnvAPIKey is the environment variable read when no API key is passed explicitly.
	EnvAPIKey = "TYPESAFE_API_KEY"

	// EnvBaseURL overrides the default API base URL.
	EnvBaseURL = "TYPESAFE_BASE_URL"

	// EnvDefaultModel overrides the default model name.
	EnvDefaultModel = "TYPESAFE_DEFAULT_MODEL"
)

// Defaults used when options are not provided.
const (
	// DefaultBaseURL is the production TypeSafe API base URL.
	DefaultBaseURL = "https://api.typesafe.ai"

	// DefaultModel is the model alias used when no model is specified.
	DefaultModel = "jev-latest"

	// DefaultTimeout is the per-request HTTP timeout.
	DefaultTimeout = 10 * time.Second
)
