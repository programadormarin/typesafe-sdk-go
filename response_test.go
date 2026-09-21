package typesafe

import (
	"encoding/json"
	"testing"
)

// ─── NoulAnswer ───────────────────────────────────────────────────────────────

func TestSystemOneResponse_noulAnswer(t *testing.T) {
	raw := `{
		"model": "jev-1.13.0",
		"answers": {
			"billing": {"type": "noul", "noul": 0.95}
		},
		"usage": {"input_tokens": 100, "output_tokens": 10}
	}`

	resp := decodeSystemOneResponse(t, raw)

	nouls := resp.Nouls()
	if len(nouls) != 1 {
		t.Fatalf("Nouls() len = %d, want 1", len(nouls))
	}
	a, ok := nouls["billing"]
	if !ok {
		t.Fatal("Nouls()[\"billing\"] not found")
	}
	if a.Noul != 0.95 {
		t.Errorf("Noul = %v, want 0.95", a.Noul)
	}
	if resp.Model != "jev-1.13.0" {
		t.Errorf("Model = %q, want %q", resp.Model, "jev-1.13.0")
	}
}

// ─── ChoiceAnswer ─────────────────────────────────────────────────────────────

func TestSystemOneResponse_choiceAnswer(t *testing.T) {
	raw := `{
		"model": "jev-1.13.0",
		"answers": {
			"tone": {
				"type": "choice",
				"choice": "frustrated",
				"probabilities": {"calm": 0.05, "frustrated": 0.88, "angry": 0.07},
				"confidence": 0.82
			}
		},
		"usage": {}
	}`

	resp := decodeSystemOneResponse(t, raw)

	choices := resp.Choices()
	if len(choices) != 1 {
		t.Fatalf("Choices() len = %d, want 1", len(choices))
	}
	a := choices["tone"]
	if a.Choice != "frustrated" {
		t.Errorf("Choice = %q, want %q", a.Choice, "frustrated")
	}
	if a.Confidence != 0.82 {
		t.Errorf("Confidence = %v, want 0.82", a.Confidence)
	}
	if a.Probabilities["calm"] != 0.05 {
		t.Errorf("Probabilities[calm] = %v, want 0.05", a.Probabilities["calm"])
	}
}

// ─── ScoreAnswer ─────────────────────────────────────────────────────────────

func TestSystemOneResponse_scoreAnswer(t *testing.T) {
	raw := `{
		"model": "jev-1.13.0",
		"answers": {
			"urgency": {
				"type": "score",
				"score": 1.05,
				"legend": {"0": "Calm", "1": "Frustrated", "2": "Very angry"},
				"probabilities": {"0": 0.0, "1": 0.95, "2": 0.05},
				"confidence": 0.92
			}
		},
		"usage": {"input_tokens": 304, "output_tokens": 18}
	}`

	resp := decodeSystemOneResponse(t, raw)

	scores := resp.Scores()
	if len(scores) != 1 {
		t.Fatalf("Scores() len = %d, want 1", len(scores))
	}
	a := scores["urgency"]
	if a.Score != 1.05 {
		t.Errorf("Score = %v, want 1.05", a.Score)
	}
	if a.Confidence != 0.92 {
		t.Errorf("Confidence = %v, want 0.92", a.Confidence)
	}
	// Legend keys must be coerced from string to int.
	if a.Legend[0] != "Calm" {
		t.Errorf("Legend[0] = %v, want %q", a.Legend[0], "Calm")
	}
	if a.Legend[1] != "Frustrated" {
		t.Errorf("Legend[1] = %v, want %q", a.Legend[1], "Frustrated")
	}
	// Probability keys must also be coerced.
	if a.Probabilities[1] != 0.95 {
		t.Errorf("Probabilities[1] = %v, want 0.95", a.Probabilities[1])
	}
}

// ─── Mixed answers ────────────────────────────────────────────────────────────

func TestSystemOneResponse_mixedAnswers(t *testing.T) {
	raw := `{
		"model": "jev-1.13.0",
		"answers": {
			"is_billing": {"type": "noul", "noul": 0.9},
			"dept": {
				"type": "choice",
				"choice": "billing",
				"probabilities": {"billing": 0.9, "tech": 0.1},
				"confidence": 0.85
			},
			"severity": {
				"type": "score",
				"score": 2.0,
				"legend": {"0": "low", "1": "mid", "2": "high"},
				"probabilities": {"0": 0.0, "1": 0.0, "2": 1.0},
				"confidence": 1.0
			}
		},
		"usage": {}
	}`

	resp := decodeSystemOneResponse(t, raw)

	if len(resp.Answers) != 3 {
		t.Fatalf("Answers len = %d, want 3", len(resp.Answers))
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
}

// ─── Forward-compat: unknown answer type is silently dropped ─────────────────

func TestSystemOneResponse_unknownTypeSkipped(t *testing.T) {
	raw := `{
		"model": "jev-future",
		"answers": {
			"known":   {"type": "noul", "noul": 0.7},
			"unknown": {"type": "embedding", "vector": [0.1, 0.2]}
		},
		"usage": {}
	}`

	resp := decodeSystemOneResponse(t, raw)

	// Only the known answer must survive.
	if len(resp.Answers) != 1 {
		t.Errorf("Answers len = %d, want 1 (unknown type should be dropped)", len(resp.Answers))
	}
	if _, ok := resp.Answers["known"]; !ok {
		t.Error("known answer must be present")
	}
}

// ─── Usage fields ─────────────────────────────────────────────────────────────

func TestSystemOneResponse_usagePopulated(t *testing.T) {
	raw := `{
		"model": "jev-1.13.0",
		"answers": {},
		"usage": {"input_tokens": 200, "output_tokens": 30}
	}`
	resp := decodeSystemOneResponse(t, raw)

	if resp.Usage.InputTokens == nil || *resp.Usage.InputTokens != 200 {
		t.Errorf("InputTokens = %v, want 200", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens == nil || *resp.Usage.OutputTokens != 30 {
		t.Errorf("OutputTokens = %v, want 30", resp.Usage.OutputTokens)
	}
}

func TestSystemOneResponse_usageNilWhenAbsent(t *testing.T) {
	raw := `{"model": "jev-1.13.0", "answers": {}, "usage": {}}`
	resp := decodeSystemOneResponse(t, raw)
	if resp.Usage.InputTokens != nil {
		t.Errorf("InputTokens should be nil when not in response")
	}
}

// ─── decodeIntKeyedMap ────────────────────────────────────────────────────────

func TestDecodeIntKeyedMap_valid(t *testing.T) {
	raw := json.RawMessage(`{"0": "zero", "1": "one", "2": "two"}`)
	got, err := decodeIntKeyedMap[string](raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0] != "zero" || got[1] != "one" || got[2] != "two" {
		t.Errorf("got = %v", got)
	}
}

func TestDecodeIntKeyedMap_nonIntegerKeyReturnsError(t *testing.T) {
	raw := json.RawMessage(`{"level_0": "x"}`)
	_, err := decodeIntKeyedMap[string](raw)
	if err == nil {
		t.Fatal("expected error for non-integer key")
	}
}

func TestDecodeIntKeyedMap_nilRawReturnsNil(t *testing.T) {
	got, err := decodeIntKeyedMap[string](nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map, got %v", got)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func decodeSystemOneResponse(t *testing.T, raw string) *SystemOneResponse {
	t.Helper()
	var resp SystemOneResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal SystemOneResponse: %v", err)
	}
	return &resp
}
