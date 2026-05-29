package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func CreatePDDManualEdit(runID string, request model.PDDManualEditRequest) (model.PDDManualEditResult, error) {
	ctx, run, data, node, nodeDir, err := manualEditContext(runID, request.ProductKey, request.NodeID)
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	if data.runItem().Status == model.PDDRunStatusRunning {
		return model.PDDManualEditResult{}, safeMessageError{message: "当前 run 仍在运行，完成或停止后才能人工编辑副本"}
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return model.PDDManualEditResult{}, safeMessageError{message: "人工编辑副本需要填写 prompt"}
	}
	sourcePath, err := ResolvePDDRunFile(runID, request.ArtifactPath)
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	if !isImagePath(sourcePath) {
		return model.PDDManualEditResult{}, safeMessageError{message: "人工编辑副本第一版只支持图片产物"}
	}
	nowTime := time.Now()
	editID := fmt.Sprintf("edit_%s_%09d", nowTime.Format("20060102_150405"), nowTime.Nanosecond())
	editDir := filepath.Join(nodeDir, "manual_edits", editID)
	if err := os.MkdirAll(editDir, 0755); err != nil {
		return model.PDDManualEditResult{}, err
	}
	editNode := node
	editNode.Operation = "image_edit"
	editNode.Prompt = prompt
	editNode.Model = firstString(strings.TrimSpace(request.Model), strings.TrimSpace(node.Model), "gpt-image-2")
	editNode.Size = firstString(strings.TrimSpace(request.Size), strings.TrimSpace(node.Size), "1:1")
	editNode.Quality = firstString(strings.TrimSpace(request.Quality), strings.TrimSpace(node.Quality), "high")
	editNode.Count = normalizeWorkflowImageCount(request.Count)
	refs := []workflowNodeOutputFile{{Path: sourcePath, Kind: "image", MimeType: firstString(mimeFromPath(sourcePath), "image/png")}}
	maskPath, err := manualEditMaskPath(runID, editDir, request)
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	var images [][]byte
	if maskPath != "" {
		images, err = requestWorkflowImageEditWithMask(editNode, prompt, refs, workflowNodeOutputFile{Path: maskPath, Kind: "image", MimeType: firstString(mimeFromPath(maskPath), "image/png")}, editNode.Count)
	} else {
		images, err = requestWorkflowImageEdit(editNode, prompt, refs, editNode.Count)
	}
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	output := workflowNodeOutput{NodeID: node.ID, Type: "image"}
	for index, body := range images {
		if index >= editNode.Count {
			break
		}
		outputPath := filepath.Join(editDir, fmt.Sprintf("output_%02d.png", index+1))
		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			return model.PDDManualEditResult{}, err
		}
		output.Files = append(output.Files, workflowNodeOutputFile{Path: outputPath, Kind: "image", MimeType: "image/png"})
	}
	if len(output.Files) == 0 {
		return model.PDDManualEditResult{}, safeMessageError{message: "图片模型没有返回人工编辑副本"}
	}
	metadata := map[string]any{
		"edit_id":     editID,
		"run_id":      run.ID,
		"product_key": ctx.ProductKey,
		"node_id":     node.ID,
		"source_path": relativeOrAbsolute(run.RunDir, sourcePath),
		"mask_path":   relativeOrAbsolute(run.RunDir, maskPath),
		"prompt":      prompt,
		"model":       editNode.Model,
		"count":       editNode.Count,
		"size":        editNode.Size,
		"quality":     editNode.Quality,
		"created_at":  now(),
		"output":      output,
	}
	if err := writeJSONFile(filepath.Join(editDir, "edit.json"), metadata); err != nil {
		return model.PDDManualEditResult{}, err
	}
	result := model.PDDManualEditResult{
		EditID:     editID,
		ProductKey: ctx.ProductKey,
		NodeID:     node.ID,
		Artifacts:  manualEditArtifacts(run.ID, run.RunDir, editID, output.Files),
	}
	if request.Apply {
		if err := applyManualEditOutput(ctx, run, data.customSpec.Nodes, data.customSpec.Edges, node, nodeDir, editID, output, request.RerunDownstream); err != nil {
			return result, err
		}
		result.Applied = true
		result.RerunDownstream = request.RerunDownstream
		result.Output = "人工编辑副本已应用"
		if request.RerunDownstream {
			result.Output = "人工编辑副本已应用，并已重跑后续节点"
		}
	}
	return result, nil
}

