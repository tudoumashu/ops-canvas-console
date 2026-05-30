package localworkspace

import (
	"os"
	"path/filepath"
	"strings"
)

func ResolvePath(explicitPath string) (ResolvedPath, error) {
	if strings.TrimSpace(explicitPath) != "" {
		path, err := expandPath(explicitPath)
		if err != nil {
			return ResolvedPath{}, err
		}
		return ResolvedPath{Path: path, Source: PathSourceFlag}, nil
	}
	if envPath := strings.TrimSpace(os.Getenv(EnvWorkspace)); envPath != "" {
		path, err := expandPath(envPath)
		if err != nil {
			return ResolvedPath{}, err
		}
		return ResolvedPath{Path: path, Source: PathSourceEnv}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ResolvedPath{}, WrapError(ErrorInternal, "resolve home directory", 5, err)
	}
	path, err := filepath.Abs(filepath.Join(home, DefaultWorkspaceDir))
	if err != nil {
		return ResolvedPath{}, WrapError(ErrorInvalidArgument, "resolve workspace path", 1, err)
	}
	return ResolvedPath{Path: filepath.Clean(path), Source: PathSourceDefault}, nil
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(os.ExpandEnv(path))
	if path == "" {
		return "", NewError(ErrorInvalidArgument, "workspace path is empty", 1, nil)
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", WrapError(ErrorInternal, "resolve home directory", 5, err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve workspace path", 1, err)
	}
	return filepath.Clean(abs), nil
}

func workspaceRelativePath(root string, path string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve workspace root", 1, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve workspace path", 1, err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve workspace relative path", 1, err)
	}
	if rel == "." {
		return "", nil
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", NewError(ErrorInvalidArgument, "path escapes workspace root", 1, map[string]string{"path": path})
	}
	return filepath.ToSlash(rel), nil
}

func validatePathComponent(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return NewError(ErrorInvalidArgument, label+" is empty", 1, nil)
	}
	if filepath.IsAbs(value) || strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "..") {
		return NewError(ErrorInvalidArgument, label+" is not path safe", 1, map[string]string{label: value})
	}
	return nil
}
