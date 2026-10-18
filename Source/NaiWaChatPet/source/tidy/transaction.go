package tidy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Operation struct {
	Move   Move
	Before Identity
	Status string
	Error  string
}
type Journal struct {
	Time             time.Time `json:"time"`
	Root             string    `json:"root"`
	Moves            []Move    `json:"moves"`
	Created          []string  `json:"created,omitempty"`
	Version          int
	Operations       []Operation
	DirectoryIntents []string
}
type Report struct {
	Moved   []Move
	Skipped []Skip
	Journal string
}

var transactionMu sync.Mutex

// AtomicJSON syncs a temporary file before replacing the snapshot. Its parent must be trusted.
func AtomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".naiwa-journal-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(tmp, path)
}
func readJournal(path string) (Journal, error) {
	var j Journal
	f, e := os.Open(path)
	if e != nil {
		return j, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 8<<20+1))
	if e != nil {
		return j, e
	}
	if len(b) > 8<<20 {
		return j, errors.New("日志过大")
	}
	e = json.Unmarshal(b, &j)
	return j, e
}

type Session struct {
	path    string
	j       Journal
	report  Report
	facts   map[string]Identity
	allowed map[string]Move
	made    map[string]bool
	taken   map[string]bool
	fatal   error
	closed  bool
	release func()
}

