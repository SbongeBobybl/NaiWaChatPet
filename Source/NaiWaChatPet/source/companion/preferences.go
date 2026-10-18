package companion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Preferences intentionally exclude API Key, conversation text, screenshots,
// and auto-observation grants. Same-version text companionship preferences persist.
type Preferences struct {
	ProactiveSeconds int          `json:"proactiveSeconds"`
	Proactive        bool         `json:"proactive"`
	Personality      *Personality `json:"personality,omitempty"`
	ExcludedApps     string       `json:"excludedApps"`
	ActionsOnly      bool         `json:"actionsOnly"`
	MaxReminders     int          `json:"maxReminders"`
	QuietStart       int          `json:"quietStart"`
	QuietEnd         int          `json:"quietEnd"`
	LeaseMinutes     int          `json:"leaseMinutes"`
	Greeting         bool         `json:"greeting"`
	Breaks           bool         `json:"breaks"`
	Encourage        bool         `json:"encourage"`
	IdleActions      bool         `json:"idleActions"`
	EmotionEnabled   *bool        `json:"emotionEnabled,omitempty"`
	ModelChoice      string       `json:"modelChoice"`
	ModelID          string       `json:"modelId"`
	BaseURL          string       `json:"baseUrl"`
	Thinking         bool         `json:"thinking"`
	Streaming        *bool        `json:"streaming,omitempty"`
	CustomVision     bool         `json:"customVision"`
	CustomThinking   bool         `json:"customThinking"`
	Persona          string       `json:"persona"`
	Length           string       `json:"length"`
	ProactiveMinutes int          `json:"proactiveMinutes"`
	ScreenMinutes    int          `json:"screenMinutes"`
}

func prefsPath() (string, error) {
	dir, e := ProfileDirectory()
	if e != nil {
		return "", e
	}
	return filepath.Join(dir, "companion-preferences.json"), nil
}
func readPreferences() Preferences {
	path, e := prefsPath()
	if e != nil {
		return Preferences{}
	}
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 8192 {
		return Preferences{}
	}
	var p Preferences
	if json.Unmarshal(b, &p) != nil {
		return Preferences{}
	}
	if len([]rune(p.Persona)) > 1500 {
		p.Persona = ""
	}
	if p.Length != "short" && p.Length != "normal" && p.Length != "long" && p.Length != "one" {
		p.Length = "short"
	}
	if p.ProactiveMinutes < 10 || p.ProactiveMinutes > 180 {
		p.ProactiveMinutes = 25
	}
	if p.ScreenMinutes < 10 || p.ScreenMinutes > 60 {
		p.ScreenMinutes = 15
	}
	return p
}
func writePreferences(s Settings) error {
	path, err := prefsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	p := Preferences{Persona: strings.TrimSpace(s.Persona), Length: s.Length, ProactiveMinutes: s.ProactiveMinutes, ScreenMinutes: s.ScreenMinutes, ModelChoice: s.ModelChoice, ModelID: s.ModelID, BaseURL: s.BaseURL, Thinking: s.Thinking, Streaming: s.Streaming, CustomVision: s.CustomVision, CustomThinking: s.CustomThinking}
	p.MaxReminders = s.MaxReminders
	p.ProactiveSeconds = s.ProactiveSeconds
	p.Proactive = s.Proactive
	p.QuietStart = s.QuietStart
	p.QuietEnd = s.QuietEnd
	p.LeaseMinutes = s.LeaseMinutes
	p.Greeting = s.Greeting
	p.Breaks = s.Breaks
	p.Encourage = s.Encourage
	p.IdleActions = s.IdleActions
	p.EmotionEnabled = s.EmotionEnabled
	p.Personality = s.Personality
	p.ExcludedApps = s.ExcludedApps
	p.ActionsOnly = s.ActionsOnly
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err = os.WriteFile(temp, b, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
