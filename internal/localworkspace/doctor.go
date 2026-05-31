package localworkspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DoctorOptions struct {
	Path      string
	ShowPath  bool
	CheckLock bool
}

func Doctor(opts DoctorOptions) (*DoctorReport, error) {
	resolved, err := ResolvePath(opts.Path)
	if err != nil {
		return nil, err
	}
	report := &DoctorReport{Checks: []DoctorCheck{}}
	if opts.ShowPath {
		report.Path = resolved.Path
	}
	addCheck := func(name string, ok bool, severity string, message string) {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:     name,
			OK:       ok,
			Severity: severity,
			Message:  message,
		})
		if !ok {
			if severity == "error" {
				report.Errors = append(report.Errors, message)
			} else {
				report.Warnings = append(report.Warnings, message)
			}
		}
	}

	if stat, err := os.Stat(resolved.Path); err != nil {
		if os.IsNotExist(err) {
			addCheck("workspace_root", false, "error", "workspace root does not exist")
			report.OK = false
			return report, nil
		}
		return nil, WrapError(ErrorInternal, "stat workspace root", 5, err)
	} else if !stat.IsDir() {
		addCheck("workspace_root", false, "error", "workspace root is not a directory")
		report.OK = false
		return report, nil
	} else {
		addCheck("workspace_root", true, "info", "workspace root exists")
	}

	document, err := readWorkspaceDocument(filepath.Join(resolved.Path, WorkspaceFileName))
	if err != nil {
		addCheck("workspace_document", false, "error", err.Error())
		report.OK = false
		return report, nil
	}
	report.WorkspaceID = document.ID
	report.SchemaVersion = document.SchemaVersion
	addCheck("workspace_document", true, "info", "workspace document is valid")

	for _, dir := range RequiredDirs {
		stat, err := os.Stat(filepath.Join(resolved.Path, dir))
		if err != nil {
			if os.IsNotExist(err) {
				addCheck("dir:"+dir, false, "error", "required directory missing: "+dir)
				continue
			}
			return nil, WrapError(ErrorInternal, "stat workspace directory", 5, err)
		}
		if !stat.IsDir() {
			addCheck("dir:"+dir, false, "error", "required path is not a directory: "+dir)
			continue
		}
		addCheck("dir:"+dir, true, "info", "required directory exists: "+dir)
	}

	indexPath := filepath.Join(resolved.Path, IndexFileName)
	if stat, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			addCheck("index", false, "warning", "index.sqlite is missing; run opsc workspace index rebuild")
		} else {
			return nil, WrapError(ErrorInternal, "stat index", 5, err)
		}
	} else if stat.IsDir() {
		addCheck("index", false, "error", "index.sqlite is a directory")
	} else {
		addCheck("index", true, "info", "index.sqlite exists")
		if err := checkIndexFreshness(resolved.Path, stat.ModTime(), addCheck); err != nil {
			return nil, err
		}
	}

	workspace := Workspace{Root: resolved.Path, Source: resolved.Source, Document: document}
	if opts.CheckLock {
		lockPath, err := workspaceWriteLockPath(workspace)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(lockPath); err == nil {
			addCheck("workspace_lock", false, "warning", "workspace write lock exists")
		} else if err != nil && !os.IsNotExist(err) {
			return nil, WrapError(ErrorInternal, "stat workspace write lock", 5, err)
		} else {
			addCheck("workspace_lock", true, "info", "workspace write lock is clear")
		}
	}
	checkExecutorRuntime(workspace, addCheck)

	if err := runObjectDoctorChecks(resolved, document, opts.ShowPath, addCheck); err != nil {
		return nil, err
	}
	report.OK = len(report.Errors) == 0
	return report, nil
}

type doctorAddCheck func(name string, ok bool, severity string, message string)

type doctorEnvelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	Kind          string          `json:"kind"`
	ID            string          `json:"id"`
	Data          json.RawMessage `json:"data"`
}

type doctorRunData struct {
	TemplateID   string            `json:"templateId"`
	ProfileID    string            `json:"profileId"`
	ProjectID    string            `json:"projectId"`
	Status       string            `json:"status"`
	Metadata     map[string]any    `json:"metadata"`
	ArtifactRefs []json.RawMessage `json:"artifactRefs"`
}

type doctorRunArtifactRefData struct {
	ArtifactID string `json:"artifactId"`
}

type doctorAssetData struct {
	SourceArtifactID string            `json:"sourceArtifactId"`
	Files            map[string]string `json:"files"`
}

