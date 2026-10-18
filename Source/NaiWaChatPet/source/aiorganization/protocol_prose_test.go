package aiorganization

import (
	"strings"
	"testing"
)

func TestModelProseWithPunctuationIsAccepted(t *testing.T) {
	categories := []Category{
		{ID: "apps", DisplayName: "应用/工具"},
		{ID: "code", DisplayName: "开发: 代码"},
		{ID: "ai", DisplayName: "AI/模型"},
		{ID: "media", DisplayName: "音视频"},
		{ID: "docs", DisplayName: "文档/资料"},
		{ID: "pending", DisplayName: "待整理"},
	}
	normalized := NormalizeCategoryNames(categories)
	for _, category := range normalized {
		if strings.ContainsAny(category.DisplayName, `/\:*?"<>|`) {
			t.Fatalf("unsafe category name: %q", category.DisplayName)
		}
	}
	r := Request{Operation: "classify", ProposeCategories: true,
		Items: []Item{{ItemID: "a", DisplayName: "报告.pdf", Extension: ".pdf", Kind: "file"}}}
	result := Result{
		Categories: normalized,
		Items: []Suggestion{
			{ItemID: "a", CategoryID: "apps", Confidence: 0.8, Reason: "名称含“/”和冒号：属于应用/工具"},
		},
	}
	if err := ValidateResult(r, result); err != nil {
		t.Fatalf("display-only prose rejected: %v", err)
	}
	result.Items[0].Reason = "line\nbreak"
	if ValidateResult(r, result) == nil {
		t.Fatal("accepted control character in reason")
	}
	result.Items[0].Reason = strings.Repeat("长", 161)
	if ValidateResult(r, result) == nil {
		t.Fatal("accepted overlong reason")
	}
	duplicates := NormalizeCategoryNames([]Category{{ID: "a", DisplayName: "AI/工具"}, {ID: "b", DisplayName: "AI-工具"}})
	if duplicates[0].DisplayName == duplicates[1].DisplayName {
		t.Fatal("duplicate labels survived normalization")
	}
}
