package companion

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serverWithMock(t *testing.T, c *Client) *Server {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, e := Start(nil)
	if e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	s.persistenceDisabled = true
	s.key = "test-credential-private"
	c.Diagnostic = s.recordDiagnostic
	s.client = c
	// These legacy chat transport tests isolate the chat channel. The dual-channel
	// tests explicitly enable emotion classification against their own fixtures.
	emotionOff := false
	s.settings.EmotionEnabled = &emotionOff
	// Older scheduler fixtures cover the retained minute-based/local path.
	s.settings.ProactiveSeconds = 0
	s.mu.Unlock()
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func waitView(t *testing.T, s *Server, stage string) ReplyView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v := s.View()
		if v.Stage == stage {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("stage never %s: %+v", stage, s.View())
	return ReplyView{}
}
func Test100DialoguesCommitOnlyCompletePairs(t *testing.T) {
	var n atomic.Int32
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		var p chatRequest
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Error("invalid payload")
		}
		if !p.Stream {
			t.Error("provider streaming not default")
		}
		if p.MaxTokens < 768 {
			t.Error("unsafe output budget")
		}
		n.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < len(normalSSE); i += 7 {
			io.WriteString(w, normalSSE[i:min(i+7, len(normalSSE))])
			w.(http.Flusher).Flush()
		}
	}, 1)
	defer api.Close()
	s := serverWithMock(t, c)
	for i := 0; i < 100; i++ {
		resp := request(t, s, "POST", "/api/chat", chatBody{Text: "一轮模拟对话"}, true)
		var out struct {
			Reply string `json:"reply"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode != 200 || out.Reply != "完整回答" {
			t.Fatalf("dialogue %d status %d reply %q", i, resp.StatusCode, out.Reply)
		}
		s.mu.Lock()
		h := append([]Message(nil), s.history...)
		s.mu.Unlock()
		if len(h) != min((i+1)*2, 24) {
			t.Fatalf("duplicate/lost history after %d: %d", i, len(h))
		}
		for j, m := range h {
			if j%2 == 0 && m.Role != "user" || j%2 == 1 && m.Role != "assistant" {
				t.Fatal("broken history pairs")
			}
		}
	}
	if n.Load() != 100 {
		t.Fatal("unexpected model replays", n.Load())
	}
	t.Log("100 dialogue transactions: no blank reply, duplicate history, or parser interruption")
}
func TestPartialOutputNotCommittedAndExplicitRetry(t *testing.T) {
	var n atomic.Int32
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if n.Add(1) == 1 {
			io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"半句\"}}]}\n\n")
			return
		}
		io.WriteString(w, normalSSE)
	}, 3)
	defer api.Close()
	s := serverWithMock(t, c)
	if _, e := s.Submit("需要重试"); e != nil {
		t.Fatal(e)
	}
	v := waitView(t, s, "error")
	if v.Text != "半句" || v.Error == "" || v.Code != "interrupted" {
		t.Fatalf("%+v", v)
	}
	s.mu.Lock()
	h := len(s.history)
	s.mu.Unlock()
	if h != 0 || n.Load() != 1 {
		t.Fatal("partial committed or replayed")
	}
	deadline := time.Now().Add(time.Second)
	for {
		_, e := s.Retry()
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(e)
		}
		time.Sleep(time.Millisecond)
	}
	waitView(t, s, "completed")
	s.mu.Lock()
	h = len(s.history)
	s.mu.Unlock()
	if h != 2 || n.Load() != 2 {
		t.Fatal("retry duplicated history")
	}
}
func TestCancelClearAndRapidSubmitIsolation(t *testing.T) {
	started := make(chan struct{}, 4)
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-time.After(150 * time.Millisecond):
		}
		io.WriteString(w, normalReply)
	}, 1)
	defer api.Close()
	s := serverWithMock(t, c)
	_, e := s.Submit("不要留下失败历史")
	if e != nil {
		t.Fatal(e)
	}
	<-started
	if _, e = s.Submit("第二条"); e != ErrBusy {
		t.Fatal("rapid request not blocked", e)
	}
	if !s.Cancel() {
		t.Fatal("cancel ignored")
	}
	if s.View().Stage != "cancelled" {
		t.Fatal(s.View())
	}
	time.Sleep(30 * time.Millisecond)
	s.mu.Lock()
	n := len(s.history)
	s.mu.Unlock()
	if n != 0 {
		t.Fatal("cancelled history committed")
	}
	_, e = s.Submit("清空前的消息")
	if e != nil {
		t.Fatal(e)
	}
	<-started
	resp := request(t, s, "POST", "/api/clear", map[string]bool{}, true)
	resp.Body.Close()
	time.Sleep(170 * time.Millisecond)
	s.mu.Lock()
	n = len(s.history)
	s.mu.Unlock()
	if n != 0 || s.View().RequestID != "" {
		t.Fatal("stale reply survived clear")
	}
}
func TestExactHostAndCrossOriginGuard(t *testing.T) {
	s, e := Start(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, tc := range []struct{ host, origin string }{{"127.0.0.1:9999.evil.test", s.addr}, {s.listener.Addr().String(), "https://evil.example"}} {
		r, _ := http.NewRequest("GET", s.addr+"/api/status", nil)
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Naiwa-Session", s.token)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("forged origin/host accepted")
		}
	}
	resp := request(t, s, "GET", "/api/status", nil, true)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(b), s.token) {
		t.Fatal("session token in diagnostics")
	}
}
