package tidy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutionRejectsChangedAndReplacedSource(t *testing.T) {
	root := desk(t, "a.pdf")
	p, _ := Scan(root, later)
	os.WriteFile(filepath.Join(root, "a.pdf"), []byte("changed"), 0600)
	r, e := Apply(p, t.TempDir())
	if e != nil || len(r.Moved) != 0 || len(r.Skipped) != 1 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestUndoEditedFileIsRetained(t *testing.T) {
	root := desk(t, "a.pdf")
	p, _ := Scan(root, later)
	dir := t.TempDir()
	r, _ := Apply(p, dir)
	os.WriteFile(r.Moved[0].To, []byte("edited content"), 0600)
	r, e := Undo(dir)
	if e != nil || len(r.Moved) != 0 || len(r.Skipped) != 1 {
		t.Fatalf("%+v %v", r, e)
	}
	if _, _, ok := Latest(dir); !ok {
		t.Fatal("partial history lost")
	}
}
func TestPartialUndoCanContinue(t *testing.T) {
	root := desk(t, "a.pdf", "b.pdf")
	p, _ := Scan(root, later)
	dir := t.TempDir()
	Apply(p, dir)
	os.WriteFile(filepath.Join(root, "a.pdf"), []byte("new"), 0600)
	r, e := Undo(dir)
	if e != nil || len(r.Moved) != 1 {
		t.Fatalf("%+v %v", r, e)
	}
	os.Remove(filepath.Join(root, "a.pdf"))
	r, e = Undo(dir)
	if e != nil || len(r.Moved) != 1 {
		t.Fatalf("retry %+v %v", r, e)
	}
	if _, _, ok := Latest(dir); ok {
		t.Fatal("completed journal still active")
	}
}
func TestJournalFailureStopsBeforeMove(t *testing.T) {
	root := desk(t, "a.pdf")
	p, _ := Scan(root, later)
	s, e := Begin(p, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s.path = filepath.Join(t.TempDir(), "missing", "journal.json")
	_, ok := s.Move(p.Moves[0])
	s.Finish()
	if ok {
		t.Fatal("moved without durable intent")
	}
	if _, e = os.Stat(p.Moves[0].From); e != nil {
		t.Fatal(e)
	}
}
func TestRecoveredPreparedMoveCanUndo(t *testing.T) {
	root := desk(t, "a.pdf")
	p, _ := Scan(root, later)
	dir := t.TempDir()
	r, _ := Apply(p, dir)
	j, e := readJournal(r.Journal)
	if e != nil {
		t.Fatal(e)
	}
	j.Operations[0].Status = "prepared"
	j.Moves = nil
	AtomicJSON(r.Journal, j)
	if len(Recovery(dir)) != 1 {
		t.Fatal("missing interrupted record")
	}
	r, e = Undo(dir)
	if e != nil || len(r.Moved) != 1 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestRejectEscapeAndBadLabels(t *testing.T) {
	root := desk(t, "a.pdf")
	p, _ := Scan(root, later)
	for _, label := range []string{"../escape", "CON", "a/b", "a\\b", "x:", "bad.", "NUL.txt"} {
		if SafeLabel(label) {
			t.Fatal(label)
		}
		if _, e := Select(p, map[string]string{p.Moves[0].From: label}); e == nil {
			t.Fatal("accepted", label)
		}
	}
	p.Moves[0].To = filepath.Join(t.TempDir(), "a.pdf")
	if _, e := Begin(p, t.TempDir()); e == nil {
		t.Fatal("escaped root")
	}
}
func TestUserRuleKeepAndConflicts(t *testing.T) {
	root := desk(t, "a.pdf", "b.pdf")
	o := later
	o.Rules = []Rule{{Pattern: "a.pdf", Keep: true}, {Pattern: "b.pdf", Target: "甲"}, {Pattern: "b.pdf", Target: "乙"}}
	p, _ := Scan(root, o)
	if len(p.Moves) != 1 || !p.Review[p.Moves[0].From] {
		t.Fatalf("%+v", p)
	}
}
func TestProjectsAreReviewOnlyAndNoSingleLink(t *testing.T) {
	root := desk(t, "低碳园区_v1.pdf", "低碳园区_final.png", "实验1报告.pdf", "实验2报告.png")
	p, _ := Scan(root, later)
	n := 0
	for _, m := range p.Moves {
		if strings.HasPrefix(m.Kind, "项目") {
			n++
			if !p.Review[m.From] {
				t.Fatal("project pre-approved")
			}
		}
	}
	if n != 2 {
		t.Fatalf("project members %d", n)
	}
}
func TestMalformedAIAdviceRejected(t *testing.T) {
	h := []Hint{{ID: "f1"}}
	for _, v := range []string{`{"suggestions":[{"id":"other","group":"项目","category":"project","confidence":1}]}`, `{"suggestions":[{"id":"f1","group":"../../escape","category":"project","confidence":1}]}`, `{"suggestions":[{"id":"f1","group":"项目","category":"delete","confidence":1}]}`, `{"suggestions":[],"command":"delete"}`, `{"suggestions":[]} {}`, `{"suggestions":[{"id":"f1","group":"项目","category":"project"},{"id":"f1","group":"项目","category":"project"}]}`} {
		if _, e := ParseAdvice(v, h); e == nil {
			t.Fatal(v)
		}
	}
}
func TestJournalJSONPreservesUnicode(t *testing.T) {
	root := desk(t, "中文 空格.pdf")
	p, _ := Scan(root, later)
	r, e := Apply(p, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(r.Journal)
	var j Journal
	if json.Unmarshal(b, &j) != nil || j.Operations[0].Move.From != p.Moves[0].From {
		t.Fatal("invalid journal")
	}
}
