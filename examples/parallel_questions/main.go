// Parallel questions example for the TypeSafe Go SDK.
//
// Shows how to batch many questions — including speculative ones — into a single
// System One call. TypeSafe evaluates all questions in parallel, so one request
// is far cheaper and faster than issuing separate calls.
//
// The scenario: a support-ticket triage pipeline that needs routing, urgency,
// sentiment, and a handful of compliance checks — all in one round-trip.
//
// Set TYPESAFE_API_KEY before running:
//
//	TYPESAFE_API_KEY=your_key go run ./examples/parallel_questions
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	typesafe "github.com/programadormarin/typesafe-sdk-go"
)

// ticket is the state sent to the model.
type ticket struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Channel string `json:"channel"` // "email" | "chat" | "phone"
}

func main() {
	client, err := typesafe.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}

	t := ticket{
		ID:      "TKT-9821",
		Subject: "Double charge on my last invoice",
		Body: `Hi,
I noticed that my credit card was charged twice for order #88234.
The charges appeared on the 14th and 15th. This is very frustrating.
Please refund the duplicate charge immediately or I will dispute with my bank.`,
		Channel: "email",
	}

	// All questions are independent and run together in one API call.
	// Speculative questions (e.g. "is_chargeback_risk") are included up-front;
	// code selects which answers to act on.
	questions := map[string]typesafe.Question{
		// ── Routing ─────────────────────────────────────────────────────────
		"department": typesafe.Choice{
			Instructions: "Which department should handle this ticket?",
			Criteria: map[string]any{
				"billing":   "Payments, charges, refunds, invoices",
				"technical": "Bugs, outages, feature requests, API issues",
				"sales":     "Pricing questions, upgrades, new accounts",
				"other":     "Does not fit the above categories",
			},
		},

		// ── Urgency ──────────────────────────────────────────────────────────
		"urgency": typesafe.Score{
			Instructions: "How urgently must this ticket be addressed?",
			Criteria:     []any{"low – can wait days", "medium – handle within 24h", "high – same-day response needed"},
		},

		// ── Sentiment ────────────────────────────────────────────────────────
		"sentiment": typesafe.Choice{
			Instructions: "What is the overall customer sentiment?",
			Criteria: map[string]any{
				"positive": nil,
				"neutral":  nil,
				"negative": nil,
			},
		},

		// ── Yes/No checks (Noul) ─────────────────────────────────────────────
		"is_billing_issue": typesafe.Noul{
			Instructions: "Does the ticket describe a billing or payment problem?",
		},
		"is_chargeback_risk": typesafe.Noul{
			Instructions: "Does the customer mention a bank dispute, chargeback, or credit-card reversal?",
			Criteria: &typesafe.NoulCriteria{
				True:  "Customer explicitly threatens or mentions a chargeback or bank dispute",
				False: "No mention of a chargeback or bank dispute",
			},
		},
		"contains_pii": typesafe.Noul{
			Instructions: "Does the ticket body contain personally identifiable information such as a full name, address, credit card number, or government ID?",
		},
		"needs_escalation": typesafe.Noul{
			Instructions: "Does this ticket require immediate escalation to a senior agent or manager?",
		},
	}

	resp, err := client.SystemOne(context.Background(), t, questions)
	if err != nil {
		// Demonstrate typed error handling.
		var authErr *typesafe.TypeSafeAuthenticationError
		var rateLimitErr *typesafe.TypeSafeRateLimitError
		switch {
		case errors.As(err, &authErr):
			log.Fatalf("authentication failed — check TYPESAFE_API_KEY: %v", err)
		case errors.As(err, &rateLimitErr):
			waitMs := int64(0)
			if rateLimitErr.RetryAfterMs != nil {
				waitMs = *rateLimitErr.RetryAfterMs
			}
			log.Fatalf("rate limited; retry after %dms: %v", waitMs, err)
		default:
			log.Fatalf("SystemOne: %v", err)
		}
	}

	printResults(t, resp)
}

func printResults(t ticket, resp *typesafe.SystemOneResponse) {
	fmt.Printf("═══ Triage results for %s ═══\n\n", t.ID)

	// Routing decision.
	dept := resp.Choices()["department"]
	fmt.Printf("Department:       %s  (confidence %.2f)\n", dept.Choice, dept.Confidence)

	// Urgency score.
	urgency := resp.Scores()["urgency"]
	fmt.Printf("Urgency:          %.2f / 2  (confidence %.2f)\n",
		urgency.Score, urgency.Confidence)

	// Sentiment.
	sentiment := resp.Choices()["sentiment"]
	fmt.Printf("Sentiment:        %s  (confidence %.2f)\n",
		sentiment.Choice, sentiment.Confidence)

	fmt.Println()

	// Yes/No signals — print with threshold-based actions.
	printNoul(resp, "is_billing_issue", 0.7,
		"✓ Billing issue detected — route to billing queue",
		"  Not a billing issue")

	printNoul(resp, "is_chargeback_risk", 0.5,
		"⚠ Chargeback risk — flag for immediate attention",
		"  No chargeback risk")

	printNoul(resp, "contains_pii", 0.5,
		"⚠ PII detected — apply data-handling policy",
		"  No PII detected")

	printNoul(resp, "needs_escalation", 0.6,
		"⬆ Escalation required — assign to senior agent",
		"  No escalation needed")

	fmt.Println()
	fmt.Printf("Model:  %s\n", resp.Model)
	if resp.Usage.InputTokens != nil {
		fmt.Printf("Tokens: %d in / %d out\n",
			*resp.Usage.InputTokens, *resp.Usage.OutputTokens)
	}
}

func printNoul(resp *typesafe.SystemOneResponse, name string, threshold float64, yesMsg, noMsg string) {
	a, ok := resp.Nouls()[name]
	if !ok {
		return
	}
	if a.Noul >= threshold {
		fmt.Printf("  %s  (p=%.2f)\n", yesMsg, a.Noul)
	} else {
		fmt.Printf("  %s  (p=%.2f)\n", noMsg, a.Noul)
	}
}
