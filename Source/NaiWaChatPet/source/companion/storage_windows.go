//go:build windows

package companion

import (
	"runtime"
	"syscall"
	"unsafe"
)

type secretBlob struct {
	Size uint32
	Data *byte
}

var localCrypto = syscall.NewLazyDLL("crypt32.dll")
var localProtect = localCrypto.NewProc("CryptProtectData")
var localUnprotect = localCrypto.NewProc("CryptUnprotectData")
var localFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
var localCopy = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")

func cryptLocal(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, errLocalProtection
	}
	in := secretBlob{Size: uint32(len(data)), Data: &data[0]}
	var out secretBlob
	proc := localProtect
	if decrypt {
		proc = localUnprotect
	}
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out))) // UI forbidden; user scope, never machine scope.
	runtime.KeepAlive(data)
	if r == 0 {
		return nil, errLocalProtection
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.Data)))
	if out.Size == 0 || out.Size > 1<<20 {
		return nil, errLocalProtection
	}
	result := make([]byte, int(out.Size))
	localCopy.Call(uintptr(unsafe.Pointer(&result[0])), uintptr(unsafe.Pointer(out.Data)), uintptr(out.Size))
	runtime.KeepAlive(out)
	runtime.KeepAlive(result)
	return result, nil
}
func protectLocal(b []byte) ([]byte, error)   { return cryptLocal(b, false) }
func unprotectLocal(b []byte) ([]byte, error) { return cryptLocal(b, true) }
