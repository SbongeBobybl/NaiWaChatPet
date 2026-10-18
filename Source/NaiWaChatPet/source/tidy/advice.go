package tidy

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

type Hint struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Ext  string `json:"ext"`
	Age  string `json:"age_band"`
}
type Suggestion struct {
	ID         string  `json:"id"`
	Group      string  `json:"group"`
	Category   string  `json:"category"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// Only credential-shaped metadata is withheld from the model. Long digit runs
// are common in dates and generated ids, so the numeric rule starts at 15
// digits (national id and bank card lengths) and leaves dates/timestamps alone.
var sensitive = regexp.MustCompile(`(?i)(secret|password|token|api.?key|密码|密钥|身份证|账户|银行|[0-9]{15,}|@)`)

func Sensitive(name string) bool { return sensitive.MatchString(name) }
func ParseAdvice(text string, hints []Hint) ([]Suggestion, error) {
	var response struct {
		Suggestions []Suggestion `json:"suggestions"`
	}
	if len(text) > 64<<10 {
		return nil, errors.New("AI 响应过大")
	}
	d := json.NewDecoder(strings.NewReader(text))
	d.DisallowUnknownFields()
	if e := d.Decode(&response); e != nil {
		return nil, errors.New("AI 返回格式不符合整理协议")
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("AI 返回了额外数据")
	}
	allowed := map[string]bool{}
	for _, h := range hints {
		allowed[h.ID] = true
	}
	seen := map[string]bool{}
	for _, s := range response.Suggestions {
		if !allowed[s.ID] || seen[s.ID] || !SafeLabel(s.Group) || s.Confidence < 0 || s.Confidence > 1 || len([]rune(s.Reason)) > 160 || !strings.Contains("|project|document|spreadsheet|presentation|image|video|audio|archive|code|pending|apps|projects|temp|references|other|", "|"+s.Category+"|") {
			return nil, errors.New("AI 返回未知 ID、类别或不安全名称")
		}
		seen[s.ID] = true
	}
	return response.Suggestions, nil
}
