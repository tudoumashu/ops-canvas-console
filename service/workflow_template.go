package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

const (
	customWorkflowTypePDD       = "pdd"
	customRunLegacyDirName      = "custom_workflow"
	customRunDataRelDir         = "logs/custom_workflow"
	customWorkflowRunLogRelPath = "logs/custom_workflow.log"
)

const workflowImageRequestMaxCount = 15

var workflowImageQualityBase = map[string]int{
	"low":      1024,
	"medium":   2048,
	"high":     2880,
	"standard": 1024,
	"hd":       2048,
}

var workflowImageQualityAliases = map[string]string{
	"1k": "low",
	"2k": "medium",
	"4k": "high",
}

type workflowNodeOutput struct {
	NodeID string                   `json:"nodeId"`
	Type   string                   `json:"type"`
	Text   string                   `json:"text,omitempty"`
	Files  []workflowNodeOutputFile `json:"files,omitempty"`
	Meta   map[string]any           `json:"meta,omitempty"`
}

type workflowNodeOutputFile struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	MimeType string `json:"mimeType"`
}

type workflowProductContext struct {
	Run            model.WorkflowRun
	Input          map[string]any
	InputIndex     int
	ProductKey     string
	ProductTitle   string
	ProductDirName string
	ProductLogDir  string
	Outputs        map[string]workflowNodeOutput
}

