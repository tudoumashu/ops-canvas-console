package localworkspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitCreatesContractLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	fixed := time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC)
	result, err := Init(InitOptions{Path: root, Name: "Test Workspace", Now: func() time.Time { return fixed }})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !result.Created {
		t.Fatal("Init() Created = false, want true")
	}
	if result.Workspace.Document.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion = %q", result.Workspace.Document.SchemaVersion)
	}
	if result.Workspace.Document.Kind != KindWorkspace {
		t.Fatalf("kind = %q", result.Workspace.Document.Kind)
	}
	if !strings.HasPrefix(result.Workspace.Document.ID, "ws_") {
		t.Fatalf("id = %q, want ws_ prefix", result.Workspace.Document.ID)
	}
	if result.Workspace.Document.Revision != 1 {
		t.Fatalf("revision = %d, want 1", result.Workspace.Document.Revision)
	}
	if result.Workspace.Document.Data.Name != "Test Workspace" {
		t.Fatalf("name = %q", result.Workspace.Document.Data.Name)
	}
	for _, dir := range RequiredDirs {
		stat, err := os.Stat(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("required dir %s missing: %v", dir, err)
		}
		if !stat.IsDir() {
			t.Fatalf("required path %s is not dir", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("index placeholder missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, WorkspaceFileName))
	if err != nil {
		t.Fatalf("read workspace file: %v", err)
	}
	var document WorkspaceDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("workspace file is not json: %v", err)
	}
	if document.Data.Name != "Test Workspace" {
		t.Fatalf("workspace file name = %q", document.Data.Name)
	}
}

func TestInitIsIdempotentAndDoesNotOverwriteWorkspaceID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	first, err := Init(InitOptions{Path: root, Name: "First"})
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	second, err := Init(InitOptions{Path: root, Name: "Second"})
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	if second.Created {
		t.Fatal("second Init() Created = true, want false")
	}
	if second.Workspace.Document.ID != first.Workspace.Document.ID {
		t.Fatalf("workspace id changed: %q -> %q", first.Workspace.Document.ID, second.Workspace.Document.ID)
	}
	if second.Workspace.Document.Data.Name != "First" {
		t.Fatalf("workspace name overwritten: %q", second.Workspace.Document.Data.Name)
	}
}

func TestOpenDoesNotCreateWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	_, err := Open(root)
	if err == nil {
		t.Fatal("Open() error = nil, want workspace_not_found")
	}
	var workspaceErr *Error
	if !asWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceNotFound {
		t.Fatalf("Open() error = %#v, want workspace_not_found", err)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("Open() created workspace root, stat err = %v", statErr)
	}
}

func TestDoctorReportsMissingRequiredDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Init(InitOptions{Path: root}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runs")); err != nil {
		t.Fatalf("remove runs dir: %v", err)
	}
	report, err := Doctor(DoctorOptions{Path: root})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.OK {
		t.Fatal("Doctor() OK = true, want false")
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "dir:runs" && !check.OK && check.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing runs dir check not found: %#v", report.Checks)
	}
}

func TestDoctorReportsBrokenRefsSecretRefsAndProjectRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	document := result.Workspace.Document
	document.Data.DefaultProfileID = "profile_missing"
	if err := AtomicWriteJSON(filepath.Join(root, WorkspaceFileName), document, 0o600); err != nil {
		t.Fatalf("write workspace default profile: %v", err)
	}

	missingProjectRoot := filepath.Join(t.TempDir(), "missing-project-root")
	writeTestObject(t, root, "profiles", "profile_bad", "profile.json", "profile", map[string]any{
		"name": "bad profile",
		"channels": []map[string]any{
			{
				"id":      "channel_plain",
				"enabled": true,
				"secretRef": map[string]any{
					"type": "literal",
					"name": "SHOULD_NOT_PRINT",
				},
				"apiKey": "not-a-real-secret",
			},
		},
	})
	writeTestObject(t, root, "projects", "proj_bad", "project.json", "project", map[string]any{
		"name":     "bad project",
		"rootPath": missingProjectRoot,
	})
	writeTestObject(t, root, "runs", "run_bad", "run.json", "run", map[string]any{
		"templateId": "tpl_missing",
		"profileId":  "profile_missing_2",
		"projectId":  "proj_missing",
		"artifactRefs": []any{
			"art_missing",
			map[string]any{"artifactId": "art_missing_from_object"},
		},
	})
	writeTestObjectAt(t, filepath.Join(root, "runs", "run_bad", "artifacts", "art_missing_ref.ref.json"), "art_missing_ref", "run_artifact_ref", map[string]any{
		"artifactId": "art_missing_from_ref",
	})
	writeTestObject(t, root, "assets", "asset_bad", "asset.json", "asset", map[string]any{
		"type":             "image",
		"sourceArtifactId": "art_missing_from_asset",
		"files": map[string]string{
			"original": "../outside.png",
		},
	})
	writeTestObject(t, root, "prompts", "prompt_bad", "prompt.json", "prompt", map[string]any{
		"title": "bad prompt",
	})
	writeTestObject(t, root, "workbench-logs", "wblog_bad", "workbench-log.json", "workbench_log", map[string]any{
		"modality": "image",
		"media": []map[string]any{
			{"key": "original", "path": "../outside.png"},
		},
		"payload": map[string]any{"apiKey": "not-a-real-secret"},
	})
	writeTestObject(t, root, "canvas-projects", "canvas_bad", "canvas-project.json", "canvas_project", map[string]any{
		"title":          "bad canvas",
		"nodes":          []any{},
		"connections":    []any{},
		"chatSessions":   []any{},
		"activeChatId":   nil,
		"backgroundMode": "lines",
		"viewport":       map[string]any{"x": 0, "y": 0, "k": 1},
		"files": map[string]any{
			"original": map[string]any{"path": "../outside.png"},
		},
	})

	report, err := Doctor(DoctorOptions{Path: root})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.OK {
		t.Fatal("Doctor() OK = true, want false")
	}
	joined := strings.Join(append(report.Errors, report.Warnings...), "\n")
	wants := []string{
		"broken reference: ref:workspace.defaultProfileId -> profile_missing",
		"broken reference: ref:run.templateId -> tpl_missing",
		"broken reference: ref:run.artifactRefs -> art_missing",
		"broken reference: ref:run_artifact_ref.artifactId -> art_missing_from_ref",
		"broken reference: ref:asset.sourceArtifactId -> art_missing_from_asset",
		"asset file path escapes asset directory: asset_bad.original",
		"prompt content.md is missing: prompt_bad",
		"workbench log media path escapes files directory: wblog_bad.original",
		"plaintext secret field is not allowed: workbench_log wblog_bad.payload.apiKey",
		"secretRef type is not allowed: profile profile_bad.channels[]",
		"plaintext secret field is not allowed: profile profile_bad.channels[].apiKey",
		"project root is not accessible: proj_bad",
		"canvas project file path escapes files directory: canvas_bad.original",
	}
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Fatalf("Doctor() diagnostics missing %q:\n%s", want, joined)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if strings.Contains(string(encoded), missingProjectRoot) {
		t.Fatalf("Doctor() leaked project root without ShowPath: %s", encoded)
	}
	if strings.Contains(string(encoded), "not-a-real-secret") || strings.Contains(string(encoded), "SHOULD_NOT_PRINT") {
		t.Fatalf("Doctor() leaked secret material: %s", encoded)
	}
}

