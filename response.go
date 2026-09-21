package typesafe

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

// ─── Answer types ─────────────────────────────────────────────────────────────

// Answer is the sealed interface implemented by [NoulAnswer], [ChoiceAnswer], and [ScoreAnswer].
type Answer interface {
	answerType() string
}

// NoulAnswer is the response to a [Noul] question.
type NoulAnswer struct {
	// Noul is the probability that the answer is yes, from 0 (no) to 1 (yes).
	Noul float64 `json:"noul"`
}

func (NoulAnswer) answerType() string { return "noul" }

// ChoiceAnswer is the response to a [Choice] question.
type ChoiceAnswer struct {
	// Choice is the highest-probability option name.
	Choice string `json:"choice"`

	// Probabilities maps every option to its probability. Values sum to 1.
	Probabilities map[string]float64 `json:"probabilities"`

	// Confidence summarises how concentrated the distribution is (0–1).
	Confidence float64 `json:"confidence"`
}

func (ChoiceAnswer) answerType() string { return "choice" }

// ScoreAnswer is the response to a [Score] question.
type ScoreAnswer struct {
	// Score is the probability-weighted position across the levels; may fall between levels.
	Score float64 `json:"score"`

	// Legend maps integer level indices to their descriptions.
	Legend map[int]any `json:"legend"`

	// Probabilities maps integer level indices to their probabilities. Values sum to 1.
	Probabilities map[int]float64 `json:"probabilities"`

	// Confidence summarises how concentrated the distribution is (0–1).
	Confidence float64 `json:"confidence"`
}

func (ScoreAnswer) answerType() string { return "score" }

// ─── Wire answer (for JSON unmarshalling) ────────────────────────────────────

// wireAnswer is a raw decoded answer before type dispatch.
type wireAnswer struct {
	Type string `json:"type"`
	// Noul answer fields
	Noul *float64 `json:"noul"`
	// Choice answer fields
	Choice        *string         `json:"choice"`
	Probabilities json.RawMessage `json:"probabilities"`
	Confidence    *float64        `json:"confidence"`
	// Score answer fields
	Score  *float64        `json:"score"`
	Legend json.RawMessage `json:"legend"`
}

// toAnswer converts the raw wire answer to the appropriate typed Answer.
// Returns nil, nil for unrecognised types (forward-compat).
func (w wireAnswer) toAnswer() (Answer, error) {
	switch w.Type {
	case "noul":
		if w.Noul == nil {
			return nil, fmt.Errorf("noul answer missing 'noul' field")
		}
		return &NoulAnswer{Noul: *w.Noul}, nil

	case "choice":
		if w.Choice == nil {
			return nil, fmt.Errorf("choice answer missing 'choice' field")
		}
		if w.Confidence == nil {
			return nil, fmt.Errorf("choice answer missing 'confidence' field")
		}
		var probs map[string]float64
		if w.Probabilities != nil {
			if err := json.Unmarshal(w.Probabilities, &probs); err != nil {
				return nil, fmt.Errorf("choice answer: invalid 'probabilities': %w", err)
			}
		}
		return &ChoiceAnswer{
			Choice:        *w.Choice,
			Probabilities: probs,
			Confidence:    *w.Confidence,
		}, nil

	case "score":
		if w.Score == nil {
			return nil, fmt.Errorf("score answer missing 'score' field")
		}
		if w.Confidence == nil {
			return nil, fmt.Errorf("score answer missing 'confidence' field")
		}
		legend, err := decodeIntKeyedMap[any](w.Legend)
		if err != nil {
			return nil, fmt.Errorf("score answer: invalid 'legend': %w", err)
		}
		probs, err := decodeIntKeyedMap[float64](w.Probabilities)
		if err != nil {
			return nil, fmt.Errorf("score answer: invalid 'probabilities': %w", err)
		}
		return &ScoreAnswer{
			Score:         *w.Score,
			Legend:        legend,
			Probabilities: probs,
			Confidence:    *w.Confidence,
		}, nil

	default:
		// Unknown answer type — silently skip for forward-compat.
		return nil, nil
	}
}

