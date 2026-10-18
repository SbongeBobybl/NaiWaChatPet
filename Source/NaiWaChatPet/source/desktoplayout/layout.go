// Package desktoplayout computes deterministic positions without filesystem operations.
package desktoplayout

import (
	"errors"
	"math"
	"sort"
	"strings"
)

type Point struct{ X, Y int32 }
type Rect struct{ Left, Top, Right, Bottom int32 }
type Monitor struct {
	ID      string
	Area    Rect
	Primary bool
}
type Icon struct {
	Key, Name, Category, Identity string
	Position                      Point
	Fixed                         bool
}
type Snapshot struct {
	ID          string
	Icons       []Icon
	Monitors    []Monitor
	Spacing     Point
	AutoArrange bool
	DPI         uint32
	Zones       map[string]string
}
type Placement struct {
	Key            string
	Before, Target Point
	Identity       string
}
type Plan struct {
	Snapshot Snapshot
	Mode     string
	Moves    []Placement
	Skipped  []string
}

func inside(p Point, r Rect) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}
func near(a, b Point, s Point) bool { return abs(a.X-b.X) < s.X && abs(a.Y-b.Y) < s.Y }
func abs(a int32) int32 {
	if a < 0 {
		return -a
	}
	return a
}
func PlanGrid(s Snapshot, mode string, pins map[string]bool, all bool) (Plan, error) {
	p := Plan{Snapshot: s, Mode: mode}
	if s.AutoArrange {
		return p, errors.New("Windows 自动排列已开启，请在桌面右键 → 查看中关闭后重新扫描")
	}
	if s.Spacing.X < 16 || s.Spacing.Y < 16 {
		return p, errors.New("无法读取可靠图标间距")
	}
	if mode != "columns" && mode != "rows" && mode != "zones" && mode != "minimal" {
		return p, errors.New("未知排版模式")
	}
	var cells []Point
	for _, m := range s.Monitors {
		if !all && !m.Primary {
			continue
		}
		var grid []Point
		for x := m.Area.Left + s.Spacing.X/8; x+s.Spacing.X <= m.Area.Right; x += s.Spacing.X {
			for y := m.Area.Top + s.Spacing.Y/8; y+s.Spacing.Y <= m.Area.Bottom; y += s.Spacing.Y {
				grid = append(grid, Point{x, y})
			}
		}
		if mode == "rows" {
			sort.SliceStable(grid, func(i, j int) bool {
				if grid[i].Y != grid[j].Y {
					return grid[i].Y < grid[j].Y
				}
				return grid[i].X < grid[j].X
			})
		}
		cells = append(cells, grid...)
	}
	eligible := func(i Icon) bool {
		if i.Fixed || pins[i.Key] {
			return false
		}
		for _, m := range s.Monitors {
			if (all || m.Primary) && inside(i.Position, m.Area) {
				return true
			}
		}
		return false
	}
	var moving []Icon
	var occupied []Point
	for _, i := range s.Icons {
		if eligible(i) {
			moving = append(moving, i)
		} else {
			occupied = append(occupied, i.Position)
		}
	}
	available := cells[:0]
	for _, c := range cells {
		blocked := false
		for _, o := range occupied {
			if near(c, o, s.Spacing) {
				blocked = true
				break
			}
		}
		if !blocked {
			available = append(available, c)
		}
	}
	cells = available
	if len(moving) > len(cells) {
		return p, errors.New("可用网格容量不足，请减少固定项或选择所有显示器")
	}
	sort.SliceStable(moving, func(i, j int) bool {
		if mode == "zones" && moving[i].Category != moving[j].Category {
			return moving[i].Category < moving[j].Category
		}
		return moving[i].Key < moving[j].Key
	})
	for _, i := range moving {
		pick := 0
		if mode == "zones" {
			zone := map[string]string{"apps": "left-top", "projects": "right-top", "temp": "left-bottom", "references": "right-bottom"}[i.Category]
			if s.Zones[i.Category] != "" {
				zone = s.Zones[i.Category]
			}
			for j, c := range cells {
				matched := false
				for _, m := range s.Monitors {
					if !inside(c, m.Area) {
						continue
					}
					left := c.X < (m.Area.Left+m.Area.Right)/2
					top := c.Y < (m.Area.Top+m.Area.Bottom)/2
					matched = (!strings.Contains(zone, "left") || left) && (!strings.Contains(zone, "right") || !left) && (!strings.Contains(zone, "top") || top) && (!strings.Contains(zone, "bottom") || !top)
				}
				if matched {
					pick = j
					break
				}
			}
		}
		if mode == "minimal" {
			best := math.MaxFloat64
			for j, c := range cells {
				d := math.Hypot(float64(c.X-i.Position.X), float64(c.Y-i.Position.Y))
				if d < best {
					best = d
					pick = j
				}
			}
		}
		target := cells[pick]
		cells = append(cells[:pick], cells[pick+1:]...)
		if target != i.Position {
			p.Moves = append(p.Moves, Placement{i.Key, i.Position, target, i.Identity})
		}
	}
	return p, nil
}

