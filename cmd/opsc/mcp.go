package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/internal/localworkspace"
)

const (
	mcpProtocolVersion = "2025-06-18"
	mcpServerName      = "opsc-local-workspace"
	mcpServerVersion   = "0.1.0"
)

type mcpRequest struct {
	ID     json.RawMessage
	HasID  bool
	Method string
	Params json.RawMessage
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolCallResult struct {
	Content           []mcpToolContent `json:"content"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError"`
}

type mcpToolDefinition struct {
	Name        string                                                                       `json:"name"`
	Description string                                                                       `json:"description"`
	InputSchema map[string]any                                                               `json:"inputSchema"`
	BuildArgs   func(cliOptions, map[string]any) ([]string, error)                           `json:"-"`
	Call        func(context.Context, cliOptions, map[string]any) (mcpToolCallResult, error) `json:"-"`
}

func runMCPCommand(ctx context.Context, opts cliOptions, stdout io.Writer, stderr io.Writer) int {
	if len(opts.Command) != 1 {
		return writeError(stderr, opts.JSON, localworkspace.NewError(localworkspace.ErrorInvalidArgument, "mcp does not accept subcommands", 1, nil))
	}
	return runMCPServer(ctx, opts, os.Stdin, stdout, stderr)
}

func runMCPServer(ctx context.Context, opts cliOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	encoder := json.NewEncoder(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		response := handleMCPRequest(ctx, opts, []byte(line))
		if response == nil {
			continue
		}
		if err := encoder.Encode(response); err != nil {
			fmt.Fprintf(stderr, "mcp write error: %v\n", err)
			return 1
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(stderr, "mcp read error: %v\n", err)
		return 1
	}
	return 0
}

func handleMCPRequest(ctx context.Context, opts cliOptions, data []byte) *mcpResponse {
	request, err := parseMCPRequest(data)
	if err != nil {
		return newMCPResponse(json.RawMessage("null"), nil, &mcpError{Code: -32700, Message: "parse error", Data: err.Error()})
	}
	if !request.HasID && strings.HasPrefix(request.Method, "notifications/") {
		return nil
	}
	if !request.HasID {
		return nil
	}
	switch request.Method {
	case "initialize":
		return newMCPResponse(request.ID, mcpInitializeResult(request.Params), nil)
	case "ping":
		return newMCPResponse(request.ID, map[string]any{}, nil)
	case "tools/list":
		return newMCPResponse(request.ID, map[string]any{"tools": publicMCPTools()}, nil)
	case "tools/call":
		result, callErr := callMCPTool(ctx, opts, request.Params)
		if callErr != nil {
			return newMCPResponse(request.ID, nil, &mcpError{Code: -32602, Message: callErr.Error()})
		}
		return newMCPResponse(request.ID, result, nil)
	default:
		return newMCPResponse(request.ID, nil, &mcpError{Code: -32601, Message: "method not found: " + request.Method})
	}
}

func parseMCPRequest(data []byte) (mcpRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return mcpRequest{}, err
	}
	var method string
	if err := json.Unmarshal(raw["method"], &method); err != nil || strings.TrimSpace(method) == "" {
		return mcpRequest{}, errors.New("missing method")
	}
	request := mcpRequest{Method: method}
	if id, ok := raw["id"]; ok {
		request.ID = id
		request.HasID = true
	} else {
		request.ID = json.RawMessage("null")
	}
	if params, ok := raw["params"]; ok {
		request.Params = params
	}
	return request, nil
}

func newMCPResponse(id json.RawMessage, result any, err *mcpError) *mcpResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &mcpResponse{JSONRPC: "2.0", ID: id, Result: result, Error: err}
}

func mcpInitializeResult(params json.RawMessage) map[string]any {
	version := mcpProtocolVersion
	if len(params) > 0 {
		var payload struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &payload); err == nil && strings.TrimSpace(payload.ProtocolVersion) != "" {
			version = payload.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    mcpServerName,
			"version": mcpServerVersion,
		},
		"instructions": "Use these tools to inspect the local opsc workspace. Tools wrap existing opsc CLI/core commands and do not expose secrets by default.",
	}
}

