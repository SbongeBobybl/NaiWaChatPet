package tidy

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// desk makes a fake desktop holding the named files, each containing its name.
func desk(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		if strings.HasSuffix(n, "/") {
			if err := os.Mkdir(filepath.Join(root, n), 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(root, n), []byte(n), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// later is a clock far enough ahead that no file counts as just changed.
var later = Options{Now: time.Now().Add(time.Hour)}

func listing(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != root {
			rel, _ := filepath.Rel(root, p)
			if d.IsDir() {
				rel += "/"
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func TestScanSortsFilesByKind(t *testing.T) {
	root := desk(t, "report.PDF", "budget.xlsx", "data.csv", "notes.txt", "photo.jpg", "app.lnk", "site.url", "song.mp3", "thing.xyz", "noext")
	plan, err := Scan(root, later)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"report.PDF": "PDF 文档", "budget.xlsx": "Excel 表格", "data.csv": "Excel 表格", "notes.txt": "文本笔记", "photo.jpg": "图片",
		"song.mp3": "音频", "thing.xyz": Other, "noext": Other}
	if len(plan.Moves) != len(want) {
		t.Fatalf("planned %d moves, want %d", len(plan.Moves), len(want))
	}
	for _, m := range plan.Moves {
		name := filepath.Base(m.From)
		if m.Kind != want[name] || m.To != filepath.Join(root, want[name], name) {
			t.Fatalf("%s goes to %s (%s), want %s", name, m.To, m.Kind, want[name])
		}
	}
	counts := Counts(plan.Moves)
	if counts[0] != (Count{"PDF 文档", 1}) || counts[1] != (Count{"Excel 表格", 2}) || counts[len(counts)-1] != (Count{Other, 2}) {
		t.Fatalf("counts = %v", counts)
	}
	// A scan changes nothing.
	if got := listing(t, root); len(got) != len(want)+2 {
		t.Fatalf("scan touched the desktop: %v", got)
	}
}

func TestScanLeavesFoldersAndProtectedFilesAlone(t *testing.T) {
	root := desk(t, "Projects/", "desktop.ini", "~$budget.xlsx", "movie.mp4.crdownload", "secret.txt", "NaiWaChatPet.lnk", "draft.docx")
	opt := later
	opt.Keep = func(name string) bool { return name == "NaiWaChatPet.lnk" }
	opt.Hidden = func(path string) bool { return filepath.Base(path) == "secret.txt" }
	plan, err := Scan(root, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Moves) != 1 || filepath.Base(plan.Moves[0].From) != "draft.docx" {
		t.Fatalf("moves = %+v", plan.Moves)
	}
	skipped := map[string]bool{}
	for _, s := range plan.Skipped {
		skipped[s.Name] = true
	}
	for _, name := range []string{"desktop.ini", "~$budget.xlsx", "movie.mp4.crdownload"} {
		if !skipped[name] {
			t.Fatalf("%s should be reported as left alone, skipped = %v", name, plan.Skipped)
		}
	}
	if !skipped["secret.txt"] || !skipped["NaiWaChatPet.lnk"] || !skipped["Projects"] {
		t.Fatalf("protected entries must be visible in preview: %v", plan.Skipped)
	}
}

func TestScanLeavesFreshFilesAlone(t *testing.T) {
	root := desk(t, "old.pdf", "new.pdf")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, "old.pdf"), past, past); err != nil {
		t.Fatal(err)
	}
	plan, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Moves) != 1 || filepath.Base(plan.Moves[0].From) != "old.pdf" || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != reasonFresh {
		t.Fatalf("moves %+v, skipped %+v", plan.Moves, plan.Skipped)
	}
}

func TestScanKeepsAPortableProgramTogether(t *testing.T) {
	root := desk(t, "game.exe", "engine.dll", "settings.ini", "manual.pdf")
	plan, err := Scan(root, later)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Moves) != 1 || filepath.Base(plan.Moves[0].From) != "manual.pdf" || len(plan.Skipped) != 3 {
		t.Fatalf("moves %+v, skipped %+v", plan.Moves, plan.Skipped)
	}
	// A lone executable remains protected as well.
	root = desk(t, "setup.exe")
	if plan, _ := Scan(root, later); len(plan.Moves) != 0 {
		t.Fatalf("a lone executable must stay in place: %+v", plan)
	}
}

func TestScanAvoidsNameClashes(t *testing.T) {
	root := desk(t, "a.pdf", "PDF 文档/", "图片", "b.png")
	if err := os.WriteFile(filepath.Join(root, "PDF 文档", "a.pdf"), []byte("older"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := Scan(root, later)
	if err != nil {
		t.Fatal(err)
	}
	to := map[string]string{}
	for _, m := range plan.Moves {
		to[filepath.Base(m.From)] = m.To
	}
	if to["a.pdf"] != filepath.Join(root, "PDF 文档", "a (2).pdf") {
		t.Fatalf("a.pdf planned for %s", to["a.pdf"])
	}
	// A file called 图片 is in the way of the folder, so the folder steps aside.
	if to["b.png"] != filepath.Join(root, "图片 (整理)", "b.png") {
		t.Fatalf("b.png planned for %s", to["b.png"])
	}
}

func TestApplyThenUndoRestoresTheDesktop(t *testing.T) {
	root := desk(t, "a.pdf", "b.docx", "c.lnk", "Keep/", "d.zip")
	before := listing(t, root)
	journals := t.TempDir()
	plan, err := Scan(root, later)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Apply(plan, journals)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Moved) != 3 || report.Journal == "" {
		t.Fatalf("report = %+v", report)
	}
	after := listing(t, root)
	for _, want := range []string{"PDF 文档/a.pdf", "Word 文档/b.docx", "c.lnk", "压缩包/d.zip", "Keep/"} {
		found := false
		for _, got := range after {
			found = found || got == want
		}
		if !found {
			t.Fatalf("after tidying, %s is missing from %v", want, after)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(root, "PDF 文档", "a.pdf")); string(b) != "a.pdf" {
		t.Fatal("a moved file lost its contents")
	}
	if _, _, ok := Latest(journals); !ok {
		t.Fatal("no journal to undo")
	}
	undo, err := Undo(journals)
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.Moved) != 3 || len(undo.Skipped) != 0 {
		t.Fatalf("undo = %+v", undo)
	}
	if got := listing(t, root); strings.Join(got, "|") != strings.Join(before, "|") {
		t.Fatalf("undo left %v, want %v", got, before)
	}
	if _, _, ok := Latest(journals); ok {
		t.Fatal("an undone run can be undone again")
	}
}

func TestUndoNeverOverwritesANewFile(t *testing.T) {
	root := desk(t, "a.pdf")
	journals := t.TempDir()
	plan, _ := Scan(root, later)
	if _, err := Apply(plan, journals); err != nil {
		t.Fatal(err)
	}
	// The user saves a new a.pdf on the desktop before undoing.
	if err := os.WriteFile(filepath.Join(root, "a.pdf"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	undo, err := Undo(journals)
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.Moved) != 0 || len(undo.Skipped) != 1 {
		t.Fatalf("undo = %+v", undo)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.pdf")); string(b) != "new" {
		t.Fatal("undo overwrote the new file")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "PDF 文档", "a.pdf")); string(b) != "a.pdf" {
		t.Fatal("the tidied file was lost")
	}
}

func TestApplyCarriesOnPastAFileThatVanished(t *testing.T) {
	root := desk(t, "a.pdf", "b.pdf")
	plan, _ := Scan(root, later)
	if err := os.Remove(filepath.Join(root, "a.pdf")); err != nil {
		t.Fatal(err)
	}
	report, err := Apply(plan, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Moved) != 1 || len(report.Skipped) != 1 || report.Skipped[0].Reason != reasonGone {
		t.Fatalf("report = %+v", report)
	}
}

func TestApplyWithNothingMovedLeavesNoTrace(t *testing.T) {
	root := desk(t, "a.pdf")
	plan, _ := Scan(root, later)
	if err := os.Remove(filepath.Join(root, "a.pdf")); err != nil {
		t.Fatal(err)
	}
	journals := t.TempDir()
	report, err := Apply(plan, journals)
	if err != nil {
		t.Fatal(err)
	}
	if report.Journal != "" || len(listing(t, root)) != 0 || len(listing(t, journals)) != 0 {
		t.Fatalf("an empty run left %v on the desktop and %v in the journals", listing(t, root), listing(t, journals))
	}
}

func TestMoveRefusesToReplace(t *testing.T) {
	root := desk(t, "a.txt", "b.txt")
	if err := move(filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")); err == nil {
		t.Fatal("move replaced an existing file")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "b.txt")); string(b) != "b.txt" {
		t.Fatal("the existing file was changed")
	}
}

func TestASessionJournalsEachMoveAndCleansUpUnusedFolders(t *testing.T) {
	root := desk(t, "a.pdf", "b.png")
	journals := t.TempDir()
	plan, _ := Scan(root, later)
	s, err := Begin(plan, journals)
	if err != nil {
		t.Fatal(err)
	}
	s.Prepare(plan.Moves)
	for _, dir := range []string{"PDF 文档", "图片"} {
		if info, err := os.Stat(filepath.Join(root, dir)); err != nil || !info.IsDir() {
			t.Fatalf("Prepare did not make %s ahead of the moves", dir)
		}
	}
	var pdf Move
	for _, m := range plan.Moves {
		if m.Kind == "PDF 文档" {
			pdf = m
		}
	}
	if _, ok := s.Move(pdf); !ok {
		t.Fatal("the first move failed")
	}
	// Cut short here: the move so far is already in a journal that undoes it.
	if _, j, ok := Latest(journals); !ok || len(j.Moves) != 1 {
		t.Fatalf("after one move the journal holds %+v", j)
	}
	report, err := s.Finish()
	if err != nil || len(report.Moved) != 1 {
		t.Fatalf("finish = %+v, %v", report, err)
	}
	// The picture never moved, so its prepared folder is gone again.
	if _, err := os.Stat(filepath.Join(root, "图片")); err == nil {
		t.Fatal("an unused prepared folder was left on the desktop")
	}
	if _, err := Undo(journals); err != nil {
		t.Fatal(err)
	}
	if got := listing(t, root); strings.Join(got, "|") != "a.pdf|b.png" {
		t.Fatalf("after undo the desktop holds %v", got)
	}
}
