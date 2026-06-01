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
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	osexec "os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
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

	builtinPDDMockupBaseAssetID = "pdd-mockup-sku-artwork-base"
)

var templateVarPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

var executorImageQualityBase = map[string]int{
	"low":      1024,
	"medium":   2048,
	"high":     2880,
	"standard": 1024,
	"hd":       2048,
}

var executorImageQualityAliases = map[string]string{
	"1k": "low",
	"2k": "medium",
	"4k": "high",
}

var executorPixelSizePattern = regexp.MustCompile(`^\d+x\d+$`)

var executorAIImageRequestTimeout = 120 * time.Second
var executorSensitiveURLPattern = regexp.MustCompile(`https?://[^\s"']+`)

type ExecutorOptions struct {
	WorkspacePath    string
	RunID            string
	HybridSingleSync bool
	HTTPClient       *http.Client
	Now              func() time.Time
}

type ExecutorWatchOptions struct {
	ExecutorOptions
	PollInterval  time.Duration
	MaxIterations int
	OnResult      func(ExecutorResult) error
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
	X              float64                 `json:"x,omitempty"`
	Y              float64                 `json:"y,omitempty"`
	Width          float64                 `json:"width,omitempty"`
	Height         float64                 `json:"height,omitempty"`
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
	ID           string         `json:"id"`
	From         string         `json:"from"`
	To           string         `json:"to"`
	Source       string         `json:"source,omitempty"`
	Target       string         `json:"target,omitempty"`
	FromHandle   string         `json:"fromHandle,omitempty"`
	InputOrder   int            `json:"inputOrder,omitempty"`
	InputAlias   string         `json:"inputAlias,omitempty"`
	FileSelector string         `json:"fileSelector,omitempty"`
	Condition    map[string]any `json:"condition,omitempty"`
}

type executorContext struct {
	workspace Workspace
	run       Envelope[RunData]
	template  Envelope[TemplateData]
	project   *Envelope[ProjectData]
	edges     []executorEdge
	input     map[string]any
	nodeOut   map[string]map[string]any
	client    *http.Client
	now       func() time.Time
	product   *executorProductInput
}

type executorProductInput struct {
	Key           string
	Index         int
	SourceProduct string
	Input         map[string]any
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

func RunExecutorWatch(ctx context.Context, opts ExecutorWatchOptions) (ExecutorResult, error) {
	workspace, err := Open(opts.WorkspacePath)
	if err != nil {
		return ExecutorResult{}, err
	}
	stateDir, err := workspace.StateDir()
	if err != nil {
		return ExecutorResult{}, err
	}
	if err := ensurePrivateStateDir(stateDir); err != nil {
		return ExecutorResult{}, err
	}
	if err := clearStaleExecutorState(*workspace); err != nil {
		return ExecutorResult{}, err
	}
	lock, err := AcquireLock(workspace.LockPath(executorWatchLock))
	if err != nil {
		return ExecutorResult{}, err
	}
	defer lock.Release()
	defer cleanupExecutorRuntimeFiles(*workspace)

	interval := opts.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	startedAt := now().UTC().Format(time.RFC3339)
	aggregate := ExecutorResult{}
	for iteration := 0; ; iteration++ {
		if err := ctx.Err(); err != nil {
			return aggregate, nil
		}
		heartbeat := ExecutorRuntimeMetadata{
			SchemaVersion:      SchemaVersion,
			PID:                os.Getpid(),
			WorkspaceID:        workspace.Document.ID,
			Mode:               "watch",
			RunID:              strings.TrimSpace(opts.RunID),
			StartedAt:          startedAt,
			HeartbeatAt:        now().UTC().Format(time.RFC3339),
			PollIntervalMillis: int(interval / time.Millisecond),
			Iteration:          iteration + 1,
			Processed:          aggregate.Processed,
		}
		if err := writeExecutorRuntimeFiles(*workspace, heartbeat); err != nil {
			return aggregate, err
		}
		once := opts.ExecutorOptions
		once.HybridSingleSync = true
		result, err := RunExecutorOnce(ctx, once)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return aggregate, nil
			}
			return aggregate, err
		}
		if aggregate.WorkspaceID == "" {
			aggregate.WorkspaceID = result.WorkspaceID
		}
		aggregate.Processed += result.Processed
		aggregate.Runs = append(aggregate.Runs, result.Runs...)
		aggregate.Warnings = append(aggregate.Warnings, result.Warnings...)
		if len(result.Runs) > 0 {
			last := result.Runs[len(result.Runs)-1]
			heartbeat.Processed = aggregate.Processed
			heartbeat.LastRunID = last.RunID
			heartbeat.LastRunStatus = last.Status
			heartbeat.LastError = last.Error
			heartbeat.HeartbeatAt = now().UTC().Format(time.RFC3339)
			if err := writeExecutorRuntimeFiles(*workspace, heartbeat); err != nil {
				return aggregate, err
			}
		}
		if opts.OnResult != nil && (result.Processed > 0 || len(result.Warnings) > 0) {
			if err := opts.OnResult(result); err != nil {
				return aggregate, err
			}
		}
		if opts.MaxIterations > 0 && iteration+1 >= opts.MaxIterations {
			return aggregate, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return aggregate, nil
		case <-timer.C:
		}
	}
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
	if config, ok, err := hybridEcommerceConfigFromRunTemplate(run, template); err != nil {
		return failRunResult(workspace, run, runResult, err)
	} else if ok {
		return executeHybridEcommerceRun(ctx, workspace, run, template, opts, config)
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
	if _, ok := localEcommerceConfigFromTemplate(template); ok {
		products := executorRunProducts(run.Data.Input)
		if len(products) > 1 {
			return executeLocalMultiProductRun(ctx, workspace, run, template, nodes, edges, ordered, products, opts)
		}
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
		edges:     edges,
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

func executeLocalMultiProductRun(ctx context.Context, workspace Workspace, run Envelope[RunData], template Envelope[TemplateData], nodes []executorNode, edges []executorEdge, ordered []executorNode, products []executorProductInput, opts ExecutorOptions) (ExecutorRunResult, error) {
	runResult := ExecutorRunResult{RunID: run.ID, Status: run.Data.Status}
	if run.Data.Status == RunStatusPending {
		var err error
		if run, err = updateExecutorRun(workspace, run, RunStatusRunning, nil, ""); err != nil {
			return runResult, err
		}
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventClaimed, "info", "Local ecommerce executor claimed run", map[string]any{"templateId": run.Data.TemplateID, "productTotal": len(products)}); err != nil {
			return runResult, err
		}
	} else if _, err := appendExecutorEvent(workspace, run.ID, executorEventResumed, "info", "Local ecommerce executor resumed run", map[string]any{"productTotal": len(products)}); err != nil {
		return runResult, err
	}

	project, err := executorProject(workspace, run)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	if err := ensureAggregateProductNodeStates(workspace, run.ID, nodes, products); err != nil {
		return runResult, err
	}
	concurrency := executorProductConcurrency(run, template)
	productJobs := make(chan executorProductInput)
	productResults := make(chan ExecutorRunResult, len(products))
	var aggregateMu sync.Mutex
	workerCount := min(concurrency, len(products))
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			for product := range productJobs {
				productResult := executeLocalProduct(ctx, workspace, run, template, project, edges, ordered, products, product, opts, &aggregateMu)
				productResults <- productResult
			}
		}()
	}
	for _, product := range products {
		select {
		case <-ctx.Done():
			close(productJobs)
			return runResult, ctx.Err()
		case productJobs <- product:
		}
	}
	close(productJobs)

	failedProducts := []string{}
	for range products {
		productResult := <-productResults
		runResult.Executed += productResult.Executed
		runResult.Skipped += productResult.Skipped
		if productResult.Error != "" {
			failedProducts = append(failedProducts, productResult.RunID)
		}
	}
	if err := refreshAggregateProductNodeStates(workspace, run.ID, nodes, products); err != nil {
		return runResult, err
	}
	refs, err := listRunArtifactRefs(workspace, run.ID)
	if err != nil {
		return runResult, err
	}
	runResult.ArtifactRefs = len(refs)
	if len(failedProducts) > 0 {
		message := fmt.Sprintf("%d product(s) failed", len(failedProducts))
		output := map[string]any{"productTotal": len(products), "failedProducts": len(failedProducts)}
		run, err = updateExecutorRun(workspace, run, RunStatusError, output, message)
		if err != nil {
			return runResult, err
		}
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventRunFailed, "error", "Local ecommerce run failed", map[string]any{"error": message, "failedProducts": len(failedProducts)}); err != nil {
			return runResult, err
		}
		runResult.Status = RunStatusError
		runResult.Error = message
		return runResult, nil
	}
	output := map[string]any{"productTotal": len(products), "completedProducts": len(products)}
	run, err = updateExecutorRun(workspace, run, RunStatusSuccess, output, "")
	if err != nil {
		return runResult, err
	}
	if _, err := appendExecutorEvent(workspace, run.ID, executorEventRunCompleted, "info", "Local ecommerce run completed", output); err != nil {
		return runResult, err
	}
	runResult.Status = RunStatusSuccess
	return runResult, nil
}

