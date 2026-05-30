package localworkspace

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

func AtomicWriteJSON(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return WrapError(ErrorInternal, "write json", 5, err)
	}
	data = append(data, '\n')
	return AtomicWriteFile(path, data, perm)
}

func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return AtomicWriteFromReader(path, bytes.NewReader(data), perm)
}

func AtomicWriteFromReader(path string, reader io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WrapError(ErrorInternal, "create parent directory", 5, err)
	}
	tmp, err := tempPath(dir, filepath.Base(path))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return WrapError(ErrorInternal, "create temporary file", 5, err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return WrapError(ErrorInternal, "write file", 5, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return WrapError(ErrorInternal, "sync file", 5, err)
	}
	if err := file.Close(); err != nil {
		return WrapError(ErrorInternal, "close file", 5, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return WrapError(ErrorInternal, "replace file", 5, err)
	}
	ok = true
	syncDir(dir)
	return nil
}

func tempPath(dir string, base string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", WrapError(ErrorInternal, "generate temporary filename", 5, err)
	}
	return filepath.Join(dir, "."+base+"."+hex.EncodeToString(buf)+".tmp"), nil
}

func syncDir(dir string) {
	file, err := os.Open(dir)
	if err != nil {
		return
	}
	defer file.Close()
	_ = file.Sync()
}
