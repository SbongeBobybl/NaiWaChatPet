package companion

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type demoTransport struct{}

func (demoTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var p chatRequest
	_ = json.NewDecoder(r.Body).Decode(&p)
	text := ""
	if len(p.Messages) > 0 {
		text, _ = p.Messages[len(p.Messages)-1].Content.(string)
	}
	answer := "我在呢，慢慢说，我陪你。"
	if strings.Contains(text, "可爱") {
		answer = "嘿嘿，你这样夸我，我都有点不好意思了。"
	}
	if strings.Contains(text, "笑") {
		answer = "哈哈，这个真的把我逗乐了！"
	}
	if strings.Contains(text, "长回复") {
		answer = "这是离线长回复演示，不会调用模型服务。\n\n" + strings.Repeat("原生气泡会自动换行、限制高度，并允许滚动与展开阅读。长回答会保留，直到你主动关闭。\n", 12)
	}
	reader, writer := io.Pipe()
	if p.ResponseFormat["type"] == "json_object" {
		var input struct {
			Current string `json:"current_message"`
		}
		_ = json.Unmarshal([]byte(text), &input)
		text = input.Current
		b, _ := json.Marshal(demoEmotion(text))
		answer = string(b)
	}
	go func() {
		defer writer.Close()
		if !p.Stream {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": answer}}}})
			_, _ = writer.Write(b)
			return
		}
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"offline hidden reasoning\"}}]}\n\n")
		runes := []rune(answer)
		for i := 0; i < len(runes); i += 5 {
			select {
			case <-r.Context().Done():
				writer.CloseWithError(r.Context().Err())
				return
			case <-time.After(70 * time.Millisecond):
			}
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": string(runes[i:min(i+5, len(runes))])}}}})
			if _, e := writer.Write(append(append([]byte("data: "), b...), []byte("\n\n")...)); e != nil {
				return
			}
			if strings.Contains(text, "模拟中断") && i > 10 {
				return
			}
		}
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}()
	contentType := "text/event-stream"
	if !p.Stream {
		contentType = "application/json"
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: reader, Request: r}, nil
}
func (s *Server) EnableOfflineDemo() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.demo = true
	s.history = nil
	s.memory = nil
	s.storageStatus = "离线演示不保存或使用真实 Key 和聊天记忆。"
	s.key = "offline-demo-key"
	s.client = &Client{HTTP: &http.Client{Transport: demoTransport{}}, Endpoint: deepSeekEndpoint, Attempts: 1, Diagnostic: s.recordDiagnostic}
}
