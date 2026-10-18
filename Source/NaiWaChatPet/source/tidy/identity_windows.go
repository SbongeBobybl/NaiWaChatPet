//go:build windows

package tidy

import (
	"errors"
	"os"
	"syscall"
)

func unsafeAttributes(path string) bool {
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return true
	}
	a, e := syscall.GetFileAttributes(p)
	return e != nil || a&(0x400|0x1000|0x40000|0x400000) != 0
}
func identify(path string) (Identity, error) {
	i, e := os.Lstat(path)
	if e != nil {
		return Identity{}, e
	}
	if !i.Mode().IsRegular() || unsafeAttributes(path) {
		return Identity{}, errors.New("链接或云端占位文件受保护")
	}
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return Identity{}, e
	}
	a, e := syscall.GetFileAttributes(p)
	if e != nil || a&7 != 0 {
		return Identity{}, errors.New("隐藏、系统或只读文件受保护")
	}
	h, e := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return Identity{}, e
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if e = syscall.GetFileInformationByHandle(h, &info); e != nil {
		return Identity{}, e
	}
	return Identity{int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow), info.LastWriteTime.Nanoseconds(), info.VolumeSerialNumber, uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}, nil
}
