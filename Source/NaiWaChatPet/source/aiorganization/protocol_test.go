package aiorganization

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDeskBoxResultRejectsUntrustedReferences(t *testing.T) {
	r := Request{Items: []Item{{ItemID: "known"}}, Categories: []Category{{ID: "study"}}, Operation: "classify"}
	valid := Suggestion{ItemID: "known", CategoryID: "study", Confidence: 0.8, Reason: "name matches"}
	for _, tc := range []struct {
		name  string
		items []Suggestion
	}{
		{"unknown", []Suggestion{{ItemID: "injected", CategoryID: "study", Confidence: 0.9, Reason: "x"}}},
		{"duplicate", []Suggestion{valid, valid}},
		{"category", []Suggestion{{ItemID: "known", CategoryID: "C:/temp", Confidence: 0.9, Reason: "x"}}},
		{"negative", []Suggestion{{ItemID: "known", CategoryID: "study", Confidence: -1, Reason: "x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ValidateResult(r, Result{Items: tc.items}) == nil {
				t.Fatal("accepted")
			}
		})
	}
	r.Items[0].IsProtected = true
	if ValidateResult(r, Result{Items: []Suggestion{valid}}) == nil {
		t.Fatal("protection overridden")
	}
}
func TestDeskBoxFrameBoundsAndStrictJSON(t *testing.T) {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(MaxMessage+1))
	var r Request
	if ReadFrame(&b, &r) == nil {
		t.Fatal("oversized frame")
	}
	for _, input := range []string{"```json\n{}\n```", "{} {}", `{"command":"move"}`} {
		if DecodeStrict([]byte(input), &r) == nil {
			t.Fatal("accepted", input)
		}
	}
}

func TestProposedTaxonomyBoundsAndNames(t *testing.T) {
	r := Request{Operation: "classify", ProposeCategories: true, Items: []Item{{ItemID: "a"}}}
	cats := []Category{{ID: "dev", DisplayName: "编程"}, {ID: "study", DisplayName: "课程"}, {ID: "media", DisplayName: "媒体"}, {ID: "design", DisplayName: "设计"}, {ID: "ai", DisplayName: "AI工具"}, {ID: "other", DisplayName: "待确认"}}
	result := Result{Categories: cats, Items: []Suggestion{{ItemID: "a", CategoryID: "dev", Confidence: .9, Reason: "名称是开发工具"}}}
	if err := ValidateResult(r, result); err != nil {
		t.Fatal(err)
	}
	result.Categories = cats[:3]
	if ValidateResult(r, result) == nil {
		t.Fatal("accepted fewer than six")
	}
	result.Categories = append(append([]Category{}, cats...), cats...)
	result.Categories = append(result.Categories, Category{ID: "nine", DisplayName: "九"})
	if ValidateResult(r, result) == nil {
		t.Fatal("accepted more than ten")
	}
	result.Categories = append([]Category{}, cats...)
	result.Categories[1].DisplayName = cats[0].DisplayName
	if ValidateResult(r, result) == nil {
		t.Fatal("accepted duplicate name")
	}
}
func TestCarryBatchRejectsDuplicates(t *testing.T) {
	r := Request{SchemaVersion: Version, RequestID: "batch", Operation: "carry", Carry: &CarryMove{SourcePath: "C:/Desktop/a.txt", DestinationName: "资料", SourcePaths: []string{"C:/Desktop/a.txt", "C:/Desktop/b.txt"}}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Carry.SourcePaths[1] = r.Carry.SourcePaths[0]
	if r.Validate() == nil {
		t.Fatal("accepted duplicate path")
	}
}