type doctorProjectData struct {
	RootPath        string               `json:"rootPath"`
	RootFingerprint string               `json:"rootFingerprint"`
	Capabilities    ProjectCapabilities  `json:"capabilities"`
	Execution       ProjectExecution     `json:"execution"`
	CredentialRefs  map[string]SecretRef `json:"credentialRefs"`
}

type secretRefStats struct {
	Found    int
	Problems int
}

func runObjectDoctorChecks(resolved ResolvedPath, document WorkspaceDocument, showPaths bool, addCheck doctorAddCheck) error {
	workspace := Workspace{Root: resolved.Path, Source: resolved.Source, Document: document}
	executorStatus := readExecutorRuntimeStatus(workspace)
	scan, err := Scan(context.Background(), workspace, ScanOptions{})
	if err != nil {
		return err
	}
	if len(scan.Warnings) == 0 {
		addCheck("scan", true, "info", "canonical object scan completed")
	} else {
		for _, warning := range scan.Warnings {
			addCheck("scan", false, scanWarningSeverity(warning), warning)
		}
	}

	if strings.TrimSpace(document.Data.DefaultProfileID) != "" {
		addObjectRefCheck(resolved.Path, "ref:workspace.defaultProfileId", document.Data.DefaultProfileID, "profiles", "profile.json", addCheck)
	}

	stats := &secretRefStats{}
	if err := checkProfiles(resolved.Path, addCheck, stats); err != nil {
		return err
	}
	if err := checkProjects(resolved.Path, showPaths, addCheck, stats); err != nil {
		return err
	}
	if err := checkRuns(resolved.Path, executorStatus.Active, addCheck); err != nil {
		return err
	}
	if err := checkAssets(resolved.Path, addCheck); err != nil {
		return err
	}
	if err := checkPrompts(resolved.Path, addCheck); err != nil {
		return err
	}
	if err := checkCanvasProjects(resolved.Path, addCheck, stats); err != nil {
		return err
	}
	if err := checkWorkbenchLogs(resolved.Path, addCheck, stats); err != nil {
		return err
	}
	if err := checkExportPlan(workspace, addCheck); err != nil {
		return err
	}
	if err := checkGCPlan(workspace, addCheck); err != nil {
		return err
	}
	if stats.Problems == 0 {
		if stats.Found == 0 {
			addCheck("secret_refs", true, "info", "no secretRef placeholders found")
		} else {
			addCheck("secret_refs", true, "info", "secretRef placeholders are structurally valid")
		}
	}
	return nil
}

func checkIndexFreshness(root string, indexModTime time.Time, addCheck doctorAddCheck) error {
	latest, err := latestCanonicalFileModTime(root)
	if err != nil {
		return err
	}
	if latest.IsZero() || !latest.After(indexModTime.Add(time.Second)) {
		addCheck("index_freshness", true, "info", "index.sqlite appears fresh")
		return nil
	}
	addCheck("index_freshness", false, "warning", "index.sqlite may be stale; run opsc workspace index rebuild")
	return nil
}

func checkExecutorRuntime(workspace Workspace, addCheck doctorAddCheck) {
	status := readExecutorRuntimeStatus(workspace)
	switch {
	case status.Active:
		addCheck("executor_worker", true, "info", "executor worker is active")
	case status.Stale:
		addCheck("executor_worker", false, "warning", "executor worker runtime is stale; restart opsc executor --watch")
	default:
		addCheck("executor_worker", true, "info", "executor worker is not running; start opsc executor --watch for Web-created local runs")
	}
}

func scanWarningSeverity(warning string) string {
	lower := strings.ToLower(warning)
	errorHints := []string{
		"schema",
		"kind",
		"revision",
		"createdat",
		"updatedat",
		"invalid object document",
		"object id",
		"missing object document",
	}
	for _, hint := range errorHints {
		if strings.Contains(lower, hint) {
			return "error"
		}
	}
	return "warning"
}

func checkProfiles(root string, addCheck doctorAddCheck, stats *secretRefStats) error {
	return forEachObject(root, "profiles", "profile.json", func(id string, envelope doctorEnvelope) error {
		inspectSecretRefs(envelope.Data, "profile "+id, addCheck, stats)
		return nil
	})
}

