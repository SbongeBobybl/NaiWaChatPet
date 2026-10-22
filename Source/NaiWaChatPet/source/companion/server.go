package companion

import (
	"codexpet/reconstruction/companionship"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var webFiles embed.FS

type CaptureFunc func() ([]byte, error)

type Settings struct {
	ProactiveSeconds                                 int          `json:"proactiveSeconds"`
	ObservationMode                                  string       `json:"observationMode"`
	ExcludedApps                                     string       `json:"excludedApps"`
	AllowPaidProactive                               bool         `json:"allowPaidProactive"`
	ActionsOnly                                      bool         `json:"actionsOnly"`
	Personality                                      *Personality `json:"personality,omitempty"`
	MaxReminders, QuietStart, QuietEnd, LeaseMinutes int
	Greeting, Breaks, Encourage, IdleActions         bool
	ScreenScope                                      string `json:"screenScope"`
	EmotionEnabled                                   *bool  `json:"emotionEnabled,omitempty"`
	ModelChoice                                      string `json:"modelChoice"`
	ModelID                                          string `json:"modelId"`
	BaseURL                                          string `json:"baseUrl"`
	Thinking                                         bool   `json:"thinking"`
	Streaming                                        *bool  `json:"streaming,omitempty"`
	CustomVision                                     bool   `json:"customVision"`
	CustomThinking                                   bool   `json:"customThinking"`
	APIKey                                           string `json:"apiKey,omitempty"`
	ClearAPIKey                                      bool   `json:"clearAPIKey,omitempty"`
	Persona                                          string `json:"persona"`
	Length                                           string `json:"length"`
	Proactive                                        bool   `json:"proactive"`
	ProactiveMinutes                                 int    `json:"proactiveMinutes"`
	ScreenEnabled                                    bool   `json:"screenEnabled"`
	ScreenMinutes                                    int    `json:"screenMinutes"`
	ScreenConsent                                    bool   `json:"screenConsent"`
}
type Event struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Time string `json:"time"`
}
type Server struct {
	activeProactive, desktopInputVisible bool
	proactiveReplies                     []string
	emotionDebug                         EmotionDebug
	emotionSequence                      uint64
	emotionTimeout                       time.Duration
	nativePreviewSequence                uint64
	desktopReplyVisible                  bool
	screenContext                        ScreenContextFunc
	observationSummary                   string
	screenNotice                         string
	localQueue                           []localMessage
	lastLocalAt                          time.Time
	memory                               []Message
	storageStatus                        string
	persistenceDisabled                  bool
	demo                                 bool
	emotionStatus                        string
	mu                                   sync.Mutex
	key                                  string
	model                                string
	settings                             Settings
	history                              []Message
	events                               []Event
	screenExpires                        time.Time
	lastProactive                        time.Time
	lastScreen                           time.Time
	lastInteraction                      time.Time
	desktopGameActive                    bool
	running                              bool
	screenCapture                        CaptureFunc
	server                               *http.Server
	listener                             net.Listener
	token                                string
	addr                                 string
	busy                                 bool
	activeCancel                         context.CancelFunc
	done                                 chan struct{}
	client                               *Client
	view                                 ReplyView
	generation                           uint64
	sequence                             uint64
	activeID                             string
	activeScreen                         bool
	lastText                             string
	requestTimeout                       time.Duration
	diagnostics                          []Diagnostic
	closeOnce                            sync.Once
	textVerified                         string
	scopedCapture                        ScopedCaptureFunc
	scopeList                            func() []CaptureScope
	preview                              *screenPreview
	captureState                         string
	captureSequence, screenGeneration    uint64
	activeCaptureCancel                  context.CancelFunc
	lastWasScreen                        bool
	scheduler                            companionship.Scheduler
	localAction                          string
	localSequence                        uint64
	localManualSequence                  uint64
}

