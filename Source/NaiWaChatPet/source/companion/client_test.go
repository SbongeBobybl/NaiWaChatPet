package companion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const normalReply = `{"choices":[{"index":0,"finish_reason":"stop","message":{"content":"完整回答"}}]}`
const normalSSE = "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hidden\"},\"finish_reason\":null}]}\r\n\r\n" +
	"data: {\"choices\":[]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"完整\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"回答\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
	"data: [DONE]\n\n"

func errorCode(t *testing.T, err error) string {
	t.Helper()
	var e *ReplyError
	if !errors.As(err, &e) {
		t.Fatalf("expected safe ReplyError, got %v", err)
	}
	return e.Code
}
func mockClient(h http.HandlerFunc, attempts int) (*Client, *httptest.Server) {
	s := httptest.NewServer(h)
	return &Client{Endpoint: s.URL, HTTP: s.Client(), Attempts: attempts, Wait: func(context.Context, time.Duration) error { return nil }}, s
}
func invoke(t *testing.T, c *Client, stream bool, delta func(string)) (Result, error) {
	t.Helper()
	return c.Complete(context.Background(), "fixture-not-a-real-api-credential", RequestOptions{RequestID: "test-1", Model: "deepseek-flash", Stream: stream, MaxTokens: 2048}, []Message{{Role: "user", Content: "private user prompt"}}, delta)
}
func TestFragmentedSSEAndJSON100Requests(t *testing.T) {
	var count atomic.Int32
	c, s := mockClient(func(w http.ResponseWriter, r *http.Request) {
		var p chatRequest
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Error("invalid request")
		}
		if p.Thinking["type"] != "disabled" {
			t.Error("default thinking enabled")
		}
		count.Add(1)
		if !p.Stream {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, normalReply)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// Deliberately split every UTF-8 codepoint and JSON token at arbitrary bytes.
		for i := 0; i < len(normalSSE); i += 3 {
			end := min(i+3, len(normalSSE))
			io.WriteString(w, normalSSE[i:end])
			w.(http.Flusher).Flush()
		}
	}, 1)
	defer s.Close()
	for i := 0; i < 100; i++ {
		var out strings.Builder
		r, err := invoke(t, c, i%2 == 0, func(s string) { out.WriteString(s) })
		if err != nil || r.Text != "完整回答" || out.String() != r.Text {
			t.Fatalf("request %d: result=%+v emitted=%q err=%v", i, r, out.String(), err)
		}
		if strings.Contains(out.String(), "hidden") {
			t.Fatal("reasoning leaked")
		}
	}
	if count.Load() != 100 {
		t.Fatal(count.Load())
	}
	t.Log("100 sequential JSON/SSE completions passed with fragmented UTF-8")
}
func TestSSETerminalAndMalformedCases(t *testing.T) {
	cases := []struct{ name, body, code, text string }{
		{"reasoning-only", "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hidden\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "empty", ""},
		{"blank", "data: {\"choices\":[{\"delta\":{\"content\":\"  \"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "empty", ""},
		{"early-close", "data: {\"choices\":[{\"delta\":{\"content\":\"半句\"}}]}\n\n", "interrupted", "半句"},
		{"missing-done", "data: {\"choices\":[{\"delta\":{\"content\":\"半句\"},\"finish_reason\":\"stop\"}]}\n\n", "interrupted", "半句"},
		{"length", "data: {\"choices\":[{\"delta\":{\"content\":\"半句\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n", "truncated", "半句"},
		{"bad-json", "data: {broken}\n\n", "protocol", ""},
		{"done-before-finish", "data: [DONE]\n\n", "interrupted", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var n atomic.Int32
			c, s := mockClient(func(w http.ResponseWriter, r *http.Request) {
				n.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, tc.body)
			}, 3)
			defer s.Close()
			r, err := invoke(t, c, true, nil)
			if errorCode(t, err) != tc.code || strings.TrimSpace(r.Text) != tc.text {
				t.Fatalf("%+v %v", r, err)
			}
			if n.Load() != 1 {
				t.Fatal("replayed a generated/invalid response")
			}
		})
	}
}
func TestSSECommentsMultilineAndFinalEventWithoutBlank(t *testing.T) {
	body := ": heartbeat\n\nevent: message\ndata: {\ndata: \"choices\": [{\"delta\": {\"content\": \"ok\"}, \"finish_reason\": \"stop\"}]}\n\ndata:[DONE]"
	c, s := mockClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, body)
	}, 1)
	defer s.Close()
	r, e := invoke(t, c, true, nil)
	if e != nil || r.Text != "ok" {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestHTTPErrorClassificationAndPreOutputRetry(t *testing.T) {
	for status, code := range map[int]string{400: "invalid", 401: "auth", 402: "balance", 429: "rate_limit", 503: "server"} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c, s := mockClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.WriteString(w, `{"error":{"message":"sk-secret and private prompt"}}`)
			}, 1)
			defer s.Close()
			_, err := invoke(t, c, false, nil)
			if errorCode(t, err) != code || strings.Contains(err.Error(), "secret") {
				t.Fatal(err)
			}
		})
	}
	var count atomic.Int32
	c, s := mockClient(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, normalReply)
	}, 2)
	defer s.Close()
	r, e := invoke(t, c, false, nil)
	if e != nil || r.Attempts != 2 || count.Load() != 2 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestTimeoutCancellationAndSafeDiagnostics(t *testing.T) {
	c, s := mockClient(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(150 * time.Millisecond):
		}
	}, 2)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, e := c.Complete(ctx, "test-credential-key", RequestOptions{Model: "deepseek-flash"}, nil, nil)
	if errorCode(t, e) != "timeout" {
		t.Fatal(e)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	_, e = c.Complete(ctx, "test-credential-key", RequestOptions{Model: "deepseek-flash"}, nil, nil)
	if errorCode(t, e) != "cancelled" {
		t.Fatal(e)
	}
	c2, s2 := mockClient(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, normalReply) }, 1)
	defer s2.Close()
	var log string
	c2.Diagnostic = func(d Diagnostic) { b, _ := json.Marshal(d); log += string(b) }
	_, e = invoke(t, c2, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(log, "private") || strings.Contains(log, "完整") || strings.Contains(log, "sk-") {
		t.Fatal("private data logged", log)
	}
}
