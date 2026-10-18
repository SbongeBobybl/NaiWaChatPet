//go:build !windows

package tidy

func directoryLock(dir string) (func(), error) { return func() {}, nil }