func manualEditMaskPath(runID string, editDir string, request model.PDDManualEditRequest) (string, error) {
	if strings.TrimSpace(request.MaskDataURL) != "" {
		body, err := imageURLBytes(strings.TrimSpace(request.MaskDataURL))
		if err != nil {
			return "", safeMessageError{message: "蒙版数据无法读取"}
		}
		maskPath := filepath.Join(editDir, "mask.png")
		if err := os.WriteFile(maskPath, body, 0644); err != nil {
			return "", err
		}
		return maskPath, nil
	}
	if strings.TrimSpace(request.MaskPath) == "" {
		return "", nil
	}
	maskPath, err := ResolvePDDRunFile(runID, request.MaskPath)
	if err != nil {
		return "", err
	}
	if !isImagePath(maskPath) {
		return "", safeMessageError{message: "蒙版必须是图片文件"}
	}
	return maskPath, nil
}

func ApplyPDDManualEdit(runID string, editID string, request model.PDDManualEditApplyRequest) (model.PDDManualEditResult, error) {
	ctx, run, data, node, nodeDir, err := manualEditContext(runID, request.ProductKey, request.NodeID)
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	editID = safeFileName(editID, "")
	if editID == "" {
		return model.PDDManualEditResult{}, safeMessageError{message: "缺少人工编辑副本 ID"}
	}
	editDir := filepath.Join(nodeDir, "manual_edits", editID)
	files := imageFiles(editDir)
	if len(files) == 0 {
		return model.PDDManualEditResult{}, safeMessageError{message: "未找到人工编辑副本输出"}
	}
	output := workflowNodeOutput{NodeID: node.ID, Type: "image"}
	for _, file := range files {
		output.Files = append(output.Files, workflowNodeOutputFile{Path: file, Kind: "image", MimeType: mimeFromPath(file)})
	}
	if err := applyManualEditOutput(ctx, run, data.customSpec.Nodes, data.customSpec.Edges, node, nodeDir, editID, output, request.RerunDownstream); err != nil {
		return model.PDDManualEditResult{}, err
	}
	return model.PDDManualEditResult{
		EditID:          editID,
		ProductKey:      ctx.ProductKey,
		NodeID:          node.ID,
		Artifacts:       manualEditArtifacts(run.ID, run.RunDir, editID, output.Files),
		Applied:         true,
		RerunDownstream: request.RerunDownstream,
		Output:          "人工编辑副本已应用",
	}, nil
}

