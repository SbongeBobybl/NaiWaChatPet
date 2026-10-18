// Package safestore transfers a closed, regular file between fixed local Windows
// disks without overwriting an existing destination. This is a reconstruction
// component, not recovered Codex Pet V10 source.
package safestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	ErrUnsafePath       = errors.New("path is outside the supported regular-file scope")
	ErrUnsupported      = errors.New("safe source deletion is supported only on Windows")
	ErrVerification     = errors.New("copied file failed independent verification")
	ErrNamedDataStreams = errors.New("source has named data streams")
)

// Result describes the actual outcome, including a completed destination when
// deleting the source fails. TargetExists concerns only a destination created by
// this operation; a pre-existing conflicting destination is never owned by it.
type Result struct {
	Source                 string `json:"source"`
	Target                 string `json:"target"`
	BytesCopied            int64  `json:"bytesCopied"`
	SHA256                 string `json:"sha256,omitempty"`
	TargetCreated          bool   `json:"targetCreated"`
	TargetExists           bool   `json:"targetExists"`
	TargetVerified         bool   `json:"targetVerified"`
	SourceRemoved          bool   `json:"sourceRemoved"`
	SourceRemovalUncertain bool   `json:"sourceRemovalUncertain,omitempty"`
	// SourceRetainedForStreams means Windows copied a multi-stream file without deleting the original.
	SourceRetainedForStreams bool `json:"sourceRetainedForStreams,omitempty"`
	// CloseWarning records a handle-close problem after the data and source
	// deletion already succeeded. It does not invalidate a completed transfer.
	CloseWarning string `json:"closeWarning,omitempty"`
}

// TransferError preserves both the failing stage and the resulting file state.
// On a delete-source error, TargetVerified is true and the two copies remain.
// CleanupError indicates that removal of an unverified partial target failed.
type TransferError struct {
	Stage        string
	Cause        error
	CleanupError error
	Result       Result
}

func (e *TransferError) Error() string {
	s := fmt.Sprintf("safe transfer failed at %s: %v (target verified=%t, target exists=%t, source removed=%t, source removal uncertain=%t)", e.Stage, e.Cause, e.Result.TargetVerified, e.Result.TargetExists, e.Result.SourceRemoved, e.Result.SourceRemovalUncertain)
	if e.CleanupError != nil {
		s += "; partial target cleanup: " + e.CleanupError.Error()
	}
	return s
}

func (e *TransferError) Unwrap() error { return e.Cause }

