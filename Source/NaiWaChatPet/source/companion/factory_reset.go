package companion

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) factoryReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirmed bool `json:"confirmed"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || !body.Confirmed {
		jsonErr(w, "请先确认恢复出厂设置。", 400)
		return
	}
	if e := s.RestoreFactory(); e != nil {
		jsonErr(w, e.Error(), 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *Server) RestoreFactory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	if s.activeCancel != nil {
		s.activeCancel()
	}
	s.stopScreenLocked("factory reset")
	s.settings = factorySettings()
	s.key = ""
	s.model = "deepseek-flash"
	s.client.Endpoint = deepSeekEndpoint
	s.history = nil
	s.memory = nil
	s.proactiveReplies = nil
	s.activeProactive = false
	s.events = nil
	s.preview = nil
	s.observationSummary = ""
	s.lastText = ""
	s.lastWasScreen = false
	s.busy = false
	s.activeID = ""
	s.activeCancel = nil
	s.activeScreen = false
	s.view = ReplyView{}
	s.localQueue = nil
	s.localAction = "neutral"
	s.localSequence++
	s.lastInteraction = time.Now()
	s.diagnostics = nil
	s.textVerified = ""
	s.scheduler.Reset(time.Now())
	s.resetEmotionLocked("factory_reset")
	// Delete only this version's known preference/memory files; never user files
	// in the pet's receive folder, unrelated profiles or other releases.
	p, e := prefsPath()
	if e == nil {
		for _, name := range []string{"companion-preferences.json", "companion-memory.dpapi", "companion-memory.dpapi.tmp"} {
			if err := os.Remove(filepath.Join(filepath.Dir(p), name)); err != nil && !os.IsNotExist(err) {
				e = err
				break
			}
		}
		for _, name := range []string{"requests.jsonl", "diagnostic.log"} {
			if err := os.Remove(filepath.Join(filepath.Dir(p), "logs", name)); err != nil && !os.IsNotExist(err) {
				e = err
				break
			}
		}
	}
	if e != nil {
		s.storageStatus = "已清空本次会话，但本地文件清除失败，请检查用户目录权限。"
		return errors.New(s.storageStatus)
	}
	s.storageStatus = "已恢复出厂设置。请重新配置 API Key。"
	return nil
}
