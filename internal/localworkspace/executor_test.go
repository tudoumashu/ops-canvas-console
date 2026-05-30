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
