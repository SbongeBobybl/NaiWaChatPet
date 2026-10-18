//go:build windows

package safestore

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

const (
	deleteAccess        = 0x00010000
	openReparsePoint    = 0x00200000
	reparseAttribute    = 0x00000400
	directoryAttribute  = 0x00000010
	encryptedAttribute  = 0x00004000
	fileDispositionInfo = 4
	fileStreamInfo      = 7
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var setFileInformationByHandle = kernel32.NewProc("SetFileInformationByHandle")
var getDriveType = kernel32.NewProc("GetDriveTypeW")
var getFileInformationByHandleEx = kernel32.NewProc("GetFileInformationByHandleEx")
var copyFileW = kernel32.NewProc("CopyFileW")

func validateInputPath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: empty path", ErrUnsafePath)
	}
	volume := filepath.VolumeName(path)
	if volume != "" && (len(volume) != 2 || volume[1] != ':') {
		return fmt.Errorf("%w: only drive-letter local paths are supported", ErrUnsafePath)
	}
	rest := strings.TrimPrefix(path, volume)
	if strings.Contains(rest, ":") {
		return fmt.Errorf("%w: alternate data stream", ErrUnsafePath)
	}
	for _, part := range strings.Split(strings.ReplaceAll(rest, "/", "\\"), "\\") {
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			continue
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return fmt.Errorf("%w: ambiguous trailing dot or space", ErrUnsafePath)
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if isReservedName(stem) {
			return fmt.Errorf("%w: reserved Windows device name", ErrUnsafePath)
		}
	}
	return nil
}

func isReservedName(stem string) bool {
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CLOCK$" || stem == "CONIN$" || stem == "CONOUT$" {
		return true
	}
	for _, prefix := range []string{"COM", "LPT"} {
		if !strings.HasPrefix(stem, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(stem, prefix)
		if len(suffix) == 1 && suffix[0] >= '1' && suffix[0] <= '9' {
			return true
		}
		if suffix == "\u00b9" || suffix == "\u00b2" || suffix == "\u00b3" {
			return true
		}
	}
	return false
}

func validatePath(path string) error {
	if err := validateInputPath(path); err != nil {
		return err
	}
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(path) {
		return fmt.Errorf("%w: only absolute drive-letter local paths are supported", ErrUnsafePath)
	}
	root, err := syscall.UTF16PtrFromString(volume + "\\")
	if err != nil {
		return err
	}
	typeID, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(root)))
	if typeID != 3 {
		return fmt.Errorf("%w: only fixed local disks are supported", ErrUnsafePath)
	}
	// All parent directories must exist and be ordinary directories. Inspecting
	// parents is not an atomic filesystem transaction: callers must ensure these
	// directories are trusted and are not being replaced concurrently.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		p, err := syscall.UTF16PtrFromString(parent)
		if err != nil {
			return err
		}
		attributes, err := syscall.GetFileAttributes(p)
		if err != nil {
			return fmt.Errorf("parent %q: %w", parent, err)
		}
		if attributes&reparseAttribute != 0 || attributes&directoryAttribute == 0 {
			return fmt.Errorf("%w: parent %q is not an ordinary directory", ErrUnsafePath, parent)
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	return nil
}

func openSource(path string) (*os.File, error) {
	return openRegular(path, syscall.GENERIC_READ|deleteAccess, syscall.OPEN_EXISTING)
}
func createTarget(path string) (*os.File, error) {
	return openRegular(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE|deleteAccess, syscall.CREATE_NEW)
}
func openRegular(path string, access, creation uint32) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Share mode zero requires the file to be closed elsewhere and keeps writers,
	// renames and deletion away while copying and verifying this exact file.
	handle, err := syscall.CreateFile(p, access, 0, nil, creation, syscall.FILE_ATTRIBUTE_NORMAL|openReparsePoint, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open-regular", Path: path, Err: err}
	}
	var info syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(handle, &info); err != nil {
		if creation == syscall.CREATE_NEW {
			return os.NewFile(uintptr(handle), path), err
		}
		_ = syscall.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&(reparseAttribute|directoryAttribute) != 0 {
		if creation == syscall.CREATE_NEW {
			return os.NewFile(uintptr(handle), path), fmt.Errorf("%w: target is a reparse point or directory", ErrUnsafePath)
		}
		_ = syscall.CloseHandle(handle)
		return nil, fmt.Errorf("%w: file is a reparse point or directory", ErrUnsafePath)
	}
	if creation == syscall.OPEN_EXISTING {
		if info.FileAttributes&encryptedAttribute != 0 || info.NumberOfLinks != 1 {
			_ = syscall.CloseHandle(handle)
			return nil, fmt.Errorf("%w: encrypted file or multiple hard links", ErrUnsafePath)
		}
		if err := verifySingleDataStream(handle); err != nil {
			_ = syscall.CloseHandle(handle)
			return nil, err
		}
	}
	return os.NewFile(uintptr(handle), path), nil
}