type workflowRunStage struct {
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	StartedAt       string  `json:"started_at"`
	FinishedAt      string  `json:"finished_at,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Error           string  `json:"error,omitempty"`
}

type workflowRetryConfig struct {
	Enabled         bool
	RetryCount      int
	IntervalSeconds int
}

func customWorkflowWriteDir(runDir string) string {
	return filepath.Join(runDir, customRunDataRelDir)
}

func customWorkflowDataDir(runDir string) string {
	preferred := customWorkflowWriteDir(runDir)
	if exists(preferred) || !exists(filepath.Join(runDir, customRunLegacyDirName)) {
		return preferred
	}
	return filepath.Join(runDir, customRunLegacyDirName)
}

func customWorkflowRunLogPath(runDir string) string {
	return filepath.Join(runDir, customWorkflowRunLogRelPath)
}

func appendCustomWorkflowLog(run model.WorkflowRun, format string, args ...any) {
	path := customWorkflowRunLogPath(run.RunDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	line := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(file, "%s %s\n", now(), line)
}

func nodeStatusPayload(node model.WorkflowTemplateNode, status string, started string, finished string, output workflowNodeOutput, errorText string, parsedStarted time.Time) map[string]any {
	payload := map[string]any{
		"node":        node,
		"status":      status,
		"started_at":  started,
		"finished_at": finished,
		"updated_at":  finished,
	}
	if !parsedStarted.IsZero() {
		if parsedFinished, err := time.Parse(time.RFC3339, finished); err == nil {
			payload["duration_seconds"] = parsedFinished.Sub(parsedStarted).Seconds()
		}
	}
	if len(output.Files) > 0 || output.Text != "" || output.Type != "" {
		payload["output"] = output
	}
	if output.Meta != nil {
		if steps, ok := output.Meta["internalSteps"]; ok {
			payload["internal_steps"] = steps
		}
		if guardrail, ok := output.Meta["guardrail"]; ok {
			payload["guardrail"] = guardrail
		}
	}
	if errorText != "" {
		payload["error"] = errorText
	}
	return payload
}

func ListWorkflowTemplates(workflowType string, q model.Query) (model.WorkflowTemplateList, error) {
	items, total, err := repository.ListWorkflowTemplates(workflowType, q)
	for i := range items {
		items[i].Spec = normalizeWorkflowTemplateSpec(items[i].Spec)
	}
	return model.WorkflowTemplateList{Items: items, Total: int(total)}, err
}

func GetWorkflowTemplate(id string) (model.WorkflowTemplate, error) {
	item, err := repository.GetWorkflowTemplate(id)
	if err != nil {
		return model.WorkflowTemplate{}, err
	}
	item.Spec = normalizeWorkflowTemplateSpec(item.Spec)
	return item, nil
}

func SaveWorkflowTemplate(workflowType string, item model.WorkflowTemplate) (model.WorkflowTemplate, error) {
	current := now()
	if item.ID == "" {
		item.ID = newID("workflow-template")
		item.CreatedAt = current
	} else if saved, err := repository.GetWorkflowTemplate(item.ID); err == nil && item.CreatedAt == "" {
		item.CreatedAt = saved.CreatedAt
	}
	item.WorkflowType = workflowType
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		item.Title = "未命名工作流模板"
	}
	item.Spec = normalizeWorkflowTemplateSpec(item.Spec)
	if err := validateWorkflowTemplateSpec(item.Spec); err != nil {
		return model.WorkflowTemplate{}, err
	}
	item.UpdatedAt = current
	return repository.SaveWorkflowTemplate(item)
}

func DeleteWorkflowTemplate(id string) error {
	return repository.DeleteWorkflowTemplate(id)
}

func LoadPDDWorkflowThemes() ([]map[string]any, error) {
	path := filepath.Join(config.Cfg.PDDPromptsRoot, "themes.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	items := []any{}
	switch value := parsed.(type) {
	case []any:
		items = value
	case map[string]any:
		if list := anySlice(value["themes"]); len(list) > 0 {
			items = list
		} else if list := anySlice(value["items"]); len(list) > 0 {
			items = list
		}
	}
	result := []map[string]any{}
	for _, item := range items {
		switch value := item.(type) {
		case map[string]any:
			result = append(result, value)
		case string:
			result = append(result, map[string]any{"theme": value})
		}
	}
	return result, nil
}

func ListWorkflowRuns(workflowType string, q model.Query) (model.WorkflowRunList, error) {
	items, total, err := repository.ListWorkflowRuns(workflowType, q)
	return model.WorkflowRunList{Items: items, Total: int(total)}, err
}

func GetWorkflowRun(id string) (model.WorkflowRun, error) {
	return repository.GetWorkflowRun(id)
}

func StartWorkflowTemplateRun(templateID string, request model.StartWorkflowTemplateRunRequest) (model.StartWorkflowTemplateRunResult, error) {
	template, err := repository.GetWorkflowTemplate(templateID)
	if err != nil {
		return model.StartWorkflowTemplateRunResult{}, err
	}
	if template.WorkflowType != customWorkflowTypePDD {
		return model.StartWorkflowTemplateRunResult{}, safeMessageError{message: "当前只支持 PDD 工作流模板"}
	}
	if len(request.Inputs) == 0 {
		return model.StartWorkflowTemplateRunResult{}, safeMessageError{message: "至少需要 1 条输入"}
	}
	spec := normalizeWorkflowTemplateSpec(template.Spec)
	if err := validateWorkflowTemplateSpec(spec); err != nil {
		return model.StartWorkflowTemplateRunResult{}, err
	}
	runID := strings.TrimSpace(request.RunID)
	if runID == "" {
		runID = "custom_" + time.Now().Format("20060102_150405")
	}
	if !runIDPattern.MatchString(runID) {
		return model.StartWorkflowTemplateRunResult{}, safeMessageError{message: "run_id 不合法"}
	}
	runDir := filepath.Join(pddRunsRoot(), runID)
	if _, err := os.Stat(runDir); err == nil {
		return model.StartWorkflowTemplateRunResult{}, safeMessageError{message: "run_id 已存在"}
	}
	current := now()
	run := model.WorkflowRun{
		ID:            runID,
		WorkflowType:  template.WorkflowType,
		TemplateID:    template.ID,
		TemplateTitle: template.Title,
		Status:        model.WorkflowRunStatusRunning,
		RunDir:        runDir,
		InputCount:    len(request.Inputs),
		SpecSnapshot:  spec,
		CreatedAt:     current,
		UpdatedAt:     current,
	}
	if _, err := repository.SaveWorkflowRun(run); err != nil {
		return model.StartWorkflowTemplateRunResult{}, err
	}
	if err := initCustomWorkflowRunDir(run, template, request.Inputs); err != nil {
		run.Status = model.WorkflowRunStatusError
		run.Error = err.Error()
		run.UpdatedAt = now()
		_, _ = repository.SaveWorkflowRun(run)
		return model.StartWorkflowTemplateRunResult{}, err
	}
	go executeWorkflowTemplateRun(run, request.Inputs, request)
	return model.StartWorkflowTemplateRunResult{RunID: run.ID, RunDir: run.RunDir}, nil
}

func normalizeWorkflowTemplateSpec(spec model.WorkflowTemplateSpec) model.WorkflowTemplateSpec {
	if spec.Version <= 0 {
		spec.Version = 1
	}
	if spec.Settings.ProductConcurrency <= 0 {
		spec.Settings.ProductConcurrency = 2
	}
	if spec.Settings.MaxRetries <= 0 {
		spec.Settings.MaxRetries = 3
	}
	for i := range spec.Nodes {
		node := &spec.Nodes[i]
		node.ID = strings.TrimSpace(node.ID)
		if node.Title == "" {
			node.Title = node.ID
		}
		if node.Width <= 0 {
			node.Width = 320
		}
		if node.Height <= 0 {
			node.Height = 220
		}
		if node.Count <= 0 {
			node.Count = 1
		}
		if node.Count > 10 {
			node.Count = 10
		}
		if node.Operation == "" {
			node.Operation = defaultWorkflowNodeOperation(node.Type)
		}
		if node.Operation == "json_generation" {
			node.Operation = "text_generation"
			if node.Extra == nil {
				node.Extra = map[string]any{}
			}
			node.Extra["outputFormat"] = model.WorkflowTextOutputFormatJSON
		}
		if workflowNodeUsesModel(node.Operation) {
			node.Retry = normalizeWorkflowNodeRetry(node.Retry)
		} else {
			node.Retry = nil
		}
		upgradeDefaultOutputMappings(node)
	}
	return spec
}

func workflowNodeUsesModel(operation string) bool {
	switch operation {
	case "text_generation", "image_generation", "image_edit", "video_generation":
		return true
	default:
		return false
	}
}

func normalizeWorkflowNodeRetry(retry *model.WorkflowNodeRetry) *model.WorkflowNodeRetry {
	enabled := true
	if retry == nil {
		return &model.WorkflowNodeRetry{Enabled: &enabled, RetryCount: 0, IntervalSeconds: 0}
	}
	if retry.Enabled == nil {
		retry.Enabled = &enabled
	}
	if retry.RetryCount < 0 {
		retry.RetryCount = 0
	}
	if retry.IntervalSeconds < 0 {
		retry.IntervalSeconds = 0
	}
	return retry
}

func upgradeDefaultOutputMappings(node *model.WorkflowTemplateNode) {
	for i := range node.OutputMappings {
		switch strings.TrimSpace(node.OutputMappings[i].Path) {
		case "generated/{{productTitle}}/source_{{index}}.png":
			node.OutputMappings[i].Path = "generated/{{productTitle}}/{{index4}}.png"
		case "generated/{{productTitle}}/mockup_{{index}}.png":
			node.OutputMappings[i].Path = "待上架/{{productTitle}}/规格图/{{index1}}_规格图.png"
		case "待上架/{{productTitle}}/主图/1_主图.png":
			node.OutputMappings[i].Path = "待上架/{{productTitle}}/主图/{{index1}}_主图.png"
		}
	}
}

func defaultWorkflowNodeOperation(nodeType string) string {
	switch strings.TrimSpace(nodeType) {
	case "material":
		return "material_lookup"
	case "text":
		return "text_static"
	case "video":
		return "video_generation"
	default:
		return "image_generation"
	}
}

func validateWorkflowTemplateSpec(spec model.WorkflowTemplateSpec) error {
	if len(spec.Nodes) == 0 {
		return safeMessageError{message: "工作流模板至少需要 1 个节点"}
	}
	nodeByID := map[string]model.WorkflowTemplateNode{}
	for _, node := range spec.Nodes {
		if node.ID == "" {
			return safeMessageError{message: "存在缺少 ID 的节点"}
		}
		if _, ok := nodeByID[node.ID]; ok {
			return safeMessageError{message: "存在重复节点 ID"}
		}
		nodeByID[node.ID] = node
		if err := validateWorkflowOperation(node.Operation); err != nil {
			return err
		}
		for _, mapping := range node.OutputMappings {
			if strings.TrimSpace(mapping.Path) == "" {
				return safeMessageError{message: "输出路径不能为空"}
			}
			if err := validateOutputTemplatePath(mapping.Path); err != nil {
				return err
			}
		}
	}
	for _, edge := range spec.Edges {
		from, ok := nodeByID[edge.From]
		if !ok {
			return safeMessageError{message: "连线起点节点不存在"}
		}
		to, ok := nodeByID[edge.To]
		if !ok {
			return safeMessageError{message: "连线终点节点不存在"}
		}
		if err := validateMediaInputEdge(from, to); err != nil {
			return err
		}
	}
	_, err := workflowTopologicalNodes(spec)
	return err
}

func validateWorkflowOperation(operation string) error {
	switch operation {
	case "input", "material_lookup", "text_static", "text_generation", "condition", "script", "image_select", "image_generation", "image_edit", "video_generation":
		return nil
	default:
		return safeMessageError{message: "不支持的节点操作：" + operation}
	}
}

func validateMediaInputEdge(from model.WorkflowTemplateNode, to model.WorkflowTemplateNode) error {
	fromMedia := workflowOperationOutputType(from.Operation)
	if fromMedia == "video" {
		return safeMessageError{message: "第一版不支持把视频节点作为后续模型输入"}
	}
	if to.Operation == "image_generation" && fromMedia == "image" {
		return safeMessageError{message: "图片输入到图片节点时，请将目标节点操作改为 image_edit"}
	}
	return nil
}

func workflowOperationOutputType(operation string) string {
	switch operation {
	case "text_static", "text_generation", "condition", "script", "input":
		return "text"
	case "video_generation":
		return "video"
	case "image_select":
		return "image"
	default:
		return "image"
	}
}

func workflowTopologicalNodes(spec model.WorkflowTemplateSpec) ([]model.WorkflowTemplateNode, error) {
	nodeByID := map[string]model.WorkflowTemplateNode{}
	indegree := map[string]int{}
	nextByID := map[string][]string{}
	for _, node := range spec.Nodes {
		nodeByID[node.ID] = node
		indegree[node.ID] = 0
	}
	for _, edge := range spec.Edges {
		if workflowEdgeLoopEnabled(edge) {
			continue
		}
		if _, ok := nodeByID[edge.From]; !ok {
			return nil, safeMessageError{message: "连线起点节点不存在"}
		}
		if _, ok := nodeByID[edge.To]; !ok {
			return nil, safeMessageError{message: "连线终点节点不存在"}
		}
		nextByID[edge.From] = append(nextByID[edge.From], edge.To)
		indegree[edge.To]++
	}
	queue := []string{}
	for _, node := range spec.Nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	result := []model.WorkflowTemplateNode{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result = append(result, nodeByID[id])
		for _, nextID := range nextByID[id] {
			indegree[nextID]--
			if indegree[nextID] == 0 {
				queue = append(queue, nextID)
			}
		}
	}
	if len(result) != len(spec.Nodes) {
		return nil, safeMessageError{message: "工作流模板不能包含循环连线"}
	}
	return result, nil
}

func validateOutputTemplatePath(path string) error {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if filepath.IsAbs(cleaned) || cleaned == "." || strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return safeMessageError{message: "输出路径必须是 run 目录内的相对路径"}
	}
	return nil
}

func initCustomWorkflowRunDir(run model.WorkflowRun, template model.WorkflowTemplate, inputs []map[string]any) error {
	if err := os.MkdirAll(filepath.Join(run.RunDir, "logs"), 0755); err != nil {
		return err
	}
	customDir := customWorkflowWriteDir(run.RunDir)
	if err := os.MkdirAll(customDir, 0755); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(customDir, "template_snapshot.json"), template); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(customDir, "inputs.json"), inputs); err != nil {
		return err
	}
	appendCustomWorkflowLog(run, "run initialized template=%s inputs=%d", template.Title, len(inputs))
	if err := writeCustomManifest(run, false, ""); err != nil {
		return err
	}
	return writeCustomWorkflowStatus(run, "running", []workflowRunStage{{Name: "custom_workflow", Status: "running", StartedAt: now()}}, "")
}

func executeWorkflowTemplateRun(run model.WorkflowRun, inputs []map[string]any, request model.StartWorkflowTemplateRunRequest) {
	spec := run.SpecSnapshot
	concurrency := request.ProductConcurrency
	if concurrency <= 0 {
		concurrency = spec.Settings.ProductConcurrency
	}
	if concurrency <= 0 {
		concurrency = 2
	}
	if concurrency > len(inputs) {
		concurrency = len(inputs)
	}
	maxRetries := request.MaxRetries
	if maxRetries <= 0 {
		maxRetries = spec.Settings.MaxRetries
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	started := time.Now()
	stage := workflowRunStage{Name: "custom_workflow", Status: "running", StartedAt: started.Format(time.RFC3339)}
	ordered, err := workflowTopologicalNodes(spec)
	if err != nil {
		finishWorkflowRun(run, stage, err)
		return
	}
	appendCustomWorkflowLog(run, "run started products=%d concurrency=%d max_retries=%d", len(inputs), concurrency, maxRetries)
	var mu sync.Mutex
	completed := 0
	failed := 0
	run.Status = model.WorkflowRunStatusRunning
	writeProductSummary := func() {
		_ = writeCustomProductSummary(run)
		stage.DurationSeconds = time.Since(started).Seconds()
		_ = writeCustomWorkflowStatus(run, "running", []workflowRunStage{stage}, "")
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				err := executeWorkflowProduct(run, inputs[index], index+1, ordered, spec.Edges, maxRetries)
				mu.Lock()
				if err != nil {
					failed++
				} else {
					completed++
				}
				run.CompletedCount = completed
				run.FailedCount = failed
				run.UpdatedAt = now()
				_, _ = repository.SaveWorkflowRun(run)
				writeProductSummary()
				mu.Unlock()
			}
		}()
	}
	for index := range inputs {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	stage.FinishedAt = now()
	stage.DurationSeconds = time.Since(started).Seconds()
	if failed > 0 {
		stage.Status = "failed"
		appendCustomWorkflowLog(run, "run failed completed=%d failed=%d", completed, failed)
		finishWorkflowRun(run, stage, fmt.Errorf("custom workflow failed for %d product(s)", failed))
		return
	}
	stage.Status = "completed"
	run.Status = model.WorkflowRunStatusSuccess
	run.CompletedCount = completed
	run.FailedCount = failed
	run.UpdatedAt = now()
	_, _ = repository.SaveWorkflowRun(run)
	_ = writeCustomManifest(run, true, "")
	_ = writeCustomWorkflowStatus(run, "completed", []workflowRunStage{stage}, "")
	_ = writeCustomProductSummary(run)
	appendCustomWorkflowLog(run, "run completed products=%d duration=%.1fs", completed, stage.DurationSeconds)
}

func finishWorkflowRun(run model.WorkflowRun, stage workflowRunStage, err error) {
	if stage.FinishedAt == "" {
		stage.FinishedAt = now()
	}
	stage.Status = "failed"
	stage.Error = err.Error()
	run.Status = model.WorkflowRunStatusError
	run.Error = err.Error()
	run.UpdatedAt = now()
	_, _ = repository.SaveWorkflowRun(run)
	_ = writeCustomManifest(run, false, err.Error())
	_ = writeCustomWorkflowStatus(run, "failed", []workflowRunStage{stage}, err.Error())
	_ = writeCustomProductSummary(run)
}

func executeWorkflowProduct(run model.WorkflowRun, input map[string]any, inputIndex int, nodes []model.WorkflowTemplateNode, edges []model.WorkflowTemplateEdge, maxRetries int) error {
	productTitle := productTitleFromInput(input, inputIndex)
	productKey := safeFileName(productTitle, fmt.Sprintf("product_%03d", inputIndex))
	productDirName := safeFileName(productTitle, productKey)
	ctx := workflowProductContext{
		Run:            run,
		Input:          input,
		InputIndex:     inputIndex,
		ProductKey:     productKey,
		ProductTitle:   productTitle,
		ProductDirName: productDirName,
		ProductLogDir:  filepath.Join(run.RunDir, "logs", "product_pipeline", productKey),
		Outputs:        map[string]workflowNodeOutput{},
	}
	started := now()
	if err := os.MkdirAll(ctx.ProductLogDir, 0755); err != nil {
		return err
	}
	appendCustomWorkflowLog(run, "product started index=%03d key=%s title=%s", inputIndex, productKey, productTitle)
	_ = writePipelineStatus(ctx, "running", started, "", "")
	if err := executeWorkflowProductGraph(ctx, nodes, edges, maxRetries); err != nil {
		_ = writePipelineStatus(ctx, "failed", started, now(), err.Error())
		appendCustomWorkflowLog(run, "product failed key=%s error=%s", productKey, err.Error())
		return err
	}
	ctx.ProductTitle = productTitleFromInput(ctx.Input, inputIndex)
	ctx.ProductDirName = safeFileName(ctx.ProductTitle, ctx.ProductKey)
	_ = writePipelineStatus(ctx, "completed", started, now(), "")
	appendCustomWorkflowLog(run, "product completed key=%s", productKey)
	return nil
}

func executeWorkflowProductGraph(ctx workflowProductContext, nodes []model.WorkflowTemplateNode, edges []model.WorkflowTemplateEdge, maxRetries int) error {
	nodeByID := map[string]model.WorkflowTemplateNode{}
	orderedIDs := []string{}
	remainingIncoming := map[string]int{}
	outgoing := map[string][]model.WorkflowTemplateEdge{}
	for _, node := range nodes {
		nodeByID[node.ID] = node
		orderedIDs = append(orderedIDs, node.ID)
		remainingIncoming[node.ID] = 0
	}
	for _, edge := range edges {
		if _, ok := nodeByID[edge.From]; !ok {
			continue
		}
		if _, ok := nodeByID[edge.To]; !ok {
			continue
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge)
		if !workflowEdgeLoopEnabled(edge) {
			remainingIncoming[edge.To]++
		}
	}
	queue := []string{}
	queued := map[string]bool{}
	active := map[string]bool{}
	skipped := map[string]bool{}
	blocked := map[string]bool{}
	for _, id := range orderedIDs {
		if remainingIncoming[id] == 0 {
			queue = append(queue, id)
			queued[id] = true
			active[id] = true
		}
	}
	executed := map[string]int{}
	loopCounts := map[string]int{}
	var skipNode func(string)
	resolveEdge := func(edge model.WorkflowTemplateEdge, follow bool, rerun bool) {
		if workflowEdgeLoopEnabled(edge) {
			return
		}
		if rerun {
			if follow {
				active[edge.To] = true
				skipped[edge.To] = false
				blocked[edge.To] = false
				if !queued[edge.To] {
					queue = append(queue, edge.To)
					queued[edge.To] = true
				}
			}
			return
		}
		remainingIncoming[edge.To]--
		if follow {
			active[edge.To] = true
			skipped[edge.To] = false
			if workflowEdgeHasGate(edge) {
				blocked[edge.To] = false
			}
		} else if workflowEdgeHasGate(edge) {
			blocked[edge.To] = true
		}
		if remainingIncoming[edge.To] > 0 || executed[edge.To] > 0 || queued[edge.To] {
			return
		}
		if active[edge.To] && !blocked[edge.To] {
			queue = append(queue, edge.To)
			queued[edge.To] = true
			return
		}
		skipNode(edge.To)
	}
	skipNode = func(nodeID string) {
		if skipped[nodeID] || executed[nodeID] > 0 || active[nodeID] {
			return
		}
		skipped[nodeID] = true
		for _, edge := range outgoing[nodeID] {
			resolveEdge(edge, false, false)
		}
	}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		queued[nodeID] = false
		if skipped[nodeID] || !active[nodeID] {
			continue
		}
		node, ok := nodeByID[nodeID]
		if !ok {
			continue
		}
		output, err := executeWorkflowNode(ctx, node, edges, maxRetries)
		if err != nil {
			return fmt.Errorf("node=%s: %w", node.ID, err)
		}
		ctx.Outputs[node.ID] = output
		executed[node.ID]++
		rerun := executed[node.ID] > 1
		for _, edge := range outgoing[node.ID] {
			if workflowEdgeLoopEnabled(edge) {
				if !workflowEdgeShouldFollow(edge, output) {
					continue
				}
				loopKey := workflowEdgeKey(edge)
				loopCounts[loopKey]++
				maxIterations := workflowEdgeLoopMax(edge)
				if maxIterations > 0 && loopCounts[loopKey] > maxIterations {
					return safeMessageError{message: fmt.Sprintf("循环边超过最大轮次：%s -> %s", edge.From, edge.To)}
				}
				if !queued[edge.To] {
					queue = append(queue, edge.To)
					queued[edge.To] = true
				}
				continue
			}
			resolveEdge(edge, workflowEdgeShouldFollow(edge, output), rerun)
		}
	}
	return nil
}

func executeWorkflowNode(ctx workflowProductContext, node model.WorkflowTemplateNode, edges []model.WorkflowTemplateEdge, maxRetries int) (workflowNodeOutput, error) {
	nodeDir := filepath.Join(customWorkflowWriteDir(ctx.Run.RunDir), "products", fmt.Sprintf("%03d_%s", ctx.InputIndex, ctx.ProductKey), "nodes", safeFileName(node.ID, "node"))
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return workflowNodeOutput{}, err
	}
	statusPath := filepath.Join(nodeDir, "status.json")
	nodeStarted := now()
	startedAt, _ := time.Parse(time.RFC3339, nodeStarted)
	_ = writeJSONFile(statusPath, map[string]any{"node": node, "status": "running", "started_at": nodeStarted})
	appendCustomWorkflowLog(ctx.Run, "node started product=%s node=%s title=%s operation=%s", ctx.ProductKey, node.ID, node.Title, node.Operation)
	var output workflowNodeOutput
	var err error
	retryConfig := workflowNodeRetryConfig(node, maxRetries)
	for attempt := 1; ; attempt++ {
		output, err = executeWorkflowNodeOnce(ctx, node, edges, nodeDir)
		if err == nil {
			break
		}
		if !isRetryableWorkflowError(err) || !workflowShouldRetry(retryConfig, attempt) {
			break
		}
		delay := workflowRetryDelay(retryConfig, attempt)
		appendCustomWorkflowLog(ctx.Run, "node retry product=%s node=%s attempt=%d delay=%s error=%s", ctx.ProductKey, node.ID, attempt, delay.String(), err.Error())
		time.Sleep(delay)
	}
	if err != nil {
		finishedAt := now()
		_ = writeJSONFile(statusPath, nodeStatusPayload(node, "failed", nodeStarted, finishedAt, workflowNodeOutput{}, err.Error(), startedAt))
		appendCustomWorkflowLog(ctx.Run, "node failed product=%s node=%s error=%s", ctx.ProductKey, node.ID, err.Error())
		return workflowNodeOutput{}, err
	}
	if workflowNodeTitleProvider(node) {
		if title := workflowTitleFromOutput(output); title != "" {
			ctx.Input["productTitle"] = title
			ctx.Input["product_title"] = title
			appendCustomWorkflowLog(ctx.Run, "title updated product=%s node=%s title=%s", ctx.ProductKey, node.ID, title)
		}
	}
	if err := applyWorkflowOutputMappings(ctx, node, output); err != nil {
		finishedAt := now()
		_ = writeJSONFile(statusPath, nodeStatusPayload(node, "failed", nodeStarted, finishedAt, workflowNodeOutput{}, err.Error(), startedAt))
		appendCustomWorkflowLog(ctx.Run, "node failed product=%s node=%s output_mapping_error=%s", ctx.ProductKey, node.ID, err.Error())
		return workflowNodeOutput{}, err
	}
	finishedAt := now()
	_ = writeJSONFile(statusPath, nodeStatusPayload(node, "completed", nodeStarted, finishedAt, output, "", startedAt))
	appendCustomWorkflowLog(ctx.Run, "node completed product=%s node=%s files=%d", ctx.ProductKey, node.ID, len(output.Files))
	return output, nil
}

func workflowNodeRetryConfig(node model.WorkflowTemplateNode, legacyMaxRetries int) workflowRetryConfig {
	if workflowNodeUsesModel(node.Operation) {
		retry := normalizeWorkflowNodeRetry(node.Retry)
		return workflowRetryConfig{Enabled: retry.Enabled == nil || *retry.Enabled, RetryCount: retry.RetryCount, IntervalSeconds: retry.IntervalSeconds}
	}
	if legacyMaxRetries <= 1 {
		return workflowRetryConfig{Enabled: false}
	}
	return workflowRetryConfig{Enabled: true, RetryCount: legacyMaxRetries - 1}
}

func workflowShouldRetry(config workflowRetryConfig, attempt int) bool {
	if !config.Enabled {
		return false
	}
	if config.RetryCount == 0 {
		return true
	}
	return attempt <= config.RetryCount
}

func workflowRetryDelay(config workflowRetryConfig, attempt int) time.Duration {
	if config.IntervalSeconds > 0 {
		return time.Duration(config.IntervalSeconds) * time.Second
	}
	return workflowTransientRetryDelay(attempt)
}

func executeWorkflowNodeOnce(ctx workflowProductContext, node model.WorkflowTemplateNode, edges []model.WorkflowTemplateEdge, nodeDir string) (workflowNodeOutput, error) {
	upstreamRefs := upstreamNodeOutputRefs(node.ID, edges, ctx.Outputs)
	upstream := upstreamOutputsFromRefs(upstreamRefs)
	prompt := renderWorkflowTemplateWithRefs(node.Prompt, ctx, node, 1, upstreamRefs)
	textInputs := upstreamTexts(upstream)
	imageInputs := upstreamFilesFromRefs(upstreamRefs, "image")
	if textInputs != "" {
		prompt = strings.TrimSpace(prompt + "\n\n" + textInputs)
	}
	switch node.Operation {
	case "input":
		body, _ := json.MarshalIndent(ctx.Input, "", "  ")
		return writeTextNodeOutput(node, nodeDir, string(body))
	case "material_lookup":
		return materialLookupNodeOutput(ctx, node, nodeDir)
	case "text_static":
		return writeTextNodeOutput(node, nodeDir, prompt)
	case "text_generation":
		text, err := requestWorkflowText(node.Model, prompt, imageInputs)
		if err != nil {
			return workflowNodeOutput{}, err
		}
		if workflowTextOutputFormat(node) == model.WorkflowTextOutputFormatJSON {
			return writeJSONNodeOutput(node, nodeDir, text)
		}
		return writeTextNodeOutput(node, nodeDir, text)
	case "condition":
		return conditionNodeOutput(node, nodeDir, textInputs)
	case "script":
		return scriptNodeOutput(ctx, node, nodeDir)
	case "image_select":
		return imageSelectNodeOutput(node, nodeDir, upstream)
	case "image_generation":
		if workflowImageGuardrailEnabled(node) {
			return requestWorkflowImagesWithGuardrail(ctx, node, nodeDir, prompt, nil)
		}
		return requestWorkflowImages(node, nodeDir, prompt, nil)
	case "image_edit":
		if workflowImageGuardrailEnabled(node) {
			return requestWorkflowImagesWithGuardrail(ctx, node, nodeDir, prompt, imageInputs)
		}
		return requestWorkflowImages(node, nodeDir, prompt, imageInputs)
	case "video_generation":
		return requestWorkflowVideo(node, nodeDir, prompt, imageInputs)
	default:
		return workflowNodeOutput{}, safeMessageError{message: "不支持的节点操作：" + node.Operation}
	}
}

func materialLookupNodeOutput(ctx workflowProductContext, node model.WorkflowTemplateNode, nodeDir string) (workflowNodeOutput, error) {
	if fixedWorkflowAssetMode(node) {
		assetID := fixedWorkflowAssetID(node)
		if assetID == "" {
			return workflowNodeOutput{}, safeMessageError{message: "固定素材节点未选择素材：" + node.Title}
		}
		asset, err := repository.GetAsset(assetID)
		if err != nil {
			return workflowNodeOutput{}, err
		}
		return materialAssetNodeOutput(node, nodeDir, asset)
	}
	asset, path, err := findPDDMaterialForInput(ctx.Input)
	if err != nil {
		return workflowNodeOutput{}, err
	}
	return materialAssetNodeOutput(node, nodeDir, asset, path)
}

func materialAssetNodeOutput(node model.WorkflowTemplateNode, nodeDir string, asset model.Asset, resolvedPath ...string) (workflowNodeOutput, error) {
	path := ""
	if len(resolvedPath) > 0 {
		path = resolvedPath[0]
	}
	if path == "" {
		var err error
		path, err = assetFilePath(asset)
		if err != nil {
			return workflowNodeOutput{}, err
		}
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".png"
	}
	outputPath := filepath.Join(nodeDir, "output_01"+ext)
	if err := copyFile(path, outputPath); err != nil {
		return workflowNodeOutput{}, err
	}
	return workflowNodeOutput{
		NodeID: node.ID,
		Type:   "image",
		Text:   asset.Title,
		Files: []workflowNodeOutputFile{{
			Path:     outputPath,
			Kind:     "image",
			MimeType: mime.TypeByExtension(ext),
		}},
	}, nil
}

func fixedWorkflowAssetID(node model.WorkflowTemplateNode) string {
	if node.Extra == nil {
		return ""
	}
	value, _ := node.Extra["assetId"].(string)
	return strings.TrimSpace(value)
}

func fixedWorkflowAssetMode(node model.WorkflowTemplateNode) bool {
	if node.Extra == nil {
		return false
	}
	mode, _ := node.Extra["assetMode"].(string)
	return strings.TrimSpace(mode) == "fixed" || fixedWorkflowAssetID(node) != ""
}

func workflowTextOutputFormat(node model.WorkflowTemplateNode) string {
	format := strings.ToLower(workflowExtraString(node.Extra, "outputFormat"))
	if format == "" {
		format = strings.ToLower(workflowExtraString(node.Extra, "output_format"))
	}
	if format == model.WorkflowTextOutputFormatJSON {
		return model.WorkflowTextOutputFormatJSON
	}
	return model.WorkflowTextOutputFormatText
}

func assetFilePath(asset model.Asset) (string, error) {
	if asset.Type != model.AssetTypeImage {
		return "", safeMessageError{message: "固定素材不是图片：" + asset.ID}
	}
	parsed, err := url.Parse(asset.URL)
	if err != nil {
		return "", err
	}
	switch parsed.Path {
	case "/api/assets/pdd-materials/file":
		return ResolvePDDMaterialFile(parsed.Query().Get("path"))
	case "/api/assets/local/file":
		return ResolveConsoleAssetFile(parsed.Query().Get("path"))
	default:
		return "", safeMessageError{message: "固定素材不支持作为工作流输入：" + asset.ID}
	}
}

func writeTextNodeOutput(node model.WorkflowTemplateNode, nodeDir string, text string) (workflowNodeOutput, error) {
	outputPath := filepath.Join(nodeDir, "output_01.txt")
	if err := os.WriteFile(outputPath, []byte(text), 0644); err != nil {
		return workflowNodeOutput{}, err
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "text", Text: text, Files: []workflowNodeOutputFile{{Path: outputPath, Kind: "text", MimeType: "text/plain; charset=utf-8"}}}, nil
}

func writeJSONNodeOutput(node model.WorkflowTemplateNode, nodeDir string, text string) (workflowNodeOutput, error) {
	cleaned, err := normalizeJSONText(text)
	if err != nil {
		return workflowNodeOutput{}, err
	}
	outputPath := filepath.Join(nodeDir, "output_01.json")
	if err := os.WriteFile(outputPath, []byte(cleaned+"\n"), 0644); err != nil {
		return workflowNodeOutput{}, err
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "json", Text: cleaned, Files: []workflowNodeOutputFile{{Path: outputPath, Kind: "json", MimeType: "application/json"}}}, nil
}

func conditionNodeOutput(node model.WorkflowTemplateNode, nodeDir string, inputText string) (workflowNodeOutput, error) {
	payload, _ := parseWorkflowJSON(inputText)
	rules := workflowConditionRules(node.Extra)
	decision := workflowExtraString(node.Extra, "defaultOutput")
	if decision == "" {
		decision = workflowExtraString(node.Extra, "defaultDecision")
	}
	if decision == "" {
		decision = "default"
	}
	matched := -1
	for index, rule := range rules {
		if workflowRuleMatches(payload, rule) {
			if value := firstString(anyToString(rule["output"]), anyToString(rule["decision"]), anyToString(rule["handle"])); value != "" {
				decision = value
			}
			matched = index
			break
		}
	}
	body, _ := json.MarshalIndent(map[string]any{
		"decision":       decision,
		"matched_rule":   matched,
		"source_summary": truncateText(inputText, 800),
	}, "", "  ")
	outputPath := filepath.Join(nodeDir, "output_01.json")
	if err := os.WriteFile(outputPath, append(body, '\n'), 0644); err != nil {
		return workflowNodeOutput{}, err
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "json", Text: string(body), Files: []workflowNodeOutputFile{{Path: outputPath, Kind: "json", MimeType: "application/json"}}}, nil
}

func scriptNodeOutput(ctx workflowProductContext, node model.WorkflowTemplateNode, nodeDir string) (workflowNodeOutput, error) {
	executor := firstString(workflowExtraString(node.Extra, "executor"), "vps")
	scriptPath := strings.TrimSpace(workflowExtraString(node.Extra, "scriptPath"))
	if scriptPath == "" {
		return workflowNodeOutput{}, safeMessageError{message: "脚本节点缺少 scriptPath"}
	}
	timeout := time.Duration(workflowExtraInt(node.Extra, "timeoutSeconds", 600)) * time.Second
	args := renderScriptArgs(node, ctx)
	var output string
	var err error
	if executor == "local_agent" {
		output, err = RunLocalAgentScript(LocalAgentScriptJob{
			RunID:          ctx.Run.ID,
			ProductKey:     ctx.ProductKey,
			NodeID:         node.ID,
			ScriptPath:     scriptPath,
			Args:           args,
			TimeoutSeconds: int(timeout.Seconds()),
		}, timeout)
	} else {
		output, err = runVPSScript(scriptPath, args, timeout)
	}
	outputPath := filepath.Join(nodeDir, "output_01.txt")
	if writeErr := os.WriteFile(outputPath, []byte(output), 0644); writeErr != nil {
		return workflowNodeOutput{}, writeErr
	}
	if err != nil {
		return workflowNodeOutput{}, scriptNodeError(err, output)
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "text", Text: output, Files: []workflowNodeOutputFile{{Path: outputPath, Kind: "text", MimeType: "text/plain; charset=utf-8"}}}, nil
}

func imageSelectNodeOutput(node model.WorkflowTemplateNode, nodeDir string, upstream []workflowNodeOutput) (workflowNodeOutput, error) {
	mode := firstString(workflowExtraString(node.Extra, "selectMode"), "last")
	selected := []workflowNodeOutputFile{}
	for _, output := range upstream {
		files := []workflowNodeOutputFile{}
		for _, file := range output.Files {
			if file.Kind == "image" || isImagePath(file.Path) {
				files = append(files, file)
			}
		}
		if len(files) == 0 {
			continue
		}
		switch mode {
		case "all":
			selected = append(selected, files...)
		case "first":
			if len(selected) == 0 {
				selected = files
			}
		default:
			selected = files
		}
	}
	if len(selected) == 0 {
		return workflowNodeOutput{}, safeMessageError{message: "图片选择节点没有可用的上游图片"}
	}
	files := []workflowNodeOutputFile{}
	for index, file := range selected {
		ext := filepath.Ext(file.Path)
		if ext == "" {
			ext = ".png"
		}
		outputPath := filepath.Join(nodeDir, fmt.Sprintf("output_%02d%s", index+1, ext))
		if err := copyFile(file.Path, outputPath); err != nil {
			return workflowNodeOutput{}, err
		}
		files = append(files, workflowNodeOutputFile{Path: outputPath, Kind: "image", MimeType: firstString(file.MimeType, mime.TypeByExtension(ext))})
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "image", Files: files}, nil
}

func requestWorkflowImages(node model.WorkflowTemplateNode, nodeDir string, prompt string, refs []workflowNodeOutputFile) (workflowNodeOutput, error) {
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return workflowNodeOutput{}, err
	}
	count := normalizeWorkflowImageCount(node.Count)
	var images [][]byte
	var err error
	if len(refs) > 0 {
		images, err = requestWorkflowImageEdit(node, prompt, refs, count)
	} else {
		images, err = requestWorkflowImageGeneration(node, prompt, count)
	}
	if err != nil {
		return workflowNodeOutput{}, err
	}
	files := []workflowNodeOutputFile{}
	for index, body := range images {
		if index >= count {
			break
		}
		outputPath := filepath.Join(nodeDir, fmt.Sprintf("output_%02d.png", index+1))
		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			return workflowNodeOutput{}, err
		}
		files = append(files, workflowNodeOutputFile{Path: outputPath, Kind: "image", MimeType: "image/png"})
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "image", Files: files}, nil
}

func normalizeWorkflowImageCount(count int) int {
	if count <= 0 {
		return 1
	}
	if count > workflowImageRequestMaxCount {
		return workflowImageRequestMaxCount
	}
	return count
}

type workflowGuardrailDecision struct {
	Decision     string   `json:"decision"`
	Severity     string   `json:"severity"`
	Issues       []string `json:"issues"`
	RepairPrompt string   `json:"repair_prompt"`
	Raw          any      `json:"raw,omitempty"`
}

func requestWorkflowImagesWithGuardrail(ctx workflowProductContext, node model.WorkflowTemplateNode, nodeDir string, prompt string, refs []workflowNodeOutputFile) (workflowNodeOutput, error) {
	guardrailDir := filepath.Join(nodeDir, "guardrail")
	if err := os.MkdirAll(guardrailDir, 0755); err != nil {
		return workflowNodeOutput{}, err
	}
	steps := []map[string]any{}
	currentRefs := refs
	generationAttempts := workflowGuardrailInt(node, "regenerate", "maxRounds", 1)
	if generationAttempts <= 0 {
		generationAttempts = 1
	}
	repairMax := workflowGuardrailInt(node, "repair", "maxRounds", 5)
	if repairMax < 0 {
		repairMax = 0
	}
	transientRetry := workflowGuardrailTransientRetry(node)
	var lastOutput workflowNodeOutput
	var lastDecision workflowGuardrailDecision
	for generationRound := 1; generationRound <= generationAttempts; generationRound++ {
		roundDir := filepath.Join(guardrailDir, fmt.Sprintf("generation_%02d", generationRound))
		output, err := requestWorkflowImagesWithTransientRetry(node, roundDir, prompt, refs, transientRetry)
		steps = append(steps, guardrailStep("generate", generationRound, 0, output, err, ""))
		if err != nil {
			return workflowNodeOutput{}, err
		}
		lastOutput = output
		if !workflowGuardrailBool(node, "review", "enabled", true) {
			final, err := copyGuardrailFinalOutput(node, nodeDir, output)
			if err != nil {
				return workflowNodeOutput{}, err
			}
			final.Meta = map[string]any{
				"internalSteps": steps,
				"guardrail": map[string]any{
					"enabled":          true,
					"preset":           workflowGuardrailPreset(node),
					"decision":         "pass",
					"severity":         "minor",
					"repair_rounds":    0,
					"generation_round": generationRound,
					"review_skipped":   true,
				},
			}
			return final, nil
		}
		for repairRound := 0; repairRound <= repairMax; repairRound++ {
			review, err := reviewWorkflowGuardrailImages(node, guardrailDir, generationRound, repairRound, prompt, output.Files, append(output.Files, refs...), transientRetry)
			steps = append(steps, guardrailStep("review", generationRound, repairRound, output, err, review.Decision))
			if err != nil {
				return workflowNodeOutput{}, err
			}
			lastDecision = review
			if workflowGuardrailReviewPass(review) {
				final, err := copyGuardrailFinalOutput(node, nodeDir, output)
				if err != nil {
					return workflowNodeOutput{}, err
				}
				final.Meta = map[string]any{
					"internalSteps": steps,
					"guardrail": map[string]any{
						"enabled":          true,
						"preset":           workflowGuardrailPreset(node),
						"decision":         review.Decision,
						"severity":         review.Severity,
						"repair_rounds":    repairRound,
						"generation_round": generationRound,
					},
				}
				return final, nil
			}
			if !workflowGuardrailBool(node, "repair", "enabled", true) || repairRound >= repairMax {
				break
			}
			repairPrompt := workflowGuardrailRepairPrompt(node, prompt, review)
			repairNode := workflowGuardrailRepairNode(node)
			repairDir := filepath.Join(guardrailDir, fmt.Sprintf("generation_%02d_repair_%02d", generationRound, repairRound+1))
			repairRefs := append([]workflowNodeOutputFile{}, output.Files...)
			if workflowGuardrailBool(node, "repair", "includeReferenceImages", true) {
				repairRefs = append(repairRefs, currentRefs...)
			}
			repaired, err := requestWorkflowImagesWithTransientRetry(repairNode, repairDir, repairPrompt, repairRefs, transientRetry)
			steps = append(steps, guardrailStep("repair", generationRound, repairRound+1, repaired, err, ""))
			if err != nil {
				return workflowNodeOutput{}, err
			}
			output = repaired
			lastOutput = output
		}
		if workflowGuardrailFailureStrategy(node) != "regenerate" {
			break
		}
	}
	strategy := workflowGuardrailFailureStrategy(node)
	if strategy == "manual_review" || strategy == "regenerate" {
		_ = writeJSONFile(filepath.Join(guardrailDir, "manual_review_required.json"), map[string]any{
			"node_id":       node.ID,
			"product":       ctx.ProductKey,
			"decision":      lastDecision,
			"last_output":   lastOutput,
			"internalSteps": steps,
			"created_at":    now(),
		})
	}
	return workflowNodeOutput{}, safeMessageError{message: fmt.Sprintf("图片节点 %s 质检/修复未通过", node.Title)}
}

func requestWorkflowImagesWithTransientRetry(node model.WorkflowTemplateNode, nodeDir string, prompt string, refs []workflowNodeOutputFile, retryConfig workflowRetryConfig) (workflowNodeOutput, error) {
	var output workflowNodeOutput
	var err error
	for attempt := 1; ; attempt++ {
		output, err = requestWorkflowImages(node, nodeDir, prompt, refs)
		if err == nil {
			return output, nil
		}
		if !isRetryableWorkflowError(err) || !workflowShouldRetry(retryConfig, attempt) {
			return workflowNodeOutput{}, err
		}
		time.Sleep(workflowRetryDelay(retryConfig, attempt))
	}
}

func requestWorkflowTextWithTransientRetry(modelName string, prompt string, refs []workflowNodeOutputFile, retryConfig workflowRetryConfig) (string, error) {
	var text string
	var err error
	for attempt := 1; ; attempt++ {
		text, err = requestWorkflowText(modelName, prompt, refs)
		if err == nil {
			return text, nil
		}
		if !isRetryableWorkflowError(err) || !workflowShouldRetry(retryConfig, attempt) {
			return "", err
		}
		time.Sleep(workflowRetryDelay(retryConfig, attempt))
	}
}

func workflowTransientRetryDelay(attempt int) time.Duration {
	seconds := 2
	for i := 1; i < attempt && seconds < 60; i++ {
		seconds *= 2
	}
	if seconds > 60 {
		seconds = 60
	}
	jitter := time.Duration(time.Now().UnixNano()%1500) * time.Millisecond
	return time.Duration(seconds)*time.Second + jitter
}

func reviewWorkflowGuardrailImages(node model.WorkflowTemplateNode, guardrailDir string, generationRound int, repairRound int, basePrompt string, outputFiles []workflowNodeOutputFile, refs []workflowNodeOutputFile, retryConfig workflowRetryConfig) (workflowGuardrailDecision, error) {
	reviewModel := workflowGuardrailString(node, "review", "model", "gpt-5.5")
	reviewPrompt := workflowGuardrailReviewPrompt(node, basePrompt)
	text, err := requestWorkflowTextWithTransientRetry(reviewModel, reviewPrompt, refs, retryConfig)
	if err != nil {
		return workflowGuardrailDecision{}, err
	}
	decision, err := parseWorkflowGuardrailDecision(text)
	if err != nil {
		return workflowGuardrailDecision{}, err
	}
	reviewPath := filepath.Join(guardrailDir, fmt.Sprintf("generation_%02d_review_%02d.json", generationRound, repairRound))
	_ = writeJSONFile(reviewPath, map[string]any{
		"decision":      decision,
		"review_text":   text,
		"output_files":  outputFiles,
		"reviewed_at":   now(),
		"repair_round":  repairRound,
		"generation_id": generationRound,
	})
	return decision, nil
}

func parseWorkflowGuardrailDecision(text string) (workflowGuardrailDecision, error) {
	payload, err := parseWorkflowJSON(text)
	if err != nil {
		return workflowGuardrailDecision{}, err
	}
	record, ok := payload.(map[string]any)
	if !ok {
		return workflowGuardrailDecision{}, safeMessageError{message: "质检结果必须是 JSON 对象"}
	}
	decision := strings.ToLower(firstString(anyToString(record["decision"]), anyToString(record["status"]), "pass"))
	severity := strings.ToLower(firstString(anyToString(record["severity"]), anyToString(record["level"]), "minor"))
	issues := []string{}
	for _, item := range anySlice(record["issues"]) {
		if value := strings.TrimSpace(anyToString(item)); value != "" {
			issues = append(issues, value)
		}
	}
	if len(issues) == 0 {
		if value := strings.TrimSpace(anyToString(record["issue"])); value != "" {
			issues = append(issues, value)
		}
	}
	return workflowGuardrailDecision{
		Decision:     decision,
		Severity:     severity,
		Issues:       issues,
		RepairPrompt: firstString(anyToString(record["repair_prompt"]), anyToString(record["repairPrompt"]), anyToString(record["repair_instruction"])),
		Raw:          payload,
	}, nil
}

func workflowGuardrailReviewPass(review workflowGuardrailDecision) bool {
	severity := strings.ToLower(strings.TrimSpace(review.Severity))
	decision := strings.ToLower(strings.TrimSpace(review.Decision))
	if severity == "minor" || severity == "none" || severity == "ok" {
		return true
	}
	return decision == "pass" || decision == "ok" || decision == "approve" || decision == "approved"
}

func copyGuardrailFinalOutput(node model.WorkflowTemplateNode, nodeDir string, output workflowNodeOutput) (workflowNodeOutput, error) {
	files := []workflowNodeOutputFile{}
	for index, file := range output.Files {
		ext := filepath.Ext(file.Path)
		if ext == "" {
			ext = ".png"
		}
		target := filepath.Join(nodeDir, fmt.Sprintf("output_%02d%s", index+1, ext))
		if err := copyFile(file.Path, target); err != nil {
			return workflowNodeOutput{}, err
		}
		files = append(files, workflowNodeOutputFile{Path: target, Kind: firstString(file.Kind, "image"), MimeType: firstString(file.MimeType, mime.TypeByExtension(ext))})
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "image", Files: files}, nil
}

func workflowGuardrailReviewPrompt(node model.WorkflowTemplateNode, basePrompt string) string {
	if custom := workflowGuardrailString(node, "review", "prompt", ""); custom != "" {
		return custom + "\n\n# Original image-generation requirements\n" + basePrompt
	}
	preset := workflowGuardrailPreset(node)
	focus := "Check only clear production blockers. Do not fail for personal taste or harmless style differences."
	switch preset {
	case "pdd_source":
		focus = "This is PDD source artwork QA. Check obvious hand/foot/face/anatomy defects, broken full-body visibility, broken two-panel structure, character identity drift, unsafe/suggestive details, and prompt-breaking composition."
	case "pdd_mockup":
		focus = "This is PDD body-pillow mockup QA. Check product readability, complete pillow-cover shape, visible source artwork, front/back relationship when applicable, and severe artifacting."
	case "pdd_main":
		focus = "This is PDD ecommerce main-image QA. Check hero product dominance, complete product edges, readable Chinese copy, SKU identity, marketplace safety, and severe layout/artifact blockers."
	}
	return strings.TrimSpace(`You are a strict but practical image QA reviewer for a no-code workflow.
Return JSON only. Do not use Markdown.

Output schema:
{
  "decision": "pass" | "repair",
  "severity": "minor" | "major" | "critical",
  "issues": ["short issue strings"],
  "repair_prompt": "conservative repair instruction if decision is repair"
}

Rules:
- Minor issues are record-only and should use decision "pass".
- Use decision "repair" only for major or critical issues that would block production.
- Do not fail for subjective taste, tiny style differences, harmless layout variation, or minor imperfections.
- If repair is needed, write a conservative repair_prompt that preserves the task, character, composition, and upstream references.

` + focus + `

Uploaded images:
- The first uploaded image(s) are the generated output to review.
- Any later uploaded images are upstream references and should be used only for identity/product consistency.

# Original image-generation requirements
` + basePrompt)
}

func workflowGuardrailRepairPrompt(node model.WorkflowTemplateNode, basePrompt string, review workflowGuardrailDecision) string {
	if custom := workflowGuardrailString(node, "repair", "prompt", ""); custom != "" {
		return strings.TrimSpace(custom + "\n\n# Issues\n" + strings.Join(review.Issues, "\n") + "\n\n# Repair instruction\n" + review.RepairPrompt + "\n\n# Original requirements\n" + basePrompt)
	}
	return strings.TrimSpace(`Edit the provided image conservatively.
The first uploaded image is the failed generated image and is the primary edit target.
Additional uploaded images are references for identity/product consistency only.

Fix only the listed major/critical issues. Preserve the original task, composition, character/product identity, style, aspect ratio, and safe marketplace presentation.

# Issues
` + strings.Join(review.Issues, "\n") + `

# Repair instruction
` + firstString(review.RepairPrompt, "Repair the visible blockers while preserving the original prompt requirements.") + `

# Original requirements
` + basePrompt)
}

func workflowGuardrailRepairNode(node model.WorkflowTemplateNode) model.WorkflowTemplateNode {
	repairNode := node
	if modelName := workflowGuardrailString(node, "repair", "model", ""); modelName != "" {
		repairNode.Model = modelName
	}
	if size := workflowGuardrailString(node, "repair", "size", ""); size != "" {
		repairNode.Size = size
	}
	if quality := workflowGuardrailString(node, "repair", "quality", ""); quality != "" {
		repairNode.Quality = quality
	}
	repairNode.Count = 1
	return repairNode
}

func guardrailStep(kind string, generationRound int, repairRound int, output workflowNodeOutput, err error, decision string) map[string]any {
	step := map[string]any{
		"type":             kind,
		"generation_round": generationRound,
		"repair_round":     repairRound,
		"status":           "completed",
		"updated_at":       now(),
	}
	if decision != "" {
		step["decision"] = decision
	}
	if len(output.Files) > 0 {
		step["files"] = output.Files
	}
	if err != nil {
		step["status"] = "failed"
		step["error"] = err.Error()
	}
	return step
}

func workflowImageGuardrailEnabled(node model.WorkflowTemplateNode) bool {
	guardrail := workflowGuardrailMap(node)
	return mapBool(guardrail, "enabled")
}

func workflowGuardrailPreset(node model.WorkflowTemplateNode) string {
	return workflowGuardrailString(node, "", "preset", "generic_image")
}

func workflowGuardrailFailureStrategy(node model.WorkflowTemplateNode) string {
	return strings.ToLower(workflowGuardrailString(node, "", "failureStrategy", "manual_review"))
}

func workflowGuardrailTransientRetry(node model.WorkflowTemplateNode) workflowRetryConfig {
	source := workflowGuardrailSection(node, "transientRetry")
	enabled := true
	if _, ok := source["enabled"]; ok {
		enabled = mapBool(source, "enabled")
	}
	retryCount := workflowNonNegativeInt(source, "retryCount", -1)
	if retryCount < 0 {
		retryCount = workflowNonNegativeInt(source, "maxAttempts", 0)
	}
	return workflowRetryConfig{
		Enabled:         enabled,
		RetryCount:      retryCount,
		IntervalSeconds: workflowNonNegativeInt(source, "intervalSeconds", 0),
	}
}

func workflowGuardrailMap(node model.WorkflowTemplateNode) map[string]any {
	if node.Extra == nil {
		return nil
	}
	return anyMap(node.Extra["guardrail"])
}

func workflowGuardrailSection(node model.WorkflowTemplateNode, section string) map[string]any {
	guardrail := workflowGuardrailMap(node)
	if section == "" {
		return guardrail
	}
	return anyMap(guardrail[section])
}

func workflowGuardrailString(node model.WorkflowTemplateNode, section string, key string, fallback string) string {
	value := workflowExtraString(workflowGuardrailSection(node, section), key)
	if value == "" {
		return fallback
	}
	return value
}

func workflowGuardrailInt(node model.WorkflowTemplateNode, section string, key string, fallback int) int {
	return workflowExtraInt(workflowGuardrailSection(node, section), key, fallback)
}

func workflowGuardrailBool(node model.WorkflowTemplateNode, section string, key string, fallback bool) bool {
	source := workflowGuardrailSection(node, section)
	if source == nil {
		return fallback
	}
	if _, ok := source[key]; !ok {
		return fallback
	}
	return mapBool(source, key)
}

func requestWorkflowVideo(node model.WorkflowTemplateNode, nodeDir string, prompt string, refs []workflowNodeOutputFile) (workflowNodeOutput, error) {
	body, err := requestWorkflowVideoGeneration(node, prompt, refs)
	if err != nil {
		return workflowNodeOutput{}, err
	}
	outputPath := filepath.Join(nodeDir, "output_01.mp4")
	if err := os.WriteFile(outputPath, body, 0644); err != nil {
		return workflowNodeOutput{}, err
	}
	return workflowNodeOutput{NodeID: node.ID, Type: "video", Files: []workflowNodeOutputFile{{Path: outputPath, Kind: "video", MimeType: "video/mp4"}}}, nil
}

type workflowUpstreamRef struct {
	Edge        model.WorkflowTemplateEdge
	Output      workflowNodeOutput
	SourceIndex int
}

func upstreamNodeOutputRefs(nodeID string, edges []model.WorkflowTemplateEdge, outputs map[string]workflowNodeOutput) []workflowUpstreamRef {
	result := []workflowUpstreamRef{}
	for index, edge := range edges {
		if edge.To != nodeID {
			continue
		}
		if output, ok := outputs[edge.From]; ok {
			result = append(result, workflowUpstreamRef{Edge: edge, Output: output, SourceIndex: index})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := result[i].Edge.InputOrder
		right := result[j].Edge.InputOrder
		if left <= 0 {
			left = result[i].SourceIndex + 1
		}
		if right <= 0 {
			right = result[j].SourceIndex + 1
		}
		if left == right {
			return result[i].SourceIndex < result[j].SourceIndex
		}
		return left < right
	})
	return result
}

func upstreamNodeOutputs(nodeID string, edges []model.WorkflowTemplateEdge, outputs map[string]workflowNodeOutput) []workflowNodeOutput {
	return upstreamOutputsFromRefs(upstreamNodeOutputRefs(nodeID, edges, outputs))
}

func upstreamOutputsFromRefs(refs []workflowUpstreamRef) []workflowNodeOutput {
	result := make([]workflowNodeOutput, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ref.Output)
	}
	return result
}

func upstreamTexts(outputs []workflowNodeOutput) string {
	lines := []string{}
	for _, output := range outputs {
		if strings.TrimSpace(output.Text) != "" {
			lines = append(lines, strings.TrimSpace(output.Text))
		}
	}
	return strings.Join(lines, "\n\n")
}

func upstreamFiles(outputs []workflowNodeOutput, kind string) []workflowNodeOutputFile {
	return upstreamFilesFromRefs(outputsToRefs(outputs), kind)
}

func outputsToRefs(outputs []workflowNodeOutput) []workflowUpstreamRef {
	result := make([]workflowUpstreamRef, 0, len(outputs))
	for index, output := range outputs {
		result = append(result, workflowUpstreamRef{Output: output, SourceIndex: index})
	}
	return result
}

func upstreamFilesFromRefs(refs []workflowUpstreamRef, kind string) []workflowNodeOutputFile {
	files := []workflowNodeOutputFile{}
	for _, ref := range refs {
		selected := selectedWorkflowFiles(ref.Output.Files, ref.Edge.FileSelector)
		for _, file := range selected {
			if file.Kind == kind {
				files = append(files, file)
			}
		}
	}
	return files
}

func selectedWorkflowFiles(files []workflowNodeOutputFile, selector string) []workflowNodeOutputFile {
	selector = strings.ToLower(strings.TrimSpace(selector))
	if selector == "" || selector == "all" || selector == "*" {
		return files
	}
	if len(files) == 0 {
		return nil
	}
	switch selector {
	case "first", "1", "index:1":
		return files[:1]
	case "last":
		return files[len(files)-1:]
	case "none", "skip":
		return nil
	}
	if strings.HasPrefix(selector, "index:") {
		selector = strings.TrimPrefix(selector, "index:")
	}
	if value, err := strconv.Atoi(selector); err == nil && value >= 1 && value <= len(files) {
		return files[value-1 : value]
	}
	return files
}

func applyWorkflowOutputMappings(ctx workflowProductContext, node model.WorkflowTemplateNode, output workflowNodeOutput) error {
	if len(node.OutputMappings) == 0 {
		return nil
	}
	for _, mapping := range node.OutputMappings {
		for index, file := range output.Files {
			relative := renderWorkflowTemplate(mapping.Path, ctx, node, index+1)
			target, err := safeRunOutputPath(ctx.Run.RunDir, relative, file.Path, index+1)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if file.Kind == "text" {
				body, err := os.ReadFile(file.Path)
				if err != nil {
					return err
				}
				if err := os.WriteFile(target, body, 0644); err != nil {
					return err
				}
			} else if err := copyFile(file.Path, target); err != nil {
				return err
			}
			appendCustomWorkflowLog(ctx.Run, "output mapped product=%s node=%s source=%s target=%s", ctx.ProductKey, node.ID, filepath.Base(file.Path), relativeOrAbsolute(ctx.Run.RunDir, target))
		}
	}
	return nil
}

func safeRunOutputPath(runDir string, relative string, sourcePath string, index int) (string, error) {
	relative = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if err := validateOutputTemplatePath(relative); err != nil {
		return "", err
	}
	if index > 1 && !strings.Contains(relative, strconv.Itoa(index)) {
		ext := filepath.Ext(relative)
		relative = strings.TrimSuffix(relative, ext) + fmt.Sprintf("_%02d", index) + ext
	}
	if filepath.Ext(relative) == "" {
		relative += strings.ToLower(filepath.Ext(sourcePath))
	}
	target := filepath.Join(runDir, relative)
	resolvedRun, _ := filepath.Abs(runDir)
	resolvedTarget, _ := filepath.Abs(target)
	if resolvedTarget != resolvedRun && !strings.HasPrefix(resolvedTarget, resolvedRun+string(filepath.Separator)) {
		return "", safeMessageError{message: "输出路径不能逃逸 run 目录"}
	}
	return target, nil
}

var workflowVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func renderWorkflowTemplate(template string, ctx workflowProductContext, node model.WorkflowTemplateNode, index int) string {
	return renderWorkflowTemplateWithRefs(template, ctx, node, index, nil)
}

func renderWorkflowTemplateWithRefs(template string, ctx workflowProductContext, node model.WorkflowTemplateNode, index int, refs []workflowUpstreamRef) string {
	return workflowVarPattern.ReplaceAllStringFunc(template, func(match string) string {
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
		switch key {
		case "runId":
			return ctx.Run.ID
		case "runDir":
			return ctx.Run.RunDir
		case "index", "item.index":
			return fmt.Sprintf("%02d", index)
		case "index1", "item.index1":
			return strconv.Itoa(index)
		case "index4", "item.index4":
			return fmt.Sprintf("%04d", index)
		case "nodeId":
			return node.ID
		case "nodeTitle":
			return node.Title
		case "productTitle":
			return safePathSegment(productTitleFromInput(ctx.Input, ctx.InputIndex))
		case "productTitleRaw":
			return productTitleFromInput(ctx.Input, ctx.InputIndex)
		case "sourceProduct", "sourceProductKey":
			return ctx.ProductKey
		case "sourceTitle", "sourceProductTitle":
			return ctx.ProductDirName
		case "productKey":
			return ctx.ProductKey
		case "input.index":
			return fmt.Sprintf("%03d", ctx.InputIndex)
		case "count":
			return strconv.Itoa(normalizeWorkflowImageCount(node.Count))
		case "uploaded_image_order":
			return workflowUploadedImageOrder(refs)
		case "refs_json", "references_json":
			return workflowRefsJSON(refs)
		}
		if strings.HasPrefix(key, "ref.") {
			return workflowRefValue(key, refs)
		}
		if strings.HasPrefix(key, "input.") {
			return safePathSegment(anyToString(ctx.Input[strings.TrimPrefix(key, "input.")]))
		}
		if value, ok := ctx.Input[key]; ok {
			return safePathSegment(anyToString(value))
		}
		if strings.HasPrefix(key, "node.") && strings.HasSuffix(key, ".text") {
			nodeID := strings.TrimSuffix(strings.TrimPrefix(key, "node."), ".text")
			return ctx.Outputs[nodeID].Text
		}
		if strings.HasPrefix(key, "node.") && strings.HasSuffix(key, ".first_file") {
			nodeID := strings.TrimSuffix(strings.TrimPrefix(key, "node."), ".first_file")
			files := ctx.Outputs[nodeID].Files
			if len(files) > 0 {
				return files[0].Path
			}
			return ""
		}
		if strings.HasPrefix(key, "node.") && strings.HasSuffix(key, ".files_json") {
			nodeID := strings.TrimSuffix(strings.TrimPrefix(key, "node."), ".files_json")
			body, _ := json.Marshal(ctx.Outputs[nodeID].Files)
			return string(body)
		}
		return ""
	})
}

func workflowRefValue(key string, refs []workflowUpstreamRef) string {
	parts := strings.Split(key, ".")
	if len(parts) < 3 {
		return ""
	}
	alias := parts[1]
	field := strings.Join(parts[2:], ".")
	ref, ok := workflowRefByAlias(refs, alias)
	if !ok {
		return ""
	}
	files := selectedWorkflowFiles(ref.Output.Files, ref.Edge.FileSelector)
	switch field {
	case "text":
		return ref.Output.Text
	case "count":
		return strconv.Itoa(len(files))
	case "first_file", "file", "image":
		if len(files) > 0 {
			return files[0].Path
		}
		return ""
	case "last_file":
		if len(files) > 0 {
			return files[len(files)-1].Path
		}
		return ""
	case "files_json", "images_json":
		return workflowFilesJSON(files)
	}
	if strings.HasPrefix(field, "file_") || strings.HasPrefix(field, "image_") {
		raw := strings.TrimPrefix(strings.TrimPrefix(field, "file_"), "image_")
		if idx, err := strconv.Atoi(raw); err == nil && idx >= 1 && idx <= len(files) {
			return files[idx-1].Path
		}
	}
	if strings.HasPrefix(field, "file") || strings.HasPrefix(field, "image") {
		raw := strings.TrimPrefix(strings.TrimPrefix(field, "file"), "image")
		if idx, err := strconv.Atoi(raw); err == nil && idx >= 1 && idx <= len(files) {
			return files[idx-1].Path
		}
	}
	return ""
}

func workflowRefByAlias(refs []workflowUpstreamRef, alias string) (workflowUpstreamRef, bool) {
	alias = workflowSafeAlias(alias)
	for _, ref := range refs {
		candidates := []string{
			ref.Edge.InputAlias,
			ref.Edge.From,
			ref.Output.NodeID,
		}
		for _, candidate := range candidates {
			if workflowSafeAlias(candidate) == alias {
				return ref, true
			}
		}
	}
	return workflowUpstreamRef{}, false
}

func workflowRefAlias(ref workflowUpstreamRef) string {
	return workflowSafeAlias(firstString(ref.Edge.InputAlias, ref.Edge.From, ref.Output.NodeID, fmt.Sprintf("ref_%d", ref.SourceIndex+1)))
}

func workflowSafeAlias(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		case r == ' ' || r == '.' || r == '/':
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func workflowUploadedImageOrder(refs []workflowUpstreamRef) string {
	lines := []string{}
	imageIndex := 1
	for _, ref := range refs {
		alias := workflowRefAlias(ref)
		for _, file := range selectedWorkflowFiles(ref.Output.Files, ref.Edge.FileSelector) {
			if file.Kind != "image" {
				continue
			}
			lines = append(lines, fmt.Sprintf("Uploaded image %d: alias=%s, source_node=%s, file=%s", imageIndex, alias, firstString(ref.Edge.From, ref.Output.NodeID), filepath.Base(file.Path)))
			imageIndex++
		}
	}
	return strings.Join(lines, "\n")
}

func workflowRefsJSON(refs []workflowUpstreamRef) string {
	items := []map[string]any{}
	for _, ref := range refs {
		items = append(items, map[string]any{
			"alias":        workflowRefAlias(ref),
			"source_node":  firstString(ref.Edge.From, ref.Output.NodeID),
			"input_order":  ref.Edge.InputOrder,
			"selector":     ref.Edge.FileSelector,
			"text":         ref.Output.Text,
			"files":        selectedWorkflowFiles(ref.Output.Files, ref.Edge.FileSelector),
			"output_type":  ref.Output.Type,
			"source_index": ref.SourceIndex,
		})
	}
	body, _ := json.Marshal(items)
	return string(body)
}

func workflowFilesJSON(files []workflowNodeOutputFile) string {
	body, _ := json.Marshal(files)
	return string(body)
}

func workflowNodeTitleProvider(node model.WorkflowTemplateNode) bool {
	if node.Operation != "text_generation" {
		return false
	}
	return mapBool(node.Extra, "titleProvider") || strings.EqualFold(workflowExtraString(node.Extra, "titleProvider"), "true")
}

func workflowTitleFromOutput(output workflowNodeOutput) string {
	text := strings.TrimSpace(output.Text)
	if text == "" {
		return ""
	}
	if parsed, err := parseWorkflowJSON(text); err == nil {
		if record, ok := parsed.(map[string]any); ok {
			for _, key := range []string{"title", "productTitle", "product_title", "name"} {
				if value := strings.TrimSpace(anyToString(record[key])); value != "" {
					return value
				}
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'`，,。 ")
		if line != "" {
			return line
		}
	}
	return ""
}

