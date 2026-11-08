// Package consts holds SDK-internal constants: API paths, HTTP header names,
// and the SDK name embedded in request headers.
package consts

// API paths.
const (
	// PathSystemOne is the System One evaluation endpoint.
	PathSystemOne = "/v1/systemone"

	// PathModels is the model listing endpoint.
	PathModels = "/v1/models"
)

// HTTP header names used by the SDK.
const (
	HeaderAuthorization = "Authorization"
	HeaderAccept        = "Accept"
	HeaderContentType   = "Content-Type"
	HeaderUserAgent     = "User-Agent"
	HeaderSDK           = "X-TypeSafe-SDK"
	HeaderRuntime       = "X-TypeSafe-Runtime"
	HeaderRetryCount    = "X-TypeSafe-Retry-Count"
	HeaderRequestID     = "x-typesafe-request-id"
	HeaderRetryAfter    = "Retry-After"
	HeaderRetryAfterMs  = "Retry-After-Ms"
	ContentTypeJSON     = "application/json"
)

// SDKName is embedded in the SDK and User-Agent headers.
const SDKName = "typesafe-sdk-go"

// Version is the SDK release version. It is embedded in request headers and
// exposed as [typesafe.Version]. Bump it and tag the matching git tag on every
// release.
const Version = "0.1.0"
