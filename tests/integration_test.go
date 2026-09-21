//go:build integration

package typesafe_test

import (
	"context"
	"errors"
	"os"
	"testing"

	typesafe "github.com/programadormarin/typesafe-sdk-go"
)

// Integration tests make real API calls. Run with:
//
//	TYPESAFE_API_KEY=your_key go test -tags integration ./...

func TestIntegration_SystemOne_noul(t *testing.T) {
	client := integrationClient(t)

	resp, err := client.SystemOne(
		context.Background(),
		"Help! My payouts have been failing for 3 days.",
		map[string]typesafe.Question{
			"is_urgent": typesafe.Noul{Instructions: "Does this convey urgency?"},
		},
	)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}

	a, ok := resp.Nouls()["is_urgent"]
	if !ok {
		t.Fatal("expected noul answer for 'is_urgent'")
	}
	if a.Noul < 0 || a.Noul > 1 {
		t.Errorf("noul = %v, want [0, 1]", a.Noul)
	}
	t.Logf("is_urgent noul = %.3f", a.Noul)
}

func TestIntegration_SystemOne_choice(t *testing.T) {
	client := integrationClient(t)

	resp, err := client.SystemOne(
		context.Background(),
		"Help! My payouts have been failing for 3 days.",
		map[string]typesafe.Question{
			"department": typesafe.Choice{
				Instructions: "Which team should handle this?",
				Criteria: map[string]any{
					"billing":   "Payments, invoicing, refunds",
					"technical": "Bugs, outages, integrations",
					"sales":     "Pricing, upgrades, new accounts",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}

	a, ok := resp.Choices()["department"]
	if !ok {
		t.Fatal("expected choice answer for 'department'")
	}
	if a.Choice == "" {
		t.Error("choice answer should not be empty")
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		t.Errorf("confidence = %v, want [0, 1]", a.Confidence)
	}
	t.Logf("department = %s (confidence %.3f)", a.Choice, a.Confidence)
}

func TestIntegration_SystemOne_score(t *testing.T) {
	client := integrationClient(t)

	resp, err := client.SystemOne(
		context.Background(),
		"Help! My payouts have been failing for 3 days.",
		map[string]typesafe.Question{
			"frustration": typesafe.Score{
				Instructions: "How frustrated is the customer?",
				Criteria:     []any{"Calm", "Frustrated", "Very angry"},
			},
		},
	)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}

	a, ok := resp.Scores()["frustration"]
	if !ok {
		t.Fatal("expected score answer for 'frustration'")
	}
	if a.Score < 0 || a.Score > 2 {
		t.Errorf("score = %v, want [0, 2]", a.Score)
	}
	if len(a.Legend) != 3 {
		t.Errorf("legend len = %d, want 3", len(a.Legend))
	}
	t.Logf("frustration score = %.3f (confidence %.3f)", a.Score, a.Confidence)
}

func TestIntegration_SystemOne_allPrimitives(t *testing.T) {
	client := integrationClient(t)

	resp, err := client.SystemOne(
		context.Background(),
		map[string]any{
			"document": "I was charged twice. Please fix this ASAP.",
		},
		map[string]typesafe.Question{
			"billing": typesafe.Noul{Instructions: "Is this about billing?"},
			"tone": typesafe.Choice{
				Instructions: "What is the customer's tone?",
				Criteria:     map[string]any{"calm": nil, "frustrated": nil, "angry": nil},
			},
			"urgency": typesafe.Score{
				Instructions: "How urgent is this?",
				Criteria:     []any{"can wait", "this week", "today"},
			},
		},
	)
	if err != nil {
		t.Fatalf("SystemOne: %v", err)
	}

	if len(resp.Nouls()) != 1 {
		t.Errorf("Nouls() len = %d, want 1", len(resp.Nouls()))
	}
	if len(resp.Choices()) != 1 {
		t.Errorf("Choices() len = %d, want 1", len(resp.Choices()))
	}
	if len(resp.Scores()) != 1 {
		t.Errorf("Scores() len = %d, want 1", len(resp.Scores()))
	}
	if resp.Model == "" {
		t.Error("Model should not be empty")
	}
	t.Logf("model=%s billing=%.2f tone=%s urgency=%.2f",
		resp.Model,
		resp.Nouls()["billing"].Noul,
		resp.Choices()["tone"].Choice,
		resp.Scores()["urgency"].Score,
	)
}

func TestIntegration_Models(t *testing.T) {
	client := integrationClient(t)

	resp, err := client.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(resp.Models) == 0 {
		t.Error("expected at least one model")
	}
	for _, m := range resp.Models {
		if m.Name == "" {
			t.Error("model name should not be empty")
		}
		t.Logf("model: %s — %s", m.Name, m.Description)
	}
}

func TestIntegration_InvalidAPIKey_returnsAuthError(t *testing.T) {
	client, err := typesafe.New(
		typesafe.WithAPIKey("invalid-key"),
		typesafe.WithRetry(typesafe.RetryPolicy{MaxRetries: 0}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.SystemOne(context.Background(), "x", map[string]typesafe.Question{
		"q": typesafe.Noul{Instructions: "x"},
	})
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
	var authErr *typesafe.TypeSafeAuthenticationError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *TypeSafeAuthenticationError, got %T: %v", err, err)
	}
}

// integrationClient creates a client for integration tests.
// It skips the test if TYPESAFE_API_KEY is not set.
func integrationClient(t *testing.T) *typesafe.Client {
	t.Helper()
	key := os.Getenv(typesafe.EnvAPIKey)
	if key == "" {
		t.Skipf("skipping integration test: %s not set", typesafe.EnvAPIKey)
	}
	client, err := typesafe.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}