func requestWorkflowText(modelName string, prompt string, refs []workflowNodeOutputFile) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		modelName = "gpt-5.5"
	}
	content := any(prompt)
	if len(refs) > 0 {
		parts := []map[string]any{{"type": "text", "text": prompt}}
		for _, ref := range refs {
			dataURL, err := fileDataURL(ref.Path, ref.MimeType)
			if err != nil {
				return "", err
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}})
		}
		content = parts
	}
	payload := map[string]any{
		"model": modelName,
		"messages": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
	}
	body, err := postWorkflowJSON(modelName, "/chat/completions", payload)
	if err != nil {
		return "", err
	}
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
		return "", err
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", errors.New("文本模型没有返回内容")
	}
	return parsed.Choices[0].Message.Content, nil
}

func requestWorkflowImageGeneration(node model.WorkflowTemplateNode, prompt string, count int) ([][]byte, error) {
	modelName := strings.TrimSpace(node.Model)
	if modelName == "" {
		modelName = "gpt-image-2"
	}
	prompt = withWorkflowImagePromptInjection(prompt)
	payload := map[string]any{
		"model":           modelName,
		"prompt":          prompt,
		"n":               count,
		"response_format": "b64_json",
	}
	quality := normalizeWorkflowImageQuality(node.Quality)
	requestSize := resolveWorkflowImageRequestSize(quality, node.Size)
	if quality != "" {
		payload["quality"] = quality
	}
	if requestSize != "" {
		payload["size"] = requestSize
	}
	if generationConfig := workflowFlow2APIImageGenerationConfig(node, requestSize, quality); len(generationConfig) > 0 {
		payload["generation_config"] = generationConfig
	}
	body, err := postWorkflowJSON(modelName, "/images/generations", payload)
	if err != nil {
		return nil, err
	}
	return parseWorkflowImages(body)
}

