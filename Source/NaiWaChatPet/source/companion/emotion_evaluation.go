package companion

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

type EmotionEvaluationCase struct {
	ID       string    `json:"id"`
	User     string    `json:"user"`
	History  []Message `json:"history,omitempty"`
	Previous string    `json:"previous,omitempty"`
	Expected string    `json:"expected"`
}
type EmotionEvaluationRow struct {
	ID, Expected, Predicted, State, Reason string
	Correct                                bool
	LatencyMS                              int64
}
type EmotionEvaluationReport struct {
	Model                     string
	Reference                 string
	Total, Correct, Fallbacks int
	Accuracy                  float64
	Confusion                 map[string]map[string]int
	Rows                      []EmotionEvaluationRow
}

// Explicit live benchmark only. Uses the existing DPAPI key inside this process;
// neither key nor user conversation memory is exported or used in test examples.
func (s *Server) EvaluateEmotions(ctx context.Context, cases []EmotionEvaluationCase, progress io.Writer) (EmotionEvaluationReport, error) {
	s.mu.Lock()
	key, model := s.key, s.model
	client := *s.client
	profile, _ := ModelFromSettings(s.settings)
	s.mu.Unlock()
	report := EmotionEvaluationReport{Model: model, Reference: "independent frozen assistant-authored reference; human review deferred", Confusion: map[string]map[string]int{}}
	if key == "" {
		return report, ErrNotConfigured
	}
	client.Diagnostic = nil
	thinking := "omit"
	if profile.ThinkingExtension {
		thinking = "deepseek"
	}
	for _, tc := range cases {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		m := ClassifyPetEmotion(ctx, &client, key, model, thinking, "evaluation-"+tc.ID, tc.User, tc.History, tc.Previous, 4*time.Second)
		row := EmotionEvaluationRow{ID: tc.ID, Expected: tc.Expected, Predicted: m.Effective.Emotion, State: m.State, Reason: m.Reason, LatencyMS: m.LatencyMS, Correct: tc.Expected == m.Effective.Emotion}
		report.Rows = append(report.Rows, row)
		report.Total++
		if row.Correct {
			report.Correct++
		}
		if m.State == "fallback" {
			report.Fallbacks++
		}
		if report.Confusion[tc.Expected] == nil {
			report.Confusion[tc.Expected] = map[string]int{}
		}
		report.Confusion[tc.Expected][row.Predicted]++
		if progress != nil {
			_ = json.NewEncoder(progress).Encode(map[string]any{"completed": report.Total, "total": len(cases), "state": row.State, "reason": row.Reason})
		}
	}
	if report.Total > 0 {
		report.Accuracy = float64(report.Correct) / float64(report.Total)
	}
	return report, nil
}
