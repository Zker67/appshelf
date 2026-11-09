//go:build windows

package secret

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// entropy ties the encrypted data to AppShelf: another program of the user
// calling DPAPI without it cannot decrypt the file.
var entropy = []byte("AppShelf secret v1")

const cryptprotectUIForbidden = 0x1

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func take(out windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
}

func protect(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(plain), nil, blob(entropy), 0, nil, cryptprotectUIForbidden, &out); err != nil {
		return nil, err
	}
	return take(out), nil
}

func unprotect(data []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(data), nil, blob(entropy), 0, nil, cryptprotectUIForbidden, &out); err != nil {
		return nil, err
	}
	return take(out), nil
}
