//go:build !windows

package secret

func protect([]byte) ([]byte, error)   { return nil, ErrUnsupported }
func unprotect([]byte) ([]byte, error) { return nil, ErrUnsupported }
