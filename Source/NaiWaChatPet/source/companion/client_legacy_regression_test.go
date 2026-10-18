package companion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type auditTransport func(*http.Request) (*http.Response, error)

func (f auditTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func useAuditResponse(t *testing.T, body string, inspect func(*http.Request)) {
	t.Helper()
	old := httpClient
	httpClient = &http.Client{Transport: auditTransport(func(r *http.Request) (*http.Response, error) {
		if inspect != nil {
			inspect(r)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	t.Cleanup(func() { httpClient = old })
}
func TestClientRejectsEmptyContent(t *testing.T) {
	useAuditResponse(t, `{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`, nil)
	if _, err := callDeepSeek(context.Background(), "sk-test-not-a-real-key", "deepseek-flash", []Message{{Role: "user", Content: "hello"}}, 1024); err == nil {
		t.Fatal("empty final output was accepted as a successful assistant reply")
	}
}
func TestClientRejectsTruncatedResponse(t *testing.T) {
	useAuditResponse(t, `{"choices":[{"finish_reason":"length","message":{"content":"half a sentence"}}]}`, nil)
	if _, err := callDeepSeek(context.Background(), "sk-test-not-a-real-key", "deepseek-flash", []Message{{Role: "user", Content: "hello"}}, 1024); err == nil {
		t.Fatal("length-truncated response was accepted as complete")
	}
}
func TestClientExplicitDefaultThinkingDisabled(t *testing.T) {
	useAuditResponse(t, `{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`, func(r *http.Request) {
		var p struct {
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if p.Thinking.Type != "disabled" {
			t.Errorf("non-thinking mode not explicit: %q", p.Thinking.Type)
		}
	})
	if _, err := callDeepSeek(context.Background(), "sk-test-not-a-real-key", "deepseek-flash", []Message{{Role: "user", Content: "hello"}}, 1024); err != nil {
		t.Fatal(err)
	}
}
