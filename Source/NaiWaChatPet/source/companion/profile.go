package companion

import (
	"os"
	"path/filepath"
)

const ProductName = "NaiWaChatPet"
const ReleaseProfile = "2.0.1"

// Every release starts from its own factory profile. Same-version restarts
// preserve locally encrypted credentials and memory. Never import old profiles.
func ProfileDirectory() (string, error) {
	dir, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(dir, ProductName, "versions", ReleaseProfile), nil
}

func factorySettings() Settings {
	on := true
	p := DefaultPersonality()
	return Settings{Persona: DefaultPersona, Length: "short", ProactiveMinutes: 25, ProactiveSeconds: 30, ScreenMinutes: 15,
		ModelChoice: "flash", BaseURL: "https://api.deepseek.com", Streaming: &on, EmotionEnabled: &on,
		MaxReminders: 120, QuietStart: 23, QuietEnd: 8, LeaseMinutes: 30, Greeting: true, Breaks: true, Encourage: true,
		ScreenScope: "monitor:primary", ObservationMode: "off", Personality: &p}
}
