// Package jsonfile reads and writes JSON files, replacing a file only once
// its new content is completely written.
package jsonfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Read decodes the JSON file at path into v. A missing file returns an error
// that errors.Is(err, fs.ErrNotExist) recognizes.
func Read(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

// WriteAtomic writes v as indented JSON to a temporary file next to path,
// then renames it over path, so a crash never leaves a half-written file.
func WriteAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteBytesAtomic(path, data)
}

// WriteBytesAtomic writes data to path through a temporary file renamed over
// it.
func WriteBytesAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if serr := tmp.Sync(); werr == nil {
		werr = serr
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(name)
		return werr
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
