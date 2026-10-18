package companion

import (
	"codexpet/reconstruction/companionship"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type CaptureScope struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (s *Server) schedulerConfigLocked() companionship.Config {
	return companionship.Config{Seconds: s.settings.ProactiveSeconds, Enabled: s.settings.Proactive, IdleActions: s.settings.IdleActions, Greeting: s.settings.Greeting, Breaks: s.settings.Breaks, Encourage: s.settings.Encourage, Minutes: s.settings.ProactiveMinutes, MaxPerHour: s.settings.MaxReminders, QuietStart: s.settings.QuietStart, QuietEnd: s.settings.QuietEnd, ActionsOnly: s.settings.ActionsOnly}
}

type ScopedCaptureFunc func(context.Context, string) ([]byte, error)
type screenPreview struct {
	id, scope, mime string
	data            []byte
	until           time.Time
	viewed          bool
}

func (s *Server) ConfigureCapture(capture ScopedCaptureFunc, scopes func() []CaptureScope) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scopedCapture = capture
	s.scopeList = scopes
}
func (s *Server) knownScopeLocked(scope string) bool {
	if scope == "monitor:primary" {
		return true
	}
	if s.scopeList == nil {
		return false
	}
	for _, v := range s.scopeList() {
		if v.ID == scope {
			return true
		}
	}
	return false
}
func (s *Server) capture(ctx context.Context, scope string) ([]byte, error) {
	s.mu.Lock()
	fn, old := s.scopedCapture, s.screenCapture
	s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var b []byte
	var e error
	if fn != nil {
		b, e = fn(ctx, scope)
	} else if old != nil && scope == "monitor:primary" {
		b, e = old()
	} else {
		return nil, errors.New("所选截屏范围不可用。")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return b, e
}
func (s *Server) screenScopes(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.scopeList
	s.mu.Unlock()
	scopes := []CaptureScope{{"monitor:primary", "主显示器"}}
	if fn != nil {
		scopes = fn()
	}
	jsonOut(w, map[string]any{"scopes": scopes})
}
func (s *Server) createPreview(ctx context.Context, scope string) (string, error) {
	if scope == "" {
		scope = "monitor:primary"
	}
	s.mu.Lock()
	if !s.knownScopeLocked(scope) {
		s.mu.Unlock()
		return "", errors.New("截屏范围已失效，请刷新显示器和窗口列表。")
	}
	if s.activeCaptureCancel != nil {
		s.mu.Unlock()
		return "", errors.New("正在截屏，请稍等。")
	}
	s.captureSequence++
	sequence := s.captureSequence
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	s.activeCaptureCancel = cancel
	s.captureState = "capturing"
	s.mu.Unlock()
	info, e := s.screenInfo(scope)
	var b []byte
	if e == nil && info.Excluded {
		e = errors.New("观察范围包含排除应用，未截图")
	}
	if e == nil {
		actual := scope
		if info.Scope != "" {
			actual = info.Scope
		}
		b, e = s.capture(ctx, actual)
	}
	if e == nil {
		_, e = makeImageContent("preview", b)
	}
	cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if sequence != s.captureSequence || ctx.Err() != nil && e != nil {
		if sequence == s.captureSequence {
			s.activeCaptureCancel = nil
		}
		return "", errors.New("本次截图已取消或失败，未上传。")
	}
	s.activeCaptureCancel = nil
	if e != nil {
		s.captureState = "error"
		return "", errors.New("截屏失败，未上传。请选择可用范围后重试。")
	}
	var token [16]byte
	if _, e = rand.Read(token[:]); e != nil {
		return "", e
	}
	id := hex.EncodeToString(token[:])
	mime := http.DetectContentType(b)
	s.preview = &screenPreview{id: id, scope: scope, mime: mime, data: append([]byte(nil), b...), until: time.Now().Add(2 * time.Minute)}
	s.captureState = "preview"
	return id, nil
}
func (s *Server) previewCapture(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Scope string `json:"scope"`
	}
	if json.NewDecoder(r.Body).Decode(&c) != nil {
		jsonErr(w, "预览请求格式错误", 400)
		return
	}
	id, e := s.createPreview(r.Context(), c.Scope)
	if e != nil {
		jsonErr(w, e.Error(), 400)
		return
	}
	jsonOut(w, map[string]any{"previewId": id, "expiresSeconds": 120})
}
func (s *Server) previewImage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	p := s.preview
	if p == nil || p.id != r.PathValue("id") || !time.Now().Before(p.until) {
		s.mu.Unlock()
		http.Error(w, "preview expired", 404)
		return
	}
	p.viewed = true
	data := p.data
	mime := p.mime
	s.mu.Unlock()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