func publicMCPTools() []mcpToolDefinition {
	definitions := mcpTools()
	tools := make([]mcpToolDefinition, len(definitions))
	copy(tools, definitions)
	return tools
}

func callMCPTool(ctx context.Context, opts cliOptions, params json.RawMessage) (mcpToolCallResult, error) {
	if len(params) == 0 {
		return mcpToolCallResult{}, errors.New("tools/call params are required")
	}
	var payload mcpToolCallParams
	if err := json.Unmarshal(params, &payload); err != nil {
		return mcpToolCallResult{}, err
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		return mcpToolCallResult{}, errors.New("tool name is required")
	}
	if payload.Arguments == nil {
		payload.Arguments = map[string]any{}
	}
	for _, tool := range mcpTools() {
		if tool.Name != payload.Name {
			continue
		}
		if tool.Call != nil {
			return tool.Call(ctx, opts, payload.Arguments)
		}
		if tool.BuildArgs == nil {
			return mcpToolCallResult{}, fmt.Errorf("tool %s is not configured", tool.Name)
		}
		cliArgs, err := tool.BuildArgs(opts, payload.Arguments)
		if err != nil {
			return mcpToolCallResult{}, err
		}
		return runMCPCLITool(ctx, cliArgs), nil
	}
	return mcpToolCallResult{}, fmt.Errorf("unknown tool: %s", payload.Name)
}

func runMCPCLITool(ctx context.Context, args []string) mcpToolCallResult {
	return runMCPCLIToolWithStdoutTransform(ctx, args, nil)
}

func runMCPCLIToolWithStdoutTransform(ctx context.Context, args []string, transform func([]byte) []byte) mcpToolCallResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithContext(ctx, args, &stdout, &stderr)
	stdoutBytes := stdout.Bytes()
	if transform != nil && exitCode == 0 {
		stdoutBytes = transform(stdoutBytes)
	}
	stdoutText := strings.TrimSpace(string(stdoutBytes))
	stderrText := strings.TrimSpace(stderr.String())
	text := stdoutText
	if stderrText != "" {
		if text != "" {
			text += "\n\nstderr:\n"
		}
		text += stderrText
	}
	if text == "" {
		text = "ok"
	}
	structured := map[string]any{
		"exitCode": exitCode,
	}
	if parsed, ok := parseJSONValue(stdoutBytes); ok {
		structured["stdout"] = parsed
	}
	if parsed, ok := parseJSONValue(stderr.Bytes()); ok {
		structured["stderr"] = parsed
	} else if stderrText != "" {
		structured["stderrText"] = stderrText
	}
	return mcpToolCallResult{
		Content: []mcpToolContent{
			{Type: "text", Text: text},
		},
		StructuredContent: structured,
		IsError:           exitCode != 0,
	}
}

