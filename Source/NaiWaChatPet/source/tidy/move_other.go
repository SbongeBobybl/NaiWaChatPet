//go:build !windows

package tidy

import (
	"errors"
	"io/fs"
	"os"
)

// move renames without replacing an existing file.
func move(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return &fs.PathError{Op: "move", Path: to, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(from, to)
}
