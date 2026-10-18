package aiorganization

import (
	"strings"
	"testing"
)

// The organizer sends directories as kind=folder and the scan path is
// Path.GetExtension(name), which turns "...-v2.0.1-InternalTest-x64" into a long
// suffix. Rejecting that made every AI refinement fail before the model call.
func TestDeskBoxAcceptsFolderMetadata(t *testing.T) {
	r := Request{SchemaVersion: Version, RequestID: "folders", Operation: "classify",
		Items:      []Item{{ItemID: "f1", DisplayName: "NaiWaChatPet-Portable-v2.0.1-InternalTest-x64", Extension: ".1-internaltest-x64", Kind: "folder"}},
		Categories: []Category{{ID: "pending", DisplayName: "待整理"}}}
	if err := r.Validate(); err != nil {
		t.Fatalf("version-like folder name rejected: %v", err)
	}
	r.Items[0].Kind = "device"
	if r.Validate() == nil {
		t.Fatal("accepted unknown kind")
	}
	r.Items[0] = Item{ItemID: "f2", DisplayName: strings.Repeat("长", 200) + ".txt", Extension: ".txt", Kind: "file"}
	if err := r.Validate(); err != nil {
		t.Fatalf("long file name rejected: %v", err)
	}
	r.Items[0].Extension = ".1-internaltest-x64"
	if err := r.Validate(); err != nil {
		t.Fatalf("long suffix rejected: %v", err)
	}
	r.Items[0].Extension = ".a/b"
	if r.Validate() == nil {
		t.Fatal("accepted path separator in extension")
	}
	r.Items[0].Extension = ""
	r.Items[0].DisplayName = "a/b.txt"
	if r.Validate() == nil {
		t.Fatal("accepted path separator in display name")
	}
}