type Status struct {
	ProactiveSeconds   int             `json:"proactiveSeconds"`
	Sharing            SharingStatus   `json:"sharing"`
	Scheduler          SchedulerStatus `json:"scheduler"`
	Personality        Personality     `json:"personality"`
	ActionsOnly        bool            `json:"actionsOnly"`
	AllowPaidProactive bool            `json:"allowPaidProactive"`
	ExcludedApps       string          `json:"excludedApps"`
	MemoryPairs        int             `json:"memoryPairs"`
	StorageStatus      string          `json:"storageStatus"`
	Demo               bool            `json:"demo"`
	EmotionStatus      string          `json:"emotionStatus"`
	MaxReminders       int             `json:"maxReminders"`
	QuietStart         int             `json:"quietStart"`
	QuietEnd           int             `json:"quietEnd"`
	LeaseMinutes       int             `json:"leaseMinutes"`
	Greeting           bool            `json:"greeting"`
	Breaks             bool            `json:"breaks"`
	Encourage          bool            `json:"encourage"`
	IdleActions        bool            `json:"idleActions"`
	ScreenScope        string          `json:"screenScope"`
	CaptureState       string          `json:"captureState"`
	EmotionEnabled     bool            `json:"emotionEnabled"`
	ModelChoice        string          `json:"modelChoice"`
	ModelID            string          `json:"modelId"`
	BaseURL            string          `json:"baseUrl"`
	Thinking           bool            `json:"thinking"`
	Streaming          bool            `json:"streaming"`
	CustomVision       bool            `json:"customVision"`
	CustomThinking     bool            `json:"customThinking"`
	CapabilityNote     string          `json:"capabilityNote"`
	Configured         bool            `json:"configured"`
	Model              string          `json:"model"`
	Persona            string          `json:"persona"`
	Length             string          `json:"length"`
	Proactive          bool            `json:"proactive"`
	ProactiveMinutes   int             `json:"proactiveMinutes"`
	ScreenEnabled      bool            `json:"screenEnabled"`
	ScreenMinutes      int             `json:"screenMinutes"`
	ScreenUntil        string          `json:"screenUntil"`
	Events             []Event         `json:"events"`
	Busy               bool            `json:"busy"`
	View               ReplyView       `json:"view"`
	Diagnostics        []Diagnostic    `json:"diagnostics"`
}

