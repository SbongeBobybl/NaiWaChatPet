// Package organizer serves an authenticated local preview. Browser input never supplies filesystem paths.
package organizer

import (
	"codexpet/reconstruction/desktoplayout"
	"codexpet/reconstruction/tidy"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed web.html
var assets embed.FS

type Backend struct {
	Carry          func(context.Context, tidy.Plan, string, func(Status)) (tidy.Report, error)
	Root, Journals string
	Hidden         func(string) bool
	ReadLayout     func() (desktoplayout.Snapshot, error)
	ApplyLayout    func(desktoplayout.Plan) ([]string, error)
	Model          func() any
	Advise         func(context.Context, []tidy.Hint, bool) ([]tidy.Suggestion, error)
}
type Row struct {
	ID, Name, From, To, Group, Reason string
	Review, Sensitive                 bool
	Size                              int64
}
type Status struct {
	Phase                 string
	Moved, Total, Skipped int
	Message               string
}
type LayoutPreferences struct {
	Pins              map[string]bool
	Mode, Instruction string
	All               bool
}
type Server struct {
	layoutPreferences LayoutPreferences
	b                 Backend
	ln                net.Listener
	http              *http.Server
	token, addr       string
	gate              sync.Mutex
	mu                sync.Mutex
	status            Status
	cancel            context.CancelFunc
	plan, prepared    tidy.Plan
	ids               map[string]string
	revision          string
	layout            desktoplayout.Plan
	layoutID          string
	layoutPins        map[string]bool
	layoutAll         bool
	rules             []tidy.Rule
	modelSnapshot     string
}

func New(b Backend) (*Server, error) {
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	s := &Server{b: b, ln: ln, token: nonce(), addr: "http://" + ln.Addr().String(), status: Status{Phase: "idle"}}
	if data, e := os.ReadFile(filepath.Join(b.Journals, "rules.json")); e == nil && len(data) < 64<<10 {
		_ = json.Unmarshal(data, &s.rules)
	}
	if data, e := os.ReadFile(filepath.Join(b.Journals, "layout-preferences.json")); e == nil && len(data) < 128<<10 {
		_ = json.Unmarshal(data, &s.layoutPreferences)
	}
	s.http = &http.Server{Handler: http.HandlerFunc(s.handle), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go s.http.Serve(ln)
	return s, nil
}
func nonce() string {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func (s *Server) URL() string { return s.addr + "/#" + s.token }
func (s *Server) Close() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.http.Close()
}
func (s *Server) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }
func (s *Server) set(st Status)  { s.mu.Lock(); s.status = st; s.mu.Unlock() }
func output(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")
	if r.Host != s.ln.Addr().String() || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "local only", 403)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/" {
		b, _ := assets.ReadFile("web.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Naiwa-Session")), []byte(s.token)) != 1 {
		http.Error(w, "session required", 403)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/status" {
		output(w, s.Status())
		return
	}
	if r.Method != "POST" || r.Header.Get("Origin") != s.addr || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "same-origin JSON required", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var req struct {
		Revision, Mode, Instruction     string
		Selected                        map[string]string
		IDs                             []string
		Pins                            map[string]bool
		All, Consent, Remember, Confirm bool
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	if r.URL.Path == "/cancel" {
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		output(w, map[string]bool{"ok": true})
		return
	}
	if !s.gate.TryLock() {
		http.Error(w, "已有任务，请等待或取消", 409)
		return
	}
	defer s.gate.Unlock()
	ctx, cancel := context.WithCancel(r.Context())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); s.cancel = nil; s.mu.Unlock() }()
	var result any
	var err error
	switch r.URL.Path {
	case "/preferences":
		result = s.layoutPreferences
	case "/scan":
		s.modelSnapshot = modelFingerprint(s.b.Model())
		s.set(Status{Phase: "scanning"})
		s.prepared = tidy.Plan{}
		s.layoutID = ""
		s.plan, err = tidy.Scan(s.b.Root, tidy.Options{Context: ctx, Hidden: s.b.Hidden, Mode: req.Mode, Rules: s.rules})
		if err == nil {
			s.revision = nonce()
			s.ids = map[string]string{}
			var rows []Row
			for i, m := range s.plan.Moves {
				id := fmt.Sprintf("f%04d", i+1)
				s.ids[id] = m.From
				rows = append(rows, Row{id, filepath.Base(m.From), m.From, m.To, m.Kind, s.plan.Reasons[m.From], s.plan.Review[m.From], tidy.Sensitive(filepath.Base(m.From)), s.plan.Facts[m.From].Size})
			}
			result = map[string]any{"rows": rows, "skipped": s.plan.Skipped, "revision": s.revision, "model": s.b.Model(), "recovery": tidy.Recovery(s.b.Journals)}
		}
		s.set(Status{Phase: "preview"})
	case "/prepare":
		s.prepared = tidy.Plan{}
		if req.Revision != s.revision || s.revision == "" {
			err = errors.New("预览已过期，请重新扫描")
			break
		}
		selected := map[string]string{}
		for id, target := range req.Selected {
			p, ok := s.ids[id]
			if !ok {
				err = errors.New("未知文件 ID")
				break
			}
			selected[p] = target
		}
		if err != nil {
			break
		}
		s.prepared, err = tidy.Select(s.plan, selected)
		if err == nil {
			result = map[string]any{"moves": s.prepared.Moves, "revision": s.revision}
		}
	case "/duplicates":
		if !req.Consent || req.Revision != s.revision || s.revision == "" {
			err = errors.New("请确认本次本地内容读取")
			break
		}
		selected := map[string]string{}
		for _, id := range req.IDs {
			path, ok := s.ids[id]
			if !ok {
				err = errors.New("未知文件 ID")
				break
			}
			selected[path] = "本地检查"
		}
		if err != nil {
			break
		}
		var p tidy.Plan
		p, err = tidy.Select(s.plan, selected)
		if err == nil {
			s.set(Status{Phase: "checking"})
			result, err = tidy.FindDuplicates(ctx, p)
			s.set(Status{Phase: "preview"})
		}
	case "/apply":
		if !req.Confirm || req.Revision != s.revision || len(s.prepared.Moves) == 0 {
			err = errors.New("请先预演并确认本次文件移动")
			break
		}
		plan := s.prepared
		s.prepared = tidy.Plan{}
		s.revision = ""
		s.layoutID = ""
		s.set(Status{Phase: "moving", Total: len(plan.Moves)})
		var report tidy.Report
		if s.b.Carry != nil {
			report, err = s.b.Carry(ctx, plan, s.b.Journals, s.set)
		} else {
			var session *tidy.Session
			session, err = tidy.Begin(plan, s.b.Journals)
			if err == nil {
				for _, m := range plan.Moves {
					if ctx.Err() != nil {
						break
					}
					_, moved := session.Move(m)
					st := s.Status()
					if moved {
						st.Moved++
					} else {
						st.Skipped++
					}
					st.Total = len(plan.Moves)
					s.set(st)
				}
				report, err = session.Finish()
			}
		}
		{
			result = report
			s.set(Status{Phase: "complete", Moved: len(report.Moved), Total: len(plan.Moves), Skipped: len(report.Skipped), Message: "文件整理已结束，可独立预览图标排布"})
			if req.Remember {
				for _, m := range report.Moved {
					s.rules = append(s.rules, tidy.Rule{Pattern: escapeGlob(filepath.Base(m.From)), Target: m.Kind})
				}
				if e := tidy.AtomicJSON(filepath.Join(s.b.Journals, "rules.json"), s.rules); e != nil {
					err = errors.Join(err, e)
				}
			}
		}
	case "/history":
		_, j, ok := tidy.Latest(s.b.Journals)
		result = map[string]any{"available": ok, "journal": j, "recovery": tidy.Recovery(s.b.Journals)}
	case "/undo":
		if !req.Confirm {
			err = errors.New("请确认恢复文件")
			break
		}
		s.set(Status{Phase: "undo"})
		result, err = tidy.Undo(s.b.Journals)
		s.revision = ""
		s.layoutID = ""
		s.set(Status{Phase: "complete"})
	case "/layout":
		s.modelSnapshot = modelFingerprint(s.b.Model())
		s.layoutID = ""
		s.set(Status{Phase: "scanning"})
		var snap desktoplayout.Snapshot
		snap, err = s.b.ReadLayout()
		if err == nil {
			err = desktoplayout.Constrain(&snap, req.Instruction)
		}
		if err == nil {
			s.layout, err = desktoplayout.PlanGrid(snap, req.Mode, req.Pins, req.All)
		}
		if err == nil {
			s.layoutPins = req.Pins
			s.layoutAll = req.All
			if req.Remember {
				s.layoutPreferences = LayoutPreferences{req.Pins, req.Mode, req.Instruction, req.All}
				if e := os.MkdirAll(s.b.Journals, 0700); e != nil {
					err = e
					break
				}
				if e := tidy.AtomicJSON(filepath.Join(s.b.Journals, "layout-preferences.json"), s.layoutPreferences); e != nil {
					err = e
					break
				}
			}
			s.layoutID = nonce()
			result = map[string]any{"plan": s.layout, "revision": s.layoutID, "model": s.b.Model()}
		}
		s.set(Status{Phase: "preview"})
	case "/layout-ai":
		if s.modelSnapshot != modelFingerprint(s.b.Model()) {
			err = errors.New("模型连接已改变，请重新预览上传目的地")
			break
		}
		if !req.Consent || req.Revision != s.layoutID || s.layoutID == "" {
			err = errors.New("需要本次图标名称上传授权")
			break
		}
		var hints []tidy.Hint
		index := map[string]int{}
		for i := range s.layout.Snapshot.Icons {
			index[fmt.Sprintf("i%04d", i+1)] = i
		}
		seen := map[string]bool{}
		for _, id := range req.IDs {
			i, ok := index[id]
			if !ok || seen[id] {
				err = errors.New("图标 ID 无效")
				break
			}
			seen[id] = true
			icon := s.layout.Snapshot.Icons[i]
			if icon.Fixed || tidy.Sensitive(icon.Name) {
				err = errors.New("固定或敏感图标不上传")
				break
			}
			hints = append(hints, tidy.Hint{ID: id, Name: icon.Name, Ext: strings.ToLower(filepath.Ext(icon.Name)), Age: "unknown"})
		}
		if err != nil {
			break
		}
		s.set(Status{Phase: "analyzing"})
		var advice []tidy.Suggestion
		advice, err = s.b.Advise(ctx, hints, true)
		s.set(Status{Phase: "preview"})
		if err != nil {
			break
		}
		snap := s.layout.Snapshot
		snap.Icons = append([]desktoplayout.Icon(nil), snap.Icons...)
		for _, a := range advice {
			category := a.Category
			if category == "project" {
				category = "projects"
			}
			switch category {
			case "apps", "projects", "temp", "references":
			default:
				category = "references"
			}
			snap.Icons[index[a.ID]].Category = category
		}
		s.layout, err = desktoplayout.PlanGrid(snap, "zones", s.layoutPins, s.layoutAll)
		if err == nil {
			s.layoutID = nonce()
			result = map[string]any{"plan": s.layout, "revision": s.layoutID, "advice": advice}
		}
	case "/layout-apply":
		if !req.Confirm || req.Revision != s.layoutID || s.layoutID == "" {
			err = errors.New("请先预览并确认图标位置")
			break
		}
		s.layoutID = ""
		s.set(Status{Phase: "layout"})
		if err = desktoplayout.Validate(s.layout); err != nil {
			break
		}
		if err = os.MkdirAll(s.b.Journals, 0700); err == nil {
			err = tidy.AtomicJSON(filepath.Join(s.b.Journals, "layout-"+time.Now().Format("20060102-150405.000000000")+".json"), s.layout)
		}
		if err == nil {
			err = tidy.AtomicJSON(filepath.Join(s.b.Journals, "layout-last.json"), s.layout)
		}
		if err == nil {
			var done []string
			done, err = s.b.ApplyLayout(s.layout)
			result = map[string]any{"verified": len(done), "total": len(s.layout.Moves)}
			summary := map[string]any{"time": time.Now(), "verified": done, "total": len(s.layout.Moves)}
			if err != nil {
				summary["error"] = err.Error()
			}
			if e := tidy.AtomicJSON(filepath.Join(s.b.Journals, "layout-result.json"), summary); e != nil {
				err = errors.Join(err, e)
			}
			s.set(Status{Phase: "complete", Moved: len(done), Total: len(s.layout.Moves)})
		}
	case "/layout-restore-preview":
		s.layoutID = ""
		var original desktoplayout.Plan
		var b []byte
		b, err = os.ReadFile(filepath.Join(s.b.Journals, "layout-last.json"))
		if err == nil && len(b) > 8<<20 {
			err = errors.New("布局日志过大")
		}
		if err == nil {
			err = json.Unmarshal(b, &original)
		}
		if err == nil {
			var current desktoplayout.Snapshot
			current, err = s.b.ReadLayout()
			if err == nil {
				s.layout, err = desktoplayout.Restore(original, current)
			}
		}
		if err == nil {
			s.layoutID = nonce()
			result = map[string]any{"plan": s.layout, "revision": s.layoutID}
		}
	case "/layout-restore":
		if !req.Confirm || req.Revision != s.layoutID || s.layoutID == "" || s.layout.Mode != "restore" {
			err = errors.New("请先预览恢复位置")
			break
		}
		s.layoutID = ""
		var done []string
		done, err = s.b.ApplyLayout(s.layout)
		result = map[string]any{"verified": len(done), "total": len(s.layout.Moves), "skipped": s.layout.Skipped}
	case "/ai":
		if s.modelSnapshot != modelFingerprint(s.b.Model()) {
			err = errors.New("模型连接已改变，请重新扫描并确认上传目的地")
			break
		}
		if !req.Consent || req.Revision != s.revision || s.revision == "" {
			err = errors.New("需要本次上传授权")
			break
		}
		var hints []tidy.Hint
		seen := map[string]bool{}
		for _, id := range req.IDs {
			path, ok := s.ids[id]
			if !ok || seen[id] {
				err = errors.New("候选无效")
				break
			}
			seen[id] = true
			name := filepath.Base(path)
			if tidy.Sensitive(name) {
				err = errors.New("敏感文件名不上传")
				break
			}
			hints = append(hints, tidy.Hint{ID: id, Name: name, Ext: strings.ToLower(filepath.Ext(name)), Age: "unknown"})
		}
		if err == nil {
			s.set(Status{Phase: "analyzing"})
			result, err = s.b.Advise(ctx, hints, true)
			s.set(Status{Phase: "preview"})
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		st := s.Status()
		if st.Phase == "moving" || st.Phase == "layout" || st.Phase == "undo" {
			st.Phase = "error"
		}
		st.Message = err.Error()
		s.set(st)
		w.WriteHeader(400)
		output(w, map[string]any{"error": err.Error(), "result": result})
		return
	}
	output(w, result)
}
func escapeGlob(s string) string {
	r := strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]")
	return r.Replace(s)
}
func modelFingerprint(v any) string { b, _ := json.Marshal(v); return string(b) }
