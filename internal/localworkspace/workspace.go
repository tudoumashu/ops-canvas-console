package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Init(opts InitOptions) (*InitResult, error) {
	resolved, err := ResolvePath(opts.Path)
	if err != nil {
		return nil, err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = DefaultWorkspaceName
	}
	initLockPath, err := workspaceInitLockPath(resolved.Path)
	if err != nil {
		return nil, err
	}
	if err := ensurePrivateStateDir(filepath.Dir(initLockPath)); err != nil {
		return nil, err
	}
	lock, err := AcquireLock(initLockPath)
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	if err := ensureRequiredDirs(resolved.Path); err != nil {
		return nil, err
	}
	workspacePath := filepath.Join(resolved.Path, WorkspaceFileName)
	if _, err := os.Stat(workspacePath); err == nil {
		workspace, err := OpenResolved(resolved)
		if err != nil {
			return nil, err
		}
		if err := ensureIndexPlaceholder(resolved.Path); err != nil {
			return nil, err
		}
		return &InitResult{Workspace: *workspace, Created: false}, nil
	} else if !os.IsNotExist(err) {
		return nil, WrapError(ErrorInternal, "stat workspace document", 5, err)
	}

	current := now()
	id, err := NewID("ws", current)
	if err != nil {
		return nil, err
	}
	timestamp := current.UTC().Format(time.RFC3339)
	document := WorkspaceDocument{
		SchemaVersion: SchemaVersion,
		Kind:          KindWorkspace,
		ID:            id,
		Revision:      1,
		CreatedAt:     timestamp,
		UpdatedAt:     timestamp,
		Data: WorkspaceData{
			Name:        name,
			Preferences: normalizeWorkspacePreferences(WorkspacePreferences{}),
		},
	}
	if err := AtomicWriteJSON(workspacePath, document, 0o600); err != nil {
		return nil, err
	}
	if err := ensureIndexPlaceholder(resolved.Path); err != nil {
		return nil, err
	}
	return &InitResult{
		Workspace: Workspace{
			Root:     resolved.Path,
			Source:   resolved.Source,
			Document: document,
		},
		Created: true,
	}, nil
}

func Open(path string) (*Workspace, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	return OpenResolved(resolved)
}

func OpenResolved(resolved ResolvedPath) (*Workspace, error) {
	document, err := readWorkspaceDocument(filepath.Join(resolved.Path, WorkspaceFileName))
	if err != nil {
		return nil, err
	}
	return &Workspace{
		Root:     resolved.Path,
		Source:   resolved.Source,
		Document: document,
	}, nil
}

func (w Workspace) Info(showPaths bool) WorkspaceInfo {
	info := WorkspaceInfo{
		ID:               w.Document.ID,
		Name:             w.Document.Data.Name,
		SchemaVersion:    w.Document.SchemaVersion,
		Revision:         w.Document.Revision,
		DefaultProfileID: w.Document.Data.DefaultProfileID,
		PathSource:       w.Source,
		Directories:      append([]string{}, RequiredDirs...),
		Runtime:          w.readRuntimeInfo(),
	}
	if showPaths {
		info.Path = w.Root
	}
	return info
}

func (w Workspace) Path(parts ...string) string {
	all := append([]string{w.Root}, parts...)
	return filepath.Join(all...)
}

func (w Workspace) WorkspaceFilePath() string {
	return w.Path(WorkspaceFileName)
}

func (w Workspace) LockPath(name string) string {
	stateName := name
	if name == "workspace.lock" {
		stateName = "workspace.write.lock"
	}
	if path, err := w.StatePath(stateName); err == nil {
		return path
	}
	return w.Path(".opsc", "locks", name)
}

func (w Workspace) RuntimePath(name string) string {
	if path, err := w.StatePath(name); err == nil {
		return path
	}
	return w.Path(".opsc", "runtime", name)
}

func ensureRequiredDirs(root string) error {
	for _, dir := range RequiredDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return WrapError(ErrorInternal, "create workspace directory", 5, err)
		}
	}
	return nil
}

func ensureIndexPlaceholder(root string) error {
	path := filepath.Join(root, IndexFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_ = file.Sync()
		_ = file.Close()
		syncDir(root)
		return nil
	}
	if os.IsExist(err) {
		return nil
	}
	return WrapError(ErrorInternal, "create index placeholder", 5, err)
}

func readWorkspaceDocument(path string) (WorkspaceDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkspaceDocument{}, NewError(ErrorWorkspaceNotFound, "workspace document not found", 2, nil)
		}
		return WorkspaceDocument{}, WrapError(ErrorInternal, "read workspace document", 5, err)
	}
	var document WorkspaceDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return WorkspaceDocument{}, WrapError(ErrorWorkspaceInvalid, "parse workspace document", 2, err)
	}
	if err := validateWorkspaceDocument(document); err != nil {
		return WorkspaceDocument{}, err
	}
	return document, nil
}

func validateWorkspaceDocument(document WorkspaceDocument) error {
	if document.SchemaVersion != SchemaVersion {
		return NewError(ErrorWorkspaceInvalid, "workspace schema version mismatch", 2, map[string]string{"schemaVersion": document.SchemaVersion})
	}
	if document.Kind != KindWorkspace {
		return NewError(ErrorWorkspaceInvalid, "workspace kind mismatch", 2, map[string]string{"kind": document.Kind})
	}
	if !strings.HasPrefix(document.ID, "ws_") {
		return NewError(ErrorWorkspaceInvalid, "workspace id must use ws_ prefix", 2, map[string]string{"id": document.ID})
	}
	if document.Revision < 1 {
		return NewError(ErrorWorkspaceInvalid, "workspace revision must be at least 1", 2, nil)
	}
	if strings.TrimSpace(document.Data.Name) == "" {
		return NewError(ErrorWorkspaceInvalid, "workspace name is empty", 2, nil)
	}
	if err := validateWorkspacePreferences(normalizeWorkspacePreferences(document.Data.Preferences)); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, document.CreatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse workspace createdAt", 2, err)
	}
	if _, err := time.Parse(time.RFC3339, document.UpdatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse workspace updatedAt", 2, err)
	}
	return nil
}

func (w Workspace) readRuntimeInfo() RuntimeInfo {
	path, err := w.StatePath("serve.json")
	if err != nil {
		return RuntimeInfo{Active: false}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeInfo{Active: false}
	}
	var metadata RuntimeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return RuntimeInfo{Active: false}
	}
	active := metadata.PID > 0 && metadata.Port > 0 && processExists(metadata.PID)
	return RuntimeInfo{
		Active:           active,
		PID:              metadata.PID,
		Host:             metadata.Host,
		Port:             metadata.Port,
		BaseURL:          metadata.BaseURL,
		TokenFile:        relativeRuntimeTokenFile(metadata.TokenFile),
		LaunchSecretFile: relativeRuntimeTokenFile(metadata.LaunchSecretFile),
	}
}

func relativeRuntimeTokenFile(path string) string {
	if filepath.IsAbs(path) {
		return ""
	}
	return path
}
