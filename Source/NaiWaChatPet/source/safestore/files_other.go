//go:build !windows

package safestore

import (
	"context"
	"os"
)

// Fail closed outside Windows: a portable check-then-unlink fallback cannot
// provide the same handle-bound deletion guarantee.
func validatePath(string) error             { return ErrUnsupported }
func validateInputPath(string) error        { return ErrUnsupported }
func openSource(string) (*os.File, error)   { return nil, ErrUnsupported }
func createTarget(string) (*os.File, error) { return nil, ErrUnsupported }
func deleteByHandle(*os.File) error         { return ErrUnsupported }

func copyAllStreamsKeepingSource(_ context.Context, result Result) (Result, error) {
	return result, ErrUnsupported
}
