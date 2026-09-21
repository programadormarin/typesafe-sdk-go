package typesafe

import (
	"encoding/json"
	"fmt"
)

// Question is a sealed interface satisfied by [Noul], [Choice], and [Score].
// Use one of those concrete types; do not implement this interface directly.
type Question interface {
	isQuestion()
	// questionType returns the wire "type" discriminator string.
	questionType() string
	// validate checks that the question is well-formed before sending.
	validate(name string) error
}

// ─── Noul ────────────────────────────────────────────────────────────────────

// Noul is a yes/no question. The answer is a probability between 0 (no) and 1 (yes).
//
// See https://docs.typesafe.ai/primitives/noul for details.
type Noul struct {
	// Instructions is the yes/no question to ask. May be a string, map, or slice.
	Instructions any `json:"instructions,omitempty"`

	// Criteria optionally describes what a yes and a no mean.
	Criteria *NoulCriteria `json:"criteria,omitempty"`
}

// NoulCriteria describes the yes and no outcomes of a [Noul] question.
type NoulCriteria struct {
	// True describes what a yes (probability near 1) means.
	True any `json:"true,omitempty"`
	// False describes what a no (probability near 0) means.
	False any `json:"false,omitempty"`
}

func (Noul) isQuestion()          {}
func (Noul) questionType() string { return "noul" }
func (n Noul) validate(_ string) error {
	return nil // instructions are optional for Noul
}

// MarshalJSON injects the "type" discriminator into the wire form.
func (n Noul) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type         string        `json:"type"`
		Instructions any           `json:"instructions,omitempty"`
		Criteria     *NoulCriteria `json:"criteria,omitempty"`
	}
	return json.Marshal(wire{
		Type:         "noul",
		Instructions: n.Instructions,
		Criteria:     n.Criteria,
	})
}

// ─── Choice ──────────────────────────────────────────────────────────────────

// Choice is a question that selects one option from a set you define. The answer
// includes the winning option and the full probability distribution.
//
// See https://docs.typesafe.ai/primitives/choice for details.
type Choice struct {
	// Instructions describes what the model should decide.
	// May be a string, map, or slice.
	Instructions any `json:"instructions,omitempty"`

	// Criteria maps each option name to an optional description (nil is allowed).
	// At least one option is required; a maximum of 255 options is accepted by the API.
	Criteria map[string]any `json:"criteria"`
}

func (Choice) isQuestion()          {}
func (Choice) questionType() string { return "choice" }
func (c Choice) validate(name string) error {
	if len(c.Criteria) == 0 {
		return fmt.Errorf("choice question %q: Criteria must have at least one option", name)
	}
	return nil
}

// MarshalJSON injects the "type" discriminator into the wire form.
func (c Choice) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type         string         `json:"type"`
		Instructions any            `json:"instructions,omitempty"`
		Criteria     map[string]any `json:"criteria"`
	}
	return json.Marshal(wire{
		Type:         "choice",
		Instructions: c.Instructions,
		Criteria:     c.Criteria,
	})
}

// ─── Score ────────────────────────────────────────────────────────────────────

// Score is a question that rates the state along an ordered rubric. The answer is a
// probability-weighted value that may land between levels.
//
// See https://docs.typesafe.ai/primitives/score for details.
type Score struct {
	// Instructions describes what the model should rate.
	// May be a string, map, or slice.
	Instructions any `json:"instructions,omitempty"`

	// Criteria is an ordered slice of level descriptions (at least 1, max 10).
	// Each element may be a string, map, or slice.
	Criteria []any `json:"criteria"`
}

func (Score) isQuestion()          {}
func (Score) questionType() string { return "score" }
func (s Score) validate(name string) error {
	if len(s.Criteria) == 0 {
		return fmt.Errorf("score question %q: Criteria must have at least one level", name)
	}
	return nil
}

// MarshalJSON injects the "type" discriminator into the wire form.
func (s Score) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type         string `json:"type"`
		Instructions any    `json:"instructions,omitempty"`
		Criteria     []any  `json:"criteria"`
	}
	return json.Marshal(wire{
		Type:         "score",
		Instructions: s.Instructions,
		Criteria:     s.Criteria,
	})
}

// ─── validation helper ───────────────────────────────────────────────────────

// validateQuestions checks that the map is non-empty and that each question is valid.
func validateQuestions(questions map[string]Question) error {
	if len(questions) == 0 {
		return &TypeSafeError{Message: "at least one question is required"}
	}
	for name, q := range questions {
		if q == nil {
			return &TypeSafeError{Message: fmt.Sprintf("question %q is nil", name)}
		}
		if err := q.validate(name); err != nil {
			return &TypeSafeError{Message: err.Error()}
		}
	}
	return nil
}