// Inspect the source stream inventory on the locked handle. Do not delete
// originals with ADS unless all streams have been transferred and verified.
type namedStream struct {
	Name string
	Size int64
}

func streamInventory(handle syscall.Handle) ([]namedStream, error) {
	for size := 4096; size <= 1024*1024; size *= 2 {
		buffer := make([]byte, size)
		ok, _, callErr := getFileInformationByHandleEx.Call(uintptr(handle), fileStreamInfo, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		runtime.KeepAlive(buffer)
		if ok == 0 {
			if callErr == syscall.ERROR_MORE_DATA || callErr == syscall.ERROR_INSUFFICIENT_BUFFER {
				continue
			}
			return nil, fmt.Errorf("%w: cannot verify source stream inventory: %v", ErrUnsafePath, callErr)
		}
		var inventory []namedStream
		for offset := 0; ; {
			if offset+24 > len(buffer) {
				return nil, fmt.Errorf("%w: invalid source stream inventory", ErrUnsafePath)
			}
			next := int(binary.LittleEndian.Uint32(buffer[offset:]))
			nameBytes := int(binary.LittleEndian.Uint32(buffer[offset+4:]))
			if nameBytes%2 != 0 || nameBytes == 0 || nameBytes > len(buffer)-offset-24 {
				return nil, fmt.Errorf("%w: invalid source stream name", ErrUnsafePath)
			}
			name := make([]uint16, nameBytes/2)
			for i := range name {
				name[i] = binary.LittleEndian.Uint16(buffer[offset+24+i*2:])
			}
			streamName := syscall.UTF16ToString(name)
			if !strings.HasSuffix(strings.ToUpper(streamName), ":$DATA") {
				return nil, fmt.Errorf("%w: unsupported data stream type %q", ErrUnsafePath, streamName)
			}
			streamSize := int64(binary.LittleEndian.Uint64(buffer[offset+8:]))
			if streamSize < 0 {
				return nil, fmt.Errorf("%w: invalid stream size", ErrUnsafePath)
			}
			inventory = append(inventory, namedStream{Name: streamName, Size: streamSize})
			if next == 0 {
				sort.Slice(inventory, func(i, j int) bool { return inventory[i].Name < inventory[j].Name })
				return inventory, nil
			}
			if next < 24+nameBytes || next > len(buffer)-offset {
				return nil, fmt.Errorf("%w: invalid source stream offset", ErrUnsafePath)
			}
			offset += next
		}
	}
	return nil, fmt.Errorf("%w: source stream inventory exceeds inspection limit", ErrUnsafePath)
}

func verifySingleDataStream(handle syscall.Handle) error {
	streams, err := streamInventory(handle)
	if err != nil {
		return err
	}
	if len(streams) == 1 && streams[0].Name == "::$DATA" {
		return nil
	}
	return ErrNamedDataStreams
}

// openForStreamVerification accepts ordinary regular files with ADS, but
// retains the same reparse, encryption and hard-link restrictions.
func openForStreamVerification(path string) (*os.File, []namedStream, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, nil, err
	}
	handle, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL|openReparsePoint, 0)
	if err != nil {
		return nil, nil, err
	}
	var info syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(handle, &info); err != nil {
		_ = syscall.CloseHandle(handle)
		return nil, nil, err
	}
	if info.FileAttributes&(reparseAttribute|directoryAttribute|encryptedAttribute) != 0 || info.NumberOfLinks != 1 {
		_ = syscall.CloseHandle(handle)
		return nil, nil, fmt.Errorf("%w: invalid file type, encrypted or hard-linked", ErrUnsafePath)
	}
	streams, err := streamInventory(handle)
	if err != nil {
		_ = syscall.CloseHandle(handle)
		return nil, nil, err
	}
	return os.NewFile(uintptr(handle), path), streams, nil
}

