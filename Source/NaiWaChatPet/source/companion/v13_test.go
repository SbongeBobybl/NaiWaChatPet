package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMessageSharingSmartEachOffAndOmit(t *testing.T) {
	for _, tc := range []struct {
		mode, text string
		omit, want bool
	}{{"each", "你好", false, true}, {"smart", "看看我的屏幕", false, true}, {"smart", "你好", false, false}, {"each", "看看这里", true, false}, {"off", "看看我的屏幕", false, false}} {
		t.Run(tc.mode+tc.text, func(t *testing.T) {
			var pics, captures atomic.Int32
			c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
				var req chatRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				b, _ := json.Marshal(req.Messages)
				if strings.Contains(string(b), "data:image/png;base64,") {
					pics.Add(1)
				}
				w.Write([]byte(normalReply))
			}, 1)
			defer api.Close()
			s := serverWithMock(t, c)
			s.ConfigureCapture(func(context.Context, string) ([]byte, error) { captures.Add(1); return tinyPNG(), nil }, nil)
			s.mu.Lock()
			s.settings.ObservationMode = tc.mode
			s.settings.ScreenEnabled = tc.mode != "off"
			s.screenExpires = time.Now().Add(time.Hour)
			s.mu.Unlock()
			if _, e := s.SubmitWithSharing(tc.text, tc.omit); e != nil {
				t.Fatal(e)
			}
			waitView(t, s, "completed")
			want := int32(0)
			if tc.want {
				want = 1
			}
			if pics.Load() != want || captures.Load() != want {
				t.Fatalf("pictures=%d captures=%d want=%d", pics.Load(), captures.Load(), want)
			}
		})
	}
}
func TestExcludedAppNeverCapturedAndRevokedCaptureNeverUploads(t *testing.T) {
	var captures, calls atomic.Int32
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write([]byte(normalReply)) }, 1)
	defer api.Close()
	s := serverWithMock(t, c)
	s.ConfigureCapture(func(context.Context, string) ([]byte, error) { captures.Add(1); return tinyPNG(), nil }, nil)
	s.ConfigureScreenContext(func(scope, excluded string) (ScreenContext, error) { return ScreenContext{Excluded: true}, nil })
	s.mu.Lock()
	s.settings.ObservationMode = "each"
	s.settings.ScreenEnabled = true
	s.screenExpires = time.Now().Add(time.Hour)
	s.mu.Unlock()
	_, e := s.Submit("test")
	if e != nil {
		t.Fatal(e)
	}
	waitView(t, s, "error")
	if captures.Load() != 0 || calls.Load() != 0 {
		t.Fatal("excluded app captured or uploaded")
	}
	started := make(chan struct{})
	s.ConfigureScreenContext(nil)
	s.ConfigureCapture(func(ctx context.Context, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}, nil)
	_, _ = s.Submit("test")
	<-started
	s.Pause()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("revoked capture uploaded")
	}
}
func TestProactiveTestPublishesVisibleEventAndPaidDefaultOff(t *testing.T) {
	s := serverWithMock(t, &Client{})
	s.TestCompanionship()
	if v := s.View(); v.Stage != "local" || v.Text == "" {
		t.Fatal("test did not publish desktop event")
	}
	action, seq := s.LocalAction()
	if action == "" || seq == 0 {
		t.Fatal("missing emotion event")
	}
	if s.settings.AllowPaidProactive {
		t.Fatal("paid permission default on")
	}
}