func executeLocalProduct(ctx context.Context, workspace Workspace, run Envelope[RunData], template Envelope[TemplateData], project *Envelope[ProjectData], edges []executorEdge, ordered []executorNode, products []executorProductInput, product executorProductInput, opts ExecutorOptions, aggregateMu *sync.Mutex) ExecutorRunResult {
	result := ExecutorRunResult{RunID: product.Key, Status: RunStatusRunning}
	eventData := map[string]any{"productKey": product.Key, "sourceProduct": product.SourceProduct}
	if _, err := appendExecutorEvent(workspace, run.ID, "executor.product.started", "info", "Product execution started", eventData); err != nil {
		result.Status = RunStatusError
		result.Error = err.Error()
		return result
	}
	states, err := executorProductNodeStateMap(workspace, run.ID, product)
	if err != nil {
		result.Status = RunStatusError
		result.Error = err.Error()
		return result
	}
	execCtx := executorContext{
		workspace: workspace,
		run:       run,
		template:  template,
		project:   project,
		edges:     edges,
		input:     product.Input,
		nodeOut:   executorNodeOutputMap(states),
		client:    opts.HTTPClient,
		now:       opts.Now,
		product:   &product,
	}
	if execCtx.client == nil {
		execCtx.client = http.DefaultClient
	}
	if execCtx.now == nil {
		execCtx.now = time.Now
	}
	for _, node := range ordered {
		if err := ctx.Err(); err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
		stateID := productScopedNodeID(product.Key, node.ID)
		current := states[node.ID]
		switch current.Data.Status {
		case RunStatusSuccess:
			result.Skipped++
			continue
		case RunStatusError:
			result.Status = RunStatusError
			result.Error = nonEmptyString(current.Data.Error, "node already failed")
			return result
		}
		if dependencyFailed(states, edges, node.ID) {
			result.Status = RunStatusError
			result.Error = "upstream node failed"
			return result
		}
		if skipped, reason := conditionRouteSkipped(states, edges, node.ID); skipped {
			finishedAt := executorNow(execCtx.now)
			output := map[string]any{"skipped": true, "reason": reason, "productKey": product.Key}
			success, err := writeExecutorNodeState(workspace, run.ID, current, RunNodeStateData{
				NodeID:     stateID,
				Status:     RunStatusSuccess,
				StartedAt:  finishedAt,
				FinishedAt: finishedAt,
				Output:     output,
				Metadata:   productNodeMetadata(node, product),
			})
			if err != nil {
				result.Status = RunStatusError
				result.Error = err.Error()
				return result
			}
			states[node.ID] = success
			execCtx.nodeOut[node.ID] = output
			result.Skipped++
			if err := refreshAggregateNodeStateLocked(aggregateMu, workspace, run.ID, node, products); err != nil {
				result.Status = RunStatusError
				result.Error = err.Error()
				return result
			}
			if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeSkipped, "info", "Product node skipped", map[string]any{"productKey": product.Key, "nodeId": node.ID, "stateNodeId": stateID, "operation": node.Operation, "reason": reason}); err != nil {
				result.Status = RunStatusError
				result.Error = err.Error()
				return result
			}
			continue
		}
		if dependencyPending(states, edges, node.ID) {
			result.Status = RunStatusError
			result.Error = "upstream node is not ready"
			return result
		}
		startedAt := executorNow(execCtx.now)
		running, err := writeExecutorNodeState(workspace, run.ID, current, RunNodeStateData{
			NodeID:    stateID,
			Status:    RunStatusRunning,
			StartedAt: startedAt,
			Metadata:  productNodeMetadata(node, product),
		})
		if err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
		states[node.ID] = running
		if err := refreshAggregateNodeStateLocked(aggregateMu, workspace, run.ID, node, products); err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeStarted, "info", "Product node started", map[string]any{"productKey": product.Key, "nodeId": node.ID, "stateNodeId": stateID, "operation": node.Operation}); err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
		output, err := executeLocalNodeWithRetry(ctx, execCtx, node, product.Index*1000+result.Executed)
		if err != nil {
			message := redactExecutorError(execCtx, err)
			_, _ = writeExecutorNodeState(workspace, run.ID, running, RunNodeStateData{
				NodeID:     stateID,
				Status:     RunStatusError,
				StartedAt:  startedAt,
				FinishedAt: executorNow(execCtx.now),
				Error:      message,
				Metadata:   productNodeMetadata(node, product),
			})
			_ = refreshAggregateNodeStateLocked(aggregateMu, workspace, run.ID, node, products)
			_, _ = appendExecutorEvent(workspace, run.ID, executorEventNodeFailed, "error", "Product node failed", map[string]any{"productKey": product.Key, "nodeId": node.ID, "stateNodeId": stateID, "operation": node.Operation, "error": message})
			result.Status = RunStatusError
			result.Error = message
			_, _ = appendExecutorEvent(workspace, run.ID, "executor.product.failed", "error", "Product execution failed", map[string]any{"productKey": product.Key, "error": message})
			return result
		}
		if output == nil {
			output = map[string]any{}
		}
		output["productKey"] = product.Key
		finishedAt := executorNow(execCtx.now)
		success, err := writeExecutorNodeState(workspace, run.ID, running, RunNodeStateData{
			NodeID:     stateID,
			Status:     RunStatusSuccess,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Output:     output,
			Metadata:   productNodeMetadata(node, product),
		})
		if err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
		states[node.ID] = success
		execCtx.nodeOut[node.ID] = output
		result.Executed++
		if err := refreshAggregateNodeStateLocked(aggregateMu, workspace, run.ID, node, products); err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventNodeCompleted, "info", "Product node completed", map[string]any{"productKey": product.Key, "nodeId": node.ID, "stateNodeId": stateID, "operation": node.Operation}); err != nil {
			result.Status = RunStatusError
			result.Error = err.Error()
			return result
		}
	}
	if _, err := appendExecutorEvent(workspace, run.ID, "executor.product.completed", "info", "Product execution completed", eventData); err != nil {
		result.Status = RunStatusError
		result.Error = err.Error()
		return result
	}
	result.Status = RunStatusSuccess
	return result
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
	case "image_edit":
		return executeImageEdit(ctx, execCtx, node, order)
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
		if executorNonRetryableError(err) || !retry.Enabled || (retry.RetryCount > 0 && attempt >= retry.RetryCount) {
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

func executorNonRetryableError(err error) bool {
	var workspaceErr *Error
	if !asLocalWorkspaceError(err, &workspaceErr) {
		return false
	}
	if workspaceErr.Code == ErrorWorkspaceNotFound || workspaceErr.Code == ErrorInvalidArgument || workspaceErr.Code == ErrorAuthFailed {
		return true
	}
	if workspaceErr.Code != ErrorWorkspaceInvalid {
		return false
	}
	message := strings.ToLower(workspaceErr.Message)
	if strings.Contains(message, "ai provider request failed") {
		status := executorErrorStatus(workspaceErr.Details)
		if status >= 400 && status < 500 {
			return true
		}
	}
	for _, marker := range []string{
		"profile",
		"secretref",
		"secret ref",
		"channel",
		"project capability",
		"project root",
		"path escapes",
		"requires run projectid",
		"material library",
		"no matching material",
		"prompt is empty",
		"requires upstream",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func executorErrorStatus(details any) int {
	switch typed := details.(type) {
	case map[string]any:
		switch value := typed["status"].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		case json.Number:
			parsed, _ := value.Int64()
			return int(parsed)
		}
	case map[string]string:
		var parsed int
		_, _ = fmt.Sscan(typed["status"], &parsed)
		return parsed
	}
	return 0
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
		return executeAutoMaterialLookup(execCtx, node, order)
	}
	asset, err := ReadAsset(execCtx.workspace, assetID)
	if err != nil {
		if assetID == builtinPDDMockupBaseAssetID && stringFromMap(node.Extra, "fallback") == "builtin_pdd_mockup_base" {
			data, mimeType, err := builtinPDDMockupBaseImage()
			if err != nil {
				return nil, err
			}
			artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "image", mimeType, nonEmptyString(node.Title, "Mockup base"), data, "input", "material", order, map[string]any{
				"type":       "builtin_material",
				"assetId":    assetID,
				"templateId": execCtx.template.ID,
				"nodeId":     node.ID,
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"assetId": assetID, "artifactIds": []string{artifact.ID}, "artifactId": artifact.ID, "first_file": executorArtifactWorkspaceRef(artifact)}, nil
		}
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
	return map[string]any{"assetId": assetID, "artifactIds": []string{artifact.ID}, "artifactId": artifact.ID, "first_file": executorArtifactWorkspaceRef(artifact)}, nil
}

func executeAutoMaterialLookup(execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	root := executorLocalMaterialLibraryPath(execCtx, node)
	if root == "" {
		return nil, NewError(ErrorWorkspaceInvalid, "auto material_lookup requires a local material library", 2, map[string]string{"nodeId": node.ID})
	}
	match, err := findLocalMaterialReference(root, execCtx.input)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(match.Path)
	if err != nil {
		return nil, WrapError(ErrorInternal, "read matched material", 5, err)
	}
	mimeType := nonEmptyString(mime.TypeByExtension(filepath.Ext(match.Path)), "image/png")
	title := nonEmptyString(strings.TrimSpace(match.Work+" "+match.Character), nonEmptyString(node.Title, "Matched material"))
	artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "image", mimeType, title, data, "input", "material", order, map[string]any{
		"type":            "local_material_library",
		"library":         localEcommerceMaterialLibrary,
		"templateId":      execCtx.template.ID,
		"nodeId":          node.ID,
		"work":            match.Work,
		"character":       match.Character,
		"matchedBy":       match.MatchedBy,
		"sourceDirectory": filepath.Base(filepath.Dir(match.Path)),
	})
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"materialLibrary": localEcommerceMaterialLibrary,
		"work":            match.Work,
		"character":       match.Character,
		"matchedBy":       match.MatchedBy,
		"artifactIds":     []string{artifact.ID},
		"artifactId":      artifact.ID,
		"first_file":      executorArtifactWorkspaceRef(artifact),
	}
	if match.Presentation != "" {
		out["presentation"] = match.Presentation
	}
	return out, nil
}

