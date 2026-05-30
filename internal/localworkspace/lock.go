package localworkspace

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Lock struct {
	path string
	file *os.File
}

func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, WrapError(ErrorInternal, "create lock directory", 5, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, NewError(ErrorWorkspaceLocked, "workspace lock already exists", 2, nil)
		}
		return nil, WrapError(ErrorInternal, "create workspace lock", 5, err)
	}
	_, _ = fmt.Fprintf(file, "pid=%d\ncreatedAt=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	_ = file.Sync()
	return &Lock{path: path, file: file}, nil
}

func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return WrapError(ErrorInternal, "remove workspace lock", 5, err)
	}
	syncDir(filepath.Dir(l.path))
	return nil
}