func requestWorkflowImageEdit(node model.WorkflowTemplateNode, prompt string, refs []workflowNodeOutputFile, count int) ([][]byte, error) {
	modelName := strings.TrimSpace(node.Model)
	if modelName == "" {
		modelName = "gpt-image-2"
	}
	prompt = withWorkflowImagePromptInjection(prompt)
	fields := map[string]string{
		"model":           modelName,
		"prompt":          prompt,
		"n":               strconv.Itoa(count),
		"response_format": "b64_json",
	}
	quality := normalizeWorkflowImageQuality(node.Quality)
	requestSize := resolveWorkflowImageRequestSize(quality, node.Size)
	if quality != "" {
		fields["quality"] = quality
	}
	if requestSize != "" {
		fields["size"] = requestSize
	}
	if generationConfig := workflowFlow2APIImageGenerationConfig(node, requestSize, quality); len(generationConfig) > 0 {
		body, _ := json.Marshal(generationConfig)
		fields["generation_config"] = string(body)
	}
	body, err := postWorkflowMultipart(modelName, "/images/edits", fields, refs)
	if err != nil {
		return nil, err
	}
	return parseWorkflowImages(body)
}

func requestWorkflowImageEditWithMask(node model.WorkflowTemplateNode, prompt string, refs []workflowNodeOutputFile, mask workflowNodeOutputFile, count int) ([][]byte, error) {
	modelName := strings.TrimSpace(node.Model)
	if modelName == "" {
		modelName = "gpt-image-2"
	}
	channel, err := SelectModelChannel(modelName)
	if err != nil {
		return nil, err
	}
	if IsFlow2APIChannel(channel) {
		return nil, safeMessageError{message: "Flow2API 图片编辑暂不支持局部蒙版；请改用 gpt-image-2 或关闭蒙版"}
	}
	prompt = withWorkflowImagePromptInjection(prompt)
	fields := map[string]string{
		"model":           modelName,
		"prompt":          prompt,
		"n":               strconv.Itoa(count),
		"response_format": "b64_json",
	}
	quality := normalizeWorkflowImageQuality(node.Quality)
	requestSize := resolveWorkflowImageRequestSize(quality, node.Size)
	if quality != "" {
		fields["quality"] = quality
	}
	if requestSize != "" {
		fields["size"] = requestSize
	}
	body, err := postWorkflowMultipartWithMask(channel, "/images/edits", fields, refs, mask)
	if err != nil {
		return nil, err
	}
	return parseWorkflowImages(body)
}

