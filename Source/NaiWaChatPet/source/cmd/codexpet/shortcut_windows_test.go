//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShortcutUsesCurrentExeAndNativeFrogIcon(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "NaiWaChatPet.lnk")
	if e = writeShortcut(exe, path); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(path)
	if e != nil || len(b) < 76 || b[0] != 76 {
		t.Fatal("shortcut not created as a real Shell Link")
	}
}
