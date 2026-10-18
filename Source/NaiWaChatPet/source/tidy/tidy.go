// Package tidy sorts the loose files on a desktop into a folder per kind:
// PDFs together, spreadsheets together, shortcuts together and so on.
//
// It never deletes anything, never overwrites anything and leaves folders
// where they are. Every move goes into a journal, so a whole run can be put
// back with Undo.
package tidy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"strings"
	"time"
)

// Kind is one destination folder and the extensions that go into it.
type Kind struct {
	Folder string
	Exts   []string
}

// Kinds lists the destinations in the order a summary names them.
var Kinds = []Kind{
	{"PDF 文档", []string{".pdf"}},
	{"Word 文档", []string{".doc", ".docx", ".docm", ".dot", ".dotx", ".rtf", ".odt", ".wps", ".pages"}},
	{"Excel 表格", []string{".xls", ".xlsx", ".xlsm", ".xlsb", ".csv", ".tsv", ".et", ".ods", ".numbers"}},
	{"PPT 演示", []string{".ppt", ".pptx", ".pptm", ".pps", ".ppsx", ".dps", ".odp", ".key"}},
	{"文本笔记", []string{".txt", ".md", ".markdown", ".log"}},
	{"图片", []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".heic", ".heif", ".svg", ".ico", ".tif", ".tiff", ".psd", ".ai", ".raw", ".avif"}},
	{"视频", []string{".mp4", ".mov", ".avi", ".mkv", ".wmv", ".flv", ".webm", ".m4v", ".3gp", ".ts"}},
	{"音频", []string{".mp3", ".wav", ".flac", ".aac", ".m4a", ".ogg", ".wma", ".opus", ".amr"}},
	{"压缩包", []string{".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".iso"}},
	{"安装程序", []string{".exe", ".msi", ".msix", ".msixbundle", ".appx", ".appxbundle"}},
	{"代码", []string{".py", ".ipynb", ".js", ".mjs", ".jsx", ".tsx", ".go", ".c", ".h", ".cpp", ".hpp", ".cs", ".java", ".kt", ".rs", ".rb", ".php", ".html", ".htm", ".css", ".scss", ".json", ".xml", ".yaml", ".yml", ".toml", ".sql", ".sh", ".bat", ".cmd", ".ps1", ".vue", ".lua", ".r", ".m"}},
	{"快捷方式", []string{".lnk", ".url", ".website", ".appref-ms"}},
}

// Other collects whatever no kind claims.
const Other = "其他文件"

// Fresh is how recently a file may have changed before it is left alone:
// it is probably still downloading, or open in an editor.
const Fresh = 2 * time.Minute

// Options adjusts a scan. Every field may be left empty.
type Options struct {
	Context context.Context
	// Now is the scan's clock; zero means time.Now.
	Now   time.Time
	Mode  string
	Rules []Rule
	// Keep names files to leave on the desktop, such as the pet's own shortcut.
	Keep func(name string) bool
	// Hidden reports files the system hides, which are never moved.
	Hidden func(path string) bool
}

type Move struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Skip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Plan is what a run would do. Nothing on disk changes until Apply.
type Plan struct {
	Root    string
	Moves   []Move
	Skipped []Skip
	Facts   map[string]Identity
	Reasons map[string]string
	Review  map[string]bool
}

// Count is how many files go to one destination.
type Count struct {
	Kind string
	N    int
}

// Counts summarises moves by destination, in the order of Kinds.
func Counts(moves []Move) []Count {
	n := map[string]int{}
	for _, m := range moves {
		n[m.Kind]++
	}
	var out []Count
	for _, k := range Kinds {
		if n[k.Folder] > 0 {
			out = append(out, Count{k.Folder, n[k.Folder]})
		}
	}
	if n[Other] > 0 {
		out = append(out, Count{Other, n[Other]})
	}
	known := map[string]bool{}
	for _, c := range out {
		known[c.Kind] = true
	}
	var extra []string
	for k := range n {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		out = append(out, Count{k, n[k]})
	}
	return out
}

// Skipped reasons, kept short enough for the pet's speech bubble.
const (
	reasonFresh    = "刚刚有改动，可能还在下载或编辑"
	reasonPortable = "和桌面上的程序文件是一套，分开会打不开"
	reasonPartial  = "下载或保存还没完成"
	reasonInUse    = "正在被其他程序使用"
	reasonGone     = "整理时已经不在了"
)

func temporary(name string) bool {
	lower := strings.ToLower(name)
	if lower == "desktop.ini" || lower == "thumbs.db" || strings.HasPrefix(name, "~$") || strings.HasPrefix(name, ".~") {
		return true
	}
	for _, ext := range []string{".tmp", ".crdownload", ".part", ".partial", ".download", ".opdownload", ".!ut", ".td"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func kindOf(ext string) string {
	for _, k := range Kinds {
		for _, e := range k.Exts {
			if e == ext {
				return k.Folder
			}
		}
	}
	return Other
}

// portable lists what travels with a program run straight from the desktop.
var portable = map[string]bool{".exe": true, ".dll": true, ".bat": true, ".cmd": true, ".ps1": true, ".ini": true, ".cfg": true, ".dat": true, ".pak": true, ".bin": true}

// Scan plans a tidy of the files directly inside root.
func Scan(root string, opt Options) (Plan, error) {
	if opt.Context == nil {
		opt.Context = context.Background()
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return Plan{}, err
	}
	// A DLL loose on the desktop means a program is being run from there.
	program := false
	for _, e := range entries {
		program = program || !e.IsDir() && (strings.EqualFold(filepath.Ext(e.Name()), ".dll") || strings.EqualFold(filepath.Ext(e.Name()), ".exe"))
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Plan{}, err
	}
	if err = safeDirectory(root); err != nil {
		return Plan{}, err
	}
	plan := Plan{Root: root, Facts: map[string]Identity{}, Reasons: map[string]string{}, Review: map[string]bool{}}
	taken := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if err := opt.Context.Err(); err != nil {
			return Plan{}, err
		}
		path := filepath.Join(root, name)
		if e.IsDir() || e.Type()&fs.ModeSymlink != 0 || !e.Type().IsRegular() {
			plan.Skipped = append(plan.Skipped, Skip{name, "文件夹或链接，保持原位"})
			continue
		}
		if opt.Keep != nil && opt.Keep(name) || opt.Hidden != nil && opt.Hidden(path) {
			plan.Skipped = append(plan.Skipped, Skip{name, "用户固定或系统保护"})
			continue
		}
		if temporary(name) {
			plan.Skipped = append(plan.Skipped, Skip{name, reasonPartial})
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if protectedExtension(ext) || program && portable[ext] {
			plan.Skipped = append(plan.Skipped, Skip{name, reasonPortable})
			continue
		}
		info, err := e.Info()
		if err != nil {
			plan.Skipped = append(plan.Skipped, Skip{name, reasonGone})
			continue
		}
		if opt.Now.Sub(info.ModTime()) < Fresh {
			plan.Skipped = append(plan.Skipped, Skip{name, reasonFresh})
			continue
		}
		identity, err := identify(path)
		if err != nil {
			plan.Skipped = append(plan.Skipped, Skip{name, "无法安全读取文件身份"})
			continue
		}
		plan.Facts[path] = identity
		kind := kindOf(ext)
		if program && kind == kindOf(".py") {
			plan.Skipped = append(plan.Skipped, Skip{name, "桌面存在可执行程序，代码与配置可能属于其依赖"})
			delete(plan.Facts, path)
			continue
		}
		if kind == Other {
			plan.Review[path] = true
		}
		plan.Reasons[path] = "按文件类型分类"
		dir := folderFor(root, kind)
		plan.Moves = append(plan.Moves, Move{From: path, To: free(dir, name, taken), Kind: kind})
	}
	refine(&plan, opt)
	return plan, nil
}

// folderFor is the destination folder for a kind, stepping aside if a file of
// that name is in the way.
func folderFor(root, kind string) string {
	dir := filepath.Join(root, kind)
	if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
		return filepath.Join(root, kind+" (整理)")
	}
	return dir
}

// free returns a path in dir for name that nothing occupies, adding " (2)",
// " (3)" and so on before the extension when it must.
func free(dir, name string, taken map[string]bool) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 10000; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		path := filepath.Join(dir, candidate)
		if taken[strings.ToLower(path)] {
			continue
		}
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			taken[strings.ToLower(path)] = true
			return path
		}
	}
	return filepath.Join(dir, ".naiwa-no-safe-destination")
}