func ApplyPDDCreativeCanvasOutput(runID string, request model.PDDCreativeCanvasApplyRequest) (model.PDDManualEditResult, error) {
	targetNodeID := strings.TrimSpace(request.TargetNodeID)
	if targetNodeID == "" {
		return model.PDDManualEditResult{}, safeMessageError{message: "缺少目标工作流节点"}
	}
	ctx, run, data, node, nodeDir, err := manualEditContext(runID, request.ProductKey, targetNodeID)
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	if data.runItem().Status == model.PDDRunStatusRunning {
		return model.PDDManualEditResult{}, safeMessageError{message: "当前 run 仍在运行，完成或停止后才能应用创作副本"}
	}
	nowTime := time.Now()
	editID := fmt.Sprintf("creative_%s_%09d", nowTime.Format("20060102_150405"), nowTime.Nanosecond())
	editDir := filepath.Join(nodeDir, "manual_edits", editID)
	if err := os.MkdirAll(editDir, 0755); err != nil {
		return model.PDDManualEditResult{}, err
	}
	output, err := creativeCanvasApplyOutput(runID, editDir, node.ID, request)
	if err != nil {
		return model.PDDManualEditResult{}, err
	}
	metadata := map[string]any{
		"edit_id":            editID,
		"run_id":             run.ID,
		"product_key":        ctx.ProductKey,
		"node_id":            node.ID,
		"source_node_id":     strings.TrimSpace(request.SourceNodeID),
		"source_path":        request.ArtifactPath,
		"mime_type":          request.MimeType,
		"created_at":         now(),
		"creative_canvas":    true,
		"rerun_downstream":   request.RerunDownstream,
		"output":             output,
		"origin_workflow_id": node.ID,
	}
	if err := writeJSONFile(filepath.Join(editDir, "edit.json"), metadata); err != nil {
		return model.PDDManualEditResult{}, err
	}
	if err := applyManualEditOutput(ctx, run, data.customSpec.Nodes, data.customSpec.Edges, node, nodeDir, editID, output, request.RerunDownstream); err != nil {
		return model.PDDManualEditResult{}, err
	}
	return model.PDDManualEditResult{
		EditID:          editID,
		ProductKey:      ctx.ProductKey,
		NodeID:          node.ID,
		Artifacts:       manualEditArtifacts(run.ID, run.RunDir, editID, output.Files),
		Applied:         true,
		RerunDownstream: request.RerunDownstream,
		Output:          "创作副本已应用",
	}, nil
}

func creativeCanvasApplyOutput(runID string, editDir string, nodeID string, request model.PDDCreativeCanvasApplyRequest) (workflowNodeOutput, error) {
	mimeType := strings.TrimSpace(request.MimeType)
	if strings.TrimSpace(request.ArtifactPath) != "" {
		sourcePath, err := ResolvePDDRunFile(runID, request.ArtifactPath)
		if err != nil {
			return workflowNodeOutput{}, err
		}
		mimeType = firstString(mimeType, mimeFromPath(sourcePath))
		kind := workflowFileKind(sourcePath, mimeType)
		targetPath := filepath.Join(editDir, "output_01"+firstString(filepath.Ext(sourcePath), creativeAssetExt(mimeType, filepath.Base(sourcePath))))
		if err := copyFile(sourcePath, targetPath); err != nil {
			return workflowNodeOutput{}, err
		}
		return workflowNodeOutput{NodeID: nodeID, Type: kind, Files: []workflowNodeOutputFile{{Path: targetPath, Kind: kind, MimeType: mimeType}}}, nil
	}
	content := strings.TrimSpace(request.Content)
	if content == "" {
		return workflowNodeOutput{}, safeMessageError{message: "创作副本缺少可应用的产物"}
	}
	var (
		body []byte
		err  error
	)
	if strings.HasPrefix(content, "data:") || !strings.HasPrefix(strings.ToLower(mimeType), "text/") {
		body, mimeType, err = decodeCreativeAssetContent(content, mimeType)
		if err != nil {
			return workflowNodeOutput{}, err
		}
	} else {
		body = []byte(content)
		mimeType = firstString(mimeType, "text/plain; charset=utf-8")
	}
	kind := workflowFileKind("", mimeType)
	targetPath := filepath.Join(editDir, "output_01"+creativeAssetExt(mimeType, "output"))
	if err := os.WriteFile(targetPath, body, 0644); err != nil {
		return workflowNodeOutput{}, err
	}
	output := workflowNodeOutput{NodeID: nodeID, Type: kind, Files: []workflowNodeOutputFile{{Path: targetPath, Kind: kind, MimeType: mimeType}}}
	if kind == "text" {
		output.Text = string(body)
	}
	return output, nil
}

