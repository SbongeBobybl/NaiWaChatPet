package main

import (
	"fmt"
	"strings"

	"codexpet/reconstruction/tidy"
)

// petShortcut is the pet's own desktop shortcut, which tidying leaves in place:
// the pet would only put it back on its next start.
const petShortcut = "NaiWaChatPet.lnk"

func countsText(moves []tidy.Move) string {
	var parts []string
	for _, c := range tidy.Counts(moves) {
		parts = append(parts, fmt.Sprintf("%s %d", c.Kind, c.N))
	}
	return strings.Join(parts, " · ")
}

// skippedText explains in one line why some files stayed where they were.
func skippedText(skipped []tidy.Skip) string {
	if len(skipped) == 0 {
		return ""
	}
	n := map[string]int{}
	var order []string
	for _, s := range skipped {
		if n[s.Reason] == 0 {
			order = append(order, s.Reason)
		}
		n[s.Reason]++
	}
	var parts []string
	for _, reason := range order {
		parts = append(parts, fmt.Sprintf("%d 个%s", n[reason], reason))
	}
	return fmt.Sprintf("有 %d 个文件没动：%s。", len(skipped), strings.Join(parts, "，"))
}

// tidyQuestion asks before anything moves, saying exactly what will.
func tidyQuestion(plan tidy.Plan) string {
	text := fmt.Sprintf("奶蛙要把桌面上的 %d 个文件放进分类文件夹：\n%s\n\n文件夹、正在使用或刚有改动的文件，以及奶蛙自己的快捷方式都不会动。", len(plan.Moves), countsText(plan.Moves))
	if s := skippedText(plan.Skipped); s != "" {
		text += "\n" + s
	}
	return text + "\n\n整理完可以随时在右键菜单里“撤销上次整理”。现在开始吗？"
}

// tidySummary is what the pet says when a tidy or an undo is finished.
func tidySummary(r tidy.Report, undo bool) string {
	var text string
	switch {
	case undo && len(r.Moved) > 0:
		text = fmt.Sprintf("已经把 %d 个文件放回桌面原来的位置了。", len(r.Moved))
	case undo:
		text = "这次没有文件能放回去。"
	case len(r.Moved) == 0:
		text = "这次一个文件也没能移动。"
	default:
		folders := len(tidy.Counts(r.Moved))
		text = fmt.Sprintf("整理好啦！%d 个文件进了 %d 个文件夹：%s。", len(r.Moved), folders, countsText(r.Moved))
	}
	if s := skippedText(r.Skipped); s != "" {
		text += "\n" + s
	}
	if !undo && len(r.Moved) > 0 {
		text += "\n不满意的话，右键 → 撤销上次整理。"
	}
	return text
}