func TestDoctorReportsIndexFreshnessStaleExecutorAndHybridRepair(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	writeTestObject(t, root, "templates", "tpl_hybrid", "template.json", "template", map[string]any{
		"title": "Hybrid template",
		"metadata": map[string]any{
			hybridEcommerceKey: map[string]any{"backend": hybridEcommerceBackend},
		},
	})
	writeTestObject(t, root, "runs", "run_hybrid", "run.json", "run", map[string]any{
		"templateId": "tpl_hybrid",
		"status":     RunStatusPending,
	})
	if _, err := AppendRunEvent(workspace, "run_hybrid", RunEventInput{
		Type:    "run.waiting_for_executor",
		Level:   "info",
		Actor:   RunEventActor{Type: "web", ID: "test"},
		Message: "run created",
	}); err != nil {
		t.Fatalf("AppendRunEvent() error = %v", err)
	}
	if _, err := workspace.StateDir(); err != nil {
		t.Fatalf("StateDir() error = %v", err)
	}
	lock, err := AcquireLock(workspace.LockPath(executorWatchLock))
	if err != nil {
		t.Fatalf("AcquireLock(executor watch) error = %v", err)
	}
	defer lock.Release()
	if err := writeExecutorRuntimeFiles(workspace, ExecutorRuntimeMetadata{
		SchemaVersion: SchemaVersion,
		PID:           999999,
		WorkspaceID:   workspace.Document.ID,
		Mode:          "watch",
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		HeartbeatAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("writeExecutorRuntimeFiles() error = %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, IndexFileName), old, old); err != nil {
		t.Fatalf("chtimes index: %v", err)
	}

	report, err := Doctor(DoctorOptions{Path: root})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	joined := strings.Join(append(report.Errors, report.Warnings...), "\n")
	wants := []string{
		"index.sqlite may be stale; run opsc workspace index rebuild",
		"executor worker runtime is stale; restart opsc executor --watch",
		"hybrid run is waiting for executor: run_hybrid; start opsc executor --watch",
	}
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Fatalf("Doctor() diagnostics missing %q:\n%s", want, joined)
		}
	}
}

func TestAcquireLockPreventsSecondLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	defer first.Release()
	_, err = AcquireLock(path)
	if err == nil {
		t.Fatal("second AcquireLock() error = nil, want workspace_locked")
	}
	var workspaceErr *Error
	if !asWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceLocked {
		t.Fatalf("second AcquireLock() error = %#v, want workspace_locked", err)
	}
}

func TestRepositoryWritesEnvelopeWithAtomicJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	type templateData struct {
		Title string `json:"title"`
	}
	repo := Repository[templateData]{
		Workspace:  result.Workspace,
		Collection: "templates",
		FileName:   "template.json",
		Kind:       "template",
		IDPrefix:   "tpl",
		Now: func() time.Time {
			return time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC)
		},
	}
	document, err := repo.New(templateData{Title: "Local Template"})
	if err != nil {
		t.Fatalf("Repository.New() error = %v", err)
	}
	if err := repo.Write(document); err != nil {
		t.Fatalf("Repository.Write() error = %v", err)
	}
	read, err := repo.Read(document.ID)
	if err != nil {
		t.Fatalf("Repository.Read() error = %v", err)
	}
	if read.Kind != "template" || read.Data.Title != "Local Template" {
		t.Fatalf("read document = %#v", read)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", document.ID, "template.json")); err != nil {
		t.Fatalf("repository file missing: %v", err)
	}
}