func workflowFileKind(path string, mimeType string) string {
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") || isImagePath(path) {
		return "image"
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "video/") {
		return "video"
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "text/") || strings.EqualFold(filepath.Ext(path), ".txt") {
		return "text"
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "application/json") || strings.EqualFold(filepath.Ext(path), ".json") {
		return "json"
	}
	return "file"
}

func manualEditContext(runID string, productKey string, nodeID string) (workflowProductContext, model.WorkflowRun, pddRunData, model.WorkflowTemplateNode, string, error) {
	data, err := loadPDDRun(runID)
	if err != nil {
		return workflowProductContext{}, model.WorkflowRun{}, pddRunData{}, model.WorkflowTemplateNode{}, "", err
	}
	if !data.customWorkflow {
		return workflowProductContext{}, model.WorkflowRun{}, pddRunData{}, model.WorkflowTemplateNode{}, "", safeMessageError{message: "人工编辑副本只支持画布模板 run"}
	}
	product, ok := data.findProduct(productKey)
	if !ok {
		return workflowProductContext{}, model.WorkflowRun{}, pddRunData{}, model.WorkflowTemplateNode{}, "", safeMessageError{message: "未找到商品"}
	}
	var node model.WorkflowTemplateNode
	for _, item := range data.customSpec.Nodes {
		if item.ID == nodeID {
			node = item
			break
		}
	}
	if node.ID == "" {
		return workflowProductContext{}, model.WorkflowRun{}, pddRunData{}, model.WorkflowTemplateNode{}, "", safeMessageError{message: "未找到节点"}
	}
	productDir := data.customProductNodeDir(product.SourceProduct)
	if productDir == "" {
		return workflowProductContext{}, model.WorkflowRun{}, pddRunData{}, model.WorkflowTemplateNode{}, "", safeMessageError{message: "未找到商品节点目录"}
	}
	inputIndex := productIndexFromCustomDir(productDir)
	inputs := data.customInputs()
	if inputIndex <= 0 || inputIndex > len(inputs) {
		return workflowProductContext{}, model.WorkflowRun{}, pddRunData{}, model.WorkflowTemplateNode{}, "", safeMessageError{message: "未找到商品输入数据"}
	}
	run, err := repository.GetWorkflowRun(runID)
	if err != nil {
		run = model.WorkflowRun{ID: data.runID(), WorkflowType: customWorkflowTypePDD, RunDir: data.runDir, SpecSnapshot: data.customSpec}
	}
	run.RunDir = data.runDir
	if len(run.SpecSnapshot.Nodes) == 0 {
		run.SpecSnapshot = data.customSpec
	}
	input := map[string]any{}
	for key, value := range inputs[inputIndex-1] {
		input[key] = value
	}
	ctx := workflowProductContext{
		Run:            run,
		Input:          input,
		InputIndex:     inputIndex,
		ProductKey:     product.SourceProduct,
		ProductTitle:   productTitleFromInput(input, inputIndex),
		ProductDirName: safeFileName(productTitleFromInput(input, inputIndex), product.SourceProduct),
		ProductLogDir:  filepath.Join(run.RunDir, "logs", "product_pipeline", product.SourceProduct),
		Outputs:        map[string]workflowNodeOutput{},
	}
	nodeDir := filepath.Join(productDir, "nodes", safeFileName(node.ID, "node"))
	return ctx, run, data, node, nodeDir, nil
}

func productIndexFromCustomDir(productDir string) int {
	name := filepath.Base(productDir)
	if len(name) < 3 {
		return 0
	}
	index, _ := strconv.Atoi(name[:3])
	return index
}

