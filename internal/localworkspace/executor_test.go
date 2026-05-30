package localworkspace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const executorTestPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/l6r3JwAAAABJRU5ErkJggg=="

func TestRunExecutorOnceExecutesFixedMaterialTextAndImage(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, assetID := seedExecutorWorkspace(t, root)

	var providerAuths []string
	var providerPaths []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerAuths = append(providerAuths, r.Header.Get("Authorization"))
		providerPaths = append(providerPaths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			if !strings.Contains(string(body), "Mug") || !strings.Contains(string(body), "art_") {
				t.Fatalf("chat payload missing rendered prompt context: %s", body)
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"short product copy"}}]}`)
		case "/v1/images/generations":
			if !strings.Contains(string(body), "short product copy") {
				t.Fatalf("image payload missing text node output: %s", body)
			}
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "material", "type": "material_lookup", "operation": "material_lookup", "title": "Fixed Material", "extra": map[string]any{"assetMode": "fixed", "assetId": assetID}},
		{"id": "copy", "type": "text_generation", "operation": "text_generation", "title": "Copy", "model": "gpt-test", "prompt": "Write {{input.productTitle}} with {{node.material.artifactId}}"},
		{"id": "image", "type": "image_generation", "operation": "image_generation", "title": "Image", "model": "image-test", "prompt": "Render {{node.copy.text}}"},
	}, []map[string]any{
		{"from": "input", "to": "copy"},
		{"from": "material", "to": "copy"},
		{"from": "copy", "to": "image"},
	})
	run := writeExecutorRun(t, workspace, template.ID, profile.ID, RunStatusPending, map[string]any{"productTitle": "Mug"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one processed run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 4 || got.Skipped != 0 || got.ArtifactRefs != 3 {
		t.Fatalf("run result = %#v, want success with material/text/image artifacts", got)
	}
	for _, auth := range providerAuths {
		if auth != "Bearer provider-secret" {
			t.Fatalf("provider auth = %q, want secretRef bearer", auth)
		}
	}
	if strings.Join(providerPaths, ",") != "/v1/chat/completions,/v1/images/generations" {
		t.Fatalf("provider paths = %#v", providerPaths)
	}

	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if status.Run.Status != RunStatusSuccess || status.Run.ArtifactCount != 3 {
		t.Fatalf("run status = %#v, want success with three artifacts", status.Run)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	if len(states) != 4 {
		t.Fatalf("node states len = %d, want 4", len(states))
	}
	for _, state := range states {
		if state.Data.Status != RunStatusSuccess {
			t.Fatalf("node %s status = %s, want success", state.ID, state.Data.Status)
		}
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	for _, want := range []string{executorEventClaimed, executorEventNodeStarted, executorEventNodeCompleted, executorEventRunCompleted} {
		if !runEventTypesContain(events, want) {
			t.Fatalf("events missing %s: %#v", want, events)
		}
	}

	second, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root})
	if err != nil {
		t.Fatalf("RunExecutorOnce() second error = %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second executor result = %#v, want no duplicate work for success run", second)
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("artifact refs after second run = %d, want 3", len(refs))
	}
	assertNoExecutorSecretLeak(t, status, events, refs)

	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuiltStatus, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() after rebuild error = %v", err)
	}
	if rebuiltStatus.Run.Status != RunStatusSuccess || len(rebuiltStatus.Nodes) != 4 || rebuiltStatus.LatestEventSequence == 0 {
		t.Fatalf("rebuilt run status = %#v, want success with nodes and events", rebuiltStatus)
	}
	rebuiltRefs, err := ListRunArtifactSummaries(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifactSummaries() after rebuild error = %v", err)
	}
	if len(rebuiltRefs) != 3 {
		t.Fatalf("rebuilt run artifact refs = %d, want 3", len(rebuiltRefs))
	}
}

func TestRunExecutorOnceResumesRunningRunWithoutReexecutingSuccessNode(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)

	var providerPaths []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerPaths = append(providerPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/images/generations":
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
		default:
			t.Fatalf("unexpected provider path on resume: %s", r.URL.Path)
		}
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "copy", "type": "text_generation", "operation": "text_generation", "title": "Copy", "model": "gpt-test", "prompt": "Write {{input.productTitle}}"},
		{"id": "image", "type": "image_generation", "operation": "image_generation", "title": "Image", "model": "image-test", "prompt": "Render {{node.copy.text}}"},
	}, []map[string]any{{"from": "copy", "to": "image"}})
	run := writeExecutorRun(t, workspace, template.ID, profile.ID, RunStatusRunning, map[string]any{"productTitle": "Mug"}, map[string]any{"executor": executorID})
	copyState, err := NewRunNodeState("copy", RunNodeStateData{
		NodeID: "copy",
		Status: RunStatusSuccess,
		Output: map[string]any{"text": "already done", "artifactIds": []any{"art_existing"}, "artifactId": "art_existing"},
	})
	if err != nil {
		t.Fatalf("NewRunNodeState() error = %v", err)
	}
	if err := WriteRunNodeState(workspace, run.ID, copyState); err != nil {
		t.Fatalf("WriteRunNodeState() error = %v", err)
	}

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one resumed run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 1 || got.Skipped != 1 || got.ArtifactRefs != 1 {
		t.Fatalf("run result = %#v, want resumed image-only execution", got)
	}
	if strings.Join(providerPaths, ",") != "/v1/images/generations" {
		t.Fatalf("provider paths = %#v, want only image generation", providerPaths)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("node states len = %d, want 2", len(states))
	}
	for _, state := range states {
		if state.Data.Status != RunStatusSuccess {
			t.Fatalf("node %s status = %s, want success", state.ID, state.Data.Status)
		}
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if !runEventTypesContain(events, executorEventResumed) {
		t.Fatalf("events missing resume marker: %#v", events)
	}
}

func TestRunExecutorOnceExecutesProjectScriptConditionRetryAndOutputMapping(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, _, _ := seedExecutorWorkspace(t, root)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "outputs"), 0o755); err != nil {
		t.Fatalf("create outputs dir: %v", err)
	}
	scriptPath := filepath.Join(projectRoot, "scripts", "generate.sh")
	script := `#!/bin/sh
