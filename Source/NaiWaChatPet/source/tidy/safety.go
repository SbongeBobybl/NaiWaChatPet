package tidy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Identity struct {
	Size     int64
	Modified int64
	Volume   uint32
	Index    uint64
}

func protectedExtension(ext string) bool {
	return portable[ext] || strings.Contains("|.lnk|.url|.website|.appref-ms|.sys|.msi|.msix|.appx|.msixbundle|.appxbundle|", "|"+ext+"|")
}
func safeDirectory(path string) error {
	path = filepath.Clean(path)
	for p := path; ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 || unsafeAttributes(p) {
			return fmt.Errorf("不安全目录: %s", p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func SafeLabel(s string) bool {
	if s == "" || len([]rune(s)) > 60 || strings.TrimSpace(s) != s || strings.HasSuffix(s, ".") || strings.ContainsAny(s, "/\\:*?\"<>|\x00") || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.Split(s, ".")[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
		return false
	}
	return true
}
func validateMove(root string, m Move) error {
	if !filepath.IsAbs(root) || !filepath.IsAbs(m.From) || !filepath.IsAbs(m.To) {
		return errors.New("路径必须为绝对路径")
	}
	if !strings.EqualFold(filepath.Dir(m.From), root) || !strings.EqualFold(filepath.Dir(filepath.Dir(m.To)), root) {
		return errors.New("整理路径超出桌面边界")
	}
	if !SafeLabel(filepath.Base(filepath.Dir(m.To))) || !SafeLabel(filepath.Base(m.To)) || strings.HasPrefix(filepath.Base(m.To), ".naiwa-") {
		return errors.New("目标名称不安全")
	}
	if protectedExtension(strings.ToLower(filepath.Ext(m.From))) || temporary(filepath.Base(m.From)) {
		return errors.New("受保护文件不能移动")
	}
	if e := safeDirectory(root); e != nil {
		return e
	}
	parent := filepath.Dir(m.To)
	if _, e := os.Lstat(parent); e == nil {
		return safeDirectory(parent)
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}

type Rule struct {
	Pattern string
	Target  string
	Keep    bool
}

var suffix = regexp.MustCompile("(?i)([_ .-]*(v[0-9]+([.][0-9]+)*|final[0-9]*|最终版|修订版|副本|20[0-9]{2}[-_][0-9]{2}[-_][0-9]{2}|[(][0-9]+[)]))+$")

func Normalize(name string) string {
	return strings.Trim(suffix.ReplaceAllString(strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name))), ""), " _-.")
}
func refine(p *Plan, opt Options) {
	groups := map[string][]int{}
	for i, m := range p.Moves {
		k := Normalize(filepath.Base(m.From))
		if len([]rune(k)) >= 4 && k != "实验报告" && k != "工作报告" && k != "研究报告" {
			groups[k] = append(groups[k], i)
		}
	}
	if opt.Mode != "conservative" {
		// Shared anchored prefixes are candidates only, never transitive merges.
		// Each member must start with the same specific prefix; generic headings are excluded.
		if len(p.Moves) <= 1500 {
			keys := make([]string, len(p.Moves))
			for i, m := range p.Moves {
				keys[i] = Normalize(filepath.Base(m.From))
			}
			anchors := map[string]bool{}
			for i, a := range keys {
				for j := i + 1; j < len(keys); j++ {
					b := keys[j]
					ar, br := []rune(a), []rune(b)
					n := 0
					for n < len(ar) && n < len(br) && ar[n] == br[n] {
						n++
					}
					if n < 4 {
						continue
					}
					k := strings.Trim(string(ar[:n]), " _-.")
					if len([]rune(k)) < 4 || strings.HasPrefix(k, "实验") || strings.HasPrefix(k, "报告") || strings.HasPrefix(k, "新建") || strings.HasPrefix(k, "截图") {
						continue
					}
					anchors[k] = true
				}
			}
			for k := range anchors {
				var indexes []int
				exts := map[string]bool{}
				for i, key := range keys {
					if strings.HasPrefix(key, k) {
						indexes = append(indexes, i)
						exts[filepath.Ext(p.Moves[i].From)] = true
					}
				}
				if len(indexes) >= 2 && len(exts) >= 2 {
					groups[k] = indexes
				}
			}
		}
		// Process stronger, longer anchors first; ambiguity remains a review item.
		var ordered []string
		for k := range groups {
			ordered = append(ordered, k)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if len(ordered[i]) != len(ordered[j]) {
				return len(ordered[i]) > len(ordered[j])
			}
			return ordered[i] < ordered[j]
		})
		claimed := map[int]bool{}
		for _, k := range ordered {
			indexes := groups[k]
			var free []int
			for _, i := range indexes {
				if !claimed[i] {
					free = append(free, i)
				}
			}
			indexes = free
			if len(indexes) < 2 || !SafeLabel(k) {
				continue
			}
			for _, i := range indexes {
				claimed[i] = true
				m := &p.Moves[i]
				m.Kind = "项目 · " + k
				m.To = filepath.Join(p.Root, m.Kind, filepath.Base(m.From))
				p.Reasons[m.From] = "共同名称锚点“" + k + "”：项目/版本候选，需逐项确认"
				p.Review[m.From] = true
			}
		}
	}
	if opt.Mode == "deep" {
		for _, m := range p.Moves {
			if opt.Now.Sub(time.Unix(0, p.Facts[m.From].Modified)) > 180*24*time.Hour {
				p.Reasons[m.From] += "；超过 180 天未修改，可考虑归档（不代表未使用）"
				p.Review[m.From] = true
			}
		}
	}
	var moves []Move
	for _, m := range p.Moves {
		keep := false
		targets := map[string]bool{}
		for _, r := range opt.Rules {
			matched, e := filepath.Match(strings.ToLower(r.Pattern), strings.ToLower(filepath.Base(m.From)))
			if e != nil || !matched {
				continue
			}
			if r.Keep {
				keep = true
			}
			if SafeLabel(r.Target) {
				targets[r.Target] = true
			}
		}
		if keep {
			p.Skipped = append(p.Skipped, Skip{filepath.Base(m.From), "用户规则：保留原位"})
			continue
		}
		if len(targets) > 1 {
			p.Review[m.From] = true
			p.Reasons[m.From] = "用户规则冲突，需确认"
		} else {
			for target := range targets {
				m.Kind = target
				m.To = filepath.Join(p.Root, target, filepath.Base(m.From))
				p.Review[m.From] = false
				p.Reasons[m.From] = "已保存的用户规则"
			}
		}
		moves = append(moves, m)
	}
	p.Moves = moves
	taken := map[string]bool{}
	for i := range p.Moves {
		m := &p.Moves[i]
		m.To = free(folderFor(p.Root, m.Kind), filepath.Base(m.From), taken)
	}
}
func Select(p Plan, selected map[string]string) (Plan, error) {
	q := p
	q.Moves = nil
	for _, m := range p.Moves {
		target, ok := selected[m.From]
		if !ok {
			continue
		}
		if !SafeLabel(target) {
			return Plan{}, errors.New("目标文件夹名称无效")
		}
		m.Kind = target
		m.To = filepath.Join(p.Root, target, filepath.Base(m.From))
		if e := validateMove(p.Root, m); e != nil {
			return Plan{}, e
		}
		q.Moves = append(q.Moves, m)
	}
	if len(q.Moves) != len(selected) {
		return Plan{}, errors.New("文件列表已失效，请重新扫描")
	}
	sort.Slice(q.Moves, func(i, j int) bool { return q.Moves[i].From < q.Moves[j].From })
	return q, nil
}
