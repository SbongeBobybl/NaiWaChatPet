package companion

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultPrivacyAndWebOrigin(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var captures atomic.Int32
	s, err := Start(func() ([]byte, error) { captures.Add(1); return []byte("jpeg"), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state := request(t, s, "GET", "/api/status", nil, true)
	if state.StatusCode != 200 {
		t.Fatalf("status=%d", state.StatusCode)
	}
	var st Status
	if err := json.NewDecoder(state.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	state.Body.Close()
	if st.Configured || st.Proactive || st.ScreenEnabled {
		t.Fatalf("privacy defaults wrong: %+v", st)
	}
	// No token: even same-machine endpoint rejects settings.
	req, _ := http.NewRequest("POST", s.addr+"/api/settings", strings.NewReader(`{}`))
	req.Header.Set("Origin", s.addr)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("no-token POST accepted: %d", resp.StatusCode)
	}
	// No explicit consent: no auto screenshots.
	opts := Settings{APIKey: "fixture-not-a-real-api-credential", Persona: DefaultPersona, Length: "short", ScreenEnabled: true, ScreenMinutes: 10}
	r := request(t, s, "POST", "/api/settings", opts, true)
	if r.StatusCode != 400 {
		t.Fatalf("screen without consent accepted: %d", r.StatusCode)
	}
	r.Body.Close()
	if captures.Load() != 0 {
		t.Fatal("unexpected background screenshot")
	}
	// Enabling is consented explicitly, visible and reversible.
	opts.ScreenConsent = true
	r = request(t, s, "POST", "/api/settings", opts, true)
	if r.StatusCode != 200 {
		t.Fatalf("enable failed: %d", r.StatusCode)
	}
	r.Body.Close()
	if !s.ScreenActive() {
		t.Fatal("opted-in screen observation not active")
	}
	r = request(t, s, "POST", "/api/pause", map[string]bool{}, true)
	if r.StatusCode != 200 {
		t.Fatal("pause failed")
	}
	r.Body.Close()
	if s.ScreenActive() {
		t.Fatal("pause failed to stop screen observation")
	}
	if captures.Load() != 0 {
		t.Fatal("pause should not capture")
	}
}
func TestManualScreenRequiresConfirmation(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var captures atomic.Int32
	s, err := Start(func() ([]byte, error) { captures.Add(1); return []byte("jpeg"), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := request(t, s, "POST", "/api/chat", chatBody{Text: "看看我在做什么", WithScreen: true, ConfirmScreen: false}, true)
	if r.StatusCode != 400 {
		t.Fatalf("unconfirmed manual capture accepted: %d", r.StatusCode)
	}
	r.Body.Close()
	if captures.Load() != 0 {
		t.Fatal("captured without permission")
	}
}
func TestLocalInterfaceAndPersona(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Start(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	resp, err := http.Get(s.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), ProductName) {
		t.Fatal("missing UI")
	}
	if !strings.Contains(BuildPrompt("", "short", false), GentlePersona) {
		t.Fatal("default persona omitted")
	}
	if strings.Contains(BuildPrompt("", "short", false), "当前附有") {
		t.Fatal("fabricated screen observation")
	}
	if !strings.Contains(BuildPrompt("", "short", true), "截屏") {
		t.Fatal("vision prompt omitted")
	}
}
func request(t *testing.T, s *Server, method, path string, payload any, token bool) *http.Response {
	t.Helper()
	var data []byte
	if payload != nil {
		data, _ = json.Marshal(payload)
	}
	req, err := http.NewRequest(method, s.addr+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", s.addr)
	req.Header.Set("Content-Type", "application/json")
	if token {
		req.Header.Set("X-Naiwa-Session", s.token)
	}
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