func (s *Server) discardPreview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.preview = nil
	if s.settings.ScreenEnabled {
		s.captureState = "active"
	} else {
		s.captureState = "off"
	}
	s.mu.Unlock()
	jsonOut(w, map[string]bool{"ok": true})
}
func (s *Server) observePreview(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Text, PreviewID string
		Confirm         bool
	}
	if json.NewDecoder(r.Body).Decode(&c) != nil {
		jsonErr(w, "观察请求格式错误", 400)
		return
	}
	j, e := s.begin(context.Background(), chatBody{Text: c.Text, WithScreen: true, ConfirmScreen: c.Confirm, PreviewID: c.PreviewID})
	if e != nil {
		jsonErr(w, e.Error(), 400)
		return
	}
	go s.run(j, nil)
	jsonOut(w, map[string]string{"requestId": j.id})
}
func (s *Server) screenAllowedLocked(generation uint64, now time.Time) bool {
	return s.settings.ScreenEnabled && generation == s.screenGeneration && now.Before(s.screenExpires)
}
func (s *Server) stopScreenLocked(reason string) {
	s.settings.ObservationMode = "off"
	s.observationSummary = ""
	s.settings.ScreenEnabled = false
	s.screenExpires = time.Time{}
	s.screenGeneration++
	s.captureSequence++
	s.preview = nil
	if s.activeCaptureCancel != nil {
		s.activeCaptureCancel()
		s.activeCaptureCancel = nil
	}
	if s.activeScreen && s.busy {
		if s.activeCancel != nil {
			s.activeCancel()
		}
		s.generation++
		s.view = ReplyView{RequestID: s.activeID, Stage: "cancelled", Error: "屏幕观察已停止，本次上传请求已取消。", Code: "cancelled", UpdatedAt: time.Now()}
		s.busy = false
		s.activeID = ""
		s.activeCancel = nil
		s.activeScreen = false
	}
	s.captureState = reason
}
func (s *Server) expireLocked(now time.Time) {
	if s.settings.ScreenEnabled && !now.Before(s.screenExpires) {
		s.stopScreenLocked("expired")
	}
	if s.preview != nil && !now.Before(s.preview.until) {
		s.preview = nil
		if !s.settings.ScreenEnabled {
			s.captureState = "off"
		}
	}
}
func (s *Server) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopScreenLocked("paused")
	s.settings.Proactive = false
	s.settings.IdleActions = false
}
func (s *Server) LocalAction() (string, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localAction, s.localSequence
}
func (s *Server) LocalActionDetails() (string, uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localAction, s.localSequence, s.localSequence != 0 && s.localSequence == s.localManualSequence
}
func (s *Server) EmotionEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings.EmotionEnabled == nil || *s.settings.EmotionEnabled
}

