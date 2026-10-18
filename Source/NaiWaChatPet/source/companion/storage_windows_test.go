//go:build windows

package companion

import (
	"bytes"
	"net/http/httptest"
	"os"
	"testing"
)

func TestEncryptedMemorySurvivesRestartAndCanBeCleared(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	s, e := Start(nil)
	if e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	s.key = "sk-local-test-key"
	s.memory = []Message{{Role: "user", Content: "remember-local-test"}, {Role: "assistant", Content: "completed-local-test"}}
	s.persistMemoryLocked()
	s.mu.Unlock()
	_ = s.Close()
	p, _ := memoryPath()
	b, e := os.ReadFile(p)
	if e != nil || bytes.Contains(b, []byte("sk-local-test-key")) || bytes.Contains(b, []byte("remember-local-test")) {
		t.Fatal("not encrypted or not saved")
	}
	fresh, e := Start(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	if fresh.key != "sk-local-test-key" || len(fresh.history) != 2 || len(fresh.memory) != 2 {
		t.Fatal("restart lost memory")
	}
	if fresh.settings.ScreenEnabled || fresh.settings.Proactive {
		t.Fatal("restart restored grants")
	}
	fresh.clearHistory(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/clear", nil))
	last, e := Start(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer last.Close()
	if last.key != "sk-local-test-key" || len(last.memory) != 0 {
		t.Fatal("clear was not persistent")
	}
}
