package localworkspace

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ExportPlanOptions struct {
	IncludeLocalPaths     bool
	IncludeFileSecretRefs bool
}

type ExportPlan struct {
	WorkspaceID  string               `json:"workspaceId"`
	IncludePaths []string             `json:"includePaths"`
	ExcludePaths []ExportExcludedPath `json:"excludePaths"`
	Warnings     []string             `json:"warnings,omitempty"`
}

type ExportExcludedPath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func BuildExportPlan(workspace Workspace, opts ExportPlanOptions) (ExportPlan, error) {
	if strings.TrimSpace(workspace.Root) == "" {
		return ExportPlan{}, NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	plan := ExportPlan{WorkspaceID: workspace.Document.ID}
	addExclude := func(path string, reason string) {
		if path == "" {
			path = "."
		}
		plan.ExcludePaths = append(plan.ExcludePaths, ExportExcludedPath{Path: filepath.ToSlash(path), Reason: reason})
	}
	err := filepath.WalkDir(workspace.Root, func(current string, entry fs.DirEntry, walkErr error) error {
		rel, relErr := workspaceRelativePath(workspace.Root, current)
		if relErr != nil {
			plan.Warnings = append(plan.Warnings, relErr.Error())
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			if rel == "" {
				plan.Warnings = append(plan.Warnings, walkErr.Error())
			} else {
				plan.Warnings = append(plan.Warnings, rel+": "+walkErr.Error())
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "" {
			return nil
		}
		top := rel
		if slash := strings.IndexByte(rel, '/'); slash >= 0 {
			top = rel[:slash]
		}
		if entry.IsDir() {
			switch top {
			case ".opsc":
				addExclude(rel, "workspace control directory")
				return filepath.SkipDir
			case "cache":
				addExclude(rel, "derived cache directory")
				return filepath.SkipDir
			case "exports":
				addExclude(rel, "export output directory")
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			addExclude(rel, "symlink is not exported by default")
			return nil
		}
		if rel == IndexFileName {
			addExclude(rel, "derived index can be rebuilt")
			return nil
		}
		if rel == WorkspaceFileName {
			plan.IncludePaths = append(plan.IncludePaths, rel)
			return nil
		}
		if isProjectDocumentPath(rel) && !opts.IncludeLocalPaths {
			addExclude(rel, "project document contains local rootPath metadata")
			return nil
		}
		if strings.HasSuffix(rel, ".json") {
			containsFileSecret, err := jsonFileContainsFileSecretRef(current)
			if err != nil {
				plan.Warnings = append(plan.Warnings, rel+": "+err.Error())
				return nil
			}
			if containsFileSecret && !opts.IncludeFileSecretRefs {
				addExclude(rel, "file secretRef points to local private secret material")
				return nil
			}
		}
		plan.IncludePaths = append(plan.IncludePaths, rel)
		return nil
	})
	if err != nil {
		return ExportPlan{}, WrapError(ErrorInternal, "build export plan", 5, err)
	}
	return plan, nil
}

func isProjectDocumentPath(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) == 3 && parts[0] == "projects" && parts[2] == "project.json"
}

func jsonFileContainsFileSecretRef(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return rawContainsFileSecretRef(data), nil
}

func rawContainsFileSecretRef(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for key, value := range object {
			if key == "secretRef" {
				var ref SecretRef
				if err := json.Unmarshal(value, &ref); err == nil && ref.Type == SecretRefTypeFile {
					return true
				}
				continue
			}
			if rawContainsFileSecretRef(value) {
				return true
			}
		}
		return false
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		for _, value := range array {
			if rawContainsFileSecretRef(value) {
				return true
			}
		}
	}
	return false
}
