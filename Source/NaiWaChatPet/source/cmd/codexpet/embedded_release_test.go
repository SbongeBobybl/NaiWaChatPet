package main

import "testing"

// Regression test: changing any released PNG without synchronizing the
// runtime.json SHA-256 manifest must fail the build's test gate.
func TestEmbeddedReleaseAssetsIntegrity(t *testing.T) {
	if bundledAssets == nil {
		t.Fatal("embedded asset bundle is unavailable")
	}
	loaded, err := loadAssets(bundledAssets, "runtime.json")
	if err != nil {
		t.Fatalf("embedded asset integrity check failed: %v", err)
	}
	if len(loaded.Counts) != 14 || loaded.Canvas.Dx() != 420 || loaded.Canvas.Dy() != 560 {
		t.Fatalf("unexpected release inventory: %v canvas %v", loaded.Counts, loaded.Canvas)
	}
}
