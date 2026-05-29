package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
)

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type pddRunData struct {
	runDir         string
	manifest       map[string]any
	status         map[string]any
	summary        map[string]any
	pipeline       map[string]any
	customWorkflow bool
	customSpec     model.WorkflowTemplateSpec
	products       []model.PDDProductSummary
}

func ListPDDRuns() (model.PDDRunList, error) {
	root := pddRunsRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.PDDRunList{Root: root, Items: []model.PDDRunItem{}}, nil
		}
		return model.PDDRunList{}, err
	}
	items := make([]model.PDDRunItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !runIDPattern.MatchString(entry.Name()) {
			continue
		}
		data, err := loadPDDRun(entry.Name())
		if err != nil {
			continue
		}
		items = append(items, data.runItem())
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].RunID > items[j].RunID
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return model.PDDRunList{Root: root, Items: items}, nil
}

func PDDRunOverview(runID string) (model.PDDRunOverview, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return model.PDDRunOverview{}, err
	}
	stages := data.stageNodes()
	return model.PDDRunOverview{
		Run:          data.runItem(),
		Stages:       stages,
		Edges:        data.stageEdges(stages),
		Products:     data.products,
		RecentErrors: data.recentErrors(8),
	}, nil
}

func ListPDDProducts(runID string, q model.Query) ([]model.PDDProductSummary, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return nil, err
	}
	keyword := strings.ToLower(strings.TrimSpace(q.Keyword))
	statusFilter := strings.TrimSpace(q.Type)
	items := make([]model.PDDProductSummary, 0, len(data.products))
	for _, item := range data.products {
		if statusFilter != "" && string(item.Status) != statusFilter && item.RawStatus != statusFilter {
			continue
		}
		haystack := strings.ToLower(item.SourceProduct + " " + item.Product + " " + item.ThemeName)
		if keyword != "" && !strings.Contains(haystack, keyword) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func PDDProductDetail(runID string, productKey string) (model.PDDProductDetail, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return model.PDDProductDetail{}, err
	}
	productKey, _ = url.PathUnescape(productKey)
	product, ok := data.findProduct(productKey)
	if !ok {
		return model.PDDProductDetail{}, safeMessageError{message: "未找到商品"}
	}
	nodes, edges, files := data.productGraph(product)
	return model.PDDProductDetail{
		RunID:   runID,
		Product: product,
		Nodes:   nodes,
		Edges:   edges,
		Files:   files,
	}, nil
}

func PDDCreativeCanvas(runID string, productKey string) (model.PDDCreativeCanvas, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return model.PDDCreativeCanvas{}, err
	}
	productKey, _ = url.PathUnescape(productKey)
	product, ok := data.findProduct(productKey)
	if !ok {
		return model.PDDCreativeCanvas{}, safeMessageError{message: "未找到商品"}
	}
	graphNodes, graphEdges, _ := data.productGraph(product)
	liveCanvas := data.initialCreativeCanvas(product, graphNodes, graphEdges)
	path := data.creativeCanvasPath(product)
	if exists(path) && !isDir(path) {
		var canvas model.PDDCreativeCanvas
		if body, err := os.ReadFile(path); err == nil && json.Unmarshal(body, &canvas) == nil {
			canvas = mergeCreativeCanvasLiveState(canvas, liveCanvas)
			canvas.RunID = runID
			canvas.ProductKey = product.Key
			canvas.Product = product
			canvas.Saved = true
			canvas.UpdatedAt = latestTimeString(canvas.UpdatedAt, liveCanvas.UpdatedAt, stringValue(data.status, "updated_at"), product.FinishedAt, product.StartedAt, fileModTime(path))
			return canvas, nil
		}
	}
	canvas := liveCanvas
	_ = writeJSONFile(path, canvas)
	canvas.Saved = true
	return canvas, nil
}

func SavePDDCreativeCanvas(runID string, productKey string, request model.PDDCreativeCanvasSaveRequest) (model.PDDCreativeCanvas, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return model.PDDCreativeCanvas{}, err
	}
	productKey, _ = url.PathUnescape(productKey)
	product, ok := data.findProduct(productKey)
	if !ok {
		return model.PDDCreativeCanvas{}, safeMessageError{message: "未找到商品"}
	}
	canvas := model.PDDCreativeCanvas{
		RunID:          runID,
		ProductKey:     product.Key,
		Product:        product,
		Nodes:          request.Nodes,
		Edges:          request.Edges,
		Viewport:       request.Viewport,
		BackgroundMode: firstString(request.BackgroundMode, "lines"),
		ShowImageInfo:  request.ShowImageInfo,
		Saved:          true,
		UpdatedAt:      time.Now().Format(time.RFC3339),
		Context: map[string]interface{}{
			"source": "creative_canvas",
		},
	}
	if err := writeJSONFile(data.creativeCanvasPath(product), canvas); err != nil {
		return model.PDDCreativeCanvas{}, err
	}
	return canvas, nil
}

func SavePDDCreativeCanvasAsset(runID string, productKey string, request model.PDDCreativeAssetRequest) (model.PDDCreativeAsset, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return model.PDDCreativeAsset{}, err
	}
	productKey, _ = url.PathUnescape(firstString(productKey, request.ProductKey))
	product, ok := data.findProduct(productKey)
	if !ok {
		return model.PDDCreativeAsset{}, safeMessageError{message: "未找到商品"}
	}
	nodeID := safeFileName(strings.TrimSpace(request.NodeID), "node")
	if nodeID == "" {
		nodeID = "node"
	}
	body, mimeType, err := decodeCreativeAssetContent(request.Content, request.MimeType)
	if err != nil {
		return model.PDDCreativeAsset{}, err
	}
	if len(body) == 0 {
		return model.PDDCreativeAsset{}, safeMessageError{message: "素材内容为空"}
	}
	ext := creativeAssetExt(mimeType, request.FileName)
	fileName := safeFileName(strings.TrimSuffix(firstString(request.FileName, "artifact"), filepath.Ext(request.FileName)), "artifact") + ext
	path := filepath.Join(data.runDir, "logs", "creative_canvas", "products", safeFileName(product.SourceProduct, "product"), "artifacts", nodeID, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return model.PDDCreativeAsset{}, err
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		return model.PDDCreativeAsset{}, err
	}
	rel, _ := relativeToBase(data.runDir, path)
	asset := model.PDDCreativeAsset{URL: runFileURL(data.runID(), rel), Path: rel, FileName: fileName, MimeType: mimeType, Bytes: int64(len(body))}
	if strings.HasPrefix(mimeType, "image/") {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(body)); err == nil {
			asset.Width = float64(cfg.Width)
			asset.Height = float64(cfg.Height)
		}
	}
	return asset, nil
}

func decodeCreativeAssetContent(content string, fallbackMime string) ([]byte, string, error) {
	content = strings.TrimSpace(content)
	mimeType := firstString(fallbackMime, "application/octet-stream")
	if strings.HasPrefix(content, "data:") {
		comma := strings.Index(content, ",")
		if comma < 0 {
			return nil, "", safeMessageError{message: "data URL 不合法"}
		}
		header := content[5:comma]
		payload := content[comma+1:]
		parts := strings.Split(header, ";")
		if parts[0] != "" {
			mimeType = parts[0]
		}
		if strings.Contains(header, "base64") {
			body, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				return nil, "", safeMessageError{message: "素材 base64 解码失败"}
			}
			return body, mimeType, nil
		}
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			return nil, "", safeMessageError{message: "素材内容解码失败"}
		}
		return []byte(decoded), mimeType, nil
	}
	body, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, "", safeMessageError{message: "素材内容必须是 data URL 或 base64"}
	}
	return body, mimeType, nil
}

func creativeAssetExt(mimeType string, fileName string) string {
	if ext := strings.ToLower(filepath.Ext(fileName)); ext != "" {
		return ext
	}
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "text/plain":
		return ".txt"
	default:
		return ".bin"
	}
}

func ResolvePDDRunFile(runID string, relativePath string) (string, error) {
	runDir, err := safeRunDir(runID)
	if err != nil {
		return "", err
	}
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "", safeMessageError{message: "缺少文件路径"}
	}
	if filepath.IsAbs(relativePath) {
		if rel, ok := relativeToBase(runDir, relativePath); ok {
			relativePath = rel
		} else {
			return "", safeMessageError{message: "文件路径不在 run 目录内"}
		}
	}
	target := filepath.Clean(filepath.Join(runDir, relativePath))
	if _, ok := relativeToBase(runDir, target); !ok {
		return "", safeMessageError{message: "文件路径不在 run 目录内"}
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", safeMessageError{message: "不能直接读取目录"}
	}
	return target, nil
}

func ReadFileTail(path string, offset int64, limit int64) (string, int64) {
	file, err := os.Open(path)
	if err != nil {
		return "", offset
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", offset
	}
	size := info.Size()
	if offset > size {
		offset = 0
	}
	if offset == size {
		return "", offset
	}
	if limit <= 0 {
		limit = 64 * 1024
	}
	if size-offset > limit {
		offset = size - limit
	}
	readSize := size - offset
	if readSize > limit {
		readSize = limit
	}
	buf := make([]byte, int(readSize))
	_, _ = file.ReadAt(buf, offset)
	return string(buf), size
}

