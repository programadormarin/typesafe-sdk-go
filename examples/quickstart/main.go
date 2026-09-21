// Quickstart example for the TypeSafe Go SDK.
//
// Demonstrates the three question primitives (Noul, Choice, Score) in a single
// System One call. Set TYPESAFE_API_KEY before running:
//
//	TYPESAFE_API_KEY=your_key go run ./examples/quickstart
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	typesafe "github.com/programadormarin/typesafe-sdk-go"
)

func main() {
	// New() reads TYPESAFE_API_KEY from the environment automatically.
	// Pass typesafe.WithAPIKey("...") to supply the key explicitly.
	client, err := typesafe.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}

	// state is the content the model will evaluate.
	// It can be a plain string, a map, a slice — any JSON-serialisable value.
	state := map[string]any{
		"document": "I was charged twice for the same order. Please fix this ASAP.",
	}

	// questions is a map of named typed questions.
	// All three run in a single API call.
	questions := map[string]typesafe.Question{
		// Noul: yes/no probability — "is this about billing?"
		"billing": typesafe.Noul{
			Instructions: "Is this support ticket about a billing issue?",
		},

		// Choice: pick one from a defined set — "what is the customer's tone?"
		"tone": typesafe.Choice{
			Instructions: "What is the customer's overall tone?",
			Criteria: map[string]any{
				"calm":       "Polite, neutral language",
				"frustrated": "Clearly annoyed but not hostile",
				"angry":      "Hostile, demanding, or threatening",
			},
		},

		// Score: rate along an ordered rubric — "how urgent is this?"
		"urgency": typesafe.Score{
			Instructions: "How urgently does this ticket need attention?",
			Criteria:     []any{"can wait", "handle this week", "needs same-day attention"},
		},
	}

	resp, err := client.SystemOne(context.Background(), state, questions)
	if err != nil {
		log.Fatalf("SystemOne: %v", err)
	}

	// Read the Noul answer (probability 0–1).
	billing := resp.Nouls()["billing"]
	fmt.Printf("Billing issue?   %.2f  (%.0f%% likely yes)\n",
		billing.Noul, billing.Noul*100)

	// Read the Choice answer.
	tone := resp.Choices()["tone"]
	fmt.Printf("Customer tone:   %s  (confidence %.2f)\n",
		tone.Choice, tone.Confidence)
	fmt.Println("  Probabilities:")
	for option, prob := range tone.Probabilities {
		fmt.Printf("    %-12s %.2f\n", option, prob)
	}

	// Read the Score answer (value may fall between levels).
	urgency := resp.Scores()["urgency"]
	fmt.Printf("Urgency score:   %.2f  (confidence %.2f)\n",
		urgency.Score, urgency.Confidence)
	fmt.Println("  Rubric:")
	for i := 0; i < len(urgency.Legend); i++ {
		fmt.Printf("    %d = %v  (p=%.2f)\n",
			i, urgency.Legend[i], urgency.Probabilities[i])
	}

	fmt.Printf("\nModel: %s\n", resp.Model)
	if resp.Usage.InputTokens != nil {
		fmt.Printf("Tokens: %d in / %d out\n",
			*resp.Usage.InputTokens, *resp.Usage.OutputTokens)
	}
}