type localMaterialMatch struct {
	Path         string
	Work         string
	Character    string
	Presentation string
	MatchedBy    string
	Score        int
}

func executorLocalMaterialLibraryPath(execCtx executorContext, node executorNode) string {
	if pathValue := stringFromMap(node.Extra, "materialLibraryPath"); pathValue != "" {
		return pathValue
	}
	if config, ok := localEcommerceConfigFromTemplate(execCtx.template); ok {
		if pathValue := strings.TrimSpace(config.MaterialLibraryPath); pathValue != "" {
			return pathValue
		}
	}
	return strings.TrimSpace(os.Getenv(localEcommerceMaterialLibraryEnv))
}

func findLocalMaterialReference(root string, input map[string]any) (localMaterialMatch, error) {
	root = strings.TrimSpace(root)
	if root == "" || !filepath.IsAbs(root) {
		return localMaterialMatch{}, NewError(ErrorWorkspaceInvalid, "material library path is not configured", 2, nil)
	}
	stat, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return localMaterialMatch{}, NewError(ErrorWorkspaceNotFound, "material library is not accessible", 2, nil)
		}
		return localMaterialMatch{}, WrapError(ErrorInternal, "open material library", 5, err)
	}
	if !stat.IsDir() {
		return localMaterialMatch{}, NewError(ErrorWorkspaceInvalid, "material library is not a directory", 2, nil)
	}
	best := localMaterialMatch{}
	err = filepath.WalkDir(root, func(pathValue string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasPrefix(name, "reference.") || !executorImageFileName(name) {
			return nil
		}
		candidate := localMaterialMatch{Path: pathValue}
		dir := filepath.Dir(pathValue)
		if meta, err := readLocalMaterialMetadata(filepath.Join(dir, "metadata.json")); err == nil {
			candidate.Work = stringFromMap(meta, "work")
			candidate.Character = stringFromMap(meta, "character")
			candidate.Presentation = stringFromMap(meta, "presentation")
			candidate.Score, candidate.MatchedBy = scoreLocalMaterialCandidate(input, candidate, meta)
		} else {
			rel, _ := filepath.Rel(root, dir)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) >= 1 {
				candidate.Work = parts[0]
			}
			if len(parts) >= 2 {
				candidate.Character = parts[1]
			}
			candidate.Score, candidate.MatchedBy = scoreLocalMaterialCandidate(input, candidate, nil)
		}
		if candidate.Score > best.Score || (candidate.Score == best.Score && best.Path == "") {
			best = candidate
		}
		return nil
	})
	if err != nil {
		return localMaterialMatch{}, WrapError(ErrorInternal, "scan material library", 5, err)
	}
	if best.Path == "" || best.Score <= 0 {
		return localMaterialMatch{}, NewError(ErrorWorkspaceNotFound, "no matching material found in local library", 2, nil)
	}
	return best, nil
}

func readLocalMaterialMetadata(pathValue string) (map[string]any, error) {
	data, err := os.ReadFile(pathValue)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func scoreLocalMaterialCandidate(input map[string]any, candidate localMaterialMatch, metadata map[string]any) (int, string) {
	needles := []struct {
		label string
		value string
		score int
	}{
		{"character", stringFromAny(firstNonNil(input["character"], input["animeCharacter"], input["ipCharacter"])), 60},
		{"theme", firstNonEmptyString(stringFromAny(firstNonNil(input["theme"], input["work"], input["animeIP"], input["sourceTitle"])), ""), 40},
		{"productTitle", stringFromAny(firstNonNil(input["productTitle"], input["title"], input["name"])), 20},
	}
	haystack := normalizedMaterialText(candidate.Work + " " + candidate.Character + " " + stringFromAny(metadata["aliases"]) + " " + stringFromAny(metadata["tags"]))
	score := 0
	matched := []string{}
	for _, needle := range needles {
		value := normalizedMaterialText(needle.value)
		if value == "" {
			continue
		}
		if strings.Contains(haystack, value) || strings.Contains(value, normalizedMaterialText(candidate.Character)) || strings.Contains(value, normalizedMaterialText(candidate.Work)) {
			score += needle.score
			matched = append(matched, needle.label)
		}
	}
	if score == 0 && candidate.Path != "" {
		score = 1
		matched = append(matched, "fallback")
	}
	return score, strings.Join(matched, ",")
}

func normalizedMaterialText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", "·", "", "　", "")
	return replacer.Replace(value)
}

func executorImageFileName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp"
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
	body, err := postLocalAIJSON(ctx, execCtx.workspace, execCtx.run.Data.ProfileID, executorAIChannelID(execCtx), execCtx.client, "/ai/v1/chat/completions", payload)
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
	quality := normalizeExecutorImageQuality(node.Quality)
	requestSize := resolveExecutorImageRequestSize(quality, node.Size)
	if requestSize != "" {
		payload["size"] = requestSize
	}
	if quality != "" {
		payload["quality"] = quality
	}
	if generationConfig := executorFlow2APIImageGenerationConfig(node, requestSize, quality); len(generationConfig) > 0 {
		payload["generation_config"] = generationConfig
	}
	body, err := postLocalAIJSON(ctx, execCtx.workspace, execCtx.run.Data.ProfileID, executorAIChannelID(execCtx), execCtx.client, "/ai/v1/images/generations", payload)
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

