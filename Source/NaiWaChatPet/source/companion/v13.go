package companion

import (
	"codexpet/reconstruction/emotion"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Personality struct {
	Gentleness int `json:"gentleness"`
	Liveliness int `json:"liveliness"`
	Humor      int `json:"humor"`
	Initiative int `json:"initiative"`
}

func DefaultPersonality() Personality { return Personality{85, 65, 40, 40} }

type ScreenContext struct {
	App      string `json:"app"`
	Window   string `json:"window"`
	Scope    string `json:"scope"`
	Excluded bool   `json:"excluded"`
}
type ScreenContextFunc func(scope, exclusions string) (ScreenContext, error)
type SharingStatus struct {
	Mode      string `json:"mode"`
	Allowed   bool   `json:"allowed"`
	Uploading bool   `json:"uploading"`
	Capturing bool   `json:"capturing"`
	Scope     string `json:"scope"`
	Target    string `json:"target"`
	Notice    string `json:"notice"`
}
type SchedulerStatus struct {
	Running bool   `json:"running"`
	Next    string `json:"next"`
	Last    string `json:"last"`
	Reason  string `json:"reason"`
	Queued  int    `json:"queued"`
}

func (s *Server) ConfigureScreenContext(fn ScreenContextFunc) {
	s.mu.Lock()
	s.screenContext = fn
	s.mu.Unlock()
}
func (s *Server) Sharing() SharingStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(time.Now())
	mode := s.settings.ObservationMode
	if mode == "" {
		mode = "off"
	}
	return SharingStatus{Mode: mode, Allowed: s.settings.ScreenEnabled, Uploading: s.captureState == "uploading", Capturing: s.captureState == "capturing", Scope: s.settings.ScreenScope, Target: s.model + " @ " + s.settings.BaseURL, Notice: s.screenNotice}
}
func (s *Server) screenInfo(scope string) (ScreenContext, error) {
	s.mu.Lock()
	fn, excluded := s.screenContext, s.settings.ExcludedApps
	s.mu.Unlock()
	if fn == nil {
		return ScreenContext{Scope: scope}, nil
	}
	return fn(scope, excluded)
}
func NeedsVision(text string) bool {
	t := strings.TrimSpace(text)
	for _, phrase := range []string{"看看我的屏幕", "看下屏幕", "屏幕上", "这张图", "这个报错", "这里有什么问题", "看看这里", "这个代码", "这个PPT", "这个ppt", "你看我在干什么", "截图", "图中", "当前页面", "这个界面"} {
		if strings.Contains(t, phrase) {
			return true
		}
	}
	return false
}
func (s *Server) SubmitWithSharing(text string, omit bool) (string, error) {
	s.mu.Lock()
	mode, scope, generation, allowed := s.settings.ObservationMode, s.settings.ScreenScope, s.screenGeneration, s.settings.ScreenEnabled
	profile, e := ModelFromSettings(s.settings)
	s.mu.Unlock()
	wanted := !omit && allowed && (mode == "each" || mode == "smart" && NeedsVision(text))
	if !wanted {
		return s.submitText(text)
	}
	if e != nil || !profile.Vision {
		return "", replyError("unsupported", 400, false)
	}
	j, e := s.begin(context.Background(), chatBody{Text: text})
	if e != nil {
		return "", e
	}
	s.mu.Lock()
	j.screen = true
	j.automatic = true
	j.authGeneration = generation
	j.captureScope = scope
	s.activeScreen = true
	s.lastWasScreen = true
	s.captureState = "capturing"
	s.mu.Unlock()
	go func() {
		info, err := s.screenInfo(scope)
		if err == nil && info.Excluded {
			err = errors.New("授权范围包含排除的应用，本次未截取或上传图片。")
		}
		if err == nil {
			captureScope := scope
			if info.Scope != "" {
				captureScope = info.Scope
			}
			j.image, err = s.capture(j.ctx, captureScope)
		}
		if err == nil {
			_, err = makeImageContent("capture", j.image)
		}
		if err != nil {
			s.failCapture(j, err)
			return
		}
		s.mu.Lock()
		allowed := s.screenAllowedLocked(generation, time.Now()) && s.activeID == j.id && s.generation == j.generation
		if allowed {
			s.captureState = "uploading"
			j.screenContext = info
			j.observationSummary = s.observationSummary
		}
		s.mu.Unlock()
		if !allowed {
			s.failCapture(j, errors.New("附图授权已撤销，本次没有上传。"))
			return
		}
		_, _ = s.run(j, nil)
	}()
	return j.id, nil
}
func (s *Server) failCapture(j *chatJob, e error) {
	j.cancel()
	j.image = nil
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeID != j.id {
		return
	}
	s.busy = false
	s.activeCancel = nil
	s.activeScreen = false
	s.activeID = ""
	s.captureState = "error"
	s.screenNotice = e.Error()
	s.view = ReplyView{RequestID: j.id, Stage: "error", Error: e.Error(), Code: "capture", UpdatedAt: time.Now()}
}
func (s *Server) RemoveCurrentImage() {
	s.mu.Lock()
	if s.activeScreen && s.busy {
		s.mu.Unlock()
		s.Cancel()
		return
	}
	s.preview = nil
	s.mu.Unlock()
}
func (s *Server) Preview(ctx context.Context, scope string) (string, []byte, error) {
	info, e := s.screenInfo(scope)
	if e != nil {
		return "", nil, e
	}
	if info.Excluded {
		return "", nil, errors.New("选定范围包含排除应用，无法截图。")
	}
	id, e := s.createPreview(ctx, scope)
	if e != nil {
		return "", nil, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.preview == nil || s.preview.id != id {
		return "", nil, errors.New("预览已取消")
	}
	return id, append([]byte(nil), s.preview.data...), nil
}
func (s *Server) MarkNativePreviewViewed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.preview != nil && s.preview.id == id && time.Now().Before(s.preview.until) {
		s.preview.viewed = true
	}
}
func (s *Server) DiscardNativePreview() {
	s.mu.Lock()
	s.preview = nil
	if s.activeCaptureCancel != nil {
		s.activeCaptureCancel()
		s.captureSequence++
	}
	s.mu.Unlock()
}
func (s *Server) SendPreview(text, id string) (string, error) {
	j, e := s.begin(context.Background(), chatBody{Text: text, WithScreen: true, ConfirmScreen: true, PreviewID: id})
	if e != nil {
		return "", e
	}
	go s.run(j, nil)
	return j.id, nil
}
func (s *Server) CaptureScopes() []CaptureScope {
	s.mu.Lock()
	fn := s.scopeList
	s.mu.Unlock()
	if fn != nil {
		return fn()
	}
	return []CaptureScope{{"monitor:primary", "主显示器"}}
}
func (s *Server) TestCompanionship() {
	s.mu.Lock()
	useModel := s.settings.ProactiveSeconds > 0
	s.mu.Unlock()
	if useModel {
		s.startProactive(true, time.Now())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		s.localQueue = append(s.localQueue, localMessage{"先把这条回复聊完，我等你。", "curious"})
		return
	}
	s.publishLocalLocked("已经忙了一阵子啦，放松一下肩膀吧，我在这里陪你。", "comforting")
	s.scheduler.Reset(time.Now())
	s.scheduler.Reason = "本地测试已显示桌面气泡"
}
func (s *Server) RequestDiagnosticEmotion(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !emotion.Known(name) {
		return errors.New("未知情绪")
	}
	s.localAction = name
	s.localSequence++
	s.localManualSequence = s.localSequence
	return nil
}

func (s *Server) NativePreviewRequest() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nativePreviewSequence
}
func (s *Server) openNativePreview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.nativePreviewSequence++
	s.mu.Unlock()
	jsonOut(w, map[string]bool{"ok": true})
}
func (s *Server) testProactive(w http.ResponseWriter, r *http.Request) {
	s.TestCompanionship()
	jsonOut(w, map[string]bool{"ok": true})
}
func (s *Server) testEmotion(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Emotion string `json:"emotion"`
	}
	if json.NewDecoder(r.Body).Decode(&c) != nil {
		jsonErr(w, "请求无效", 400)
		return
	}
	if e := s.RequestDiagnosticEmotion(c.Emotion); e != nil {
		jsonErr(w, e.Error(), 400)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

type localMessage struct{ text, action string }