func parseJSONValue(data []byte) (any, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

func mcpTools() []mcpToolDefinition {
	return []mcpToolDefinition{
		{
			Name:        "opsc_workspace_info",
			Description: "Return sanitized local workspace metadata.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
			}, nil),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"workspace", "info", "--json"})
			},
			Call: func(ctx context.Context, opts cliOptions, args map[string]any) (mcpToolCallResult, error) {
				cliArgs, err := mcpCLIArgs(opts, args, []string{"workspace", "info", "--json"})
				if err != nil {
					return mcpToolCallResult{}, err
				}
				return runMCPCLIToolWithStdoutTransform(ctx, cliArgs, sanitizeMCPWorkspaceInfoStdout), nil
			},
		},
		{
			Name:        "opsc_workspace_doctor",
			Description: "Run workspace diagnostics and return the doctor report.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
			}, nil),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"workspace", "doctor", "--json"})
			},
		},
		{
			Name:        "opsc_workspace_index_rebuild",
			Description: "Rebuild the derived index.sqlite through the active opsc serve single-writer API.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
			}, nil),
			Call: callMCPIndexRebuild,
		},
		{
			Name:        "opsc_workspace_export_plan",
			Description: "Return the local workspace export plan without exporting files.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
			}, nil),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"workspace", "export", "plan", "--json"})
			},
		},
		{
			Name:        "opsc_workspace_gc_plan",
			Description: "Return the local workspace GC dry-run plan. It never deletes files.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
			}, nil),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"workspace", "gc", "plan", "--json"})
			},
		},
		{
			Name:        "opsc_template_list",
			Description: "List local workspace workflow template summaries.",
			InputSchema: mcpWorkspaceOnlySchema(),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"template", "list", "--json"})
			},
		},
		{
			Name:        "opsc_run_list",
			Description: "List local workspace run summaries.",
			InputSchema: mcpWorkspaceOnlySchema(),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"run", "list", "--json"})
			},
		},
		{
			Name:        "opsc_run_status",
			Description: "Return local workspace run status, node summaries, and latest event sequence.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
				"runId":     mcpStringSchema("Run id, for example run_<ULID>."),
			}, []string{"runId"}),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				runID, err := mcpRequiredStringArg(args, "runId")
				if err != nil {
					return nil, err
				}
				return mcpCLIArgs(opts, args, []string{"run", "status", runID, "--json"})
			},
		},
		{
			Name:        "opsc_run_events",
			Description: "Return local workspace run events as JSONL text. Follow mode is intentionally not exposed.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
				"runId":     mcpStringSchema("Run id, for example run_<ULID>."),
			}, []string{"runId"}),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				runID, err := mcpRequiredStringArg(args, "runId")
				if err != nil {
					return nil, err
				}
				return mcpCLIArgs(opts, args, []string{"run", "events", runID})
			},
		},
		{
			Name:        "opsc_artifact_list",
			Description: "List local workspace artifact summaries, optionally scoped to a run.",
			InputSchema: mcpObjectSchema(map[string]any{
				"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
				"runId":     mcpStringSchema("Optional run id for run-scoped artifact refs."),
			}, nil),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				runID, err := mcpOptionalStringArg(args, "runId")
				if err != nil {
					return nil, err
				}
				command := []string{"artifact", "list", "--json"}
				if runID != "" {
					command = []string{"artifact", "list", "--run", runID, "--json"}
				}
				return mcpCLIArgs(opts, args, command)
			},
		},
		{
			Name:        "opsc_profile_list",
			Description: "List local workspace profile summaries without secret values.",
			InputSchema: mcpWorkspaceOnlySchema(),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"profile", "list", "--json"})
			},
		},
		{
			Name:        "opsc_project_list",
			Description: "List local workspace project summaries without root paths.",
			InputSchema: mcpWorkspaceOnlySchema(),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"project", "list", "--json"})
			},
		},
		{
			Name:        "opsc_asset_list",
			Description: "List local workspace private asset summaries.",
			InputSchema: mcpWorkspaceOnlySchema(),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"asset", "list", "--json"})
			},
		},
		{
			Name:        "opsc_prompt_list",
			Description: "List local workspace private prompt summaries.",
			InputSchema: mcpWorkspaceOnlySchema(),
			BuildArgs: func(opts cliOptions, args map[string]any) ([]string, error) {
				return mcpCLIArgs(opts, args, []string{"prompt", "list", "--json"})
			},
		},
	}
}

func sanitizeMCPWorkspaceInfoStdout(data []byte) []byte {
	var envelope map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &envelope); err != nil {
		return data
	}
	body, ok := envelope["data"].(map[string]any)
	if !ok {
		return data
	}
	runtime, ok := body["runtime"].(map[string]any)
	if !ok {
		return data
	}
	active, _ := runtime["active"].(bool)
	body["runtime"] = map[string]any{"active": active}
	sanitized, err := json.Marshal(envelope)
	if err != nil {
		return data
	}
	return sanitized
}

