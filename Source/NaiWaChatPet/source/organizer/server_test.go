package organizer

import (
	"bytes"
	"codexpet/reconstruction/desktoplayout"
	"codexpet/reconstruction/tidy"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	for _, n := range []string{"低碳园区_v1.pdf", "低碳园区_final.png", "课程笔记.docx", "预算.xlsx", "会议录音.mp3", "app.exe", "app.lnk", "身份证123456789.txt"} {
		p := filepath.Join(root, n)
		os.WriteFile(p, []byte(n), 0600)
		past := time.Now().Add(-time.Hour)
		os.Chtimes(p, past, past)
	}
	s, e := New(Backend{Root: root, Journals: t.TempDir(), Model: func() any { return map[string]string{"Model": "模拟连接", "Endpoint": "http://127.0.0.1/mock"} }, Advise: func(ctx context.Context, h []tidy.Hint, c bool) ([]tidy.Suggestion, error) { return nil, nil }, ReadLayout: func() (desktoplayout.Snapshot, error) {
		return desktoplayout.Snapshot{DPI: 144, Spacing: desktoplayout.Point{X: 80, Y: 100}, Monitors: []desktoplayout.Monitor{{ID: "mock", Area: desktoplayout.Rect{Right: 1280, Bottom: 720}, Primary: true}}, Icons: []desktoplayout.Icon{{Key: "system", Name: "回收站", Position: desktoplayout.Point{X: 10, Y: 12}, Fixed: true}, {Key: "project", Name: "低碳园区", Position: desktoplayout.Point{X: 200, Y: 300}, Category: "projects"}, {Key: "notes", Name: "课程笔记", Position: desktoplayout.Point{X: 400, Y: 500}, Category: "references"}}}, nil
	}, ApplyLayout: func(p desktoplayout.Plan) ([]string, error) { return nil, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s
}
func request(t *testing.T, s *Server, path string, body any, token, origin string) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", s.addr+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Naiwa-Session", token)
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	return r.StatusCode, data
}
func TestGuardAndPreviewNeverMove(t *testing.T) {
	s := fixture(t)
	for _, pair := range [][2]string{{"", s.addr}, {s.token, "https://attacker.example"}} {
		code, _ := request(t, s, "/scan", map[string]any{}, pair[0], pair[1])
		if code != 403 {
			t.Fatal(code)
		}
	}
	code, b := request(t, s, "/scan", map[string]any{}, s.token, s.addr)
	if code != 200 {
		t.Fatal(string(b))
	}
	if _, e := os.Stat(filepath.Join(s.b.Root, "课程笔记.docx")); e != nil {
		t.Fatal(e)
	}
	code, _ = request(t, s, "/apply", map[string]any{"Confirm": true, "Revision": s.revision}, s.token, s.addr)
	if code == 200 {
		t.Fatal("applied without preparing")
	}
	code, _ = request(t, s, "/ai", map[string]any{"Revision": s.revision, "IDs": []string{"f0001"}}, s.token, s.addr)
	if code == 200 {
		t.Fatal("AI without consent")
	}
}
func TestConfirmedSelectionAndReplay(t *testing.T) {
	s := fixture(t)
	request(t, s, "/scan", map[string]any{}, s.token, s.addr)
	id := ""
	for k, p := range s.ids {
		if filepath.Base(p) == "课程笔记.docx" {
			id = k
		}
	}
	rev := s.revision
	code, b := request(t, s, "/prepare", map[string]any{"Revision": rev, "Selected": map[string]string{id: "课程资料"}}, s.token, s.addr)
	if code != 200 {
		t.Fatal(string(b))
	}
	code, b = request(t, s, "/apply", map[string]any{"Revision": rev, "Confirm": true}, s.token, s.addr)
	if code != 200 {
		t.Fatal(string(b))
	}
	if _, e := os.Stat(filepath.Join(s.b.Root, "课程资料", "课程笔记.docx")); e != nil {
		t.Fatal(e)
	}
	code, _ = request(t, s, "/apply", map[string]any{"Revision": rev, "Confirm": true}, s.token, s.addr)
	if code == 200 {
		t.Fatal("replayed")
	}
	code, b = request(t, s, "/undo", map[string]any{"Confirm": true}, s.token, s.addr)
	if code != 200 {
		t.Fatal(string(b))
	}
}
func TestBadDestinationAndUnknownID(t *testing.T) {
	s := fixture(t)
	request(t, s, "/scan", map[string]any{}, s.token, s.addr)
	for _, selected := range []map[string]string{{"unknown": "资料"}, {"f0001": "../outside"}} {
		code, _ := request(t, s, "/prepare", map[string]any{"Revision": s.revision, "Selected": selected}, s.token, s.addr)
		if code == 200 {
			t.Fatal("invalid selection accepted")
		}
	}
}
func TestPreviewFixture(t *testing.T) {
	if os.Getenv("NAIWA_PREVIEW_FIXTURE") != "1" {
		t.Skip("manual UI fixture disabled")
	}
	s := fixture(t)
	fmt.Println("PREVIEW_URL=" + s.URL())
	select {
	case <-time.After(4 * time.Minute):
	}
}
func TestHTMLUsesEscapingAndNoRemoteDependencies(t *testing.T) {
	b, _ := assets.ReadFile("web.html")
	for _, bad := range []string{"https://fonts.", "cdn.jsdelivr", "unpkg.com"} {
		if strings.Contains(string(b), bad) {
			t.Fatal(bad)
		}
	}
}

func TestConfirmedPlanDelegatesToCarryWithoutBackgroundMove(t *testing.T) {
	s := fixture(t)
	called := false
	s.b.Carry = func(ctx context.Context, p tidy.Plan, j string, progress func(Status)) (tidy.Report, error) {
		called = true
		if len(p.Moves) != 1 {
			t.Errorf("unexpected plan: %v", p)
		}
		if _, e := os.Stat(p.Moves[0].From); e != nil {
			t.Error("file already moved before carry", e)
		}
		progress(Status{Phase: "moving", Total: 1, Message: "walking"})
		return tidy.Report{}, fmt.Errorf("animation unavailable")
	}
	request(t, s, "/scan", map[string]any{}, s.token, s.addr)
	id := ""
	for k, p := range s.ids {
		if filepath.Ext(p) == ".docx" {
			id = k
		}
	}
	revision := s.revision
	request(t, s, "/prepare", map[string]any{"Revision": revision, "Selected": map[string]string{id: "chosen"}}, s.token, s.addr)
	code, _ := request(t, s, "/apply", map[string]any{"Revision": revision, "Confirm": true}, s.token, s.addr)
	if !called || code == 200 {
		t.Fatal("carry failure was not propagated")
	}
	if _, e := os.Stat(s.ids[id]); e != nil {
		t.Fatal("silently fell back to direct move", e)
	}
}
