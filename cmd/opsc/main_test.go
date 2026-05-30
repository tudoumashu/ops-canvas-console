package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basketikun/infinite-canvas/internal/localworkspace"
)

func TestWorkspaceInitInfoDoctorJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	var initOut bytes.Buffer
	var initErr bytes.Buffer
	if code := run([]string{"workspace", "init", "--workspace", root, "--name", "CLI Workspace", "--json"}, &initOut, &initErr); code != 0 {
		t.Fatalf("workspace init exit = %d stderr=%s", code, initErr.String())
	}
	if initErr.Len() != 0 {
		t.Fatalf("workspace init stderr = %s, want empty", initErr.String())
	}
	var initEnvelope map[string]any
	if err := json.Unmarshal(initOut.Bytes(), &initEnvelope); err != nil {
		t.Fatalf("init output is not json: %v\n%s", err, initOut.String())
	}
	if initEnvelope["ok"] != true {
		t.Fatalf("init ok = %#v", initEnvelope["ok"])
	}
	if warnings, ok := initEnvelope["warnings"].([]any); !ok || len(warnings) != 0 {
		t.Fatalf("init warnings = %#v, want empty array", initEnvelope["warnings"])
	}
	if strings.Contains(initOut.String(), root) {
		t.Fatalf("init output leaked path without --show-paths: %s", initOut.String())
	}

	var infoOut bytes.Buffer
	var infoErr bytes.Buffer
	if code := run([]string{"--workspace", root, "workspace", "info", "--json"}, &infoOut, &infoErr); code != 0 {
		t.Fatalf("workspace info exit = %d stderr=%s", code, infoErr.String())
	}
	if infoErr.Len() != 0 {
		t.Fatalf("workspace info stderr = %s, want empty", infoErr.String())
	}
	if strings.Contains(infoOut.String(), root) {
		t.Fatalf("info output leaked path without --show-paths: %s", infoOut.String())
	}
	if !strings.Contains(infoOut.String(), "CLI Workspace") {
		t.Fatalf("info output missing workspace name: %s", infoOut.String())
	}

	var doctorOut bytes.Buffer
	var doctorErr bytes.Buffer
	if code := run([]string{"workspace", "doctor", "--workspace", root, "--json"}, &doctorOut, &doctorErr); code != 0 {
		t.Fatalf("workspace doctor exit = %d stderr=%s", code, doctorErr.String())
	}
	if !strings.Contains(doctorErr.String(), "Workspace OK") {
		t.Fatalf("workspace doctor stderr missing human report: %s", doctorErr.String())
	}
	if !strings.Contains(doctorOut.String(), `"ok": true`) {
		t.Fatalf("doctor output missing ok: %s", doctorOut.String())
	}
	if strings.Contains(doctorOut.String(), root) || strings.Contains(doctorErr.String(), root) {
		t.Fatalf("doctor output leaked path without --show-paths:\nstdout=%s\nstderr=%s", doctorOut.String(), doctorErr.String())
	}
}

func TestWorkspaceInfoMissingJSONError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"workspace", "info", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("workspace info exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `"code": "workspace_not_found"`) {
		t.Fatalf("stderr missing workspace_not_found: %s", stderr.String())
	}
}

func TestWorkspaceInfoMissingHumanErrorUsesStderr(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"workspace", "info", "--workspace", root}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("workspace info exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "workspace document not found") {
		t.Fatalf("stderr missing human error: %s", stderr.String())
	}
}