func workflowFlow2APIImageGenerationConfig(node model.WorkflowTemplateNode, requestSize string, quality string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(requestSize) != "" {
		result["size"] = requestSize
	}
	if strings.TrimSpace(quality) != "" {
		result["quality"] = quality
	}
	if node.Size != "" {
		result["aspectRatio"] = node.Size
	}
	if node.Quality != "" {
		result["qualityLabel"] = node.Quality
	}
	for _, key := range []string{"aspectRatio", "outputFormat", "background", "style", "seed"} {
		if value := workflowExtraString(node.Extra, key); value != "" {
			result[key] = value
		}
	}
	return result
}

func withWorkflowImagePromptInjection(prompt string) string {
	settings, err := repository.GetSettings()
	if err != nil {
		return prompt
	}
	prefix := strings.TrimSpace(normalizePublicSetting(settings.Public).ModelChannel.PromptInjection.Image)
	if prefix == "" || strings.HasPrefix(strings.TrimSpace(prompt), prefix) {
		return prompt
	}
	return strings.TrimSpace(prefix + "\n\n" + strings.TrimSpace(prompt))
}

func normalizeWorkflowImageQuality(quality string) string {
	value := strings.ToLower(strings.TrimSpace(quality))
	if alias := workflowImageQualityAliases[value]; alias != "" {
		value = alias
	}
	if _, ok := workflowImageQualityBase[value]; ok {
		return value
	}
	return ""
}

