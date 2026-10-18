package emotion

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
)

// Analysis is the private classifier protocol, never a conversational Message.
type Analysis struct {
	Emotion    string  `json:"emotion"`
	Intensity  float64 `json:"intensity"`
	Confidence float64 `json:"confidence"`
	DurationMS int     `json:"duration_ms"`
}

// Labels is the automatic classifier vocabulary. Neutral is the safe idle fallback.
var Labels = automatic()

const MinimumConfidence = .72

func Neutral() Analysis { return Analysis{Emotion: "neutral"} }
func (a Analysis) Valid() bool {
	return Known(a.Emotion) && !math.IsNaN(a.Intensity) && !math.IsInf(a.Intensity, 0) && !math.IsNaN(a.Confidence) && !math.IsInf(a.Confidence, 0) && a.Intensity >= 0 && a.Intensity <= 1 && a.Confidence >= 0 && a.Confidence <= 1 && a.DurationMS >= 0 && a.DurationMS <= 30000 && (a.Emotion == "neutral" || a.DurationMS >= 500)
}

// Require four unique fields; reject fences, unknown fields, nulls and trailing data.
func ParseAnalysis(text string) (Analysis, error) {
	bad := errors.New("invalid_json")
	if len(text) > 4096 {
		return Neutral(), bad
	}
	d := json.NewDecoder(strings.NewReader(text))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return Neutral(), bad
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		key, e := d.Token()
		if e != nil {
			return Neutral(), bad
		}
		k, ok := key.(string)
		if !ok {
			return Neutral(), bad
		}
		if _, exists := fields[k]; exists {
			return Neutral(), bad
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || string(value) == "null" {
			return Neutral(), bad
		}
		fields[k] = value
	}
	if _, e = d.Token(); e != nil {
		return Neutral(), bad
	}
	if _, e = d.Token(); e != io.EOF || len(fields) != 4 {
		return Neutral(), bad
	}
	var a Analysis
	for k, raw := range fields {
		switch k {
		case "emotion":
			e = json.Unmarshal(raw, &a.Emotion)
		case "intensity":
			e = json.Unmarshal(raw, &a.Intensity)
		case "confidence":
			e = json.Unmarshal(raw, &a.Confidence)
		case "duration_ms":
			e = json.Unmarshal(raw, &a.DurationMS)
		default:
			return Neutral(), bad
		}
		if e != nil {
			return Neutral(), bad
		}
	}
	automatic := false
	for _, label := range Labels {
		automatic = automatic || a.Emotion == label
	}
	if !a.Valid() || !automatic {
		return Neutral(), errors.New("invalid_value")
	}
	return a, nil
}