func mcpCLIArgs(opts cliOptions, args map[string]any, command []string) ([]string, error) {
	workspace, err := mcpWorkspaceArg(opts, args)
	if err != nil {
		return nil, err
	}
	cliArgs := make([]string, 0, len(command)+2)
	if workspace != "" {
		cliArgs = append(cliArgs, "--workspace", workspace)
	}
	cliArgs = append(cliArgs, command...)
	return cliArgs, nil
}

func callMCPIndexRebuild(ctx context.Context, opts cliOptions, args map[string]any) (mcpToolCallResult, error) {
	workspacePath, err := mcpWorkspaceArg(opts, args)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	workspace, err := localworkspace.Open(workspacePath)
	if err != nil {
		return mcpToolErrorResult("open workspace failed; run opsc workspace info --json to verify the workspace"), nil
	}
	runtime := workspace.Info(false).Runtime
	if !runtime.Active || strings.TrimSpace(runtime.BaseURL) == "" {
		return mcpToolErrorResult("opsc serve is not active for this workspace; start opsc serve and retry"), nil
	}
	if err := validateMCPLoopbackBaseURL(runtime.BaseURL); err != nil {
		return mcpToolErrorResult("opsc serve runtime is not loopback; restart opsc serve on 127.0.0.1"), nil
	}
	token, err := readMCPServeToken(*workspace, runtime.TokenFile)
	if err != nil {
		return mcpToolErrorResult("opsc serve bearer token is unavailable; restart opsc serve and retry"), nil
	}
	endpoint := strings.TrimRight(runtime.BaseURL, "/") + "/api/local/workspace/index/rebuild"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return mcpToolErrorResult("build opsc serve request failed"), nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return mcpToolErrorResult("call opsc serve index rebuild failed; verify opsc serve is running"), nil
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return mcpToolErrorResult("read opsc serve response failed"), nil
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = fmt.Sprintf("opsc serve returned HTTP %d", resp.StatusCode)
	}
	structured := map[string]any{
		"httpStatus": resp.StatusCode,
	}
	if parsed, ok := parseJSONValue(body); ok {
		structured["response"] = parsed
	}
	isError := resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices
	return mcpToolCallResult{
		Content: []mcpToolContent{
			{Type: "text", Text: text},
		},
		StructuredContent: structured,
		IsError:           isError,
	}, nil
}

func mcpWorkspaceArg(opts cliOptions, args map[string]any) (string, error) {
	workspace, err := mcpOptionalStringArg(args, "workspace")
	if err != nil {
		return "", err
	}
	if workspace == "" {
		workspace = strings.TrimSpace(opts.Workspace)
	}
	return workspace, nil
}

func readMCPServeToken(workspace localworkspace.Workspace, tokenFile string) (string, error) {
	tokenFile = strings.TrimSpace(tokenFile)
	if tokenFile == "" || filepath.IsAbs(tokenFile) || tokenFile != filepath.Base(tokenFile) {
		return "", errors.New("invalid token file")
	}
	path, err := workspace.StatePath(tokenFile)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("empty token")
	}
	return token, nil
}

func validateMCPLoopbackBaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" {
		return errors.New("invalid base url")
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("non-loopback base url")
	}
	return nil
}

func mcpToolErrorResult(message string) mcpToolCallResult {
	return mcpToolCallResult{
		Content: []mcpToolContent{
			{Type: "text", Text: message},
		},
		StructuredContent: map[string]any{
			"error": message,
		},
		IsError: true,
	}
}

func mcpWorkspaceOnlySchema() map[string]any {
	return mcpObjectSchema(map[string]any{
		"workspace": mcpStringSchema("Optional workspace directory. Defaults to --workspace, OPSC_WORKSPACE, or ~/OpsCanvas."),
	}, nil)
}

func mcpObjectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mcpStringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func mcpRequiredStringArg(args map[string]any, name string) (string, error) {
	value, err := mcpOptionalStringArg(args, name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func mcpOptionalStringArg(args map[string]any, name string) (string, error) {
	raw, ok := args[name]
	if !ok || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return strings.TrimSpace(value), nil
}