// Transfer always copies, including when both paths are on the same disk. It
// flushes the target, then independently rereads both handles and compares their
// lengths and SHA-256 hashes before deleting the original source via its handle.
// Source and target must be regular files under trusted existing parent
// directories. Symlinks, reparse points, directories, device paths, alternate data
// streams, network paths, and files already open elsewhere are rejected.
//
// It never creates directories and never overwrites a conflicting target. Every
// failure before source deletion preserves the source. Unverified partial targets
// are removed best-effort; verified targets are retained if source deletion fails.
// Cancellation is checked immediately before deletion; cancellation after that
// final check cannot roll back a completed transfer.
func Transfer(ctx context.Context, source, target string) (result Result, err error) {
	if ctx == nil {
		return result, &TransferError{Stage: "context", Cause: errors.New("nil context"), Result: result}
	}
	// Windows absolute-path resolution can strip trailing dots/spaces. Reject
	// ambiguous user input before resolving it, rather than validating its alias.
	if err = validateInputPath(source); err != nil {
		return result, &TransferError{Stage: "source-path", Cause: err, Result: result}
	}
	if err = validateInputPath(target); err != nil {
		return result, &TransferError{Stage: "target-path", Cause: err, Result: result}
	}
	result.Source, err = filepath.Abs(source)
	if err != nil {
		return result, &TransferError{Stage: "source-path", Cause: err, Result: result}
	}
	result.Target, err = filepath.Abs(target)
	if err != nil {
		return result, &TransferError{Stage: "target-path", Cause: err, Result: result}
	}
	fail := func(stage string, cause error) error {
		return &TransferError{Stage: stage, Cause: cause, Result: result}
	}
	if err = ctx.Err(); err != nil {
		return result, fail("cancelled", err)
	}
	if err = validatePath(result.Source); err != nil {
		return result, fail("source-path", err)
	}
	if err = validatePath(result.Target); err != nil {
		return result, fail("target-path", err)
	}

	src, err := openSource(result.Source)
	if errors.Is(err, ErrNamedDataStreams) {
		// A file with ADS cannot safely be moved by copying only its unnamed stream.
		// Windows CopyFileW preserves all streams; keep the original rather than
		// risk metadata loss, and verify stream names/sizes after the copy.
		return copyAllStreamsKeepingSource(ctx, result)
	}
	if err != nil {
		return result, fail("open-source", err)
	}
	defer func() {
		if src != nil {
			_ = src.Close()
		}
	}()
	before, err := src.Stat()
	if err != nil {
		return result, fail("stat-source", err)
	}
	if !before.Mode().IsRegular() {
		return result, fail("source-type", ErrUnsafePath)
	}
	dst, err := createTarget(result.Target)
	if dst == nil {
		return result, fail("create-target", err)
	}
	result.TargetCreated, result.TargetExists = true, true
	defer func() {
		var cleanupErr error
		deletingPartial := false
		if err != nil && !result.TargetVerified {
			if e := deleteByHandle(dst); e != nil {
				cleanupErr = e
			} else {
				deletingPartial = true
			}
		}
		if e := dst.Close(); e != nil {
			if err != nil {
				cleanupErr = errors.Join(cleanupErr, e)
			} else {
				result.CloseWarning = e.Error()
			}
		} else if deletingPartial {
			result.TargetExists = false
		}
		if transferErr, ok := err.(*TransferError); ok {
			transferErr.CleanupError = cleanupErr
			transferErr.Result = result
		}
	}()
	if err != nil {
		return result, fail("target-type", err)
	}

	copyHash := sha256.New()
	result.BytesCopied, err = io.CopyBuffer(io.MultiWriter(dst, copyHash), &contextReader{ctx: ctx, reader: src}, make([]byte, 256*1024))
	if err != nil {
		return result, fail("copy", err)
	}
	if result.BytesCopied != before.Size() {
		return result, fail("copy-length", ErrVerification)
	}
	if err = dst.Sync(); err != nil {
		return result, fail("flush-target", err)
	}
	if err = ctx.Err(); err != nil {
		return result, fail("cancelled", err)
	}
	sourceSize, sourceHash, err := digest(ctx, src)
	if err != nil {
		return result, fail("verify-source", err)
	}
	targetSize, targetHash, err := digest(ctx, dst)
	if err != nil {
		return result, fail("verify-target", err)
	}
	if sourceSize != before.Size() || targetSize != sourceSize || sourceHash != targetHash || sourceHash != hex.EncodeToString(copyHash.Sum(nil)) {
		return result, fail("verify-content", ErrVerification)
	}
	result.SHA256, result.TargetVerified = targetHash, true
	if err = ctx.Err(); err != nil {
		return result, fail("cancelled-before-delete", err)
	}
	if err = deleteByHandle(src); err != nil {
		return result, fail("delete-source", err)
	}
	// The exclusive handle prevents other handles from keeping the original
	// pending deletion. A successful Close completes deletion of that source.
	if err = src.Close(); err != nil {
		result.SourceRemovalUncertain = true
		src = nil
		return result, fail("close-deleting-source", err)
	}
	src = nil
	result.SourceRemoved = true
	return result, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func digest(ctx context.Context, file *os.File) (int64, string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.CopyBuffer(h, &contextReader{ctx: ctx, reader: file}, make([]byte, 256*1024))
	if err != nil {
		return n, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
