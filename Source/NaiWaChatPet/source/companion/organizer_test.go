package companion

import (
	"codexpet/reconstruction/tidy"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOrganizerConsentIsolationAndProtocol(t *testing.T) {
	calls := 0
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, _ := io.ReadAll(r.Body)
		text := string(b)
		for _, secret := range []string{"private history", "C:\\Users", "fake-key"} {
			if strings.Contains(text, secret) {
				t.Errorf("request leaked %s", secret)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"index":0,"message":{"content":"{\"suggestions\":[{\"id\":\"f1\",\"group\":\"学习资料\",\"category\":\"project\",\"reason\":\"同一主题候选\",\"confidence\":0.7}]}"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	s := &Server{key: "fake-key", client: &Client{HTTP: vendor.Client()}, settings: Settings{ModelChoice: "custom", ModelID: "test", BaseURL: vendor.URL}, history: []Message{{Role: "user", Content: "private history"}}}
	h := []tidy.Hint{{ID: "f1", Name: "课程.pdf", Ext: ".pdf"}}
	if _, e := s.AdviseOrganize(context.Background(), h, false); e == nil || calls != 0 {
		t.Fatal("unconsented call")
	}
	r, e := s.AdviseOrganize(context.Background(), h, true)
	if e != nil || len(r) != 1 || calls != 1 {
		t.Fatalf("%+v %v %d", r, e, calls)
	}
	if len(s.history) != 1 {
		t.Fatal("history changed")
	}
	h[0].Name = "password.txt"
	if _, e = s.AdviseOrganize(context.Background(), h, true); e == nil || calls != 1 {
		t.Fatal("sensitive hint sent")
	}
}
