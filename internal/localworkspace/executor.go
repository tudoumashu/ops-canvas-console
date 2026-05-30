package localworkspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	osexec "os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	executorID = "opsc-executor"

	executorEventClaimed       = "executor.run.claimed"
	executorEventResumed       = "executor.run.resumed"
	executorEventNodeStarted   = "executor.node.started"
	executorEventNodeCompleted = "executor.node.completed"
	executorEventNodeFailed    = "executor.node.failed"
	executorEventNodeRetrying  = "executor.node.retrying"
	executorEventNodeSkipped   = "executor.node.skipped"
	executorEventRunCompleted  = "executor.run.completed"
	executorEventRunFailed     = "executor.run.failed"
)

var templateVarPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

type ExecutorOptions struct {
	WorkspacePath string
	RunID         string
	HTTPClient    *http.Client
	Now           func() time.Time
}

type ExecutorResult struct {
	WorkspaceID string              `json:"workspaceId"`
	Processed   int                 `json:"processed"`
	Runs        []ExecutorRunResult `json:"runs"`
	Warnings    []string            `json:"warnings,omitempty"`
}

type ExecutorRunResult struct {
	RunID        string `json:"runId"`
	Status       string `json:"status"`
	Executed     int    `json:"executed"`
	Skipped      int    `json:"skipped"`
	ArtifactRefs int    `json:"artifactRefs"`
	Error        string `json:"error,omitempty"`
}

type executorNode struct {
	ID             string                  `json:"id"`
	Type           string                  `json:"type"`
	Title          string                  `json:"title"`
	Operation      string                  `json:"operation"`
	Model          string                  `json:"model"`
	Prompt         string                  `json:"prompt"`
	Count          int                     `json:"count"`
	Size           string                  `json:"size"`
	Quality        string                  `json:"quality"`
	Retry          *executorRetry          `json:"retry,omitempty"`
	Extra          map[string]any          `json:"extra"`
	OutputMappings []executorOutputMapping `json:"outputMappings,omitempty"`
}

type executorRetry struct {
	Enabled         *bool `json:"enabled,omitempty"`
	RetryCount      int   `json:"retryCount,omitempty"`
	IntervalSeconds int   `json:"intervalSeconds,omitempty"`
}