func executeImageEdit(ctx context.Context, execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	refs, err := executorImageInputRefs(execCtx, node)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, NewError(ErrorWorkspaceInvalid, "image_edit requires upstream image artifacts", 2, map[string]string{"nodeId": node.ID})
	}
	promptInput := cloneMap(execCtx.input)
	promptInput["uploaded_image_order"] = executorUploadedImageOrder(refs)
	promptInput["index1"] = 1
	promptInput["count"] = max(1, node.Count)
	promptInput["runId"] = execCtx.run.ID
	prompt := renderExecutorPrompt(node.Prompt, promptInput, execCtx.nodeOut)
	if strings.TrimSpace(prompt) == "" {
		return nil, NewError(ErrorWorkspaceInvalid, "image_edit prompt is empty", 2, map[string]string{"nodeId": node.ID})
	}
	model := nonEmptyString(node.Model, "gpt-image-2")
	fields := map[string]string{
		"model":           model,
		"prompt":          prompt,
		"n":               fmt.Sprint(max(1, node.Count)),
		"response_format": "b64_json",
	}
	quality := normalizeExecutorImageQuality(node.Quality)
	requestSize := resolveExecutorImageRequestSize(quality, node.Size)
	if requestSize != "" {
		fields["size"] = requestSize
	}
	if quality != "" {
		fields["quality"] = quality
	}
	if generationConfig := executorFlow2APIImageGenerationConfig(node, requestSize, quality); len(generationConfig) > 0 {
		body, _ := json.Marshal(generationConfig)
		fields["generation_config"] = string(body)
	}
	body, err := postLocalAIMultipart(ctx, execCtx.workspace, execCtx.run.Data.ProfileID, executorAIChannelID(execCtx), execCtx.client, "/ai/v1/images/edits", fields, refs)
	if err != nil {
		return nil, err
	}
	images, err := parseLocalAIImages(ctx, execCtx.client, body)
	if err != nil {
		return nil, err
	}
	inputIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		inputIDs = append(inputIDs, ref.ID)
	}
	artifactIDs := make([]string, 0, len(images))
	files := make([]executorGeneratedFile, 0, len(images))
	for i, imageData := range images {
		artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "image", "image/png", nonEmptyString(node.Title, "Edited image"), imageData, "primary_output", "image", order+i, map[string]any{
			"type":             "image_edit",
			"model":            model,
			"templateId":       execCtx.template.ID,
			"nodeId":           node.ID,
			"index":            i,
			"inputArtifactIds": inputIDs,
		})
		if err != nil {
			return nil, err
		}
		artifactIDs = append(artifactIDs, artifact.ID)
		files = append(files, executorGeneratedFile{Name: "image", Data: imageData, MIME: "image/png"})
	}
	output := map[string]any{"artifactIds": artifactIDs, "model": model, "count": len(artifactIDs), "inputArtifactIds": inputIDs}
	if len(artifactIDs) > 0 {
		output["artifactId"] = artifactIDs[0]
		output["first_file"] = "artifact:" + artifactIDs[0]
	}
	return applyProjectOutputMappings(execCtx, node, output, files)
}

func executorAIChannelID(execCtx executorContext) string {
	if execCtx.run.Data.Metadata != nil {
		for _, key := range []string{localEcommerceKey, hybridEcommerceKey} {
			values, ok := asMapStringAny(execCtx.run.Data.Metadata[key])
			if !ok {
				continue
			}
			if channelID := stringFromMap(values, "channelId"); channelID != "" {
				return channelID
			}
		}
	}
	if config, ok := localEcommerceConfigFromTemplate(execCtx.template); ok {
		return config.ChannelID
	}
	if config, ok, err := hybridEcommerceConfigFromTemplate(execCtx.template); err == nil && ok {
		return config.ChannelID
	}
	return ""
}

type executorImageInputRef struct {
	ID    string
	Title string
	MIME  string
	Data  []byte
	Role  string
}

func executorImageInputRefs(execCtx executorContext, node executorNode) ([]executorImageInputRef, error) {
	type source struct {
		edge executorEdge
		ids  []string
	}
	sources := []source{}
	for _, edge := range execCtx.edges {
		if edge.To != node.ID {
			continue
		}
		ids := executorArtifactIDsFromOutput(execCtx.nodeOut[edge.From])
		if len(ids) == 0 {
			continue
		}
		if strings.TrimSpace(edge.FileSelector) == "last" && len(ids) > 1 {
			ids = ids[len(ids)-1:]
		} else if strings.TrimSpace(edge.FileSelector) != "all" && len(ids) > 1 {
			ids = ids[:1]
		}
		sources = append(sources, source{edge: edge, ids: ids})
	}
	sort.SliceStable(sources, func(i int, j int) bool {
		if sources[i].edge.InputOrder != sources[j].edge.InputOrder {
			return sources[i].edge.InputOrder < sources[j].edge.InputOrder
		}
		return sources[i].edge.From < sources[j].edge.From
	})
	out := []executorImageInputRef{}
	for _, source := range sources {
		role := firstNonEmptyString(source.edge.InputAlias, source.edge.From)
		for _, artifactID := range source.ids {
			ref, err := readExecutorArtifactImage(execCtx, artifactID)
			if err != nil {
				return nil, err
			}
			ref.Role = role
			out = append(out, ref)
		}
	}
	return out, nil
}