func TestWorkspaceDoctorUnhealthyOutputConvention(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var initOut bytes.Buffer
	var initErr bytes.Buffer
	if code := run([]string{"workspace", "init", "--workspace", root}, &initOut, &initErr); code != 0 {
		t.Fatalf("workspace init exit = %d stderr=%s", code, initErr.String())
	}
	if err := os.RemoveAll(filepath.Join(root, "runs")); err != nil {
		t.Fatalf("remove runs dir: %v", err)
	}

	var humanOut bytes.Buffer
	var humanErr bytes.Buffer
	code := run([]string{"workspace", "doctor", "--workspace", root}, &humanOut, &humanErr)
	if code != 2 {
		t.Fatalf("workspace doctor human exit = %d, want 2", code)
	}
	if humanOut.Len() != 0 {
		t.Fatalf("human stdout = %s, want empty", humanOut.String())
	}
	if !strings.Contains(humanErr.String(), "Workspace has problems") || !strings.Contains(humanErr.String(), "dir:runs") {
		t.Fatalf("human stderr missing doctor report: %s", humanErr.String())
	}

	var jsonOut bytes.Buffer
	var jsonErr bytes.Buffer
	code = run([]string{"workspace", "doctor", "--workspace", root, "--json"}, &jsonOut, &jsonErr)
	if code != 2 {
		t.Fatalf("workspace doctor json exit = %d, want 2", code)
	}
	if !strings.Contains(jsonOut.String(), `"ok": false`) || !strings.Contains(jsonOut.String(), "dir:runs") {
		t.Fatalf("json stdout missing machine-readable report: %s", jsonOut.String())
	}
	if !strings.Contains(jsonErr.String(), "Workspace has problems") || !strings.Contains(jsonErr.String(), "dir:runs") {
		t.Fatalf("json stderr missing human report: %s", jsonErr.String())
	}
}

func TestTemplateRunArtifactQueryCommandsJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	seed := seedQueryWorkspace(t, root)

	cases := []struct {
		name        string
		args        []string
		contains    []string
		notContains []string
	}{
		{
			name: "template list",
			args: []string{"template", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.TemplateID,
				"CLI Template",
				`"workflowType": "generic"`,
			},
		},
		{
			name: "run list",
			args: []string{"run", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.RunID,
				`"status": "running"`,
				`"artifactCount": 1`,
			},
		},
		{
			name: "run status",
			args: []string{"run", "status", seed.RunID, "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.RunID,
				seed.TemplateID,
				`"status": "running"`,
				`"latestEventSequence": 1`,
				`"nodeId": "image_generation_may1"`,
			},
		},
		{
			name: "run events",
			args: []string{"run", "events", seed.RunID, "--workspace", root, "--json"},
			contains: []string{
				`"schemaVersion":"local-workspace-v1"`,
				`"type":"run.created"`,
				`"subject":{"kind":"run","id":"` + seed.RunID + `"}`,
			},
		},
		{
			name: "artifact list all",
			args: []string{"artifact", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.ArtifactID,
				"CLI Artifact",
				`"original": "original.png"`,
			},
		},
		{
			name: "artifact list by run",
			args: []string{"artifact", "list", "--run", seed.RunID, "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				`"runId": "` + seed.RunID + `"`,
				seed.ArtifactID,
				`"role": "primary_output"`,
			},
		},
		{
			name: "index rebuild",
			args: []string{"workspace", "index", "rebuild", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				`"workspaceId":`,
				`"entries":`,
			},
		},
		{
			name: "profile list",
			args: []string{"profile", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.ProfileID,
				"CLI Profile",
				`"channelCount": 1`,
			},
			notContains: []string{"OPENAI_API_KEY"},
		},
		{
			name: "project list",
			args: []string{"project", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.ProjectID,
				"CLI Project",
				`"hasRootPath": true`,
				`"rootFingerprint": "rootfp_`,
			},
			notContains: []string{seed.ProjectRoot},
		},
		{
			name: "asset list",
			args: []string{"asset", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.AssetID,
				"CLI Asset",
				seed.ArtifactID,
				`"categoryPath": "cli/test"`,
			},
		},
		{
			name: "prompt list",
			args: []string{"prompt", "list", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				seed.PromptID,
				"CLI Prompt",
				`"hasContent": true`,
				`"model": "gpt-test"`,
			},
		},
		{
			name: "workspace export plan",
			args: []string{"workspace", "export", "plan", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				`"includePaths":`,
				`"excludePaths":`,
				`"index.sqlite"`,
			},
			notContains: []string{seed.ProjectRoot},
		},
		{
			name: "workspace gc plan",
			args: []string{"workspace", "gc", "plan", "--workspace", root, "--json"},
			contains: []string{
				`"ok": true`,
				`"candidates":`,
				`"warnings": []`,
			},
			notContains: []string{seed.ProjectRoot},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%#v) exit = %d stderr=%s", tt.args, code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %s, want empty", stderr.String())
			}
			for _, want := range tt.contains {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
			for _, unwanted := range tt.notContains {
				if unwanted != "" && strings.Contains(stdout.String(), unwanted) {
					t.Fatalf("stdout unexpectedly contained %q:\n%s", unwanted, stdout.String())
				}
			}
			if strings.Contains(stdout.String(), root) {
				t.Fatalf("stdout leaked workspace path: %s", stdout.String())
			}
		})
	}
}

