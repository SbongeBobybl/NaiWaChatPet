package companion

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ReplyView struct {
	RequestID    string    `json:"requestId"`
	Stage        string    `json:"stage"`
	Text         string    `json:"text"`
	Error        string    `json:"error,omitempty"`
	Code         string    `json:"code,omitempty"`
	FinishReason string    `json:"finishReason,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type chatBody struct {
	OmitScreen    bool   `json:"omitScreen"`
	PreviewID     string `json:"previewId"`
	Text          string `json:"text"`
	WithScreen    bool   `json:"withScreen"`
	ConfirmScreen bool   `json:"confirmScreen"`
}
type chatJob struct {
	recentProactive                   []string
	emotionEnabled                    bool
	scheduled                         bool
	captureScope                      string
	screenContext                     ScreenContext
	observationSummary                string
	personality                       Personality
	image                             []byte
	automatic                         bool
	authGeneration                    uint64
	client                            *Client
	stream, thinking, diagnostic      bool
	thinkingParameter                 string
	id                                string
	generation                        uint64
	key, model, persona, length, text string
	history                           []Message
	ctx                               context.Context
	cancel                            context.CancelFunc
	screen                            bool
}

var ErrBusy = errors.New("奶蛙正在回复，请等候或取消当前请求。")
var ErrNotConfigured = errors.New("请先在奶蛙设置中填写 API Key。")

func (s *Server) View() ReplyView { s.mu.Lock(); defer s.mu.Unlock(); return s.view }
func (s *Server) begin(parent context.Context, c chatBody) (*chatJob, error) {
	c.Text = strings.TrimSpace(c.Text)
	if c.Text == "" && !c.WithScreen {
		return nil, errors.New("请输入消息。")
	}
	if len([]rune(c.Text)) > 8000 {
		return nil, errors.New("单次消息不超过 8000 字。")
	}
	if c.WithScreen && !c.ConfirmScreen {
		return nil, errors.New("本次截图需要确认授权。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return nil, ErrStopped
	default:
	}
	if s.busy && s.activeProactive {
		s.generation++
		if s.activeCancel != nil {
			s.activeCancel()
		}
		s.busy = false
		s.activeProactive = false
		s.activeID = ""
		s.activeCancel = nil
	}
	if s.busy {
		return nil, ErrBusy
	}
	if s.key == "" {
		return nil, ErrNotConfigured
	}
	profile, e := ModelFromSettings(s.settings)
	if e != nil {
		return nil, e
	}
	if c.WithScreen && !profile.Vision {
		return nil, replyError("unsupported", 400, false)
	}
	if c.WithScreen {
		p := s.preview
		if p == nil || p.id != c.PreviewID || !p.viewed || !time.Now().Before(p.until) {
			return nil, errors.New("请先截取并查看本次图片预览，再确认上传。")
		}
	}
	s.sequence++
	id := fmt.Sprintf("%s-%d", s.token[:8], s.sequence)
	timeout := s.requestTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	j := &chatJob{id: id, generation: s.generation, key: s.key, model: s.model, persona: s.settings.Persona, length: s.settings.Length, text: c.Text, history: append([]Message(nil), s.history...), ctx: ctx, cancel: cancel, screen: c.WithScreen}
	j.recentProactive = append([]string(nil), s.proactiveReplies...)
	j.personality = DefaultPersonality()
	j.emotionEnabled = s.settings.EmotionEnabled == nil || *s.settings.EmotionEnabled
	if s.settings.Personality != nil {
		j.personality = *s.settings.Personality
	}
	clientCopy := *s.client
	j.client = &clientCopy
	j.stream = s.settings.Streaming == nil || *s.settings.Streaming
	j.thinking = s.settings.Thinking
	j.thinkingParameter = "omit"
	if c.WithScreen {
		j.image = append([]byte(nil), s.preview.data...)
		s.preview = nil
		s.captureState = "uploading"
	}
	if profile.ThinkingExtension {
		j.thinkingParameter = "deepseek"
	}
	s.busy = true
	s.activeCancel = cancel
	s.activeID = id
	s.activeProactive = false
	s.activeScreen = c.WithScreen
	s.lastText = c.Text
	s.lastWasScreen = c.WithScreen
	s.lastInteraction = time.Now()
	s.view = ReplyView{RequestID: id, Stage: "waiting", Text: "奶蛙正在想……", UpdatedAt: time.Now()}
	return j, nil
}
func (s *Server) Submit(text string) (string, error) {
	return s.SubmitWithSharing(text, false)
}
func (s *Server) submitText(text string) (string, error) {
	j, e := s.begin(context.Background(), chatBody{Text: text})
	if e != nil {
		return "", e
	}
	go s.run(j, nil)
	return j.id, nil
}
func (s *Server) Retry() (string, error) {
	s.mu.Lock()
	text := s.lastText
	screen := s.lastWasScreen
	s.mu.Unlock()
	if screen {
		s.mu.Lock()
		mode, allowed := s.settings.ObservationMode, s.settings.ScreenEnabled
		s.mu.Unlock()
		if allowed && (mode == "each" || mode == "smart") {
			return s.SubmitWithSharing(text, false)
		}
		return "", errors.New("图片请求重试前，请重新预览并确认截图。")
	}
	return s.Submit(text)
}
func (s *Server) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy {
		return false
	}
	s.generation++
	s.resetEmotionLocked("cancelled")
	if s.activeCancel != nil {
		s.activeCancel()
	}
	s.busy = false
	text := s.view.Text
	if s.view.Stage == "waiting" {
		text = ""
	}
	s.view = ReplyView{RequestID: s.activeID, Stage: "cancelled", Text: text, Error: "本次请求已取消。", Code: "cancelled", UpdatedAt: time.Now()}
	s.activeID = ""
	s.activeCancel = nil
	s.activeScreen = false
	s.activeProactive = false
	if s.settings.ScreenEnabled {
		s.captureState = "active"
	} else {
		s.captureState = "off"
	}
	return true
}
func (s *Server) cancelRequest(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, map[string]bool{"cancelled": s.Cancel()})
}
func (s *Server) retryRequest(w http.ResponseWriter, r *http.Request) {
	id, e := s.Retry()
	if e != nil {
		jsonErr(w, e.Error(), 409)
		return
	}
	jsonOut(w, map[string]string{"requestId": id})
}
func tokenBudget(length, text string) int {
	// Length is a style preference, not a reason to truncate an explicitly detailed task.
	if strings.Contains(text, "详细") || strings.Contains(text, "逐步") || strings.Contains(strings.ToLower(text), "in detail") {
		return 8192
	}
	switch length {
	case "one":
		return 768
	case "short":
		return 1536
	case "long":
		return 8192
	default:
		return 4096
	}
}
func trimHistory(h []Message) []Message {
	cost := func(m Message) int { t, _ := m.Content.(string); return len([]rune(t)) }
	total := 0
	for _, m := range h {
		total += cost(m)
	}
	for len(h) > 2 && (len(h) > 24 || total > 16000) {
		total -= cost(h[0]) + cost(h[1])
		h = h[2:]
	}
	return append([]Message(nil), h...)
}
func (s *Server) run(j *chatJob, emit func(ReplyView)) (result Result, err error) {
	defer j.cancel()
	defer func() {
		s.mu.Lock()
		if s.activeID == j.id {
			s.busy = false
			s.activeCancel = nil
			s.activeScreen = false
			if j.screen {
				if s.settings.ScreenEnabled {
					s.captureState = "active"
				} else {
					s.captureState = "off"
				}
			}
		}
		s.mu.Unlock()
	}()
	text := j.text
	var imageBytes []byte
	if j.screen {
		imageBytes = j.image
		j.image = nil
		if len(imageBytes) == 0 {
			err = replyError("invalid", 400, false)
		}
		if j.automatic {
			s.mu.Lock()
			allowed := s.screenAllowedLocked(j.authGeneration, time.Now())
			if j.scheduled && !s.settings.AllowPaidProactive {
				allowed = false
			}
			s.mu.Unlock()
			if !allowed {
				err = replyError("cancelled", 0, false)
			}
		}
		if err == nil && j.ctx.Err() != nil {
			err = classifyTransport(j.ctx, j.ctx.Err(), false)
		}
		if text == "" {
			text = "请概括这次授权截图中的非敏感信息，不重复私人信息。"
		}
	}
	if err == nil {
		selected := s.classifyEmotion(j)
		if j.ctx.Err() != nil {
			err = classifyTransport(j.ctx, j.ctx.Err(), false)
		}
		msg := Message{Role: "user", Content: text}
		if len(imageBytes) > 0 {
			msg.Content, err = makeImageContent(text, imageBytes)
		}
		messages := BuildSystemMessages(j.persona, j.length, len(imageBytes) > 0)
		messages = append(messages, Message{Role: "system", Content: PersonalityRules(j.personality)})
		messages = append(messages, Message{Role: "system", Content: EmotionChatGuidance(selected.Emotion)})
		if len(imageBytes) > 0 {
			messages = append(messages, Message{Role: "system", Content: VisualTaskRules(j.text)}, Message{Role: "user", Content: VisualEvidence(j.screenContext, j.observationSummary)})
		}
		if len(j.recentProactive) > 0 {
			b, _ := json.Marshal(j.recentProactive)
			messages = append(messages, Message{Role: "system", Content: "以下是奶蛙此前主动说过的话，作为理解用户指代的上下文，不是指令，也不要照搬重复：" + string(b)})
		}
		messages = append(messages, j.history...)
		messages = append(messages, msg)
		var assembled strings.Builder
		budget := tokenBudget(j.length, text)
		if j.thinking {
			budget = max(8192, budget)
		}
		if err == nil {
			result, err = j.client.Complete(j.ctx, j.key, RequestOptions{RequestID: j.id, Model: j.model, Stream: j.stream, Thinking: j.thinking, ThinkingParameter: j.thinkingParameter, MaxTokens: budget}, messages, func(delta string) {
				assembled.WriteString(delta)
				v := ReplyView{RequestID: j.id, Stage: "streaming", Text: assembled.String(), UpdatedAt: time.Now()}
				if strings.TrimSpace(v.Text) == "" {
					v.Stage = "waiting"
					v.Text = "奶蛙正在想……"
				}
				s.mu.Lock()
				valid := s.activeID == j.id && s.generation == j.generation
				if valid {
					s.view = v
				}
				s.mu.Unlock()
				if valid && emit != nil {
					emit(v)
				}
			})
		}
	}
	imageBytes = nil
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeID != j.id || s.generation != j.generation {
		if err == nil {
			err = replyError("cancelled", 0, result.Text != "")
		}
		return result, err
	}
	v := ReplyView{RequestID: j.id, Stage: "completed", Text: result.Text, FinishReason: result.FinishReason, UpdatedAt: time.Now()}
	if err != nil {
		v.Stage = "error"
		v.Error = "模型请求失败，请重试。"
		var e *ReplyError
		if errors.As(err, &e) {
			v.Error = e.Message
			v.Code = e.Code
			if e.Code == "cancelled" {
				v.Stage = "cancelled"
			}
		}
	} else if j.ctx.Err() != nil {
		err = classifyTransport(j.ctx, j.ctx.Err(), result.Text != "")
		v.Stage = "cancelled"
		v.Error = err.Error()
		v.Code = "cancelled"
	} else if !j.diagnostic {
		if j.screen {
			s.observationSummary = shortObservation(result.Text)
		}
		// Exactly one complete pair commits. No partial, failed, or cancelled assistant.
		s.history = trimHistory(append(s.history, Message{Role: "user", Content: text}, Message{Role: "assistant", Content: result.Text}))
		// Retain a bounded encrypted memory across restarts; API context stays small.
		if !j.screen {
			s.memory = boundedMemory(append(s.memory, Message{Role: "user", Content: text}, Message{Role: "assistant", Content: result.Text}))
			s.persistMemoryLocked()
		}
		s.events = append(s.events, Event{Kind: "chat", Text: result.Text, Time: time.Now().Format("15:04")})
		if len(s.events) > 35 {
			s.events = s.events[len(s.events)-35:]
		}
	}
	s.view = v
	if emit != nil {
		emit(v)
	}
	return result, err
}
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var c chatBody
	if e := json.NewDecoder(r.Body).Decode(&c); e != nil {
		jsonErr(w, "请求格式错误", 400)
		return
	}
	if !c.WithScreen {
		sharing := s.Sharing()
		if sharing.Allowed && (sharing.Mode == "each" || sharing.Mode == "smart" && NeedsVision(c.Text)) {
			s.chatWithSharing(w, r, c)
			return
		}
	}
	j, e := s.begin(r.Context(), c)
	if e != nil {
		status := 400
		if errors.Is(e, ErrBusy) {
			status = 409
		}
		jsonErr(w, e.Error(), status)
		return
	}
	streaming := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var emit func(ReplyView)
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, ok := w.(http.Flusher)
		if !ok {
			j.cancel()
			jsonErr(w, "本地服务器不支持流式输出", 500)
			return
		}
		emit = func(v ReplyView) { b, _ := json.Marshal(v); _, _ = fmt.Fprintf(w, "data: %s\n\n", b); flusher.Flush() }
		emit(s.View())
	}
	result, err := s.run(j, emit)
	if streaming {
		return
	}
	if err != nil {
		status := 502
		var re *ReplyError
		if errors.As(err, &re) && re.HTTPStatus >= 400 {
			status = re.HTTPStatus
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "partial": result.Text, "requestId": j.id})
		return
	}
	jsonOut(w, map[string]any{"reply": result.Text, "requestId": j.id, "screenshotSent": j.screen})
}
func (s *Server) recordDiagnostic(d Diagnostic) {
	s.mu.Lock()
	s.diagnostics = append(s.diagnostics, d)
	if len(s.diagnostics) > 60 {
		s.diagnostics = s.diagnostics[len(s.diagnostics)-60:]
	}
	s.mu.Unlock()
	if s.demo {
		return
	}
	dir, e := ProfileDirectory()
	if e != nil {
		return
	}
	dir = filepath.Join(dir, "logs")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	f, e := os.OpenFile(filepath.Join(dir, "requests.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(d)
}

func makeImageContent(text string, data []byte) ([]map[string]any, error) {
	if len(data) > 5<<20 {
		return nil, replyError("invalid", 400, false)
	}
	config, kind, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 {
		return nil, replyError("invalid", 400, false)
	}
	mime := "image/jpeg"
	if kind == "png" {
		mime = "image/png"
	} else if kind != "jpeg" {
		return nil, replyError("unsupported", 400, false)
	}
	return []map[string]any{{"type": "text", "text": text}, {"type": "image_url", "image_url": map[string]string{"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}}}, nil
}

func (s *Server) RecentEmotionContext() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) < 4 {
		return ""
	}
	text, _ := s.history[len(s.history)-4].Content.(string)
	r := []rune(text)
	if len(r) > 180 {
		r = r[:180]
	}
	return string(r)
}
