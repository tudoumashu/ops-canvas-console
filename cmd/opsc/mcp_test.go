package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/basketikun/infinite-canvas/internal/localworkspace"
)

func TestMCPInitializeAndToolsList(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list"}`,
		"",
	}, "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	responses := decodeMCPResponses(t, stdout.String())
	if len(responses) != 2 {
		t.Fatalf("responses = %d, want 2\n%s", len(responses), stdout.String())
	}
	initializeResult := responses[0]["result"].(map[string]any)
	serverInfo := initializeResult["serverInfo"].(map[string]any)
	if serverInfo["name"] != mcpServerName {
		t.Fatalf("serverInfo.name = %#v", serverInfo["name"])
	}
	toolsResult := responses[1]["result"].(map[string]any)
	tools := toolsResult["tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("tools/list returned no tools")
	}
	if !mcpToolListContains(tools, "opsc_workspace_info") || !mcpToolListContains(tools, "opsc_run_status") {
		t.Fatalf("tools/list missing expected tools: %#v", tools)
	}
	for _, forbidden := range []string{
		"opsc_template_create",
		"opsc_template_update",
		"opsc_template_delete",
		"opsc_run_create",
		"opsc_run_update",
		"opsc_run_delete",
		"opsc_artifact_create",
		"opsc_artifact_update",
		"opsc_artifact_delete",
		"opsc_profile_create",
		"opsc_profile_update",
		"opsc_profile_delete",
		"opsc_project_create",
		"opsc_project_update",
		"opsc_project_delete",
		"opsc_asset_create",
		"opsc_asset_update",
		"opsc_asset_delete",
		"opsc_prompt_create",
		"opsc_prompt_update",
		"opsc_prompt_delete",
	} {
		if mcpToolListContains(tools, forbidden) {
			t.Fatalf("tools/list exposes forbidden object mutation tool %q", forbidden)
		}
	}
	for _, item := range tools {
		tool := item.(map[string]any)
		name := tool["name"].(string)
		for _, forbiddenWord := range []string{"_create", "_update", "_delete", "_write", "_import", "_attach", "_append"} {
			if strings.Contains(name, forbiddenWord) {
				t.Fatalf("tools/list exposes write-like object tool %q", name)
			}
		}
	}
	if !mcpToolListContains(tools, "opsc_workspace_index_rebuild") {
		t.Fatalf("tools/list missing maintenance index rebuild tool: %#v", tools)
	}
}

func TestMCPWorkspaceInfoToolWrapsCLIWithoutPathLeak(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	seedQueryWorkspace(t, root)
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"opsc_workspace_info","arguments":{"workspace":` + jsonString(root) + `}}}` + "\n"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("MCP workspace info leaked workspace path:\n%s", stdout.String())
	}
	responses := decodeMCPResponses(t, stdout.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("result.isError = %#v\n%s", result["isError"], stdout.String())
	}
	if !strings.Contains(stdout.String(), "Query Workspace") {
		t.Fatalf("MCP workspace info missing workspace name:\n%s", stdout.String())
	}
}

func TestMCPDoctorExportAndGCPlanTools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	seed := seedQueryWorkspace(t, root)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"doctor","method":"tools/call","params":{"name":"opsc_workspace_doctor","arguments":{"workspace":` + jsonString(root) + `}}}`,
		`{"jsonrpc":"2.0","id":"export","method":"tools/call","params":{"name":"opsc_workspace_export_plan","arguments":{"workspace":` + jsonString(root) + `}}}`,
		`{"jsonrpc":"2.0","id":"gc","method":"tools/call","params":{"name":"opsc_workspace_gc_plan","arguments":{"workspace":` + jsonString(root) + `}}}`,
		"",
	}, "\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	output := stdout.String()
	if strings.Contains(output, root) || strings.Contains(output, seed.ProjectRoot) {
		t.Fatalf("MCP diagnostic plan output leaked sensitive path:\n%s", output)
	}
	for _, want := range []string{`"includePaths"`, `"excludePaths"`, `"candidates"`, "Workspace OK"} {
		if !strings.Contains(output, want) {
			t.Fatalf("MCP diagnostic plan output missing %q:\n%s", want, output)
		}
	}
	responses := decodeMCPResponses(t, output)
	if len(responses) != 3 {
		t.Fatalf("responses = %d, want 3\n%s", len(responses), output)
	}
	for _, response := range responses {
		result := response["result"].(map[string]any)
		if result["isError"] != false {
			t.Fatalf("result.isError = %#v, want false\n%s", result["isError"], output)
		}
	}
}

func TestMCPDoctorUnhealthyIsToolError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Init(localworkspace.InitOptions{Path: root, Name: "Unhealthy Workspace"}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runs")); err != nil {
		t.Fatalf("remove runs dir: %v", err)
	}
	input := `{"jsonrpc":"2.0","id":"doctor","method":"tools/call","params":{"name":"opsc_workspace_doctor","arguments":{"workspace":` + jsonString(root) + `}}}` + "\n"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	if strings.Contains(output, root) {
		t.Fatalf("MCP unhealthy doctor leaked workspace path:\n%s", output)
	}
	if !strings.Contains(output, "dir:runs") || !strings.Contains(output, "Workspace has problems") {
		t.Fatalf("MCP unhealthy doctor missing diagnostic text:\n%s", output)
	}
	responses := decodeMCPResponses(t, output)
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("result.isError = %#v, want true\n%s", result["isError"], output)
	}
}