// Constrain parses a deliberately finite vocabulary; unrecognized clauses fail rather than execute a guessed command.
func Constrain(s *Snapshot, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if len([]rune(text)) > 300 {
		return errors.New("排布指令过长")
	}
	s.Zones = map[string]string{}
	clauses := strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("，,。；;\n", r) })
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" || clause == "只排图标" || clause == "不要移动文件" {
			continue
		}
		matched := false
		for _, entry := range []struct{ Word, Category string }{{"软件", "apps"}, {"项目", "projects"}, {"临时", "temp"}, {"截图", "temp"}, {"资料", "references"}} {
			if !strings.Contains(clause, entry.Word) {
				continue
			}
			zone := ""
			if strings.Contains(clause, "左") {
				zone = "left"
			} else if strings.Contains(clause, "右") {
				zone = "right"
			}
			if strings.Contains(clause, "上") {
				zone += "-top"
			} else if strings.Contains(clause, "下") {
				zone += "-bottom"
			}
			if zone != "" {
				s.Zones[entry.Category] = zone
				matched = true
			}
		}
		if strings.Contains(clause, "固定") || strings.Contains(clause, "别动") || strings.Contains(clause, "不动") {
			if strings.Contains(clause, "回收站") || strings.Contains(clause, "系统") {
				matched = true
			}
			for i := range s.Icons {
				if strings.Contains(clause, s.Icons[i].Name) {
					s.Icons[i].Fixed = true
					matched = true
				}
			}
		}
		if !matched {
			return errors.New("无法解释这段排布要求：" + clause + "。支持例如：软件放左上，项目放右上，资料放右下，回收站固定")
		}
	}
	return nil
}

func Validate(p Plan) error {
	seen := map[string]bool{}
	byKey := map[string]Icon{}
	for _, i := range p.Snapshot.Icons {
		byKey[i.Key] = i
	}
	var occupied []Point
	for _, m := range p.Moves {
		i, ok := byKey[m.Key]
		if !ok || i.Fixed || seen[m.Key] || i.Identity != m.Identity || i.Position != m.Before {
			return errors.New("图标身份或计划无效")
		}
		seen[m.Key] = true
		insideWork := false
		for _, monitor := range p.Snapshot.Monitors {
			r := monitor.Area
			if m.Target.X >= r.Left && m.Target.Y >= r.Top && m.Target.X+p.Snapshot.Spacing.X <= r.Right && m.Target.Y+p.Snapshot.Spacing.Y <= r.Bottom {
				insideWork = true
			}
		}
		if !insideWork {
			return errors.New("图标目标越界")
		}
		occupied = append(occupied, m.Target)
	}
	for i, a := range occupied {
		for _, b := range occupied[i+1:] {
			if near(a, b, p.Snapshot.Spacing) {
				return errors.New("图标目标重叠")
			}
		}
	}
	for _, i := range p.Snapshot.Icons {
		if seen[i.Key] {
			continue
		}
		for _, target := range occupied {
			if near(target, i.Position, p.Snapshot.Spacing) {
				return errors.New("目标与固定图标重叠")
			}
		}
	}
	return nil
}
func Compatible(a, b Snapshot) bool {
	if a.DPI != b.DPI || a.Spacing != b.Spacing || len(a.Monitors) != len(b.Monitors) {
		return false
	}
	for i := range a.Monitors {
		if a.Monitors[i] != b.Monitors[i] {
			return false
		}
	}
	return true
}
func Restore(original Plan, current Snapshot) (Plan, error) {
	p := Plan{Snapshot: current, Mode: "restore"}
	if !Compatible(original.Snapshot, current) {
		return p, errors.New("显示器或 DPI 已变化，不能直接恢复旧坐标")
	}
	if current.AutoArrange {
		return p, errors.New("自动排列已开启，请关闭后恢复")
	}
	old := map[string]Placement{}
	for _, m := range original.Moves {
		old[m.Key] = m
	}
	var fixed []Point
	for _, i := range current.Icons {
		m, ok := old[i.Key]
		if !ok || i.Identity != m.Identity || i.Position != m.Target || i.Fixed {
			fixed = append(fixed, i.Position)
			if ok {
				p.Skipped = append(p.Skipped, i.Name+": 已手动移动或身份变化")
			}
			continue
		}
		p.Moves = append(p.Moves, Placement{i.Key, i.Position, m.Before, i.Identity})
	}
	for {
		var kept []Placement
		changed := false
		for _, m := range p.Moves {
			blocked := false
			for _, pt := range fixed {
				if near(m.Target, pt, current.Spacing) {
					blocked = true
					break
				}
			}
			if blocked {
				fixed = append(fixed, m.Before)
				p.Skipped = append(p.Skipped, m.Key+": 原位置被占用")
				changed = true
			} else {
				kept = append(kept, m)
			}
		}
		p.Moves = kept
		if !changed {
			break
		}
	}
	return p, nil
}