func checkProjects(root string, showPaths bool, addCheck doctorAddCheck, stats *secretRefStats) error {
	return forEachObject(root, "projects", "project.json", func(id string, envelope doctorEnvelope) error {
		var data doctorProjectData
		if len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				addCheck("project:"+id, false, "error", "project data is not valid json: "+id)
				return nil
			}
		}
		if strings.TrimSpace(data.RootPath) == "" {
			addCheck("project_root:"+id, false, "warning", "project rootPath is empty: "+id)
		} else if !filepath.IsAbs(data.RootPath) {
			addCheck("project_root:"+id, false, "error", "project rootPath is not absolute: "+id)
		} else {
			messageID := id
			if showPaths {
				messageID = id + " path=" + data.RootPath
			}
			if stat, err := os.Stat(data.RootPath); err != nil {
				addCheck("project_root:"+id, false, "warning", "project root is not accessible: "+messageID)
			} else if !stat.IsDir() {
				addCheck("project_root:"+id, false, "error", "project root is not a directory: "+messageID)
			} else {
				addCheck("project_root:"+id, true, "info", "project root is accessible: "+messageID)
			}
			checkProjectFingerprint(Workspace{Root: root}, id, data, addCheck)
		}
		checkProjectExecutionPolicy(id, data, addCheck)
		checkProjectCredentialRefs(id, data, addCheck, stats)
		inspectSecretRefs(envelope.Data, "project "+id, addCheck, stats)
		return nil
	})
}

func checkProjectFingerprint(workspace Workspace, id string, data doctorProjectData, addCheck doctorAddCheck) {
	expected, err := ExistingProjectRootFingerprint(workspace, data.RootPath)
	if err != nil {
		addCheck("project_root_fingerprint:"+id, false, "warning", "project root fingerprint cannot be verified: "+id)
		return
	}
	if strings.TrimSpace(data.RootFingerprint) == "" {
		addCheck("project_root_fingerprint:"+id, false, "warning", "project rootFingerprint is missing: "+id)
		return
	}
	if data.RootFingerprint != expected {
		addCheck("project_root_fingerprint:"+id, false, "warning", "project rootFingerprint does not match current root: "+id)
		return
	}
	addCheck("project_root_fingerprint:"+id, true, "info", "project rootFingerprint matches current root: "+id)
}

func checkProjectExecutionPolicy(id string, data doctorProjectData, addCheck doctorAddCheck) {
	denies := mergeDefaultProjectDenyGlobs(data.Execution.DenyGlobs)
	if len(denies) == 0 {
		addCheck("project_execution:"+id, false, "warning", "project execution deny globs are empty: "+id)
		return
	}
	if data.Capabilities.ProcessExec && len(normalizedProjectGlobs(data.Execution.AllowGlobs)) == 0 {
		addCheck("project_execution:"+id, false, "warning", "project process.exec is enabled without allowGlobs: "+id)
		return
	}
	addCheck("project_execution:"+id, true, "info", "project execution policy is present: "+id)
}

func checkProjectCredentialRefs(id string, data doctorProjectData, addCheck doctorAddCheck, stats *secretRefStats) {
	for name, ref := range data.CredentialRefs {
		stats.Found++
		if err := validatePathComponent("project credential ref name", name); err != nil {
			stats.Problems++
			addCheck("project_credential_ref:"+id, false, "error", "project credentialRef name is not path safe: "+id+"."+name)
			continue
		}
		if err := ref.Validate(); err != nil {
			stats.Problems++
			addCheck("project_credential_ref:"+id, false, "error", err.Error()+": "+id+"."+name)
			continue
		}
	}
}

func checkRuns(root string, executorActive bool, addCheck doctorAddCheck) error {
	return forEachObject(root, "runs", "run.json", func(id string, envelope doctorEnvelope) error {
		var data doctorRunData
		if len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				addCheck("run:"+id, false, "error", "run data is not valid json: "+id)
				return nil
			}
		}
		addObjectRefCheck(root, "ref:run.templateId", data.TemplateID, "templates", "template.json", addCheck)
		addObjectRefCheck(root, "ref:run.profileId", data.ProfileID, "profiles", "profile.json", addCheck)
		addObjectRefCheck(root, "ref:run.projectId", data.ProjectID, "projects", "project.json", addCheck)
		for _, artifactID := range runArtifactIDs(data.ArtifactRefs) {
			addObjectRefCheck(root, "ref:run.artifactRefs", artifactID, "artifacts", "artifact.json", addCheck)
		}
		if err := checkRunArtifactRefFiles(root, id, addCheck); err != nil {
			return err
		}
		if err := checkHybridRunHealth(root, id, data, executorActive, addCheck); err != nil {
			return err
		}
		return nil
	})
}