type executorOutputMapping struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type executorEdge struct {
	ID         string         `json:"id"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Source     string         `json:"source,omitempty"`
	Target     string         `json:"target,omitempty"`
	FromHandle string         `json:"fromHandle,omitempty"`
	Condition  map[string]any `json:"condition,omitempty"`
}

type executorContext struct {
	workspace Workspace
	run       Envelope[RunData]
	template  Envelope[TemplateData]
	project   *Envelope[ProjectData]
	input     map[string]any
	nodeOut   map[string]map[string]any
	client    *http.Client
	now       func() time.Time
}

func RunExecutorOnce(ctx context.Context, opts ExecutorOptions) (ExecutorResult, error) {
	workspace, err := Open(opts.WorkspacePath)
	if err != nil {
		return ExecutorResult{}, err
	}
	lock, err := acquireWorkspaceWriteLock(workspace.LockPath("executor.lock"), 2*time.Second)
	if err != nil {
		return ExecutorResult{}, err
	}
	defer lock.Release()

	result := ExecutorResult{WorkspaceID: workspace.Document.ID}
	runs, err := executorCandidateRuns(*workspace, opts.RunID)
	if err != nil {
		return ExecutorResult{}, err
	}
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		eligible, err := executorRunEligible(*workspace, run)
		if err != nil {
			return result, err
		}
		if !eligible {
			continue
		}
		runResult, err := executeLocalRun(ctx, *workspace, run, opts)
		if err != nil {
			return result, err
		}
		result.Runs = append(result.Runs, runResult)
		result.Processed++
	}
	return result, nil
}

func executorCandidateRuns(workspace Workspace, runID string) ([]Envelope[RunData], error) {
	if strings.TrimSpace(runID) != "" {
		run, err := ReadRun(workspace, runID)
		if err != nil {
			return nil, err
		}
		return []Envelope[RunData]{run}, nil
	}
	runs, err := ListRuns(workspace)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(runs, func(i int, j int) bool {
		if runs[i].CreatedAt != runs[j].CreatedAt {
			return runs[i].CreatedAt < runs[j].CreatedAt
		}
		return runs[i].ID < runs[j].ID
	})
	return runs, nil
}

func executorRunEligible(workspace Workspace, run Envelope[RunData]) (bool, error) {
	switch run.Data.Status {
	case RunStatusPending:
		return runHasEvent(workspace, run.ID, "run.waiting_for_executor")
	case RunStatusRunning:
		if fmt.Sprint(run.Data.Metadata["executor"]) == executorID {
			return true, nil
		}
		return runHasEvent(workspace, run.ID, executorEventClaimed)
	default:
		return false, nil
	}
}

func runHasEvent(workspace Workspace, runID string, eventType string) (bool, error) {
	events, err := ReadRunEvents(workspace, runID, 0)
	if err != nil {
		return false, err
	}
	for _, event := range events {
		if event.Type == eventType {
			return true, nil
		}
	}
	return false, nil
}

func executeLocalRun(ctx context.Context, workspace Workspace, run Envelope[RunData], opts ExecutorOptions) (ExecutorRunResult, error) {
	runResult := ExecutorRunResult{RunID: run.ID, Status: run.Data.Status}
	template, err := readExecutorTemplate(workspace, run)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	nodes, err := parseExecutorNodes(template.Data.Nodes)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	edges, err := parseExecutorEdges(template.Data.Edges)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	ordered, err := topologicalExecutorNodes(nodes, edges)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	if run.Data.Status == RunStatusPending {
		if run, err = updateExecutorRun(workspace, run, RunStatusRunning, nil, ""); err != nil {
			return runResult, err
		}
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventClaimed, "info", "Local executor claimed run", map[string]any{"templateId": run.Data.TemplateID}); err != nil {
			return runResult, err
		}
	} else if _, err := appendExecutorEvent(workspace, run.ID, executorEventResumed, "info", "Local executor resumed run", nil); err != nil {
		return runResult, err
	}

	states, err := executorNodeStateMap(workspace, run.ID)
	if err != nil {
		return runResult, err
	}
	project, err := executorProject(workspace, run)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	execCtx := executorContext{
		workspace: workspace,
		run:       run,
		template:  template,
		project:   project,
		input:     executorRunInput(run.Data.Input),
		nodeOut:   executorNodeOutputMap(states),
		client:    opts.HTTPClient,
		now:       opts.Now,
	}
	if execCtx.client == nil {
		execCtx.client = http.DefaultClient
	}
	if execCtx.now == nil {
		execCtx.now = time.Now
	}

	for _, node := range ordered {
		if err := ctx.Err(); err != nil {
			return runResult, err
		}
		current := states[node.ID]
		switch current.Data.Status {
		case RunStatusSuccess:
			runResult.Skipped++
			continue
		case RunStatusError:
			err := errors.New(nonEmptyString(current.Data.Error, "node already failed"))
			return failNodeAndRunResult(workspace, run, runResult, node, err, current)
		}
		if dependencyFailed(states, edges, node.ID) {
			err := errors.New("upstream node failed")
			return failNodeAndRunResult(workspace, run, runResult, node, err, current)
		}
		if skipped, reason := conditionRouteSkipped(states, edges, node.ID); skipped {
			finishedAt := executorNow(execCtx.now)
			output := map[string]any{"skipped": true, "reason": reason}
			success, err := writeExecutorNodeState(workspace, run.ID, current, RunNodeStateData{
				NodeID:     node.ID,
				Status:     RunStatusSuccess,
				StartedAt:  finishedAt,
				FinishedAt: finishedAt,
				Output:     output,
				Metadata:   executorNodeMetadata(node),
			})
			if err != nil {
				return runResult, err
			}
			states[node.ID] = success
			execCtx.nodeOut[node.ID] = output
			runResult.Skipped++
			if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeSkipped, "info", "Node skipped", map[string]any{"nodeId": node.ID, "operation": node.Operation, "reason": reason}); err != nil {
				return runResult, err
			}
			continue
		}
		if dependencyPending(states, edges, node.ID) {
			err := errors.New("upstream node is not ready")
			return failNodeAndRunResult(workspace, run, runResult, node, err, current)
		}
		startedAt := executorNow(execCtx.now)
		running, err := writeExecutorNodeState(workspace, run.ID, current, RunNodeStateData{
			NodeID:    node.ID,
			Status:    RunStatusRunning,
			StartedAt: startedAt,
			Metadata:  executorNodeMetadata(node),
		})
		if err != nil {
			return runResult, err
		}
		states[node.ID] = running
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeStarted, "info", "Node started", map[string]any{"nodeId": node.ID, "operation": node.Operation}); err != nil {
			return runResult, err
		}
		output, err := executeLocalNodeWithRetry(ctx, execCtx, node, runResult.Executed)
		if err != nil {
			return failNodeAndRunResult(workspace, run, runResult, node, err, running)
		}
		finishedAt := executorNow(execCtx.now)
		success, err := writeExecutorNodeState(workspace, run.ID, running, RunNodeStateData{
			NodeID:     node.ID,
			Status:     RunStatusSuccess,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Output:     output,
			Metadata:   executorNodeMetadata(node),
		})
		if err != nil {
			return runResult, err
		}
		states[node.ID] = success
		execCtx.nodeOut[node.ID] = output
		runResult.Executed++
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeCompleted, "info", "Node completed", map[string]any{"nodeId": node.ID, "operation": node.Operation}); err != nil {
			return runResult, err
		}
	}
	output := map[string]any{"completedNodes": len(ordered)}
	run, err = updateExecutorRun(workspace, run, RunStatusSuccess, output, "")
	if err != nil {
		return runResult, err
	}
	if _, err := appendExecutorEvent(workspace, run.ID, executorEventRunCompleted, "info", "Run completed", output); err != nil {
		return runResult, err
	}
	refs, err := listRunArtifactRefs(workspace, run.ID)
	if err != nil {
		return runResult, err
	}
	runResult.Status = RunStatusSuccess
	runResult.ArtifactRefs = len(refs)
	return runResult, nil
}

func readExecutorTemplate(workspace Workspace, run Envelope[RunData]) (Envelope[TemplateData], error) {
	snapshotPath := filepath.Join(workspace.Root, "runs", run.ID, "template.snapshot.json")
	if snapshot, err := readEnvelopeFile[TemplateData](snapshotPath); err == nil {
		return snapshot, nil
	} else {
		var workspaceErr *Error
		if !asLocalWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceNotFound {
			return Envelope[TemplateData]{}, err
		}
	}
	if strings.TrimSpace(run.Data.TemplateID) == "" {
		return Envelope[TemplateData]{}, NewError(ErrorWorkspaceInvalid, "run templateId is empty", 2, map[string]string{"runId": run.ID})
	}
	return ReadTemplate(workspace, run.Data.TemplateID)
}

func executorProject(workspace Workspace, run Envelope[RunData]) (*Envelope[ProjectData], error) {
	projectID := strings.TrimSpace(run.Data.ProjectID)
	if projectID == "" {
		return nil, nil
	}
	project, err := ReadProject(workspace, projectID)
	if err != nil {
		return nil, err
	}
	if err := validateExecutorProjectAdapter(project.Data.Adapter); err != nil {
		return nil, err
	}
	if strings.TrimSpace(project.Data.RootPath) != "" && strings.TrimSpace(project.Data.RootFingerprint) != "" {
		current, err := ProjectRootFingerprint(workspace, project.Data.RootPath)
		if err != nil {
			return nil, redactExecutorProjectError(project, err)
		}
		if current != project.Data.RootFingerprint {
			return nil, NewError(ErrorWorkspaceInvalid, "project root fingerprint changed", 2, map[string]string{"projectId": project.ID})
		}
	}
	return &project, nil
}

func validateExecutorProjectAdapter(adapter string) error {
	switch strings.TrimSpace(adapter) {
	case "", "filesystem", "generic", "article-local", "video-local", "pdd-local":
		return nil
	default:
		return NewError(ErrorWorkspaceInvalid, "project adapter is not supported by local executor", 2, map[string]string{"adapter": adapter})
	}
}

func parseExecutorNodes(rawNodes []json.RawMessage) ([]executorNode, error) {
	nodes := make([]executorNode, 0, len(rawNodes))
	seen := map[string]bool{}
	for _, raw := range rawNodes {
		var node executorNode
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, WrapError(ErrorWorkspaceInvalid, "parse template node", 2, err)
		}
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" {
			return nil, NewError(ErrorWorkspaceInvalid, "template node id is empty", 2, nil)
		}
		if seen[node.ID] {
			return nil, NewError(ErrorWorkspaceInvalid, "template node id is duplicated", 2, map[string]string{"nodeId": node.ID})
		}
		seen[node.ID] = true
		node.Operation = strings.TrimSpace(node.Operation)
		if node.Operation == "" {
			node.Operation = strings.TrimSpace(node.Type)
		}
		if node.Operation == "material" {
			node.Operation = "material_lookup"
		}
		if node.Count <= 0 {
			node.Count = 1
		}
		if node.Extra == nil {
			node.Extra = map[string]any{}
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, NewError(ErrorWorkspaceInvalid, "template has no nodes", 2, nil)
	}
	return nodes, nil
}

func parseExecutorEdges(rawEdges []json.RawMessage) ([]executorEdge, error) {
	edges := make([]executorEdge, 0, len(rawEdges))
	for _, raw := range rawEdges {
		var edge executorEdge
		if err := json.Unmarshal(raw, &edge); err != nil {
			return nil, WrapError(ErrorWorkspaceInvalid, "parse template edge", 2, err)
		}
		edge.From = strings.TrimSpace(edge.From)
		edge.To = strings.TrimSpace(edge.To)
		if edge.From == "" {
			edge.From = strings.TrimSpace(edge.Source)
		}
		if edge.To == "" {
			edge.To = strings.TrimSpace(edge.Target)
		}
		edge.FromHandle = strings.TrimSpace(edge.FromHandle)
		if edge.From == "" || edge.To == "" {
			continue
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func topologicalExecutorNodes(nodes []executorNode, edges []executorEdge) ([]executorNode, error) {
	byID := map[string]executorNode{}
	inDegree := map[string]int{}
	out := map[string][]string{}
	for _, node := range nodes {
		byID[node.ID] = node
		inDegree[node.ID] = 0
	}
	for _, edge := range edges {
		if _, ok := byID[edge.From]; !ok {
			return nil, NewError(ErrorWorkspaceInvalid, "edge source node not found", 2, map[string]string{"nodeId": edge.From})
		}
		if _, ok := byID[edge.To]; !ok {
			return nil, NewError(ErrorWorkspaceInvalid, "edge target node not found", 2, map[string]string{"nodeId": edge.To})
		}
		out[edge.From] = append(out[edge.From], edge.To)
		inDegree[edge.To]++
	}
	queue := []executorNode{}
	for _, node := range nodes {
		if inDegree[node.ID] == 0 {
			queue = append(queue, node)
		}
	}
	ordered := []executorNode{}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		ordered = append(ordered, node)
		for _, target := range out[node.ID] {
			inDegree[target]--
			if inDegree[target] == 0 {
				queue = append(queue, byID[target])
			}
		}
	}
	if len(ordered) != len(nodes) {
		return nil, NewError(ErrorWorkspaceInvalid, "template graph has a cycle", 2, nil)
	}
	return ordered, nil
}

func executeLocalNode(ctx context.Context, execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	switch node.Operation {
	case "input":
		return map[string]any{"input": execCtx.input}, nil
	case "text_static":
		text := renderExecutorPrompt(node.Prompt, execCtx.input, execCtx.nodeOut)
		artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "text", "text/plain; charset=utf-8", nonEmptyString(node.Title, node.ID), []byte(text), "output", "text", order, map[string]any{
			"type":       "text_static",
			"templateId": execCtx.template.ID,
			"nodeId":     node.ID,
		})
		if err != nil {
			return nil, err
		}
		output := map[string]any{"text": text, "artifactIds": []string{artifact.ID}, "artifactId": artifact.ID}
		return applyProjectOutputMappings(execCtx, node, output, []executorGeneratedFile{{Name: "text", Data: []byte(text), MIME: "text/plain; charset=utf-8"}})
	case "material_lookup":
		return executeMaterialLookup(execCtx, node, order)
	case "condition":
		return executeCondition(execCtx, node), nil
	case "script":
		return executeProjectScript(ctx, execCtx, node, order)
	case "text_generation":
		return executeTextGeneration(ctx, execCtx, node, order)
	case "image_generation":
		return executeImageGeneration(ctx, execCtx, node, order)
	default:
		return nil, NewError(ErrorWorkspaceInvalid, "local executor does not support node operation", 2, map[string]string{"nodeId": node.ID, "operation": node.Operation})
	}
}

func executeLocalNodeWithRetry(ctx context.Context, execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	retry := normalizedExecutorRetry(node.Retry)
	attempt := 0
	for {
		output, err := executeLocalNode(ctx, execCtx, node, order)
		if err == nil {
			if attempt > 0 {
				output["retryAttempts"] = attempt
			}
			return output, nil
		}
		if !retry.Enabled || (retry.RetryCount > 0 && attempt >= retry.RetryCount) {
			return nil, err
		}
		attempt++
		delay := executorRetryDelay(retry.IntervalSeconds, attempt)
		if _, eventErr := appendExecutorEvent(execCtx.workspace, execCtx.run.ID, executorEventNodeRetrying, "warn", "Node retrying", map[string]any{
			"nodeId":    node.ID,
			"operation": node.Operation,
			"attempt":   attempt,
			"error":     redactExecutorError(execCtx, err),
		}); eventErr != nil {
			return nil, eventErr
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type executorRetryConfig struct {
	Enabled         bool
	RetryCount      int
	IntervalSeconds int
}

func normalizedExecutorRetry(retry *executorRetry) executorRetryConfig {
	if retry == nil {
		return executorRetryConfig{}
	}
	enabled := true
	if retry.Enabled != nil {
		enabled = *retry.Enabled
	}
	count := retry.RetryCount
	if count < 0 {
		count = 0
	}
	interval := retry.IntervalSeconds
	if interval < 0 {
		interval = 0
	}
	return executorRetryConfig{Enabled: enabled, RetryCount: count, IntervalSeconds: interval}
}

func executorRetryDelay(intervalSeconds int, attempt int) time.Duration {
	if intervalSeconds > 0 {
		return time.Duration(intervalSeconds) * time.Second
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(100*(1<<min(attempt-1, 5))) * time.Millisecond
	if delay > 2*time.Second {
		return 2 * time.Second
	}
	return delay
}

func executeMaterialLookup(execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	assetID := stringFromMap(node.Extra, "assetId")
	assetMode := stringFromMap(node.Extra, "assetMode")
	if assetID == "" || (assetMode != "" && assetMode != "fixed") {
		return nil, NewError(ErrorWorkspaceInvalid, "material_lookup only supports fixed local asset mode", 2, map[string]string{"nodeId": node.ID})
	}
	asset, err := ReadAsset(execCtx.workspace, assetID)
	if err != nil {
		return nil, err
	}
	if asset.Data.Type != "image" && !strings.HasPrefix(asset.Data.MIME, "image/") {
		return nil, NewError(ErrorWorkspaceInvalid, "fixed material asset must be an image", 2, map[string]string{"nodeId": node.ID, "assetId": assetID})
	}
	rel := asset.Data.Files["original"]
	if !isWorkspaceRelativeFile(rel) {
		return nil, NewError(ErrorWorkspaceInvalid, "asset original file ref is invalid", 2, map[string]string{"assetId": assetID})
	}
	filePath := filepath.Join(AssetRepository(execCtx.workspace).Dir(asset.ID), filepath.FromSlash(rel))
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewError(ErrorWorkspaceNotFound, "asset original file not found", 2, map[string]string{"assetId": assetID})
		}
		return nil, WrapError(ErrorInternal, "read asset original file", 5, err)
	}
	mimeType := nonEmptyString(asset.Data.MIME, mime.TypeByExtension(filepath.Ext(filePath)))
	mimeType = nonEmptyString(mimeType, "image/png")
	artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "image", mimeType, nonEmptyString(asset.Data.Title, nonEmptyString(node.Title, assetID)), data, "input", "material", order, map[string]any{
		"type":       "local_asset",
		"assetId":    assetID,
		"templateId": execCtx.template.ID,
		"nodeId":     node.ID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"assetId": assetID, "artifactIds": []string{artifact.ID}, "artifactId": artifact.ID}, nil
}

func executeTextGeneration(ctx context.Context, execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	prompt := renderExecutorPrompt(node.Prompt, execCtx.input, execCtx.nodeOut)
	if strings.TrimSpace(prompt) == "" {
		return nil, NewError(ErrorWorkspaceInvalid, "text_generation prompt is empty", 2, map[string]string{"nodeId": node.ID})
	}
	model := nonEmptyString(node.Model, "gpt-5.5")
	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{{
			"role":    "user",
			"content": prompt,
		}},
	}
	body, err := postLocalAIJSON(ctx, execCtx.workspace, execCtx.run.Data.ProfileID, "", execCtx.client, "/ai/v1/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	text, err := parseLocalAIText(body)
	if err != nil {
		return nil, err
	}
	artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "text", "text/plain; charset=utf-8", nonEmptyString(node.Title, "Generated text"), []byte(text), "primary_output", "text", order, map[string]any{
		"type":       "text_generation",
		"model":      model,
		"templateId": execCtx.template.ID,
		"nodeId":     node.ID,
	})
	if err != nil {
		return nil, err
	}
	output := map[string]any{"text": text, "model": model, "artifactIds": []string{artifact.ID}, "artifactId": artifact.ID}
	if parsed, ok := parseExecutorJSONObject([]byte(text)); ok {
		output["json"] = parsed
		for key, value := range parsed {
			if _, exists := output[key]; !exists {
				output[key] = value
			}
		}
	}
	return applyProjectOutputMappings(execCtx, node, output, []executorGeneratedFile{{Name: "text", Data: []byte(text), MIME: "text/plain; charset=utf-8"}})
}

func executeImageGeneration(ctx context.Context, execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	prompt := renderExecutorPrompt(node.Prompt, execCtx.input, execCtx.nodeOut)
	if strings.TrimSpace(prompt) == "" {
		return nil, NewError(ErrorWorkspaceInvalid, "image_generation prompt is empty", 2, map[string]string{"nodeId": node.ID})
	}
	model := nonEmptyString(node.Model, "gpt-image-2")
	count := node.Count
	if count <= 0 {
		count = 1
	}
	payload := map[string]any{
		"model":           model,
		"prompt":          prompt,
		"n":               count,
		"response_format": "b64_json",
	}
	if strings.TrimSpace(node.Size) != "" {
		payload["size"] = strings.TrimSpace(node.Size)
	}
	if strings.TrimSpace(node.Quality) != "" {
		payload["quality"] = strings.TrimSpace(node.Quality)
	}
	body, err := postLocalAIJSON(ctx, execCtx.workspace, execCtx.run.Data.ProfileID, "", execCtx.client, "/ai/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	images, err := parseLocalAIImages(ctx, execCtx.client, body)
	if err != nil {
		return nil, err
	}
	artifactIDs := make([]string, 0, len(images))
	for i, imageData := range images {
		artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "image", "image/png", nonEmptyString(node.Title, "Generated image"), imageData, "primary_output", "image", order+i, map[string]any{
			"type":       "image_generation",
			"model":      model,
			"templateId": execCtx.template.ID,
			"nodeId":     node.ID,
			"index":      i,
		})
		if err != nil {
			return nil, err
		}
		artifactIDs = append(artifactIDs, artifact.ID)
	}
	output := map[string]any{"artifactIds": artifactIDs, "model": model, "count": len(artifactIDs)}
	if len(images) > 0 {
		return applyProjectOutputMappings(execCtx, node, output, []executorGeneratedFile{{Name: "image", Data: images[0], MIME: "image/png"}})
	}
	return output, nil
}

func executeCondition(execCtx executorContext, node executorNode) map[string]any {
	context := executorConditionContext(execCtx)
	rules := executorConditionRules(node.Extra["conditions"])
	for _, rule := range rules {
		if conditionRuleMatches(context, rule) {
			decision := strings.TrimSpace(rule.Output)
			if decision == "" {
				decision = stringifyTemplateValue(rule.Value)
			}
			if decision == "" {
				decision = "matched"
			}
			return map[string]any{
				"decision": decision,
				"output":   decision,
				"matched":  true,
			}
		}
	}
	defaultDecision := firstNonEmptyString(stringFromMap(node.Extra, "defaultDecision"), stringFromMap(node.Extra, "defaultOutput"), "default")
	return map[string]any{
		"decision": defaultDecision,
		"output":   defaultDecision,
		"matched":  false,
	}
}

func executeProjectScript(ctx context.Context, execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	if execCtx.project == nil {
		return nil, NewError(ErrorWorkspaceInvalid, "script node requires run projectId", 2, map[string]string{"nodeId": node.ID})
	}
	if err := validateExecutorProjectAdapter(execCtx.project.Data.Adapter); err != nil {
		return nil, err
	}
	executorMode := stringFromMap(node.Extra, "executor")
	switch executorMode {
	case "", "local", "project", "filesystem":
	default:
		return nil, NewError(ErrorWorkspaceInvalid, "script executor mode is not supported by local executor", 2, map[string]string{"nodeId": node.ID, "executor": executorMode})
	}
	scriptPath := firstNonEmptyString(stringFromMap(node.Extra, "scriptPath"), stringFromMap(node.Extra, "path"), stringFromMap(node.Extra, "command"))
	if scriptPath == "" {
		return nil, NewError(ErrorWorkspaceInvalid, "scriptPath is required for script node", 2, map[string]string{"nodeId": node.ID})
	}
	resolved, err := resolveExecutorProjectPath(execCtx, ProjectPathExec, scriptPath)
	if err != nil {
		return nil, err
	}
	args := renderExecutorArgs(node, execCtx, stringSliceFromMap(node.Extra, "args"))
	root, err := executorProjectRoot(execCtx)
	if err != nil {
		return nil, err
	}
	command := osexec.CommandContext(ctx, resolved.Path, args...)
	command.Dir = root
	command.Env = executorProcessEnv(execCtx, node)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	redactedStdout := redactExecutorText(execCtx, stdout.String())
	redactedStderr := redactExecutorText(execCtx, stderr.String())
	if runErr != nil {
		return nil, NewError(ErrorWorkspaceInvalid, "project script failed", 2, map[string]any{
			"nodeId":   node.ID,
			"exitCode": executorExitCode(runErr),
			"stderr":   truncateExecutorText(redactedStderr),
		})
	}

	output := map[string]any{
		"stdout":       truncateExecutorText(redactedStdout),
		"stderr":       truncateExecutorText(redactedStderr),
		"exitCode":     0,
		"relativePath": resolved.RelativePath,
	}
	if parsed, ok := parseExecutorJSONObject(stdout.Bytes()); ok {
		output["json"] = parsed
		for key, value := range parsed {
			if _, exists := output[key]; !exists {
				output[key] = value
			}
		}
	}
	if executorArtifactWriteAllowed(execCtx) && len(stdout.Bytes()) > 0 {
		artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "text", "text/plain; charset=utf-8", nonEmptyString(node.Title, "Script output"), []byte(redactedStdout), "primary_output", "stdout", order, map[string]any{
			"type":       "script",
			"templateId": execCtx.template.ID,
			"nodeId":     node.ID,
		})
		if err != nil {
			return nil, err
		}
		output["artifactIds"] = []string{artifact.ID}
		output["artifactId"] = artifact.ID
	}
	return applyProjectOutputMappings(execCtx, node, output, []executorGeneratedFile{{Name: "stdout", Data: stdout.Bytes(), MIME: "text/plain; charset=utf-8"}})
}

func createExecutorArtifactForNode(execCtx executorContext, nodeID string, artifactType string, mimeType string, title string, data []byte, role string, slot string, order int, source map[string]any) (Envelope[ArtifactData], error) {
	if err := requireExecutorArtifactWrite(execCtx); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	return createExecutorArtifact(execCtx.workspace, execCtx.run.ID, nodeID, artifactType, mimeType, title, data, role, slot, order, source)
}

type executorGeneratedFile struct {
	Name string
	Data []byte
	MIME string
}

func applyProjectOutputMappings(execCtx executorContext, node executorNode, output map[string]any, files []executorGeneratedFile) (map[string]any, error) {
	mappings := executorProjectOutputMappings(node)
	if len(mappings) == 0 {
		return output, nil
	}
	if execCtx.project == nil {
		return nil, NewError(ErrorWorkspaceInvalid, "project output mapping requires run projectId", 2, map[string]string{"nodeId": node.ID})
	}
	written := make([]map[string]any, 0, len(mappings))
	for _, mapping := range mappings {
		file, ok := selectExecutorGeneratedFile(files, mapping.Kind)
		if !ok {
			return nil, NewError(ErrorWorkspaceInvalid, "project output mapping has no matching output", 2, map[string]string{"nodeId": node.ID, "kind": mapping.Kind})
		}
		rel, err := writeExecutorProjectFile(execCtx, mapping.Path, file.Data)
		if err != nil {
			return nil, err
		}
		written = append(written, map[string]any{
			"path":  rel,
			"kind":  firstNonEmptyString(mapping.Kind, file.Name),
			"bytes": len(file.Data),
		})
	}
	output["projectOutputs"] = written
	return output, nil
}

func executorProjectOutputMappings(node executorNode) []executorOutputMapping {
	mappings := make([]executorOutputMapping, 0, len(node.OutputMappings)+3)
	for _, mapping := range node.OutputMappings {
		pathValue := strings.TrimSpace(mapping.Path)
		if pathValue != "" {
			mappings = append(mappings, executorOutputMapping{Path: pathValue, Kind: strings.TrimSpace(mapping.Kind)})
		}
	}
	for _, key := range []string{"outputPath", "projectOutputPath"} {
		if pathValue := stringFromMap(node.Extra, key); pathValue != "" {
			mappings = append(mappings, executorOutputMapping{Path: pathValue, Kind: stringFromMap(node.Extra, "outputKind")})
		}
	}
	for _, item := range anySliceFromMap(node.Extra, "projectOutputs") {
		m, ok := asStringAnyMap(item)
		if !ok {
			continue
		}
		pathValue := strings.TrimSpace(fmt.Sprint(m["path"]))
		if pathValue == "" {
			continue
		}
		kind := strings.TrimSpace(fmt.Sprint(firstNonNil(m["kind"], m["from"])))
		mappings = append(mappings, executorOutputMapping{Path: pathValue, Kind: kind})
	}
	return mappings
}

func selectExecutorGeneratedFile(files []executorGeneratedFile, kind string) (executorGeneratedFile, bool) {
	kind = strings.TrimSpace(kind)
	if kind != "" {
		for _, file := range files {
			if file.Name == kind || strings.HasPrefix(file.MIME, kind+"/") || strings.Contains(file.MIME, kind) {
				return file, true
			}
		}
	}
	if len(files) == 0 {
		return executorGeneratedFile{}, false
	}
	return files[0], true
}

func writeExecutorProjectFile(execCtx executorContext, relPath string, data []byte) (string, error) {
	resolved, err := resolveExecutorProjectPath(execCtx, ProjectPathWrite, relPath)
	if err != nil {
		return "", err
	}
	if err := AtomicWriteFile(resolved.Path, data, 0o600); err != nil {
		return "", redactExecutorProjectError(*execCtx.project, err)
	}
	return resolved.RelativePath, nil
}

func resolveExecutorProjectPath(execCtx executorContext, operation ProjectPathOperation, relPath string) (ProjectPathResult, error) {
	if execCtx.project == nil {
		return ProjectPathResult{}, NewError(ErrorWorkspaceInvalid, "run projectId is required", 2, nil)
	}
	result, err := ResolveProjectPath(execCtx.workspace, *execCtx.project, ProjectPathRequest{Operation: operation, Path: relPath})
	if err != nil {
		return ProjectPathResult{}, redactExecutorProjectError(*execCtx.project, err)
	}
	return result, nil
}

func executorProjectRoot(execCtx executorContext) (string, error) {
	if execCtx.project == nil {
		return "", NewError(ErrorWorkspaceInvalid, "run projectId is required", 2, nil)
	}
	root, err := filepath.EvalSymlinks(execCtx.project.Data.RootPath)
	if err != nil {
		return "", redactExecutorProjectError(*execCtx.project, WrapError(ErrorWorkspaceInvalid, "resolve project root symlinks", 2, err))
	}
	return root, nil
}

func requireExecutorArtifactWrite(execCtx executorContext) error {
	if execCtx.project == nil || execCtx.project.Data.Capabilities.ArtifactWrite {
		return nil
	}
	return NewError(ErrorWorkspaceInvalid, "project capability artifact.write is disabled", 2, nil)
}

func executorArtifactWriteAllowed(execCtx executorContext) bool {
	return execCtx.project == nil || execCtx.project.Data.Capabilities.ArtifactWrite
}

func postLocalAIJSON(ctx context.Context, workspace Workspace, profileID string, channelID string, client *http.Client, localPath string, payload any) ([]byte, error) {
	channel, err := selectLocalAIChannel(workspace, profileID, channelID)
	if err != nil {
		return nil, err
	}
	secret, err := resolveAIProxySecret(channel.SecretRef)
	if err != nil {
		return nil, err
	}
	target, err := buildAIProxyTargetURL(channel.BaseURL, localPath, "")
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(ErrorInternal, "encode ai request", 5, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(ErrorInternal, "create ai request", 5, err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, WrapError(ErrorInternal, "call ai provider", 5, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, WrapError(ErrorInternal, "read ai provider response", 5, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, NewError(ErrorWorkspaceInvalid, "ai provider request failed", 2, map[string]any{"status": response.StatusCode, "message": localAIErrorMessage(responseBody)})
	}
	return responseBody, nil
}

func parseLocalAIText(body []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", WrapError(ErrorWorkspaceInvalid, "parse text_generation response", 2, err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", NewError(ErrorWorkspaceInvalid, parsed.Error.Message, 2, nil)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", NewError(ErrorWorkspaceInvalid, "text model returned empty content", 2, nil)
	}
	return parsed.Choices[0].Message.Content, nil
}

func parseLocalAIImages(ctx context.Context, client *http.Client, body []byte) ([][]byte, error) {
	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, WrapError(ErrorWorkspaceInvalid, "parse image_generation response", 2, err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return nil, NewError(ErrorWorkspaceInvalid, parsed.Error.Message, 2, nil)
	}
	images := [][]byte{}
	for _, item := range parsed.Data {
		if item.B64JSON != "" {
			data, err := base64.StdEncoding.DecodeString(item.B64JSON)
			if err != nil {
				return nil, WrapError(ErrorWorkspaceInvalid, "decode image b64_json", 2, err)
			}
			images = append(images, data)
			continue
		}
		if item.URL != "" {
			data, err := downloadExecutorImage(ctx, client, item.URL)
			if err != nil {
				return nil, err
			}
			images = append(images, data)
		}
	}
	if len(images) == 0 {
		return nil, NewError(ErrorWorkspaceInvalid, "image model returned no images", 2, nil)
	}
	return images, nil
}

func downloadExecutorImage(ctx context.Context, client *http.Client, imageURL string) ([]byte, error) {
	if strings.HasPrefix(imageURL, "data:") {
		parts := strings.SplitN(imageURL, ",", 2)
		if len(parts) != 2 {
			return nil, NewError(ErrorWorkspaceInvalid, "image data url is invalid", 2, nil)
		}
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, WrapError(ErrorWorkspaceInvalid, "decode image data url", 2, err)
		}
		return data, nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, WrapError(ErrorWorkspaceInvalid, "create image download request", 2, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, WrapError(ErrorInternal, "download image", 5, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, NewError(ErrorWorkspaceInvalid, "download image failed", 2, map[string]int{"status": response.StatusCode})
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, WrapError(ErrorInternal, "read image download", 5, err)
	}
	return data, nil
}

func createExecutorArtifact(workspace Workspace, runID string, nodeID string, artifactType string, mimeType string, title string, data []byte, role string, slot string, order int, source map[string]any) (Envelope[ArtifactData], error) {
	sum := sha256.Sum256(data)
	files := map[string]string{"original": path.Join("files", "original"+extensionForMIME(mimeType, artifactType))}
	artifactData := ArtifactData{
		Type:    artifactType,
		MIME:    mimeType,
		Title:   title,
		SHA256:  "sha256:" + hex.EncodeToString(sum[:]),
		Bytes:   int64(len(data)),
		Source:  source,
		Privacy: PrivacyPrivate,
		Files:   files,
		Metadata: map[string]any{
			"createdBy": executorID,
			"runId":     runID,
			"nodeId":    nodeID,
		},
	}
	if artifactType == "image" {
		if config, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			artifactData.Width = config.Width
			artifactData.Height = config.Height
		}
	}
	artifact, err := NewArtifact(workspace, artifactData)
	if err != nil {
		return Envelope[ArtifactData]{}, err
	}
	rel := artifact.Data.Files["original"]
	filePath := filepath.Join(ArtifactRepository(workspace).Dir(artifact.ID), filepath.FromSlash(rel))
	if err := AtomicWriteFile(filePath, data, 0o600); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	if err := WriteArtifact(workspace, artifact); err != nil {
		_ = os.RemoveAll(ArtifactRepository(workspace).Dir(artifact.ID))
		return Envelope[ArtifactData]{}, err
	}
	now := timeNowRFC3339()
	ref := Envelope[RunArtifactRefData]{
		SchemaVersion: SchemaVersion,
		Kind:          KindRunArtifactRef,
		ID:            artifact.ID,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Data: RunArtifactRefData{
			ArtifactID: artifact.ID,
			Role:       role,
			NodeID:     nodeID,
			Slot:       slot,
			Order:      order,
			Metadata: map[string]any{
				"createdBy": executorID,
			},
		},
	}
	if err := WriteRunArtifactRef(workspace, runID, ref); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	return artifact, nil
}

func updateExecutorRun(workspace Workspace, run Envelope[RunData], status string, output map[string]any, errorMessage string) (Envelope[RunData], error) {
	data := run.Data
	data.Status = status
	if output != nil {
		data.Output = output
	}
	if data.Metadata == nil {
		data.Metadata = map[string]any{}
	}
	data.Metadata["executor"] = executorID
	if errorMessage != "" {
		data.Metadata["error"] = errorMessage
	}
	next := nextEnvelopeRevision(run, data)
	if err := SaveRun(workspace, next, SaveRunOptions{}); err != nil {
		return Envelope[RunData]{}, err
	}
	return ReadRun(workspace, run.ID)
}

func writeExecutorNodeState(workspace Workspace, runID string, existing Envelope[RunNodeStateData], data RunNodeStateData) (Envelope[RunNodeStateData], error) {
	if strings.TrimSpace(data.NodeID) == "" {
		data.NodeID = existing.ID
	}
	var document Envelope[RunNodeStateData]
	if existing.ID != "" {
		document = nextEnvelopeRevision(existing, data)
	} else {
		created, err := NewRunNodeState(data.NodeID, data)
		if err != nil {
			return Envelope[RunNodeStateData]{}, err
		}
		document = created
	}
	if err := WriteRunNodeState(workspace, runID, document); err != nil {
		return Envelope[RunNodeStateData]{}, err
	}
	return readEnvelopeFile[RunNodeStateData](runNodeStatePath(workspace, runID, data.NodeID))
}

func executorNodeStateMap(workspace Workspace, runID string) (map[string]Envelope[RunNodeStateData], error) {
	states, err := ListRunNodeStates(workspace, runID)
	if err != nil {
		return nil, err
	}
	out := map[string]Envelope[RunNodeStateData]{}
	for _, state := range states {
		out[state.Data.NodeID] = state
	}
	return out, nil
}

func executorNodeOutputMap(states map[string]Envelope[RunNodeStateData]) map[string]map[string]any {
	out := map[string]map[string]any{}
	for nodeID, state := range states {
		if state.Data.Output != nil {
			out[nodeID] = state.Data.Output
		}
	}
	return out
}

func dependencyFailed(states map[string]Envelope[RunNodeStateData], edges []executorEdge, nodeID string) bool {
	for _, edge := range edges {
		if edge.To != nodeID {
			continue
		}
		if states[edge.From].Data.Status == RunStatusError {
			return true
		}
	}
	return false
}

func dependencyPending(states map[string]Envelope[RunNodeStateData], edges []executorEdge, nodeID string) bool {
	for _, edge := range edges {
		if edge.To != nodeID {
			continue
		}
		if states[edge.From].Data.Status != RunStatusSuccess {
			return true
		}
	}
	return false
}

func conditionRouteSkipped(states map[string]Envelope[RunNodeStateData], edges []executorEdge, nodeID string) (bool, string) {
	hasConditionalInput := false
	hasMatchedConditionalInput := false
	for _, edge := range edges {
		if edge.To != nodeID {
			continue
		}
		if !edgeHasRoutingCondition(edge) {
			continue
		}
		hasConditionalInput = true
		state := states[edge.From]
		if state.Data.Status != RunStatusSuccess {
			continue
		}
		if edgeRoutingConditionMatches(edge, state.Data.Output) {
			hasMatchedConditionalInput = true
		}
	}
	if hasConditionalInput && !hasMatchedConditionalInput {
		return true, "condition_not_matched"
	}
	return false, ""
}

func edgeHasRoutingCondition(edge executorEdge) bool {
	return strings.TrimSpace(edge.FromHandle) != "" || len(edge.Condition) > 0
}

func edgeRoutingConditionMatches(edge executorEdge, sourceOutput map[string]any) bool {
	if len(edge.Condition) > 0 {
		rule := executorConditionRule{
			Path:     firstNonEmptyString(stringFromAny(edge.Condition["path"]), stringFromAny(edge.Condition["jsonPath"]), "decision"),
			Operator: stringFromAny(edge.Condition["operator"]),
			Value:    edge.Condition["value"],
			Output:   strings.TrimSpace(edge.FromHandle),
		}
		if !conditionRuleMatches(sourceOutput, rule) {
			return false
		}
	}
	if strings.TrimSpace(edge.FromHandle) != "" {
		value := firstNonNil(lookupExecutorPath(sourceOutput, "decision"), lookupExecutorPath(sourceOutput, "output"))
		return executorValuesEqual(value, edge.FromHandle)
	}
	return true
}

func failNodeAndRun(workspace Workspace, run Envelope[RunData], node executorNode, cause error, existing Envelope[RunNodeStateData]) error {
	message := safeExecutorError(cause)
	data := RunNodeStateData{
		NodeID:     node.ID,
		Status:     RunStatusError,
		StartedAt:  existing.Data.StartedAt,
		FinishedAt: timeNowRFC3339(),
		Error:      message,
		Metadata:   executorNodeMetadata(node),
	}
	if strings.TrimSpace(data.StartedAt) == "" {
		data.StartedAt = data.FinishedAt
	}
	if _, err := writeExecutorNodeState(workspace, run.ID, existing, data); err != nil {
		return err
	}
	if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeFailed, "error", "Node failed", map[string]any{"nodeId": node.ID, "operation": node.Operation, "error": message}); err != nil {
		return err
	}
	return failRun(workspace, run, cause)
}

func failNodeAndRunResult(workspace Workspace, run Envelope[RunData], runResult ExecutorRunResult, node executorNode, cause error, existing Envelope[RunNodeStateData]) (ExecutorRunResult, error) {
	if err := failNodeAndRun(workspace, run, node, cause, existing); err != nil {
		return runResult, err
	}
	return executorFailedRunResult(workspace, run.ID, runResult, cause), nil
}

func failRun(workspace Workspace, run Envelope[RunData], cause error) error {
	message := safeExecutorError(cause)
	updated, err := updateExecutorRun(workspace, run, RunStatusError, map[string]any{"error": message}, message)
	if err != nil {
		return err
	}
	_, err = appendExecutorEvent(workspace, updated.ID, executorEventRunFailed, "error", "Run failed", map[string]any{"error": message})
	return err
}

func failRunResult(workspace Workspace, run Envelope[RunData], runResult ExecutorRunResult, cause error) (ExecutorRunResult, error) {
	if err := failRun(workspace, run, cause); err != nil {
		return runResult, err
	}
	return executorFailedRunResult(workspace, run.ID, runResult, cause), nil
}

func executorFailedRunResult(workspace Workspace, runID string, runResult ExecutorRunResult, cause error) ExecutorRunResult {
	runResult.Status = RunStatusError
	runResult.Error = safeExecutorError(cause)
	if refs, err := listRunArtifactRefs(workspace, runID); err == nil {
		runResult.ArtifactRefs = len(refs)
	}
	return runResult
}

func appendExecutorEvent(workspace Workspace, runID string, eventType string, level string, message string, data map[string]any) (RunEventEnvelope, error) {
	return AppendRunEvent(workspace, runID, RunEventInput{
		Type:    eventType,
		Level:   level,
		Actor:   RunEventActor{Type: "system", ID: executorID},
		Message: message,
		Data:    data,
	})
}

func executorRunInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	for _, key := range []string{"inputs", "items"} {
		if value, ok := input[key]; ok {
			if items, ok := value.([]any); ok && len(items) > 0 {
				if item, ok := asStringAnyMap(items[0]); ok {
					return item
				}
			}
		}
	}
	return input
}

func renderExecutorPrompt(prompt string, input map[string]any, nodeOutput map[string]map[string]any) string {
	return templateVarPattern.ReplaceAllStringFunc(prompt, func(match string) string {
		parts := templateVarPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		key := strings.TrimSpace(parts[1])
		switch {
		case strings.HasPrefix(key, "input."):
			return stringifyTemplateValue(lookupPath(input, strings.TrimPrefix(key, "input.")))
		case strings.HasPrefix(key, "node."):
			rest := strings.TrimPrefix(key, "node.")
			dot := strings.Index(rest, ".")
			if dot <= 0 {
				return ""
			}
			nodeID := rest[:dot]
			field := rest[dot+1:]
			return stringifyTemplateValue(lookupPath(nodeOutput[nodeID], field))
		case key == "productTitle":
			return stringifyTemplateValue(firstNonNil(input["productTitle"], input["title"], input["name"]))
		case key == "index":
			return "0"
		default:
			return ""
		}
	})
}

func lookupPath(value any, pathValue string) any {
	current := value
	for _, part := range strings.Split(pathValue, ".") {
		if part == "" {
			return nil
		}
		m, ok := asStringAnyMap(current)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

func asStringAnyMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func stringifyTemplateValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil && fmt.Sprint(value) != "" {
			return value
		}
	}
	return nil
}

func executorNodeMetadata(node executorNode) map[string]any {
	return map[string]any{
		"title":     node.Title,
		"type":      node.Type,
		"operation": node.Operation,
		"executor":  executorID,
	}
}

func stringFromMap(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(value))
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func stringSliceFromMap(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringFromAny(item))
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	default:
		return []string{fmt.Sprint(typed)}
	}
}

func anySliceFromMap(values map[string]any, key string) []any {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type executorConditionRule struct {
	Path     string
	Operator string
	Value    any
	Output   string
}

func executorConditionContext(execCtx executorContext) map[string]any {
	context := map[string]any{
		"input": execCtx.input,
		"node":  execCtx.nodeOut,
	}
	for key, value := range execCtx.input {
		if _, exists := context[key]; !exists {
			context[key] = value
		}
	}
	for nodeID, output := range execCtx.nodeOut {
		for key, value := range output {
			if _, exists := context[key]; !exists {
				context[key] = value
			}
		}
		if _, exists := context[nodeID]; !exists {
			context[nodeID] = output
		}
	}
	return context
}

func executorConditionRules(value any) []executorConditionRule {
	if value == nil {
		return nil
	}
	var items []any
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(typed), &items); err != nil {
			return nil
		}
	case []any:
		items = typed
	case []map[string]any:
		items = make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	default:
		return nil
	}
	rules := make([]executorConditionRule, 0, len(items))
	for _, item := range items {
		m, ok := asStringAnyMap(item)
		if !ok {
			continue
		}
		pathValue := firstNonEmptyString(stringFromAny(m["path"]), stringFromAny(m["jsonPath"]))
		if pathValue == "" {
			pathValue = "decision"
		}
		rules = append(rules, executorConditionRule{
			Path:     pathValue,
			Operator: stringFromAny(m["operator"]),
			Value:    m["value"],
			Output:   stringFromAny(m["output"]),
		})
	}
	return rules
}

func conditionRuleMatches(context any, rule executorConditionRule) bool {
	operator := strings.ToLower(firstNonEmptyString(rule.Operator, "eq"))
	value := lookupExecutorPath(context, firstNonEmptyString(rule.Path, "decision"))
	switch operator {
	case "exists":
		return value != nil && strings.TrimSpace(stringFromAny(value)) != ""
	case "truthy":
		return executorTruthy(value)
	case "neq", "ne", "not_eq":
		return !executorValuesEqual(value, rule.Value)
	case "contains":
		return strings.Contains(stringFromAny(value), stringFromAny(rule.Value))
	case "in":
		return executorValueIn(value, rule.Value)
	default:
		return executorValuesEqual(value, rule.Value)
	}
}

func lookupExecutorPath(value any, pathValue string) any {
	pathValue = normalizeExecutorJSONPath(pathValue)
	if pathValue == "" {
		return value
	}
	if resolved := lookupPath(value, pathValue); resolved != nil {
		return resolved
	}
	root, ok := asStringAnyMap(value)
	if !ok {
		return nil
	}
	if strings.HasPrefix(pathValue, "json.") {
		if resolved := lookupPath(root["json"], strings.TrimPrefix(pathValue, "json.")); resolved != nil {
			return resolved
		}
	}
	if strings.HasPrefix(pathValue, "input.") {
		if resolved := lookupPath(root["input"], strings.TrimPrefix(pathValue, "input.")); resolved != nil {
			return resolved
		}
	}
	if strings.HasPrefix(pathValue, "node.") {
		if resolved := lookupPath(root["node"], strings.TrimPrefix(pathValue, "node.")); resolved != nil {
			return resolved
		}
	}
	for _, candidate := range root {
		if resolved := lookupPath(candidate, pathValue); resolved != nil {
			return resolved
		}
	}
	return nil
}

func normalizeExecutorJSONPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "$")
	value = strings.TrimPrefix(value, ".")
	return strings.TrimSpace(value)
}

func executorTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		v := strings.TrimSpace(strings.ToLower(typed))
		return v != "" && v != "false" && v != "0" && v != "null"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return strings.TrimSpace(stringFromAny(typed)) != ""
	}
}

func executorValuesEqual(left any, right any) bool {
	leftString := strings.TrimSpace(stringFromAny(left))
	rightString := strings.TrimSpace(stringFromAny(right))
	if leftString == rightString {
		return true
	}
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func executorValueIn(value any, set any) bool {
	switch typed := set.(type) {
	case []any:
		for _, item := range typed {
			if executorValuesEqual(value, item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if executorValuesEqual(value, item) {
				return true
			}
		}
		return false
	default:
		parts := strings.Split(stringFromAny(set), ",")
		for _, part := range parts {
			if executorValuesEqual(value, strings.TrimSpace(part)) {
				return true
			}
		}
		return false
	}
}

func renderExecutorArgs(node executorNode, execCtx executorContext, args []string) []string {
	rendered := make([]string, 0, len(args))
	for _, arg := range args {
		rendered = append(rendered, renderExecutorPrompt(arg, execCtx.input, execCtx.nodeOut))
	}
	return rendered
}

func executorProcessEnv(execCtx executorContext, node executorNode) []string {
	env := []string{
		"OPSC_RUN_ID=" + execCtx.run.ID,
		"OPSC_NODE_ID=" + node.ID,
		"OPSC_WORKFLOW_TEMPLATE_ID=" + execCtx.template.ID,
	}
	if pathValue := os.Getenv("PATH"); pathValue != "" {
		env = append(env, "PATH="+pathValue)
	}
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	if execCtx.project != nil {
		env = append(env, "OPSC_PROJECT_ID="+execCtx.project.ID)
		env = append(env, "OPSC_PROJECT_ROOT="+execCtx.project.Data.RootPath)
	}
	return env
}

func executorExitCode(err error) int {
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func parseExecutorJSONObject(data []byte) (map[string]any, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil, false
	}
	return out, true
}

func truncateExecutorText(value string) string {
	const limit = 8192
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func redactExecutorError(execCtx executorContext, err error) string {
	return redactExecutorText(execCtx, safeExecutorError(err))
}

func redactExecutorText(execCtx executorContext, value string) string {
	type replacement struct {
		value string
		label string
	}
	replacements := []replacement{{value: execCtx.workspace.Root, label: "<workspace>"}}
	if execCtx.project != nil {
		replacements = append(replacements, replacement{value: execCtx.project.Data.RootPath, label: "<project-root>"})
		if resolved, err := filepath.EvalSymlinks(execCtx.project.Data.RootPath); err == nil {
			replacements = append(replacements, replacement{value: resolved, label: "<project-root>"})
		}
	}
	sort.SliceStable(replacements, func(i int, j int) bool { return len(replacements[i].value) > len(replacements[j].value) })
	out := value
	for _, item := range replacements {
		value := strings.TrimSpace(item.value)
		if value == "" {
			continue
		}
		out = strings.ReplaceAll(out, value, item.label)
	}
	return strings.TrimSpace(out)
}

func redactExecutorProjectError(project Envelope[ProjectData], err error) error {
	if err == nil {
		return nil
	}
	message := strings.ReplaceAll(err.Error(), strings.TrimSpace(project.Data.RootPath), "<project-root>")
	var workspaceErr *Error
	if asLocalWorkspaceError(err, &workspaceErr) {
		return NewError(workspaceErr.Code, message, workspaceErr.ExitCode, workspaceErr.Details)
	}
	return errors.New(message)
}

func localAIErrorMessage(body []byte) string {
	var parsed struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return parsed.Error.Message
		}
		if strings.TrimSpace(parsed.Message) != "" {
			return parsed.Message
		}
	}
	return ""
}

func extensionForMIME(mimeType string, artifactType string) string {
	base := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch base {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "text/plain":
		return ".txt"
	}
	extensions, _ := mime.ExtensionsByType(base)
	if len(extensions) > 0 {
		return extensions[0]
	}
	if artifactType == "text" {
		return ".txt"
	}
	if artifactType == "image" {
		return ".png"
	}
	return ".bin"
}

func executorNow(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return now().UTC().Format(time.RFC3339)
}

func safeExecutorError(err error) string {
	if err == nil {
		return "unknown executor error"
	}
	return strings.TrimSpace(err.Error())
}

func nonEmptyString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