func executorArtifactIDsFromOutput(output map[string]any) []string {
	ids := []string{}
	for _, value := range []any{output["artifactIds"], output["artifacts"]} {
		switch typed := value.(type) {
		case []string:
			for _, id := range typed {
				if strings.TrimSpace(id) != "" {
					ids = append(ids, strings.TrimSpace(id))
				}
			}
		case []any:
			for _, item := range typed {
				if id := strings.TrimSpace(stringFromAny(item)); id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	if id := strings.TrimSpace(stringFromAny(output["artifactId"])); id != "" {
		ids = append(ids, id)
	}
	return uniqueStrings(ids)
}

func readExecutorArtifactImage(execCtx executorContext, artifactID string) (executorImageInputRef, error) {
	artifact, err := ReadArtifact(execCtx.workspace, artifactID)
	if err != nil {
		return executorImageInputRef{}, err
	}
	if artifact.Data.Type != "image" && !strings.HasPrefix(artifact.Data.MIME, "image/") {
		return executorImageInputRef{}, NewError(ErrorWorkspaceInvalid, "image_edit input artifact must be an image", 2, map[string]string{"artifactId": artifactID})
	}
	rel := artifact.Data.Files["original"]
	if !isWorkspaceRelativeFile(rel) {
		return executorImageInputRef{}, NewError(ErrorWorkspaceInvalid, "artifact original file ref is invalid", 2, map[string]string{"artifactId": artifactID})
	}
	data, err := os.ReadFile(filepath.Join(ArtifactRepository(execCtx.workspace).Dir(artifact.ID), filepath.FromSlash(rel)))
	if err != nil {
		return executorImageInputRef{}, WrapError(ErrorInternal, "read image_edit input artifact", 5, err)
	}
	return executorImageInputRef{ID: artifact.ID, Title: artifact.Data.Title, MIME: nonEmptyString(artifact.Data.MIME, "image/png"), Data: data}, nil
}

func executorUploadedImageOrder(refs []executorImageInputRef) string {
	items := make([]string, 0, len(refs))
	for index, ref := range refs {
		items = append(items, fmt.Sprintf("%d:%s", index+1, firstNonEmptyString(ref.Role, ref.Title, ref.ID)))
	}
	return strings.Join(items, ", ")
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
	if action := localEcommerceScriptAction(execCtx, node); action != "" {
		return executeLocalEcommerceScriptAction(execCtx, node, action, order)
	}
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

func localEcommerceScriptAction(execCtx executorContext, node executorNode) string {
	action := stringFromMap(node.Extra, "localEcommerceAction")
	if action != "" {
		return action
	}
	if _, ok := localEcommerceConfigFromTemplate(execCtx.template); !ok {
		return ""
	}
	switch node.ID {
	case "package", "sync_local":
		return node.ID
	default:
		return ""
	}
}

func executeLocalEcommerceScriptAction(execCtx executorContext, node executorNode, action string, order int) (map[string]any, error) {
	if execCtx.project == nil {
		return nil, NewError(ErrorWorkspaceInvalid, "local ecommerce script action requires run projectId", 2, map[string]string{"nodeId": node.ID})
	}
	if err := requireExecutorArtifactWrite(execCtx); err != nil {
		return nil, err
	}
	switch action {
	case "package":
		return executeLocalEcommercePackage(execCtx, node, order)
	case "sync_local":
		return executeLocalEcommerceSyncLocal(execCtx, node, order)
	default:
		return nil, NewError(ErrorWorkspaceInvalid, "local ecommerce script action is not supported", 2, map[string]string{"nodeId": node.ID, "action": action})
	}
}

func executeLocalEcommercePackage(execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	if stringFromMap(node.Extra, "localEcommercePackageMode") == "source_variants" || len(anySliceFromMap(node.Extra, "sourceVariants")) > 0 {
		return executeLocalEcommerceSourceVariantsPackage(execCtx, node, order)
	}
	root := localEcommerceOutputRoot(execCtx, node)
	productTitle := safeProjectFileStem(firstNonEmptyString(stringFromAny(firstNonNil(execCtx.input["productTitle"], execCtx.input["title"], execCtx.input["name"])), "product"))
	base := path.Join(root, execCtx.run.ID, productTitle)
	sourceFiles := []struct {
		NodeID string
		Path   string
		Kind   string
	}{
		{NodeID: "source", Path: path.Join(base, "generated", "source.png"), Kind: "source"},
		{NodeID: "mockup", Path: path.Join(base, "待上架", productTitle, "规格图", "0001_sku_artwork.png"), Kind: "mockup"},
		{NodeID: "main", Path: path.Join(base, "待上架", productTitle, "主图", "1_主图.png"), Kind: "main"},
	}
	written := []map[string]any{}
	for _, item := range sourceFiles {
		artifactID := firstArtifactIDFromNode(execCtx.nodeOut[item.NodeID])
		if artifactID == "" {
			return nil, NewError(ErrorWorkspaceInvalid, "local ecommerce package missing upstream artifact", 2, map[string]string{"nodeId": node.ID, "sourceNodeId": item.NodeID})
		}
		file, err := readExecutorArtifactImage(execCtx, artifactID)
		if err != nil {
			return nil, err
		}
		rel, err := writeExecutorProjectFile(execCtx, item.Path, file.Data)
		if err != nil {
			return nil, err
		}
		written = append(written, map[string]any{"kind": item.Kind, "path": rel, "artifactId": artifactID, "bytes": len(file.Data)})
	}
	manifest := map[string]any{
		"runId":        execCtx.run.ID,
		"templateId":   execCtx.template.ID,
		"productTitle": firstNonEmptyString(stringFromAny(firstNonNil(execCtx.input["productTitle"], execCtx.input["title"], execCtx.input["name"])), "product"),
		"outputs":      written,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, WrapError(ErrorInternal, "encode local ecommerce package manifest", 5, err)
	}
	manifestRel, err := writeExecutorProjectFile(execCtx, path.Join(base, "package.json"), manifestData)
	if err != nil {
		return nil, err
	}
	artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "text", "application/json", nonEmptyString(node.Title, "Local ecommerce package"), manifestData, "primary_output", "manifest", order, map[string]any{
		"type":       "local_ecommerce_package",
		"templateId": execCtx.template.ID,
		"nodeId":     node.ID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":           localEcommerceBackend,
		"packageRoot":    base,
		"manifestPath":   manifestRel,
		"projectOutputs": written,
		"artifactIds":    []string{artifact.ID},
		"artifactId":     artifact.ID,
		"first_file":     executorArtifactWorkspaceRef(artifact),
	}, nil
}

type localEcommerceSourceVariantSpec struct {
	NodeID   string
	Label    string
	FileName string
}

func executeLocalEcommerceSourceVariantsPackage(execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	root := firstNonEmptyString(stringFromMap(node.Extra, "sourceVariantRoot"), localEcommerceOutputRoot(execCtx, node), "runs")
	packageRoot := path.Join(root, localEcommerceRunTimestampFolder(execCtx.run), "generated", localEcommerceThemeCharacterFolder(execCtx.input))
	variants := localEcommerceSourceVariantSpecs(node)
	if len(variants) == 0 {
		return nil, NewError(ErrorWorkspaceInvalid, "local ecommerce source variants package requires sourceVariants", 2, map[string]string{"nodeId": node.ID})
	}

	written := []map[string]any{}
	for index, variant := range variants {
		artifactID := firstArtifactIDFromNode(execCtx.nodeOut[variant.NodeID])
		if artifactID == "" {
			return nil, NewError(ErrorWorkspaceInvalid, "local ecommerce source variants package missing upstream artifact", 2, map[string]string{"nodeId": node.ID, "sourceNodeId": variant.NodeID})
		}
		file, err := readExecutorArtifactImage(execCtx, artifactID)
		if err != nil {
			return nil, err
		}
		fileName := safeProjectPathSegment(variant.FileName, "")
		if fileName == "" {
			fileName = fmt.Sprintf("%02d-%s.png", index+1, safeProjectPathSegment(variant.Label, "source"))
		}
		rel, err := writeExecutorProjectFile(execCtx, path.Join(packageRoot, fileName), file.Data)
		if err != nil {
			return nil, err
		}
		written = append(written, map[string]any{
			"kind":         "source_variant",
			"label":        variant.Label,
			"path":         rel,
			"artifactId":   artifactID,
			"bytes":        len(file.Data),
			"sourceNodeId": variant.NodeID,
		})
	}

	manifest := map[string]any{
		"runId":       execCtx.run.ID,
		"templateId":  execCtx.template.ID,
		"theme":       stringFromAny(firstNonNil(execCtx.input["theme"], execCtx.input["animeIP"], execCtx.input["work"])),
		"character":   stringFromAny(firstNonNil(execCtx.input["character"], execCtx.input["role"], execCtx.input["name"])),
		"packageRoot": packageRoot,
		"outputs":     written,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, WrapError(ErrorInternal, "encode local ecommerce source variants manifest", 5, err)
	}
	manifestRel, err := writeExecutorProjectFile(execCtx, path.Join(packageRoot, "package.json"), manifestData)
	if err != nil {
		return nil, err
	}
	artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "text", "application/json", nonEmptyString(node.Title, "Local ecommerce source variants package"), manifestData, "primary_output", "manifest", order, map[string]any{
		"type":       "local_ecommerce_source_variants_package",
		"templateId": execCtx.template.ID,
		"nodeId":     node.ID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":           "source_variants",
		"packageRoot":    packageRoot,
		"manifestPath":   manifestRel,
		"projectOutputs": written,
		"variantCount":   len(written),
		"artifactIds":    []string{artifact.ID},
		"artifactId":     artifact.ID,
		"first_file":     executorArtifactWorkspaceRef(artifact),
	}, nil
}

func localEcommerceSourceVariantSpecs(node executorNode) []localEcommerceSourceVariantSpec {
	items := anySliceFromMap(node.Extra, "sourceVariants")
	out := make([]localEcommerceSourceVariantSpec, 0, len(items))
	for _, item := range items {
		m, ok := asStringAnyMap(item)
		if !ok {
			continue
		}
		nodeID := firstNonEmptyString(stringFromMap(m, "nodeId"), stringFromMap(m, "id"))
		if nodeID == "" {
			continue
		}
		label := firstNonEmptyString(stringFromMap(m, "label"), nodeID)
		out = append(out, localEcommerceSourceVariantSpec{
			NodeID:   nodeID,
			Label:    label,
			FileName: stringFromMap(m, "fileName"),
		})
	}
	return out
}

func localEcommerceRunTimestampFolder(run Envelope[RunData]) string {
	for _, value := range []string{run.CreatedAt, run.UpdatedAt} {
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.Local().Format("20060102_150405")
		}
	}
	return safeProjectPathSegment(run.ID, "run")
}

func localEcommerceThemeCharacterFolder(input map[string]any) string {
	theme := stripOuterTitleQuotes(stringFromAny(firstNonNil(input["theme"], input["animeIP"], input["work"])))
	character := stringFromAny(firstNonNil(input["character"], input["role"], input["name"]))
	theme = safeProjectPathSegment(theme, "")
	character = safeProjectPathSegment(character, "")
	switch {
	case theme != "" && character != "":
		return theme + " - " + character
	case theme != "":
		return theme
	case character != "":
		return character
	default:
		return "product"
	}
}

func stripOuterTitleQuotes(value string) string {
	value = strings.TrimSpace(value)
	for {
		next := strings.TrimSpace(strings.Trim(value, "《》「」“”\"'`"))
		if next == value {
			return value
		}
		value = next
	}
}

func safeProjectPathSegment(value string, fallback string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == 0:
			return '_'
		case r < 32 || r == 127:
			return -1
		default:
			return r
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, " ._-")
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		return ""
	}
	if len([]rune(value)) > 80 {
		runes := []rune(value)
		value = string(runes[:80])
	}
	return value
}

func executeLocalEcommerceSyncLocal(execCtx executorContext, node executorNode, order int) (map[string]any, error) {
	root := localEcommerceOutputRoot(execCtx, node)
	productTitle := safeProjectFileStem(firstNonEmptyString(stringFromAny(firstNonNil(execCtx.input["productTitle"], execCtx.input["title"], execCtx.input["name"])), "product"))
	packageRoot := path.Join(root, execCtx.run.ID, productTitle)
	marker := map[string]any{
		"runId":       execCtx.run.ID,
		"templateId":  execCtx.template.ID,
		"packageRoot": packageRoot,
		"status":      "synced",
	}
	markerData, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return nil, WrapError(ErrorInternal, "encode local ecommerce sync marker", 5, err)
	}
	markerRel, err := writeExecutorProjectFile(execCtx, path.Join(packageRoot, "sync-local.json"), markerData)
	if err != nil {
		return nil, err
	}
	artifact, err := createExecutorArtifactForNode(execCtx, node.ID, "text", "application/json", nonEmptyString(node.Title, "Local ecommerce sync"), markerData, "primary_output", "sync", order, map[string]any{
		"type":       "local_ecommerce_sync",
		"templateId": execCtx.template.ID,
		"nodeId":     node.ID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":           localEcommerceBackend,
		"synced":         true,
		"packageRoot":    packageRoot,
		"markerPath":     markerRel,
		"artifactIds":    []string{artifact.ID},
		"artifactId":     artifact.ID,
		"first_file":     executorArtifactWorkspaceRef(artifact),
		"projectOutputs": []map[string]any{{"kind": "sync", "path": markerRel, "bytes": len(markerData)}},
	}, nil
}

func localEcommerceOutputRoot(execCtx executorContext, node executorNode) string {
	if root := stringFromMap(node.Extra, "outputRoot"); root != "" {
		return root
	}
	if config, ok := localEcommerceConfigFromTemplate(execCtx.template); ok {
		return firstNonEmptyString(config.ProjectOutputRoot, defaultEcommerceProjectOutRoot)
	}
	return defaultEcommerceProjectOutRoot
}

func createExecutorArtifactForNode(execCtx executorContext, nodeID string, artifactType string, mimeType string, title string, data []byte, role string, slot string, order int, source map[string]any) (Envelope[ArtifactData], error) {
	if err := requireExecutorArtifactWrite(execCtx); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	refNodeID := nodeID
	if execCtx.product != nil {
		if source == nil {
			source = map[string]any{}
		} else {
			source = cloneMap(source)
		}
		source["templateNodeId"] = nodeID
		source["productKey"] = execCtx.product.Key
		source["sourceProduct"] = execCtx.product.SourceProduct
		refNodeID = productScopedNodeID(execCtx.product.Key, nodeID)
	}
	return createExecutorArtifact(execCtx.workspace, execCtx.run.ID, refNodeID, artifactType, mimeType, title, data, role, slot, order, source)
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
	parent := path.Dir(filepath.ToSlash(strings.TrimSpace(relPath)))
	if parent != "." && parent != "" {
		if err := ensureExecutorProjectDir(execCtx, parent); err != nil {
			return "", err
		}
	}
	resolved, err := resolveExecutorProjectPath(execCtx, ProjectPathWrite, relPath)
	if err != nil {
		return "", err
	}
	if err := AtomicWriteFile(resolved.Path, data, 0o600); err != nil {
		return "", redactExecutorProjectError(*execCtx.project, err)
	}
	return resolved.RelativePath, nil
}

func ensureExecutorProjectDir(execCtx executorContext, relDir string) error {
	relDir = path.Clean(filepath.ToSlash(strings.TrimSpace(relDir)))
	if relDir == "." || relDir == "" {
		return nil
	}
	parent := path.Dir(relDir)
	if parent != "." && parent != relDir {
		if err := ensureExecutorProjectDir(execCtx, parent); err != nil {
			return err
		}
	}
	resolved, err := resolveExecutorProjectPath(execCtx, ProjectPathWrite, relDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(resolved.Path, 0o700); err != nil {
		return redactExecutorProjectError(*execCtx.project, err)
	}
	return nil
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

func postLocalAIMultipart(ctx context.Context, workspace Workspace, profileID string, channelID string, client *http.Client, localPath string, fields map[string]string, files []executorImageInputRef) ([]byte, error) {
	channel, err := selectLocalAIChannel(workspace, profileID, channelID)
	if err != nil {
		return nil, err
	}
	secret, err := resolveAIProxySecret(channel.SecretRef)
	if err != nil {
		return nil, err
	}
	if localAIFlow2APIChannel(channel) && strings.HasSuffix(localPath, "/images/edits") {
		refs := make([]service.ModelReference, 0, len(files))
		for index, file := range files {
			refs = append(refs, service.ModelReference{
				Name:     fmt.Sprintf("input_%d%s", index+1, extensionForMIME(file.MIME, "image")),
				MimeType: file.MIME,
				Data:     append([]byte{}, file.Data...),
			})
		}
		count := positiveInt(fields["n"])
		if count <= 0 {
			count = 1
		}
		images, err := service.Flow2APIImageEditWithOptions(localAIModelChannel(channel, secret), fields["model"], fields["prompt"], refs, count, localAIFlow2APIImageOptions(fields))
		if err != nil {
			return nil, NewError(ErrorWorkspaceInvalid, "ai provider request failed", 2, map[string]any{"message": localAIProviderErrorMessage(err)})
		}
		return localAIImagesResponse(images), nil
	}
	fields = normalizeOpenAICompatibleImageFields(fields)
	target, err := buildAIProxyTargetURL(channel.BaseURL, localPath, "")
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, WrapError(ErrorInternal, "encode ai multipart field", 5, err)
		}
	}
	for index, file := range files {
		part, err := writer.CreateFormFile("image", fmt.Sprintf("input_%d%s", index+1, extensionForMIME(file.MIME, "image")))
		if err != nil {
			return nil, WrapError(ErrorInternal, "encode ai multipart file", 5, err)
		}
		if _, err := part.Write(file.Data); err != nil {
			return nil, WrapError(ErrorInternal, "write ai multipart file", 5, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, WrapError(ErrorInternal, "close ai multipart request", 5, err)
	}
	requestCtx, cancel := executorAIRequestContext(ctx, localPath)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target.String(), &body)
	if err != nil {
		return nil, WrapError(ErrorInternal, "create ai request", 5, err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Content-Type", writer.FormDataContentType())
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

func postLocalAIJSON(ctx context.Context, workspace Workspace, profileID string, channelID string, client *http.Client, localPath string, payload any) ([]byte, error) {
	channel, err := selectLocalAIChannel(workspace, profileID, channelID)
	if err != nil {
		return nil, err
	}
	secret, err := resolveAIProxySecret(channel.SecretRef)
	if err != nil {
		return nil, err
	}
	if localAIFlow2APIChannel(channel) && strings.HasSuffix(localPath, "/images/generations") {
		var request struct {
			Model            string         `json:"model"`
			Prompt           string         `json:"prompt"`
			N                int            `json:"n"`
			Quality          string         `json:"quality"`
			Size             string         `json:"size"`
			GenerationConfig map[string]any `json:"generation_config"`
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, WrapError(ErrorInternal, "encode ai request", 5, err)
		}
		if err := json.Unmarshal(body, &request); err != nil {
			return nil, WrapError(ErrorWorkspaceInvalid, "parse ai image request", 2, err)
		}
		if request.N <= 0 {
			request.N = 1
		}
		images, err := service.Flow2APIImageGenerationWithOptions(localAIModelChannel(channel, secret), request.Model, request.Prompt, request.N, service.Flow2APIImageOptions{
			Quality:          request.Quality,
			Size:             request.Size,
			GenerationConfig: request.GenerationConfig,
		})
		if err != nil {
			return nil, NewError(ErrorWorkspaceInvalid, "ai provider request failed", 2, map[string]any{"message": localAIProviderErrorMessage(err)})
		}
		return localAIImagesResponse(images), nil
	}
	target, err := buildAIProxyTargetURL(channel.BaseURL, localPath, "")
	if err != nil {
		return nil, err
	}
	if localAIImageEndpoint(localPath) {
		payload = normalizeOpenAICompatibleImagePayload(payload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(ErrorInternal, "encode ai request", 5, err)
	}
	requestCtx, cancel := executorAIRequestContext(ctx, localPath)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target.String(), bytes.NewReader(body))
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

func localAIFlow2APIChannel(channel ProfileChannel) bool {
	protocol := strings.ToLower(strings.TrimSpace(channel.Protocol))
	if protocol == "flow2api" {
		return true
	}
	return strings.Contains(strings.ToLower(channel.ID), "flow2api") || strings.Contains(strings.ToLower(channel.Name), "flow2api")
}

func localAIModelChannel(channel ProfileChannel, secret string) model.ModelChannel {
	protocol := strings.TrimSpace(channel.Protocol)
	if localAIFlow2APIChannel(channel) {
		protocol = "flow2api"
	}
	return model.ModelChannel{
		Name:     channel.Name,
		BaseURL:  channel.BaseURL,
		APIKey:   secret,
		Protocol: protocol,
		Models:   append([]string{}, channel.Models...),
		Weight:   max(channel.Weight, 1),
	}
}

func localAIFlow2APIImageOptions(fields map[string]string) service.Flow2APIImageOptions {
	options := service.Flow2APIImageOptions{
		Quality: strings.TrimSpace(fields["quality"]),
		Size:    strings.TrimSpace(fields["size"]),
	}
	if raw := strings.TrimSpace(fields["generation_config"]); raw != "" {
		parsed := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			options.GenerationConfig = parsed
		}
	}
	return options
}

func executorAIRequestContext(ctx context.Context, localPath string) (context.Context, context.CancelFunc) {
	if localAIImageEndpoint(localPath) && executorAIImageRequestTimeout > 0 {
		return context.WithTimeout(ctx, executorAIImageRequestTimeout)
	}
	return ctx, func() {}
}

func localAIImageEndpoint(localPath string) bool {
	return strings.Contains(localPath, "/images/")
}

func normalizeOpenAICompatibleImageFields(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		out[key] = value
	}
	if size := strings.TrimSpace(out["size"]); size != "" {
		out["size"] = normalizeOpenAICompatibleImageSize(size)
	}
	delete(out, "generation_config")
	return out
}

func normalizeOpenAICompatibleImagePayload(payload any) any {
	values, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	out := cloneMap(values)
	if size := strings.TrimSpace(stringFromAny(out["size"])); size != "" {
		out["size"] = normalizeOpenAICompatibleImageSize(size)
	}
	delete(out, "generation_config")
	return out
}

func normalizeOpenAICompatibleImageSize(size string) string {
	value := strings.TrimSpace(size)
	switch strings.ToLower(value) {
	case "", "auto":
		return value
	case "square":
		return "1024x1024"
	case "portrait":
		return "1024x1536"
	case "landscape":
		return "1536x1024"
	}
	switch value {
	case "1024x1024", "1024x1536", "1536x1024":
		return value
	}
	if width, height, ok := parseExecutorImageSize(value); ok {
		return nearestOpenAICompatibleImageSize(width, height)
	}
	if width, height, ok := parseExecutorImageRatio(value); ok {
		return nearestOpenAICompatibleImageSize(width, height)
	}
	return value
}

func parseExecutorImageSize(value string) (float64, float64, bool) {
	if !executorPixelSizePattern.MatchString(value) {
		return 0, 0, false
	}
	parts := strings.Split(value, "x")
	width, errW := strconv.ParseFloat(parts[0], 64)
	height, errH := strconv.ParseFloat(parts[1], 64)
	return width, height, errW == nil && errH == nil && width > 0 && height > 0
}

func parseExecutorImageRatio(value string) (float64, float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errW := strconv.ParseFloat(parts[0], 64)
	height, errH := strconv.ParseFloat(parts[1], 64)
	return width, height, errW == nil && errH == nil && width > 0 && height > 0
}

func nearestOpenAICompatibleImageSize(width float64, height float64) string {
	ratio := width / height
	switch {
	case ratio > 1.15:
		return "1536x1024"
	case ratio < 0.87:
		return "1024x1536"
	default:
		return "1024x1024"
	}
}

func normalizeExecutorImageQuality(quality string) string {
	value := strings.ToLower(strings.TrimSpace(quality))
	if alias := executorImageQualityAliases[value]; alias != "" {
		value = alias
	}
	if _, ok := executorImageQualityBase[value]; ok {
		return value
	}
	return ""
}

func resolveExecutorImageRequestSize(quality string, size string) string {
	value := strings.TrimSpace(size)
	if value == "" || value == "auto" {
		return ""
	}
	if executorPixelSizePattern.MatchString(value) {
		return value
	}
	if quality != "" {
		if resolved := resolveExecutorImageRatioSize(quality, value); resolved != "" {
			return resolved
		}
	}
	return value
}

func resolveExecutorImageRatioSize(quality string, ratio string) string {
	basePixels, ok := executorImageQualityBase[quality]
	if !ok || ratio == "" || ratio == "auto" {
		return ""
	}
	parts := strings.Split(ratio, ":")
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.ParseFloat(parts[0], 64)
	h, errH := strconv.ParseFloat(parts[1], 64)
	if errW != nil || errH != nil || w == 0 || h == 0 {
		return ""
	}
	targetPixels := float64(basePixels * basePixels)
	isLandscape := w >= h
	longRatio := w / h
	if !isLandscape {
		longRatio = h / w
	}
	longSide := math.Floor(math.Sqrt(targetPixels*longRatio)/16) * 16
	shortSide := math.Round((longSide/longRatio)/16) * 16
	width := int(shortSide)
	height := int(longSide)
	if isLandscape {
		width = int(longSide)
		height = int(shortSide)
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func executorFlow2APIImageGenerationConfig(node executorNode, requestSize string, quality string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(requestSize) != "" {
		result["size"] = requestSize
	}
	if strings.TrimSpace(quality) != "" {
		result["quality"] = quality
	}
	if strings.TrimSpace(node.Size) != "" {
		result["aspectRatio"] = node.Size
	}
	if strings.TrimSpace(node.Quality) != "" {
		result["qualityLabel"] = node.Quality
	}
	for _, key := range []string{"aspectRatio", "outputFormat", "background", "style", "seed"} {
		if value := stringFromMap(node.Extra, key); value != "" {
			result[key] = value
		}
	}
	return result
}

func localAIImagesResponse(images [][]byte) []byte {
	response := struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}{Data: make([]struct {
		B64JSON string `json:"b64_json"`
	}, 0, len(images))}
	for _, image := range images {
		response.Data = append(response.Data, struct {
			B64JSON string `json:"b64_json"`
		}{B64JSON: base64.StdEncoding.EncodeToString(image)})
	}
	body, _ := json.Marshal(response)
	return body
}

func localAIProviderErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return ""
	}
	message = strings.ReplaceAll(message, "\n", " ")
	message = regexp.MustCompile(`Bearer\s+[A-Za-z0-9._-]+`).ReplaceAllString(message, "Bearer <redacted>")
	message = regexp.MustCompile(`https?://[^\s"']+`).ReplaceAllString(message, "<url>")
	if len(message) > 500 {
		message = message[:500]
	}
	return message
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
	refMetadata := map[string]any{"createdBy": executorID}
	if source != nil {
		for _, key := range []string{"productKey", "sourceProduct", "templateNodeId"} {
			if value, ok := source[key]; ok && stringFromAny(value) != "" {
				refMetadata[key] = value
			}
		}
	}
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
			Metadata:   refMetadata,
		},
	}
	if err := WriteRunArtifactRef(workspace, runID, ref); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	return artifact, nil
}

func executorArtifactWorkspaceRef(artifact Envelope[ArtifactData]) string {
	if artifact.ID == "" {
		return ""
	}
	return "artifact:" + artifact.ID
}

func firstArtifactIDFromNode(output map[string]any) string {
	ids := executorArtifactIDsFromOutput(output)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func safeProjectFileStem(value string) string {
	value = strings.TrimSpace(value)
	value = regexp.MustCompile(`[^\p{L}\p{N}._-]+`).ReplaceAllString(value, "_")
	value = strings.Trim(value, "._-")
	if value == "" {
		return "item"
	}
	if len([]rune(value)) > 80 {
		runes := []rune(value)
		value = string(runes[:80])
	}
	return value
}

func builtinPDDMockupBaseImage() ([]byte, string, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 246, G: 246, B: 243, A: 255}}, image.Point{}, draw.Src)
	pillow := color.RGBA{R: 252, G: 252, B: 250, A: 255}
	shadow := color.RGBA{R: 224, G: 224, B: 220, A: 255}
	for _, rect := range []image.Rectangle{image.Rect(150, 100, 470, 930), image.Rect(555, 100, 875, 930)} {
		draw.Draw(img, rect.Add(image.Pt(10, 10)), &image.Uniform{C: shadow}, image.Point{}, draw.Src)
		draw.Draw(img, rect, &image.Uniform{C: pillow}, image.Point{}, draw.Src)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", WrapError(ErrorInternal, "encode builtin mockup base", 5, err)
	}
	return buf.Bytes(), "image/png", nil
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
	message := redactExecutorText(executorContext{workspace: workspace}, safeExecutorError(cause))
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
	message := redactExecutorText(executorContext{workspace: workspace}, safeExecutorError(cause))
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
	runResult.Error = redactExecutorText(executorContext{workspace: workspace}, safeExecutorError(cause))
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