func resolveWorkflowImageRequestSize(quality string, size string) string {
	value := strings.TrimSpace(size)
	if value == "" || value == "auto" {
		return ""
	}
	if workflowPixelSizePattern.MatchString(value) {
		return value
	}
	if quality != "" {
		if resolved := resolveWorkflowImageRatioSize(quality, value); resolved != "" {
			return resolved
		}
	}
	return value
}

var workflowPixelSizePattern = regexp.MustCompile(`^\d+x\d+$`)

func resolveWorkflowImageRatioSize(quality string, ratio string) string {
	basePixels, ok := workflowImageQualityBase[quality]
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

func requestWorkflowVideoGeneration(node model.WorkflowTemplateNode, prompt string, refs []workflowNodeOutputFile) ([]byte, error) {
	modelName := strings.TrimSpace(node.Model)
	if modelName == "" {
		modelName = "sora-2"
	}
	channel, err := SelectModelChannel(modelName)
	if err != nil {
		return nil, err
	}
	if IsFlow2APIChannel(channel) {
		modelRefs, err := workflowModelReferences(refs)
		if err != nil {
			return nil, err
		}
		return Flow2APIVideoGeneration(channel, modelName, prompt, modelRefs, workflowFlow2APIVideoOptions(node))
	}
	fields := map[string]string{
		"model":   modelName,
		"prompt":  prompt,
		"seconds": firstString(node.Seconds, "6"),
	}
	if node.Size != "" {
		fields["size"] = normalizeWorkflowVideoSize(node.Size)
	}
	if node.VideoQuality != "" {
		fields["vquality"] = node.VideoQuality
	}
	ref := []workflowNodeOutputFile{}
	if len(refs) > 0 {
		ref = refs[:1]
	}
	body, err := postWorkflowMultipart(modelName, "/videos", fields, ref)
	if err != nil {
		return nil, err
	}
	var created struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return nil, err
	}
	if created.Error != nil && created.Error.Message != "" {
		return nil, errors.New(created.Error.Message)
	}
	if created.ID == "" {
		return nil, errors.New("视频模型没有返回任务 ID")
	}
	for attempt := 0; attempt < 240; attempt++ {
		statusBody, err := getWorkflowModel(modelName, "/videos/"+url.PathEscape(created.ID))
		if err != nil {
			return nil, err
		}
		var status struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(statusBody, &status)
		if status.Status == "completed" {
			return getWorkflowModel(modelName, "/videos/"+url.PathEscape(created.ID)+"/content")
		}
		if status.Status == "failed" || status.Status == "cancelled" {
			if status.Error != nil && status.Error.Message != "" {
				return nil, errors.New(status.Error.Message)
			}
			return nil, errors.New("视频生成失败")
		}
		time.Sleep(2500 * time.Millisecond)
	}
	return nil, errors.New("视频生成超时")
}

