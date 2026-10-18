//go:build windows

package safestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected bytes in %s", path)
	}
}

func TestTransferCopiesVerifiesAndRemovesSource(t *testing.T) {
	for _, size := range []int{0, 1, 1024*1024 + 29} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dir := t.TempDir()
			source, target := filepath.Join(dir, "source.bin"), filepath.Join(dir, "target.bin")
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i*37 + 13)
			}
			writeTestFile(t, source, data)
			result, err := Transfer(context.Background(), source, target)
			if err != nil {
				t.Fatal(err)
			}
			if !result.TargetCreated || !result.TargetExists || !result.TargetVerified || !result.SourceRemoved || result.SourceRemovalUncertain {
				t.Fatalf("incomplete result: %+v", result)
			}
			if result.BytesCopied != int64(size) {
				t.Fatalf("copied %d bytes, want %d", result.BytesCopied, size)
			}
			sum := sha256.Sum256(data)
			if result.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("incorrect verified hash")
			}
			assertFileBytes(t, target, data)
			if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("source was not removed: %v", err)
			}
		})
	}
}

func TestExistingTargetIsNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	writeTestFile(t, source, []byte("source content"))
	writeTestFile(t, target, []byte("existing target"))
	result, err := Transfer(context.Background(), source, target)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if result.SourceRemoved || result.TargetCreated {
		t.Fatalf("conflict mutated files: %+v", result)
	}
	assertFileBytes(t, source, []byte("source content"))
	assertFileBytes(t, target, []byte("existing target"))
}

func TestConcurrentTargetConflictHasExactlyOneWinner(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	const count = 8
	sources := make([]string, count)
	payloads := make([][]byte, count)
	for i := range sources {
		sources[i] = filepath.Join(dir, fmt.Sprintf("source-%d", i))
		payloads[i] = bytes.Repeat([]byte{byte(i + 1)}, 512*1024)
		writeTestFile(t, sources[i], payloads[i])
	}
	start := make(chan struct{})
	type outcome struct {
		result Result
		err    error
		index  int
	}
	outcomes := make(chan outcome, count)
	var wg sync.WaitGroup
	for i := range sources {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := Transfer(context.Background(), sources[i], target)
			outcomes <- outcome{result, err, i}
		}(i)
	}
	close(start)
	wg.Wait()
	close(outcomes)
	winners := 0
	for outcome := range outcomes {
		if outcome.err == nil {
			winners++
			if !outcome.result.SourceRemoved || !outcome.result.TargetVerified {
				t.Fatalf("incomplete winner: %+v", outcome.result)
			}
			assertFileBytes(t, target, payloads[outcome.index])
		} else {
			if outcome.result.SourceRemoved || outcome.result.TargetCreated {
				t.Fatalf("loser mutated source or target: %+v", outcome.result)
			}
			assertFileBytes(t, sources[outcome.index], payloads[outcome.index])
		}
	}
	if winners != 1 {
		t.Fatalf("got %d successful transfers, want exactly 1", winners)
	}
}

func TestCancellationKeepsSourceAndCreatesNoTarget(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	writeTestFile(t, source, []byte("keep"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := Transfer(ctx, source, target)
	if !errors.Is(err, context.Canceled) || result.TargetCreated || result.SourceRemoved {
		t.Fatalf("unexpected cancellation outcome: %+v, %v", result, err)
	}
	assertFileBytes(t, source, []byte("keep"))
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target was created: %v", err)
	}
}

func TestSourceWithZoneIdentifierIsCopiedWithoutDeletingOriginal(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "downloaded.jpg"), filepath.Join(dir, "received.jpg")
	data := []byte("fake JPG for filesystem regression test")
	zone := []byte("[ZoneTransfer]\r\nZoneId=3\r\n")
	writeTestFile(t, src, data)
	if err := os.WriteFile(src+":Zone.Identifier", zone, 0600); err != nil {
		t.Skipf("alternate data streams are unavailable: %v", err)
	}
	result, err := Transfer(context.Background(), src, dst)
	if err != nil {
		t.Fatalf("Zone.Identifier copy failed: %+v, %v", result, err)
	}
	if !result.TargetVerified || !result.SourceRetainedForStreams || result.SourceRemoved {
		t.Fatalf("source should be retained and target verified: %+v", result)
	}
	assertFileBytes(t, src, data)
	assertFileBytes(t, dst, data)
	assertFileBytes(t, src+":Zone.Identifier", zone)
	assertFileBytes(t, dst+":Zone.Identifier", zone)
}

