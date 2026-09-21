# TypeSafe Go SDK

The unofficial, community-built Go client for the [TypeSafe System One API](https://typesafe.ai).

This SDK is not maintained by TypeSafe, but it is useful, secure, and well developed.
It was inspired by the official TypeSafe [Python](https://docs.typesafe.ai/sdk/python)
and [JavaScript](https://docs.typesafe.ai/sdk/javascript) SDKs.

TypeSafe's System One models make fast, structured decisions your code can consume
directly. You send **state** (text, a record, a chat log — any JSON value) and a map
of typed **questions**; the model returns typed **answers** you can act on without
parsing generated text.

- **Zero external dependencies** — pure standard library
- Sync and async-friendly via `context.Context`
- Automatic retries with exponential backoff and `Retry-After` respect
- Full typed error hierarchy — `errors.As` to any specific subtype
- All three question primitives: `Noul`, `Choice`, `Score`

## Requirements

- Go 1.21 or later

## Installation

```sh
go get github.com/programadormarin/typesafe-sdk-go
```

Pin a release instead of tracking the latest commit:

```sh
go get github.com/programadormarin/typesafe-sdk-go@v0.1.0
```

Each release is tagged (e.g. `v0.1.0`), and the running SDK version is exposed
as `typesafe.Version` and embedded in the `User-Agent` / `X-TypeSafe-SDK`
request headers.

## API key

Create an API key at <https://console.typesafe.ai/> and expose it as an environment
variable:

```sh
export TYPESAFE_API_KEY=your_api_key_here
```

Or pass it explicitly with `WithAPIKey`.

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"

    typesafe "github.com/programadormarin/typesafe-sdk-go"
)

func main() {
    client, err := typesafe.New() // reads TYPESAFE_API_KEY
    if err != nil {
        log.Fatal(err)
    }

    resp, err := client.SystemOne(
        context.Background(),
        map[string]any{"document": "I was charged twice. Please fix this ASAP."},
        map[string]typesafe.Question{
            "billing": typesafe.Noul{
                Instructions: "Is this ticket about billing?",
            },
            "tone": typesafe.Choice{
                Instructions: "What is the customer's tone?",
                Criteria: map[string]any{
                    "calm": nil, "frustrated": nil, "angry": nil,
                },
            },
            "urgency": typesafe.Score{
                Instructions: "How urgent is this ticket?",
                Criteria:     []any{"can wait", "this week", "today"},
            },
        },
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Nouls()["billing"].Noul)    // e.g. 0.97
    fmt.Println(resp.Choices()["tone"].Choice)   // e.g. "frustrated"
    fmt.Println(resp.Scores()["urgency"].Score)  // e.g. 1.8
}
```

## Question types

### Noul — yes/no probability

```go
typesafe.Noul{
    Instructions: "Does this message express urgency?",
    // Optional: describe what yes and no mean
    Criteria: &typesafe.NoulCriteria{
        True:  "Explicitly time-sensitive language",
        False: "No urgency expressed",
    },
}
```

The answer is a float64 between 0 (no) and 1 (yes).

```go
answer := resp.Nouls()["my_question"]
fmt.Println(answer.Noul) // e.g. 0.91
```

### Choice — pick one from a set

```go
typesafe.Choice{
    Instructions: "Which team should handle this?",
    Criteria: map[string]any{
        "billing":   "Payments, invoicing, refunds",
        "technical": "Bugs, outages, integrations",
        "sales":     "Pricing, upgrades, new accounts",
    },
}
```

The answer includes the winning option, all probabilities, and a confidence score.

```go
answer := resp.Choices()["my_question"]
fmt.Println(answer.Choice)        // e.g. "billing"
fmt.Println(answer.Confidence)    // e.g. 0.88
fmt.Println(answer.Probabilities) // map[billing:0.88 technical:0.10 sales:0.02]
```

### Score — rate along a rubric

```go
typesafe.Score{
    Instructions: "How frustrated is the customer?",
    Criteria:     []any{"Calm", "Frustrated", "Very angry"},
}
```

The answer is a probability-weighted value that may fall between levels (0 = first
level, len(criteria)-1 = last level).

```go
answer := resp.Scores()["my_question"]
fmt.Println(answer.Score)         // e.g. 1.05  (between level 1 and 2)
fmt.Println(answer.Legend)        // map[0:Calm 1:Frustrated 2:Very angry]
fmt.Println(answer.Probabilities) // map[0:0.00 1:0.95 2:0.05]
fmt.Println(answer.Confidence)    // e.g. 0.92
```

## Structured instructions

All `Instructions` and `Criteria` fields accept a string, a `map[string]any`, or a
`[]any`. Use structured objects to break complex questions into named parts:

```go
typesafe.Noul{
    Instructions: map[string]any{
        "candidate": map[string]any{"name": "Alice", "employer": "Acme Corp"},
        "question":  "Is the resume for the same person as `candidate`?",
    },
}
```

## Sending many questions in parallel

All questions in a single `SystemOne` call run in parallel at no extra latency cost.
Batch independent questions together:

```go
resp, err := client.SystemOne(ctx, state, map[string]typesafe.Question{
    "department":         typesafe.Choice{ /* ... */ },
    "urgency":            typesafe.Score{ /* ... */ },
    "is_billing":         typesafe.Noul{ /* ... */ },
    "is_chargeback_risk": typesafe.Noul{ /* ... */ },
})
```

See [`examples/parallel_questions`](examples/parallel_questions/main.go) for a
full triage pipeline example.

## Client options

```go
client, err := typesafe.New(
    typesafe.WithAPIKey("sk-..."),              // explicit API key
    typesafe.WithModel("jev-1.13.0"),           // override default model
    typesafe.WithTimeout(15 * time.Second),     // per-request HTTP timeout
    typesafe.WithBaseURL("https://..."),        // override API base URL
    typesafe.WithHeader("X-Org", "acme"),       // default header on every request
    typesafe.WithRetry(typesafe.RetryPolicy{    // custom retry policy
        MaxRetries: 3,
    }),
)
```

### Per-request overrides

```go
resp, err := client.SystemOne(ctx, state, questions,
    typesafe.WithRequestModel("jev-1.13.0"),
    typesafe.WithRequestTimeout(5 * time.Second),
    typesafe.WithRequestHeader("X-Trace-ID", traceID),
    typesafe.WithRequestRetry(typesafe.RetryPolicy{MaxRetries: 0}),
)
```

## Retry policy

The client retries automatically on `408`, `429`, and all `5xx` responses, with
exponential backoff and optional `Retry-After` header respect.

```go
typesafe.WithRetry(typesafe.RetryPolicy{
    MaxRetries:        3,
    BackoffInitial:    500 * time.Millisecond,
    BackoffMax:        5 * time.Second,
    BackoffJitter:     0.25,
    HTTPStatuses:      map[int]bool{429: true, 500: true, 502: true, 503: true, 504: true},
    RespectRetryAfter: true,
    RetryOnTimeout:    true,
    RetryOnConnErr:    true,
    Timeout:           30 * time.Second, // total budget across all attempts
})
```

Pass `RetryPolicy{MaxRetries: 0}` to disable retries entirely.

## Error handling

All errors implement `error` and can be unwrapped with `errors.As`:

```go
resp, err := client.SystemOne(ctx, state, questions)
if err != nil {
    var authErr  *typesafe.TypeSafeAuthenticationError
    var rateErr  *typesafe.TypeSafeRateLimitError
    var connErr  *typesafe.TypeSafeConnectionError

    switch {
    case errors.As(err, &authErr):
        log.Fatal("invalid API key")
    case errors.As(err, &rateErr):
        if rateErr.RetryAfterMs != nil {
            log.Printf("retry after %dms", *rateErr.RetryAfterMs)
        }
    case errors.As(err, &connErr):
        log.Printf("network error: %v", err)
    default:
        log.Fatal(err)
    }
}
```

### Error types

| Type | When |
|------|------|
| `TypeSafeAuthenticationError` | HTTP 401 — bad or missing API key |
| `TypeSafePermissionDeniedError` | HTTP 403 |
| `TypeSafeBadRequestError` | HTTP 400 |
| `TypeSafeNotFoundError` | HTTP 404 |
| `TypeSafeUnprocessableEntityError` | HTTP 422 — invalid request body |
| `TypeSafeRateLimitError` | HTTP 429 — includes `RetryAfterMs` |
| `TypeSafeInternalServerError` | HTTP 5xx |
| `TypeSafeConnectionError` | No HTTP response (network failure) |
| `TypeSafeTimeoutError` | Request exceeded timeout |
| `TypeSafeResponseValidationError` | 2xx but invalid response body |

All HTTP error types embed `TypeSafeAPIError` which provides `StatusCode`, `Body`,
`Headers`, `RequestID`, and `Endpoint`. All types ultimately embed `TypeSafeError`.

## Listing models

```go
models, err := client.Models(context.Background())
if err != nil {
    log.Fatal(err)
}
for _, m := range models.Models {
    fmt.Printf("%s — %s\n", m.Name, m.Description)
}
```

## Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TYPESAFE_API_KEY` | API key | — (required) |
| `TYPESAFE_BASE_URL` | API base URL | `https://api.typesafe.ai` |
| `TYPESAFE_DEFAULT_MODEL` | Default model name | `jev-latest` |

## Examples

| Example | Description |
|---------|-------------|
| [`examples/quickstart`](examples/quickstart/main.go) | All three primitives in one call |
| [`examples/parallel_questions`](examples/parallel_questions/main.go) | 7-question triage pipeline |

## Integration tests

Integration tests make real API calls and are guarded by a build tag:

```sh
TYPESAFE_API_KEY=your_key go test -tags integration ./...
```

## License

[MIT](LICENSE)