func postWorkflowJSON(modelName string, path string, payload any) ([]byte, error) {
	body, _ := json.Marshal(payload)
	channel, err := SelectModelChannel(modelName)
	if err != nil {
		return nil, err
	}
	if IsFlow2APIChannel(channel) && path == "/images/generations" {
		prompt, count, options := flow2APIImagePayload(payload)
		images, err := Flow2APIImageGenerationWithOptions(channel, modelName, prompt, count, options)
		if err != nil {
			return nil, err
		}
		return workflowImagesJSON(images)
	}
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	return doWorkflowModelRequest(request)
}

func postWorkflowMultipart(modelName string, path string, fields map[string]string, refs []workflowNodeOutputFile) ([]byte, error) {
	channel, err := SelectModelChannel(modelName)
	if err != nil {
		return nil, err
	}
	if IsFlow2APIChannel(channel) && path == "/images/edits" {
		modelRefs, err := workflowModelReferences(refs)
		if err != nil {
			return nil, err
		}
		count, _ := strconv.Atoi(fields["n"])
		generationConfig := map[string]any{}
		if raw := strings.TrimSpace(fields["generation_config"]); raw != "" {
			_ = json.Unmarshal([]byte(raw), &generationConfig)
		}
		images, err := Flow2APIImageEditWithOptions(channel, modelName, fields["prompt"], modelRefs, count, Flow2APIImageOptions{
			Quality:          fields["quality"],
			Size:             fields["size"],
			GenerationConfig: generationConfig,
		})
		if err != nil {
			return nil, err
		}
		return workflowImagesJSON(images)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	for index, ref := range refs {
		fieldName := "image"
		if path == "/videos" {
			fieldName = "input_reference"
		}
		file, err := os.Open(ref.Path)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		part, err := writer.CreateFormFile(fieldName, fmt.Sprintf("reference_%02d%s", index+1, filepath.Ext(ref.Path)))
		if err != nil {
			_ = file.Close()
			_ = writer.Close()
			return nil, err
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = file.Close()
			_ = writer.Close()
			return nil, err
		}
		_ = file.Close()
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, path), &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return doWorkflowModelRequest(request)
}

func postWorkflowMultipartWithMask(channel model.ModelChannel, path string, fields map[string]string, refs []workflowNodeOutputFile, mask workflowNodeOutputFile) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	for index, ref := range refs {
		if err := writeWorkflowMultipartFile(writer, "image", fmt.Sprintf("reference_%02d%s", index+1, filepath.Ext(ref.Path)), ref.Path); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if strings.TrimSpace(mask.Path) != "" {
		if err := writeWorkflowMultipartFile(writer, "mask", fmt.Sprintf("mask%s", filepath.Ext(mask.Path)), mask.Path); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, path), &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return doWorkflowModelRequest(request)
}

func writeWorkflowMultipartFile(writer *multipart.Writer, fieldName string, filename string, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func flow2APIImagePayload(payload any) (string, int, Flow2APIImageOptions) {
	body, _ := json.Marshal(payload)
	var parsed struct {
		Prompt           string         `json:"prompt"`
		N                int            `json:"n"`
		Quality          string         `json:"quality"`
		Size             string         `json:"size"`
		GenerationConfig map[string]any `json:"generation_config"`
	}
	_ = json.Unmarshal(body, &parsed)
	if parsed.N < 1 {
		parsed.N = 1
	}
	return parsed.Prompt, parsed.N, Flow2APIImageOptions{
		Quality:          parsed.Quality,
		Size:             parsed.Size,
		GenerationConfig: parsed.GenerationConfig,
	}
}

func workflowModelReferences(refs []workflowNodeOutputFile) ([]ModelReference, error) {
	result := make([]ModelReference, 0, len(refs))
	for _, ref := range refs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			return nil, err
		}
		result = append(result, ModelReference{
			Name:     filepath.Base(ref.Path),
			MimeType: firstString(ref.MimeType, mime.TypeByExtension(filepath.Ext(ref.Path)), "image/png"),
			Data:     data,
		})
	}
	return result, nil
}

func workflowImagesJSON(images [][]byte) ([]byte, error) {
	items := make([]map[string]string, 0, len(images))
	for _, image := range images {
		items = append(items, map[string]string{"b64_json": base64.StdEncoding.EncodeToString(image)})
	}
	return json.Marshal(map[string]any{"data": items})
}

func getWorkflowModel(modelName string, path string) ([]byte, error) {
	channel, err := SelectModelChannel(modelName)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodGet, BuildModelChannelURL(channel, path), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	return doWorkflowModelRequest(request)
}

func doWorkflowModelRequest(request *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 180 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("model request failed: status=%d body=%s", response.StatusCode, string(body[:minInt(len(body), 500)]))
	}
	return body, nil
}

func workflowFlow2APIVideoOptions(node model.WorkflowTemplateNode) Flow2APIVideoOptions {
	return Flow2APIVideoOptions{
		ReferenceMode: workflowExtraString(node.Extra, "videoReferenceMode"),
		Seconds:       firstString(node.Seconds, "6"),
		Size:          normalizeWorkflowVideoSize(node.Size),
		Resolution:    firstString(node.VideoQuality, "720"),
		GenerationConfig: map[string]any{
			"referenceMode": workflowExtraString(node.Extra, "videoReferenceMode"),
			"duration":      firstString(node.Seconds, "6"),
			"resolution":    firstString(node.VideoQuality, "720"),
			"size":          normalizeWorkflowVideoSize(node.Size),
		},
	}
}

