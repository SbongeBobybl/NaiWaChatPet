//go:build windows

package tidy

import (
	"errors"
	"path/filepath"
	"syscall"
)

func directoryLock(dir string) (func(), error) {
	p, e := syscall.UTF16PtrFromString(filepath.Join(dir, ".naiwa-transaction.lock"))
	if e != nil {
		return nil, e
	}
	h, e := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE|0x10000, 0, nil, syscall.OPEN_ALWAYS, 0x04000000|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, errors.New("另一个进程正在整理，或日志目录不可写")
	}
	return func() { syscall.CloseHandle(h) }, nil
}
