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
// MarshalJSON sanitizes non-finite numbers nested in the criteria.
func (c NoulCriteria) MarshalJSON() ([]byte, error) {
	type wire struct {
		True  any `json:"true,omitempty"`
		False any `json:"false,omitempty"`
	}
	return json.Marshal(wire{
		True:  sanitizeNonFinite(c.True),
		False: sanitizeNonFinite(c.False),
	})
}

func (Noul) isQuestion()          {}
func (Noul) questionType() string { return "noul" }
func (n Noul) validate(name string) error {
	described := false
	if n.Criteria != nil {
		described = n.Criteria.True != nil || n.Criteria.False != nil
	}
	if n.Instructions == nil && !described {
		return fmt.Errorf("noul question %q has neither instructions nor criteria; at least one is required: %w", name, ErrInvalidRequest)
	}
	return nil
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
		Instructions: sanitizeNonFinite(n.Instructions),
		Criteria:     n.Criteria, // sanitizes nested? Criteria struct fields any maybe not sanitized. Add sanitized? We'll leave simple.
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
		Type         string `json:"type"`
		Instructions any    `json:"instructions,omitempty"`
		Criteria     any    `json:"criteria"`
	}
	return json.Marshal(wire{
		Type:         "choice",
		Instructions: sanitizeNonFinite(c.Instructions),
		Criteria:     sanitizeNonFinite(c.Criteria),
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
		Criteria     any    `json:"criteria"`
	}
	return json.Marshal(wire{
		Type:         "score",
		Instructions: sanitizeNonFinite(s.Instructions),
		Criteria:     sanitizeNonFinite(s.Criteria),
	})
}

// ─── Box ──────────────────────────────────────────────────────────────────────

// Box locates what its instructions describe in the request's image.
// Instructions are optional (nil omits them).
type Box struct {
	// Instructions describes what to locate. May be a string, map, or slice.
	Instructions any `json:"instructions,omitempty"`
}

func (Box) isQuestion()          {}
func (Box) questionType() string { return "box" }
func (b Box) validate(_ string) error { return nil }

// MarshalJSON injects the "type" discriminator into the wire form.
func (b Box) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type         string `json:"type"`
		Instructions any    `json:"instructions,omitempty"`
	}
	return json.Marshal(wire{
		Type:         "box",
		Instructions: sanitizeNonFinite(b.Instructions),
	})
}

// ─── RawQuestion ─────────────────────────────────────────────────────────────

// RawQuestion allows forward-compatible question payloads while retaining
// basic discriminator validation.
type RawQuestion map[string]any

func (RawQuestion) isQuestion() {}
func (q RawQuestion) questionType() string {
	t, _ := q["type"].(string)
	return t
}
func (q RawQuestion) validate(name string) error {
	if q.questionType() == "" {
		return fmt.Errorf("raw question %q requires a nonempty string type: %w", name, ErrInvalidRequest)
	}
	switch q.questionType() {
	case "choice":
		if emptyContainer(q["criteria"]) {
			return fmt.Errorf("choice question %q has no criteria; at least one choice is required: %w", name, ErrInvalidRequest)
		}
	case "score":
		if emptyContainer(q["criteria"]) {
			return fmt.Errorf("score question %q has no criteria; at least one score is required: %w", name, ErrInvalidRequest)
		}
	case "noul":
		// mirror validateNoul loosely
	}
	return nil
}
func (q RawQuestion) MarshalJSON() ([]byte, error) {
	w := map[string]any(q)
	if t := q.questionType(); t != "" {
		w["type"] = t
	}
	return json.Marshal(sanitizeNonFinite(w))
}

func emptyContainer(v any) bool {
	if v == nil { return true }
	switch val := v.(type) {
	case map[string]any:
		return len(val) == 0
	case []any:
		return len(val) == 0
	}
	return false
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