func checkHybridRunHealth(root string, runID string, data doctorRunData, executorActive bool, addCheck doctorAddCheck) error {
	if !doctorRunIsHybrid(root, data) {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(data.Status))
	switch status {
	case RunStatusPending:
		if doctorRunHasEvent(root, runID, "run.waiting_for_executor") {
			addCheck("hybrid_run:"+runID, false, "warning", "hybrid run is waiting for executor: "+runID+"; start opsc executor --watch")
		}
	case RunStatusRunning:
		if !executorActive {
			addCheck("hybrid_run:"+runID, false, "warning", "hybrid run is running but no active executor worker was detected: "+runID+"; start opsc executor --watch")
		}
	case RunStatusError:
		addCheck("hybrid_run:"+runID, false, "warning", "hybrid run failed: "+runID+"; inspect opsc run events "+runID+" and rerun after fixing credential or backend access")
	}
	return nil
}

func doctorRunIsHybrid(root string, data doctorRunData) bool {
	if doctorHybridMetadataMatches(data.Metadata) {
		return true
	}
	templateID := strings.TrimSpace(data.TemplateID)
	if templateID == "" {
		return false
	}
	envelope, err := readDoctorEnvelope(filepath.Join(root, "templates", templateID, "template.json"))
	if err != nil {
		return false
	}
	var template TemplateData
	if err := json.Unmarshal(envelope.Data, &template); err != nil {
		return false
	}
	return doctorHybridMetadataMatches(template.Metadata) || doctorHybridMetadataMatches(template.Settings)
}

func doctorHybridMetadataMatches(metadata map[string]any) bool {
	values, ok := asMapStringAny(metadata[hybridEcommerceKey])
	if !ok {
		return false
	}
	backend := firstNonEmptyString(stringFromMap(values, "backend"), hybridEcommerceBackend)
	return backend == hybridEcommerceBackend
}

func doctorRunHasEvent(root string, runID string, eventType string) bool {
	events, err := ReadRunEvents(Workspace{Root: root}, runID, 0)
	if err != nil {
		return false
	}
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func checkRunArtifactRefFiles(root string, runID string, addCheck doctorAddCheck) error {
	dir := filepath.Join(root, "runs", runID, "artifacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return WrapError(ErrorInternal, "read run artifact refs", 5, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ref.json") {
			continue
		}
		envelope, err := readDoctorEnvelope(filepath.Join(dir, entry.Name()))
		if err != nil {
			addCheck("run_artifact_ref:"+runID, false, "error", "run artifact ref is not valid json: "+runID+"/"+entry.Name())
			continue
		}
		var data doctorRunArtifactRefData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			addCheck("run_artifact_ref:"+runID, false, "error", "run artifact ref data is not valid json: "+runID+"/"+entry.Name())
			continue
		}
		addObjectRefCheck(root, "ref:run_artifact_ref.artifactId", data.ArtifactID, "artifacts", "artifact.json", addCheck)
	}
	return nil
}

func checkAssets(root string, addCheck doctorAddCheck) error {
	return forEachObject(root, "assets", "asset.json", func(id string, envelope doctorEnvelope) error {
		var data doctorAssetData
		if len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				addCheck("asset:"+id, false, "error", "asset data is not valid json: "+id)
				return nil
			}
		}
		addObjectRefCheck(root, "ref:asset.sourceArtifactId", data.SourceArtifactID, "artifacts", "artifact.json", addCheck)
		for name, filePath := range data.Files {
			if strings.TrimSpace(filePath) == "" {
				continue
			}
			if !isWorkspaceRelativeFile(filePath) {
				addCheck("asset_file:"+id, false, "error", "asset file path escapes asset directory: "+id+"."+name)
				continue
			}
			path := filepath.Join(root, "assets", id, filepath.FromSlash(filePath))
			if stat, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					addCheck("asset_file:"+id, false, "warning", "asset file is missing: "+id+"."+name)
					continue
				}
				addCheck("asset_file:"+id, false, "warning", "asset file is not accessible: "+id+"."+name)
			} else if stat.IsDir() {
				addCheck("asset_file:"+id, false, "error", "asset file points to a directory: "+id+"."+name)
			} else {
				addCheck("asset_file:"+id, true, "info", "asset file exists: "+id+"."+name)
			}
		}
		return nil
	})
}

