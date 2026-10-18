package main

import (
	"strings"
	"testing"

	"codexpet/reconstruction/tidy"
)

func TestTidyTextNamesWhatMovesAndWhatStays(t *testing.T) {
	plan := tidy.Plan{
		Moves:   []tidy.Move{{Kind: "PDF 文档"}, {Kind: "PDF 文档"}, {Kind: "快捷方式"}},
		Skipped: []tidy.Skip{{Name: "a.tmp", Reason: "下载或保存还没完成"}, {Name: "b.docx", Reason: "正在被其他程序使用"}, {Name: "c.tmp", Reason: "下载或保存还没完成"}},
	}
	q := tidyQuestion(plan)
	for _, want := range []string{"3 个文件", "PDF 文档 2 · 快捷方式 1", "有 3 个文件没动", "2 个下载或保存还没完成", "撤销上次整理"} {
		if !strings.Contains(q, want) {
			t.Fatalf("question lacks %q:\n%s", want, q)
		}
	}
	done := tidySummary(tidy.Report{Moved: plan.Moves}, false)
	if !strings.Contains(done, "3 个文件进了 2 个文件夹") || !strings.Contains(done, "撤销上次整理") {
		t.Fatalf("summary = %q", done)
	}
	back := tidySummary(tidy.Report{Moved: plan.Moves[:1]}, true)
	if !strings.Contains(back, "1 个文件放回") || strings.Contains(back, "撤销上次整理") {
		t.Fatalf("undo summary = %q", back)
	}
	if got := skippedText(nil); got != "" {
		t.Fatalf("nothing skipped still says %q", got)
	}
}