func Begin(plan Plan, dir string) (*Session, error) {
	if !transactionMu.TryLock() {
		return nil, errors.New("已有整理或撤销任务")
	}
	ok := false
	defer func() {
		if !ok {
			transactionMu.Unlock()
		}
	}()
	if e := safeDirectory(plan.Root); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	if e := safeDirectory(dir); e != nil {
		return nil, e
	}
	release, e := directoryLock(dir)
	if e != nil {
		return nil, e
	}
	defer func() {
		if !ok {
			release()
		}
	}()
	s := &Session{j: Journal{Version: 2, Time: time.Now(), Root: plan.Root}, report: Report{Skipped: append([]Skip(nil), plan.Skipped...)}, facts: plan.Facts, allowed: map[string]Move{}, made: map[string]bool{}, taken: map[string]bool{}}
	s.release = release
	f, e := os.CreateTemp(dir, "tidy-"+time.Now().Format("20060102-150405")+"-*.json")
	if e != nil {
		return nil, e
	}
	s.path = f.Name()
	if e = f.Close(); e != nil {
		return nil, e
	}
	for _, m := range plan.Moves {
		if e := validateMove(plan.Root, m); e != nil {
			os.Remove(s.path)
			return nil, e
		}
		s.allowed[m.From] = m
	}
	if e = s.save(); e != nil {
		os.Remove(s.path)
		return nil, e
	}
	ok = true
	return s, nil
}
func (s *Session) save() error {
	s.j.Created = nil
	for d := range s.made {
		s.j.Created = append(s.j.Created, d)
	}
	sort.Strings(s.j.Created)
	return AtomicJSON(s.path, s.j)
}
func (s *Session) folder(target string) error {
	if e := safeDirectory(s.j.Root); e != nil {
		return e
	}
	if !strings.EqualFold(filepath.Dir(target), s.j.Root) {
		return errors.New("目录越界")
	}
	if _, e := os.Lstat(target); e == nil {
		return safeDirectory(target)
	} else if !os.IsNotExist(e) {
		return e
	}
	s.j.DirectoryIntents = append(s.j.DirectoryIntents, target)
	if e := s.save(); e != nil {
		s.fatal = e
		return e
	}
	if e := os.Mkdir(target, 0755); e != nil {
		return e
	}
	s.made[target] = true
	if e := s.save(); e != nil {
		s.fatal = e
		return e
	}
	return safeDirectory(target)
}
func (s *Session) Prepare(moves []Move) {
	for _, m := range moves {
		if s.fatal != nil {
			return
		}
		if allowed, ok := s.allowed[m.From]; ok && allowed == m {
			if e := validateMove(s.j.Root, m); e == nil {
				_ = s.folder(filepath.Dir(m.To))
			}
		}
	}
}
func (s *Session) Move(m Move) (Move, bool) {
	fail := func(e error) (Move, bool) {
		reason := e.Error()
		if os.IsNotExist(e) {
			reason = reasonGone
		}
		s.report.Skipped = append(s.report.Skipped, Skip{filepath.Base(m.From), reason})
		return Move{}, false
	}
	if s.closed {
		return fail(errors.New("会话已关闭"))
	}
	if s.fatal != nil {
		return fail(s.fatal)
	}
	if allowed, ok := s.allowed[m.From]; !ok || allowed != m {
		return fail(errors.New("文件不在已确认计划中"))
	}
	delete(s.allowed, m.From)
	if e := validateMove(s.j.Root, m); e != nil {
		return fail(e)
	}
	id, e := identify(m.From)
	if e != nil {
		return fail(e)
	}
	expected, ok := s.facts[m.From]
	if !ok || expected != id {
		return fail(errors.New("文件自扫描后已变化，请重新扫描"))
	}
	if e = s.folder(filepath.Dir(m.To)); e != nil {
		return fail(e)
	}
	if _, e = os.Lstat(m.To); e == nil || s.taken[strings.ToLower(m.To)] {
		m.To = free(filepath.Dir(m.To), filepath.Base(m.From), s.taken)
	} else if !os.IsNotExist(e) {
		return fail(e)
	}
	if e = validateMove(s.j.Root, m); e != nil {
		return fail(e)
	}
	op := Operation{Move: m, Before: id, Status: "prepared"}
	s.j.Operations = append(s.j.Operations, op)
	idx := len(s.j.Operations) - 1
	if e = s.save(); e != nil {
		s.fatal = e
		return fail(e)
	}
	if now, e := identify(m.From); e != nil || now != id {
		s.j.Operations[idx].Status = "skipped"
		s.j.Operations[idx].Error = "源文件已变化"
		if e = s.save(); e != nil {
			s.fatal = e
		}
		return fail(errors.New("源文件已变化"))
	}
	if e = validateMove(s.j.Root, m); e == nil {
		e = move(m.From, m.To)
	}
	if e != nil {
		s.j.Operations[idx].Status = "failed"
		s.j.Operations[idx].Error = e.Error()
		if se := s.save(); se != nil {
			s.fatal = se
		}
		return fail(e)
	}
	s.taken[strings.ToLower(m.To)] = true
	s.report.Moved = append(s.report.Moved, m)
	s.j.Moves = append(s.j.Moves, m)
	after, e := identify(m.To)
	if e != nil || after != id {
		s.fatal = errors.New("移动后身份核验失败，请查看恢复记录")
		return m, true
	}
	s.j.Operations[idx].Status = "done"
	if e = s.save(); e != nil {
		s.fatal = fmt.Errorf("文件已移动，完成记录保存失败；停止后续操作: %w", e)
	}
	return m, true
}
func (s *Session) Finish() (Report, error) {
	if s.closed {
		return s.report, s.fatal
	}
	s.closed = true
	defer transactionMu.Unlock()
	defer s.release()
	for d := range s.made {
		if safeDirectory(d) == nil && os.Remove(d) == nil {
			delete(s.made, d)
		}
	}
	if len(s.j.Operations) == 0 && len(s.j.Moves) == 0 && s.fatal == nil {
		os.Remove(s.path)
		return s.report, nil
	}
	s.report.Journal = s.path
	if e := s.save(); e != nil {
		s.fatal = errors.Join(s.fatal, e)
	}
	return s.report, s.fatal
}
func Apply(plan Plan, dir string) (Report, error) {
	s, e := Begin(plan, dir)
	if e != nil {
		return Report{}, e
	}
	for _, m := range plan.Moves {
		s.Move(m)
	}
	return s.Finish()
}
func pending(j Journal) bool {
	if j.Version != 2 {
		return len(j.Moves) > 0
	}
	for _, op := range j.Operations {
		if op.Status == "done" || op.Status == "prepared" || op.Status == "undo_prepared" {
			return true
		}
	}
	return false
}
func Latest(dir string) (string, Journal, bool) {
	paths, _ := filepath.Glob(filepath.Join(dir, "tidy-*.json"))
	sort.Strings(paths)
	for i := len(paths) - 1; i >= 0; i-- {
		if strings.HasSuffix(paths[i], ".undone.json") {
			continue
		}
		j, e := readJournal(paths[i])
		if e == nil && pending(j) {
			return paths[i], j, true
		}
	}
	return "", Journal{}, false
}
func Undo(dir string) (Report, error) {
	if !transactionMu.TryLock() {
		return Report{}, errors.New("已有整理或撤销任务")
	}
	defer transactionMu.Unlock()
	if e := safeDirectory(dir); e != nil {
		return Report{}, e
	}
	release, e := directoryLock(dir)
	if e != nil {
		return Report{}, e
	}
	defer release()
	path, j, ok := Latest(dir)
	if !ok {
		return Report{}, errors.New("没有可以撤销的整理")
	}
	r := Report{Journal: path}
	if j.Version != 2 {
		return r, errors.New("旧版日志缺少文件身份信息，请按日志路径手动审核恢复")
	}
	if e := safeDirectory(j.Root); e != nil {
		return r, e
	}
	for i := len(j.Operations) - 1; i >= 0; i-- {
		op := &j.Operations[i]
		if op.Status != "done" && op.Status != "prepared" && op.Status != "undo_prepared" {
			continue
		}
		m := op.Move
		conflict := func(reason string) {
			r.Skipped = append(r.Skipped, Skip{filepath.Base(m.From), reason})
			op.Error = reason
		}
		if e := validateMove(j.Root, m); e != nil {
			conflict(e.Error())
			continue
		}
		from, fe := identify(m.From)
		to, te := identify(m.To)
		if fe == nil && from == op.Before && os.IsNotExist(te) && (op.Status == "prepared" || op.Status == "undo_prepared") {
			op.Status = "undone"
			if e := AtomicJSON(path, j); e != nil {
				return r, e
			}
			continue
		}
		if te != nil || to != op.Before {
			conflict("目标缺失、已编辑或身份已变化")
			continue
		}
		if !os.IsNotExist(fe) {
			conflict("原位置已被占用或无法安全检查")
			continue
		}
		op.Status = "undo_prepared"
		if e := AtomicJSON(path, j); e != nil {
			return r, e
		}
		if e := move(m.To, m.From); e != nil {
			conflict(e.Error())
			continue
		}
		r.Moved = append(r.Moved, Move{From: m.To, To: m.From, Kind: m.Kind})
		op.Status = "undone"
		op.Error = ""
		if e := AtomicJSON(path, j); e != nil {
			return r, e
		}
	}
	for _, d := range j.Created {
		if strings.EqualFold(filepath.Dir(d), j.Root) && safeDirectory(d) == nil {
			_ = os.Remove(d)
		}
	}
	if e := AtomicJSON(path, j); e != nil {
		return r, e
	}
	if !pending(j) {
		if e := os.Rename(path, strings.TrimSuffix(path, ".json")+".undone.json"); e != nil {
			return r, e
		}
	}
	return r, nil
}

// Recovery is read-only. Prepared records are reconciled by Undo only after confirmation.
func Recovery(dir string) []string {
	var out []string
	paths, _ := filepath.Glob(filepath.Join(dir, "tidy-*.json"))
	for _, p := range paths {
		j, e := readJournal(p)
		if e != nil {
			out = append(out, filepath.Base(p)+": 日志无法读取")
			continue
		}
		for _, op := range j.Operations {
			if op.Status == "prepared" || op.Status == "undo_prepared" {
				out = append(out, filepath.Base(p)+": 存在未完成操作，请预览撤销或人工核对")
			}
		}
	}
	return out
}
