package companion

import (
	"codexpet/reconstruction/emotion"
	"strings"
	"testing"
	"time"
)

func TestEmotionDebugRecordsActualTransitionsWithoutReclassifying(t *testing.T) {
	s := &Server{}
	s.emotionDebug.Model = EmotionModelResult{State: "validated", Analysis: emotion.Analysis{Emotion: "happy", Intensity: .8, Confidence: .9, DurationMS: 8000}}
	for _, clip := range []string{"toLaugh", "video", "video", "fromLaugh", "idle"} {
		s.ReportEmotionPlayback(EmotionPlayback{Clip: clip, Success: true, UpdatedAt: time.Now()})
	}
	d := s.EmotionDebug()
	if len(d.PlaybackTransitions) != 4 || d.Playback.Clip != "idle" || d.Model.Analysis.Emotion != "happy" {
		t.Fatal("model/playback mixed", d)
	}
	d.PlaybackTransitions[0].Clip = "changed"
	if s.EmotionDebug().PlaybackTransitions[0].Clip != "toLaugh" {
		t.Fatal("mutable snapshot")
	}
	for _, label := range []string{"模型判断结果", "动画调度结果", "实际播放结果", "fromLaugh"} {
		if !strings.Contains(s.EmotionDebug().Text(), label) {
			t.Fatal("panel missing section", label)
		}
	}
}