func ensureAggregateProductNodeStates(workspace Workspace, runID string, nodes []executorNode, products []executorProductInput) error {
	for _, node := range nodes {
		if err := refreshAggregateProductNodeState(workspace, runID, node, products); err != nil {
			return err
		}
	}
	return nil
}

func refreshAggregateProductNodeStates(workspace Workspace, runID string, nodes []executorNode, products []executorProductInput) error {
	for _, node := range nodes {
		if err := refreshAggregateProductNodeState(workspace, runID, node, products); err != nil {
			return err
		}
	}
	return nil
}

func refreshAggregateNodeStateLocked(mu *sync.Mutex, workspace Workspace, runID string, node executorNode, products []executorProductInput) error {
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	return refreshAggregateProductNodeState(workspace, runID, node, products)
}

func refreshAggregateProductNodeState(workspace Workspace, runID string, node executorNode, products []executorProductInput) error {
	states, err := executorNodeStateMap(workspace, runID)
	if err != nil {
		return err
	}
	total := len(products)
	success := 0
	failed := 0
	running := 0
	skipped := 0
	recentError := ""
	startedAt := ""
	finishedAt := ""
	for _, product := range products {
		state := states[productScopedNodeID(product.Key, node.ID)]
		switch state.Data.Status {
		case RunStatusSuccess:
			success++
			if state.Data.Output != nil && state.Data.Output["skipped"] == true {
				skipped++
			}
		case RunStatusError:
			failed++
			if recentError == "" {
				recentError = state.Data.Error
			}
		case RunStatusRunning:
			running++
		}
		if startedAt == "" || (state.Data.StartedAt != "" && state.Data.StartedAt < startedAt) {
			startedAt = state.Data.StartedAt
		}
		if state.Data.FinishedAt != "" && state.Data.FinishedAt > finishedAt {
			finishedAt = state.Data.FinishedAt
		}
	}
	status := RunStatusPending
	switch {
	case failed > 0:
		status = RunStatusError
	case running > 0:
		status = RunStatusRunning
	case total > 0 && success == total:
		status = RunStatusSuccess
	}
	output := map[string]any{
		"total":   total,
		"success": success,
		"failed":  failed,
		"running": running,
		"pending": max(0, total-success-failed-running),
		"skipped": skipped,
	}
	_, err = writeExecutorNodeState(workspace, runID, states[node.ID], RunNodeStateData{
		NodeID:     node.ID,
		Status:     status,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Error:      recentError,
		Output:     output,
		Metadata:   aggregateProductNodeMetadata(node, total),
	})
	return err
}

