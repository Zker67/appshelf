package secret

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRoundTrip encrypts with the real DPAPI of this user.
func TestRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI is Windows only")
	}
	path := filepath.Join(t.TempDir(), "sub", "token.bin")
	if v, err := Load(path); v != "" || err != nil {
		t.Fatalf("missing file = %q %v", v, err)
	}
	const value = "ghp_ExampleOnly1234567890"
	if err := Save(path, value); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(value)) || bytes.Contains(data, []byte("ExampleOnly")) {
		t.Fatal("the file holds the value in clear")
	}
	if v, err := Load(path); v != value || err != nil {
		t.Fatalf("load = %q %v", v, err)
	}
	if err := Save(path, "other"); err != nil {
		t.Fatal(err)
	}
	if v, _ := Load(path); v != "other" {
		t.Fatalf("replaced = %q", v)
	}

	// Data that is not DPAPI's does not decrypt.
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("garbage decrypted")
	}

	if err := Delete(path); err != nil {
		t.Fatal(err)
	}
	if err := Delete(path); err != nil {
		t.Fatalf("deleting a missing file: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}