func TestRunEventsFollowCommandJSONL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	seed := seedQueryWorkspace(t, root)

	ctx, cancel := context.WithCancel(context.Background())
	writer := &cancelAfterLineWriter{cancel: cancel}
	var stderr bytes.Buffer
	if code := runWithContext(ctx, []string{"run", "events", seed.RunID, "--workspace", root, "--follow"}, writer, &stderr); code != 0 {
		t.Fatalf("run events --follow exit = %d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(writer.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("follow lines = %#v, want one line before cancel", lines)
	}
	var event localworkspace.RunEventEnvelope
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("follow output is not json event: %v\n%s", err, writer.String())
	}
	if event.Type != "run.created" || event.Subject.ID != seed.RunID {
		t.Fatalf("follow event = %#v", event)
	}
}

func TestExecutorCommandJSONProcessesTextStaticRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := localworkspace.Init(localworkspace.InitOptions{Path: root, Name: "Executor CLI Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	template, err := localworkspace.NewTemplate(workspace, localworkspace.TemplateData{
		Title:        "Executor CLI Template",
		WorkflowType: "generic",
		Version:      1,
		Nodes: []json.RawMessage{
			json.RawMessage(`{"id":"copy","type":"text_static","operation":"text_static","title":"Copy","prompt":"Hello {{input.productTitle}}"}`),
		},
	})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := localworkspace.WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	runDoc, err := localworkspace.NewRun(workspace, localworkspace.RunData{
		TemplateID: template.ID,
		Status:     localworkspace.RunStatusPending,
		Input:      map[string]any{"productTitle": "Mug"},
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := localworkspace.WriteRun(workspace, runDoc); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	if _, err := localworkspace.AppendRunEvent(workspace, runDoc.ID, localworkspace.RunEventInput{
		Type:    "run.waiting_for_executor",
		Level:   "info",
		Actor:   localworkspace.RunEventActor{Type: "web", ID: "ops-canvas-web"},
		Message: "Local run draft created",
		Data:    map[string]any{"mode": "local"},
	}); err != nil {
		t.Fatalf("AppendRunEvent() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"executor", "--workspace", root, "--run", runDoc.ID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("executor exit = %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	for _, want := range []string{`"ok": true`, `"processed": 1`, `"status": "success"`, `"executed": 1`, `"artifactRefs": 1`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("stdout leaked workspace path: %s", stdout.String())
	}
	snapshot, err := localworkspace.GetRunStatus(workspace, runDoc.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if snapshot.Run.Status != localworkspace.RunStatusSuccess || snapshot.Run.ArtifactCount != 1 {
		t.Fatalf("run snapshot = %#v, want success with artifact", snapshot.Run)
	}
}

func TestEcommerceImportTemplateCommandJSONRedactsSecretAndPath(t *testing.T) {
	t.Setenv("OPSC_HYBRID_CLI_TOKEN", "cli-remote-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := localworkspace.Init(localworkspace.InitOptions{Path: root, Name: "Hybrid CLI Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	var gotAuth string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/workflows/pdd/templates/remote_tpl" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"id":"remote_tpl","workflowType":"pdd","title":"CLI Ecommerce","spec":{"version":1,"nodes":[{"id":"stage_generate","title":"Generate","type":"image","operation":"image_generation"}],"edges":[],"settings":{"productConcurrency":1,"maxRetries":0}}},"msg":"ok"}`)
	}))
	defer remote.Close()
	profile, err := localworkspace.NewProfile(workspace, localworkspace.ProfileData{
		Name: "Hybrid VPS",
		Mode: localworkspace.ProfileModeHybrid,
		Channels: []localworkspace.ProfileChannel{{
			ID:        "vps",
			Protocol:  "ops-canvas-vps",
			BaseURL:   remote.URL,
			Enabled:   true,
			SecretRef: &localworkspace.SecretRef{Type: localworkspace.SecretRefTypeEnv, Name: "OPSC_HYBRID_CLI_TOKEN"},
		}},
	})
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if err := localworkspace.WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"ecommerce", "import-template", "--workspace", root, "--remote-template", "remote_tpl", "--profile", profile.ID, "--channel", "vps", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("ecommerce import-template exit = %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	if gotAuth != "Bearer cli-remote-secret" {
		t.Fatalf("auth = %q, want secretRef bearer", gotAuth)
	}
	for _, want := range []string{`"ok": true`, `"created": true`, `"remoteTemplateId": "remote_tpl"`, `"title": "CLI Ecommerce"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	for _, secret := range []string{root, "cli-remote-secret"} {
		if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatalf("CLI output leaked %q:\nstdout=%s\nstderr=%s", secret, stdout.String(), stderr.String())
		}
	}
	templates, err := localworkspace.ListTemplates(workspace)
	if err != nil {
		t.Fatalf("ListTemplates() error = %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("templates = %#v, want imported template", templates)
	}
	inputPath := filepath.Join(t.TempDir(), "hybrid-input.json")
	if err := os.WriteFile(inputPath, []byte(`{"inputs":[{"productTitle":"Mug"}]}`), 0o600); err != nil {
		t.Fatalf("write input file: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"ecommerce", "create-run", templates[0].ID, "--workspace", root, "--input-file", inputPath, "--profile", profile.ID, "--channel", "vps", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("ecommerce create-run exit = %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("create-run stderr = %s, want empty", stderr.String())
	}
	for _, want := range []string{`"ok": true`, `"remoteTemplateId": "remote_tpl"`, `"status": "pending"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("create-run stdout missing %q:\n%s", want, stdout.String())
		}
	}
	for _, secret := range []string{root, inputPath, "cli-remote-secret"} {
		if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatalf("create-run output leaked %q:\nstdout=%s\nstderr=%s", secret, stdout.String(), stderr.String())
		}
	}
	runs, err := localworkspace.ListRuns(workspace)
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Data.TemplateID != templates[0].ID || runs[0].Data.Input["productConcurrency"] == nil {
		t.Fatalf("runs = %#v, want one ecommerce draft with defaults", runs)
	}
	events, err := localworkspace.ReadRunEvents(workspace, runs[0].ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if len(events) < 2 || events[len(events)-1].Type != "run.waiting_for_executor" {
		t.Fatalf("events = %#v, want waiting_for_executor", events)
	}
	states, err := localworkspace.ListRunNodeStates(workspace, runs[0].ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	if len(states) != 1 || states[0].Data.NodeID != "stage_generate" || states[0].Data.Status != localworkspace.RunStatusPending {
		t.Fatalf("node states = %#v, want pending template node", states)
	}
}

func TestServeCommandJSONOutputsRuntimeWithoutToken(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	seedQueryWorkspace(t, root)

	ctx, cancel := context.WithCancel(context.Background())
	writer := &cancelOnSubstringWriter{cancel: cancel, needle: `"baseUrl"`}
	var stderr bytes.Buffer
	code := runWithContext(ctx, []string{"serve", "--workspace", root, "--port", "0", "--origin", "http://127.0.0.1:3000", "--json"}, writer, &stderr)
	if code != 0 {
		t.Fatalf("serve exit = %d stderr=%s stdout=%s", code, stderr.String(), writer.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	stdout := writer.String()
	for _, want := range []string{`"ok": true`, `"active": true`, `"baseUrl":`, `"tokenFile": "bearer.token"`, `"launchSecretFile": "launch.secret"`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("serve stdout missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, root) {
		t.Fatalf("serve stdout leaked workspace path: %s", stdout)
	}
	token := readMCPServeTokenForTest(t, root)
	if token != "" && strings.Contains(stdout, token) {
		t.Fatalf("serve stdout leaked bearer token: %s", stdout)
	}
}

func TestServeCommandHumanOutputRedactsRuntimeSecrets(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	seedQueryWorkspace(t, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout bytes.Buffer
	stderr := &cancelOnSubstringWriter{cancel: cancel, needle: "launch secret file:"}
	code := runWithContext(ctx, []string{"serve", "--workspace", root, "--port", "0", "--origin", "http://127.0.0.1:3000"}, &stdout, stderr)
	if code != 0 {
		t.Fatalf("serve exit = %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s, want empty", stdout.String())
	}
	output := stderr.String()
	for _, want := range []string{"opsc serve listening on http://127.0.0.1:", "token file: bearer.token", "launch secret file: launch.secret"} {
		if !strings.Contains(output, want) {
			t.Fatalf("serve stderr missing %q:\n%s", want, output)
		}
	}
	workspace, err := localworkspace.Open(root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	stateDir, err := workspace.StateDir()
	if err != nil {
		t.Fatalf("StateDir() error = %v", err)
	}
	token := readMCPServeTokenForTest(t, root)
	tokenFile, err := workspace.StatePath("bearer.token")
	if err != nil {
		t.Fatalf("StatePath() error = %v", err)
	}
	for _, secret := range []string{root, stateDir, tokenFile, token} {
		if strings.TrimSpace(secret) != "" && strings.Contains(output, secret) {
			t.Fatalf("serve stderr leaked %q:\n%s", secret, output)
		}
	}
}

type querySeed struct {
	TemplateID  string
	RunID       string
	ArtifactID  string
	ProfileID   string
	ProjectID   string
	AssetID     string
	PromptID    string
	ProjectRoot string
}

func seedQueryWorkspace(t *testing.T, root string) querySeed {
	t.Helper()
	result, err := localworkspace.Init(localworkspace.InitOptions{Path: root, Name: "Query Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	template, err := localworkspace.NewTemplate(workspace, localworkspace.TemplateData{
		Title:        "CLI Template",
		WorkflowType: "generic",
		Version:      1,
	})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := localworkspace.WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	artifact, err := localworkspace.NewArtifact(workspace, localworkspace.ArtifactData{
		Type:    "image",
		MIME:    "image/png",
		Title:   "CLI Artifact",
		Bytes:   256,
		Width:   128,
		Height:  64,
		Privacy: "private",
		Files: map[string]string{
			"original":  "original.png",
			"thumbnail": "thumb.webp",
		},
	})
	if err != nil {
		t.Fatalf("NewArtifact() error = %v", err)
	}
	if err := localworkspace.WriteArtifact(workspace, artifact); err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	runDoc, err := localworkspace.NewRun(workspace, localworkspace.RunData{
		TemplateID: template.ID,
		Status:     localworkspace.RunStatusRunning,
		ArtifactRefs: []localworkspace.RunArtifactRefData{
			{ArtifactID: artifact.ID, Role: "primary_output", Slot: "output", Order: 0},
		},
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := localworkspace.WriteRun(workspace, runDoc); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	nodeDoc, err := localworkspace.NewRunNodeState("image_generation_may1", localworkspace.RunNodeStateData{Status: localworkspace.RunStatusRunning})
	if err != nil {
		t.Fatalf("NewRunNodeState() error = %v", err)
	}
	if err := localworkspace.WriteRunNodeState(workspace, runDoc.ID, nodeDoc); err != nil {
		t.Fatalf("WriteRunNodeState() error = %v", err)
	}
	refDoc := testCLIEnvelope(artifact.ID, localworkspace.KindRunArtifactRef, localworkspace.RunArtifactRefData{
		ArtifactID: artifact.ID,
		Role:       "primary_output",
		NodeID:     "image_generation_may1",
		Slot:       "output",
		Order:      0,
	})
	if err := localworkspace.WriteRunArtifactRef(workspace, runDoc.ID, refDoc); err != nil {
		t.Fatalf("WriteRunArtifactRef() error = %v", err)
	}
	profile, err := localworkspace.NewProfile(workspace, localworkspace.ProfileData{
		Name: "CLI Profile",
		Mode: localworkspace.ProfileModeHybrid,
		Channels: []localworkspace.ProfileChannel{
			{
				ID:       "openai",
				Protocol: "openai-compatible",
				Enabled:  true,
				SecretRef: &localworkspace.SecretRef{
					Type: localworkspace.SecretRefTypeEnv,
					Name: "OPENAI_API_KEY",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if err := localworkspace.WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	projectRoot := filepath.Join(t.TempDir(), "project-root")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("create project root: %v", err)
	}
	project, err := localworkspace.NewProject(workspace, localworkspace.ProjectData{
		Name:     "CLI Project",
		Kind:     "video",
		Adapter:  "filesystem",
		RootPath: projectRoot,
		Capabilities: localworkspace.ProjectCapabilities{
			FSRead:        true,
			ArtifactWrite: true,
		},
		AdapterMetadata: map[string]any{
			"adapterVersion": "test",
		},
		CredentialRefs: map[string]localworkspace.SecretRef{
			"git": {
				Type: localworkspace.SecretRefTypeEnv,
				Name: "GIT_TOKEN",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if err := localworkspace.WriteProject(workspace, project); err != nil {
		t.Fatalf("WriteProject() error = %v", err)
	}
	asset, err := localworkspace.NewAsset(workspace, localworkspace.AssetData{
		Type:             "image",
		MIME:             "image/png",
		Title:            "CLI Asset",
		MediaType:        "image",
		CategoryPath:     "cli/test",
		Purpose:          "reference",
		Source:           "workspace",
		SourceArtifactID: artifact.ID,
		Privacy:          localworkspace.PrivacyPrivate,
		Tags:             []string{"cli"},
		Files: map[string]string{
			"original": "files/original.png",
		},
	})
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	if err := localworkspace.WriteAsset(workspace, asset); err != nil {
		t.Fatalf("WriteAsset() error = %v", err)
	}
	if err := localworkspace.AtomicWriteFile(filepath.Join(localworkspace.AssetRepository(workspace).Dir(asset.ID), "files", "original.png"), []byte("png"), 0o600); err != nil {
		t.Fatalf("write asset file: %v", err)
	}
	prompt, err := localworkspace.NewPrompt(workspace, localworkspace.PromptData{
		Title:    "CLI Prompt",
		Kind:     "system",
		Category: "agent",
		Domain:   "workflow",
		Stage:    "draft",
		Provider: "local",
		Model:    "gpt-test",
		Mode:     "chat",
		Status:   "active",
		Privacy:  localworkspace.PrivacyPrivate,
		Tags:     []string{"cli"},
	})
	if err != nil {
		t.Fatalf("NewPrompt() error = %v", err)
	}
	if err := localworkspace.SavePrompt(workspace, prompt, "Use the CLI workspace."); err != nil {
		t.Fatalf("SavePrompt() error = %v", err)
	}
	return querySeed{
		TemplateID:  template.ID,
		RunID:       runDoc.ID,
		ArtifactID:  artifact.ID,
		ProfileID:   profile.ID,
		ProjectID:   project.ID,
		AssetID:     asset.ID,
		PromptID:    prompt.ID,
		ProjectRoot: projectRoot,
	}
}

type cancelAfterLineWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
	lines  int
}

func (w *cancelAfterLineWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.lines += strings.Count(string(p), "\n")
	if w.lines >= 1 {
		w.cancel()
	}
	return n, err
}

type cancelOnSubstringWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
	needle string
}

func (w *cancelOnSubstringWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(w.Buffer.String(), w.needle) {
		w.cancel()
	}
	return n, err
}

func testCLIEnvelope[T any](id string, kind string, data T) localworkspace.Envelope[T] {
	now := time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC).Format(time.RFC3339)
	return localworkspace.Envelope[T]{
		SchemaVersion: localworkspace.SchemaVersion,
		Kind:          kind,
		ID:            id,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Data:          data,
	}
}
