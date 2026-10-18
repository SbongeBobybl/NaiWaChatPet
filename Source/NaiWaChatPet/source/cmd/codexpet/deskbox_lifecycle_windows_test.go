//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrganizerBundleCannotBeRedirectedToOldBuild(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "NaiWaChatPet.exe")
	override := filepath.Join(root, "old", "DeskBox.exe")
	if got := deskBoxExecutable(exe, override); got != override {
		t.Fatalf("standalone development override: %s", got)
	}
	if err := os.Mkdir(filepath.Join(root, "Organizer"), 0700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Organizer", "DeskBox.exe")
	if got := deskBoxExecutable(exe, override); got != want {
		t.Fatalf("bundle redirected to %s; want %s", got, want)
	}
	if err := os.WriteFile(want, []byte("test executable placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := deskBoxExecutable(exe, override); got != want {
		t.Fatalf("complete bundle redirected to %s", got)
	}
}
