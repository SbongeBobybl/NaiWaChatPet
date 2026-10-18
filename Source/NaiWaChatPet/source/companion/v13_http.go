package companion

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) chatWithSharing(w http.ResponseWriter, r *http.Request, c chatBody) {
	id, e := s.SubmitWithSharing(c.Text, c.OmitScreen)
	if e != nil {
		jsonErr(w, e.Error(), 400)
		return
	}
	stream := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var flush http.Flusher
	if stream {
		var ok bool
		flush, ok = w.(http.Flusher)
		if !ok {
			s.Cancel()
			jsonErr(w, "本地响应不支持流式输出", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
	}
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	var updated time.Time
	for {
		v := s.View()
		if v.RequestID != id {
			v = ReplyView{RequestID: id, Stage: "cancelled", Error: "本次请求已取消", Code: "cancelled", UpdatedAt: time.Now()}
		}
		if stream && !v.UpdatedAt.Equal(updated) {
			b, _ := json.Marshal(v)
			_, _ = w.Write(append(append([]byte("data: "), b...), []byte("\n\n")...))
			flush.Flush()
			updated = v.UpdatedAt
		}
		terminal := v.Stage == "completed" || v.Stage == "error" || v.Stage == "cancelled"
		if terminal {
			if !stream {
				if v.Stage == "completed" {
					jsonOut(w, map[string]any{"reply": v.Text, "requestId": id, "screenshotSent": !c.OmitScreen})
				} else {
					jsonErr(w, v.Error, 502)
				}
			}
			return
		}
		select {
		case <-r.Context().Done():
			s.mu.Lock()
			same := s.activeID == id
			s.mu.Unlock()
			if same {
				s.Cancel()
			}
			return
		case <-s.done:
			return
		case <-ticker.C:
		}
	}
}