// decodeIntKeyedMap decodes a JSON object whose string keys are integer indices
// (e.g. {"0": ..., "1": ...}) into a map[int]V.
func decodeIntKeyedMap[V any](raw json.RawMessage) (map[int]V, error) {
	if raw == nil {
		return nil, nil
	}
	var strKeyed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &strKeyed); err != nil {
		return nil, err
	}
	out := make(map[int]V, len(strKeyed))
	for k, v := range strKeyed {
		idx, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("expected integer key, got %q", k)
		}
		var val V
		if err := json.Unmarshal(v, &val); err != nil {
			return nil, fmt.Errorf("key %d: %w", idx, err)
		}
		out[idx] = val
	}
	return out, nil
}

// ─── SystemOneResponse ────────────────────────────────────────────────────────

// SystemOneResponse holds all answers for a System One request, plus model and usage metadata.
type SystemOneResponse struct {
	// Model is the model that performed the evaluation.
	Model string `json:"model"`

	// Answers maps question names to their typed answers.
	Answers map[string]Answer

	// Usage contains token counts for the request.
	Usage Usage `json:"usage"`

	// once guards lazy accessor initialisation.
	once    sync.Once
	nouls   map[string]*NoulAnswer
	choices map[string]*ChoiceAnswer
	scores  map[string]*ScoreAnswer
}

// Nouls returns all NoulAnswer values keyed by question name.
func (r *SystemOneResponse) Nouls() map[string]*NoulAnswer {
	r.buildAccessors()
	return r.nouls
}

// Choices returns all ChoiceAnswer values keyed by question name.
func (r *SystemOneResponse) Choices() map[string]*ChoiceAnswer {
	r.buildAccessors()
	return r.choices
}

// Scores returns all ScoreAnswer values keyed by question name.
func (r *SystemOneResponse) Scores() map[string]*ScoreAnswer {
	r.buildAccessors()
	return r.scores
}

func (r *SystemOneResponse) buildAccessors() {
	r.once.Do(func() {
		r.nouls = make(map[string]*NoulAnswer)
		r.choices = make(map[string]*ChoiceAnswer)
		r.scores = make(map[string]*ScoreAnswer)
		for name, a := range r.Answers {
			switch v := a.(type) {
			case *NoulAnswer:
				r.nouls[name] = v
			case *ChoiceAnswer:
				r.choices[name] = v
			case *ScoreAnswer:
				r.scores[name] = v
			}
		}
	})
}

// UnmarshalJSON implements a custom decoder that dispatches each answer by "type".
func (r *SystemOneResponse) UnmarshalJSON(data []byte) error {
	// Use a shadow struct to capture the raw answers map without recursion.
	var raw struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
		Usage   Usage                      `json:"usage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Model = raw.Model
	r.Usage = raw.Usage
	r.Answers = make(map[string]Answer, len(raw.Answers))

	for name, rawAnswer := range raw.Answers {
		var w wireAnswer
		if err := json.Unmarshal(rawAnswer, &w); err != nil {
			return fmt.Errorf("answers.%s: %w", name, err)
		}
		answer, err := w.toAnswer()
		if err != nil {
			return fmt.Errorf("answers.%s: %w", name, err)
		}
		if answer == nil {
			// Unknown type — skip for forward-compat.
			continue
		}
		r.Answers[name] = answer
	}
	return nil
}

// ─── Usage ────────────────────────────────────────────────────────────────────

// Usage contains token counts reported by the API for a request.
type Usage struct {
	// InputTokens is the number of input tokens used, or nil if not reported.
	InputTokens *int `json:"input_tokens"`
	// OutputTokens is the number of output tokens used, or nil if not reported.
	OutputTokens *int `json:"output_tokens"`
}
