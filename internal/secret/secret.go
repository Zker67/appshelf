// Package secret keeps a secret of the user, as the GitHub token, in a file
// that only this Windows user can read: Windows encrypts it with the user's
// key (DPAPI), so a copy of the file is useless elsewhere.
package secret

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrUnsupported is a system without the user's encryption.
var ErrUnsupported = errors.New("这个系统不支持加密保存")

// Save encrypts value for the current user and writes it to path, replacing
// the file at once.
func Save(path, value string) error {
	data, err := protect([]byte(value))
	if err != nil {
		return fmt.Errorf("无法加密：%w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Load reads and decrypts the value at path; a missing file is "" and no
// error.
func Load(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := unprotect(data)
	if err != nil {
		return "", fmt.Errorf("无法解密（可能来自另一个 Windows 用户）：%w", err)
	}
	return string(plain), nil
}

// Delete removes the file at path; a missing file is no error.
func Delete(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
