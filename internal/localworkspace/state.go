package localworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const stateAppDir = "opsc"

func stateHome() (string, error) {
	if value := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); value != "" {
		expanded, err := expandPath(value)
		if err != nil {
			return "", err
		}
		return expanded, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", WrapError(ErrorInternal, "resolve home directory", 5, err)
	}
	return filepath.Join(home, ".local", "state"), nil
}

func workspaceStateDir(workspace Workspace) (string, error) {
	if strings.TrimSpace(workspace.Root) == "" {
		return "", NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	workspaceID := strings.TrimSpace(workspace.Document.ID)
	if workspaceID == "" {
		workspaceID = "workspace"
	}
	return workspaceStateDirForRoot(workspace.Root, workspaceID)
}

func workspaceStateDirForRoot(root string, workspaceID string) (string, error) {
	home, err := stateHome()
	if err != nil {
		return "", err
	}
	hash, err := workspaceRootHash(root)
	if err != nil {
		return "", err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		workspaceID = "workspace"
	}
	name := workspaceID + "-" + hash[:16]
	return filepath.Join(home, stateAppDir, "workspaces", name), nil
}

func workspaceRootHash(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve workspace root", 1, err)
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = filepath.Clean(resolved)
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:]), nil
}

func ensurePrivateStateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return WrapError(ErrorInternal, "create runtime state directory", 5, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return WrapError(ErrorInternal, "chmod runtime state directory", 5, err)
	}
	return nil
}

func workspaceStatePath(workspace Workspace, parts ...string) (string, error) {
	dir, err := workspaceStateDir(workspace)
	if err != nil {
		return "", err
	}
	all := append([]string{dir}, parts...)
	return filepath.Join(all...), nil
}

func workspaceInitLockPath(root string) (string, error) {
	dir, err := workspaceStateDirForRoot(root, "pending")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspace.init.lock"), nil
}

func workspaceWriteLockPath(workspace Workspace) (string, error) {
	return workspaceStatePath(workspace, "workspace.write.lock")
}

func (w Workspace) StateDir() (string, error) {
	return workspaceStateDir(w)
}

func (w Workspace) StatePath(parts ...string) (string, error) {
	return workspaceStatePath(w, parts...)
}
