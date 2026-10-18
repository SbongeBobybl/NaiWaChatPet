package tidy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicatesRequireMatchingStableContent(t *testing.T) {
	root := desk(t, "a.pdf", "b.pdf", "c.pdf")
	os.WriteFile(filepath.Join(root, "a.pdf"), []byte("same"), 0600)
	os.WriteFile(filepath.Join(root, "b.pdf"), []byte("same"), 0600)
	os.WriteFile(filepath.Join(root, "c.pdf"), []byte("diff"), 0600)
	p, _ := Scan(root, later)
	r, e := FindDuplicates(context.Background(), p)
	if e != nil || len(r.Groups) != 1 || len(r.Groups[0]) != 2 {
		t.Fatalf("%+v %v", r, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = FindDuplicates(ctx, p); e == nil {
		t.Fatal("cancel ignored")
	}
}