func checkPrompts(root string, addCheck doctorAddCheck) error {
	return forEachObject(root, "prompts", "prompt.json", func(id string, _ doctorEnvelope) error {
		path := filepath.Join(root, "prompts", id, "content.md")
		if stat, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				addCheck("prompt_content:"+id, false, "warning", "prompt content.md is missing: "+id)
			} else {
				addCheck("prompt_content:"+id, false, "warning", "prompt content.md is not accessible: "+id)
			}
		} else if stat.IsDir() {
			addCheck("prompt_content:"+id, false, "error", "prompt content.md is a directory: "+id)
		} else {
			addCheck("prompt_content:"+id, true, "info", "prompt content.md exists: "+id)
		}
		return nil
	})
}

func checkCanvasProjects(root string, addCheck doctorAddCheck, stats *secretRefStats) error {
	return forEachObject(root, "canvas-projects", "canvas-project.json", func(id string, envelope doctorEnvelope) error {
		var data CanvasProjectData
		if len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				addCheck("canvas_project:"+id, false, "error", "canvas project data is not valid json: "+id)
				return nil
			}
			if _, err := prepareCanvasProjectData(data); err != nil {
				addCheck("canvas_project:"+id, false, "error", err.Error()+": "+id)
			} else {
				addCheck("canvas_project:"+id, true, "info", "canvas project data is valid: "+id)
			}
			checkCanvasProjectFiles(root, id, data.Files, addCheck)
		}
		inspectSecretRefs(envelope.Data, "canvas_project "+id, addCheck, stats)
		return nil
	})
}

func checkCanvasProjectFiles(root string, id string, files map[string]CanvasProjectFile, addCheck doctorAddCheck) {
	for key, item := range files {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		if !isWorkspaceRelativeFile(item.Path) || !strings.HasPrefix(filepath.ToSlash(filepath.Clean(item.Path)), "files/") {
			addCheck("canvas_project_file:"+id, false, "error", "canvas project file path escapes files directory: "+id+"."+key)
			continue
		}
		path := filepath.Join(root, "canvas-projects", id, filepath.FromSlash(item.Path))
		if stat, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				addCheck("canvas_project_file:"+id, false, "warning", "canvas project file is missing: "+id+"."+key)
				continue
			}
			addCheck("canvas_project_file:"+id, false, "warning", "canvas project file is not accessible: "+id+"."+key)
		} else if stat.IsDir() {
			addCheck("canvas_project_file:"+id, false, "error", "canvas project file points to a directory: "+id+"."+key)
		} else {
			addCheck("canvas_project_file:"+id, true, "info", "canvas project file exists: "+id+"."+key)
		}
	}
}

func checkWorkbenchLogs(root string, addCheck doctorAddCheck, stats *secretRefStats) error {
	return forEachObject(root, "workbench-logs", "workbench-log.json", func(id string, envelope doctorEnvelope) error {
		var data WorkbenchLogData
		if len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				addCheck("workbench_log:"+id, false, "error", "workbench log data is not valid json: "+id)
				return nil
			}
			if err := validateWorkbenchLogData(prepareWorkbenchLogData(data)); err != nil {
				addCheck("workbench_log:"+id, false, "error", err.Error()+": "+id)
			} else {
				addCheck("workbench_log:"+id, true, "info", "workbench log data is valid: "+id)
			}
			checkWorkbenchLogMediaFiles(root, id, data.Media, addCheck)
		}
		inspectSecretRefs(envelope.Data, "workbench_log "+id, addCheck, stats)
		return nil
	})
}

func checkWorkbenchLogMediaFiles(root string, id string, media []WorkbenchLogMedia, addCheck doctorAddCheck) {
	for _, item := range media {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		if !isWorkspaceRelativeFile(item.Path) || !strings.HasPrefix(filepath.ToSlash(filepath.Clean(item.Path)), "files/") {
			addCheck("workbench_log_file:"+id, false, "error", "workbench log media path escapes files directory: "+id+"."+item.Key)
			continue
		}
		path := filepath.Join(root, "workbench-logs", id, filepath.FromSlash(item.Path))
		if stat, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				addCheck("workbench_log_file:"+id, false, "warning", "workbench log media file is missing: "+id+"."+item.Key)
				continue
			}
			addCheck("workbench_log_file:"+id, false, "warning", "workbench log media file is not accessible: "+id+"."+item.Key)
		} else if stat.IsDir() {
			addCheck("workbench_log_file:"+id, false, "error", "workbench log media file points to a directory: "+id+"."+item.Key)
		} else {
			addCheck("workbench_log_file:"+id, true, "info", "workbench log media file exists: "+id+"."+item.Key)
		}
	}
}