func executorProductNodeStateMap(workspace Workspace, runID string, product executorProductInput) (map[string]Envelope[RunNodeStateData], error) {
	all, err := executorNodeStateMap(workspace, runID)
	if err != nil {
		return nil, err
	}
	out := map[string]Envelope[RunNodeStateData]{}
	prefix := productScopedNodePrefix(product.Key)
	for nodeID, state := range all {
		if strings.HasPrefix(nodeID, prefix) {
			out[strings.TrimPrefix(nodeID, prefix)] = state
		}
	}
	return out, nil
}

func productScopedNodePrefix(productKey string) string {
	return "product/" + productKey + "/"
}

func productScopedNodeID(productKey string, nodeID string) string {
	return productScopedNodePrefix(productKey) + nodeID
}

func productNodeMetadata(node executorNode, product executorProductInput) map[string]any {
	metadata := executorNodeMetadata(node)
	metadata["templateNodeId"] = node.ID
	metadata["productKey"] = product.Key
	metadata["productIndex"] = product.Index
	if product.SourceProduct != "" {
		metadata["sourceProduct"] = product.SourceProduct
	}
	return metadata
}

func aggregateProductNodeMetadata(node executorNode, productTotal int) map[string]any {
	metadata := executorNodeMetadata(node)
	metadata["aggregate"] = true
	metadata["productTotal"] = productTotal
	return metadata
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

func executorRunProducts(input map[string]any) []executorProductInput {
	if len(input) == 0 {
		return []executorProductInput{executorProductFromInput(0, map[string]any{})}
	}
	items := anySliceFromMap(input, "inputs")
	if len(items) == 0 {
		items = anySliceFromMap(input, "items")
	}
	if len(items) == 0 {
		return []executorProductInput{executorProductFromInput(0, input)}
	}
	products := make([]executorProductInput, 0, len(items))
	seen := map[string]int{}
	for index, item := range items {
		values, ok := asStringAnyMap(item)
		if !ok {
			values = map[string]any{"value": item}
		}
		product := executorProductFromInput(index, values)
		if count := seen[product.Key]; count > 0 {
			product.Key = fmt.Sprintf("%s_%02d", product.Key, count+1)
			product.Input["productKey"] = product.Key
		}
		seen[product.Key]++
		products = append(products, product)
	}
	return products
}

func executorProductFromInput(index int, input map[string]any) executorProductInput {
	values := cloneMap(input)
	if values == nil {
		values = map[string]any{}
	}
	sourceProduct := firstNonEmptyString(
		stringFromAny(firstNonNil(values["sourceProduct"], values["productTitle"], values["title"], values["name"], values["product"])),
		fmt.Sprintf("product-%03d", index+1),
	)
	key := firstNonEmptyString(
		stringFromAny(firstNonNil(values["productKey"], values["key"], values["id"])),
		fmt.Sprintf("product_%03d", index+1),
	)
	key = safeProductKey(key)
	values["productKey"] = key
	values["productIndex"] = index
	values["sourceProduct"] = sourceProduct
	if _, ok := values["productTitle"]; !ok {
		values["productTitle"] = sourceProduct
	}
	return executorProductInput{
		Key:           key,
		Index:         index,
		SourceProduct: sourceProduct,
		Input:         values,
	}
}

func safeProductKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "product"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	out := strings.Trim(builder.String(), "_-")
	if out == "" {
		return "product"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func executorProductConcurrency(run Envelope[RunData], template Envelope[TemplateData]) int {
	for _, value := range []any{run.Data.Input["productConcurrency"], template.Data.Settings["productConcurrency"]} {
		if parsed := positiveInt(value); parsed > 0 {
			return min(parsed, 8)
		}
	}
	return 1
}

func positiveInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
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
		case key == "index1":
			return stringifyTemplateValue(firstNonNil(input["index1"], 1))
		case key == "count":
			return stringifyTemplateValue(firstNonNil(input["count"], 1))
		case key == "uploaded_image_order":
			return stringifyTemplateValue(input["uploaded_image_order"])
		default:
			return stringifyTemplateValue(lookupPath(input, key))
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
	out = executorSensitiveURLPattern.ReplaceAllString(out, "<url>")
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
