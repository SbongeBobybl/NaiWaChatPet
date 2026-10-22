package companion

import (
    "os"
    "path/filepath"
    "testing"
)

func TestReleaseProfileDoesNotImportLegacyPrivateState(t *testing.T) {
    root := t.TempDir()
    t.Setenv("APPDATA", root)
    legacy := filepath.Join(root, ProductName, "versions", "13.4.0")
    if err := os.MkdirAll(legacy, 0700); err != nil { t.Fatal(err) }
    for _, name := range []string{"companion-preferences.json", "companion-memory.dpapi", "play.json"} {
        if err := os.WriteFile(filepath.Join(legacy,name), []byte(`{"persona":"legacy-private-fixture"}`),0600); err != nil { t.Fatal(err) }
    }
    current, err := ProfileDirectory()
    if err != nil { t.Fatal(err) }
    if current == legacy || filepath.Base(current) != "2.1.0" { t.Fatal("release reused legacy profile") }
    if p := readPreferences(); p.Persona != "" { t.Fatal("legacy preferences imported") }
    if _, err := os.Stat(current); !os.IsNotExist(err) { t.Fatal("profile reads must not import files") }
    if _, err := os.Stat(filepath.Join(legacy,"companion-memory.dpapi")); err != nil { t.Fatal("legacy data was deleted") }
}