func RunPDDAction(request model.PDDActionRequest, user model.AuthUser) (model.PDDActionResult, error) {
	if config.Cfg.PDDConsoleReadOnly {
		return model.PDDActionResult{}, safeMessageError{message: "当前控制台已启用只读模式"}
	}
	action := strings.TrimSpace(request.Action)
	if action == "" {
		action = "run"
	}
	var (
		output string
		err    error
		runID  string
	)
	switch action {
	case "run":
		runID, output, err = startPDDWorkflow(request)
	case "stop":
		runID, output, err = stopPDDWorkflow(request.RunID)
	case "health_check", "docker_status", "restart_chatgpt2api", "restart_sub2api", "restart_cli_proxy", "warp_reconnect":
		output, err = runPDDServiceAction(action)
	default:
		return model.PDDActionResult{}, safeMessageError{message: "不支持的动作"}
	}
	auditPDDAction(user, action, runID, err, output)
	return model.PDDActionResult{Action: action, RunID: runID, Output: output}, err
}

func loadPDDRun(runID string) (pddRunData, error) {
	runDir, err := safeRunDir(runID)
	if err != nil {
		return pddRunData{}, err
	}
	data := pddRunData{
		runDir:   runDir,
		manifest: readJSONMap(filepath.Join(runDir, "manifest.json")),
		status:   readJSONMap(filepath.Join(runDir, "logs", "workflow_status.json")),
		summary:  readJSONMap(filepath.Join(runDir, "logs", "run_summary.json")),
		pipeline: readJSONMap(filepath.Join(runDir, "logs", "product_pipeline_summary.json")),
	}
	data.customWorkflow = boolValue(data.manifest, "custom_workflow") || exists(filepath.Join(customWorkflowDataDir(runDir), "template_snapshot.json"))
	if data.customWorkflow {
		data.customSpec = data.loadCustomWorkflowSpec()
	}
	data.products = data.loadProducts()
	return data, nil
}

func (data pddRunData) runID() string {
	if value := stringValue(data.manifest, "run_id"); value != "" {
		return value
	}
	if value := stringValue(data.status, "run_id"); value != "" {
		return value
	}
	return filepath.Base(data.runDir)
}

func (data pddRunData) runItem() model.PDDRunItem {
	status := pddStatus(stringValue(data.status, "status"))
	if boolValue(data.manifest, "completed") && status != model.PDDRunStatusError {
		status = model.PDDRunStatusSuccess
	}
	productTotal := len(data.products)
	completed := 0
	failed := 0
	running := 0
	startedAt := ""
	finishedAt := ""
	for _, item := range data.products {
		if startedAt == "" || (item.StartedAt != "" && item.StartedAt < startedAt) {
			startedAt = item.StartedAt
		}
		if item.FinishedAt != "" && item.FinishedAt > finishedAt {
			finishedAt = item.FinishedAt
		}
		switch item.Status {
		case model.PDDRunStatusSuccess:
			completed++
		case model.PDDRunStatusRunning:
			running++
		case model.PDDRunStatusError:
			failed++
		}
	}
	if status == model.PDDRunStatusIdle && running > 0 {
		status = model.PDDRunStatusRunning
	}
	if data.customWorkflow && status == model.PDDRunStatusRunning && productTotal > 0 && running == 0 && failed > 0 {
		status = model.PDDRunStatusError
	}
	if data.customWorkflow && status == model.PDDRunStatusRunning && productTotal > 0 && completed == productTotal && failed == 0 && running == 0 {
		status = model.PDDRunStatusSuccess
	}
	return model.PDDRunItem{
		RunID:             data.runID(),
		Status:            status,
		RunDir:            data.runDir,
		UpdatedAt:         firstString(stringValue(data.status, "updated_at"), stringValue(data.manifest, "updated_at"), dirModTime(data.runDir)),
		CustomWorkflow:    data.customWorkflow,
		StartedAt:         startedAt,
		FinishedAt:        finishedAt,
		Completed:         boolValue(data.manifest, "completed") || status == model.PDDRunStatusSuccess,
		HasLogs:           data.status != nil || data.pipeline != nil,
		ProductTotal:      productTotal,
		CompletedProducts: completed,
		FailedProducts:    failed,
		RunningProducts:   running,
		RecentError:       firstString(stringValue(data.status, "error"), firstError(data.products)),
	}
}

func (data pddRunData) loadProducts() []model.PDDProductSummary {
	byKey := map[string]model.PDDProductSummary{}
	for _, item := range anySlice(data.pipeline["products"]) {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		product := data.productFromRecord(record)
		if product.SourceProduct != "" {
			byKey[product.SourceProduct] = product
		}
	}
	pipelineRoot := filepath.Join(data.runDir, "logs", "product_pipeline")
	_ = filepath.WalkDir(pipelineRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "pipeline_status.json" {
			return nil
		}
		record := readJSONMap(path)
		if record == nil {
			return nil
		}
		product := data.productFromRecord(record)
		if product.SourceProduct != "" {
			byKey[product.SourceProduct] = product
		}
		return nil
	})
	if len(byKey) == 0 {
		for _, dir := range productDirs(filepath.Join(data.runDir, "generated")) {
			product := data.productFromRecord(map[string]any{"source_product": dir, "status": "completed", "product": ""})
			byKey[product.SourceProduct] = product
		}
	}
	if data.customWorkflow {
		for index, input := range data.customInputs() {
			title := productTitleFromInput(input, index+1)
			key := safeFileName(title, fmt.Sprintf("product_%03d", index+1))
			if _, ok := byKey[key]; ok {
				continue
			}
			byKey[key] = data.productFromRecord(map[string]any{
				"source_product":    key,
				"generated_product": key,
				"product":           safeFileName(title, key),
				"theme_name":        title,
				"status":            "idle",
			})
		}
	}
	items := make([]model.PDDProductSummary, 0, len(byKey))
	for _, item := range byKey {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].SourceProduct < items[j].SourceProduct
	})
	return items
}

func (data pddRunData) productFromRecord(record map[string]any) model.PDDProductSummary {
	source := firstString(stringValue(record, "source_product"), stringValue(record, "theme_name"))
	generated := firstString(stringValue(record, "generated_product"), source)
	product := stringValue(record, "product")
	rawStatus := firstString(stringValue(record, "status"), "unknown")
	if product == "" {
		product = outputProductFromFolderRename(data.productLogDir(source))
	}
	generatedImages := len(imageFiles(filepath.Join(data.runDir, "generated", generated)))
	specImages := 0
	mainImages := 0
	if product != "" {
		specImages = len(imageFiles(filepath.Join(data.runDir, "待上架", product, "规格图")))
		mainImages = len(imageFiles(filepath.Join(data.runDir, "待上架", product, "主图")))
	}
	artifactCount := 0
	if data.customWorkflow {
		artifactCount = generatedImages + specImages + mainImages
	}
	return model.PDDProductSummary{
		Key:              source,
		SourceProduct:    source,
		GeneratedProduct: generated,
		Product:          product,
		ThemeName:        firstString(stringValue(record, "theme_name"), source),
		Status:           pddStatus(rawStatus),
		RawStatus:        rawStatus,
		StartedAt:        stringValue(record, "started_at"),
		FinishedAt:       stringValue(record, "finished_at"),
		Error:            stringValue(record, "error"),
		GeneratedImages:  generatedImages,
		SpecImages:       specImages,
		MainImages:       mainImages,
		ArtifactCount:    artifactCount,
	}
}

func (data pddRunData) findProduct(key string) (model.PDDProductSummary, bool) {
	for _, item := range data.products {
		if item.Key == key || item.SourceProduct == key || item.GeneratedProduct == key || item.Product == key {
			return item, true
		}
	}
	return model.PDDProductSummary{}, false
}

