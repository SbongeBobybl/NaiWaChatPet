package companion

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type ModelProfile struct {
	Choice, Name, ID, BaseURL, Endpoint     string
	Vision, ThinkingExtension, VerifiedSpec bool
}

func ModelFromSettings(s Settings) (ModelProfile, error) {
	base := strings.TrimSpace(s.BaseURL)
	if base == "" {
		base = "https://api.deepseek.com"
	}
	u, e := url.Parse(base)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ModelProfile{}, errors.New("模型服务地址必须是有效 Base URL，不能含凭据、查询或片段。")
	}
	host := u.Hostname()
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return ModelProfile{}, errors.New("远程模型服务必须使用 HTTPS；HTTP 仅限本机模型服务。")
	}
	if strings.Contains(u.Path, "..") || strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/chat/completions") {
		return ModelProfile{}, errors.New("请填写 Base URL，不要填写完整 chat/completions 路径。")
	}
	base = strings.TrimRight(base, "/")
	p := ModelProfile{Choice: s.ModelChoice, BaseURL: base, Endpoint: base + "/chat/completions"}
	if p.Choice == "" {
		p.Choice = "flash"
	}
	switch p.Choice {
	case "flash":
		p.Name = "DeepSeek V4.1 Flash"
		p.ID = "deepseek-flash"
		p.Vision = true
		p.ThinkingExtension = true
		p.VerifiedSpec = true
	case "pro":
		p.Name = "DeepSeek V4 Pro"
		p.ID = "deepseek-v4-pro"
		p.ThinkingExtension = true
		p.VerifiedSpec = true
	case "custom":
		p.Name = "自定义兼容模型"
		p.ID = strings.TrimSpace(s.ModelID)
		p.Vision = s.CustomVision
		p.ThinkingExtension = s.CustomThinking
	default:
		return p, errors.New("未知模型选项。")
	}
	if !modelPattern.MatchString(p.ID) {
		return p, errors.New("请填写有效的真实 API 模型 ID。")
	}
	if s.Thinking && !p.ThinkingExtension {
		return p, errors.New("这个自定义模型尚未确认支持 thinking 参数，请使用非思考模式或确认服务商规范。")
	}
	return p, nil
}
func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	j, e := s.begin(r.Context(), chatBody{Text: "请用一句中文确认连接成功。"})
	if e != nil {
		jsonErr(w, e.Error(), 400)
		return
	}
	j.diagnostic = true
	_, e = s.run(j, nil)
	if e != nil {
		jsonErr(w, e.Error(), 502)
		return
	}
	s.mu.Lock()
	s.textVerified = j.model
	demo := s.demo
	s.mu.Unlock()
	if demo {
		jsonOut(w, map[string]any{"ok": true, "modelId": j.model, "message": "离线演示测试通过；未连接真实模型服务。"})
		return
	}
	jsonOut(w, map[string]any{"ok": true, "modelId": j.model, "message": "文本连接测试通过。图片能力按模型规范或用户确认的能力设置管理；上传前仍需要截图预览确认。"})
}
