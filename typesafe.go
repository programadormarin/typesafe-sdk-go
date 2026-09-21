// Package typesafe is an unofficial, community-built Go client for the TypeSafe
// System One API. It is not maintained by TypeSafe, but it is useful, secure, and
// well developed. It was inspired by the official TypeSafe Python and JavaScript
// SDKs.
//
// TypeSafe's System One models make fast, structured decisions that software can
// consume directly. You send state (text, a record, a chat log — any JSON value)
// and a map of typed questions; the model returns typed answers your code can act
// on immediately, without parsing generated text.
//
// # Quickstart
//
//	client, err := typesafe.New() // reads TYPESAFE_API_KEY from the environment
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	resp, err := client.SystemOne(
//	    context.Background(),
//	    map[string]any{"document": "I was charged twice. Please fix this ASAP."},
//	    map[string]typesafe.Question{
//	        "billing": typesafe.Noul{Instructions: "Is this ticket about billing?"},
//	        "tone": typesafe.Choice{
//	            Instructions: "What is the customer's tone?",
//	            Criteria: map[string]any{"calm": nil, "frustrated": nil, "angry": nil},
//	        },
//	        "urgency": typesafe.Score{
//	            Instructions: "How urgent is this ticket?",
//	            Criteria:     []any{"can wait", "this week", "today"},
//	        },
//	    },
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Println(resp.Nouls()["billing"].Noul)    // e.g. 0.97
//	fmt.Println(resp.Choices()["tone"].Choice)   // e.g. "frustrated"
//	fmt.Println(resp.Scores()["urgency"].Score)  // e.g. 1.8
//
// # API key
//
// Obtain an API key at https://console.typesafe.ai/ and expose it as
// TYPESAFE_API_KEY, or pass it explicitly with [WithAPIKey].
//
// # Question types
//
//   - [Noul] — yes/no question; answer is a probability 0–1.
//   - [Choice] — pick one of N named options; answer includes the winner and probabilities.
//   - [Score] — rate along an ordered rubric; answer is a probability-weighted value.
//
// # Error handling
//
// All errors returned by this package implement error and can be unwrapped with
// errors.As to reach status-specific types such as [TypeSafeAuthenticationError],
// [TypeSafeRateLimitError], or [TypeSafeInternalServerError].
//
// # Retries
//
// The client retries failed requests automatically. Configure the policy with
// [WithRetry] or [DefaultRetryPolicy]. Pass RetryPolicy{MaxRetries: 0} to disable.
//
// See https://docs.typesafe.ai/ for the full documentation.
package typesafe
