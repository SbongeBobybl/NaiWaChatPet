package aiorganization

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
)

type fakeHandler struct {
	calls   atomic.Int32
	block   bool
	entered chan struct{}
}

func (f *fakeHandler) Configured() bool { return true }
func (f *fakeHandler) Classify(ctx context.Context, r Request) (Result, error) {
	f.calls.Add(1)
	if f.block {
		close(f.entered)
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	return Result{Items: []Suggestion{{r.Items[0].ItemID, r.Categories[0].ID, 0.8, "local fake"}}}, nil
}
func (f *fakeHandler) ParseIntent(ctx context.Context, r Request) (Result, error) {
	return f.Classify(ctx, r)
}
func fixture(t *testing.T, f Handler) (*Server, Descriptor) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "agent")
	s, e := Start(dir, f)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	b, e := os.ReadFile(filepath.Join(dir, "agent.json"))
	if e != nil {
		t.Fatal(e)
	}
	var d Descriptor
	if json.Unmarshal(b, &d) != nil {
		t.Fatal("descriptor")
	}
	return s, d
}
func exchange(d Descriptor, r Request) (Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, e := winio.DialPipeContext(ctx, `\\.\pipe\`+d.PipeName)
	if e != nil {
		return Response{}, e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	r.SchemaVersion = Version
	r.Session = d.Session
	r.Token = d.Token
	if e = WriteFrame(c, r); e != nil {
		return Response{}, e
	}
	var result Response
	e = ReadFrame(c, &result)
	return result, e
}
func candidate() Request {
	return Request{RequestID: "request-1", Operation: "classify", Items: []Item{{ItemID: "file-1", DisplayName: "研究笔记.txt", Extension: ".txt", Kind: "file"}}, Categories: []Category{{ID: "study", DisplayName: "学习"}}}
}
func TestDeskBoxPipeConsentAndAuthentication(t *testing.T) {
	f := &fakeHandler{}
	_, d := fixture(t, f)
	r := candidate()
	res, e := exchange(d, r)
	if e != nil || res.Status != "consent_required" || f.calls.Load() != 0 {
		t.Fatalf("consent gate: %v %+v", e, res)
	}
	r.Consent = true
	res, e = exchange(d, r)
	if e != nil || res.Status != "ok" || res.Result.Items[0].CategoryID != "study" {
		t.Fatalf("round trip: %v %+v", e, res)
	}
	d.Token = "wrong"
	if _, e = exchange(d, Request{RequestID: "health-1", Operation: "health"}); e == nil {
		t.Fatal("unauthenticated request accepted")
	}
}
func TestDeskBoxPipeCancel(t *testing.T) {
	f := &fakeHandler{block: true, entered: make(chan struct{})}
	_, d := fixture(t, f)
	r := candidate()
	r.Consent = true
	done := make(chan Response, 1)
	go func() { res, _ := exchange(d, r); done <- res }()
	select {
	case <-f.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	if res, e := exchange(d, Request{RequestID: "cancel-1", Operation: "cancel", CancelRequestID: r.RequestID}); e != nil || res.Status != "ok" {
		t.Fatal("cancel failed", e)
	}
	select {
	case res := <-done:
		if res.Status != "cancelled" {
			t.Fatal(res.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel not propagated")
	}
}
