package localworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type IndexRebuilder interface {
	Rebuild(ctx context.Context, workspace Workspace, scan ScanResult) error
}

type NoopIndexRebuilder struct{}

func (NoopIndexRebuilder) Rebuild(ctx context.Context, workspace Workspace, scan ScanResult) error {
	return ctx.Err()
}

type scanSpec struct {
	Collection string
	FileName   string
	Kind       string
	IDPrefix   string
}

var canonicalScanSpecs = []scanSpec{
	{Collection: "profiles", FileName: "profile.json", Kind: KindProfile, IDPrefix: "profile"},
	{Collection: "projects", FileName: "project.json", Kind: KindProject, IDPrefix: "proj"},
	{Collection: "templates", FileName: "template.json", Kind: KindTemplate, IDPrefix: "tpl"},
	{Collection: "runs", FileName: "run.json", Kind: KindRun, IDPrefix: "run"},
	{Collection: "artifacts", FileName: "artifact.json", Kind: KindArtifact, IDPrefix: "art"},
	{Collection: "assets", FileName: "asset.json", Kind: KindAsset, IDPrefix: "asset"},
	{Collection: "prompts", FileName: "prompt.json", Kind: KindPrompt, IDPrefix: "prompt"},
	{Collection: "canvas-projects", FileName: "canvas-project.json", Kind: KindCanvasProject, IDPrefix: "canvas"},
	{Collection: "workbench-logs", FileName: "workbench-log.json", Kind: KindWorkbenchLog, IDPrefix: "wblog"},
}

type envelopeHeader struct {
	SchemaVersion string `json:"schemaVersion"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Revision      int    `json:"revision"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func Scan(ctx context.Context, workspace Workspace, opts ScanOptions) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	if workspace.Root == "" {
		return ScanResult{}, NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	result := ScanResult{WorkspaceID: workspace.Document.ID}
	scanFile(ctx, workspace, scanSpec{FileName: WorkspaceFileName, Kind: KindWorkspace, IDPrefix: "ws"}, workspace.Document.ID, &result)

	for _, spec := range canonicalScanSpecs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		dir := workspace.Path(spec.Collection)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				result.Warnings = append(result.Warnings, "missing collection directory: "+spec.Collection)
				continue
			}
			return result, WrapError(ErrorInternal, "read collection directory", 5, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			id := entry.Name()
			if entry.Type()&os.ModeSymlink != 0 {
				result.Warnings = append(result.Warnings, "skip symlink object directory: "+filepath.ToSlash(filepath.Join(spec.Collection, id)))
				continue
			}
			if !entry.IsDir() {
				continue
			}
			if err := validatePathComponent("object id", id); err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			if err := validateScannedID(id, spec.IDPrefix); err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			scanFile(ctx, workspace, spec, id, &result)
		}
	}

	sort.Slice(result.Entries, func(i int, j int) bool {
		return result.Entries[i].Path < result.Entries[j].Path
	})
	return result, nil
}

func RebuildIndex(ctx context.Context, workspace Workspace, rebuilder IndexRebuilder, opts ScanOptions) (ScanResult, error) {
	if rebuilder == nil {
		return ScanResult{}, NewError(ErrorInvalidArgument, "index rebuilder is nil", 1, nil)
	}
	if workspace.Root == "" {
		return ScanResult{}, NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	lockPath, err := workspaceWriteLockPath(workspace)
	if err != nil {
		return ScanResult{}, err
	}
	if err := ensurePrivateStateDir(filepath.Dir(lockPath)); err != nil {
		return ScanResult{}, err
	}
	lock, err := AcquireLock(lockPath)
	if err != nil {
		return ScanResult{}, err
	}
	defer lock.Release()

	result, err := Scan(ctx, workspace, opts)
	if err != nil {
		return result, err
	}
	if err := rebuilder.Rebuild(ctx, workspace, result); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, err
		}
		return result, WrapError(ErrorInternal, "rebuild workspace index", 5, err)
	}
	return result, nil
}

func scanFile(ctx context.Context, workspace Workspace, spec scanSpec, expectedID string, result *ScanResult) {
	if err := ctx.Err(); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return
	}
	path := workspace.Path(spec.FileName)
	if spec.Collection != "" {
		path = workspace.Path(spec.Collection, expectedID, spec.FileName)
	}
	rel, err := workspaceRelativePath(workspace.Root, path)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, "missing object document: "+rel)
			return
		}
		result.Warnings = append(result.Warnings, err.Error())
		return
	}
	var header envelopeHeader
	if err := json.Unmarshal(data, &header); err != nil {
		result.Warnings = append(result.Warnings, "invalid object document json: "+rel)
		return
	}
	if err := validateScannedEnvelope(header, spec, expectedID); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return
	}
	result.Entries = append(result.Entries, ScanEntry{
		Kind:      header.Kind,
		ID:        header.ID,
		Path:      rel,
		FileName:  spec.FileName,
		Revision:  header.Revision,
		UpdatedAt: header.UpdatedAt,
	})
}

func validateScannedEnvelope(header envelopeHeader, spec scanSpec, expectedID string) error {
	if header.SchemaVersion != SchemaVersion {
		return NewError(ErrorWorkspaceInvalid, "object schema version mismatch", 2, map[string]string{"schemaVersion": header.SchemaVersion})
	}
	if header.Kind != spec.Kind {
		return NewError(ErrorWorkspaceInvalid, "object kind mismatch", 2, map[string]string{"kind": header.Kind})
	}
	if expectedID != "" && header.ID != expectedID {
		return NewError(ErrorWorkspaceInvalid, "object id does not match path", 2, map[string]string{"id": header.ID, "pathId": expectedID})
	}
	if err := validateScannedID(header.ID, spec.IDPrefix); err != nil {
		return err
	}
	if header.Revision < 1 {
		return NewError(ErrorWorkspaceInvalid, "object revision must be at least 1", 2, map[string]string{"id": header.ID})
	}
	if _, err := time.Parse(time.RFC3339, header.CreatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse object createdAt", 2, err)
	}
	if _, err := time.Parse(time.RFC3339, header.UpdatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse object updatedAt", 2, err)
	}
	return nil
}

func validateScannedID(id string, prefix string) error {
	if err := validatePathComponent("object id", id); err != nil {
		return err
	}
	if prefix == "" {
		return nil
	}
	if len(id) <= len(prefix)+1 || id[:len(prefix)+1] != prefix+"_" {
		return NewError(ErrorWorkspaceInvalid, "object id prefix mismatch", 2, map[string]string{"id": id})
	}
	return nil
}
