package companion

import (
	"strings"
	"testing"
)

func TestAngryAllowedByPromptAndManualAction(t *testing.T) {
	if !strings.Contains(emotionRules, "生气(angry)") || !strings.Contains(emotionRules, "sad、angry、") || strings.Contains(emotionRules, "不要生气") {
		t.Fatal("angry is excluded or contradicted in model instructions")
	}
	s := serverWithMock(t, &Client{})
	if e := s.RequestDiagnosticEmotion("angry"); e != nil {
		t.Fatal(e)
	}
	label, sequence := s.LocalAction()
	if label != "angry" || sequence == 0 {
		t.Fatal("manual angry never reached action queue")
	}
	if a := demoEmotion("奶蛙，生气一下"); a.Emotion != "angry" {
		t.Fatal("offline demonstration still excludes angry")
	}
}
