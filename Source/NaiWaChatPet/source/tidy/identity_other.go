//go:build !windows

package tidy

import (
	"errors"
	"os"
)

func unsafeAttributes(path string) bool { return false }
func identify(path string) (Identity, error) {
	i, e := os.Lstat(path)
	if e != nil {
		return Identity{}, e
	}
	if !i.Mode().IsRegular() {
		return Identity{}, errors.New("not a regular file")
	}
	return Identity{Size: i.Size(), Modified: i.ModTime().UnixNano()}, nil
}
