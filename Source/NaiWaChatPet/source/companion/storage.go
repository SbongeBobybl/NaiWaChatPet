package companion

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type savedMemory struct {
	ProactiveReplies []string  `json:"proactiveReplies,omitempty"`
	Version          int       `json:"version"`
	Provider         string    `json:"provider"`
	Key              string    `json:"key"`
	Messages         []Message `json:"messages"`
}

func memoryPath() (string, error) {
	p, e := prefsPath()
	return filepath.Join(filepath.Dir(p), "companion-memory.dpapi"), e
}

func boundedMemory(h []Message) []Message {
	if len(h)%2 != 0 {
		return nil
	}
	if len(h) > 200 {
		h = h[len(h)-200:]
	}
	count := 0
	for _, m := range h {
		t, ok := m.Content.(string)
		if !ok {
			return nil
		}
		count += len([]rune(t))
	}
	for count > 128000 && len(h) > 2 {
		count -= len([]rune(h[0].Content.(string))) + len([]rune(h[1].Content.(string)))
		h = h[2:]
	}
	return append([]Message(nil), h...)
}

// Called with the server lock. Screenshots and failed replies never enter this file.
func (s *Server) persistMemoryLocked() {
	if s.demo || s.persistenceDisabled {
		return
	}
	p, e := memoryPath()
	if e == nil {
		var plain []byte
		plain, e = json.Marshal(savedMemory{Version: 1, Provider: s.settings.BaseURL, Key: s.key, Messages: boundedMemory(s.memory), ProactiveReplies: s.proactiveReplies})
		if e == nil {
			var encrypted []byte
			encrypted, e = protectLocal(plain)
			for i := range plain {
				plain[i] = 0
			}
			if e == nil {
				e = os.MkdirAll(filepath.Dir(p), 0700)
			}
			if e == nil {
				e = os.WriteFile(p+".tmp", encrypted, 0600)
			}
			if e == nil {
				e = os.Rename(p+".tmp", p)
			}
		}
	}
	if e != nil {
		s.storageStatus = "本地加密保存失败，请检查用户目录权限；本次会话仍可使用。"
	} else {
		s.storageStatus = "API Key 与最近聊天已由 Windows 当前用户加密保存，重启自动恢复。"
	}
}

func (s *Server) restoreMemory() {
	p, e := memoryPath()
	if e != nil {
		return
	}
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		s.storageStatus = "填写一次 API Key 后会加密保存；成功聊天会自动记忆。"
		return
	}
	if e != nil || len(b) > 1<<20 {
		s.storageStatus = "无法读取本地记忆，请重新配置。"
		return
	}
	plain, e := unprotectLocal(b)
	if e != nil {
		s.storageStatus = "当前 Windows 用户无法解密旧记忆，请重新配置。"
		return
	}
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()
	var v savedMemory
	if json.Unmarshal(plain, &v) != nil || v.Version != 1 || v.Provider != s.settings.BaseURL || len(v.Messages) > 200 {
		s.storageStatus = "本地记忆与当前服务不匹配，请重新配置。"
		return
	}
	if v.Key != "" && (len(v.Key) < 8 || len(v.Key) > 512 || strings.ContainsAny(v.Key, "\r\n\t ")) {
		return
	}
	for i, m := range v.Messages {
		t, ok := m.Content.(string)
		expected := "user"
		if i%2 == 1 {
			expected = "assistant"
		}
		if !ok || m.Role != expected || strings.TrimSpace(t) == "" || len(t) > 1<<18 {
			return
		}
	}
	s.key = v.Key
	if len(v.ProactiveReplies) <= 5 {
		for _, text := range v.ProactiveReplies {
			if len([]rune(text)) <= 4000 {
				s.proactiveReplies = append(s.proactiveReplies, text)
			}
		}
	}
	s.memory = boundedMemory(v.Messages)
	s.history = trimHistory(append([]Message(nil), s.memory...))
	s.storageStatus = "已从本地加密文件恢复 API Key 和聊天记忆。"
}

var errLocalProtection = errors.New("Windows user encryption unavailable")