func TestMCPIndexRebuildRequiresServe(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	seedQueryWorkspace(t, root)
	input := `{"jsonrpc":"2.0","id":"index-rebuild","method":"tools/call","params":{"name":"opsc_workspace_index_rebuild","arguments":{"workspace":` + jsonString(root) + `}}}` + "\n"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), root) || strings.Contains(strings.ToLower(stdout.String()), "bearer") {
		t.Fatalf("inactive serve result leaked sensitive details:\n%s", stdout.String())
	}
	responses := decodeMCPResponses(t, stdout.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("result.isError = %#v, want true\n%s", result["isError"], stdout.String())
	}
	if !strings.Contains(stdout.String(), "opsc serve is not active") {
		t.Fatalf("inactive serve result missing actionable error:\n%s", stdout.String())
	}
}

func TestMCPIndexRebuildUsesServe(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	seedQueryWorkspace(t, root)
	runtime, stopServe := startMCPServe(t, root)
	defer stopServe()
	token := readMCPServeTokenForTest(t, root)
	input := `{"jsonrpc":"2.0","id":"index-rebuild","method":"tools/call","params":{"name":"opsc_workspace_index_rebuild","arguments":{"workspace":` + jsonString(root) + `}}}` + "\n"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	output := stdout.String()
	if strings.Contains(output, root) || strings.Contains(output, token) || strings.Contains(strings.ToLower(output), "bearer.token") {
		t.Fatalf("active serve result leaked sensitive details:\n%s", output)
	}
	if strings.Contains(output, runtime.BaseURL) {
		t.Fatalf("active serve result leaked serve base URL:\n%s", output)
	}
	responses := decodeMCPResponses(t, output)
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("result.isError = %#v\n%s", result["isError"], output)
	}
	if !strings.Contains(output, `"entries"`) || !strings.Contains(output, `"code":0`) {
		t.Fatalf("index rebuild result missing serve response:\n%s", output)
	}
}

func TestMCPRunStatusTool(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	seed := seedQueryWorkspace(t, root)
	input := `{"jsonrpc":"2.0","id":"run-status","method":"tools/call","params":{"name":"opsc_run_status","arguments":{"workspace":` + jsonString(root) + `,"runId":` + jsonString(seed.RunID) + `}}}` + "\n"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), seed.ProjectRoot) {
		t.Fatalf("MCP run status leaked sensitive path:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), seed.RunID) || !strings.Contains(stdout.String(), `"latestEventSequence"`) {
		t.Fatalf("MCP run status missing run details:\n%s", stdout.String())
	}
	responses := decodeMCPResponses(t, stdout.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("result.isError = %#v\n%s", result["isError"], stdout.String())
	}
}

func TestMCPRunEventsToolReturnsJSONLText(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	seed := seedQueryWorkspace(t, root)
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"opsc_run_events","arguments":{"workspace":` + jsonString(root) + `,"runId":` + jsonString(seed.RunID) + `}}}` + "\n"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"type":"run.created"`) || !strings.Contains(stdout.String(), seed.RunID) {
		t.Fatalf("MCP run events missing JSONL event:\n%s", stdout.String())
	}
}

func TestMCPUnknownToolReturnsInvalidParamsError(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"opsc_missing","arguments":{}}}` + "\n"
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMCPServer(context.Background(), cliOptions{}, strings.NewReader(input), &stdout, &stderr); code != 0 {
		t.Fatalf("runMCPServer exit = %d stderr=%s", code, stderr.String())
	}
	responses := decodeMCPResponses(t, stdout.String())
	errObj := responses[0]["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("error.code = %#v\n%s", errObj["code"], stdout.String())
	}
}

func decodeMCPResponses(t *testing.T, output string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("response is not json: %v\n%s", err, output)
		}
		responses = append(responses, response)
	}
	return responses
}

func mcpToolListContains(tools []any, name string) bool {
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if tool["name"] == name {
			return true
		}
	}
	return false
}

func startMCPServe(t *testing.T, root string) (localworkspace.ServeRuntimeInfo, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan localworkspace.ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- localworkspace.Serve(ctx, localworkspace.ServeOptions{
			WorkspacePath: root,
			Host:          localworkspace.DefaultServeHost,
			Port:          0,
			Ready: func(runtime localworkspace.ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	select {
	case runtime := <-ready:
		return runtime, func() {
			cancel()
			select {
			case err := <-errCh:
				if err != nil {
					t.Errorf("Serve() shutdown error = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Error("timeout waiting for opsc serve shutdown")
			}
		}
	case err := <-errCh:
		cancel()
		t.Fatalf("Serve() returned before ready: %v", err)
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("timeout waiting for opsc serve ready")
	}
	return localworkspace.ServeRuntimeInfo{}, func() {}
}

func readMCPServeTokenForTest(t *testing.T, root string) string {
	t.Helper()
	workspace, err := localworkspace.Open(root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
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

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
