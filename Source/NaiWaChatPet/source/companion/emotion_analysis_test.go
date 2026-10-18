package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func classifierEnvelope(w http.ResponseWriter, text string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": text}}}})
}
func TestTwoChannelsAndClassifierFailureNeverBreaksChat(t *testing.T) {
	fixtures := []struct {
		name, text, want string
		delay            bool
	}{
		{"valid", `{"emotion":"shy","intensity":0.65,"confidence":0.9,"duration_ms":7000}`, "shy", false},
		{"angry", `{"emotion":"angry","intensity":0.8,"confidence":0.95,"duration_ms":7000}`, "angry", false},
		{"invalid_json", `broken JSON`, "neutral", false},
		{"illegal", `{"emotion":"laugh","intensity":1,"confidence":1,"duration_ms":7000}`, "neutral", false},
		{"weak", `{"emotion":"happy","intensity":1,"confidence":0.4,"duration_ms":7000}`, "neutral", false},
		{"empty", ``, "neutral", false},
		{"timeout", ``, "neutral", true},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			var classification, chat atomic.Int32
			c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
				var p chatRequest
				_ = json.NewDecoder(r.Body).Decode(&p)
				if p.ResponseFormat["type"] == "json_object" {
					classification.Add(1)
					if p.Stream || p.MaxTokens != 256 || p.Thinking["type"] != "disabled" {
						t.Error("wrong classifier options")
					}
					b, _ := json.Marshal(p.Messages)
					if strings.Contains(string(b), "data:image") {
						t.Error("image leaked")
					}
					if tc.delay {
						<-r.Context().Done()
						return
					}
					classifierEnvelope(w, tc.text)
					return
				}
				if classification.Load() != 1 || !p.Stream {
					t.Error("chat not independent or classifier not first")
				}
				chat.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, normalSSE)
			}, 3)
			defer api.Close()
			s := serverWithMock(t, c)
			on := true
			s.settings.EmotionEnabled = &on
			s.emotionTimeout = 30 * time.Millisecond
			if _, e := s.Submit("你好呀"); e != nil {
				t.Fatal(e)
			}
			v := waitView(t, s, "completed")
			if v.Text != "完整回答" || strings.Contains(v.Text, "confidence") {
				t.Fatal("internal classifier exposed", v)
			}
			if s.EmotionDebug().Model.Effective.Emotion != tc.want {
				t.Fatal(s.EmotionDebug())
			}
			if classification.Load() != 1 || chat.Load() != 1 {
				t.Fatal("unexpected retries", classification.Load(), chat.Load())
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.history) != 2 || s.history[1].Content != "完整回答" || len(s.memory) != 2 {
				t.Fatal("classification entered history")
			}
		})
	}
}

func TestClassifierCancelledCannotPublishLateEmotion(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		classifierEnvelope(w, `{"emotion":"happy","intensity":1,"confidence":1,"duration_ms":8000}`)
	}, 1)
	defer api.Close()
	s := serverWithMock(t, c)
	on := true
	s.settings.EmotionEnabled = &on
	_, e := s.Submit("开心")
	if e != nil {
		t.Fatal(e)
	}
	<-entered
	s.Cancel()
	close(release)
	time.Sleep(30 * time.Millisecond)
	if s.EmotionDebug().Model.Effective.Emotion == "happy" || len(s.history) > 0 {
		t.Fatal("late response influenced state")
	}
}

func TestClassifierContextsPersonaAndConfidenceSeparate(t *testing.T) {
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		var p chatRequest
		_ = json.NewDecoder(r.Body).Decode(&p)
		b, _ := json.Marshal(p.Messages)
		for _, word := range []string{"温柔", "不机械复制用户情绪", "previous_pet_expression", "recent_conversation", "earlier"} {
			if !strings.Contains(string(b), word) {
				t.Error("missing context", word)
			}
		}
		classifierEnvelope(w, `{"emotion":"happy","intensity":0.2,"confidence":0.99,"duration_ms":5000}`)
	}, 1)
	defer api.Close()
	m := ClassifyPetEmotion(context.Background(), c, "sk-test-emotion", "deepseek-flash", "deepseek", "id", "now", []Message{{Role: "user", Content: "earlier"}}, "sad", time.Second)
	if m.Effective.Intensity != .2 || m.Effective.Confidence != .99 {
		t.Fatal(fmt.Sprint(m))
	}
}