func parseWorkflowImages(body []byte) ([][]byte, error) {
	var payload struct {
		Data  []map[string]any `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Error != nil && payload.Error.Message != "" {
		return nil, errors.New(payload.Error.Message)
	}
	images := [][]byte{}
	for _, item := range payload.Data {
		if value, ok := item["b64_json"].(string); ok && value != "" {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				return nil, err
			}
			images = append(images, decoded)
			continue
		}
		if value, ok := item["url"].(string); ok && value != "" {
			decoded, err := imageURLBytes(value)
			if err != nil {
				return nil, err
			}
			images = append(images, decoded)
		}
	}
	if len(images) == 0 {
		return nil, errors.New("图片模型没有返回图片")
	}
	return images, nil
}

func imageURLBytes(value string) ([]byte, error) {
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma < 0 {
			return nil, errors.New("invalid data url")
		}
		return base64.StdEncoding.DecodeString(value[comma+1:])
	}
	response, err := http.Get(value)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("download image failed: status=%d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func findPDDMaterialForInput(input map[string]any) (model.Asset, string, error) {
	assets, err := allPDDMaterialAssets()
	if err != nil {
		return model.Asset{}, "", err
	}
	character := normalizeLookupText(firstString(anyToString(input["character"]), anyToString(input["role"]), anyToString(input["name"])))
	theme := normalizeLookupText(firstString(anyToString(input["theme"]), anyToString(input["ip"]), anyToString(input["work"])))
	bestScore := -1
	var best model.Asset
	for _, asset := range assets {
		score := 0
		haystack := normalizeLookupText(asset.Title + " " + strings.Join(asset.Tags, " ") + " " + asset.Description)
		if character != "" && strings.Contains(haystack, character) {
			score += 100
		}
		if theme != "" && strings.Contains(haystack, theme) {
			score += 30
		}
		if asset.Purpose == assetPurposeStandardReference || containsString(asset.Tags, "标准参考图") {
			score += 20
		} else if asset.Purpose == assetPurposeOfficialReference || containsString(asset.Tags, "官方参考图") {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			best = asset
		}
	}
	if bestScore < 100 {
		return model.Asset{}, "", safeMessageError{message: "素材库未匹配到输入角色：" + firstString(anyToString(input["character"]), anyToString(input["theme"]))}
	}
	materialPath, err := assetMaterialPath(best)
	return best, materialPath, err
}

func allPDDMaterialAssets() ([]model.Asset, error) {
	ensureTaxonomyNormalized()
	items := []model.Asset{}
	for _, purpose := range []string{assetPurposeStandardReference, assetPurposeOfficialReference} {
		for page := 1; ; page++ {
			batch, _, err := repository.ListAssets(model.Query{Type: string(model.AssetTypeImage), Purpose: purpose, Page: page, PageSize: model.MaxPageSize})
			if err != nil {
				return nil, err
			}
			items = append(items, batch...)
			if len(batch) < model.MaxPageSize {
				break
			}
		}
	}
	return items, nil
}

func assetMaterialPath(asset model.Asset) (string, error) {
	parsed, err := url.Parse(asset.URL)
	if err != nil {
		return "", err
	}
	return ResolvePDDMaterialFile(parsed.Query().Get("path"))
}

func writePipelineStatus(ctx workflowProductContext, status string, startedAt string, finishedAt string, message string) error {
	productTitle := productTitleFromInput(ctx.Input, ctx.InputIndex)
	productDirName := safeFileName(productTitle, ctx.ProductKey)
	payload := map[string]any{
		"source_product":    ctx.ProductKey,
		"generated_product": ctx.ProductKey,
		"product":           productDirName,
		"theme_name":        productTitle,
		"status":            status,
		"started_at":        startedAt,
		"updated_at":        now(),
		"custom_workflow":   true,
	}
	if finishedAt != "" {
		payload["finished_at"] = finishedAt
	}
	if message != "" {
		payload["error"] = message
	}
	return writeJSONFile(filepath.Join(ctx.ProductLogDir, "pipeline_status.json"), payload)
}

func writeCustomProductSummary(run model.WorkflowRun) error {
	pipelineRoot := filepath.Join(run.RunDir, "logs", "product_pipeline")
	products := []map[string]any{}
	_ = filepath.WalkDir(pipelineRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "pipeline_status.json" {
			return nil
		}
		var payload map[string]any
		if body, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(body, &payload)
		}
		if payload != nil {
			products = append(products, payload)
		}
		return nil
	})
	sort.Slice(products, func(i, j int) bool {
		return anyToString(products[i]["source_product"]) < anyToString(products[j]["source_product"])
	})
	return writeJSONFile(filepath.Join(run.RunDir, "logs", "product_pipeline_summary.json"), map[string]any{
		"run_id":     run.ID,
		"status":     string(run.Status),
		"updated_at": now(),
		"products":   products,
	})
}

func writeCustomManifest(run model.WorkflowRun, completed bool, message string) error {
	payload := map[string]any{
		"run_id":          run.ID,
		"run_dir":         run.RunDir,
		"generated_dir":   filepath.Join(run.RunDir, "generated"),
		"output_dir":      filepath.Join(run.RunDir, "待上架"),
		"completed":       completed,
		"custom_workflow": true,
		"updated_at":      now(),
	}
	if message != "" {
		payload["error"] = message
	}
	return writeJSONFile(filepath.Join(run.RunDir, "manifest.json"), payload)
}

func writeCustomWorkflowStatus(run model.WorkflowRun, status string, stages []workflowRunStage, message string) error {
	payload := map[string]any{
		"run_id":     run.ID,
		"status":     status,
		"updated_at": now(),
		"stages":     stages,
	}
	if message != "" {
		payload["error"] = message
	}
	if err := os.MkdirAll(filepath.Join(run.RunDir, "logs"), 0755); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(run.RunDir, "logs", "workflow_status.json"), payload)
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0644)
}

func fileDataURL(path string, mimeType string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(path))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(body), nil
}

func copyFile(source string, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func isRetryableWorkflowError(err error) bool {
	text := strings.ToLower(err.Error())
	for _, token := range []string{"403", "408", "409", "429", "502", "503", "504", "bad gateway", "timeout", "cloudflare", "connection reset", "temporar", "throttled", "no text", "no image", "没有返回内容", "没有返回图片", "files failed"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func productTitleFromInput(input map[string]any, index int) string {
	for _, key := range []string{"productTitle", "product_title", "title", "name"} {
		if value := strings.TrimSpace(anyToString(input[key])); value != "" {
			return value
		}
	}
	parts := []string{}
	for _, key := range []string{"theme", "character"} {
		if value := strings.TrimSpace(anyToString(input[key])); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " - ")
	}
	return fmt.Sprintf("商品%03d", index)
}

func anyToString(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case fmt.Stringer:
		return item.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(item)
	}
}

func normalizeLookupText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "《》「」[]()（） ")
	value = strings.ReplaceAll(value, "《", "")
	value = strings.ReplaceAll(value, "》", "")
	return strings.ToLower(value)
}

func safeFileName(value string, fallback string) string {
	value = safePathSegment(value)
	if value == "" {
		return fallback
	}
	return value
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", "\n", " ", "\r", " ").Replace(value)
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 80 {
		runes := []rune(value)
		value = string(runes[:80])
	}
	return value
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func normalizeWorkflowVideoSize(value string) string {
	if regexp.MustCompile(`^\d+x\d+$`).MatchString(value) {
		return value
	}
	if value == "9:16" || value == "2:3" || value == "3:4" {
		return "720x1280"
	}
	return "1280x720"
}

func workflowEdgeKey(edge model.WorkflowTemplateEdge) string {
	if strings.TrimSpace(edge.ID) != "" {
		return edge.ID
	}
	return edge.From + "->" + edge.To
}

func workflowEdgeLoopEnabled(edge model.WorkflowTemplateEdge) bool {
	return mapBool(edge.Loop, "enabled") || mapBool(edge.Loop, "loop")
}

func workflowEdgeLoopMax(edge model.WorkflowTemplateEdge) int {
	value := mapInt(edge.Loop, "maxIterations", 0)
	if value <= 0 {
		value = mapInt(edge.Loop, "max", 0)
	}
	return value
}

func workflowEdgeShouldFollow(edge model.WorkflowTemplateEdge, output workflowNodeOutput) bool {
	payload, _ := parseWorkflowJSON(output.Text)
	if handle := strings.TrimSpace(edge.FromHandle); handle != "" {
		return anyToString(jsonPathValue(payload, "$.decision")) == handle || strings.TrimSpace(output.Text) == handle
	}
	if len(edge.Condition) == 0 {
		return true
	}
	return workflowRuleMatches(payload, edge.Condition)
}

func workflowEdgeHasGate(edge model.WorkflowTemplateEdge) bool {
	return strings.TrimSpace(edge.FromHandle) != "" || len(edge.Condition) > 0
}

func workflowConditionRules(extra map[string]any) []map[string]any {
	value := extra["conditions"]
	if value == nil {
		value = extra["rules"]
	}
	if text, ok := value.(string); ok {
		var parsed []map[string]any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			return parsed
		}
	}
	result := []map[string]any{}
	for _, item := range anySlice(value) {
		if record, ok := item.(map[string]any); ok {
			result = append(result, record)
		}
	}
	return result
}

func workflowRuleMatches(payload any, rule map[string]any) bool {
	path := firstString(anyToString(rule["jsonPath"]), anyToString(rule["path"]), anyToString(rule["expression"]))
	actual := jsonPathValue(payload, path)
	operator := strings.ToLower(firstString(anyToString(rule["operator"]), anyToString(rule["op"]), "eq"))
	expected := rule["value"]
	if expected == nil {
		expected = rule["equals"]
	}
	switch operator {
	case "exists":
		return actual != nil
	case "truthy":
		return truthyAny(actual)
	case "neq", "not_eq", "!=":
		return anyToString(actual) != anyToString(expected)
	case "in":
		for _, value := range ruleValues(rule) {
			if anyToString(actual) == anyToString(value) {
				return true
			}
		}
		return false
	case "contains":
		return strings.Contains(anyToString(actual), anyToString(expected))
	default:
		return anyToString(actual) == anyToString(expected)
	}
}

func ruleValues(rule map[string]any) []any {
	if items := anySlice(rule["values"]); len(items) > 0 {
		return items
	}
	if text := anyToString(rule["values"]); text != "" {
		parts := strings.Split(text, ",")
		values := make([]any, 0, len(parts))
		for _, part := range parts {
			if value := strings.TrimSpace(part); value != "" {
				values = append(values, value)
			}
		}
		return values
	}
	return []any{rule["value"]}
}

func truthyAny(value any) bool {
	switch item := value.(type) {
	case bool:
		return item
	case string:
		return strings.TrimSpace(item) != "" && strings.TrimSpace(item) != "false" && strings.TrimSpace(item) != "0"
	case float64:
		return item != 0
	case int:
		return item != 0
	default:
		return value != nil
	}
}

func jsonPathValue(payload any, path string) any {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return payload
	}
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, ".")
	current := payload
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		record, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = record[part]
	}
	return current
}

func normalizeJSONText(text string) (string, error) {
	payload, err := parseWorkflowJSON(text)
	if err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseWorkflowJSON(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return map[string]any{}, errors.New("JSON 内容为空")
	}
	candidates := []string{text}
	if start := strings.IndexAny(text, "{["); start >= 0 {
		for end := len(text); end > start; end-- {
			part := strings.TrimSpace(text[start:end])
			if part != "" {
				candidates = append(candidates, part)
			}
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		var payload any
		if err := json.Unmarshal([]byte(candidate), &payload); err != nil {
			lastErr = err
			continue
		}
		return payload, nil
	}
	return nil, safeMessageError{message: "模型没有返回可解析 JSON：" + firstString(lastErrString(lastErr), truncateText(text, 120))}
}

func lastErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func workflowExtraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	return strings.TrimSpace(anyToString(extra[key]))
}

func workflowExtraInt(extra map[string]any, key string, fallback int) int {
	value := mapInt(extra, key, fallback)
	if value <= 0 {
		return fallback
	}
	return value
}

func workflowNonNegativeInt(source map[string]any, key string, fallback int) int {
	if source == nil {
		return fallback
	}
	switch value := source[key].(type) {
	case int:
		if value >= 0 {
			return value
		}
	case int64:
		if value >= 0 {
			return int(value)
		}
	case float64:
		if value >= 0 {
			return int(value)
		}
	case json.Number:
		parsed, err := strconv.Atoi(value.String())
		if err == nil && parsed >= 0 {
			return parsed
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && parsed >= 0 {
			return parsed
		}
	}
	return fallback
}

func mapBool(source map[string]any, key string) bool {
	if source == nil {
		return false
	}
	switch value := source[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true") || strings.TrimSpace(value) == "1"
	default:
		return false
	}
}

func anyMap(value any) map[string]any {
	if record, ok := value.(map[string]any); ok {
		return record
	}
	return nil
}

func mapInt(source map[string]any, key string, fallback int) int {
	if source == nil {
		return fallback
	}
	return positivePayloadInt(source[key], fallback)
}

func renderScriptArgs(node model.WorkflowTemplateNode, ctx workflowProductContext) []string {
	raw := workflowExtraString(node.Extra, "args")
	if raw == "" {
		raw = workflowExtraString(node.Extra, "arguments")
	}
	if raw == "" {
		return nil
	}
	var parsed []string
	if json.Unmarshal([]byte(raw), &parsed) == nil {
		for i := range parsed {
			parsed[i] = renderWorkflowTemplate(parsed[i], ctx, node, 1)
		}
		return parsed
	}
	args := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			args = append(args, renderWorkflowTemplate(line, ctx, node, 1))
		}
	}
	return args
}

func runVPSScript(scriptPath string, args []string, timeout time.Duration) (string, error) {
	root := pddWorkflowRoot()
	target, err := safeRepositoryScriptPath(root, scriptPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(filepath.ToSlash(scriptPath)) == "scripts/trigger_local_receive_and_upload.sh" {
		output, err := runHostShell("timeout 5 bash -c ': < /dev/tcp/127.0.0.1/22222' 2>/dev/null || { echo '本地同步通道不可用：VPS 无法连接 127.0.0.1:22222，请先建立反向 SSH 后再运行同步节点。'; exit 1; }", 8*time.Second)
		if err != nil {
			return output, safeMessageError{message: strings.TrimSpace(output)}
		}
	}
	commandArgs := []string{}
	switch strings.ToLower(filepath.Ext(target)) {
	case ".py":
		commandArgs = append(commandArgs, config.Cfg.PDDPython, target)
	case ".sh":
		commandArgs = append(commandArgs, "bash", target)
	default:
		commandArgs = append(commandArgs, target)
	}
	commandArgs = append(commandArgs, args...)
	quoted := make([]string, 0, len(commandArgs))
	for _, item := range commandArgs {
		quoted = append(quoted, shellQuote(item))
	}
	script := "cd " + shellQuote(root) + "\n" + strings.Join(quoted, " ")
	return runHostShell(script, timeout)
}

func scriptNodeError(err error, output string) error {
	tail := strings.TrimSpace(tailLines(output, 8))
	if tail == "" {
		return err
	}
	return safeMessageError{message: err.Error() + ": " + tail}
}

func tailLines(text string, limit int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	result := []string{}
	for index := len(lines) - 1; index >= 0 && len(result) < limit; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return strings.Join(result, "\n")
}

func safeRepositoryScriptPath(root string, scriptPath string) (string, error) {
	scriptPath = filepath.Clean(filepath.FromSlash(strings.TrimSpace(scriptPath)))
	if scriptPath == "." || filepath.IsAbs(scriptPath) || strings.HasPrefix(scriptPath, "..") || strings.Contains(scriptPath, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", safeMessageError{message: "脚本路径必须是仓库内相对路径"}
	}
	target := filepath.Join(root, scriptPath)
	resolvedRoot, _ := filepath.Abs(root)
	resolvedTarget, _ := filepath.Abs(target)
	if resolvedTarget != resolvedRoot && !strings.HasPrefix(resolvedTarget, resolvedRoot+string(filepath.Separator)) {
		return "", safeMessageError{message: "脚本路径不能逃逸仓库目录"}
	}
	if info, err := os.Stat(resolvedTarget); err != nil {
		return "", err
	} else if info.IsDir() {
		return "", safeMessageError{message: "脚本路径不能是目录"}
	}
	return resolvedTarget, nil
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
