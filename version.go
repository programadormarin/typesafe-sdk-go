package typesafe

import "github.com/programadormarin/typesafe-sdk-go/internal/consts"

// Version is the version of the TypeSafe Go SDK. It is also embedded in the
// User-Agent and X-TypeSafe-SDK request headers.
//
// The version must match the git tag used for the release (e.g. v0.1.0), which
// is what the go command uses to resolve the module via go get.
const Version = consts.Version
