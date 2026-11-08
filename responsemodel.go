package typesafe

import (
	"encoding/json"
	"errors"
	"fmt"
)

var knownAnswerTypes = map[string]struct{}{
	"noul":   {},
	"choice": {},
	"score":  {},
	"box":    {},
}

func liftAnswers(data []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode response object: %w", err)
	}
	rawAnswers, ok := root["answers"]
	if !ok {
		return data, nil
	}
	var answers map[string]json.RawMessage
	if err := json.Unmarshal(rawAnswers, &answers); err != nil {
		return nil, fmt.Errorf("decode answers object: %w", err)
	}
	kept := make(map[string]json.RawMessage, len(answers))
	for name, raw := range answers {
		var probe struct {
			Type *string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil || probe.Type == nil {
			return nil, fmt.Errorf("answers.%s.type: %w", name, ErrInvalidResponse)
		}
		if _, known := knownAnswerTypes[*probe.Type]; !known {
			continue
		}
		kept[name] = raw
		switch name {
		case "model", "usage", "answers":
			continue
		}
		if _, exists := root[name]; exists {
			continue
		}
		root[name] = raw
	}
	pruned, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	root["answers"] = pruned
	return json.Marshal(root)
}

func validationCause(err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return fmt.Errorf("field %q: cannot decode %s into %s: %w", typeErr.Field, typeErr.Value, typeErr.Type, ErrInvalidResponse)
	}
	return fmt.Errorf("decode response into typed model: %w: %w", err, ErrInvalidResponse)
}