func sameStreamInventory(a, b []namedStream) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// copyAllStreamsKeepingSource copies every NTFS stream with Windows CopyFileW
// (fail-if-exists) and retains the original. The default stream is verified by
// SHA-256 and ALL stream names/sizes must agree. Do not silently discard
// Zone.Identifier or arbitrary user metadata; do not delete the source.
func copyAllStreamsKeepingSource(ctx context.Context, result Result) (Result, error) {
	fail := func(stage string, cause error) error {
		return &TransferError{Stage: stage, Cause: cause, Result: result}
	}
	if err := ctx.Err(); err != nil {
		return result, fail("cancelled", err)
	}
	psrc, err := syscall.UTF16PtrFromString(result.Source)
	if err != nil {
		return result, fail("source-path", err)
	}
	pdst, err := syscall.UTF16PtrFromString(result.Target)
	if err != nil {
		return result, fail("target-path", err)
	}

	// FailIfExists guarantees no existing target is overwritten. Windows
	// CopyFileW copies the named streams, unlike io.Copy(default stream).
	ok, _, copyErr := copyFileW.Call(uintptr(unsafe.Pointer(psrc)), uintptr(unsafe.Pointer(pdst)), 1)
	if ok == 0 {
		if copyErr == syscall.Errno(0) {
			copyErr = syscall.EINVAL
		}
		// On failure, CopyFileW might leave a partial destination. Do not
		// delete a path unless its ownership can be established conclusively.
		return result, fail("copy-with-streams", copyErr)
	}
	result.TargetCreated, result.TargetExists = true, true
	var src, dst *os.File
	closeHandles := func() {
		if dst != nil {
			_ = dst.Close()
			dst = nil
		}
		if src != nil {
			_ = src.Close()
			src = nil
		}
	}
	defer closeHandles()
	reject := func(stage string, cause error) (Result, error) {
		closeHandles()
		// We own this target because the fail-if-exists CopyFileW succeeded.
		// Leave the source untouched even if verification or cleanup fails.
		cleanupErr := os.Remove(result.Target)
		if cleanupErr == nil {
			result.TargetExists = false
		}
		te := &TransferError{Stage: stage, Cause: cause, CleanupError: cleanupErr, Result: result}
		return result, te
	}
	if err := ctx.Err(); err != nil {
		return reject("cancelled", err)
	}
	var srcStreams, dstStreams []namedStream
	src, srcStreams, err = openForStreamVerification(result.Source)
	if err != nil {
		return reject("verify-source-open", err)
	}
	dst, dstStreams, err = openForStreamVerification(result.Target)
	if err != nil {
		return reject("verify-target-open", err)
	}
	if !sameStreamInventory(srcStreams, dstStreams) {
		return reject("verify-streams", ErrVerification)
	}
	sizeSrc, hashSrc, err := digest(ctx, src)
	if err != nil {
		return reject("verify-source", err)
	}
	sizeDst, hashDst, err := digest(ctx, dst)
	if err != nil {
		return reject("verify-target", err)
	}
	if sizeSrc != sizeDst || hashSrc != hashDst {
		return reject("verify-content", ErrVerification)
	}
	result.BytesCopied, result.SHA256 = sizeDst, hashDst
	result.TargetVerified = true
	result.SourceRetainedForStreams = true
	// Deliberately retain source files containing any named streams.
	return result, nil
}

func deleteByHandle(file *os.File) error {
	var deleteFile byte = 1
	ok, _, callErr := setFileInformationByHandle.Call(file.Fd(), fileDispositionInfo, uintptr(unsafe.Pointer(&deleteFile)), unsafe.Sizeof(deleteFile))
	if ok == 0 {
		if callErr == syscall.Errno(0) {
			callErr = syscall.EINVAL
		}
		return &os.PathError{Op: "delete-by-handle", Path: file.Name(), Err: callErr}
	}
	return nil
}