func (s *Server) SetEmotionStatus(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emotionStatus = text
}
func (s *Server) EmotionCue() (id, stage, user, reply string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view.RequestID, s.view.Stage, s.lastText, s.view.Text
}
func (s *Server) publishLocalLocked(text, action string) {
	if action != "" {
		s.localAction = action
		s.localSequence++
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	s.sequence++
	s.view = ReplyView{RequestID: "local-" + s.token[:8] + "-" + time.Now().Format("150405.000000000"), Stage: "local", Text: text, UpdatedAt: time.Now()}
	s.events = append(s.events, Event{Kind: "reminder", Text: text, Time: time.Now().Format("15:04")})
	if len(s.events) > 35 {
		s.events = s.events[len(s.events)-35:]
	}
}
func (s *Server) periodic(now time.Time) {
	s.mu.Lock()
	s.expireLocked(now)
	if s.busy || s.activeCaptureCancel != nil || s.desktopInputVisible || s.desktopReplyVisible && s.settings.ProactiveSeconds > 0 {
		s.scheduler.Reason = "正在回复或截图，提醒稍后显示"
		s.mu.Unlock()
		return
	}
	cfg := s.schedulerConfigLocked()
	event := s.scheduler.Tick(now, s.lastInteraction, cfg)
	modelDue := cfg.Seconds > 0 && cfg.Enabled && !cfg.ActionsOnly && event.Text != ""
	if modelDue {
		event.Text = ""
		event.Action = ""
	}
	if event.Text != "" && len(s.history) > 0 && s.settings.Encourage {
		last, _ := s.history[max(0, len(s.history)-2)].Content.(string)
		if strings.Contains(last, "代码") || strings.Contains(last, "学习") {
			event.Text = "刚才的学习问题可以先做一小步，也记得让眼睛休息一下。"
		}
	}
	if event.Text != "" || event.Action != "" {
		if len(s.localQueue) < 3 {
			s.localQueue = append(s.localQueue, localMessage{event.Text, event.Action})
		}
	}
	if len(s.localQueue) > 0 && !s.desktopReplyVisible {
		item := s.localQueue[0]
		s.localQueue = s.localQueue[1:]
		s.publishLocalLocked(item.text, item.action)
		s.lastLocalAt = now
	} else if len(s.localQueue) > 0 {
		s.scheduler.Reason = "提醒已排队，等待当前气泡关闭"
	}
	profile, _ := ModelFromSettings(s.settings)
	due := s.settings.ScreenEnabled && s.settings.ObservationMode == "periodic" && s.settings.AllowPaidProactive && profile.Vision && s.key != "" && now.Sub(s.lastScreen) >= time.Duration(s.settings.ScreenMinutes)*time.Minute
	generation, scope := s.screenGeneration, s.settings.ScreenScope
	if due {
		s.lastScreen = now
	}
	if s.settings.ScreenEnabled && s.settings.ObservationMode == "periodic" && !s.settings.AllowPaidProactive {
		s.screenNotice = "未允许主动付费 API，定时观察不会截图或上传。"
	}
	s.mu.Unlock()
	if modelDue {
		go s.startProactive(false, now)
	}
	if due && !modelDue {
		go s.automaticObservation(generation, scope)
	}
}
func (s *Server) automaticObservation(generation uint64, scope string) {
	s.mu.Lock()
	if !s.screenAllowedLocked(generation, time.Now()) || s.busy || s.activeCaptureCancel != nil {
		s.mu.Unlock()
		return
	}
	s.captureSequence++
	sequence := s.captureSequence
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	s.activeCaptureCancel = cancel
	s.captureState = "capturing"
	s.mu.Unlock()
	info, e := s.screenInfo(scope)
	var b []byte
	if e == nil && info.Excluded {
		e = errors.New("excluded application")
	}
	if e == nil {
		actual := scope
		if info.Scope != "" {
			actual = info.Scope
		}
		b, e = s.capture(ctx, actual)
	}
	if e == nil {
		_, e = makeImageContent("capture", b)
	}
	cancel()
	s.mu.Lock()
	if sequence == s.captureSequence {
		s.activeCaptureCancel = nil
	}
	allowed := s.screenAllowedLocked(generation, time.Now()) && sequence == s.captureSequence
	if !allowed || e != nil {
		if allowed {
			s.captureState = "error"
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	j, e := s.begin(context.Background(), chatBody{Text: "请根据这次授权截图，简短回应与学习、工作或娱乐相关的非敏感内容。"})
	if e != nil {
		return
	}
	s.mu.Lock()
	if !s.screenAllowedLocked(generation, time.Now()) {
		j.cancel()
		s.mu.Unlock()
		_, _ = s.run(j, nil)
		return
	}
	j.screen = true
	j.scheduled = true
	j.screenContext = info
	j.observationSummary = s.observationSummary
	j.image = append([]byte(nil), b...)
	j.automatic = true
	j.authGeneration = generation
	s.activeScreen = true
	s.lastWasScreen = true
	s.captureState = "uploading"
	s.mu.Unlock()
	_, _ = s.run(j, nil)
}
func (s *Server) heartbeat() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case now := <-ticker.C:
			s.periodic(now)
		}
	}
}
