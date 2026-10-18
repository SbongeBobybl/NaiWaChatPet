//go:build !windows

package companion

func protectLocal([]byte) ([]byte, error)   { return nil, errLocalProtection }
func unprotectLocal([]byte) ([]byte, error) { return nil, errLocalProtection }
