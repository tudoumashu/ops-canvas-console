package localworkspace

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type localPDDRunOverview struct {
	Run          localPDDRunItem          `json:"run"`
	Stages       []localPDDStageNode      `json:"stages"`
	Edges        []localPDDGraphEdge      `json:"edges"`
	Products     []localPDDProductSummary `json:"products"`
	RecentErrors []string                 `json:"recentErrors"`
}

type localPDDRunItem struct {
	RunID             string `json:"runId"`
	Status            string `json:"status"`
	RunDir            string `json:"runDir"`
	UpdatedAt         string `json:"updatedAt"`
	CustomWorkflow    bool   `json:"customWorkflow"`
	StartedAt         string `json:"startedAt,omitempty"`
	FinishedAt        string `json:"finishedAt,omitempty"`
	Completed         bool   `json:"completed"`
	HasLogs           bool   `json:"hasLogs"`
	ProductTotal      int    `json:"productTotal"`
	CompletedProducts int    `json:"completedProducts"`
	FailedProducts    int    `json:"failedProducts"`
	RunningProducts   int    `json:"runningProducts"`
	RecentError       string `json:"recentError,omitempty"`
}

type localPDDStageNode struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Type            string  `json:"type,omitempty"`
	Status          string  `json:"status"`
	Total           int     `json:"total"`
	Success         int     `json:"success"`
	Failed          int     `json:"failed"`
	Running         int     `json:"running"`
	Idle            int     `json:"idle"`
	Skipped         int     `json:"skipped"`
	X               float64 `json:"x,omitempty"`
	Y               float64 `json:"y,omitempty"`
	Width           float64 `json:"width,omitempty"`
	Height          float64 `json:"height,omitempty"`
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
	RecentError     string  `json:"recentError,omitempty"`
}

type localPDDGraphEdge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type localPDDProductSummary struct {
	Key              string `json:"key"`
	SourceProduct    string `json:"sourceProduct"`
	GeneratedProduct string `json:"generatedProduct,omitempty"`
	Product          string `json:"product"`
	ThemeName        string `json:"themeName"`
	Status           string `json:"status"`
	RawStatus        string `json:"rawStatus"`
	StartedAt        string `json:"startedAt,omitempty"`
	FinishedAt       string `json:"finishedAt,omitempty"`
	Error            string `json:"error,omitempty"`
	GeneratedImages  int    `json:"generatedImages"`
	SpecImages       int    `json:"specImages"`
	MainImages       int    `json:"mainImages"`
	ArtifactCount    int    `json:"artifactCount,omitempty"`
}

type localPDDProductDetail struct {
	RunID   string                 `json:"runId"`
	Product localPDDProductSummary `json:"product"`
	Nodes   []localPDDGraphNode    `json:"nodes"`
	Edges   []localPDDGraphEdge    `json:"edges"`
	Files   []localPDDDetailFile   `json:"files"`
}

type localPDDGraphNode struct {
	ID              string               `json:"id"`
	Type            string               `json:"type"`
	Title           string               `json:"title"`
	Status          string               `json:"status"`
	X               float64              `json:"x"`
	Y               float64              `json:"y"`
	Width           float64              `json:"width"`
	Height          float64              `json:"height"`
	Summary         string               `json:"summary,omitempty"`
	Config          map[string]any       `json:"config,omitempty"`
	DurationSeconds float64              `json:"durationSeconds,omitempty"`
	Artifacts       []localPDDArtifact   `json:"artifacts,omitempty"`
	Files           []localPDDDetailFile `json:"files,omitempty"`
}

type localPDDArtifact struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	MIMEType string `json:"mimeType,omitempty"`
}