set -eu
marker="outputs/.retry-marker"
if [ ! -f "$marker" ]; then
  touch "$marker"
  echo "transient failure" >&2
  exit 7
fi
printf '{"decision":"pass","message":"project:%s"}\n' "$1"
`
	if err := AtomicWriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	project := writeExecutorProject(t, workspace, projectRoot, ProjectCapabilities{FSRead: true, FSWrite: true, ProcessExec: true, ArtifactWrite: true})
	retryEnabled := true
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "gate", "type": "text", "operation": "condition", "title": "Gate", "extra": map[string]any{
			"conditions":    []any{map[string]any{"path": "input.shouldRun", "operator": "eq", "value": true, "output": "run"}},
			"defaultOutput": "skip",
		}},
		{"id": "script", "type": "text", "operation": "script", "title": "Project Script", "retry": map[string]any{"enabled": retryEnabled, "retryCount": 2, "intervalSeconds": 0}, "extra": map[string]any{
			"executor":   "project",
			"scriptPath": "scripts/generate.sh",
			"args":       []any{"{{input.productTitle}}"},
			"outputPath": "outputs/script-output.json",
		}},
	}, []map[string]any{
		{"source": "input", "target": "gate"},
		{"source": "gate", "target": "script", "fromHandle": "run"},
	})
	run := writeExecutorRunWithProject(t, workspace, template.ID, "", project.ID, RunStatusPending, map[string]any{"productTitle": "Mug", "shouldRun": true}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one project run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 3 || got.Skipped != 0 || got.ArtifactRefs != 1 {
		t.Fatalf("run result = %#v, want project script success with one artifact", got)
	}
	projectOutput, err := os.ReadFile(filepath.Join(projectRoot, "outputs", "script-output.json"))
	if err != nil {
		t.Fatalf("read project output: %v", err)
	}
	if !strings.Contains(string(projectOutput), `"project:Mug"`) {
		t.Fatalf("project output = %s, want rendered script stdout", projectOutput)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if !runEventTypesContain(events, executorEventNodeRetrying) {
		t.Fatalf("events missing retry marker: %#v", events)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	stateByID := executorTestStateMap(states)
	scriptState := stateByID["script"]
	if scriptState.Data.Status != RunStatusSuccess || len(scriptState.Data.Output["projectOutputs"].([]any)) != 1 {
		t.Fatalf("script state = %#v, want success with projectOutputs", scriptState.Data)
	}
	if scriptState.Data.Output["decision"] != "pass" {
		t.Fatalf("script output decision = %#v, want parsed JSON decision", scriptState.Data.Output["decision"])
	}
	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	assertNoExecutorRootLeak(t, projectRoot, status, events, states, refs)

	second, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() second error = %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second executor result = %#v, want no duplicate project work", second)
	}
	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuiltStatus, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() after rebuild error = %v", err)
	}
	if rebuiltStatus.Run.Status != RunStatusSuccess || rebuiltStatus.Run.ArtifactCount != 1 || len(rebuiltStatus.Nodes) != 3 {
		t.Fatalf("rebuilt run status = %#v, want project executor data indexed", rebuiltStatus)
	}
}

func TestRunExecutorProjectGuardsCapabilitiesAndPathEscapes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		capabilities ProjectCapabilities
		outputPath   string
		wantError    string
	}{
		{
			name:         "process exec capability disabled",
			capabilities: ProjectCapabilities{FSRead: true, FSWrite: true, ArtifactWrite: true},
			outputPath:   "outputs/out.txt",
			wantError:    "project capability process.exec is disabled",
		},
		{
			name:         "project output path escapes root",
			capabilities: ProjectCapabilities{FSRead: true, FSWrite: true, ProcessExec: true, ArtifactWrite: true},
			outputPath:   "../outside.txt",
			wantError:    "project path escapes root",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			workspace, _, _ := seedExecutorWorkspace(t, root)
			projectRoot := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
				t.Fatalf("create scripts dir: %v", err)
			}
			if err := os.MkdirAll(filepath.Join(projectRoot, "outputs"), 0o755); err != nil {
				t.Fatalf("create outputs dir: %v", err)
			}
			if err := AtomicWriteFile(filepath.Join(projectRoot, "scripts", "ok.sh"), []byte("#!/bin/sh\nprintf 'ok'\n"), 0o700); err != nil {
				t.Fatalf("write script: %v", err)
			}
			project := writeExecutorProject(t, workspace, projectRoot, tc.capabilities)
			template := writeExecutorTemplate(t, workspace, []map[string]any{
				{"id": "script", "type": "text", "operation": "script", "title": "Project Script", "extra": map[string]any{"executor": "project", "scriptPath": "scripts/ok.sh", "outputPath": tc.outputPath}},
			}, nil)
			run := writeExecutorRunWithProject(t, workspace, template.ID, "", project.ID, RunStatusPending, map[string]any{}, nil)
			appendWaitingForExecutor(t, workspace, run.ID)

			result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
			if err != nil {
				t.Fatalf("RunExecutorOnce() error = %v", err)
			}
			if result.Processed != 1 || len(result.Runs) != 1 || result.Runs[0].Status != RunStatusError {
				t.Fatalf("executor result = %#v, want failed project run", result)
			}
			if !strings.Contains(result.Runs[0].Error, tc.wantError) {
				t.Fatalf("run error = %q, want %q", result.Runs[0].Error, tc.wantError)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(projectRoot), "outside.txt")); !os.IsNotExist(err) {
				t.Fatalf("outside file stat err = %v, want not exist", err)
			}
			events, err := ReadRunEvents(workspace, run.ID, 0)
			if err != nil {
				t.Fatalf("ReadRunEvents() error = %v", err)
			}
			status, err := GetRunStatus(workspace, run.ID)
			if err != nil {
				t.Fatalf("GetRunStatus() error = %v", err)
			}
			assertNoExecutorRootLeak(t, projectRoot, result, events, status)
		})
	}
}

func seedExecutorWorkspace(t *testing.T, root string) (Workspace, string, string) {
	t.Helper()
	result, err := Init(InitOptions{Path: root, Name: "Executor Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	profile, err := NewProfile(workspace, ProfileData{
		Name: "Executor Profile",
		Mode: ProfileModeLocal,
		Channels: []ProfileChannel{{
			ID:        "openai",
			Protocol:  "openai-compatible",
			BaseURL:   "http://127.0.0.1:1",
			Enabled:   true,
			SecretRef: &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_EXECUTOR_TEST_KEY"},
		}},
	})
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	imageData, err := base64.StdEncoding.DecodeString(executorTestPNGBase64)
	if err != nil {
		t.Fatalf("decode test png: %v", err)
	}
	asset, err := NewAsset(workspace, AssetData{
		Type:      "image",
		MIME:      "image/png",
		Title:     "Fixed Image",
		MediaType: "image",
		Privacy:   PrivacyPrivate,
		Files:     map[string]string{"original": "files/original.png"},
	})
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	if err := WriteAsset(workspace, asset); err != nil {
		t.Fatalf("WriteAsset() error = %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(AssetRepository(workspace).Dir(asset.ID), "files", "original.png"), imageData, 0o600); err != nil {
		t.Fatalf("write asset file: %v", err)
	}
	return workspace, profile.ID, asset.ID
}

func readExecutorProfile(t *testing.T, workspace Workspace, profileID string) Envelope[ProfileData] {
	t.Helper()
	profile, err := ReadProfile(workspace, profileID)
	if err != nil {
		t.Fatalf("ReadProfile() error = %v", err)
	}
	return profile
}

func writeExecutorProject(t *testing.T, workspace Workspace, projectRoot string, capabilities ProjectCapabilities) Envelope[ProjectData] {
	t.Helper()
	project, err := NewProject(workspace, ProjectData{
		Name:         "Executor Project",
		Kind:         "code",
		Adapter:      "filesystem",
		RootPath:     projectRoot,
		Capabilities: capabilities,
		Execution: ProjectExecution{
			AllowGlobs: []string{"scripts/**", "outputs/**"},
		},
	})
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if err := WriteProject(workspace, project); err != nil {
		t.Fatalf("WriteProject() error = %v", err)
	}
	return project
}

func writeExecutorTemplate(t *testing.T, workspace Workspace, nodes []map[string]any, edges []map[string]any) Envelope[TemplateData] {
	t.Helper()
	rawNodes := make([]json.RawMessage, 0, len(nodes))
	for _, node := range nodes {
		rawNodes = append(rawNodes, marshalExecutorRaw(t, node))
	}
	rawEdges := make([]json.RawMessage, 0, len(edges))
	for _, edge := range edges {
		rawEdges = append(rawEdges, marshalExecutorRaw(t, edge))
	}
	template, err := NewTemplate(workspace, TemplateData{
		Title:        "Executor Template",
		WorkflowType: "ecommerce",
		Version:      1,
		Nodes:        rawNodes,
		Edges:        rawEdges,
	})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	return template
}

func writeExecutorRun(t *testing.T, workspace Workspace, templateID string, profileID string, status string, input map[string]any, metadata map[string]any) Envelope[RunData] {
	t.Helper()
	run, err := NewRun(workspace, RunData{
		TemplateID: templateID,
		ProfileID:  profileID,
		Status:     status,
		Input:      input,
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	return run
}

func writeExecutorRunWithProject(t *testing.T, workspace Workspace, templateID string, profileID string, projectID string, status string, input map[string]any, metadata map[string]any) Envelope[RunData] {
	t.Helper()
	run, err := NewRun(workspace, RunData{
		TemplateID: templateID,
		ProfileID:  profileID,
		ProjectID:  projectID,
		Status:     status,
		Input:      input,
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	return run
}

func appendWaitingForExecutor(t *testing.T, workspace Workspace, runID string) {
	t.Helper()
	if _, err := AppendRunEvent(workspace, runID, RunEventInput{
		Type:    "run.waiting_for_executor",
		Level:   "info",
		Actor:   RunEventActor{Type: "web", ID: "ops-canvas-web"},
		Message: "Local run draft created",
		Data:    map[string]any{"mode": "local"},
	}); err != nil {
		t.Fatalf("AppendRunEvent(waiting) error = %v", err)
	}
}

func marshalExecutorRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	return data
}

func runEventTypesContain(events []RunEventEnvelope, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func executorTestStateMap(states []Envelope[RunNodeStateData]) map[string]Envelope[RunNodeStateData] {
	out := map[string]Envelope[RunNodeStateData]{}
	for _, state := range states {
		out[state.Data.NodeID] = state
	}
	return out
}

func assertNoExecutorSecretLeak(t *testing.T, values ...any) {
	t.Helper()
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal value for secret leak check: %v", err)
		}
		if strings.Contains(string(data), "provider-secret") {
			t.Fatalf("executor output leaked provider secret: %s", data)
		}
	}
}

func assertNoExecutorRootLeak(t *testing.T, root string, values ...any) {
	t.Helper()
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal value for root leak check: %v", err)
		}
		if strings.Contains(string(data), root) {
			t.Fatalf("executor output leaked project root %q: %s", root, data)
		}
	}
}