func applyManualEditOutput(ctx workflowProductContext, run model.WorkflowRun, nodes []model.WorkflowTemplateNode, edges []model.WorkflowTemplateEdge, node model.WorkflowTemplateNode, nodeDir string, editID string, output workflowNodeOutput, rerunDownstream bool) error {
	started := now()
	if statusPath := filepath.Join(nodeDir, "status.json"); exists(statusPath) {
		_ = copyFile(statusPath, filepath.Join(nodeDir, "manual_edits", editID, "status_before_apply.json"))
	}
	if err := applyWorkflowOutputMappings(ctx, node, output); err != nil {
		return err
	}
	finishedAt := now()
	parsedStartedAt, err := time.Parse(time.RFC3339, started)
	if err != nil {
		parsedStartedAt = time.Now()
	}
	payload := nodeStatusPayload(node, "completed", started, finishedAt, output, "", parsedStartedAt)
	payload["manual_override"] = map[string]any{"edit_id": editID, "applied_at": now()}
	if err := writeJSONFile(filepath.Join(nodeDir, "manual_override.json"), payload["manual_override"]); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(nodeDir, "status.json"), payload); err != nil {
		return err
	}
	appendCustomWorkflowLog(run, "manual edit applied product=%s node=%s edit=%s rerun_downstream=%v", ctx.ProductKey, node.ID, editID, rerunDownstream)
	if !rerunDownstream {
		_ = writeCustomProductSummary(run)
		return nil
	}
	run.Status = model.WorkflowRunStatusRunning
	run.UpdatedAt = now()
	_, _ = repository.SaveWorkflowRun(run)
	_ = writeCustomManifest(run, false, "")
	_ = writeCustomWorkflowStatus(run, "running", []workflowRunStage{{Name: "manual_downstream_rerun", Status: "running", StartedAt: started}}, "")
	_ = writePipelineStatus(ctx, "running", started, "", "")
	err = rerunWorkflowDownstream(ctx, nodes, edges, node.ID, output, workflowMaxRetries(run))
	finished := now()
	stage := workflowRunStage{Name: "manual_downstream_rerun", Status: "completed", StartedAt: started, FinishedAt: finished}
	if err != nil {
		stage.Status = "failed"
		stage.Error = err.Error()
		run.Status = model.WorkflowRunStatusError
		run.Error = err.Error()
		run.UpdatedAt = finished
		_, _ = repository.SaveWorkflowRun(run)
		_ = writePipelineStatus(ctx, "failed", started, finished, err.Error())
		_ = writeCustomManifest(run, false, err.Error())
		_ = writeCustomWorkflowStatus(run, "failed", []workflowRunStage{stage}, err.Error())
		_ = writeCustomProductSummary(run)
		return err
	}
	run.Status = model.WorkflowRunStatusSuccess
	run.Error = ""
	run.UpdatedAt = finished
	_, _ = repository.SaveWorkflowRun(run)
	_ = writePipelineStatus(ctx, "completed", started, finished, "")
	_ = writeCustomManifest(run, true, "")
	_ = writeCustomWorkflowStatus(run, "completed", []workflowRunStage{stage}, "")
	_ = writeCustomProductSummary(run)
	appendCustomWorkflowLog(run, "manual downstream rerun completed product=%s node=%s edit=%s", ctx.ProductKey, node.ID, editID)
	return nil
}

func workflowMaxRetries(run model.WorkflowRun) int {
	if run.SpecSnapshot.Settings.MaxRetries > 0 {
		return run.SpecSnapshot.Settings.MaxRetries
	}
	return 3
}

