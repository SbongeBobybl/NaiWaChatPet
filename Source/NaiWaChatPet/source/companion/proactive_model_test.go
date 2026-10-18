package companion

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProactiveUsesSeparateModelRequestAndRetainsContext(t *testing.T) {
	var calls atomic.Int32
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var p chatRequest
		json.NewDecoder(r.Body).Decode(&p)
		b, _ := json.Marshal(p.Messages)
		if !p.Stream || !strings.Contains(string(b), "previous gentle company") || !strings.Contains(string(b), "recent conversation") {
			t.Error("proactive request missing streaming/context")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		jsonText, _ := json.Marshal("我刚想到一个小趣事。等你忙完，我再慢慢讲给你听。")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":" + string(jsonText) + "}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}, 1)
	defer api.Close()
	s := serverWithMock(t, c)
	s.settings.ProactiveSeconds = 30
	s.proactiveReplies = []string{"previous gentle company"}
	s.history = []Message{{Role: "user", Content: "recent conversation"}, {Role: "assistant", Content: "earlier reply"}}
	s.startProactive(true, time.Now())
	v := waitView(t, s, "completed")
	if calls.Load() != 1 || !strings.Contains(v.Text, "趣事") || strings.Contains(v.Text, "emotion") {
		t.Fatalf("bad proactive reply %+v calls=%d", v, calls.Load())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) != 2 || len(s.proactiveReplies) != 2 || len(s.memory) != 0 {
		t.Fatal("proactive replies polluted normal pair history or lost memory")
	}
}

func TestFactoryResetClearsAllCurrentVersionMemoryAndPreferences(t *testing.T) {
	s := serverWithMock(t, &Client{})
	s.settings.Proactive = true
	s.settings.ProactiveSeconds = 90
	s.settings.Persona = "private persona"
	s.history = []Message{{Role: "user", Content: "private history"}}
	s.memory = append([]Message(nil), s.history...)
	s.proactiveReplies = []string{"private proactive memory"}
	if e := s.RestoreFactory(); e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != "" || len(s.history)+len(s.memory)+len(s.proactiveReplies) != 0 || s.settings.Persona != DefaultPersona || s.settings.Proactive || s.settings.ProactiveSeconds != 30 {
		t.Fatal("factory data retained")
	}
}