func checkExportPlan(workspace Workspace, addCheck doctorAddCheck) error {
	plan, err := BuildExportPlan(workspace, ExportPlanOptions{})
	if err != nil {
		return err
	}
	if len(plan.Warnings) == 0 {
		addCheck("export_rules", true, "info", "default export rules evaluated")
	} else {
		for _, warning := range plan.Warnings {
			addCheck("export_rules", false, "warning", warning)
		}
	}
	return nil
}

func checkGCPlan(workspace Workspace, addCheck doctorAddCheck) error {
	plan, err := BuildGCPlan(workspace, GCPlanOptions{})
	if err != nil {
		return err
	}
	if len(plan.Warnings) > 0 {
		for _, warning := range plan.Warnings {
			addCheck("gc_rules", false, "warning", warning)
		}
		return nil
	}
	if len(plan.Candidates) == 0 {
		addCheck("gc_rules", true, "info", "gc dry-run found no cleanup candidates")
		return nil
	}
	addCheck("gc_rules", false, "warning", "gc dry-run found cleanup candidates: "+strconv.Itoa(len(plan.Candidates)))
	return nil
}

func forEachObject(root string, collection string, fileName string, fn func(id string, envelope doctorEnvelope) error) error {
	dir := filepath.Join(root, collection)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return WrapError(ErrorInternal, "read workspace collection", 5, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := validatePathComponent("object id", id); err != nil {
			continue
		}
		envelope, err := readDoctorEnvelope(filepath.Join(dir, id, fileName))
		if err != nil {
			continue
		}
		if err := fn(id, envelope); err != nil {
			return err
		}
	}
	return nil
}

func readDoctorEnvelope(path string) (doctorEnvelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return doctorEnvelope{}, err
	}
	var envelope doctorEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return doctorEnvelope{}, err
	}
	return envelope, nil
}

func addObjectRefCheck(root string, name string, id string, collection string, fileName string, addCheck doctorAddCheck) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if err := validatePathComponent("object id", id); err != nil {
		addCheck(name, false, "error", "object reference id is not path safe: "+id)
		return
	}
	path := filepath.Join(root, collection, id, fileName)
	if stat, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			addCheck(name, false, "error", "broken reference: "+name+" -> "+id)
			return
		}
		addCheck(name, false, "warning", "reference is not accessible: "+name+" -> "+id)
	} else if stat.IsDir() {
		addCheck(name, false, "error", "reference points to a directory: "+name+" -> "+id)
	} else {
		addCheck(name, true, "info", "reference exists: "+name+" -> "+id)
	}
}

func runArtifactIDs(rawRefs []json.RawMessage) []string {
	ids := []string{}
	for _, raw := range rawRefs {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil {
			if strings.TrimSpace(id) != "" {
				ids = append(ids, id)
			}
			continue
		}
		var ref struct {
			ArtifactID string `json:"artifactId"`
			ID         string `json:"id"`
		}
		if err := json.Unmarshal(raw, &ref); err == nil {
			if strings.TrimSpace(ref.ArtifactID) != "" {
				ids = append(ids, ref.ArtifactID)
			} else if strings.TrimSpace(ref.ID) != "" {
				ids = append(ids, ref.ID)
			}
		}
	}
	return ids
}

func inspectSecretRefs(raw json.RawMessage, scope string, addCheck doctorAddCheck, stats *secretRefStats) {
	if len(raw) == 0 {
		return
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for key, value := range object {
			if key == "secretRef" {
				stats.Found++
				var ref SecretRef
				if err := json.Unmarshal(value, &ref); err != nil {
					stats.Problems++
					addCheck("secret_ref:"+scope, false, "error", "secretRef is not valid json: "+scope)
					continue
				}
				if err := ref.Validate(); err != nil {
					stats.Problems++
					addCheck("secret_ref:"+scope, false, "error", err.Error()+": "+scope)
					continue
				}
				continue
			}
			if isPlaintextSecretKey(key) {
				stats.Problems++
				addCheck("secret_ref:"+scope, false, "error", "plaintext secret field is not allowed: "+scope+"."+key)
				continue
			}
			inspectSecretRefs(value, scope+"."+key, addCheck, stats)
		}
		return
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		for _, value := range array {
			inspectSecretRefs(value, scope+"[]", addCheck, stats)
		}
	}
}

func isPlaintextSecretKey(key string) bool {
	switch strings.ToLower(key) {
	case "apikey", "api_key", "secret", "token", "password":
		return true
	default:
		return false
	}
}
