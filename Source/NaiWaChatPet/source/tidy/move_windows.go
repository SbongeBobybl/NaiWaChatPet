//go:build windows

package tidy

import (
	"syscall"
	"unsafe"
)

var moveFile = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

// move renames without ever replacing: os.Rename on Windows would overwrite
// a file that appeared at the destination after it was checked.
func move(from, to string) error {
	src, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// No flags: fail if the target exists, and stay on the same volume.
	if ok, _, e := moveFile.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(dst)), 0); ok == 0 {
		return &linkError{from, to, e}
	}
	return nil
}

type linkError struct {
	from, to string
	err      error
}

func (e *linkError) Error() string { return "move " + e.from + " to " + e.to + ": " + e.err.Error() }

// Unwrap lets errors.Is see a missing source as fs.ErrNotExist.
func (e *linkError) Unwrap() error { return e.err }
