package main

import (
	"bytes"
	"image"
	"testing"
)

func TestWholeButtonMovesAndRebounds(t *testing.T) {
	a, _ := buttonFace(nil, image.Point{}, 100, 36, 1, 0, true, false, false)
	b, _ := buttonFace(nil, image.Point{}, 100, 36, 1, 1, true, false, false)
	rebound, _ := buttonFace(nil, image.Point{}, 100, 36, 1, 0, true, false, false)
	changed := 0
	for i := 0; i < len(a); i += 4 {
		if !bytes.Equal(a[i:i+3], b[i:i+3]) {
			changed++
		}
	}
	if changed < 1800 {
		t.Fatalf("only caption/small area moves: %d", changed)
	}
	if !bytes.Equal(a, rebound) {
		t.Fatal("button does not rebound to its original face")
	}
}
