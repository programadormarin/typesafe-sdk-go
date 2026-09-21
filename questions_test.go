package typesafe

import (
	"encoding/json"
	"testing"
)

// ─── MarshalJSON injects the "type" discriminator ─────────────────────────────

func TestNoulMarshalJSON_typeInjected(t *testing.T) {
	n := Noul{Instructions: "Is this urgent?"}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, b, &got)

	if got["type"] != "noul" {
		t.Errorf("type = %q, want %q", got["type"], "noul")
	}
	if got["instructions"] != "Is this urgent?" {
		t.Errorf("instructions = %v, want %q", got["instructions"], "Is this urgent?")
	}
}

func TestNoulMarshalJSON_omitsNilFields(t *testing.T) {
	n := Noul{}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, b, &got)

	if _, ok := got["instructions"]; ok {
		t.Error("instructions should be omitted when nil")
	}
	if _, ok := got["criteria"]; ok {
		t.Error("criteria should be omitted when nil")
	}
}

func TestNoulMarshalJSON_withCriteria(t *testing.T) {
	n := Noul{
		Instructions: "Is this urgent?",
		Criteria:     &NoulCriteria{True: "Very urgent", False: "Not urgent"},
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, b, &got)

	criteria, ok := got["criteria"].(map[string]any)
	if !ok {
		t.Fatalf("criteria is %T, want map", got["criteria"])
	}
	if criteria["true"] != "Very urgent" {
		t.Errorf("criteria.true = %v, want %q", criteria["true"], "Very urgent")
	}
}

func TestChoiceMarshalJSON_typeAndCriteria(t *testing.T) {
	c := Choice{
		Instructions: "What is the tone?",
		Criteria:     map[string]any{"calm": nil, "angry": "clearly hostile"},
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, b, &got)

	if got["type"] != "choice" {
		t.Errorf("type = %q, want %q", got["type"], "choice")
	}
	criteria, ok := got["criteria"].(map[string]any)
	if !ok {
		t.Fatalf("criteria is %T, want map", got["criteria"])
	}
	if _, ok := criteria["calm"]; !ok {
		t.Error("criteria.calm should be present (nil value)")
	}
	if criteria["angry"] != "clearly hostile" {
		t.Errorf("criteria.angry = %v, want %q", criteria["angry"], "clearly hostile")
	}
}

func TestScoreMarshalJSON_typeAndCriteria(t *testing.T) {
	s := Score{
		Instructions: "How urgent?",
		Criteria:     []any{"can wait", "this week", "today"},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, b, &got)

	if got["type"] != "score" {
		t.Errorf("type = %q, want %q", got["type"], "score")
	}
	criteria, ok := got["criteria"].([]any)
	if !ok {
		t.Fatalf("criteria is %T, want slice", got["criteria"])
	}
	if len(criteria) != 3 {
		t.Errorf("criteria len = %d, want 3", len(criteria))
	}
}

// ─── validateQuestions ────────────────────────────────────────────────────────

func TestValidateQuestions_emptyReturnsError(t *testing.T) {
	err := validateQuestions(map[string]Question{})
	if err == nil {
		t.Fatal("expected error for empty questions map")
	}
}

func TestValidateQuestions_nilQuestionReturnsError(t *testing.T) {
	err := validateQuestions(map[string]Question{"q": nil})
	if err == nil {
		t.Fatal("expected error for nil question")
	}
}

func TestValidateQuestions_choiceNoCriteriaReturnsError(t *testing.T) {
	err := validateQuestions(map[string]Question{
		"q": Choice{Criteria: map[string]any{}},
	})
	if err == nil {
		t.Fatal("expected error for choice with empty criteria")
	}
}

func TestValidateQuestions_scoreNoCriteriaReturnsError(t *testing.T) {
	err := validateQuestions(map[string]Question{
		"q": Score{Criteria: []any{}},
	})
	if err == nil {
		t.Fatal("expected error for score with empty criteria")
	}
}

func TestValidateQuestions_validPassesThrough(t *testing.T) {
	err := validateQuestions(map[string]Question{
		"a": Noul{Instructions: "yes/no?"},
		"b": Choice{Criteria: map[string]any{"x": nil}},
		"c": Score{Criteria: []any{"low", "high"}},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── structured instructions (map / slice) ───────────────────────────────────

func TestNoulMarshalJSON_structuredInstructions(t *testing.T) {
	n := Noul{
		Instructions: map[string]any{
			"question": "Is this the same person?",
			"context":  map[string]any{"name": "Alice"},
		},
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, b, &got)
	instr, ok := got["instructions"].(map[string]any)
	if !ok {
		t.Fatalf("instructions is %T, want map", got["instructions"])
	}
	if instr["question"] != "Is this the same person?" {
		t.Errorf("instructions.question = %v", instr["question"])
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustUnmarshal(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("unmarshal %q: %v", b, err)
	}
}