func (data pddRunData) stageNodes() []model.PDDStageNode {
	if data.customWorkflow {
		return data.customStageNodes()
	}
	products := data.products
	stages := []model.PDDStageNode{
		stageNode("input_themes", "输入主题", products, func(model.PDDProductSummary) string { return "success" }),
		stageNode("standard_reference", "标准参考图关卡", products, data.standardReferenceStatus),
		stageNode("source_generation", "源图生成", products, func(item model.PDDProductSummary) string {
			if item.GeneratedImages > 0 {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("quality_review", "质检", products, func(item model.PDDProductSummary) string {
			if exists(filepath.Join(data.productLogDir(item.SourceProduct), "quality_review")) || item.Status == model.PDDRunStatusSuccess {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("repair", "修复", products, func(item model.PDDProductSummary) string {
			logDir := data.productLogDir(item.SourceProduct)
			if exists(filepath.Join(logDir, "repair_summary.json")) || exists(filepath.Join(logDir, "repairs.json")) {
				return "success"
			}
			if item.Status == model.PDDRunStatusSuccess {
				return "skipped"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("title_generation", "标题生成", products, func(item model.PDDProductSummary) string {
			if exists(filepath.Join(data.productLogDir(item.SourceProduct), "folder_renames.json")) || item.Product != "" {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("mockup", "Mockup", products, func(item model.PDDProductSummary) string {
			if item.SpecImages > 0 {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("final_main_image", "最终主图", products, func(item model.PDDProductSummary) string {
			if exists(filepath.Join(data.productLogDir(item.SourceProduct), "final_main_images.json")) || hasMainImage(data.runDir, item.Product) {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("final_main_image_review", "最终复检", products, func(item model.PDDProductSummary) string {
			if exists(filepath.Join(data.productLogDir(item.SourceProduct), "final_main_image_review")) || item.Status == model.PDDRunStatusSuccess {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("upload_check", "上传检查", products, func(item model.PDDProductSummary) string {
			if item.Product != "" && exists(filepath.Join(data.runDir, "待上架", item.Product)) {
				return "success"
			}
			return rawProductStageStatus(item)
		}),
		stageNode("upload", "上传", products, func(model.PDDProductSummary) string { return "skipped" }),
	}
	applyWorkflowDurations(stages, data.status)
	return stages
}

func (data pddRunData) stageEdges(stages []model.PDDStageNode) []model.PDDGraphEdge {
	if data.customWorkflow && len(data.customSpec.Edges) > 0 {
		return workflowTemplateEdges(data.customSpec.Edges)
	}
	return stageEdges(stages)
}

func (data pddRunData) customStageNodes() []model.PDDStageNode {
	if len(data.customSpec.Nodes) == 0 {
		return []model.PDDStageNode{stageNode("custom_workflow", "自定义工作流", data.products, rawProductStageStatus)}
	}
	stages := make([]model.PDDStageNode, 0, len(data.customSpec.Nodes))
	for _, node := range data.customSpec.Nodes {
		stage := model.PDDStageNode{
			ID:     node.ID,
			Title:  node.Title,
			Type:   node.Type,
			Total:  len(data.products),
			X:      node.X,
			Y:      node.Y,
			Width:  node.Width,
			Height: node.Height,
		}
		for _, product := range data.products {
			status, errText := data.customNodeStatus(product.SourceProduct, node.ID)
			switch status {
			case model.PDDRunStatusSuccess:
				stage.Success++
			case model.PDDRunStatusRunning:
				stage.Running++
			case model.PDDRunStatusError:
				stage.Failed++
				if stage.RecentError == "" {
					stage.RecentError = errText
				}
			default:
				stage.Idle++
			}
		}
		stage.Status = aggregatePDDStageStatus(stage)
		stages = append(stages, stage)
	}
	return stages
}

func (data pddRunData) standardReferenceStatus(item model.PDDProductSummary) string {
	gate := mapValue(data.summary, "standard_reference_gate")
	if gate == nil {
		if item.GeneratedImages > 0 || item.Status == model.PDDRunStatusSuccess {
			return "success"
		}
		return rawProductStageStatus(item)
	}
	if countAny(gate["missing_all"]) > 0 || countAny(gate["ambiguous"]) > 0 {
		return "error"
	}
	return "success"
}

func (data pddRunData) productGraph(product model.PDDProductSummary) ([]model.PDDGraphNode, []model.PDDGraphEdge, []model.PDDDetailFile) {
	if data.customWorkflow {
		return data.customProductGraph(product)
	}
	logDir := data.productLogDir(product.SourceProduct)
	generatedDir := filepath.Join(data.runDir, "generated", product.GeneratedProduct)
	outputDir := filepath.Join(data.runDir, "待上架", product.Product)
	specDir := filepath.Join(outputDir, "规格图")
	mainDir := filepath.Join(outputDir, "主图")

	sourceArtifacts := data.artifactsFromFiles("source", imageFiles(generatedDir))
	specArtifacts := data.artifactsFromFiles("spec", imageFiles(specDir))
	mainArtifacts := data.artifactsFromFiles("main", imageFiles(mainDir))

	files := []model.PDDDetailFile{}
	addFile := func(title, path string) {
		if exists(path) && !isDir(path) {
			files = append(files, data.detailFile(title, path))
		}
	}
	addFile("商品流水线状态", filepath.Join(logDir, "pipeline_status.json"))
	addFile("质检问题汇总", filepath.Join(logDir, "quality_review", "quality_issues.json"))
	addFile("修复汇总", filepath.Join(logDir, "repair_summary.json"))
	addFile("修复记录", filepath.Join(logDir, "repairs.json"))
	addFile("标题映射", filepath.Join(logDir, "folder_renames.json"))
	addFile("最终主图汇总", filepath.Join(logDir, "final_main_images.json"))
	addFile("最终主图复检", filepath.Join(logDir, "final_main_image_review", "final_main_image_review.json"))
	addFile("主图重编号", filepath.Join(logDir, "main_image_renumber.json"))

	nodes := []model.PDDGraphNode{
		{ID: "product", Type: "product", Title: product.SourceProduct, Status: product.Status, X: 0, Y: 0, Width: 280, Height: 150, Summary: product.Product, Files: files},
		{ID: "source_images", Type: "artifact_batch", Title: "源图 batch", Status: statusFromArtifacts(sourceArtifacts, product.Status), X: 360, Y: -170, Width: 300, Height: 190, Artifacts: sourceArtifacts, Summary: fmt.Sprintf("%d 张", len(sourceArtifacts))},
		{ID: "quality_review", Type: "json", Title: "质检结果", Status: statusFromFile(filepath.Join(logDir, "quality_review"), product.Status), X: 720, Y: -170, Width: 260, Height: 150, Files: filterDetailFiles(files, "质检")},
		{ID: "repair", Type: "json", Title: "修复轮次", Status: statusFromFile(filepath.Join(logDir, "repair_summary.json"), product.Status), X: 1040, Y: -170, Width: 260, Height: 150, Files: filterDetailFiles(files, "修复")},
		{ID: "title", Type: "json", Title: "标题映射", Status: statusFromFile(filepath.Join(logDir, "folder_renames.json"), product.Status), X: 360, Y: 100, Width: 300, Height: 150, Summary: product.Product, Files: filterDetailFiles(files, "标题")},
		{ID: "spec_images", Type: "artifact_batch", Title: "规格图", Status: statusFromArtifacts(specArtifacts, product.Status), X: 720, Y: 100, Width: 300, Height: 190, Artifacts: specArtifacts, Summary: fmt.Sprintf("%d 张", len(specArtifacts))},
		{ID: "main_images", Type: "artifact_batch", Title: "最终主图", Status: statusFromArtifacts(mainArtifacts, product.Status), X: 1080, Y: 100, Width: 300, Height: 190, Artifacts: mainArtifacts, Summary: fmt.Sprintf("%d 张", len(mainArtifacts))},
		{ID: "final_review", Type: "json", Title: "最终复检结果", Status: statusFromFile(filepath.Join(logDir, "final_main_image_review"), product.Status), X: 1440, Y: 100, Width: 260, Height: 150, Files: filterDetailFiles(files, "最终")},
		{ID: "output_dir", Type: "path", Title: "待上架目录", Status: statusFromFile(outputDir, product.Status), X: 1780, Y: 100, Width: 300, Height: 150, Summary: relativeOrAbsolute(data.runDir, outputDir)},
	}
	edges := []model.PDDGraphEdge{
		{ID: "e1", From: "product", To: "source_images"},
		{ID: "e2", From: "source_images", To: "quality_review"},
		{ID: "e3", From: "quality_review", To: "repair"},
		{ID: "e4", From: "source_images", To: "title"},
		{ID: "e5", From: "title", To: "spec_images"},
		{ID: "e6", From: "spec_images", To: "main_images"},
		{ID: "e7", From: "main_images", To: "final_review"},
		{ID: "e8", From: "final_review", To: "output_dir"},
	}
	return nodes, edges, files
}

func (data pddRunData) customProductGraph(product model.PDDProductSummary) ([]model.PDDGraphNode, []model.PDDGraphEdge, []model.PDDDetailFile) {
	nodes := make([]model.PDDGraphNode, 0, len(data.customSpec.Nodes))
	allFiles := []model.PDDDetailFile{}
	for _, templateNode := range data.customSpec.Nodes {
		statusPath := data.customNodeStatusPath(product.SourceProduct, templateNode.ID)
		statusPayload := readJSONMap(statusPath)
		nodeStatus, _ := customStatusFromPayload(statusPayload)
		if nodeStatus == model.PDDRunStatusIdle && product.Status == model.PDDRunStatusError {
			nodeStatus = model.PDDRunStatusIdle
		}
		files := []model.PDDDetailFile{}
		if exists(statusPath) && !isDir(statusPath) {
			file := data.detailFile(templateNode.Title+" 状态", statusPath)
			files = append(files, file)
			allFiles = append(allFiles, file)
		}
		artifacts := []model.PDDArtifact{}
		outputFiles := customOutputFiles(statusPayload)
		if templateNode.Count > 0 && len(outputFiles) > templateNode.Count {
			outputFiles = outputFiles[:templateNode.Count]
		}
		for index, outputFile := range outputFiles {
			if outputFile.Path == "" || !exists(outputFile.Path) || isDir(outputFile.Path) {
				continue
			}
			file := data.detailFile(templateNode.Title+" 输出 "+strconv.Itoa(index+1), outputFile.Path)
			files = append(files, file)
			allFiles = append(allFiles, file)
			if outputFile.Kind == "image" || isImagePath(outputFile.Path) {
				artifacts = append(artifacts, model.PDDArtifact{
					ID:       fmt.Sprintf("%s-%03d", templateNode.ID, index+1),
					Title:    filepath.Base(outputFile.Path),
					Path:     relativeOrAbsolute(data.runDir, outputFile.Path),
					URL:      runFileURL(data.runID(), relativeOrAbsolute(data.runDir, outputFile.Path)),
					Kind:     "image",
					MimeType: firstString(outputFile.MimeType, mimeFromPath(outputFile.Path)),
				})
			}
		}
		nodes = append(nodes, model.PDDGraphNode{
			ID:              templateNode.ID,
			Type:            templateNode.Type,
			Title:           templateNode.Title,
			Status:          nodeStatus,
			X:               templateNode.X,
			Y:               templateNode.Y,
			Width:           templateNode.Width,
			Height:          templateNode.Height,
			Summary:         customNodeSummary(templateNode, statusPayload),
			Config:          customNodeDisplayConfig(templateNode, statusPayload, data.customSpec.Edges),
			DurationSeconds: floatValue(statusPayload, "duration_seconds"),
			Artifacts:       artifacts,
			Files:           files,
		})
	}
	return nodes, workflowTemplateEdges(data.customSpec.Edges), allFiles
}

func (data pddRunData) creativeCanvasPath(product model.PDDProductSummary) string {
	return filepath.Join(data.runDir, "logs", "creative_canvas", "products", safeFileName(product.SourceProduct, "product"), "canvas.json")
}

func (data pddRunData) initialCreativeCanvas(product model.PDDProductSummary, graphNodes []model.PDDGraphNode, graphEdges []model.PDDGraphEdge) model.PDDCreativeCanvas {
	nodes := []model.PDDCreativeNode{}
	firstByWorkflowNode := map[string]string{}
	for _, graphNode := range graphNodes {
		added := data.creativeNodesFromGraphNode(graphNode)
		if len(added) == 0 {
			continue
		}
		firstByWorkflowNode[graphNode.ID] = added[0].ID
		nodes = append(nodes, added...)
	}
	edges := []model.PDDCreativeEdge{}
	seen := map[string]bool{}
	for _, graphEdge := range graphEdges {
		from := firstByWorkflowNode[graphEdge.From]
		to := firstByWorkflowNode[graphEdge.To]
		if from == "" || to == "" || from == to {
			continue
		}
		id := "creative-" + from + "-" + to
		if seen[id] {
			continue
		}
		seen[id] = true
		edges = append(edges, model.PDDCreativeEdge{ID: id, FromNodeID: from, ToNodeID: to})
	}
	layoutCreativeCanvasNodes(nodes)
	return model.PDDCreativeCanvas{
		RunID:      data.runID(),
		ProductKey: product.Key,
		Product:    product,
		Nodes:      nodes,
		Edges:      edges,
		Viewport: map[string]float64{
			"x": 120,
			"y": 120,
			"k": 0.82,
		},
		BackgroundMode: "lines",
		ShowImageInfo:  true,
		Saved:          false,
		UpdatedAt:      time.Now().Format(time.RFC3339),
		Context: map[string]interface{}{
			"source": "run_artifacts",
		},
	}
}

func mergeCreativeCanvasLiveState(saved model.PDDCreativeCanvas, live model.PDDCreativeCanvas) model.PDDCreativeCanvas {
	liveByID := map[string]model.PDDCreativeNode{}
	for _, node := range live.Nodes {
		liveByID[node.ID] = node
	}
	seen := map[string]bool{}
	nodes := make([]model.PDDCreativeNode, 0, len(saved.Nodes)+len(live.Nodes))
	for _, node := range saved.Nodes {
		if liveNode, ok := liveByID[node.ID]; ok {
			node = mergeCreativeNodeLiveState(node, liveNode)
		}
		seen[node.ID] = true
		nodes = append(nodes, node)
	}
	for _, node := range live.Nodes {
		if seen[node.ID] {
			continue
		}
		nodes = append(nodes, node)
	}
	edges := append([]model.PDDCreativeEdge{}, saved.Edges...)
	edgeSeen := map[string]bool{}
	for _, edge := range edges {
		edgeSeen[edge.FromNodeID+"->"+edge.ToNodeID] = true
	}
	for _, edge := range live.Edges {
		key := edge.FromNodeID + "->" + edge.ToNodeID
		if edgeSeen[key] {
			continue
		}
		edgeSeen[key] = true
		edges = append(edges, edge)
	}
	saved.Nodes = nodes
	saved.Edges = edges
	saved.Product = live.Product
	if creativeCanvasShouldAutoRelayout(saved.Nodes) {
		layoutCreativeCanvasNodes(saved.Nodes)
	}
	if saved.BackgroundMode == "" {
		saved.BackgroundMode = live.BackgroundMode
	}
	if saved.Viewport == nil {
		saved.Viewport = live.Viewport
	}
	return saved
}

func mergeCreativeNodeLiveState(saved model.PDDCreativeNode, live model.PDDCreativeNode) model.PDDCreativeNode {
	if strings.TrimSpace(saved.Title) == "" {
		saved.Title = live.Title
	}
	if strings.TrimSpace(saved.Type) == "" {
		saved.Type = live.Type
	}
	if saved.Position == nil {
		saved.Position = map[string]float64{"x": live.Position["x"], "y": live.Position["y"]}
	}
	saved.Metadata = mergeCreativeMetadata(saved.Metadata, live.Metadata)
	if !creativeNodeFreeResized(saved) && shouldResizeFromLiveNode(saved, live) {
		centerX := saved.Position["x"] + saved.Width/2
		centerY := saved.Position["y"] + saved.Height/2
		saved.Width = live.Width
		saved.Height = live.Height
		saved.Position["x"] = centerX - saved.Width/2
		saved.Position["y"] = centerY - saved.Height/2
	}
	return saved
}

func mergeCreativeMetadata(saved map[string]interface{}, live map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range saved {
		result[key] = value
	}
	for _, key := range []string{"workflowNodeId", "originWorkflowNodeId", "source"} {
		if _, exists := result[key]; exists {
			continue
		}
		if value, ok := live[key]; ok && value != nil {
			result[key] = value
		}
	}
	localOverride := creativeMetadataHasLocalContentOverride(result)
	if !localOverride {
		for _, key := range []string{"status", "errorDetails", "operation"} {
			if value, ok := live[key]; ok && value != nil {
				result[key] = value
			}
		}
		if status := strings.TrimSpace(anyToString(live["status"])); status != "" && status != "error" {
			if _, hasLiveError := live["errorDetails"]; !hasLiveError {
				delete(result, "errorDetails")
			}
		}
	} else if _, exists := result["status"]; !exists {
		if value, ok := live["status"]; ok && value != nil {
			result["status"] = value
		}
	}
	if !localOverride {
		for _, key := range []string{"content", "artifactPath", "artifactKind", "storageKey", "mimeType", "naturalWidth", "naturalHeight", "bytes"} {
			if value, ok := live[key]; ok && value != nil {
				result[key] = value
			}
		}
	}
	for _, key := range []string{"prompt", "model", "size", "quality", "count", "seconds", "vquality", "videoReferenceMode"} {
		if _, exists := result[key]; exists {
			continue
		}
		if value, ok := live[key]; ok && value != nil {
			result[key] = value
		}
	}
	return result
}

func creativeMetadataHasLocalContentOverride(metadata map[string]interface{}) bool {
	source := strings.TrimSpace(anyToString(metadata["source"]))
	return source == "user_upload" || strings.HasPrefix(source, "creative_")
}

func creativeNodeFreeResized(node model.PDDCreativeNode) bool {
	if node.Metadata == nil {
		return false
	}
	value, _ := node.Metadata["freeResize"].(bool)
	return value
}

func shouldResizeFromLiveNode(saved model.PDDCreativeNode, live model.PDDCreativeNode) bool {
	if live.Width <= 0 || live.Height <= 0 {
		return false
	}
	if saved.Type != "image" && saved.Type != "video" {
		return false
	}
	if saved.Metadata == nil {
		return true
	}
	source := strings.TrimSpace(anyToString(saved.Metadata["source"]))
	if strings.HasPrefix(source, "creative_") || source == "user_upload" {
		return false
	}
	return anyToString(saved.Metadata["artifactPath"]) != anyToString(live.Metadata["artifactPath"]) || anyToString(saved.Metadata["content"]) == "" || saved.Width < live.Width || saved.Height < live.Height
}

func layoutCreativeCanvasNodes(nodes []model.PDDCreativeNode) {
	if len(nodes) == 0 {
		return
	}
	for index := range nodes {
		if nodes[index].Position == nil {
			nodes[index].Position = map[string]float64{"x": 0, "y": 0}
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Position["x"] == nodes[j].Position["x"] {
			return nodes[i].Position["y"] < nodes[j].Position["y"]
		}
		return nodes[i].Position["x"] < nodes[j].Position["x"]
	})
	type column struct {
		key   float64
		nodes []*model.PDDCreativeNode
		width float64
	}
	columns := []column{}
	for index := range nodes {
		node := &nodes[index]
		x := node.Position["x"]
		if len(columns) == 0 || absFloat(columns[len(columns)-1].key-x) > 180 {
			columns = append(columns, column{key: x})
		}
		current := &columns[len(columns)-1]
		current.nodes = append(current.nodes, node)
		if node.Width > current.width {
			current.width = node.Width
		}
	}
	xCursor := 80.0
	for _, col := range columns {
		sort.SliceStable(col.nodes, func(i, j int) bool {
			return col.nodes[i].Position["y"] < col.nodes[j].Position["y"]
		})
		yCursor := 80.0
		for _, node := range col.nodes {
			node.Position["x"] = xCursor
			node.Position["y"] = yCursor
			yCursor += node.Height + 88
		}
		xCursor += col.width + 140
	}
}

func creativeCanvasShouldAutoRelayout(nodes []model.PDDCreativeNode) bool {
	if len(nodes) < 2 {
		return false
	}
	for _, node := range nodes {
		if creativeMetadataHasLocalContentOverride(node.Metadata) {
			return false
		}
	}
	return creativeCanvasHasOverlappingNodes(nodes, 24)
}

func creativeCanvasHasOverlappingNodes(nodes []model.PDDCreativeNode, padding float64) bool {
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if creativeNodesOverlap(nodes[i], nodes[j], padding) {
				return true
			}
		}
	}
	return false
}

func creativeNodesOverlap(a model.PDDCreativeNode, b model.PDDCreativeNode, padding float64) bool {
	if a.Position == nil || b.Position == nil || a.Width <= 0 || a.Height <= 0 || b.Width <= 0 || b.Height <= 0 {
		return false
	}
	ax1 := a.Position["x"] - padding
	ay1 := a.Position["y"] - padding
	ax2 := a.Position["x"] + a.Width + padding
	ay2 := a.Position["y"] + a.Height + padding
	bx1 := b.Position["x"] - padding
	by1 := b.Position["y"] - padding
	bx2 := b.Position["x"] + b.Width + padding
	by2 := b.Position["y"] + b.Height + padding
	return ax1 < bx2 && ax2 > bx1 && ay1 < by2 && ay2 > by1
}

func (data pddRunData) creativeNodesFromGraphNode(graphNode model.PDDGraphNode) []model.PDDCreativeNode {
	nodes := []model.PDDCreativeNode{}
	if shouldIncludeCreativeTextNode(graphNode) {
		metadata := creativeNodeMetadata(graphNode, "run_text")
		metadata["content"] = graphNode.Summary
		nodes = append(nodes, model.PDDCreativeNode{
			ID:       graphNode.ID,
			Type:     "text",
			Title:    graphNode.Title,
			Position: map[string]float64{"x": graphNode.X, "y": graphNode.Y},
			Width:    graphNode.Width,
			Height:   graphNode.Height,
			Metadata: metadata,
		})
	}
	for index, artifact := range graphNode.Artifacts {
		nodeType := "image"
		if strings.HasPrefix(strings.ToLower(artifact.MimeType), "video/") || artifact.Kind == "video" {
			nodeType = "video"
		}
		id := graphNode.ID
		if len(graphNode.Artifacts) > 1 {
			id = fmt.Sprintf("%s-%02d", graphNode.ID, index+1)
		}
		x := graphNode.X + float64(index%3)*340
		y := graphNode.Y + float64(index/3)*280
		metadata := creativeNodeMetadata(graphNode, "run_artifact")
		metadata["content"] = artifact.URL
		metadata["artifactPath"] = artifact.Path
		metadata["artifactKind"] = artifact.Kind
		metadata["storageKey"] = artifact.Path
		metadata["mimeType"] = artifact.MimeType
		width := creativeNodeWidth(nodeType, graphNode.Width)
		height := creativeNodeHeight(nodeType, graphNode.Height)
		if naturalWidth, naturalHeight, ok := data.creativeArtifactImageSize(artifact); ok {
			metadata["naturalWidth"] = naturalWidth
			metadata["naturalHeight"] = naturalHeight
			width, height = creativeNodeSizeFromNatural(nodeType, naturalWidth, naturalHeight, width, height)
		}
		nodes = append(nodes, model.PDDCreativeNode{
			ID:       id,
			Type:     nodeType,
			Title:    firstString(graphNode.Title, artifact.Title),
			Position: map[string]float64{"x": x, "y": y},
			Width:    width,
			Height:   height,
			Metadata: metadata,
		})
	}
	if len(nodes) == 0 {
		nodeType := creativeNodeTypeFromGraphNode(graphNode)
		if nodeType != "" {
			metadata := creativeNodeMetadata(graphNode, "run_placeholder")
			if nodeType == "text" && strings.TrimSpace(graphNode.Summary) != "" {
				metadata["content"] = graphNode.Summary
			}
			nodes = append(nodes, model.PDDCreativeNode{
				ID:       graphNode.ID,
				Type:     nodeType,
				Title:    graphNode.Title,
				Position: map[string]float64{"x": graphNode.X, "y": graphNode.Y},
				Width:    creativeNodeWidth(nodeType, graphNode.Width),
				Height:   creativeNodeHeight(nodeType, graphNode.Height),
				Metadata: metadata,
			})
		}
	}
	return nodes
}

func creativeNodeMetadata(graphNode model.PDDGraphNode, source string) map[string]interface{} {
	status := string(graphNode.Status)
	if status == string(model.PDDRunStatusRunning) {
		status = "loading"
	}
	metadata := map[string]interface{}{
		"status":         status,
		"workflowNodeId": graphNode.ID,
		"source":         source,
		"prompt":         configStringValue(graphNode.Config, "prompt"),
		"model":          configStringValue(graphNode.Config, "model"),
		"size":           configStringValue(graphNode.Config, "size"),
		"quality":        configStringValue(graphNode.Config, "quality"),
		"count":          configStringValue(graphNode.Config, "count"),
		"operation":      configStringValue(graphNode.Config, "operation"),
	}
	if graphNode.Status == model.PDDRunStatusError && strings.TrimSpace(graphNode.Summary) != "" {
		metadata["errorDetails"] = graphNode.Summary
	}
	return metadata
}

func creativeNodeTypeFromGraphNode(node model.PDDGraphNode) string {
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	operation := strings.ToLower(configStringValue(node.Config, "operation"))
	title := strings.ToLower(node.Title)
	if nodeType == "video" || strings.Contains(operation, "video") {
		return "video"
	}
	if nodeType == "image" || nodeType == "material" || strings.Contains(operation, "image") || strings.Contains(operation, "mockup") || strings.Contains(title, "图") || strings.Contains(title, "mockup") {
		return "image"
	}
	if nodeType == "text" || operation == "input" || strings.Contains(operation, "text") || strings.Contains(operation, "title") || strings.Contains(title, "标题") || strings.Contains(title, "输入") || strings.Contains(title, "质检") || strings.Contains(title, "判定") {
		return "text"
	}
	if nodeType == "config" || strings.Contains(operation, "condition") || strings.Contains(operation, "script") {
		return "config"
	}
	return ""
}

func shouldIncludeCreativeTextNode(node model.PDDGraphNode) bool {
	if strings.TrimSpace(node.Summary) == "" || node.Type != "text" {
		return false
	}
	operation := configStringValue(node.Config, "operation")
	title := strings.ToLower(node.Title)
	if operation == "input" {
		return true
	}
	if strings.Contains(title, "标题") {
		return true
	}
	if extra, ok := node.Config["extra"].(map[string]any); ok {
		if titleProvider, ok := extra["titleProvider"].(bool); ok && titleProvider {
			return true
		}
	}
	return false
}

func creativeNodeWidth(nodeType string, fallback float64) float64 {
	if nodeType == "video" {
		return 420
	}
	if fallback >= 220 {
		return fallback
	}
	return 320
}

func creativeNodeHeight(nodeType string, fallback float64) float64 {
	if nodeType == "video" {
		return 236
	}
	if fallback >= 160 {
		return fallback
	}
	return 220
}

func (data pddRunData) creativeArtifactImageSize(artifact model.PDDArtifact) (float64, float64, bool) {
	if artifact.Kind != "image" && !strings.HasPrefix(strings.ToLower(artifact.MimeType), "image/") && !isImagePath(artifact.Path) {
		return 0, 0, false
	}
	path := artifact.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(data.runDir, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return float64(cfg.Width), float64(cfg.Height), true
}

func creativeNodeSizeFromNatural(nodeType string, width float64, height float64, fallbackWidth float64, fallbackHeight float64) (float64, float64) {
	if width <= 0 || height <= 0 {
		return fallbackWidth, fallbackHeight
	}
	maxWidth := 640.0
	maxHeight := 640.0
	if nodeType == "video" {
		maxWidth = 420
		maxHeight = 420
	}
	scale := 1.0
	if width > maxWidth || height > maxHeight {
		scale = maxWidth / width
		if heightScale := maxHeight / height; heightScale < scale {
			scale = heightScale
		}
	}
	return width * scale, height * scale
}

func configStringValue(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value := config[key]
	switch typed := value.(type) {
	case string:
		return typed
	case float64, int, bool:
		return fmt.Sprint(typed)
	default:
		return ""
	}
}

func (data pddRunData) productLogDir(sourceProduct string) string {
	return filepath.Join(data.runDir, "logs", "product_pipeline", sourceProduct)
}

func (data pddRunData) loadCustomWorkflowSpec() model.WorkflowTemplateSpec {
	var template model.WorkflowTemplate
	body, err := os.ReadFile(filepath.Join(customWorkflowDataDir(data.runDir), "template_snapshot.json"))
	if err != nil {
		return model.WorkflowTemplateSpec{}
	}
	if err := json.Unmarshal(body, &template); err != nil {
		return model.WorkflowTemplateSpec{}
	}
	return normalizeWorkflowTemplateSpec(template.Spec)
}

func (data pddRunData) customInputs() []map[string]any {
	body, err := os.ReadFile(filepath.Join(customWorkflowDataDir(data.runDir), "inputs.json"))
	if err != nil {
		return nil
	}
	var inputs []map[string]any
	if err := json.Unmarshal(body, &inputs); err != nil {
		return nil
	}
	return inputs
}

func (data pddRunData) customProductNodeDir(productKey string) string {
	root := filepath.Join(customWorkflowDataDir(data.runDir), "products")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && len(name) > 4 && name[3] == '_' && name[4:] == productKey {
			return filepath.Join(root, name)
		}
	}
	return ""
}

func (data pddRunData) customNodeStatusPath(productKey string, nodeID string) string {
	productDir := data.customProductNodeDir(productKey)
	if productDir == "" {
		return ""
	}
	return filepath.Join(productDir, "nodes", safeFileName(nodeID, "node"), "status.json")
}

func (data pddRunData) customNodeStatus(productKey string, nodeID string) (model.PDDRunStatus, string) {
	return customStatusFromPayload(readJSONMap(data.customNodeStatusPath(productKey, nodeID)))
}

func customStatusFromPayload(payload map[string]any) (model.PDDRunStatus, string) {
	if payload == nil {
		return model.PDDRunStatusIdle, ""
	}
	return pddStatus(stringValue(payload, "status")), stringValue(payload, "error")
}

func aggregatePDDStageStatus(stage model.PDDStageNode) model.PDDRunStatus {
	if stage.Failed > 0 {
		return model.PDDRunStatusError
	}
	if stage.Running > 0 || (stage.Success > 0 && stage.Idle > 0) {
		return model.PDDRunStatusRunning
	}
	if stage.Total > 0 && stage.Success+stage.Skipped == stage.Total {
		return model.PDDRunStatusSuccess
	}
	return model.PDDRunStatusIdle
}

func workflowTemplateEdges(edges []model.WorkflowTemplateEdge) []model.PDDGraphEdge {
	result := make([]model.PDDGraphEdge, 0, len(edges))
	for index, edge := range edges {
		id := strings.TrimSpace(edge.ID)
		if id == "" {
			id = fmt.Sprintf("edge-%d", index+1)
		}
		result = append(result, model.PDDGraphEdge{ID: id, From: edge.From, To: edge.To})
	}
	return result
}

func customOutputFiles(payload map[string]any) []workflowNodeOutputFile {
	output := mapValue(payload, "output")
	if output == nil {
		return nil
	}
	files := []workflowNodeOutputFile{}
	for _, item := range anySlice(output["files"]) {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(anyToString(record["path"]))
		if path == "" {
			continue
		}
		files = append(files, workflowNodeOutputFile{
			Path:     path,
			Kind:     firstString(anyToString(record["kind"]), detailKind(path)),
			MimeType: firstString(anyToString(record["mimeType"]), mimeFromPath(path)),
		})
	}
	return files
}

func customNodeSummary(node model.WorkflowTemplateNode, payload map[string]any) string {
	if payload == nil {
		return node.Operation
	}
	if errText := stringValue(payload, "error"); errText != "" {
		return errText
	}
	if output := mapValue(payload, "output"); output != nil {
		if text := strings.TrimSpace(anyToString(output["text"])); text != "" {
			return truncateText(text, 120)
		}
		if files := len(anySlice(output["files"])); files > 0 {
			if node.Count > 0 && files > node.Count {
				files = node.Count
			}
			return fmt.Sprintf("%s · %d 个输出文件", node.Operation, files)
		}
	}
	return firstString(node.Operation, node.Type)
}

func customNodeDisplayConfig(node model.WorkflowTemplateNode, payload map[string]any, edges []model.WorkflowTemplateEdge) map[string]any {
	config := map[string]any{
		"type":      node.Type,
		"operation": node.Operation,
	}
	if node.Model != "" {
		config["model"] = node.Model
	}
	if strings.TrimSpace(node.Prompt) != "" {
		config["prompt"] = node.Prompt
	}
	if node.Count > 0 {
		config["count"] = node.Count
	}
	if node.Size != "" {
		config["size"] = node.Size
	}
	if node.Quality != "" {
		config["quality"] = node.Quality
	}
	if node.Seconds != "" {
		config["seconds"] = node.Seconds
	}
	if node.VideoQuality != "" {
		config["videoQuality"] = node.VideoQuality
	}
	if len(node.OutputMappings) > 0 {
		config["outputMappings"] = node.OutputMappings
	}
	if len(node.Extra) > 0 {
		config["extra"] = node.Extra
	}
	upstream := []string{}
	for _, edge := range edges {
		if edge.To == node.ID {
			upstream = append(upstream, edge.From)
		}
	}
	if len(upstream) > 0 {
		config["upstream"] = upstream
	}
	if payload != nil {
		if value := stringValue(payload, "started_at"); value != "" {
			config["startedAt"] = value
		}
		if value := stringValue(payload, "finished_at"); value != "" {
			config["finishedAt"] = value
		}
		if value := floatValue(payload, "duration_seconds"); value > 0 {
			config["durationSeconds"] = value
		}
	}
	return config
}

func (data pddRunData) customProductArtifactCount(productKey string) int {
	if !data.customWorkflow {
		return 0
	}
	productDir := data.customProductNodeDir(productKey)
	if productDir == "" {
		return 0
	}
	count := 0
	_ = filepath.WalkDir(filepath.Join(productDir, "nodes"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "status.json" {
			return nil
		}
		payload := readJSONMap(path)
		count += len(customOutputFiles(payload))
		return nil
	})
	return count
}

func (data pddRunData) artifactsFromFiles(kind string, files []string) []model.PDDArtifact {
	result := make([]model.PDDArtifact, 0, len(files))
	for i, file := range files {
		rel := relativeOrAbsolute(data.runDir, file)
		result = append(result, model.PDDArtifact{
			ID:       fmt.Sprintf("%s-%03d", kind, i+1),
			Title:    filepath.Base(file),
			Path:     rel,
			URL:      runFileURL(data.runID(), rel),
			Kind:     kind,
			MimeType: mimeFromPath(file),
		})
	}
	return result
}

func (data pddRunData) detailFile(title string, path string) model.PDDDetailFile {
	rel := relativeOrAbsolute(data.runDir, path)
	return model.PDDDetailFile{Title: title, Path: rel, URL: runFileURL(data.runID(), rel), Kind: detailKind(path)}
}

func (data pddRunData) recentErrors(limit int) []string {
	result := []string{}
	if text := stringValue(data.status, "error"); text != "" {
		result = append(result, text)
	}
	for _, item := range data.products {
		if item.Error != "" {
			result = append(result, fmt.Sprintf("%s: %s", item.SourceProduct, item.Error))
		}
		if len(result) >= limit {
			break
		}
	}
	return result
}

func stageNode(id string, title string, products []model.PDDProductSummary, resolve func(model.PDDProductSummary) string) model.PDDStageNode {
	node := model.PDDStageNode{ID: id, Title: title, Total: len(products)}
	for _, product := range products {
		switch resolve(product) {
		case "success":
			node.Success++
		case "running":
			node.Running++
		case "error":
			node.Failed++
			if node.RecentError == "" {
				node.RecentError = product.Error
			}
		case "skipped":
			node.Skipped++
		default:
			node.Idle++
		}
	}
	node.Status = aggregateStageStatus(node)
	return node
}

func aggregateStageStatus(node model.PDDStageNode) model.PDDRunStatus {
	if node.Failed > 0 {
		return model.PDDRunStatusError
	}
	if node.Running > 0 {
		return model.PDDRunStatusRunning
	}
	if node.Success > 0 {
		return model.PDDRunStatusSuccess
	}
	return model.PDDRunStatusIdle
}

func stageEdges(stages []model.PDDStageNode) []model.PDDGraphEdge {
	edges := make([]model.PDDGraphEdge, 0, len(stages)-1)
	for i := 1; i < len(stages); i++ {
		edges = append(edges, model.PDDGraphEdge{
			ID:   fmt.Sprintf("stage-%d", i),
			From: stages[i-1].ID,
			To:   stages[i].ID,
		})
	}
	return edges
}

func applyWorkflowDurations(stages []model.PDDStageNode, status map[string]any) {
	workflowStages := anySlice(status["stages"])
	durationByName := map[string]float64{}
	errorByName := map[string]string{}
	for _, item := range workflowStages {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := stringValue(record, "name")
		durationByName[name] = floatValue(record, "duration_seconds")
		errorByName[name] = stringValue(record, "error")
	}
	for i := range stages {
		switch stages[i].ID {
		case "input_themes":
			stages[i].DurationSeconds = durationByName["load_themes"]
		case "standard_reference", "source_generation", "quality_review", "repair", "title_generation", "mockup", "final_main_image", "final_main_image_review":
			stages[i].DurationSeconds = durationByName["generate_and_process_products"]
			if stages[i].RecentError == "" {
				stages[i].RecentError = errorByName["generate_and_process_products"]
			}
		case "upload_check":
			stages[i].DurationSeconds = durationByName["validate_products"]
		case "upload":
			stages[i].DurationSeconds = durationByName["upload"]
		}
	}
}

func pddRunsRoot() string {
	return filepath.Clean(config.Cfg.PDDRunsRoot)
}

func pddWorkflowRoot() string {
	return filepath.Clean(config.Cfg.PDDWorkflowRoot)
}

func safeRunDir(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || !runIDPattern.MatchString(runID) {
		return "", safeMessageError{message: "run_id 不合法"}
	}
	root := pddRunsRoot()
	dir := filepath.Clean(filepath.Join(root, runID))
	if _, ok := relativeToBase(root, dir); !ok {
		return "", safeMessageError{message: "run 目录不合法"}
	}
	if info, err := os.Stat(dir); err != nil {
		return "", err
	} else if !info.IsDir() {
		return "", safeMessageError{message: "run 不是目录"}
	}
	return dir, nil
}

func relativeToBase(base, path string) (string, bool) {
	baseAbs, err := filepath.Abs(filepath.Clean(base))
	if err != nil {
		return "", false
	}
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		if pathAbs == baseAbs {
			return ".", true
		}
		return "", false
	}
	return rel, true
}

func readJSONMap(path string) map[string]any {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	return payload
}

func productDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	result := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			result = append(result, entry.Name())
		}
	}
	sort.Strings(result)
	return result
}

func imageFiles(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	result := []string{}
	for _, entry := range entries {
		if entry.IsDir() || !isImagePath(entry.Name()) {
			continue
		}
		result = append(result, filepath.Join(root, entry.Name()))
	}
	sort.Strings(result)
	return result
}

func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func pddStatus(raw string) model.PDDRunStatus {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "completed", "complete", "success", "succeeded":
		return model.PDDRunStatusSuccess
	case "running", "processing", "pending":
		return model.PDDRunStatusRunning
	case "failed", "error", "manual_review", "source_regeneration_exhausted":
		return model.PDDRunStatusError
	default:
		return model.PDDRunStatusIdle
	}
}

func rawProductStageStatus(item model.PDDProductSummary) string {
	switch item.Status {
	case model.PDDRunStatusRunning:
		return "running"
	case model.PDDRunStatusError:
		return "error"
	case model.PDDRunStatusSuccess:
		return "success"
	default:
		return "idle"
	}
}

func statusFromArtifacts(artifacts []model.PDDArtifact, fallback model.PDDRunStatus) model.PDDRunStatus {
	if len(artifacts) > 0 {
		return model.PDDRunStatusSuccess
	}
	if fallback == model.PDDRunStatusError || fallback == model.PDDRunStatusRunning {
		return fallback
	}
	return model.PDDRunStatusIdle
}

func statusFromFile(path string, fallback model.PDDRunStatus) model.PDDRunStatus {
	if exists(path) {
		return model.PDDRunStatusSuccess
	}
	if fallback == model.PDDRunStatusError || fallback == model.PDDRunStatusRunning {
		return fallback
	}
	return model.PDDRunStatusIdle
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func hasMainImage(runDir, product string) bool {
	if product == "" {
		return false
	}
	return exists(filepath.Join(runDir, "待上架", product, "主图", "1_主图.png"))
}

func outputProductFromFolderRename(logDir string) string {
	payload := readJSONMap(filepath.Join(logDir, "folder_renames.json"))
	for _, item := range anySlice(payload["folders"]) {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if title := stringValue(record, "title"); title != "" {
			return title
		}
		if target := stringValue(record, "target"); target != "" {
			return filepath.Base(target)
		}
	}
	return ""
}

func runFileURL(runID, relativePath string) string {
	return "/api/workflows/pdd/runs/" + url.PathEscape(runID) + "/file?path=" + url.QueryEscape(relativePath)
}

func relativeOrAbsolute(base, path string) string {
	if rel, ok := relativeToBase(base, path); ok {
		return rel
	}
	return path
}

func detailKind(path string) string {
	if isImagePath(path) {
		return "image"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".log", ".txt":
		return "text"
	default:
		return "file"
	}
}

func mimeFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".txt", ".log", ".md":
		return "text/plain"
	case ".json":
		return "application/json"
	default:
		return ""
	}
}

func filterDetailFiles(files []model.PDDDetailFile, keyword string) []model.PDDDetailFile {
	result := []model.PDDDetailFile{}
	for _, item := range files {
		if strings.Contains(item.Title, keyword) {
			result = append(result, item)
		}
	}
	return result
}

func mapValue(source map[string]any, key string) map[string]any {
	value, ok := source[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func anySlice(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func stringValue(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	switch value := source[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func boolValue(source map[string]any, key string) bool {
	value, ok := source[key].(bool)
	return ok && value
}

func floatValue(source map[string]any, key string) float64 {
	switch value := source[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		result, _ := value.Float64()
		return result
	default:
		return 0
	}
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstError(products []model.PDDProductSummary) string {
	for _, product := range products {
		if product.Error != "" {
			return product.Error
		}
	}
	return ""
}

func countAny(value any) int {
	switch item := value.(type) {
	case []any:
		return len(item)
	case []string:
		return len(item)
	case float64:
		return int(item)
	case int:
		return item
	default:
		return 0
	}
}

func dirModTime(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return info.ModTime().Format(time.RFC3339)
}

func fileModTime(path string) string {
	return dirModTime(path)
}

func latestTimeString(values ...string) string {
	latest := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if latest == "" || value > latest {
			latest = value
		}
	}
	return latest
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func startPDDWorkflow(request model.PDDActionRequest) (string, string, error) {
	runID := strings.TrimSpace(request.RunID)
	if runID == "" {
		runID = time.Now().Format("20060102_150405")
	}
	if !runIDPattern.MatchString(runID) {
		return "", "", safeMessageError{message: "run_id 不合法"}
	}
	args := []string{"run_workflow.py", "--config", config.Cfg.PDDWorkflowConfig, "--run-id", runID, "--skip-upload"}
	if len(request.ConsoleSpec) > 0 {
		specPath, err := writePDDConsoleSpec(runID, request.ConsoleSpec)
		if err != nil {
			return "", "", err
		}
		args = append(args, "--console-spec", specPath)
	}
	if request.CountPerTheme > 0 {
		args = append(args, "--count-per-theme", strconv.Itoa(request.CountPerTheme))
	}
	for _, arg := range request.ExtraArgs {
		if !allowedWorkflowExtraArg(arg) {
			return "", "", safeMessageError{message: "包含不允许的 workflow 参数"}
		}
		args = append(args, arg)
	}
	runDir := filepath.Join(pddRunsRoot(), runID)
	script := fmt.Sprintf(`set -euo pipefail
cd %s
mkdir -p %s
log=%s
pid_file=%s
if [ -s "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
  echo "[pdd-console] run already running pid=$(cat "$pid_file")"
  exit 0
fi
printf '\n[pdd-console] ==== start run_id=%s at %%s ====\n' "$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" >> "$log"
if [ -f %s ]; then
  set -a
  . %s
  set +a
fi
nohup %s %s >> "$log" 2>&1 < /dev/null &
pid=$!
echo "$pid" > "$pid_file"
echo "[pdd-console] detached run_id=%s pid=$pid log=$log"
`, shellQuote(pddWorkflowRoot()), shellQuote(runDir), shellQuote(filepath.Join(runDir, "remote_workflow.log")), shellQuote(filepath.Join(runDir, "remote_workflow.pid")), runID, shellQuote(config.Cfg.PDDWorkflowEnvFile), shellQuote(config.Cfg.PDDWorkflowEnvFile), shellQuote(config.Cfg.PDDPython), joinShellArgs(args), runID)
	output, err := runHostShell(script, 45*time.Second)
	return runID, output, err
}

func writePDDConsoleSpec(runID string, payload map[string]any) (string, error) {
	if err := validatePDDConsoleSpec(payload); err != nil {
		return "", err
	}
	spec := map[string]any{}
	for key, value := range payload {
		spec[key] = value
	}
	if _, ok := spec["version"]; !ok {
		spec["version"] = 1
	}
	spec["created_by"] = "ops-canvas-console"
	spec["created_at"] = time.Now().Format(time.RFC3339)
	path := filepath.Join(pddRunsRoot(), runID, "console_input", "console_workflow_spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func validatePDDConsoleSpec(payload map[string]any) error {
	themes, ok := payload["themes"].([]any)
	if !ok || len(themes) == 0 {
		return safeMessageError{message: "控制台任务至少需要 1 个主题"}
	}
	stages, _ := payload["stages"].(map[string]any)
	for _, name := range []string{"source", "final_main"} {
		stage, _ := stages[name].(map[string]any)
		if stage == nil {
			continue
		}
		count, err := consoleStageImageCount(stage)
		if err != nil {
			return err
		}
		if count > 10 {
			return safeMessageError{message: "单个商品每个图片阶段最多生成 10 张图片"}
		}
	}
	return nil
}

func consoleStageImageCount(stage map[string]any) (int, error) {
	raw := stage["prompts"]
	if raw == nil {
		return positivePayloadInt(stage["count"], 1), nil
	}
	if _, ok := raw.(string); ok {
		return positivePayloadInt(stage["count"], 1), nil
	}
	items, ok := raw.([]any)
	if !ok {
		return 0, safeMessageError{message: "图片阶段 prompts 必须是文本或数组"}
	}
	total := 0
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			total += positivePayloadInt(row["count"], 1)
		} else {
			total++
		}
	}
	return total, nil
}

func positivePayloadInt(value any, fallback int) int {
	switch item := value.(type) {
	case float64:
		if item > 0 {
			return int(item)
		}
	case int:
		if item > 0 {
			return item
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(item))
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func stopPDDWorkflow(runID string) (string, string, error) {
	if strings.TrimSpace(runID) == "" || !runIDPattern.MatchString(runID) {
		return "", "", safeMessageError{message: "run_id 不合法"}
	}
	runDir := filepath.Join(pddRunsRoot(), runID)
	script := fmt.Sprintf(`set -euo pipefail
pid_file=%s
if [ ! -s "$pid_file" ]; then
  echo "[pdd-console] no pid file for run_id=%s"
  exit 0
fi
pid="$(cat "$pid_file")"
if ! kill -0 "$pid" 2>/dev/null; then
  echo "[pdd-console] pid not running: $pid"
  exit 0
fi
echo "[pdd-console] stopping run_id=%s pid=$pid"
pkill -TERM -P "$pid" 2>/dev/null || true
kill -TERM "$pid" 2>/dev/null || true
sleep 3
if kill -0 "$pid" 2>/dev/null; then
  pkill -KILL -P "$pid" 2>/dev/null || true
  kill -KILL "$pid" 2>/dev/null || true
fi
`, shellQuote(filepath.Join(runDir, "remote_workflow.pid")), runID, runID)
	output, err := runHostShell(script, 30*time.Second)
	if err == nil {
		if markErr := markPDDWorkflowStopped(runID, runDir); markErr != nil {
			output = strings.TrimSpace(output) + "\n[pdd-console] stop status update failed: " + markErr.Error()
		}
	}
	return runID, output, err
}

func markPDDWorkflowStopped(runID string, runDir string) error {
	now := time.Now().Format(time.RFC3339)
	message := "用户手动停止"
	statusPath := filepath.Join(runDir, "logs", "workflow_status.json")
	status := readJSONMap(statusPath)
	if status == nil {
		status = map[string]any{}
	}
	status["run_id"] = runID
	status["status"] = "failed"
	status["state"] = "failed"
	status["completed"] = true
	status["error"] = message
	status["finished_at"] = now
	status["updated_at"] = now
	if err := writeJSONFile(statusPath, status); err != nil {
		return err
	}
	manifestPath := filepath.Join(runDir, "manifest.json")
	manifest := readJSONMap(manifestPath)
	if manifest == nil {
		manifest = map[string]any{}
	}
	manifest["run_id"] = runID
	manifest["completed"] = true
	manifest["status"] = "failed"
	manifest["error"] = message
	manifest["finished_at"] = now
	manifest["updated_at"] = now
	_ = writeJSONFile(manifestPath, manifest)
	markStoppedStatusFiles(filepath.Join(runDir, "logs", "custom_workflow", "products"), message, now)
	markStoppedStatusFiles(filepath.Join(runDir, "logs", "product_pipeline"), message, now)
	_ = appendTextFile(filepath.Join(runDir, "remote_workflow.log"), "[pdd-console] stopped by user at "+now+"\n")
	return nil
}

func markStoppedStatusFiles(root string, message string, now string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || (entry.Name() != "status.json" && entry.Name() != "pipeline_status.json") {
			return nil
		}
		payload := readJSONMap(path)
		if payload == nil {
			return nil
		}
		current := strings.ToLower(stringValue(payload, "status"))
		if current == "success" || current == "failed" || current == "error" {
			return nil
		}
		payload["status"] = "failed"
		payload["state"] = "failed"
		payload["error"] = message
		payload["finished_at"] = now
		payload["updated_at"] = now
		_ = writeJSONFile(path, payload)
		return nil
	})
}

func appendTextFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func runPDDServiceAction(action string) (string, error) {
	var script string
	switch action {
	case "health_check":
		script = `set -e
echo "[pdd-console] host=$(hostname)"
echo "[pdd-console] docker:"
docker ps --format '{{.Names}} {{.Status}}' || true
echo "[pdd-console] ports:"
ss -ltnp | grep -E ':(8000|8080|8317|3000)\b' || true
echo "[pdd-console] services:"
systemctl is-active warp-svc privoxy docker 2>/dev/null || true
echo "[pdd-console] chatgpt2api:"
curl -fsS http://127.0.0.1:8000/version 2>/dev/null || true
`
	case "docker_status":
		script = `docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'`
	case "restart_chatgpt2api":
		script = `docker restart chatgpt2api && docker ps --filter name=chatgpt2api --format '{{.Names}} {{.Status}}'`
	case "restart_sub2api":
		script = `docker restart sub2api && docker ps --filter name=sub2api --format '{{.Names}} {{.Status}}'`
	case "restart_cli_proxy":
		script = `docker restart cli-proxy-api-vps && docker ps --filter name=cli-proxy-api-vps --format '{{.Names}} {{.Status}}'`
	case "warp_reconnect":
		script = `warp-cli --accept-tos disconnect || true
sleep 3
warp-cli --accept-tos connect
sleep 5
warp-cli --accept-tos status`
	default:
		return "", safeMessageError{message: "不支持的服务动作"}
	}
	return runHostShell(script, time.Duration(config.Cfg.PDDActionTimeoutSecs)*time.Second)
}

func runHostShell(script string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := []string{"-lc", script}
	if config.Cfg.PDDActionNSenter {
		args = []string{"-t", "1", "-m", "-u", "-n", "-i", "-p", "--", "bash", "-lc", script}
	}
	bin := "bash"
	if config.Cfg.PDDActionNSenter {
		bin = "nsenter"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.String(), safeMessageError{message: "动作执行超时"}
	}
	return out.String(), err
}

func auditPDDAction(user model.AuthUser, action string, runID string, err error, output string) {
	path := strings.TrimSpace(config.Cfg.PDDActionAuditLog)
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	file, openErr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if openErr != nil {
		return
	}
	defer file.Close()
	record := map[string]any{
		"at":     time.Now().Format(time.RFC3339),
		"user":   user.Username,
		"action": action,
		"run_id": runID,
		"ok":     err == nil,
		"error":  "",
		"output": truncateText(output, 4000),
	}
	if err != nil {
		record["error"] = err.Error()
	}
	body, _ := json.Marshal(record)
	_, _ = file.Write(append(body, '\n'))
}

func allowedWorkflowExtraArg(arg string) bool {
	switch arg {
	case "--dry-run", "--skip-generation", "--skip-quality-review", "--skip-repair", "--skip-title-rename", "--skip-mockup", "--skip-final-main-image", "--skip-final-main-image-review":
		return true
	default:
		return false
	}
}

func joinShellArgs(args []string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, shellQuote(arg))
	}
	return strings.Join(escaped, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
