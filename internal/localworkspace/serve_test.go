package localworkspace

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServeRuntimeAuthCORSAndWorkspaceAPI(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", stateRoot)
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Serve Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	template, err := NewTemplate(workspace, TemplateData{Title: "Serve Template", WorkflowType: "generic"})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	artifact, err := NewArtifact(workspace, ArtifactData{
		Type:    "image",
		MIME:    "image/png",
		Title:   "Serve Artifact",
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
	if err := AtomicWriteFile(filepath.Join(ArtifactRepository(workspace).Dir(artifact.ID), "original.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write artifact file: %v", err)
	}
	prompt, err := NewPrompt(workspace, PromptData{Title: "Serve Prompt", Kind: "system"})
	if err != nil {
		t.Fatalf("NewPrompt() error = %v", err)
	}
	if err := SavePrompt(workspace, prompt, "Use serve."); err != nil {
		t.Fatalf("SavePrompt() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			WorkspacePath:  root,
			Host:           DefaultServeHost,
			Port:           0,
			AllowedOrigins: []string{"http://127.0.0.1:3000"},
			Ready: func(runtime ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	runtime := waitServeReady(t, ready)
	baseURL := runtime.BaseURL
	stateDir, err := workspace.StateDir()
	if err != nil {
		t.Fatalf("StateDir() error = %v", err)
	}
	if !strings.HasPrefix(stateDir, stateRoot) {
		t.Fatalf("stateDir = %s, want under %s", stateDir, stateRoot)
	}
	assertPrivateDir(t, stateDir)
	token := readServeToken(t, workspace)
	if token == "" {
		t.Fatal("serve token is empty")
	}
	assertPrivateFile(t, filepath.Join(stateDir, "bearer.token"))
	assertPrivateFile(t, filepath.Join(stateDir, "launch.secret"))
	if _, err := os.Stat(filepath.Join(root, ".opsc", "runtime", "serve.json")); !os.IsNotExist(err) {
		t.Fatalf("workspace runtime file exists: %v", err)
	}
	metadata, err := os.ReadFile(filepath.Join(stateDir, "serve.json"))
	if err != nil {
		t.Fatalf("read serve.json: %v", err)
	}
	if strings.Contains(string(metadata), token) || strings.Contains(string(metadata), root) {
		t.Fatalf("serve.json leaked token or workspace path: %s", metadata)
	}

	status, body := serveRequest(t, http.MethodGet, baseURL+"/api/health", "", "", nil, nil)
	if status != http.StatusOK || strings.TrimSpace(body) != "ok" {
		t.Fatalf("health status=%d body=%s", status, body)
	}
	if strings.Contains(body, root) || strings.Contains(body, token) || strings.Contains(body, workspace.Document.ID) {
		t.Fatalf("health leaked workspace details: %s", body)
	}

	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/workspace", "", "", nil, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":1`) {
		t.Fatalf("unauthorized status=%d body=%s", status, body)
	}
	assertNoServeLeak(t, body, root, token, workspace.Document.ID)

	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/runtime", "wrong-token", "", nil, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":1`) {
		t.Fatalf("bad bearer status=%d body=%s", status, body)
	}
	assertNoServeLeak(t, body, root, token, workspace.Document.ID)

	req, err := http.NewRequest(http.MethodOptions, baseURL+"/api/local/workspace", nil)
	if err != nil {
		t.Fatalf("new options request: %v", err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:3000" || resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight status=%d allow-origin=%q credentials=%q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"), resp.Header.Get("Access-Control-Allow-Credentials"))
	}

	req, err = http.NewRequest(http.MethodGet, baseURL+"/api/local/workspace", nil)
	if err != nil {
		t.Fatalf("new disallowed request: %v", err)
	}
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("disallowed request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d, want 403", resp.StatusCode)
	}

	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/workspace", token, "http://127.0.0.1:3000", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("origin bearer status=%d body=%s, want 401", status, body)
	}

	launchSecret := readLaunchSecret(t, workspace)
	status, body, cookie := bootstrapSession(t, baseURL, launchSecret)
	if status != http.StatusOK || !strings.Contains(body, `"code":0`) || cookie == nil || !cookie.HttpOnly {
		t.Fatalf("bootstrap status=%d body=%s cookie=%#v", status, body, cookie)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "launch.secret")); !os.IsNotExist(err) {
		t.Fatalf("launch.secret still exists after bootstrap: %v", err)
	}
	assertPrivateFile(t, filepath.Join(stateDir, "sessions.json"))
	status, body, _ = bootstrapSession(t, baseURL, launchSecret)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":1`) {
		t.Fatalf("second bootstrap status=%d body=%s", status, body)
	}
	assertNoServeLeak(t, body, root, token, launchSecret, workspace.Document.ID)

	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/workspace", "", "http://127.0.0.1:3000", nil, cookie)
	if status != http.StatusOK || !strings.Contains(body, `"code":0`) || !strings.Contains(body, `"active":true`) {
		t.Fatalf("workspace info with cookie status=%d body=%s", status, body)
	}
	if strings.Contains(body, root) || strings.Contains(body, token) {
		t.Fatalf("workspace info leaked path or token: %s", body)
	}
	assertNoServeLeak(t, body, root, token, launchSecret)

	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/runtime", "", "http://127.0.0.1:3000", nil, cookie)
	if status != http.StatusOK || !strings.Contains(body, `"tokenFile":"bearer.token"`) || !strings.Contains(body, `"launchSecretFile":"launch.secret"`) {
		t.Fatalf("runtime with cookie status=%d body=%s", status, body)
	}
	assertNoServeLeak(t, body, root, token, launchSecret)

	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/templates", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, template.ID) || !strings.Contains(body, "Serve Template") {
		t.Fatalf("templates status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/templates/"+template.ID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"template"`) || !strings.Contains(body, `"title":"Serve Template"`) {
		t.Fatalf("template get status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/artifacts/"+artifact.ID+"/files/original", token, "", nil, nil)
	if status != http.StatusOK || body != "png-bytes" {
		t.Fatalf("artifact file status=%d body=%q", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, baseURL+"/api/local/prompts/"+prompt.ID+"/content", token, "", nil, nil)
	if status != http.StatusOK || body != "Use serve." {
		t.Fatalf("prompt content status=%d body=%q", status, body)
	}

	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "serve.lock")); !os.IsNotExist(err) {
		t.Fatalf("serve.lock still exists after shutdown: %v", err)
	}
	if info := workspace.Info(false); info.Runtime.Active {
		t.Fatalf("workspace runtime active after shutdown: %#v", info.Runtime)
	}
}

func TestServeLocalTemplateRunDraftArtifactRefHappyPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Local Run Draft Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			WorkspacePath: root,
			Port:          0,
			Ready: func(runtime ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	runtime := waitServeReady(t, ready)
	token := readServeToken(t, workspace)

	assetData := `{"type":"image","title":"Fixed Material","mediaType":"image","privacy":"private","metadata":{"width":1,"height":1}}`
	status, body := serveMultipartRequest(t, runtime.BaseURL+"/api/local/assets/import", token, assetData, "original", "", "fixed.png", "image/png", []byte("fixed-asset-png"))
	if status != http.StatusOK || !strings.Contains(body, `"kind":"asset"`) {
		t.Fatalf("asset import status=%d body=%s", status, body)
	}
	assetID := jsonPathString(t, body, "data.id")
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/assets/"+assetID+"/files/original", token, "", nil, nil)
	if status != http.StatusOK || body != "fixed-asset-png" {
		t.Fatalf("asset file status=%d body=%q", status, body)
	}

	templateBody := `{"data":{"title":"Fixed Material Template","workflowType":"pdd","version":1,"nodes":[{"id":"material_1","type":"material_lookup","extra":{"assetMode":"fixed","assetId":` + strconvQuote(assetID) + `}},{"id":"image_1","type":"image_generation"}],"edges":[{"source":"material_1","target":"image_1"}],"settings":{"maxRetries":0}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/templates", token, "", strings.NewReader(templateBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"title":"Fixed Material Template"`) {
		t.Fatalf("template create status=%d body=%s", status, body)
	}
	templateID := jsonPathString(t, body, "data.id")

	runBody := `{"data":{"templateId":` + strconvQuote(templateID) + `,"status":"pending","input":{"items":[{"theme":"fixed material"}]},"metadata":{"executor":"not_connected","source":"web.local_template"}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs", token, "", strings.NewReader(runBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"run"`) || !strings.Contains(body, `"status":"pending"`) {
		t.Fatalf("run create status=%d body=%s", status, body)
	}
	runID := jsonPathString(t, body, "data.id")

	artifactData := `{"type":"image","title":"Fixed Material Artifact","privacy":"private","source":{"type":"local_asset","assetId":` + strconvQuote(assetID) + `,"templateId":` + strconvQuote(templateID) + `,"runId":` + strconvQuote(runID) + `,"nodeId":"material_1"}}`
	status, body = serveMultipartRequest(t, runtime.BaseURL+"/api/local/artifacts/import", token, artifactData, "original", "", "fixed.png", "image/png", []byte("fixed-asset-png"))
	if status != http.StatusOK || !strings.Contains(body, `"kind":"artifact"`) || !strings.Contains(body, `"original":"files/original.png"`) {
		t.Fatalf("artifact import status=%d body=%s", status, body)
	}
	artifactID := jsonPathString(t, body, "data.id")

	refBody := `{"data":{"artifactId":` + strconvQuote(artifactID) + `,"role":"input","nodeId":"material_1","slot":"material","order":0,"metadata":{"sourceAssetId":` + strconvQuote(assetID) + `}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs/"+runID+"/artifacts", token, "", strings.NewReader(refBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"run_artifact_ref"`) {
		t.Fatalf("run artifact ref status=%d body=%s", status, body)
	}
	nodeBody := `{"data":{"nodeId":"material_1","status":"success","output":{"assetId":` + strconvQuote(assetID) + `,"artifactId":` + strconvQuote(artifactID) + `}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs/"+runID+"/nodes/material_1", token, "", strings.NewReader(nodeBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"nodeId":"material_1"`) || !strings.Contains(body, `"status":"success"`) {
		t.Fatalf("run node state status=%d body=%s", status, body)
	}
	eventBody := `{"event":{"type":"run.waiting_for_executor","level":"info","actor":{"type":"web","id":"ops-canvas-web"},"message":"Local run draft created","data":{"mode":"local"}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs/"+runID+"/events", token, "", strings.NewReader(eventBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"type":"run.waiting_for_executor"`) {
		t.Fatalf("run event status=%d body=%s", status, body)
	}

	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/runs/"+runID+"/status", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"artifactCount":1`) || !strings.Contains(body, `"nodeId":"material_1"`) || !strings.Contains(body, `"status":"success"`) {
		t.Fatalf("run status status=%d body=%s", status, body)
	}
	assertNoServeLeak(t, body, root, token)
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/runs/"+runID+"/artifacts", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, artifactID) || !strings.Contains(body, `"role":"input"`) || !strings.Contains(body, `"slot":"material"`) {
		t.Fatalf("run artifacts status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/artifacts/"+artifactID+"/files/original", token, "", nil, nil)
	if status != http.StatusOK || body != "fixed-asset-png" {
		t.Fatalf("artifact file status=%d body=%q", status, body)
	}

	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
}