type localPDDDetailFile struct {
	Title string `json:"title"`
	Path  string `json:"path"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

type localPDDCreativeCanvas struct {
	RunID          string                       `json:"runId"`
	ProductKey     string                       `json:"productKey"`
	Product        localPDDProductSummary       `json:"product"`
	Nodes          []localPDDCreativeCanvasNode `json:"nodes"`
	Edges          []localPDDCreativeCanvasEdge `json:"edges"`
	Viewport       map[string]float64           `json:"viewport,omitempty"`
	BackgroundMode string                       `json:"backgroundMode,omitempty"`
	ShowImageInfo  bool                         `json:"showImageInfo"`
	Saved          bool                         `json:"saved"`
	UpdatedAt      string                       `json:"updatedAt,omitempty"`
}

type localPDDCreativeCanvasNode struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Title    string             `json:"title"`
	Position map[string]float64 `json:"position"`
	Width    float64            `json:"width"`
	Height   float64            `json:"height"`
	Metadata map[string]any     `json:"metadata,omitempty"`
}

type localPDDCreativeCanvasEdge struct {
	ID         string `json:"id"`
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
}

func buildLocalPDDRunOverview(workspace Workspace, runID string, baseURL string) (localPDDRunOverview, error) {
	run, template, nodes, edges, products, states, refs, err := localPDDRunContext(workspace, runID)
	if err != nil {
		return localPDDRunOverview{}, err
	}
	_ = baseURL
	productSummaries := make([]localPDDProductSummary, 0, len(products))
	for _, product := range products {
		productSummaries = append(productSummaries, localPDDProductStatus(product, nodes, states, refs, len(products) == 1))
	}
	stages := make([]localPDDStageNode, 0, len(nodes))
	for index, node := range nodes {
		stages = append(stages, localPDDStageFromNode(node, states[node.ID], len(products), index))
	}
	runItem := localPDDRunItem{
		RunID:             run.ID,
		Status:            localPDDRunStatus(run.Data.Status),
		RunDir:            "local-workspace",
		UpdatedAt:         run.UpdatedAt,
		CustomWorkflow:    true,
		Completed:         run.Data.Status == RunStatusSuccess,
		HasLogs:           true,
		ProductTotal:      len(products),
		CompletedProducts: countLocalPDDProducts(productSummaries, "success"),
		FailedProducts:    countLocalPDDProducts(productSummaries, "error"),
		RunningProducts:   countLocalPDDProducts(productSummaries, "running"),
	}
	for _, stage := range stages {
		if runItem.StartedAt == "" || (stage.Status != "idle" && stage.ID != "" && states[stage.ID].Data.StartedAt < runItem.StartedAt) {
			runItem.StartedAt = states[stage.ID].Data.StartedAt
		}
		if states[stage.ID].Data.FinishedAt > runItem.FinishedAt {
			runItem.FinishedAt = states[stage.ID].Data.FinishedAt
		}
		if runItem.RecentError == "" && stage.RecentError != "" {
			runItem.RecentError = stage.RecentError
		}
	}
	_ = template
	return localPDDRunOverview{
		Run:          runItem,
		Stages:       stages,
		Edges:        localPDDGraphEdges(edges),
		Products:     productSummaries,
		RecentErrors: localPDDRecentErrors(states),
	}, nil
}

func buildLocalPDDProductDetail(workspace Workspace, runID string, productKey string, baseURL string) (localPDDProductDetail, error) {
	_, _, nodes, edges, products, states, refs, err := localPDDRunContext(workspace, runID)
	if err != nil {
		return localPDDProductDetail{}, err
	}
	product, ok := localPDDFindProduct(products, productKey)
	if !ok {
		return localPDDProductDetail{}, NewError(ErrorWorkspaceNotFound, "product not found", 2, nil)
	}
	summary := localPDDProductStatus(product, nodes, states, refs, len(products) == 1)
	graphNodes := make([]localPDDGraphNode, 0, len(nodes))
	files := []localPDDDetailFile{}
	for index, node := range nodes {
		state := states[productScopedNodeID(product.Key, node.ID)]
		if state.ID == "" && len(products) == 1 {
			state = states[node.ID]
		}
		artifacts := localPDDArtifactsForNode(refs, product.Key, node.ID, baseURL, len(products) == 1)
		nodeFiles := localPDDDetailFilesFromArtifacts(artifacts)
		files = append(files, nodeFiles...)
		graphNodes = append(graphNodes, localPDDGraphNode{
			ID:              node.ID,
			Type:            firstNonEmptyString(node.Operation, node.Type),
			Title:           firstNonEmptyString(node.Title, node.ID),
			Status:          localPDDRunStatus(state.Data.Status),
			X:               localPDDNodeX(node, index),
			Y:               localPDDNodeY(node),
			Width:           localPDDNodeWidth(node),
			Height:          localPDDNodeHeight(node),
			Summary:         localPDDNodeSummary(node, state, artifacts),
			Config:          localPDDNodeConfig(node),
			DurationSeconds: localPDDDurationSeconds(state),
			Artifacts:       artifacts,
			Files:           nodeFiles,
		})
	}
	return localPDDProductDetail{
		RunID:   runID,
		Product: summary,
		Nodes:   graphNodes,
		Edges:   localPDDGraphEdges(edges),
		Files:   files,
	}, nil
}

func localPDDRunContext(workspace Workspace, runID string) (Envelope[RunData], Envelope[TemplateData], []executorNode, []executorEdge, []executorProductInput, map[string]Envelope[RunNodeStateData], []RunArtifactSummary, error) {
	run, err := ReadRun(workspace, runID)
	if err != nil {
		return Envelope[RunData]{}, Envelope[TemplateData]{}, nil, nil, nil, nil, nil, err
	}
	template, err := readExecutorTemplate(workspace, run)
	if err != nil {
		return run, Envelope[TemplateData]{}, nil, nil, nil, nil, nil, err
	}
	nodes, err := parseExecutorNodes(template.Data.Nodes)
	if err != nil {
		return run, template, nil, nil, nil, nil, nil, err
	}
	edges, err := parseExecutorEdges(template.Data.Edges)
	if err != nil {
		return run, template, nil, nil, nil, nil, nil, err
	}
	products := executorRunProducts(run.Data.Input)
	states, err := executorNodeStateMap(workspace, runID)
	if err != nil {
		return run, template, nil, nil, nil, nil, nil, err
	}
	refs, err := ListRunArtifacts(workspace, runID)
	if err != nil {
		return run, template, nil, nil, nil, nil, nil, err
	}
	return run, template, nodes, edges, products, states, refs, nil
}

func localPDDProductStatus(product executorProductInput, nodes []executorNode, states map[string]Envelope[RunNodeStateData], refs []RunArtifactSummary, includeUnscopedRefs bool) localPDDProductSummary {
	status := "idle"
	errorMessage := ""
	startedAt := ""
	finishedAt := ""
	success := 0
	for _, node := range nodes {
		state := states[productScopedNodeID(product.Key, node.ID)]
		if state.ID == "" {
			state = states[node.ID]
		}
		mapped := localPDDRunStatus(state.Data.Status)
		switch mapped {
		case "error":
			status = "error"
			if errorMessage == "" {
				errorMessage = state.Data.Error
			}
		case "running":
			if status != "error" {
				status = "running"
			}
		case "success":
			success++
			if status == "idle" {
				status = "success"
			}
		}
		if startedAt == "" || (state.Data.StartedAt != "" && state.Data.StartedAt < startedAt) {
			startedAt = state.Data.StartedAt
		}
		if state.Data.FinishedAt > finishedAt {
			finishedAt = state.Data.FinishedAt
		}
	}
	if success < len(nodes) && status == "success" {
		status = "running"
	}
	artifactCount := 0
	generated := 0
	spec := 0
	main := 0
	for _, ref := range refs {
		if !localPDDRefMatchesProduct(ref.Ref, product.Key, includeUnscopedRefs) {
			continue
		}
		artifactCount++
		if ref.Artifact.Type == "image" {
			generated++
		}
		switch refTemplateNodeID(ref.Ref) {
		case "mockup":
			spec++
		case "main":
			main++
		}
	}
	productTitle := firstNonEmptyString(stringFromMap(product.Input, "productTitle"), product.SourceProduct)
	return localPDDProductSummary{
		Key:             product.Key,
		SourceProduct:   product.SourceProduct,
		Product:         productTitle,
		ThemeName:       firstNonEmptyString(stringFromMap(product.Input, "theme"), stringFromMap(product.Input, "animeIP"), stringFromMap(product.Input, "work")),
		Status:          status,
		RawStatus:       status,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		Error:           errorMessage,
		GeneratedImages: generated,
		SpecImages:      spec,
		MainImages:      main,
		ArtifactCount:   artifactCount,
	}
}

func localPDDStageFromNode(node executorNode, state Envelope[RunNodeStateData], productTotal int, index int) localPDDStageNode {
	output := state.Data.Output
	total := intFromOutput(output, "total", productTotal)
	success := intFromOutput(output, "success", 0)
	failed := intFromOutput(output, "failed", 0)
	running := intFromOutput(output, "running", 0)
	skipped := intFromOutput(output, "skipped", 0)
	idle := max(0, total-success-failed-running)
	return localPDDStageNode{
		ID:              node.ID,
		Title:           firstNonEmptyString(node.Title, node.ID),
		Type:            firstNonEmptyString(node.Operation, node.Type),
		Status:          localPDDRunStatus(state.Data.Status),
		Total:           total,
		Success:         success,
		Failed:          failed,
		Running:         running,
		Idle:            idle,
		Skipped:         skipped,
		X:               localPDDNodeX(node, index),
		Y:               localPDDNodeY(node),
		Width:           localPDDNodeWidth(node),
		Height:          localPDDNodeHeight(node),
		DurationSeconds: localPDDDurationSeconds(state),
		RecentError:     state.Data.Error,
	}
}

func localPDDGraphEdges(edges []executorEdge) []localPDDGraphEdge {
	out := make([]localPDDGraphEdge, 0, len(edges))
	for index, edge := range edges {
		out = append(out, localPDDGraphEdge{ID: firstNonEmptyString(edge.ID, fmt.Sprintf("%s-%s-%d", edge.From, edge.To, index)), From: edge.From, To: edge.To})
	}
	return out
}

func localPDDArtifactsForNode(refs []RunArtifactSummary, productKey string, nodeID string, baseURL string, includeUnscopedRefs bool) []localPDDArtifact {
	out := []localPDDArtifact{}
	for _, ref := range refs {
		if !localPDDRefMatchesProduct(ref.Ref, productKey, includeUnscopedRefs) {
			continue
		}
		if refTemplateNodeID(ref.Ref) != nodeID {
			continue
		}
		out = append(out, localPDDArtifact{
			ID:       ref.Artifact.ID,
			Title:    firstNonEmptyString(ref.Artifact.Title, ref.Artifact.ID),
			Path:     "artifact:" + ref.Artifact.ID,
			URL:      localPDDArtifactURL(baseURL, ref.Artifact.ID),
			Kind:     ref.Artifact.Type,
			MIMEType: ref.Artifact.MIME,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func localPDDRefMatchesProduct(ref RunArtifactRefData, productKey string, includeUnscopedRefs bool) bool {
	refProduct := refProductKey(ref)
	if refProduct == productKey {
		return true
	}
	return includeUnscopedRefs && refProduct == ""
}

func localPDDArtifactURL(baseURL string, artifactID string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return "/api/local/artifacts/" + url.PathEscape(artifactID) + "/files/original"
	}
	return baseURL + "/api/local/artifacts/" + url.PathEscape(artifactID) + "/files/original"
}

func localPDDDetailFilesFromArtifacts(artifacts []localPDDArtifact) []localPDDDetailFile {
	out := make([]localPDDDetailFile, 0, len(artifacts))
	for _, artifact := range artifacts {
		kind := "file"
		if strings.HasPrefix(artifact.MIMEType, "image/") || artifact.Kind == "image" {
			kind = "image"
		} else if strings.HasPrefix(artifact.MIMEType, "text/") || artifact.Kind == "text" {
			kind = "text"
		}
		out = append(out, localPDDDetailFile{Title: artifact.Title, Path: artifact.Path, URL: artifact.URL, Kind: kind})
	}
	return out
}

func refProductKey(ref RunArtifactRefData) string {
	if value := stringFromMap(ref.Metadata, "productKey"); value != "" {
		return value
	}
	if strings.HasPrefix(ref.NodeID, "product/") {
		parts := strings.Split(ref.NodeID, "/")
		if len(parts) >= 3 {
			return parts[1]
		}
	}
	return ""
}

func refTemplateNodeID(ref RunArtifactRefData) string {
	if value := stringFromMap(ref.Metadata, "templateNodeId"); value != "" {
		return value
	}
	if strings.HasPrefix(ref.NodeID, "product/") {
		parts := strings.Split(ref.NodeID, "/")
		if len(parts) >= 3 {
			return strings.Join(parts[2:], "/")
		}
	}
	return ref.NodeID
}

func localPDDRunStatus(status string) string {
	switch status {
	case RunStatusSuccess:
		return "success"
	case RunStatusError, RunStatusCanceled:
		return "error"
	case RunStatusRunning:
		return "running"
	default:
		return "idle"
	}
}

func localPDDFindProduct(products []executorProductInput, key string) (executorProductInput, bool) {
	for _, product := range products {
		if product.Key == key {
			return product, true
		}
	}
	if key == "" && len(products) > 0 {
		return products[0], true
	}
	return executorProductInput{}, false
}

func localPDDRecentErrors(states map[string]Envelope[RunNodeStateData]) []string {
	out := []string{}
	for _, state := range states {
		if strings.TrimSpace(state.Data.Error) != "" {
			out = append(out, state.Data.Error)
		}
	}
	sort.Strings(out)
	if len(out) > 12 {
		out = out[:12]
	}
	return uniqueStrings(out)
}

func countLocalPDDProducts(products []localPDDProductSummary, status string) int {
	count := 0
	for _, product := range products {
		if product.Status == status {
			count++
		}
	}
	return count
}

func localPDDNodeConfig(node executorNode) map[string]any {
	return map[string]any{
		"operation":      node.Operation,
		"model":          node.Model,
		"prompt":         node.Prompt,
		"count":          node.Count,
		"size":           node.Size,
		"quality":        node.Quality,
		"extra":          node.Extra,
		"outputMappings": node.OutputMappings,
	}
}

func localPDDNodeSummary(node executorNode, state Envelope[RunNodeStateData], artifacts []localPDDArtifact) string {
	if state.Data.Error != "" {
		return state.Data.Error
	}
	operation := firstNonEmptyString(node.Operation, node.Type)
	output := state.Data.Output
	if operation == "input" || node.Type == "text" {
		if input, ok := asStringAnyMap(output["input"]); ok && len(input) > 0 {
			return compactLocalPDDJSON(input, 96)
		}
	}
	if operation == "material_lookup" || node.Type == "material" {
		work := firstNonEmptyString(stringFromMap(output, "work"), stringFromMap(output, "theme"), stringFromMap(output, "animeIP"))
		character := stringFromMap(output, "character")
		if work != "" || character != "" {
			return strings.Join(localPDDNonEmptyStrings(work, character, "标准参考图"), " - ")
		}
		if len(artifacts) > 0 {
			return fmt.Sprintf("material_lookup - %d 个素材文件", len(artifacts))
		}
	}
	if len(artifacts) > 0 {
		return fmt.Sprintf("%s - %d 个输出文件", firstNonEmptyString(operation, "artifact"), len(artifacts))
	}
	if outputs := anySliceFromMap(output, "projectOutputs"); len(outputs) > 0 {
		return fmt.Sprintf("%s - %d 个项目输出", firstNonEmptyString(operation, "script"), len(outputs))
	}
	if packageRoot := stringFromMap(output, "packageRoot"); packageRoot != "" {
		return fmt.Sprintf("%s - %s", firstNonEmptyString(operation, "script"), packageRoot)
	}
	if text := stringFromMap(state.Data.Output, "text"); text != "" {
		if len([]rune(text)) > 96 {
			return string([]rune(text)[:96]) + "..."
		}
		return text
	}
	return state.Data.Status
}

func localPDDNonEmptyStrings(values ...string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func compactLocalPDDJSON(value any, limit int) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	text := string(data)
	if limit > 0 && len([]rune(text)) > limit {
		return string([]rune(text)[:limit]) + "..."
	}
	return text
}

func localPDDNodeX(node executorNode, index int) float64 {
	if node.X != 0 {
		return node.X
	}
	return float64(index * 285)
}

func localPDDNodeY(node executorNode) float64 {
	if node.Y != 0 {
		return node.Y
	}
	return 40
}

func localPDDNodeWidth(node executorNode) float64 {
	if node.Width > 0 {
		return node.Width
	}
	return 230
}

func localPDDNodeHeight(node executorNode) float64 {
	if node.Height > 0 {
		return node.Height
	}
	return 150
}

func localPDDDurationSeconds(state Envelope[RunNodeStateData]) float64 {
	if state.Data.StartedAt == "" || state.Data.FinishedAt == "" {
		return 0
	}
	started, err := time.Parse(time.RFC3339, state.Data.StartedAt)
	if err != nil {
		return 0
	}
	finished, err := time.Parse(time.RFC3339, state.Data.FinishedAt)
	if err != nil || finished.Before(started) {
		return 0
	}
	return finished.Sub(started).Seconds()
}

func intFromOutput(output map[string]any, key string, fallback int) int {
	if output == nil {
		return fallback
	}
	if parsed := positiveInt(output[key]); parsed > 0 {
		return parsed
	}
	return fallback
}

func localPDDCreativeCanvasPath(workspace Workspace, runID string, productKey string) string {
	return filepath.Join(workspace.Root, "runs", runID, "creative_canvas", safeProductKey(productKey)+".canvas.json")
}

func loadLocalPDDCreativeCanvas(workspace Workspace, runID string, productKey string, baseURL string) (localPDDCreativeCanvas, error) {
	filePath := localPDDCreativeCanvasPath(workspace, runID, productKey)
	detail, err := buildLocalPDDProductDetail(workspace, runID, productKey, baseURL)
	if err != nil {
		return localPDDCreativeCanvas{}, err
	}
	if document, err := readEnvelopeFile[localPDDCreativeCanvas](filePath); err == nil {
		document.Data.Saved = true
		return normalizeLocalPDDCreativeCanvas(workspace, document.Data, detail), nil
	}
	return newLocalPDDCreativeCanvas(workspace, runID, productKey, detail), nil
}

func newLocalPDDCreativeCanvas(workspace Workspace, runID string, productKey string, detail localPDDProductDetail) localPDDCreativeCanvas {
	nodes := []localPDDCreativeCanvasNode{}
	firstByWorkflowNode := map[string]string{}
	for _, graphNode := range detail.Nodes {
		added := localPDDCreativeNodesFromGraphNode(workspace, graphNode)
		if len(added) == 0 {
			continue
		}
		firstByWorkflowNode[graphNode.ID] = added[0].ID
		nodes = append(nodes, added...)
	}
	layoutLocalPDDCreativeCanvasNodes(nodes)
	return localPDDCreativeCanvas{
		RunID:          runID,
		ProductKey:     productKey,
		Product:        detail.Product,
		Nodes:          nodes,
		Edges:          localPDDCreativeCanvasEdges(firstByWorkflowNode, detail.Edges),
		Viewport:       map[string]float64{"x": 120, "y": 120, "k": 0.82},
		BackgroundMode: "lines",
		ShowImageInfo:  true,
		Saved:          false,
	}
}

func localPDDCreativeNodesFromGraphNode(workspace Workspace, graphNode localPDDGraphNode) []localPDDCreativeCanvasNode {
	nodes := []localPDDCreativeCanvasNode{}
	if localPDDShouldIncludeCreativeTextNode(graphNode) {
		metadata := localPDDCreativeNodeMetadata(graphNode, "run_text")
		metadata["content"] = graphNode.Summary
		nodes = append(nodes, localPDDCreativeCanvasNode{
			ID:       graphNode.ID,
			Type:     "text",
			Title:    graphNode.Title,
			Position: map[string]float64{"x": graphNode.X, "y": graphNode.Y},
			Width:    graphNode.Width,
			Height:   graphNode.Height,
			Metadata: metadata,
		})
	}
	for artifactIndex, artifact := range graphNode.Artifacts {
		nodeType := localPDDCreativeNodeType(artifact)
		nodeID := graphNode.ID
		if len(graphNode.Artifacts) > 1 {
			nodeID = fmt.Sprintf("%s-%02d", graphNode.ID, artifactIndex+1)
		}
		x := graphNode.X + float64(artifactIndex%3)*340
		y := graphNode.Y + float64(artifactIndex/3)*280
		width := localPDDCreativeNodeWidth(nodeType, graphNode.Width)
		height := localPDDCreativeNodeHeight(nodeType, graphNode.Height)
		mediaWidth, mediaHeight, naturalWidth, naturalHeight := localPDDCreativeMediaDimensions(workspace, artifact.ID)
		if naturalWidth > 0 && naturalHeight > 0 {
			width, height = mediaWidth, mediaHeight
		}
		metadata := localPDDCreativeNodeMetadata(graphNode, "run_artifact")
		metadata["content"] = artifact.URL
		metadata["artifactId"] = artifact.ID
		metadata["artifactPath"] = artifact.Path
		metadata["artifactKind"] = artifact.Kind
		metadata["storageKey"] = artifact.Path
		metadata["mimeType"] = artifact.MIMEType
		if naturalWidth > 0 && naturalHeight > 0 {
			metadata["naturalWidth"] = naturalWidth
			metadata["naturalHeight"] = naturalHeight
		}
		nodes = append(nodes, localPDDCreativeCanvasNode{
			ID:       nodeID,
			Type:     nodeType,
			Title:    firstNonEmptyString(graphNode.Title, artifact.Title),
			Position: map[string]float64{"x": x, "y": y},
			Width:    width,
			Height:   height,
			Metadata: metadata,
		})
	}
	if len(nodes) == 0 {
		nodeType := localPDDCreativeNodeTypeFromGraphNode(graphNode)
		if nodeType != "" {
			metadata := localPDDCreativeNodeMetadata(graphNode, "run_placeholder")
			if nodeType == "text" && strings.TrimSpace(graphNode.Summary) != "" {
				metadata["content"] = graphNode.Summary
			}
			nodes = append(nodes, localPDDCreativeCanvasNode{
				ID:       graphNode.ID,
				Type:     nodeType,
				Title:    graphNode.Title,
				Position: map[string]float64{"x": graphNode.X, "y": graphNode.Y},
				Width:    localPDDCreativeNodeWidth(nodeType, graphNode.Width),
				Height:   localPDDCreativeNodeHeight(nodeType, graphNode.Height),
				Metadata: metadata,
			})
		}
	}
	return nodes
}

func localPDDCreativeNodeMetadata(graphNode localPDDGraphNode, source string) map[string]any {
	status := graphNode.Status
	if status == "running" {
		status = "loading"
	}
	metadata := map[string]any{
		"status":               status,
		"workflowNodeId":       graphNode.ID,
		"originWorkflowNodeId": graphNode.ID,
		"source":               source,
		"prompt":               stringFromMap(graphNode.Config, "prompt"),
		"model":                stringFromMap(graphNode.Config, "model"),
		"size":                 stringFromMap(graphNode.Config, "size"),
		"quality":              stringFromMap(graphNode.Config, "quality"),
		"count":                stringFromMap(graphNode.Config, "count"),
		"operation":            stringFromMap(graphNode.Config, "operation"),
	}
	if graphNode.Status == "error" && strings.TrimSpace(graphNode.Summary) != "" {
		metadata["errorDetails"] = graphNode.Summary
	}
	return metadata
}

func localPDDCreativeNodeTypeFromGraphNode(node localPDDGraphNode) string {
	nodeType := strings.ToLower(strings.TrimSpace(node.Type))
	operation := strings.ToLower(stringFromMap(node.Config, "operation"))
	title := strings.ToLower(node.Title)
	if nodeType == "video" || strings.Contains(operation, "video") {
		return "video"
	}
	if nodeType == "image" || nodeType == "material" || nodeType == "material_lookup" || strings.Contains(operation, "image") || strings.Contains(operation, "mockup") || strings.Contains(title, "图") || strings.Contains(title, "mockup") {
		return "image"
	}
	if nodeType == "text" || operation == "input" || strings.Contains(operation, "text") || strings.Contains(operation, "title") || strings.Contains(title, "标题") || strings.Contains(title, "输入") || strings.Contains(title, "质检") || strings.Contains(title, "判定") {
		return "text"
	}
	if nodeType == "config" || strings.Contains(operation, "condition") || strings.Contains(operation, "script") || strings.Contains(operation, "package") || strings.Contains(operation, "sync") {
		return "config"
	}
	return ""
}

func localPDDShouldIncludeCreativeTextNode(node localPDDGraphNode) bool {
	if strings.TrimSpace(node.Summary) == "" || node.Type != "text" {
		return false
	}
	operation := stringFromMap(node.Config, "operation")
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

func localPDDCreativeNodeWidth(nodeType string, fallback float64) float64 {
	if nodeType == "video" {
		return 420
	}
	if fallback >= 220 {
		return fallback
	}
	return 320
}

func localPDDCreativeNodeHeight(nodeType string, fallback float64) float64 {
	if nodeType == "video" {
		return 236
	}
	if fallback >= 160 {
		return fallback
	}
	return 220
}

func normalizeLocalPDDCreativeCanvas(workspace Workspace, canvas localPDDCreativeCanvas, detail localPDDProductDetail) localPDDCreativeCanvas {
	canvas.RunID = firstNonEmptyString(canvas.RunID, detail.RunID)
	canvas.ProductKey = firstNonEmptyString(canvas.ProductKey, detail.Product.Key)
	canvas.Product = detail.Product
	generated := newLocalPDDCreativeCanvas(workspace, canvas.RunID, canvas.ProductKey, detail)
	canvas.Nodes = mergeLocalPDDCreativeCanvasNodes(canvas.Nodes, generated.Nodes)
	if len(canvas.Edges) == 0 || localPDDCreativeCanvasHasOverlap(canvas.Nodes) {
		layoutLocalPDDCreativeCanvasNodes(canvas.Nodes)
	}
	canvas.Edges = mergeLocalPDDCreativeCanvasEdges(canvas.Edges, generated.Edges, canvas.Nodes)
	if canvas.Viewport == nil {
		canvas.Viewport = map[string]float64{"x": 120, "y": 120, "k": 0.82}
	}
	if canvas.BackgroundMode == "" {
		canvas.BackgroundMode = "lines"
	}
	return canvas
}

func mergeLocalPDDCreativeCanvasNodes(current []localPDDCreativeCanvasNode, generated []localPDDCreativeCanvasNode) []localPDDCreativeCanvasNode {
	generatedByID := map[string]localPDDCreativeCanvasNode{}
	generatedByKey := map[string]localPDDCreativeCanvasNode{}
	for _, node := range generated {
		generatedByID[node.ID] = node
		if key := localPDDCreativeNodeMergeKey(node); key != "" {
			generatedByKey[key] = node
		}
	}
	out := make([]localPDDCreativeCanvasNode, 0, len(current)+len(generatedByID))
	for _, node := range current {
		if generatedNode, ok := generatedByID[node.ID]; ok {
			node = mergeLocalPDDCreativeCanvasNode(node, generatedNode)
			delete(generatedByID, node.ID)
			if key := localPDDCreativeNodeMergeKey(generatedNode); key != "" {
				delete(generatedByKey, key)
			}
		} else if !localPDDCreativeMetadataHasLocalOverride(node.Metadata) {
			if key := localPDDCreativeNodeMergeKey(node); key != "" {
				if generatedNode, ok := generatedByKey[key]; ok {
					if _, pending := generatedByID[generatedNode.ID]; pending {
						node = mergeLocalPDDCreativeCanvasNode(node, generatedNode)
						node.ID = generatedNode.ID
						delete(generatedByID, generatedNode.ID)
						delete(generatedByKey, key)
					}
				}
			}
		}
		out = append(out, node)
	}
	for _, node := range generated {
		if _, ok := generatedByID[node.ID]; ok {
			out = append(out, node)
		}
	}
	return out
}

func localPDDCreativeNodeMergeKey(node localPDDCreativeCanvasNode) string {
	if node.Metadata == nil {
		return ""
	}
	if artifactID := stringFromMap(node.Metadata, "artifactId"); artifactID != "" {
		return "artifact:" + artifactID
	}
	workflowNodeID := localPDDCreativeWorkflowNodeID(node)
	source := stringFromMap(node.Metadata, "source")
	if workflowNodeID == "" || localPDDCreativeMetadataHasLocalOverride(node.Metadata) {
		return ""
	}
	return "workflow:" + workflowNodeID + ":" + source
}

func localPDDCreativeMetadataHasLocalOverride(metadata map[string]any) bool {
	source := stringFromMap(metadata, "source")
	return source == "user_upload" || strings.HasPrefix(source, "creative_")
}

func mergeLocalPDDCreativeCanvasNode(current localPDDCreativeCanvasNode, generated localPDDCreativeCanvasNode) localPDDCreativeCanvasNode {
	if current.Type == "" {
		current.Type = generated.Type
	}
	if current.Title == "" {
		current.Title = generated.Title
	}
	if current.Position == nil {
		current.Position = generated.Position
	}
	if current.Width <= 0 {
		current.Width = generated.Width
	}
	if current.Height <= 0 {
		current.Height = generated.Height
	}
	if current.Width <= 220 && current.Height <= 180 && generated.Width > current.Width && generated.Height > current.Height {
		current.Width = generated.Width
		current.Height = generated.Height
	}
	metadata := map[string]any{}
	for key, value := range current.Metadata {
		metadata[key] = value
	}
	localOverride := localPDDCreativeMetadataHasLocalOverride(current.Metadata)
	for key, value := range generated.Metadata {
		if localOverride && (key == "content" || key == "source" || key == "status" || key == "errorDetails") {
			continue
		}
		metadata[key] = value
	}
	current.Metadata = metadata
	return current
}

func mergeLocalPDDCreativeCanvasEdges(current []localPDDCreativeCanvasEdge, generated []localPDDCreativeCanvasEdge, nodes []localPDDCreativeCanvasNode) []localPDDCreativeCanvasEdge {
	nodeIDs := map[string]bool{}
	for _, node := range nodes {
		nodeIDs[node.ID] = true
	}
	seen := map[string]bool{}
	out := make([]localPDDCreativeCanvasEdge, 0, len(current)+len(generated))
	for _, edge := range current {
		if edge.FromNodeID == "" || edge.ToNodeID == "" || !nodeIDs[edge.FromNodeID] || !nodeIDs[edge.ToNodeID] {
			continue
		}
		key := edge.FromNodeID + "->" + edge.ToNodeID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, edge)
	}
	for _, edge := range generated {
		key := edge.FromNodeID + "->" + edge.ToNodeID
		if edge.FromNodeID == "" || edge.ToNodeID == "" || !nodeIDs[edge.FromNodeID] || !nodeIDs[edge.ToNodeID] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, edge)
	}
	return out
}

func layoutLocalPDDCreativeCanvasNodes(nodes []localPDDCreativeCanvasNode) {
	if len(nodes) == 0 {
		return
	}
	for index := range nodes {
		if nodes[index].Position == nil {
			nodes[index].Position = map[string]float64{"x": 0, "y": 0}
		}
		if nodes[index].Width <= 0 {
			nodes[index].Width = localPDDCreativeNodeWidth(nodes[index].Type, 0)
		}
		if nodes[index].Height <= 0 {
			nodes[index].Height = localPDDCreativeNodeHeight(nodes[index].Type, 0)
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
		nodes []*localPDDCreativeCanvasNode
		width float64
	}
	columns := []column{}
	for index := range nodes {
		node := &nodes[index]
		x := node.Position["x"]
		if len(columns) == 0 || absFloat64(columns[len(columns)-1].key-x) > 180 {
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

func localPDDCreativeCanvasEdges(firstByWorkflowNode map[string]string, graphEdges []localPDDGraphEdge) []localPDDCreativeCanvasEdge {
	out := []localPDDCreativeCanvasEdge{}
	seen := map[string]bool{}
	for _, graphEdge := range graphEdges {
		fromID := firstByWorkflowNode[graphEdge.From]
		toID := firstByWorkflowNode[graphEdge.To]
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		key := fromID + "->" + toID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, localPDDCreativeCanvasEdge{
			ID:         "creative-" + fromID + "-" + toID,
			FromNodeID: fromID,
			ToNodeID:   toID,
		})
	}
	return out
}

func localPDDCreativeWorkflowNodeID(node localPDDCreativeCanvasNode) string {
	if node.Metadata == nil {
		return ""
	}
	return firstNonEmptyString(stringFromMap(node.Metadata, "originWorkflowNodeId"), stringFromMap(node.Metadata, "workflowNodeId"))
}

func localPDDCreativeCanvasHasOverlap(nodes []localPDDCreativeCanvasNode) bool {
	for i := range nodes {
		if localPDDCreativeWorkflowNodeID(nodes[i]) == "" {
			continue
		}
		for j := i + 1; j < len(nodes); j++ {
			if localPDDCreativeWorkflowNodeID(nodes[j]) == "" {
				continue
			}
			if localPDDCreativeOverlapRatio(nodes[i], nodes[j]) > 0.08 {
				return true
			}
		}
	}
	return false
}

func localPDDCreativeOverlapRatio(a localPDDCreativeCanvasNode, b localPDDCreativeCanvasNode) float64 {
	ax, ay := localPDDCreativeNodeXY(a)
	bx, by := localPDDCreativeNodeXY(b)
	left := maxFloat64(ax, bx)
	right := minFloat64(ax+a.Width, bx+b.Width)
	top := maxFloat64(ay, by)
	bottom := minFloat64(ay+a.Height, by+b.Height)
	if right <= left || bottom <= top {
		return 0
	}
	overlap := (right - left) * (bottom - top)
	minArea := minFloat64(a.Width*a.Height, b.Width*b.Height)
	if minArea <= 0 {
		return 0
	}
	return overlap / minArea
}

func localPDDCreativeNodeXY(node localPDDCreativeCanvasNode) (float64, float64) {
	if node.Position == nil {
		return 0, 0
	}
	return node.Position["x"], node.Position["y"]
}

func maxFloat64(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat64(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func absFloat64(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func saveLocalPDDCreativeCanvas(workspace Workspace, canvas localPDDCreativeCanvas) (localPDDCreativeCanvas, error) {
	canvas.UpdatedAt = timeNowRFC3339()
	canvas.Saved = true
	now := timeNowRFC3339()
	document := Envelope[localPDDCreativeCanvas]{
		SchemaVersion: SchemaVersion,
		Kind:          "creative_canvas",
		ID:            safeProductKey(canvas.ProductKey),
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Data:          canvas,
	}
	filePath := localPDDCreativeCanvasPath(workspace, canvas.RunID, canvas.ProductKey)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return localPDDCreativeCanvas{}, WrapError(ErrorInternal, "create creative canvas directory", 5, err)
	}
	if err := AtomicWriteJSON(filePath, document, 0o600); err != nil {
		return localPDDCreativeCanvas{}, err
	}
	return canvas, nil
}

func importLocalPDDCreativeCanvasAsset(workspace Workspace, runID string, productKey string, nodeID string, fileName string, mimeType string, content string, baseURL string) (map[string]any, error) {
	data, detectedMIME, err := decodeLocalPDDContent(content)
	if err != nil {
		return nil, err
	}
	mimeType = firstNonEmptyString(mimeType, detectedMIME, mime.TypeByExtension(filepath.Ext(fileName)), "application/octet-stream")
	kind := "file"
	if strings.HasPrefix(mimeType, "image/") {
		kind = "image"
	} else if strings.HasPrefix(mimeType, "video/") {
		kind = "video"
	} else if strings.HasPrefix(mimeType, "text/") {
		kind = "text"
	}
	title := firstNonEmptyString(strings.TrimSuffix(fileName, filepath.Ext(fileName)), nodeID, "creative asset")
	artifact, err := createExecutorArtifact(workspace, runID, productScopedNodeID(productKey, nodeID), kind, mimeType, title, data, "creative_canvas", "manual", 0, map[string]any{
		"type":           "creative_canvas_upload",
		"productKey":     productKey,
		"templateNodeId": nodeID,
	})
	if err != nil {
		return nil, err
	}
	width, height := 0, 0
	if kind == "image" {
		if config, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			width, height = config.Width, config.Height
		}
	}
	return map[string]any{
		"url":      localPDDArtifactURL(baseURL, artifact.ID),
		"path":     "artifact:" + artifact.ID,
		"fileName": firstNonEmptyString(fileName, artifact.ID+extensionForMIME(mimeType, kind)),
		"mimeType": mimeType,
		"bytes":    len(data),
		"width":    width,
		"height":   height,
	}, nil
}

func applyLocalPDDCreativeCanvasOutput(workspace Workspace, runID string, productKey string, payload map[string]any, baseURL string) (map[string]any, error) {
	sourceNodeID := stringFromMap(payload, "sourceNodeId")
	targetNodeID := stringFromMap(payload, "targetNodeId")
	if sourceNodeID == "" || targetNodeID == "" {
		return nil, NewError(ErrorInvalidArgument, "sourceNodeId and targetNodeId are required", 1, nil)
	}
	content := stringFromMap(payload, "content")
	artifactPath := stringFromMap(payload, "artifactPath")
	artifacts := []localPDDArtifact{}
	if strings.HasPrefix(artifactPath, "artifact:") {
		artifactID := strings.TrimPrefix(artifactPath, "artifact:")
		artifact, err := ReadArtifact(workspace, artifactID)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, localPDDArtifact{ID: artifact.ID, Title: artifact.Data.Title, Path: "artifact:" + artifact.ID, URL: localPDDArtifactURL(baseURL, artifact.ID), Kind: artifact.Data.Type, MIMEType: artifact.Data.MIME})
	} else if content != "" {
		asset, err := importLocalPDDCreativeCanvasAsset(workspace, runID, productKey, targetNodeID, targetNodeID, stringFromMap(payload, "mimeType"), content, baseURL)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, localPDDArtifact{ID: strings.TrimPrefix(stringFromAny(asset["path"]), "artifact:"), Title: targetNodeID, Path: stringFromAny(asset["path"]), URL: stringFromAny(asset["url"]), Kind: "image", MIMEType: stringFromAny(asset["mimeType"])})
	}
	return map[string]any{
		"editId":          fmt.Sprintf("creative_%s_%d", safeProductKey(productKey), time.Now().Unix()),
		"productKey":      productKey,
		"nodeId":          targetNodeID,
		"artifacts":       artifacts,
		"applied":         true,
		"rerunDownstream": false,
		"output":          "已应用到本地创作画布；本地 executor 不会自动重跑已完成节点。",
		"sourceNodeId":    sourceNodeID,
	}, nil
}

func decodeLocalPDDContent(content string) ([]byte, string, error) {
	if strings.HasPrefix(content, "data:") {
		header, body, ok := strings.Cut(content, ",")
		if !ok {
			return nil, "", NewError(ErrorInvalidArgument, "data url is invalid", 1, nil)
		}
		mimeType := strings.TrimPrefix(strings.Split(strings.TrimPrefix(header, "data:"), ";")[0], " ")
		data, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, "", WrapError(ErrorInvalidArgument, "decode data url", 1, err)
		}
		return data, mimeType, nil
	}
	data, err := base64.StdEncoding.DecodeString(content)
	if err == nil {
		return data, "", nil
	}
	return []byte(content), "text/plain; charset=utf-8", nil
}

func localPDDCreativeMediaSize(workspace Workspace, artifactID string) (float64, float64) {
	width, height, _, _ := localPDDCreativeMediaDimensions(workspace, artifactID)
	return width, height
}

func localPDDCreativeMediaDimensions(workspace Workspace, artifactID string) (float64, float64, int, int) {
	artifact, err := ReadArtifact(workspace, artifactID)
	if err != nil {
		return 340, 240, 0, 0
	}
	if artifact.Data.Type == "text" || strings.HasPrefix(artifact.Data.MIME, "text/") || artifact.Data.MIME == "application/json" {
		return 340, 240, 0, 0
	}
	if artifact.Data.Type == "video" || strings.HasPrefix(artifact.Data.MIME, "video/") {
		if artifact.Data.Width > 0 && artifact.Data.Height > 0 {
			width, height := fitLocalPDDCreativeMediaSize(artifact.Data.Width, artifact.Data.Height, 420, 420)
			return width, height, artifact.Data.Width, artifact.Data.Height
		}
		return 420, 236, 0, 0
	}
	width, height := artifact.Data.Width, artifact.Data.Height
	if (width <= 0 || height <= 0) && (artifact.Data.Type == "image" || strings.HasPrefix(artifact.Data.MIME, "image/")) {
		width, height = decodeLocalPDDArtifactImageSize(workspace, artifact)
	}
	if width > 0 && height > 0 {
		fitWidth, fitHeight := fitLocalPDDCreativeMediaSize(width, height, 640, 640)
		return fitWidth, fitHeight, width, height
	}
	if artifact.Data.Type == "image" || strings.HasPrefix(artifact.Data.MIME, "image/") {
		return 340, 240, 0, 0
	}
	return 340, 240, 0, 0
}

func decodeLocalPDDArtifactImageSize(workspace Workspace, artifact Envelope[ArtifactData]) (int, int) {
	relPath := strings.TrimSpace(artifact.Data.Files["original"])
	if relPath == "" {
		return 0, 0
	}
	cleanPath := filepath.Clean(filepath.FromSlash(relPath))
	if cleanPath == "." || filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || cleanPath == ".." {
		return 0, 0
	}
	file, err := os.Open(filepath.Join(ArtifactRepository(workspace).Dir(artifact.ID), cleanPath))
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func fitLocalPDDCreativeMediaSize(width int, height int, maxWidth float64, maxHeight float64) (float64, float64) {
	w := float64(max(1, width))
	h := float64(max(1, height))
	scale := minFloat64(1, minFloat64(maxWidth/w, maxHeight/h))
	return w * scale, h * scale
}

func localPDDCreativeNodeType(artifact localPDDArtifact) string {
	if artifact.Kind == "video" || strings.HasPrefix(artifact.MIMEType, "video/") {
		return "video"
	}
	if artifact.Kind == "text" || strings.HasPrefix(artifact.MIMEType, "text/") {
		return "text"
	}
	return "image"
}

func (api *serveAPI) handleLocalPDDCreativeCanvas(w http.ResponseWriter, r *http.Request, runID string) {
	productKey := strings.TrimSpace(r.URL.Query().Get("key"))
	if productKey == "" {
		productKey = strings.TrimSpace(r.URL.Query().Get("productKey"))
	}
	switch r.Method {
	case http.MethodGet:
		canvas, err := loadLocalPDDCreativeCanvas(api.workspace, runID, productKey, api.runtime.BaseURL)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, canvas, nil)
	case http.MethodPost:
		var payload localPDDCreativeCanvas
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&payload); err != nil {
			writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "creative canvas payload is invalid", 1, nil))
			return
		}
		payload.RunID = runID
		payload.ProductKey = productKey
		if payload.Product.Key == "" {
			detail, err := buildLocalPDDProductDetail(api.workspace, runID, productKey, api.runtime.BaseURL)
			if err == nil {
				payload.Product = detail.Product
			}
		}
		canvas, err := saveLocalPDDCreativeCanvas(api.workspace, payload)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, canvas, nil)
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) handleLocalPDDCreativeCanvasAsset(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		return
	}
	productKey := strings.TrimSpace(r.URL.Query().Get("key"))
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&payload); err != nil {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "creative canvas asset payload is invalid", 1, nil))
		return
	}
	asset, err := importLocalPDDCreativeCanvasAsset(api.workspace, runID, productKey, stringFromMap(payload, "nodeId"), stringFromMap(payload, "fileName"), stringFromMap(payload, "mimeType"), stringFromMap(payload, "content"), api.runtime.BaseURL)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, asset, nil)
}

func (api *serveAPI) handleLocalPDDCreativeCanvasApply(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		return
	}
	productKey := strings.TrimSpace(r.URL.Query().Get("key"))
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&payload); err != nil {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "creative canvas apply payload is invalid", 1, nil))
		return
	}
	result, err := applyLocalPDDCreativeCanvasOutput(api.workspace, runID, productKey, payload, api.runtime.BaseURL)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, result, nil)
}