func TestWorkflowObjectRepositoriesPersistTemplatesRunsAndArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace

	template, err := NewTemplate(workspace, TemplateData{Title: "Local Template", WorkflowType: "generic", Version: 1})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	artifact, err := NewArtifact(workspace, ArtifactData{
		Type:    "image",
		MIME:    "image/png",
		Title:   "Generated Image",
		Bytes:   123,
		Width:   64,
		Height:  32,
		Privacy: "private",
		Files: map[string]string{
			"original":  "original.png",
			"thumbnail": "thumb.webp",
		},
	})
	if err != nil {
		t.Fatalf("NewArtifact() error = %v", err)
	}
	if err := WriteArtifact(workspace, artifact); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	run, err := NewRun(workspace, RunData{
		TemplateID: template.ID,
		Status:     RunStatusRunning,
		ArtifactRefs: []RunArtifactRefData{
			{ArtifactID: artifact.ID, Role: "primary_output", Slot: "output", Order: 1},
		},
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	ref := testEnvelope(artifact.ID, KindRunArtifactRef, RunArtifactRefData{
		ArtifactID: artifact.ID,
		Role:       "primary_output",
		Slot:       "output",
		Order:      1,
	})
	if err := WriteRunArtifactRef(workspace, run.ID, ref); err != nil {
		t.Fatalf("WriteRunArtifactRef() error = %v", err)
	}

	templates, err := ListTemplates(workspace)
	if err != nil {
		t.Fatalf("ListTemplates() error = %v", err)
	}
	if len(templates) != 1 || templates[0].Data.Title != "Local Template" {
		t.Fatalf("ListTemplates() = %#v", templates)
	}
	runs, err := ListRuns(workspace)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Data.Status != RunStatusRunning {
		t.Fatalf("ListRuns() = %#v", runs)
	}
	artifacts, err := ListArtifacts(workspace)
	if err != nil {
		t.Fatalf("ListArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Data.Title != "Generated Image" {
		t.Fatalf("ListArtifacts() = %#v", artifacts)
	}
	runArtifacts, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	if len(runArtifacts) != 1 || runArtifacts[0].Artifact.ID != artifact.ID || runArtifacts[0].Ref.Role != "primary_output" {
		t.Fatalf("ListRunArtifacts() = %#v", runArtifacts)
	}
	count, err := RunArtifactCount(workspace, run.ID)
	if err != nil {
		t.Fatalf("RunArtifactCount() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("RunArtifactCount() = %d, want 1", count)
	}
	if _, err := os.Stat(filepath.Join(root, "runs", run.ID, "template.snapshot.json")); err != nil {
		t.Fatalf("template snapshot missing: %v", err)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != "run.created" {
		t.Fatalf("ReadRunEvents() = %#v, want run.created", events)
	}
}

func TestWorkspaceIndexIncrementalAndRebuildForWorkflowObjects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace

	template, err := NewTemplate(workspace, TemplateData{Title: "Indexed Template", WorkflowType: "generic", Version: 1})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	artifact, err := NewArtifact(workspace, ArtifactData{
		Type:    "image",
		MIME:    "image/png",
		Title:   "Indexed Artifact",
		Bytes:   456,
		Width:   128,
		Height:  64,
		Privacy: "private",
		Files: map[string]string{
			"original": "original.png",
		},
	})
	if err != nil {
		t.Fatalf("NewArtifact() error = %v", err)
	}
	if err := WriteArtifact(workspace, artifact); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	run, err := NewRun(workspace, RunData{TemplateID: template.ID, Status: RunStatusRunning})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := SaveRun(workspace, run, SaveRunOptions{TemplateSnapshot: &template}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	node, err := NewRunNodeState("image_generation_may1", RunNodeStateData{Status: RunStatusRunning})
	if err != nil {
		t.Fatalf("NewRunNodeState() error = %v", err)
	}
	if err := WriteRunNodeState(workspace, run.ID, node); err != nil {
		t.Fatalf("WriteRunNodeState() error = %v", err)
	}
	ref := testEnvelope(artifact.ID, KindRunArtifactRef, RunArtifactRefData{
		ArtifactID: artifact.ID,
		Role:       "primary_output",
		NodeID:     "image_generation_may1",
		Slot:       "output",
		Order:      0,
	})
	if err := WriteRunArtifactRef(workspace, run.ID, ref); err != nil {
		t.Fatalf("WriteRunArtifactRef() error = %v", err)
	}
	if _, err := AppendRunEvent(workspace, run.ID, RunEventInput{
		Type:    "run.progress",
		Level:   "info",
		Actor:   RunEventActor{Type: "cli", ID: "opsc"},
		Message: "node running",
	}); err != nil {
		t.Fatalf("AppendRunEvent() error = %v", err)
	}

	assertIndexedWorkflowObjects(t, workspace, template.ID, run.ID, artifact.ID)
	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	assertIndexedWorkflowObjects(t, workspace, template.ID, run.ID, artifact.ID)
}

func TestPrivateObjectRepositoriesPersistIndexAndExportPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace

	artifact, err := NewArtifact(workspace, ArtifactData{
		Type:    "image",
		MIME:    "image/png",
		Title:   "Source Artifact",
		Privacy: PrivacyPrivate,
		Files: map[string]string{
			"original": "original.png",
		},
	})
	if err != nil {
		t.Fatalf("NewArtifact() error = %v", err)
	}
	if err := WriteArtifact(workspace, artifact); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	profile, err := NewProfile(workspace, ProfileData{
		Name: "Local Profile",
		Mode: ProfileModeHybrid,
		Channels: []ProfileChannel{
			{
				ID:       "openai",
				Protocol: "openai-compatible",
				Enabled:  true,
				SecretRef: &SecretRef{
					Type: SecretRefTypeEnv,
					Name: "OPENAI_API_KEY",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	projectRoot := t.TempDir()
	project, err := NewProject(workspace, ProjectData{
		Name:     "Local Project",
		Kind:     "video",
		Adapter:  "filesystem",
		RootPath: projectRoot,
		Capabilities: ProjectCapabilities{
			FSRead:        true,
			ArtifactWrite: true,
		},
		AdapterMetadata: map[string]any{
			"adapterVersion": "test",
		},
		CredentialRefs: map[string]SecretRef{
			"git": {
				Type: SecretRefTypeEnv,
				Name: "GIT_TOKEN",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if project.Data.RootFingerprint == "" || strings.Contains(project.Data.RootFingerprint, projectRoot) {
		t.Fatalf("project rootFingerprint leaked or missing: %q", project.Data.RootFingerprint)
	}
	if err := WriteProject(workspace, project); err != nil {
		t.Fatalf("WriteProject() error = %v", err)
	}
	asset, err := NewAsset(workspace, AssetData{
		Type:             "image",
		MIME:             "image/png",
		Title:            "Saved Asset",
		MediaType:        "image",
		CategoryPath:     "local/test",
		Purpose:          "reference",
		Source:           "workspace",
		SourceArtifactID: artifact.ID,
		Privacy:          PrivacyPrivate,
		Tags:             []string{"favorite"},
		Files: map[string]string{
			"original": "files/original.png",
		},
	})
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	if err := WriteAsset(workspace, asset); err != nil {
		t.Fatalf("WriteAsset() error = %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(AssetRepository(workspace).Dir(asset.ID), "files", "original.png"), []byte("png"), 0o600); err != nil {
		t.Fatalf("write asset file: %v", err)
	}
	prompt, err := NewPrompt(workspace, PromptData{
		Title:    "Prompt",
		Kind:     "system",
		Category: "agent",
		Domain:   "workflow",
		Stage:    "draft",
		Provider: "local",
		Model:    "gpt-test",
		Mode:     "chat",
		Status:   "active",
		Privacy:  PrivacyPrivate,
		Tags:     []string{"agent"},
	})
	if err != nil {
		t.Fatalf("NewPrompt() error = %v", err)
	}
	if err := SavePrompt(workspace, prompt, "Use local workspace."); err != nil {
		t.Fatalf("SavePrompt() error = %v", err)
	}

	assertIndexedPrivateObjects(t, workspace, profile.ID, project.ID, asset.ID, prompt.ID)
	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	assertIndexedPrivateObjects(t, workspace, profile.ID, project.ID, asset.ID, prompt.ID)

	fileSecretProfile, err := NewProfile(workspace, ProfileData{
		Name: "File Secret Profile",
		Mode: ProfileModeLocal,
		Channels: []ProfileChannel{
			{
				ID: "local_file_secret",
				SecretRef: &SecretRef{
					Type: SecretRefTypeFile,
					Path: "/private/opsc/local.key",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProfile() file secret error = %v", err)
	}
	if err := WriteProfile(workspace, fileSecretProfile); err != nil {
		t.Fatalf("WriteProfile() file secret error = %v", err)
	}

	plan, err := BuildExportPlan(workspace, ExportPlanOptions{})
	if err != nil {
		t.Fatalf("BuildExportPlan() error = %v", err)
	}
	included := map[string]bool{}
	for _, path := range plan.IncludePaths {
		included[path] = true
		if filepath.IsAbs(path) {
			t.Fatalf("export include path is absolute: %q", path)
		}
	}
	if !included[filepath.ToSlash(filepath.Join("prompts", prompt.ID, "content.md"))] {
		t.Fatalf("export plan missing prompt content: %#v", plan.IncludePaths)
	}
	if !included[filepath.ToSlash(filepath.Join("assets", asset.ID, "files", "original.png"))] {
		t.Fatalf("export plan missing asset file: %#v", plan.IncludePaths)
	}
	excluded := map[string]string{}
	for _, item := range plan.ExcludePaths {
		excluded[item.Path] = item.Reason
		if filepath.IsAbs(item.Path) {
			t.Fatalf("export exclude path is absolute: %q", item.Path)
		}
	}
	if _, ok := excluded[IndexFileName]; !ok {
		t.Fatalf("export plan did not exclude index.sqlite: %#v", plan.ExcludePaths)
	}
	if _, ok := excluded[".opsc"]; !ok {
		t.Fatalf("export plan did not exclude workspace control directory: %#v", plan.ExcludePaths)
	}
	projectDoc := filepath.ToSlash(filepath.Join("projects", project.ID, "project.json"))
	if _, ok := excluded[projectDoc]; !ok {
		t.Fatalf("export plan did not exclude local project doc: %#v", plan.ExcludePaths)
	}
	fileSecretProfileDoc := filepath.ToSlash(filepath.Join("profiles", fileSecretProfile.ID, "profile.json"))
	if _, ok := excluded[fileSecretProfileDoc]; !ok {
		t.Fatalf("export plan did not exclude file secret profile: %#v", plan.ExcludePaths)
	}
}

func TestFollowRunEventsEmitsHistoricalAndAppendedEvents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	run, err := NewRun(workspace, RunData{Status: RunStatusRunning})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := []string{}
	err = FollowRunEvents(ctx, workspace, run.ID, 0, 5*time.Millisecond, func(event RunEventEnvelope) error {
		seen = append(seen, event.Type)
		if len(seen) == 1 {
			_, err := AppendRunEvent(workspace, run.ID, RunEventInput{
				Type:    "run.progress",
				Level:   "info",
				Actor:   RunEventActor{Type: "cli", ID: "opsc"},
				Message: "progress",
			})
			if err != nil {
				return err
			}
		}
		if len(seen) == 2 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("FollowRunEvents() error = %v", err)
	}
	if strings.Join(seen, ",") != "run.created,run.progress" {
		t.Fatalf("events = %#v, want historical and appended events", seen)
	}
}

func TestWorkflowObjectRepositoriesValidateRunAndArtifactData(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	if _, err := NewRun(workspace, RunData{Status: "idle"}); err == nil {
		t.Fatal("NewRun() invalid status error = nil, want error")
	}
	invalidArtifacts := []ArtifactData{
		{Type: "image", Files: map[string]string{"original": "../secret.png"}},
		{Type: "image", Files: map[string]string{"original": "/tmp/secret.png"}},
		{Type: "image", Privacy: "team"},
		{Type: "unknown"},
	}
	for _, data := range invalidArtifacts {
		if _, err := NewArtifact(workspace, data); err == nil {
			t.Fatalf("NewArtifact(%#v) error = nil, want error", data)
		}
	}
}

func TestPrivateObjectValidationRejectsSecretsAndEscapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	if _, err := NewProfile(workspace, ProfileData{
		Name: "bad profile",
		Mode: ProfileModeLocal,
		Channels: []ProfileChannel{
			{
				ID: "openai",
				Metadata: map[string]any{
					"apiKey": "not-a-real-secret",
				},
			},
		},
	}); err == nil {
		t.Fatal("NewProfile() plaintext secret error = nil, want error")
	}
	if _, err := NewProject(workspace, ProjectData{Name: "bad project", RootPath: "relative/path"}); err == nil {
		t.Fatal("NewProject() relative rootPath error = nil, want error")
	}
	if _, err := NewProject(workspace, ProjectData{
		Name: "bad glob",
		Execution: ProjectExecution{
			AllowGlobs: []string{"../outside"},
		},
	}); err == nil {
		t.Fatal("NewProject() escaping glob error = nil, want error")
	}
	if _, err := NewProject(workspace, ProjectData{
		Name: "bad credential ref",
		CredentialRefs: map[string]SecretRef{
			"bad/name": {
				Type: SecretRefTypeEnv,
				Name: "TOKEN",
			},
		},
	}); err == nil {
		t.Fatal("NewProject() unsafe credential ref name error = nil, want error")
	}
	if _, err := NewProject(workspace, ProjectData{
		Name: "bad project secret",
		AdapterMetadata: map[string]any{
			"token": "not-a-real-secret",
		},
	}); err == nil {
		t.Fatal("NewProject() plaintext project secret error = nil, want error")
	}
	if _, err := NewAsset(workspace, AssetData{
		Type:  "image",
		Files: map[string]string{"original": "../outside.png"},
	}); err == nil {
		t.Fatal("NewAsset() escaping file error = nil, want error")
	}
	if _, err := NewPrompt(workspace, PromptData{Title: "", Metadata: map[string]any{"token": "not-a-real-secret"}}); err == nil {
		t.Fatal("NewPrompt() invalid data error = nil, want error")
	}
}

func TestProjectRootFingerprintAndPathAccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "allowed"), 0o755); err != nil {
		t.Fatalf("create allowed dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatalf("create node_modules dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "allowed", "input.txt"), []byte("input"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "node_modules", "pkg", "file.txt"), []byte("pkg"), 0o600); err != nil {
		t.Fatalf("write node_modules file: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	project, err := NewProject(workspace, ProjectData{
		Name:     "Safe Project",
		RootPath: projectRoot,
		Capabilities: ProjectCapabilities{
			FSRead: true,
		},
	})
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if project.Data.RootFingerprint == "" || strings.Contains(project.Data.RootFingerprint, projectRoot) {
		t.Fatalf("rootFingerprint leaked or missing: %q", project.Data.RootFingerprint)
	}
	stableFingerprint, err := ProjectRootFingerprint(workspace, projectRoot)
	if err != nil {
		t.Fatalf("ProjectRootFingerprint() error = %v", err)
	}
	if stableFingerprint != project.Data.RootFingerprint {
		t.Fatalf("rootFingerprint unstable: %q != %q", stableFingerprint, project.Data.RootFingerprint)
	}
	if err := WriteProject(workspace, project); err != nil {
		t.Fatalf("WriteProject() error = %v", err)
	}

	resolved, err := ResolveProjectPath(workspace, project, ProjectPathRequest{Operation: ProjectPathRead, Path: "allowed/input.txt"})
	if err != nil {
		t.Fatalf("ResolveProjectPath() read error = %v", err)
	}
	if resolved.RelativePath != "allowed/input.txt" || !strings.HasPrefix(resolved.Path, projectRoot) {
		t.Fatalf("ResolveProjectPath() = %#v", resolved)
	}
	badRequests := []ProjectPathRequest{
		{Operation: ProjectPathRead, Path: "../outside.txt"},
		{Operation: ProjectPathRead, Path: ".env"},
		{Operation: ProjectPathRead, Path: "node_modules/pkg/file.txt"},
		{Operation: ProjectPathWrite, Path: "allowed/output.txt"},
		{Operation: ProjectPathExec, Path: "allowed/input.txt"},
	}
	for _, request := range badRequests {
		if _, err := ResolveProjectPath(workspace, project, request); err == nil {
			t.Fatalf("ResolveProjectPath(%#v) error = nil, want error", request)
		}
	}
	linkPath := filepath.Join(projectRoot, "allowed", "link.txt")
	if err := os.Symlink(outside, linkPath); err == nil {
		if _, err := ResolveProjectPath(workspace, project, ProjectPathRequest{Operation: ProjectPathRead, Path: "allowed/link.txt"}); err == nil {
			t.Fatal("ResolveProjectPath() symlink escape error = nil, want error")
		}
	}
}

func TestGCPlanIsDryRunAndRelative(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	referenced, err := NewArtifact(workspace, ArtifactData{Type: "image", Title: "Referenced", Privacy: PrivacyPrivate})
	if err != nil {
		t.Fatalf("NewArtifact() referenced error = %v", err)
	}
	if err := WriteArtifact(workspace, referenced); err != nil {
		t.Fatalf("WriteArtifact() referenced error = %v", err)
	}
	orphan, err := NewArtifact(workspace, ArtifactData{Type: "image", Title: "Orphan", Privacy: PrivacyPrivate})
	if err != nil {
		t.Fatalf("NewArtifact() orphan error = %v", err)
	}
	if err := WriteArtifact(workspace, orphan); err != nil {
		t.Fatalf("WriteArtifact() orphan error = %v", err)
	}
	run, err := NewRun(workspace, RunData{
		Status: RunStatusRunning,
		ArtifactRefs: []RunArtifactRefData{
			{ArtifactID: referenced.ID},
			{ArtifactID: "art_01K1N2Q1A7V8M9P0Q1R2S3T4V5"},
		},
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	asset, err := NewAsset(workspace, AssetData{
		Type:             "image",
		SourceArtifactID: referenced.ID,
		Files: map[string]string{
			"original": "files/missing.png",
		},
	})
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	if err := WriteAsset(workspace, asset); err != nil {
		t.Fatalf("WriteAsset() error = %v", err)
	}
	prompt, err := NewPrompt(workspace, PromptData{Title: "No Content"})
	if err != nil {
		t.Fatalf("NewPrompt() error = %v", err)
	}
	if err := WritePrompt(workspace, prompt); err != nil {
		t.Fatalf("WritePrompt() error = %v", err)
	}
	workbenchLog, err := NewWorkbenchLog(workspace, WorkbenchLogData{
		Modality: WorkbenchModalityImage,
		Media: []WorkbenchLogMedia{
			{Key: "original", Path: "files/missing.png"},
		},
	})
	if err != nil {
		t.Fatalf("NewWorkbenchLog() error = %v", err)
	}
	if err := WriteWorkbenchLog(workspace, workbenchLog); err != nil {
		t.Fatalf("WriteWorkbenchLog() error = %v", err)
	}
	canvasProject, err := NewCanvasProject(workspace, CanvasProjectData{
		Title:          "Missing File Canvas",
		BackgroundMode: CanvasBackgroundLines,
		Viewport:       CanvasProjectViewport{K: 1},
		Nodes: []CanvasProjectNodeData{
			{ID: "node_image", Type: "image", Title: "Image", Position: CanvasPosition{}, Width: 100, Height: 100},
		},
		Files: map[string]CanvasProjectFile{
			"node_image": {Path: "files/missing.png"},
		},
	})
	if err != nil {
		t.Fatalf("NewCanvasProject() error = %v", err)
	}
	if err := WriteCanvasProject(workspace, canvasProject); err != nil {
		t.Fatalf("WriteCanvasProject() error = %v", err)
	}

	plan, err := BuildGCPlan(workspace, GCPlanOptions{})
	if err != nil {
		t.Fatalf("BuildGCPlan() error = %v", err)
	}
	kinds := map[string]bool{}
	for _, candidate := range plan.Candidates {
		kinds[candidate.Kind] = true
		if candidate.Action != "review" {
			t.Fatalf("candidate action = %q, want review: %#v", candidate.Action, candidate)
		}
		if filepath.IsAbs(candidate.Path) || strings.Contains(candidate.Path, root) {
			t.Fatalf("candidate path leaked absolute workspace path: %#v", candidate)
		}
	}
	for _, kind := range []string{"artifact", "run_artifact_ref", "asset_file", "prompt_content", "workbench_log_file", "canvas_project_file"} {
		if !kinds[kind] {
			t.Fatalf("GC candidates missing kind %q: %#v", kind, plan.Candidates)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "artifacts", orphan.ID, "artifact.json")); err != nil {
		t.Fatalf("BuildGCPlan() mutated orphan artifact: %v", err)
	}
}

func assertIndexedWorkflowObjects(t *testing.T, workspace Workspace, templateID string, runID string, artifactID string) {
	t.Helper()
	templates, err := ListTemplateSummaries(workspace)
	if err != nil {
		t.Fatalf("ListTemplateSummaries() error = %v", err)
	}
	if len(templates) != 1 || templates[0].ID != templateID || templates[0].Title != "Indexed Template" {
		t.Fatalf("ListTemplateSummaries() = %#v", templates)
	}
	runs, err := ListRunSummaries(workspace)
	if err != nil {
		t.Fatalf("ListRunSummaries() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != runID || runs[0].ArtifactCount != 1 || runs[0].LatestEventSequence != 2 {
		t.Fatalf("ListRunSummaries() = %#v", runs)
	}
	status, err := GetRunStatus(workspace, runID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if status.Run.ID != runID || len(status.Nodes) != 1 || status.Nodes[0].NodeID != "image_generation_may1" {
		t.Fatalf("GetRunStatus() = %#v", status)
	}
	artifacts, err := ListRunArtifactSummaries(workspace, runID)
	if err != nil {
		t.Fatalf("ListRunArtifactSummaries() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Artifact.ID != artifactID || artifacts[0].Ref.NodeID != "image_generation_may1" {
		t.Fatalf("ListRunArtifactSummaries() = %#v", artifacts)
	}
}

func assertIndexedPrivateObjects(t *testing.T, workspace Workspace, profileID string, projectID string, assetID string, promptID string) {
	t.Helper()
	profiles, err := ListProfileSummaries(workspace)
	if err != nil {
		t.Fatalf("ListProfileSummaries() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != profileID || profiles[0].ChannelCount != 1 {
		t.Fatalf("ListProfileSummaries() = %#v", profiles)
	}
	projects, err := ListProjectSummaries(workspace)
	if err != nil {
		t.Fatalf("ListProjectSummaries() error = %v", err)
	}
	if len(projects) != 1 || projects[0].ID != projectID || !projects[0].HasRootPath || !projects[0].Capabilities.FSRead {
		t.Fatalf("ListProjectSummaries() = %#v", projects)
	}
	if projects[0].RootFingerprint == "" || strings.HasPrefix(projects[0].RootFingerprint, "/") {
		t.Fatalf("project rootFingerprint summary is invalid: %#v", projects[0])
	}
	assets, err := ListAssetSummaries(workspace)
	if err != nil {
		t.Fatalf("ListAssetSummaries() error = %v", err)
	}
	if len(assets) != 1 || assets[0].ID != assetID || assets[0].Original != "files/original.png" || assets[0].CategoryPath != "local/test" {
		t.Fatalf("ListAssetSummaries() = %#v", assets)
	}
	prompts, err := ListPromptSummaries(workspace)
	if err != nil {
		t.Fatalf("ListPromptSummaries() error = %v", err)
	}
	if len(prompts) != 1 || prompts[0].ID != promptID || !prompts[0].HasContent || prompts[0].Model != "gpt-test" {
		t.Fatalf("ListPromptSummaries() = %#v", prompts)
	}
}

func TestWorkspaceRelativePathRejectsEscapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	inside := filepath.Join(root, "templates", "tpl_01", "template.json")
	rel, err := workspaceRelativePath(root, inside)
	if err != nil {
		t.Fatalf("workspaceRelativePath() inside error = %v", err)
	}
	if rel != "templates/tpl_01/template.json" {
		t.Fatalf("relative path = %q", rel)
	}
	if _, err := workspaceRelativePath(root, filepath.Join(root, "..", "outside.json")); err == nil {
		t.Fatal("workspaceRelativePath() escape error = nil, want error")
	}
}

func TestAtomicWriteJSONDoesNotReplaceExistingFileOnEncodeError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document.json")
	if err := AtomicWriteJSON(path, map[string]string{"status": "old"}, 0o600); err != nil {
		t.Fatalf("initial AtomicWriteJSON() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	err = AtomicWriteJSON(path, map[string]any{"bad": make(chan int)}, 0o600)
	if err == nil {
		t.Fatal("AtomicWriteJSON() error = nil, want encode error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("file changed after failed atomic write:\nbefore=%s\nafter=%s", before, after)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".document.json.*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files not cleaned: %#v", matches)
	}
}

func TestNewIDFormatAndPrefixValidation(t *testing.T) {
	first, err := NewID("run", time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	second, err := NewID("run", time.Date(2026, 5, 29, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second NewID() error = %v", err)
	}
	if !strings.HasPrefix(first, "run_") {
		t.Fatalf("id = %q, want run_ prefix", first)
	}
	suffix := strings.TrimPrefix(first, "run_")
	if len(suffix) != 26 {
		t.Fatalf("suffix length = %d, want 26", len(suffix))
	}
	for _, ch := range suffix {
		if !strings.ContainsRune(crockfordBase32, ch) {
			t.Fatalf("id suffix contains non Crockford Base32 char %q in %q", ch, first)
		}
	}
	if first >= second {
		t.Fatalf("timestamp prefix is not lexically sortable: %q >= %q", first, second)
	}
	if _, err := NewID("bad/prefix", time.Now()); err == nil {
		t.Fatal("NewID() invalid prefix error = nil, want error")
	}
}

func TestSecretRefValidate(t *testing.T) {
	valid := []SecretRef{
		{Type: SecretRefTypeEnv, Name: "OPENAI_API_KEY"},
		{Type: SecretRefTypeKeychain, Service: "opsc", Account: "openai"},
		{Type: SecretRefTypeFile, Path: "/private/opsc/openai.key"},
		{Type: SecretRefTypeCloud, ChannelID: "channel_openai"},
	}
	for _, ref := range valid {
		if err := ref.Validate(); err != nil {
			t.Fatalf("SecretRef.Validate(%#v) error = %v", ref, err)
		}
	}
	invalid := []SecretRef{
		{Type: "plain", Name: "secret"},
		{Type: "literal", Name: "secret"},
		{Type: "value", Name: "secret"},
		{Type: SecretRefTypeEnv},
		{Type: SecretRefTypeKeychain, Service: "opsc"},
		{Type: SecretRefTypeFile},
		{Type: SecretRefTypeCloud},
	}
	for _, ref := range invalid {
		if err := ref.Validate(); err == nil {
			t.Fatalf("SecretRef.Validate(%#v) error = nil, want error", ref)
		}
	}
	fileSummary := SecretRef{Type: SecretRefTypeFile, Path: "/private/opsc/openai.key"}.Summary()
	if !fileSummary.Redacted || fileSummary.Reference != "<file>" || strings.Contains(fileSummary.Reference, "/private") {
		t.Fatalf("SecretRef.Summary() leaked file path: %#v", fileSummary)
	}
}

func TestRepositoryRejectsPathUnsafeIDs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	repo := Repository[map[string]string]{
		Workspace:  result.Workspace,
		Collection: "templates",
		FileName:   "template.json",
		Kind:       "template",
		IDPrefix:   "tpl",
		Now: func() time.Time {
			return time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC)
		},
	}
	if _, err := repo.Read("tpl_../bad"); err == nil {
		t.Fatal("Repository.Read() path escape error = nil, want error")
	}
	if _, err := repo.Read("run_01K1N2Q1A7V8M9P0Q1R2S3T4V5"); err == nil {
		t.Fatal("Repository.Read() wrong prefix error = nil, want error")
	}
	unsafeRepo := repo
	unsafeRepo.FileName = "../template.json"
	if _, err := unsafeRepo.New(map[string]string{"title": "bad"}); err == nil {
		t.Fatal("Repository.New() unsafe file name error = nil, want error")
	}
}

func TestOpenRejectsInvalidWorkspaceSchema(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	document := result.Workspace.Document
	document.SchemaVersion = "wrong-version"
	if err := AtomicWriteJSON(filepath.Join(root, WorkspaceFileName), document, 0o600); err != nil {
		t.Fatalf("write invalid workspace: %v", err)
	}
	_, err = Open(root)
	if err == nil {
		t.Fatal("Open() error = nil, want workspace_invalid")
	}
	var workspaceErr *Error
	if !asWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceInvalid {
		t.Fatalf("Open() error = %#v, want workspace_invalid", err)
	}
}

func TestScanFindsCanonicalObjectsAndSkipsDerivedDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	repo := Repository[map[string]string]{
		Workspace:  result.Workspace,
		Collection: "templates",
		FileName:   "template.json",
		Kind:       "template",
		IDPrefix:   "tpl",
		Now: func() time.Time {
			return time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC)
		},
	}
	document, err := repo.New(map[string]string{"title": "Template"})
	if err != nil {
		t.Fatalf("Repository.New() error = %v", err)
	}
	if err := repo.Write(document); err != nil {
		t.Fatalf("Repository.Write() error = %v", err)
	}
	if err := AtomicWriteJSON(filepath.Join(root, "cache", "cache.json"), document, 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	if err := AtomicWriteJSON(filepath.Join(root, "exports", "export.json"), document, 0o600); err != nil {
		t.Fatalf("write export file: %v", err)
	}
	badID, err := NewID("tpl", time.Date(2026, 5, 29, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("bad id NewID() error = %v", err)
	}
	badDocument := document
	badDocument.ID = badID
	badDocument.SchemaVersion = "bad-version"
	if err := AtomicWriteJSON(filepath.Join(root, "templates", badID, "template.json"), badDocument, 0o600); err != nil {
		t.Fatalf("write bad document: %v", err)
	}
	scan, err := Scan(context.Background(), result.Workspace, ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	paths := map[string]bool{}
	for _, entry := range scan.Entries {
		paths[entry.Path] = true
		if strings.HasPrefix(entry.Path, "cache/") || strings.HasPrefix(entry.Path, "exports/") || strings.HasPrefix(entry.Path, ".opsc/") {
			t.Fatalf("Scan() included derived/control path: %#v", entry)
		}
	}
	if !paths[WorkspaceFileName] {
		t.Fatalf("Scan() missing workspace manifest: %#v", scan.Entries)
	}
	templatePath := filepath.ToSlash(filepath.Join("templates", document.ID, "template.json"))
	if !paths[templatePath] {
		t.Fatalf("Scan() missing template path %q: %#v", templatePath, scan.Entries)
	}
	if len(scan.Warnings) == 0 {
		t.Fatalf("Scan() warnings empty, want bad schema warning")
	}
}

func TestRebuildIndexUsesWorkspaceLockAndRebuilder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	rebuilder := &recordingRebuilder{}
	scan, err := RebuildIndex(context.Background(), result.Workspace, rebuilder, ScanOptions{})
	if err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	if !rebuilder.called {
		t.Fatal("RebuildIndex() did not call rebuilder")
	}
	if rebuilder.workspaceID != result.Workspace.Document.ID {
		t.Fatalf("rebuilder workspaceID = %q", rebuilder.workspaceID)
	}
	if rebuilder.entries != len(scan.Entries) {
		t.Fatalf("rebuilder entries = %d, scan entries = %d", rebuilder.entries, len(scan.Entries))
	}

	lock, err := AcquireLock(result.Workspace.LockPath("workspace.lock"))
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	defer lock.Release()
	_, err = RebuildIndex(context.Background(), result.Workspace, NoopIndexRebuilder{}, ScanOptions{})
	if err == nil {
		t.Fatal("RebuildIndex() while locked error = nil, want workspace_locked")
	}
	var workspaceErr *Error
	if !asWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceLocked {
		t.Fatalf("RebuildIndex() error = %#v, want workspace_locked", err)
	}
}

type recordingRebuilder struct {
	called      bool
	workspaceID string
	entries     int
}

func (r *recordingRebuilder) Rebuild(ctx context.Context, workspace Workspace, scan ScanResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.called = true
	r.workspaceID = workspace.Document.ID
	r.entries = len(scan.Entries)
	return nil
}

func writeTestObject(t *testing.T, root string, collection string, id string, fileName string, kind string, data any) {
	t.Helper()
	writeTestObjectAt(t, filepath.Join(root, collection, id, fileName), id, kind, data)
}

func writeTestObjectAt(t *testing.T, path string, id string, kind string, data any) {
	t.Helper()
	document := testEnvelope(id, kind, data)
	if err := AtomicWriteJSON(path, document, 0o600); err != nil {
		t.Fatalf("write test object %s: %v", path, err)
	}
}

func testEnvelope[T any](id string, kind string, data T) Envelope[T] {
	now := time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC).Format(time.RFC3339)
	return Envelope[T]{
		SchemaVersion: SchemaVersion,
		Kind:          kind,
		ID:            id,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Data:          data,
	}
}

func asWorkspaceError(err error, target **Error) bool {
	if err == nil {
		return false
	}
	if typed, ok := err.(*Error); ok {
		*target = typed
		return true
	}
	return false
}