func TestReadonlySourceDeleteFailureRetainsVerifiedCopy(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	writeTestFile(t, source, []byte("two safe copies"))
	if err := os.Chmod(source, 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(source, 0600) })
	result, err := Transfer(context.Background(), source, target)
	var transferErr *TransferError
	if !errors.As(err, &transferErr) || transferErr.Stage != "delete-source" {
		t.Fatalf("expected delete failure, got %+v, %v", result, err)
	}
	if !result.TargetVerified || !result.TargetExists || result.SourceRemoved {
		t.Fatalf("incorrect retained-copy outcome: %+v", result)
	}
	assertFileBytes(t, source, []byte("two safe copies"))
	assertFileBytes(t, target, []byte("two safe copies"))
}

func TestRejectsDirectoryAndSpecialPaths(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if result, err := Transfer(context.Background(), dir, target); err == nil || result.TargetCreated || result.SourceRemoved {
		t.Fatalf("directory accepted: %+v, %v", result, err)
	}
	source := filepath.Join(dir, "source")
	writeTestFile(t, source, []byte("keep"))
	for _, unsafe := range []string{filepath.Join(dir, "NUL"), target + ":stream", target + ".", "\\\\server\\share\\target", "\\\\.\\NUL"} {
		result, err := Transfer(context.Background(), source, unsafe)
		if !errors.Is(err, ErrUnsafePath) || result.TargetCreated || result.SourceRemoved {
			t.Fatalf("unsafe path accepted: %q: %+v, %v", unsafe, result, err)
		}
	}
	assertFileBytes(t, source, []byte("keep"))
}

func TestRejectsSymlinkSourceAndParent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	writeTestFile(t, source, []byte("keep"))
	link := filepath.Join(dir, "source-link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("Windows symlink capability unavailable: %v", err)
	}
	if result, err := Transfer(context.Background(), link, filepath.Join(dir, "target")); err == nil || result.SourceRemoved || result.TargetCreated {
		t.Fatalf("symlink accepted: %+v, %v", result, err)
	}
	parentLink := filepath.Join(dir, "parent-link")
	if err := os.Symlink(dir, parentLink); err != nil {
		t.Fatal(err)
	}
	result, err := Transfer(context.Background(), source, filepath.Join(parentLink, "target"))
	if !errors.Is(err, ErrUnsafePath) || result.SourceRemoved || result.TargetCreated {
		t.Fatalf("symlink parent accepted: %+v, %v", result, err)
	}
	assertFileBytes(t, source, []byte("keep"))
}

func TestOpenSourceIsRejectedWithoutTouchingIt(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	writeTestFile(t, source, []byte("keep"))
	open, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	result, err := Transfer(context.Background(), source, target)
	if err == nil || result.SourceRemoved || result.TargetCreated {
		t.Fatalf("open source was accepted: %+v, %v", result, err)
	}
	assertFileBytes(t, source, []byte("keep"))
}

func TestStreamPreservingCopyAndHardLinkSafety(t *testing.T) {
	t.Run("named-stream", func(t *testing.T) {
		dir := t.TempDir()
		source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
		writeTestFile(t, source, []byte("default stream"))
		stream := source + ":metadata"
		if err := os.WriteFile(stream, []byte("must not be discarded"), 0600); err != nil {
			t.Skipf("filesystem does not support named streams: %v", err)
		}
		result, err := Transfer(context.Background(), source, target)
		if err != nil {
			t.Fatalf("stream-preserving copy failed: %+v, %v", result, err)
		}
		if !result.TargetVerified || !result.TargetExists || result.SourceRemoved || !result.SourceRetainedForStreams {
			t.Fatalf("incorrect stream-preserving copy result: %+v", result)
		}
		assertFileBytes(t, source, []byte("default stream"))
		assertFileBytes(t, stream, []byte("must not be discarded"))
		assertFileBytes(t, target, []byte("default stream"))
		assertFileBytes(t, target+":metadata", []byte("must not be discarded"))
	})
	t.Run("hard-link", func(t *testing.T) {
		dir := t.TempDir()
		source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
		writeTestFile(t, source, []byte("keep"))
		alias := filepath.Join(dir, "alias")
		if err := os.Link(source, alias); err != nil {
			t.Skipf("filesystem does not support hard links: %v", err)
		}
		result, err := Transfer(context.Background(), source, target)
		if !errors.Is(err, ErrUnsafePath) || result.TargetCreated || result.SourceRemoved {
			t.Fatalf("hard linked source accepted: %+v, %v", result, err)
		}
		assertFileBytes(t, source, []byte("keep"))
		assertFileBytes(t, alias, []byte("keep"))
	})
}