func Start(capture CaptureFunc) (*Server, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{
		listener: ln, done: make(chan struct{}), addr: "http://" + ln.Addr().String(), token: hex.EncodeToString(b[:]), screenCapture: capture,
		model: "deepseek-flash", lastInteraction: time.Now(),
		settings: factorySettings(),
	}
	s.client = &Client{HTTP: httpClient, Attempts: 2, Diagnostic: s.recordDiagnostic}
	s.requestTimeout = 120 * time.Second
	s.settings.ModelChoice = "flash"
	s.settings.BaseURL = "https://api.deepseek.com"
	enabled := true
	s.settings.Streaming = &enabled
	s.settings.EmotionEnabled = &enabled
	s.settings.MaxReminders = 120
	s.settings.QuietStart = 23
	s.settings.QuietEnd = 8
	s.settings.LeaseMinutes = 30
	s.settings.Greeting = true
	s.settings.Breaks = true
	s.settings.Encourage = true
	s.settings.ScreenScope = "monitor:primary"
	p := DefaultPersonality()
	s.settings.Personality = &p
	// Restore preferences and encrypted memory; monitoring grants stay off.
	prev := readPreferences()
	if prev.MaxReminders >= 1 && prev.MaxReminders <= 120 && prev.QuietStart >= 0 && prev.QuietStart <= 23 && prev.QuietEnd >= 0 && prev.QuietEnd <= 23 {
		s.settings.MaxReminders = prev.MaxReminders
		s.settings.QuietStart = prev.QuietStart
		s.settings.QuietEnd = prev.QuietEnd
		s.settings.Greeting = prev.Greeting
		s.settings.Breaks = prev.Breaks
		s.settings.Encourage = prev.Encourage
		s.settings.IdleActions = prev.IdleActions
		if prev.LeaseMinutes >= 5 && prev.LeaseMinutes <= 60 {
			s.settings.LeaseMinutes = prev.LeaseMinutes
		}
		if prev.EmotionEnabled != nil {
			s.settings.EmotionEnabled = prev.EmotionEnabled
		}
	}
	if prev.ModelChoice != "" {
		candidate := s.settings
		candidate.ModelChoice = prev.ModelChoice
		candidate.ModelID = prev.ModelID
		candidate.BaseURL = prev.BaseURL
		candidate.Thinking = prev.Thinking
		candidate.Streaming = prev.Streaming
		candidate.CustomVision = prev.CustomVision
		candidate.CustomThinking = prev.CustomThinking
		if p, e := ModelFromSettings(candidate); e == nil {
			s.settings = candidate
			s.model = p.ID
			s.client.Endpoint = p.Endpoint
		}
	}
	if prev.Persona != "" {
		s.settings.Persona = prev.Persona
	}
	if prev.Length != "" {
		s.settings.Length = prev.Length
	}
	if prev.ProactiveMinutes > 0 {
		s.settings.ProactiveMinutes = prev.ProactiveMinutes
	}
	if prev.ScreenMinutes > 0 {
		s.settings.ScreenMinutes = prev.ScreenMinutes
	}
	s.restoreMemory()
	s.restoreV13Preferences(prev)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.page)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("POST /api/chat", s.chat)
	mux.HandleFunc("POST /api/clear", s.clearHistory)
	mux.HandleFunc("POST /api/factory-reset", s.factoryReset)
	mux.HandleFunc("POST /api/pause", s.pauseObservations)
	mux.HandleFunc("POST /api/cancel", s.cancelRequest)
	mux.HandleFunc("POST /api/retry", s.retryRequest)
	mux.HandleFunc("POST /api/test", s.testConnection)
	mux.HandleFunc("POST /api/test-proactive", s.testProactive)
	mux.HandleFunc("POST /api/test-emotion", s.testEmotion)
	mux.HandleFunc("POST /api/native-preview", s.openNativePreview)
	mux.HandleFunc("GET /api/screen/scopes", s.screenScopes)
	mux.HandleFunc("POST /api/preview", s.previewCapture)
	mux.HandleFunc("GET /api/preview/{id}", s.previewImage)
	mux.HandleFunc("POST /api/preview/discard", s.discardPreview)
	mux.HandleFunc("POST /api/observe", s.observePreview)
	s.server = &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	go func() { _ = s.server.Serve(ln) }()
	go s.heartbeat()
	return s, nil
}
func (s *Server) URL() string { return s.addr + "/?session=" + url.QueryEscape(s.token) }
func (s *Server) Close() error {
	s.mu.Lock()
	s.running = false
	s.key = ""
	s.history = nil
	s.settings.ScreenEnabled = false
	if s.activeCaptureCancel != nil {
		s.activeCaptureCancel()
	}
	s.preview = nil
	if s.activeCancel != nil {
		s.activeCancel()
	}
	s.mu.Unlock()
	s.closeOnce.Do(func() { close(s.done) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
func (s *Server) ScreenActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(time.Now())
	return s.settings.ScreenEnabled || s.captureState == "capturing" || s.captureState == "uploading"
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'self'")
		host := strings.ToLower(r.Host)
		if host != strings.ToLower(s.listener.Addr().String()) {
			http.Error(w, "local requests only", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.addr {
			http.Error(w, "request origin not allowed", 403)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site requests rejected", 403)
			return
		}
		if r.Method == http.MethodPost {
			if r.Header.Get("Origin") != s.addr {
				http.Error(w, "request origin not allowed", 403)
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Naiwa-Session")), []byte(s.token)) != 1 {
				http.Error(w, "missing session token", 403)
				return
			}
			if r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "expected application/json", 415)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		}
		if r.Method == http.MethodGet && r.URL.Path != "/" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Naiwa-Session")), []byte(s.token)) != 1 {
				http.Error(w, "missing session token", 403)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("session")), []byte(s.token)) != 1 {
		http.Error(w, "此聊天页面需要从奶蛙右键菜单打开", 403)
		return
	}
	data, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "UI missing", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := strings.ReplaceAll(string(data), "__NAIWA_TOKEN__", s.token)
	_, _ = io.WriteString(w, page)
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	sharing := s.Sharing()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(time.Now())
	state := Status{Configured: s.key != "", Model: s.model, Persona: s.settings.Persona, Length: s.settings.Length,
		Proactive: s.settings.Proactive, ProactiveMinutes: s.settings.ProactiveMinutes,
		ScreenEnabled: s.settings.ScreenEnabled, ScreenMinutes: s.settings.ScreenMinutes, Events: append([]Event(nil), s.events...), Busy: s.busy}
	state.View = s.view
	state.Demo = s.demo
	state.Sharing = sharing
	state.Scheduler = s.schedulerStatusLocked(time.Now())
	state.Personality = DefaultPersonality()
	if s.settings.Personality != nil {
		state.Personality = *s.settings.Personality
	}
	state.ActionsOnly = s.settings.ActionsOnly
	state.AllowPaidProactive = s.settings.AllowPaidProactive
	state.ProactiveSeconds = s.settings.ProactiveSeconds
	state.ExcludedApps = s.settings.ExcludedApps
	state.MemoryPairs = len(s.memory) / 2
	state.StorageStatus = s.storageStatus
	state.EmotionStatus = s.emotionStatus
	state.MaxReminders = s.settings.MaxReminders
	state.QuietStart = s.settings.QuietStart
	state.QuietEnd = s.settings.QuietEnd
	state.LeaseMinutes = s.settings.LeaseMinutes
	state.Greeting = s.settings.Greeting
	state.Breaks = s.settings.Breaks
	state.Encourage = s.settings.Encourage
	state.IdleActions = s.settings.IdleActions
	state.ScreenScope = s.settings.ScreenScope
	state.CaptureState = s.captureState
	state.EmotionEnabled = s.settings.EmotionEnabled == nil || *s.settings.EmotionEnabled
	state.ModelChoice = s.settings.ModelChoice
	state.ModelID = s.model
	state.BaseURL = s.settings.BaseURL
	state.Thinking = s.settings.Thinking
	state.Streaming = s.settings.Streaming == nil || *s.settings.Streaming
	state.CustomVision = s.settings.CustomVision
	state.CustomThinking = s.settings.CustomThinking
	profile, _ := ModelFromSettings(s.settings)
	state.CapabilityNote = "模型能力来自官方接口规范；当前会话尚未进行真实连接测试。"
	if !profile.VerifiedSpec {
		state.CapabilityNote = "自定义模型能力由你确认，文本连接测试不能证明图片能力。"
	}
	if s.textVerified == s.model {
		state.CapabilityNote += " 文本连接已通过本次测试。"
	}
	if s.demo {
		state.CapabilityNote = "离线演示：未连接真实模型服务，所有回复均为本地测试文本。"
	}
	state.Diagnostics = append([]Diagnostic(nil), s.diagnostics...)
	if s.settings.ScreenEnabled {
		state.ScreenUntil = s.screenExpires.Format(time.RFC3339)
	}
	jsonOut(w, state)
}
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var c Settings
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		jsonErr(w, "设置格式错误", 400)
		return
	}
	if len([]rune(c.Persona)) > 1500 {
		jsonErr(w, "奶蛙人设不得超过1500字", 400)
		return
	}
	if c.Length != "short" && c.Length != "normal" && c.Length != "long" && c.Length != "one" {
		c.Length = "short"
	}
	if c.ProactiveMinutes < 1 || c.ProactiveMinutes > 180 {
		c.ProactiveMinutes = 25
	}
	if c.ProactiveSeconds < 30 || c.ProactiveSeconds > 10800 {
		c.ProactiveSeconds = 30
	}
	if c.ScreenMinutes < 1 || c.ScreenMinutes > 60 {
		c.ScreenMinutes = 15
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ModelChoice == "" {
		c.ModelChoice = s.settings.ModelChoice
	}
	if c.BaseURL == "" {
		c.BaseURL = s.settings.BaseURL
	}
	if c.Streaming == nil {
		c.Streaming = s.settings.Streaming
	}
	if err := s.validateV13Locked(&c); err != nil {
		jsonErr(w, err.Error(), 400)
		return
	}
	profile, profileErr := ModelFromSettings(c)
	if profileErr != nil {
		jsonErr(w, profileErr.Error(), 400)
		return
	}
	if c.ScreenScope == "" {
		c.ScreenScope = "monitor:primary"
	}
	if !s.knownScopeLocked(c.ScreenScope) {
		jsonErr(w, "请刷新并选择有效截屏范围", 400)
		return
	}
	if c.ScreenEnabled && !profile.Vision {
		jsonErr(w, "这个模型未确认支持图片，无法开启屏幕观察", 400)
		return
	}
	if c.ScreenEnabled && s.settings.ScreenEnabled && c.ScreenScope != s.settings.ScreenScope && !c.ScreenConsent {
		jsonErr(w, "更改观察范围需要重新确认授权", 400)
		return
	}
	if c.LeaseMinutes < 5 || c.LeaseMinutes > 60 {
		c.LeaseMinutes = 30
	}
	if c.MaxReminders < 1 || c.MaxReminders > 120 {
		c.MaxReminders = 120
	}
	if c.QuietStart < 0 || c.QuietStart > 23 || c.QuietEnd < 0 || c.QuietEnd > 23 {
		jsonErr(w, "免打扰时间必须是 0 到 23 点", 400)
		return
	}
	if c.ScreenEnabled && !s.settings.ScreenEnabled && !c.ScreenConsent {
		jsonErr(w, "开启自动截屏需要明确勾选授权", 400)
		return
	}
	if s.settings.BaseURL != profile.BaseURL {
		// Credentials and conversations are bound to the chosen provider.
		s.key = ""
		s.history = nil
		s.memory = nil
		s.proactiveReplies = nil
		s.activeProactive = false
		s.generation++
		s.resetEmotionLocked("provider_changed")
		if s.activeCancel != nil {
			s.activeCancel()
		}
		s.busy, s.activeID, s.activeCancel = false, "", nil
		s.view = ReplyView{}
		s.lastText = ""
		s.lastWasScreen = false
		s.stopScreenLocked("reconfigured")
		if !c.ScreenConsent {
			c.ScreenEnabled = false
		}
	}
	if c.ClearAPIKey {
		s.key = ""
		s.history = nil
		s.memory = nil
		s.proactiveReplies = nil
		s.activeProactive = false
		s.generation++
		s.resetEmotionLocked("key_cleared")
		if s.activeCancel != nil {
			s.activeCancel()
		}
		s.view = ReplyView{RequestID: s.activeID, Stage: "cancelled", Error: "API Key 已清除。", Code: "cancelled", UpdatedAt: time.Now()}
		s.busy = false
		s.activeID = ""
		s.activeCancel = nil
		c.ScreenEnabled = false
		s.stopScreenLocked("off")
	} else if strings.TrimSpace(c.APIKey) != "" {
		key := strings.TrimSpace(c.APIKey)
		if len(key) < 8 || len(key) > 512 || strings.ContainsAny(key, "\r\n\t ") {
			jsonErr(w, "API Key 格式不正确", 400)
			return
		}
		s.key = key
	}
	if strings.TrimSpace(c.Persona) == "" {
		c.Persona = DefaultPersona
	}
	wasEnabled := s.settings.ScreenEnabled
	if wasEnabled && c.ScreenEnabled && c.ScreenConsent {
		s.stopScreenLocked("reconfigured")
		wasEnabled = false
	}
	s.settings.Persona = c.Persona
	s.settings.ObservationMode = c.ObservationMode
	s.settings.ExcludedApps = c.ExcludedApps
	s.settings.AllowPaidProactive = c.AllowPaidProactive
	s.settings.ActionsOnly = c.ActionsOnly
	s.settings.Personality = c.Personality
	if c.Proactive != s.settings.Proactive {
		s.scheduler.Reset(time.Now())
		if !c.Proactive && s.activeProactive {
			s.generation++
			if s.activeCancel != nil {
				s.activeCancel()
			}
			s.activeProactive = false
			s.busy = false
			s.activeID = ""
			s.activeCancel = nil
			s.view = ReplyView{}
		}
	}
	if s.model != profile.ID || s.settings.BaseURL != profile.BaseURL {
		if wasEnabled && !c.ScreenConsent {
			s.stopScreenLocked("paused")
			c.ScreenEnabled = false
		}
		if s.activeCancel != nil {
			s.activeCancel()
		}
		s.generation++
		s.resetEmotionLocked("model_changed")
		s.textVerified = ""
	}
	s.model = profile.ID
	s.client.Endpoint = profile.Endpoint
	s.settings.ModelChoice = profile.Choice
	s.settings.ModelID = profile.ID
	s.settings.BaseURL = profile.BaseURL
	s.settings.Thinking = c.Thinking
	s.settings.Streaming = c.Streaming
	s.settings.CustomVision = c.CustomVision
	s.settings.CustomThinking = c.CustomThinking
	s.settings.Length = c.Length
	s.settings.Proactive = c.Proactive
	s.settings.ProactiveMinutes = c.ProactiveMinutes
	s.settings.ProactiveSeconds = c.ProactiveSeconds
	s.settings.MaxReminders = c.MaxReminders
	s.settings.QuietStart = c.QuietStart
	s.settings.QuietEnd = c.QuietEnd
	s.settings.Greeting = c.Greeting
	s.settings.Breaks = c.Breaks
	s.settings.Encourage = c.Encourage
	s.settings.IdleActions = c.IdleActions
	s.settings.ScreenScope = c.ScreenScope
	s.settings.LeaseMinutes = c.LeaseMinutes
	if c.EmotionEnabled != nil {
		s.settings.EmotionEnabled = c.EmotionEnabled
	}
	s.settings.ScreenEnabled = c.ScreenEnabled
	s.settings.ScreenMinutes = c.ScreenMinutes
	if c.ScreenEnabled && (!wasEnabled || c.ScreenConsent) {
		s.screenGeneration++
		s.screenExpires = time.Now().Add(time.Duration(c.LeaseMinutes) * time.Minute)
		s.captureState = "active"
		s.lastScreen = time.Now()
	}
	if !c.ScreenEnabled {
		if wasEnabled || s.activeScreen || s.activeCaptureCancel != nil {
			s.stopScreenLocked("paused")
		} else {
			s.screenExpires = time.Time{}
		}
	}
	s.lastInteraction = time.Now()
	// Ordinary preferences and DPAPI protected secrets use separate files.
	if err := writePreferences(s.settings); err != nil { // Keeping this as non-fatal prevents an inaccessible profile dir from breaking chat.
		_ = err
	}
	s.persistMemoryLocked()
	jsonOut(w, map[string]any{"ok": true, "configured": s.key != "", "storageStatus": s.storageStatus, "screenUntil": s.screenExpires.Format(time.RFC3339)})
}
func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.history = nil
	s.events = nil
	s.memory = nil
	s.proactiveReplies = nil
	s.activeProactive = false
	s.generation++
	s.resetEmotionLocked("history_cleared")
	if s.activeCancel != nil {
		s.activeCancel()
	}
	s.view = ReplyView{}
	s.busy = false
	s.activeID = ""
	s.activeCancel = nil
	s.lastText = ""
	s.lastWasScreen = false
	s.lastInteraction = time.Now()
	s.persistMemoryLocked()
	s.mu.Unlock()
	jsonOut(w, map[string]bool{"ok": true})
}
func (s *Server) pauseObservations(w http.ResponseWriter, r *http.Request) {
	s.Pause()
	jsonOut(w, map[string]bool{"ok": true})
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func jsonErr(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

var ErrStopped = errors.New("companion server stopped")

func (s *Server) Diagnostic() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("companion on 127.0.0.1, key=%t screen=%t", s.key != "", s.settings.ScreenEnabled)
}
