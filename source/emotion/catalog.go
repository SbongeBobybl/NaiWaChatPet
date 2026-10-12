// Package emotion is the facial overlay track for NaiWaChatPet.
// Locomotion owns displacement and facing. This package never writes root motion.
package emotion

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed emotions.json
var catalogJSON []byte

// Priority is the emotion scheduler rank. It is not a locomotion interrupt
// except where the contract explicitly allows a high-priority overlay to
// replace walk/run idle variants.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// Phase is the emotion clip phase. Locomotion does not share this slot.
type Phase string

const (
	PhaseIdle Phase = "idle"
	PhaseIn   Phase = "in"
	PhaseLoop Phase = "loop"
	PhaseOut  Phase = "out"
)

// Emotion is one entry in the forced LLM set and the playback map.
type Emotion struct {
	ID            string `json:"id"`
	Zh            string `json:"zh"`
	Priority      string `json:"priority"`
	Legacy        bool   `json:"legacy"`
	Clip          string `json:"clip"`
	TransitionIn  string `json:"transition_in"`
	TransitionOut string `json:"transition_out"`
	Source        string `json:"source,omitempty"`
	Start         int    `json:"start,omitempty"`
	Step          int    `json:"step,omitempty"`
	Count         int    `json:"count,omitempty"`
	Reverse       bool   `json:"reverse,omitempty"`
	FPS           int    `json:"fps,omitempty"`
	Note          string `json:"note,omitempty"`
}

type catalogFile struct {
	Emotions []Emotion `json:"emotions"`
}

// NeutralID is the forced-choice fallback. It has no clip of its own.
const NeutralID = "neutral"

var (
	parsed   catalogFile
	byID     map[string]Emotion
	parseErr error
)

func init() {
	parseErr = json.Unmarshal(catalogJSON, &parsed)
	byID = make(map[string]Emotion, len(parsed.Emotions))
	for _, em := range parsed.Emotions {
		byID[em.ID] = em
	}
}

// Catalog returns the playable emotions in forced-choice order.
func Catalog() ([]Emotion, error) {
	if parseErr != nil {
		return nil, parseErr
	}
	out := make([]Emotion, len(parsed.Emotions))
	copy(out, parsed.Emotions)
	return out, nil
}

// ByID returns a playable emotion. neutral is not in the catalog.
func ByID(id string) (Emotion, bool) {
	em, ok := byID[id]
	return em, ok
}

// ForcedChoices is the closed set the emotion model must pick from.
// neutral is first; every catalog id follows. thinking is intentionally
// absent: the legacy client still tolerates that token, but the prompt
// forbids it.
func ForcedChoices() ([]string, error) {
	ems, err := Catalog()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ems)+1)
	out = append(out, NeutralID)
	seen := map[string]bool{NeutralID: true}
	for _, em := range ems {
		if em.ID == "" || seen[em.ID] {
			return nil, fmt.Errorf("emotion catalog id %q is empty or duplicated", em.ID)
		}
		seen[em.ID] = true
		out = append(out, em.ID)
	}
	return out, nil
}

// PlayableIDs returns catalog ids sorted by forced-choice order.
func PlayableIDs() ([]string, error) {
	choices, err := ForcedChoices()
	if err != nil {
		return nil, err
	}
	return choices[1:], nil
}

// ManifestPath is the legacy player manifest for this emotion.
func (e Emotion) ManifestPath() string {
	return e.Clip + "/manifest.json"
}

func rank(p Priority) int {
	switch p {
	case PriorityLow:
		return 1
	case PriorityNormal:
		return 2
	case PriorityHigh:
		return 3
	default:
		return 0
	}
}

func validPriority(p Priority) bool {
	return rank(p) > 0
}