func TestServeLocalObjectWritesAndSanitization(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Write Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			WorkspacePath: root,
			Port:          0,
			Ready: func(runtime ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	runtime := waitServeReady(t, ready)
	token := readServeToken(t, workspace)

	status, body := serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/workspace/preferences", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"revision":1`) || !strings.Contains(body, `"workflowFolders":[]`) {
		t.Fatalf("workspace preferences get status=%d body=%s", status, body)
	}
	preferencesBody := `{"revision":1,"preferences":{"workflowFolders":[{"id":"custom-1","title":"文章工作流","description":"本地私有入口","kind":"custom"}]}}`
	status, body = serveRequest(t, http.MethodPut, runtime.BaseURL+"/api/local/workspace/preferences", token, "", strings.NewReader(preferencesBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"revision":2`) || !strings.Contains(body, `"title":"文章工作流"`) {
		t.Fatalf("workspace preferences update status=%d body=%s", status, body)
	}
	snapshot, err := ReadWorkspacePreferences(workspace)
	if err != nil {
		t.Fatalf("ReadWorkspacePreferences() error = %v", err)
	}
	if snapshot.Revision != 2 || len(snapshot.Preferences.WorkflowFolders) != 1 || snapshot.Preferences.WorkflowFolders[0].ID != "custom-1" {
		t.Fatalf("workspace preferences snapshot = %#v", snapshot)
	}
	preferencesSecretBody := `{"revision":2,"preferences":{"workflowFolders":[{"id":"custom-2","title":"Bad","kind":"custom","apiKey":"plain"}]}}`
	status, body = serveRequest(t, http.MethodPut, runtime.BaseURL+"/api/local/workspace/preferences", token, "", strings.NewReader(preferencesSecretBody), nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":1`) {
		t.Fatalf("workspace preferences plaintext secret status=%d body=%s", status, body)
	}

	profileBody := `{"data":{"name":"Local","channels":[{"id":"openai","enabled":true,"secretRef":{"type":"env","name":"OPENAI_API_KEY"}}]}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/profiles", token, "", strings.NewReader(profileBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"code":0`) || strings.Contains(body, "OPENAI_API_KEY") == false {
		t.Fatalf("profile create status=%d body=%s", status, body)
	}
	if strings.Contains(body, `"apiKey"`) {
		t.Fatalf("profile response leaked plaintext secret field: %s", body)
	}
	secretBody := `{"data":{"name":"Bad","metadata":{"apiKey":"plain"}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/profiles", token, "", strings.NewReader(secretBody), nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":1`) {
		t.Fatalf("plaintext secret status=%d body=%s", status, body)
	}

	templateBody := `{"data":{"title":"Local Template","description":"Private workflow","workflowType":"pdd","version":1,"nodes":[{"id":"input","type":"text"}],"edges":[],"settings":{"productConcurrency":2,"maxRetries":0}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/templates", token, "", strings.NewReader(templateBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"template"`) || !strings.Contains(body, `"title":"Local Template"`) || !strings.Contains(body, `"description":"Private workflow"`) {
		t.Fatalf("template create status=%d body=%s", status, body)
	}
	templateID := jsonPathString(t, body, "data.id")
	status, body = serveRequest(t, http.MethodPut, runtime.BaseURL+"/api/local/templates/"+templateID, token, "", strings.NewReader(`{"revision":1,"data":{"title":"Updated Template","description":"Private workflow updated","workflowType":"pdd","version":1,"nodes":[],"edges":[],"settings":{"productConcurrency":1}}}`), nil)
	if status != http.StatusOK || !strings.Contains(body, `"revision":2`) || !strings.Contains(body, `"title":"Updated Template"`) {
		t.Fatalf("template update status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/templates", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, templateID) || !strings.Contains(body, `"description":"Private workflow updated"`) {
		t.Fatalf("template list status=%d body=%s", status, body)
	}
	runBody := `{"data":{"templateId":` + strconvQuote(templateID) + `,"status":"pending","input":{"items":[{"theme":"测试商品"}]},"metadata":{"source":"serve_test"}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs", token, "", strings.NewReader(runBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"run"`) || !strings.Contains(body, `"status":"pending"`) {
		t.Fatalf("run create status=%d body=%s", status, body)
	}
	runID := jsonPathString(t, body, "data.id")
	if _, err := os.Stat(filepath.Join(root, "runs", runID, "template.snapshot.json")); err != nil {
		t.Fatalf("serve run template snapshot missing: %v", err)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/runs/"+runID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"templateId":`+strconvQuote(templateID)) {
		t.Fatalf("run get status=%d body=%s", status, body)
	}
	eventBody := `{"event":{"type":"run.waiting_for_executor","level":"info","actor":{"type":"web","id":"ops-canvas-web"},"message":"Local run created from Web UI","data":{"mode":"local"}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs/"+runID+"/events", token, "", strings.NewReader(eventBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"sequence":2`) || !strings.Contains(body, `"type":"run.waiting_for_executor"`) {
		t.Fatalf("run event append status=%d body=%s", status, body)
	}
	nodeBody := `{"data":{"nodeId":"input","status":"success","output":{"items":1}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs/"+runID+"/nodes/input", token, "", strings.NewReader(nodeBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"run_node"`) || !strings.Contains(body, `"nodeId":"input"`) {
		t.Fatalf("run node write status=%d body=%s", status, body)
	}
	artifactData := `{"type":"image","title":"Generated Artifact","privacy":"private","source":{"runId":` + strconvQuote(runID) + `,"nodeId":"image_node"}}`
	status, body = serveMultipartRequest(t, runtime.BaseURL+"/api/local/artifacts/import", token, artifactData, "original", "", "generated.png", "image/png", []byte("artifact-png"))
	if status != http.StatusOK || !strings.Contains(body, `"kind":"artifact"`) || !strings.Contains(body, `"original":"files/original.png"`) {
		t.Fatalf("artifact import status=%d body=%s", status, body)
	}
	artifactID := jsonPathString(t, body, "data.id")
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/artifacts/"+artifactID+"/files/original", token, "", nil, nil)
	if status != http.StatusOK || body != "artifact-png" {
		t.Fatalf("artifact file status=%d body=%q", status, body)
	}
	refBody := `{"data":{"artifactId":` + strconvQuote(artifactID) + `,"role":"primary_output","nodeId":"image_node","slot":"output","order":0}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/runs/"+runID+"/artifacts", token, "", strings.NewReader(refBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"run_artifact_ref"`) || !strings.Contains(body, `"artifactId":`+strconvQuote(artifactID)) {
		t.Fatalf("run artifact ref status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/runs/"+runID+"/status", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"artifactCount":1`) || !strings.Contains(body, `"latestEventSequence":2`) {
		t.Fatalf("run status status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/runs/"+runID+"/artifacts", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, artifactID) || !strings.Contains(body, `"role":"primary_output"`) {
		t.Fatalf("run artifacts status=%d body=%s", status, body)
	}
	runUpdateBody := `{"revision":1,"data":{"templateId":` + strconvQuote(templateID) + `,"status":"error","input":{"items":[{"theme":"测试商品"}]},"metadata":{"source":"serve_test","reason":"manual_stop"}}}`
	status, body = serveRequest(t, http.MethodPut, runtime.BaseURL+"/api/local/runs/"+runID, token, "", strings.NewReader(runUpdateBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"revision":2`) || !strings.Contains(body, `"status":"error"`) {
		t.Fatalf("run update status=%d body=%s", status, body)
	}

	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	projectBody := `{"data":{"name":"Project","rootPath":` + strconvQuote(projectRoot) + `,"capabilities":{"fs.read":true},"credentialRefs":{"git":{"type":"file","path":"/private/token"}}}}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/projects", token, "", strings.NewReader(projectBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"rootFingerprint":"rootfp_`) || strings.Contains(body, projectRoot) || strings.Contains(body, "/private/token") {
		t.Fatalf("project create status=%d body=%s", status, body)
	}

	promptBody := `{"data":{"title":"Prompt","kind":"system"},"content":"hello"}`
	status, body = serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/prompts", token, "", strings.NewReader(promptBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"code":0`) || !strings.Contains(body, `"title":"Prompt"`) {
		t.Fatalf("prompt create status=%d body=%s", status, body)
	}
	promptID := jsonPathString(t, body, "data.id")

	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/profiles", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"channelCount":1`) {
		t.Fatalf("profile list status=%d body=%s", status, body)
	}

	assetData := `{"type":"image","title":"Local Image","mediaType":"image","privacy":"private","metadata":{"width":1,"height":1}}`
	status, body = serveMultipartRequest(t, runtime.BaseURL+"/api/local/assets/import", token, assetData, "original", "", "tiny.png", "image/png", []byte("png-data"))
	if status != http.StatusOK || !strings.Contains(body, `"code":0`) || !strings.Contains(body, `"original":"files/original.png"`) {
		t.Fatalf("asset import status=%d body=%s", status, body)
	}
	assetID := jsonPathString(t, body, "data.id")
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/assets/"+assetID+"/files/original", token, "", nil, nil)
	if status != http.StatusOK || body != "png-data" {
		t.Fatalf("asset file status=%d body=%q", status, body)
	}
	assetUpdateData := `{"type":"image","title":"Updated Image","mediaType":"image","privacy":"private","metadata":{"width":2,"height":2}}`
	status, body = serveMultipartRequest(t, runtime.BaseURL+"/api/local/assets/"+assetID+"/import", token, assetUpdateData, "original", "1", "tiny.webp", "image/webp", []byte("webp-data"))
	if status != http.StatusOK || !strings.Contains(body, `"revision":2`) || !strings.Contains(body, `"original":"files/original.webp"`) {
		t.Fatalf("asset update import status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/assets/"+assetID+"/files/original", token, "", nil, nil)
	if status != http.StatusOK || body != "webp-data" {
		t.Fatalf("updated asset file status=%d body=%q", status, body)
	}
	status, body = serveRequest(t, http.MethodDelete, runtime.BaseURL+"/api/local/prompts/"+promptID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"deleted":true`) {
		t.Fatalf("prompt delete status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodDelete, runtime.BaseURL+"/api/local/templates/"+templateID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"deleted":true`) {
		t.Fatalf("template delete status=%d body=%s", status, body)
	}
	if _, err := ReadTemplate(workspace, templateID); err == nil {
		t.Fatalf("ReadTemplate(%s) after delete error = nil", templateID)
	}
	status, body = serveRequest(t, http.MethodDelete, runtime.BaseURL+"/api/local/assets/"+assetID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"deleted":true`) {
		t.Fatalf("asset delete status=%d body=%s", status, body)
	}
	if _, err := ReadAsset(workspace, assetID); err == nil {
		t.Fatalf("ReadAsset(%s) after delete error = nil", assetID)
	}

	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
}

func TestServeAIProxyUsesProfileSecretRef(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("OPSC_TEST_AI_KEY", "provider-secret")
	var providerAuth string
	var providerPath string
	var providerCookie string
	var providerProfileHeader string
	var providerLocalToken string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerAuth = r.Header.Get("Authorization")
		providerPath = r.URL.RequestURI()
		providerCookie = r.Header.Get("Cookie")
		providerProfileHeader = r.Header.Get("X-Opsc-Profile-Id")
		providerLocalToken = r.Header.Get("X-Local-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"gpt-test"}]}`)
	}))
	defer provider.Close()

	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "AI Proxy Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	profile, err := NewProfile(workspace, ProfileData{
		Name: "Local AI",
		Mode: ProfileModeLocal,
		Channels: []ProfileChannel{
			{
				ID:        "openai",
				Protocol:  "openai",
				BaseURL:   provider.URL,
				Models:    []string{"gpt-test"},
				Enabled:   true,
				SecretRef: &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_TEST_AI_KEY"},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			WorkspacePath:  root,
			Port:           0,
			AllowedOrigins: []string{"http://127.0.0.1:3000"},
			Ready: func(runtime ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	runtime := waitServeReady(t, ready)
	token := readServeToken(t, workspace)

	status, body := serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/ai/v1/models?profileId="+profile.ID+"&internal=1", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, "gpt-test") {
		t.Fatalf("ai proxy status=%d body=%s", status, body)
	}
	if providerAuth != "Bearer provider-secret" {
		t.Fatalf("provider auth = %q, want profile secret", providerAuth)
	}
	if providerPath != "/v1/models?internal=1" {
		t.Fatalf("provider path = %q, want /v1/models?internal=1", providerPath)
	}
	if providerCookie != "" || providerProfileHeader != "" || providerLocalToken != "" {
		t.Fatalf("browser/local headers reached provider cookie=%q profile=%q local=%q", providerCookie, providerProfileHeader, providerLocalToken)
	}

	launchSecret := readLaunchSecret(t, workspace)
	status, body, cookie := bootstrapSession(t, runtime.BaseURL, launchSecret)
	if status != http.StatusOK || cookie == nil {
		t.Fatalf("bootstrap status=%d body=%s cookie=%#v", status, body, cookie)
	}
	providerAuth = ""
	providerCookie = ""
	providerProfileHeader = ""
	providerLocalToken = ""
	req, err := http.NewRequest(http.MethodGet, runtime.BaseURL+"/api/local/ai/v1/models?profileId="+profile.ID+"&internal=1", nil)
	if err != nil {
		t.Fatalf("new browser ai proxy request: %v", err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	req.Header.Set("Authorization", "Bearer browser-should-not-reach-provider")
	req.Header.Set("X-Opsc-Profile-Id", profile.ID)
	req.Header.Set("X-Local-Token", token)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("browser ai proxy request: %v", err)
	}
	browserBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read browser ai proxy body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(browserBody), "gpt-test") {
		t.Fatalf("browser ai proxy status=%d body=%s", resp.StatusCode, browserBody)
	}
	if providerAuth != "Bearer provider-secret" {
		t.Fatalf("provider auth after browser request = %q, want profile secret", providerAuth)
	}
	if providerCookie != "" || providerProfileHeader != "" || providerLocalToken != "" {
		t.Fatalf("browser/local headers reached provider cookie=%q profile=%q local=%q", providerCookie, providerProfileHeader, providerLocalToken)
	}

	t.Setenv("OPSC_TEST_AI_KEY", "")
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/ai/v1/models?profileId="+profile.ID, token, "", nil, nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":1`) || !strings.Contains(body, "ai channel env secret is not configured") {
		t.Fatalf("missing env secret status=%d body=%s", status, body)
	}
	assertNoServeLeak(t, body, root, token, launchSecret, "provider-secret", "OPSC_TEST_AI_KEY")

	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
}

func TestServeConcurrentProfileWrites(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Concurrent Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			WorkspacePath: root,
			Port:          0,
			Ready: func(runtime ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	runtime := waitServeReady(t, ready)
	token := readServeToken(t, workspace)
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := `{"data":{"name":"Profile ` + strconv.Itoa(i) + `"}}`
			status, response := serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/profiles", token, "", strings.NewReader(body), nil)
			if status != http.StatusOK || !strings.Contains(response, `"code":0`) {
				errs <- response
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for errText := range errs {
		t.Fatalf("concurrent write failed: %s", errText)
	}
	profiles, err := ListProfileSummaries(workspace)
	if err != nil {
		t.Fatalf("ListProfileSummaries() error = %v", err)
	}
	if len(profiles) != 8 {
		t.Fatalf("profiles = %d, want 8", len(profiles))
	}
	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
}

func TestServeRejectsNonLoopbackHostAndWildcardOrigin(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Init(InitOptions{Path: root}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := Serve(context.Background(), ServeOptions{WorkspacePath: root, Host: "0.0.0.0", Port: 0}); err == nil {
		t.Fatal("Serve() non-loopback host error = nil, want error")
	}
	if err := Serve(context.Background(), ServeOptions{WorkspacePath: root, Port: 0, AllowedOrigins: []string{"*"}}); err == nil {
		t.Fatal("Serve() wildcard origin error = nil, want error")
	}
}

func waitServeReady(t *testing.T, ready <-chan ServeRuntimeInfo) ServeRuntimeInfo {
	t.Helper()
	select {
	case runtime := <-ready:
		if runtime.BaseURL == "" || runtime.Port == 0 || runtime.TokenFile != "bearer.token" || runtime.LaunchSecretFile != "launch.secret" {
			t.Fatalf("runtime = %#v", runtime)
		}
		return runtime
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for serve ready")
		return ServeRuntimeInfo{}
	}
}

func waitServeDone(t *testing.T, errCh <-chan error) error {
	t.Helper()
	select {
	case err := <-errCh:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for serve shutdown")
		return nil
	}
}

func readServeToken(t *testing.T, workspace Workspace) string {
	t.Helper()
	path, err := workspace.StatePath("bearer.token")
	if err != nil {
		t.Fatalf("StatePath() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bearer.token: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func readLaunchSecret(t *testing.T, workspace Workspace) string {
	t.Helper()
	path, err := workspace.StatePath("launch.secret")
	if err != nil {
		t.Fatalf("StatePath() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read launch.secret: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	if stat, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	} else if stat.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode = %o, want 0600", path, stat.Mode().Perm())
	}
}

func assertPrivateDir(t *testing.T, path string) {
	t.Helper()
	if stat, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	} else if !stat.IsDir() || stat.Mode().Perm() != 0o700 {
		t.Fatalf("%s mode = %o dir=%v, want private dir 0700", path, stat.Mode().Perm(), stat.IsDir())
	}
}

func assertNoServeLeak(t *testing.T, body string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		if strings.Contains(body, secret) {
			t.Fatalf("serve response leaked %q in body: %s", secret, body)
		}
	}
}

func bootstrapSession(t *testing.T, baseURL string, launchSecret string) (int, string, *http.Cookie) {
	t.Helper()
	body := strings.NewReader(`{"launchSecret":` + strconvQuote(launchSecret) + `}`)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/local/bootstrap/session", body)
	if err != nil {
		t.Fatalf("new bootstrap request: %v", err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bootstrap request: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read bootstrap response: %v", err)
	}
	var session *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == serveSessionCookieName {
			copied := *cookie
			session = &copied
			break
		}
	}
	return resp.StatusCode, strings.TrimSpace(string(data)), session
}

func serveRequest(t *testing.T, method string, url string, token string, origin string, body io.Reader, cookie *http.Cookie) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, strings.TrimSpace(string(data))
}

func serveMultipartRequest(t *testing.T, url string, token string, data string, fileKey string, revision string, fileName string, contentType string, content []byte) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("data", data); err != nil {
		t.Fatalf("write multipart data: %v", err)
	}
	if fileKey != "" {
		if err := writer.WriteField("fileKey", fileKey); err != nil {
			t.Fatalf("write multipart fileKey: %v", err)
		}
	}
	if revision != "" {
		if err := writer.WriteField("revision", revision); err != nil {
			t.Fatalf("write multipart revision: %v", err)
		}
	}
	part, err := writer.CreatePart(textprotoMIMEHeader(map[string]string{
		`Content-Disposition`: `form-data; name="file"; filename="` + fileName + `"`,
		`Content-Type`:        contentType,
	}))
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if strings.Contains(url, "/import") && revision != "" {
		req, err = http.NewRequest(http.MethodPut, url, &body)
	}
	if err != nil {
		t.Fatalf("new multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("multipart request %s: %v", url, err)
	}
	defer resp.Body.Close()
	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read multipart response: %v", err)
	}
	return resp.StatusCode, strings.TrimSpace(string(responseData))
}

func textprotoMIMEHeader(values map[string]string) textproto.MIMEHeader {
	header := textproto.MIMEHeader{}
	for key, value := range values {
		header.Set(key, value)
	}
	return header
}

func jsonPathString(t *testing.T, raw string, path string) string {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	current := value
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("json path %s missing object at %s in %s", path, part, raw)
		}
		current = object[part]
	}
	text, ok := current.(string)
	if !ok || text == "" {
		t.Fatalf("json path %s = %#v, want non-empty string", path, current)
	}
	return text
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestServeJSONEnvelopeShape(t *testing.T) {
	raw := serveResponse{Code: 0, Data: map[string]string{"status": "ok"}, Msg: "ok"}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal serve envelope: %v", err)
	}
	if !bytes.Contains(data, []byte(`"code":0`)) || !bytes.Contains(data, []byte(`"msg":"ok"`)) {
		t.Fatalf("serve envelope shape = %s", data)
	}
}