func rerunWorkflowDownstream(ctx workflowProductContext, nodes []model.WorkflowTemplateNode, edges []model.WorkflowTemplateEdge, seedNodeID string, seedOutput workflowNodeOutput, maxRetries int) error {
	relevant := downstreamNodeIDs(seedNodeID, edges)
	if len(relevant) == 0 {
		ctx.Outputs[seedNodeID] = seedOutput
		return nil
	}
	for _, node := range nodes {
		if output, ok := readWorkflowNodeOutput(ctx, node.ID); ok {
			ctx.Outputs[node.ID] = output
		}
	}
	ctx.Outputs[seedNodeID] = seedOutput
	nodeByID := map[string]model.WorkflowTemplateNode{}
	remainingIncoming := map[string]int{}
	outgoing := map[string][]model.WorkflowTemplateEdge{}
	for _, node := range nodes {
		nodeByID[node.ID] = node
		if relevant[node.ID] {
			remainingIncoming[node.ID] = 0
		}
	}
	for _, edge := range edges {
		if workflowEdgeLoopEnabled(edge) {
			outgoing[edge.From] = append(outgoing[edge.From], edge)
			continue
		}
		if relevant[edge.To] && (relevant[edge.From] || edge.From == seedNodeID) {
			remainingIncoming[edge.To]++
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge)
	}
	queue := []string{seedNodeID}
	queued := map[string]bool{seedNodeID: true}
	active := map[string]bool{seedNodeID: true}
	skipped := map[string]bool{}
	blocked := map[string]bool{}
	executed := map[string]int{}
	loopCounts := map[string]int{}
	var skipNode func(string)
	resolveEdge := func(edge model.WorkflowTemplateEdge, follow bool, rerun bool) {
		if !relevant[edge.To] {
			return
		}
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
		var output workflowNodeOutput
		var err error
		if nodeID == seedNodeID {
			output = seedOutput
		} else {
			node, ok := nodeByID[nodeID]
			if !ok {
				continue
			}
			output, err = executeWorkflowNode(ctx, node, edges, maxRetries)
			if err != nil {
				return fmt.Errorf("node=%s: %w", node.ID, err)
			}
			ctx.Outputs[node.ID] = output
		}
		executed[nodeID]++
		rerun := executed[nodeID] > 1
		for _, edge := range outgoing[nodeID] {
			if workflowEdgeLoopEnabled(edge) {
				if !relevant[edge.To] || !workflowEdgeShouldFollow(edge, output) {
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

func downstreamNodeIDs(seedNodeID string, edges []model.WorkflowTemplateEdge) map[string]bool {
	next := map[string][]string{}
	for _, edge := range edges {
		if workflowEdgeLoopEnabled(edge) {
			continue
		}
		next[edge.From] = append(next[edge.From], edge.To)
	}
	result := map[string]bool{}
	queue := append([]string{}, next[seedNodeID]...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if result[id] {
			continue
		}
		result[id] = true
		queue = append(queue, next[id]...)
	}
	return result
}

func readWorkflowNodeOutput(ctx workflowProductContext, nodeID string) (workflowNodeOutput, bool) {
	path := filepath.Join(customWorkflowWriteDir(ctx.Run.RunDir), "products", fmt.Sprintf("%03d_%s", ctx.InputIndex, ctx.ProductKey), "nodes", safeFileName(nodeID, "node"), "status.json")
	payload := readJSONMap(path)
	output := mapValue(payload, "output")
	if output == nil {
		return workflowNodeOutput{}, false
	}
	result := workflowNodeOutput{
		NodeID: firstString(anyToString(output["nodeId"]), nodeID),
		Type:   anyToString(output["type"]),
		Text:   anyToString(output["text"]),
		Files:  customOutputFiles(payload),
	}
	return result, result.Text != "" || len(result.Files) > 0
}

func manualEditArtifacts(runID string, runDir string, editID string, files []workflowNodeOutputFile) []model.PDDArtifact {
	result := []model.PDDArtifact{}
	for index, file := range files {
		rel := relativeOrAbsolute(runDir, file.Path)
		kind := firstString(file.Kind, workflowFileKind(file.Path, file.MimeType))
		result = append(result, model.PDDArtifact{
			ID:       fmt.Sprintf("%s-%03d", editID, index+1),
			Title:    filepath.Base(file.Path),
			Path:     rel,
			URL:      runFileURL(runID, rel),
			Kind:     kind,
			MimeType: firstString(file.MimeType, mimeFromPath(file.Path)),
		})
	}
	return result
}
