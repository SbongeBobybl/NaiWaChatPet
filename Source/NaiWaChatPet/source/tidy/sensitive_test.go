package tidy

import "testing"

func TestSensitiveNamesKeepCredentialFocus(t *testing.T) {
	for _, name := range []string{
		"GHOS-20261011-Windows-x64.zip",
		"gemini-code-1791718910974.srt",
		"2025 年度总结.pptx",
	} {
		if Sensitive(name) {
			t.Fatalf("date or generated id treated as secret: %s", name)
		}
	}
	for _, name := range []string{
		"我的密码.txt",
		"api-key-backup.txt",
		"身份证照片.jpg",
		"card-1234567890123456.txt",
		"someone@example.com.txt",
	} {
		if !Sensitive(name) {
			t.Fatalf("credential-shaped name not withheld: %s", name)
		}
	}
}
