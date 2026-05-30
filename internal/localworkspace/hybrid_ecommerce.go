package localworkspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	hybridEcommerceKey       = "hybridEcommerce"
	hybridEcommerceBackend   = "vps_pdd"
	hybridEcommerceVersion   = 1
	hybridRemoteRunStarted   = "hybrid.remote_run.started"
	hybridRemoteRunSynced    = "hybrid.remote_run.synced"
	hybridRemoteRunCompleted = "hybrid.remote_run.completed"
	hybridRemoteRunFailed    = "hybrid.remote_run.failed"
)

type HybridEcommerceImportOptions struct {
	WorkspacePath    string
	BaseURL          string
	RemoteTemplateID string
	ProfileID        string
	ChannelID        string
	SecretEnv        string
	HTTPClient       *http.Client
}

type HybridEcommerceImportResult struct {
	Created          bool                      `json:"created"`
	Template         Envelope[TemplateData]    `json:"template"`
	RemoteTemplateID string                    `json:"remoteTemplateId"`
	ProfileID        string                    `json:"profileId,omitempty"`
	ChannelID        string                    `json:"channelId,omitempty"`
	Warnings         []string                  `json:"warnings,omitempty"`
	SecretRef        *SecretRefSummary         `json:"secretRef,omitempty"`
	Remote           hybridRemoteTemplateBrief `json:"remote"`
}

type hybridRemoteTemplateBrief struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type hybridEcommerceConfig struct {
	Backend          string
	BaseURL          string
	RemoteTemplateID string
	RemoteRunID      string
	ProfileID        string
	ChannelID        string
	SecretRef        *SecretRef
	PollInterval     time.Duration
	MaxPoll          time.Duration
}

type hybridPDDTemplate struct {
	ID           string         `json:"id"`
	WorkflowType string         `json:"workflowType"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	Spec         hybridPDDSpec  `json:"spec"`
	CreatedAt    string         `json:"createdAt"`
	UpdatedAt    string         `json:"updatedAt"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Extra        map[string]any `json:"-"`
}

type hybridPDDSpec struct {
	Version  int               `json:"version"`
	Nodes    []json.RawMessage `json:"nodes"`
	Edges    []json.RawMessage `json:"edges"`
	Settings map[string]any    `json:"settings"`
}

type hybridRemoteRunResult struct {
	RunID  string `json:"runId"`
	RunDir string `json:"runDir"`
}

type hybridPDDOverview struct {
	Run struct {
		RunID             string `json:"runId"`
		Status            string `json:"status"`
		Completed         bool   `json:"completed"`
		ProductTotal      int    `json:"productTotal"`
		CompletedProducts int    `json:"completedProducts"`
		FailedProducts    int    `json:"failedProducts"`
		RunningProducts   int    `json:"runningProducts"`
		RecentError       string `json:"recentError,omitempty"`
	} `json:"run"`
	Stages       []hybridPDDStage   `json:"stages"`
	Products     []hybridPDDProduct `json:"products"`
	RecentErrors []string           `json:"recentErrors"`
}

type hybridPDDStage struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type,omitempty"`
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Success     int    `json:"success"`
	Failed      int    `json:"failed"`
	Running     int    `json:"running"`
	Idle        int    `json:"idle"`
	Skipped     int    `json:"skipped"`
	RecentError string `json:"recentError,omitempty"`
}

type hybridPDDProduct struct {
	Key              string `json:"key"`
	SourceProduct    string `json:"sourceProduct"`
	GeneratedProduct string `json:"generatedProduct,omitempty"`
	Product          string `json:"product"`
	Status           string `json:"status"`
	RawStatus        string `json:"rawStatus"`
	Error            string `json:"error,omitempty"`
	ArtifactCount    int    `json:"artifactCount,omitempty"`
}

type hybridPDDProductDetail struct {
	RunID   string                 `json:"runId"`
	Product hybridPDDProduct       `json:"product"`
	Nodes   []hybridPDDProductNode `json:"nodes"`
	Files   []hybridPDDDetailFile  `json:"files"`
}

type hybridPDDProductNode struct {
	ID        string                `json:"id"`
	Type      string                `json:"type"`
	Title     string                `json:"title"`
	Status    string                `json:"status"`
	Artifacts []hybridPDDArtifact   `json:"artifacts,omitempty"`
	Files     []hybridPDDDetailFile `json:"files,omitempty"`
}

type hybridPDDArtifact struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	MimeType string `json:"mimeType,omitempty"`
}

type hybridPDDDetailFile struct {
	Title string `json:"title"`
	Path  string `json:"path"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

type hybridVPSClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func ImportHybridEcommerceTemplate(ctx context.Context, opts HybridEcommerceImportOptions) (HybridEcommerceImportResult, error) {
	workspace, err := Open(opts.WorkspacePath)
	if err != nil {
		return HybridEcommerceImportResult{}, err
	}
	opts.RemoteTemplateID = strings.TrimSpace(opts.RemoteTemplateID)
	if opts.RemoteTemplateID == "" {
		return HybridEcommerceImportResult{}, NewError(ErrorInvalidArgument, "remote template id is required", 1, nil)
	}
	client, summary, err := hybridEcommerceClient(*workspace, hybridEcommerceConfig{
		BaseURL:   opts.BaseURL,
		ProfileID: opts.ProfileID,
		ChannelID: opts.ChannelID,
		SecretRef: hybridSecretEnvRef(opts.SecretEnv),
	}, opts.HTTPClient)
	if err != nil {
		return HybridEcommerceImportResult{}, err
	}
	remote, err := client.getTemplate(ctx, opts.RemoteTemplateID)
	if err != nil {
		return HybridEcommerceImportResult{}, err
	}
	document, created, err := upsertHybridEcommerceTemplate(*workspace, remote, hybridEcommerceConfig{
		BaseURL:          client.baseURL,
		RemoteTemplateID: remote.ID,
		ProfileID:        opts.ProfileID,
		ChannelID:        opts.ChannelID,
		SecretRef:        hybridSecretEnvRef(opts.SecretEnv),
	})
	if err != nil {
		return HybridEcommerceImportResult{}, err
	}
	result := HybridEcommerceImportResult{
		Created:          created,
		Template:         document,
		RemoteTemplateID: remote.ID,
		ProfileID:        strings.TrimSpace(opts.ProfileID),
		ChannelID:        strings.TrimSpace(opts.ChannelID),
		Remote: hybridRemoteTemplateBrief{
			ID:        remote.ID,
			Title:     remote.Title,
			UpdatedAt: remote.UpdatedAt,
		},
	}
	if summary != nil {
		result.SecretRef = summary
	}
	return result, nil
}

func upsertHybridEcommerceTemplate(workspace Workspace, remote hybridPDDTemplate, config hybridEcommerceConfig) (Envelope[TemplateData], bool, error) {
	remote.ID = strings.TrimSpace(remote.ID)
	if remote.ID == "" {
		return Envelope[TemplateData]{}, false, NewError(ErrorWorkspaceInvalid, "remote template id is empty", 2, nil)
	}
	settings := cloneMap(remote.Spec.Settings)
	if settings == nil {
		settings = map[string]any{}
	}
	if strings.TrimSpace(config.ProfileID) != "" {
		settings["defaultProfileId"] = strings.TrimSpace(config.ProfileID)
	}
	metadata := map[string]any{
		"source": "hybrid_ecommerce_vps",
		hybridEcommerceKey: map[string]any{
			"version":          hybridEcommerceVersion,
			"backend":          hybridEcommerceBackend,
			"remoteTemplateId": remote.ID,
			"remoteTitle":      remote.Title,
			"remoteUpdatedAt":  remote.UpdatedAt,
			"profileId":        strings.TrimSpace(config.ProfileID),
			"channelId":        strings.TrimSpace(config.ChannelID),
		},
	}
	hybrid := metadata[hybridEcommerceKey].(map[string]any)
	if strings.TrimSpace(config.BaseURL) != "" && strings.TrimSpace(config.ProfileID) == "" {
		hybrid["baseUrl"] = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	}
	if config.SecretRef != nil && strings.TrimSpace(config.ProfileID) == "" {
		hybrid["secretRef"] = *config.SecretRef
	}
	data := TemplateData{
		Title:        nonEmptyString(remote.Title, "Hybrid Ecommerce Template"),
		Description:  remote.Description,
		WorkflowType: firstNonEmptyString(remote.WorkflowType, "pdd"),
		Version:      remote.Spec.Version,
		Nodes:        append([]json.RawMessage{}, remote.Spec.Nodes...),
		Edges:        append([]json.RawMessage{}, remote.Spec.Edges...),
		Settings:     settings,
		Metadata:     metadata,
	}
	if data.WorkflowType == "" {
		data.WorkflowType = "pdd"
	}
	if data.Version <= 0 {
		data.Version = 1
	}
	existing, ok, err := findHybridTemplate(workspace, remote.ID)
	if err != nil {
		return Envelope[TemplateData]{}, false, err
	}
	if ok {
		next := nextEnvelopeRevision(existing, data)
		if err := WriteTemplate(workspace, next); err != nil {
			return Envelope[TemplateData]{}, false, err
		}
		saved, err := ReadTemplate(workspace, existing.ID)
		return saved, false, err
	}
	created, err := NewTemplate(workspace, data)
	if err != nil {
		return Envelope[TemplateData]{}, false, err
	}
	if err := WriteTemplate(workspace, created); err != nil {
		return Envelope[TemplateData]{}, false, err
	}
	saved, err := ReadTemplate(workspace, created.ID)
	return saved, true, err
}

func findHybridTemplate(workspace Workspace, remoteTemplateID string) (Envelope[TemplateData], bool, error) {
	templates, err := ListTemplates(workspace)
	if err != nil {
		return Envelope[TemplateData]{}, false, err
	}
	for _, template := range templates {
		config, ok, err := hybridEcommerceConfigFromTemplate(template)
		if err != nil {
			return Envelope[TemplateData]{}, false, err
		}
		if ok && config.RemoteTemplateID == remoteTemplateID {
			return template, true, nil
		}
	}
	return Envelope[TemplateData]{}, false, nil
}

func executeHybridEcommerceRun(ctx context.Context, workspace Workspace, run Envelope[RunData], template Envelope[TemplateData], opts ExecutorOptions, config hybridEcommerceConfig) (ExecutorRunResult, error) {
	runResult := ExecutorRunResult{RunID: run.ID, Status: run.Data.Status}
	client, _, err := hybridEcommerceClient(workspace, config, opts.HTTPClient)
	if err != nil {
		return failRunResult(workspace, run, runResult, err)
	}
	if run.Data.Status == RunStatusPending {
		if run, err = updateExecutorRun(workspace, run, RunStatusRunning, nil, ""); err != nil {
			return runResult, err
		}
		if _, err := appendExecutorEvent(workspace, run.ID, executorEventClaimed, "info", "Hybrid ecommerce executor claimed run", map[string]any{"templateId": run.Data.TemplateID}); err != nil {
			return runResult, err
		}
	} else if _, err := appendExecutorEvent(workspace, run.ID, executorEventResumed, "info", "Hybrid ecommerce executor resumed run", nil); err != nil {
		return runResult, err
	}
	run, err = ReadRun(workspace, run.ID)
	if err != nil {
		return runResult, err
	}
	remoteRunID := strings.TrimSpace(config.RemoteRunID)
	if remoteRunID == "" {
		remoteRunID = stringFromMap(hybridEcommerceRunMetadata(run), "remoteRunId")
	}
	if remoteRunID == "" {
		remoteRunID = "hybrid_" + run.ID
		request := hybridStartRunPayload(run, template, remoteRunID)
		started, startErr := client.startRun(ctx, config.RemoteTemplateID, request)
		if startErr != nil {
			return failRunResult(workspace, run, runResult, startErr)
		}
		remoteRunID = nonEmptyString(started.RunID, remoteRunID)
		run, err = patchHybridRunMetadata(workspace, run.ID, map[string]any{
			"backend":          hybridEcommerceBackend,
			"remoteTemplateId": config.RemoteTemplateID,
			"remoteRunId":      remoteRunID,
			"lastSyncedAt":     timeNowRFC3339(),
		})
		if err != nil {
			return runResult, err
		}
		if _, err := appendExecutorEvent(workspace, run.ID, hybridRemoteRunStarted, "info", "Remote ecommerce run started", map[string]any{
			"remoteRunId":      remoteRunID,
			"remoteTemplateId": config.RemoteTemplateID,
		}); err != nil {
			return runResult, err
		}
	}

	interval := config.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	startedAt := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return runResult, err
		}
		overview, err := client.overview(ctx, remoteRunID)
		if err != nil {
			return failRunResult(workspace, run, runResult, err)
		}
		run, runResult, err = syncHybridRemoteOverview(ctx, workspace, run.ID, runResult, client, config, remoteRunID, overview)
		if err != nil {
			return runResult, err
		}
		if runResult.Status == RunStatusSuccess || runResult.Status == RunStatusError {
			return runResult, nil
		}
		if config.MaxPoll > 0 && time.Since(startedAt) >= config.MaxPoll {
			runResult.Status = RunStatusRunning
			return runResult, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return runResult, ctx.Err()
		case <-timer.C:
		}
	}
}

func syncHybridRemoteOverview(ctx context.Context, workspace Workspace, runID string, runResult ExecutorRunResult, client hybridVPSClient, config hybridEcommerceConfig, remoteRunID string, overview hybridPDDOverview) (Envelope[RunData], ExecutorRunResult, error) {
	if err := writeHybridStageStates(workspace, runID, overview.Stages); err != nil {
		return Envelope[RunData]{}, runResult, err
	}
	status := hybridLocalRunStatus(overview)
	output := map[string]any{
		"backend":           hybridEcommerceBackend,
		"remoteRunId":       remoteRunID,
		"remoteStatus":      overview.Run.Status,
		"productTotal":      overview.Run.ProductTotal,
		"completedProducts": overview.Run.CompletedProducts,
		"failedProducts":    overview.Run.FailedProducts,
		"runningProducts":   overview.Run.RunningProducts,
	}
	current, err := ReadRun(workspace, runID)
	if err != nil {
		return Envelope[RunData]{}, runResult, err
	}
	statusForSync := status
	if status == RunStatusSuccess || status == RunStatusError {
		statusForSync = RunStatusRunning
	}
	run, err := updateExecutorRun(workspace, current, statusForSync, output, "")
	if err != nil {
		return Envelope[RunData]{}, runResult, err
	}
	if _, err := patchHybridRunMetadata(workspace, runID, map[string]any{
		"backend":           hybridEcommerceBackend,
		"remoteTemplateId":  config.RemoteTemplateID,
		"remoteRunId":       remoteRunID,
		"remoteStatus":      overview.Run.Status,
		"lastSyncedAt":      timeNowRFC3339(),
		"productTotal":      overview.Run.ProductTotal,
		"completedProducts": overview.Run.CompletedProducts,
		"failedProducts":    overview.Run.FailedProducts,
	}); err != nil {
		return Envelope[RunData]{}, runResult, err
	}
	if _, err := appendExecutorEvent(workspace, runID, hybridRemoteRunSynced, "info", "Remote ecommerce run synced", output); err != nil {
		return Envelope[RunData]{}, runResult, err
	}
	if status == RunStatusSuccess || status == RunStatusError {
		artifactCount, err := syncHybridRemoteArtifacts(ctx, workspace, runID, client, config, remoteRunID, overview.Products)
		if err != nil {
			return Envelope[RunData]{}, runResult, err
		}
		terminalEvent := hybridRemoteRunCompleted
		executorEvent := executorEventRunCompleted
		level := "info"
		message := "Hybrid ecommerce run completed"
		if status == RunStatusError {
			terminalEvent = hybridRemoteRunFailed
			executorEvent = executorEventRunFailed
			level = "error"
			message = "Hybrid ecommerce run failed"
		}
		terminalData := cloneMap(output)
		terminalData["artifactRefs"] = artifactCount
		if len(overview.RecentErrors) > 0 {
			terminalData["recentErrors"] = overview.RecentErrors
		}
		errorMessage := ""
		if status == RunStatusError {
			errorMessage = firstNonEmptyString(overview.Run.RecentError, strings.Join(overview.RecentErrors, "; "))
		}
		run, err = updateExecutorRun(workspace, run, status, terminalData, errorMessage)
		if err != nil {
			return Envelope[RunData]{}, runResult, err
		}
		if _, err := appendExecutorEvent(workspace, runID, terminalEvent, level, message, terminalData); err != nil {
			return Envelope[RunData]{}, runResult, err
		}
		if _, err := appendExecutorEvent(workspace, runID, executorEvent, level, message, terminalData); err != nil {
			return Envelope[RunData]{}, runResult, err
		}
		runResult.Status = status
		runResult.ArtifactRefs = artifactCount
		runResult.Executed = 1
		runResult.Error = errorMessage
	}
	return run, runResult, nil
}

func writeHybridStageStates(workspace Workspace, runID string, stages []hybridPDDStage) error {
	current, err := executorNodeStateMap(workspace, runID)
	if err != nil {
		return err
	}
	for _, stage := range stages {
		nodeID := strings.TrimSpace(stage.ID)
		if nodeID == "" {
			continue
		}
		state := hybridNodeStatus(stage.Status)
		output := map[string]any{
			"total":   stage.Total,
			"success": stage.Success,
			"failed":  stage.Failed,
			"running": stage.Running,
			"idle":    stage.Idle,
			"skipped": stage.Skipped,
		}
		if stage.RecentError != "" {
			output["recentError"] = stage.RecentError
		}
		data := RunNodeStateData{
			NodeID:   nodeID,
			Status:   state,
			Output:   output,
			Metadata: map[string]any{"source": "hybrid_ecommerce_vps", "title": stage.Title, "type": stage.Type},
		}
		if state == RunStatusRunning {
			data.StartedAt = timeNowRFC3339()
		}
		if state == RunStatusSuccess || state == RunStatusError || state == RunStatusCanceled {
			data.FinishedAt = timeNowRFC3339()
		}
		if state == RunStatusError {
			data.Error = stage.RecentError
		}
		if _, err := writeExecutorNodeState(workspace, runID, current[nodeID], data); err != nil {
			return err
		}
	}
	return nil
}

func syncHybridRemoteArtifacts(ctx context.Context, workspace Workspace, runID string, client hybridVPSClient, config hybridEcommerceConfig, remoteRunID string, products []hybridPDDProduct) (int, error) {
	order := 0
	for _, product := range products {
		if strings.TrimSpace(product.Key) == "" {
			continue
		}
		detail, err := client.productDetail(ctx, remoteRunID, product.Key)
		if err != nil {
			return 0, err
		}
		for _, node := range detail.Nodes {
			for _, artifact := range node.Artifacts {
				if err := syncHybridRemoteFile(ctx, workspace, runID, client, config, remoteRunID, product.Key, node.ID, artifact.Title, artifact.Path, artifact.Kind, artifact.MimeType, "primary_output", "artifact", order); err != nil {
					return 0, err
				}
				order++
			}
			for _, file := range node.Files {
				if file.Kind != "image" && file.Kind != "video" {
					continue
				}
				if err := syncHybridRemoteFile(ctx, workspace, runID, client, config, remoteRunID, product.Key, node.ID, file.Title, file.Path, file.Kind, "", "preview", "file", order); err != nil {
					return 0, err
				}
				order++
			}
		}
	}
	refs, err := listRunArtifactRefs(workspace, runID)
	if err != nil {
		return 0, err
	}
	return len(refs), nil
}

func syncHybridRemoteFile(ctx context.Context, workspace Workspace, runID string, client hybridVPSClient, config hybridEcommerceConfig, remoteRunID string, productKey string, nodeID string, title string, remotePath string, kind string, mimeType string, role string, slot string, order int) error {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" || hybridRemoteArtifactExists(workspace, runID, remoteRunID, remotePath) {
		return nil
	}
	data, contentType, err := client.downloadRunFile(ctx, remoteRunID, remotePath)
	if err != nil {
		return err
	}
	mimeType = firstNonEmptyString(mimeType, contentType, mime.TypeByExtension(filepath.Ext(remotePath)), "application/octet-stream")
	artifactType := firstNonEmptyString(kind, artifactTypeForContentType(mimeType))
	if artifactType == "json" {
		artifactType = "text"
	}
	source := map[string]any{
		"type":             "hybrid_ecommerce_vps",
		"backend":          hybridEcommerceBackend,
		"remoteTemplateId": config.RemoteTemplateID,
		"remoteRunId":      remoteRunID,
		"remotePath":       remotePath,
		"productKey":       productKey,
		"nodeId":           nodeID,
	}
	_, err = createExecutorArtifact(workspace, runID, nodeID, artifactType, mimeType, firstNonEmptyString(title, path.Base(remotePath)), data, role, slot, order, source)
	return err
}

func hybridRemoteArtifactExists(workspace Workspace, runID string, remoteRunID string, remotePath string) bool {
	refs, err := listRunArtifactRefs(workspace, runID)
	if err != nil {
		return false
	}
	for _, ref := range refs {
		artifact, err := ReadArtifact(workspace, ref.Data.ArtifactID)
		if err != nil {
			continue
		}
		if stringFromMap(artifact.Data.Source, "remoteRunId") == remoteRunID && stringFromMap(artifact.Data.Source, "remotePath") == remotePath {
			return true
		}
	}
	return false
}

func hybridEcommerceConfigFromRunTemplate(run Envelope[RunData], template Envelope[TemplateData]) (hybridEcommerceConfig, bool, error) {
	config, ok, err := hybridEcommerceConfigFromTemplate(template)
	if err != nil || !ok {
		return config, ok, err
	}
	if strings.TrimSpace(run.Data.ProfileID) != "" {
		config.ProfileID = run.Data.ProfileID
	}
	if run.Data.Metadata != nil {
		if runHybrid, ok := asMapStringAny(run.Data.Metadata[hybridEcommerceKey]); ok {
			if value := stringFromMap(runHybrid, "remoteRunId"); value != "" {
				config.RemoteRunID = value
			}
			if value := stringFromMap(runHybrid, "profileId"); value != "" {
				config.ProfileID = value
			}
			if value := stringFromMap(runHybrid, "channelId"); value != "" {
				config.ChannelID = value
			}
		}
	}
	return config, true, nil
}

func hybridEcommerceConfigFromTemplate(template Envelope[TemplateData]) (hybridEcommerceConfig, bool, error) {
	config := hybridEcommerceConfig{}
	raw, ok := template.Data.Metadata[hybridEcommerceKey]
	if !ok && template.Data.Settings != nil {
		raw, ok = template.Data.Settings[hybridEcommerceKey]
	}
	if !ok {
		return config, false, nil
	}
	values, ok := asMapStringAny(raw)
	if !ok {
		return config, false, NewError(ErrorWorkspaceInvalid, "hybrid ecommerce metadata is invalid", 2, map[string]string{"templateId": template.ID})
	}
	config.Backend = firstNonEmptyString(stringFromMap(values, "backend"), hybridEcommerceBackend)
	if config.Backend != hybridEcommerceBackend {
		return config, false, nil
	}
	config.RemoteTemplateID = stringFromMap(values, "remoteTemplateId")
	if config.RemoteTemplateID == "" {
		return config, false, NewError(ErrorWorkspaceInvalid, "hybrid ecommerce remoteTemplateId is required", 2, map[string]string{"templateId": template.ID})
	}
	config.BaseURL = stringFromMap(values, "baseUrl")
	config.ProfileID = firstNonEmptyString(stringFromMap(values, "profileId"), stringFromMap(template.Data.Settings, "defaultProfileId"), stringFromMap(template.Data.Settings, "profileId"))
	config.ChannelID = stringFromMap(values, "channelId")
	if ref, err := secretRefFromMap(values["secretRef"]); err != nil {
		return config, false, err
	} else {
		config.SecretRef = ref
	}
	config.PollInterval = durationSecondsFromMap(values, "pollIntervalSeconds")
	config.MaxPoll = durationSecondsFromMap(values, "maxPollSeconds")
	return config, true, nil
}

func hybridEcommerceClient(workspace Workspace, config hybridEcommerceConfig, client *http.Client) (hybridVPSClient, *SecretRefSummary, error) {
	baseURL := strings.TrimSpace(config.BaseURL)
	var ref *SecretRef
	if strings.TrimSpace(config.ProfileID) != "" || strings.TrimSpace(config.ChannelID) != "" {
		channel, err := selectHybridVPSChannel(workspace, config.ProfileID, config.ChannelID)
		if err != nil {
			return hybridVPSClient{}, nil, err
		}
		baseURL = firstNonEmptyString(baseURL, channel.BaseURL)
		ref = channel.SecretRef
	}
	if ref == nil {
		ref = config.SecretRef
	}
	if ref == nil {
		return hybridVPSClient{}, nil, NewError(ErrorWorkspaceInvalid, "hybrid ecommerce secretRef is missing", 2, nil)
	}
	token, err := resolveAIProxySecret(ref)
	if err != nil {
		return hybridVPSClient{}, nil, err
	}
	normalized, err := normalizeHybridBaseURL(baseURL)
	if err != nil {
		return hybridVPSClient{}, nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	summary := ref.Summary()
	return hybridVPSClient{baseURL: normalized, token: token, client: client}, &summary, nil
}

func selectHybridVPSChannel(workspace Workspace, profileID string, channelID string) (ProfileChannel, error) {
	profiles, err := ListProfiles(workspace)
	if err != nil {
		return ProfileChannel{}, err
	}
	if profileID == "" {
		profileID = strings.TrimSpace(workspace.Document.Data.DefaultProfileID)
	}
	for _, profile := range profiles {
		if profileID != "" && profile.ID != profileID {
			continue
		}
		for _, channel := range profile.Data.Channels {
			if channelID != "" && channel.ID != channelID {
				continue
			}
			if !channel.Enabled || channel.SecretRef == nil || strings.TrimSpace(channel.BaseURL) == "" {
				continue
			}
			switch strings.TrimSpace(channel.Protocol) {
			case "ops-canvas-vps", "pdd-console", "hybrid-ecommerce", "ops-canvas-pdd":
				return channel, nil
			}
		}
		if profileID != "" {
			return ProfileChannel{}, NewError(ErrorWorkspaceInvalid, "profile has no usable hybrid ecommerce channel", 2, nil)
		}
	}
	return ProfileChannel{}, NewError(ErrorWorkspaceInvalid, "no usable hybrid ecommerce profile channel", 2, nil)
}

func (client hybridVPSClient) getTemplate(ctx context.Context, templateID string) (hybridPDDTemplate, error) {
	var template hybridPDDTemplate
	err := client.doJSON(ctx, http.MethodGet, "/api/admin/workflows/pdd/templates/"+url.PathEscape(templateID), nil, &template)
	return template, err
}

func (client hybridVPSClient) startRun(ctx context.Context, templateID string, payload map[string]any) (hybridRemoteRunResult, error) {
	var result hybridRemoteRunResult
	err := client.doJSON(ctx, http.MethodPost, "/api/admin/workflows/pdd/templates/"+url.PathEscape(templateID)+"/runs", payload, &result)
	return result, err
}

func (client hybridVPSClient) overview(ctx context.Context, runID string) (hybridPDDOverview, error) {
	var overview hybridPDDOverview
	err := client.doJSON(ctx, http.MethodGet, "/api/workflows/pdd/runs/"+url.PathEscape(runID)+"/overview", nil, &overview)
	return overview, err
}

func (client hybridVPSClient) productDetail(ctx context.Context, runID string, productKey string) (hybridPDDProductDetail, error) {
	var detail hybridPDDProductDetail
	apiPath := "/api/workflows/pdd/runs/" + url.PathEscape(runID) + "/product-detail?key=" + url.QueryEscape(productKey)
	err := client.doJSON(ctx, http.MethodGet, apiPath, nil, &detail)
	return detail, err
}

func (client hybridVPSClient) downloadRunFile(ctx context.Context, runID string, remotePath string) ([]byte, string, error) {
	apiPath := "/api/workflows/pdd/runs/" + url.PathEscape(runID) + "/file?path=" + url.QueryEscape(remotePath)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.url(apiPath), nil)
	if err != nil {
		return nil, "", WrapError(ErrorInternal, "create hybrid ecommerce file request", 5, err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, "", WrapError(ErrorInternal, "call hybrid ecommerce file endpoint", 5, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 128<<20))
	if err != nil {
		return nil, "", WrapError(ErrorInternal, "read hybrid ecommerce file response", 5, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", NewError(ErrorWorkspaceInvalid, "hybrid ecommerce file request failed", 2, map[string]any{"status": response.StatusCode})
	}
	return body, response.Header.Get("Content-Type"), nil
}

func (client hybridVPSClient) doJSON(ctx context.Context, method string, apiPath string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return WrapError(ErrorInternal, "encode hybrid ecommerce request", 5, err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.url(apiPath), body)
	if err != nil {
		return WrapError(ErrorInternal, "create hybrid ecommerce request", 5, err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.client.Do(request)
	if err != nil {
		return WrapError(ErrorInternal, "call hybrid ecommerce endpoint", 5, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 128<<20))
	if err != nil {
		return WrapError(ErrorInternal, "read hybrid ecommerce response", 5, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return NewError(ErrorWorkspaceInvalid, "hybrid ecommerce request failed", 2, map[string]any{"status": response.StatusCode, "message": hybridAPIErrorMessage(responseBody)})
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse hybrid ecommerce response", 2, err)
	}
	if envelope.Code != 0 {
		return NewError(ErrorWorkspaceInvalid, firstNonEmptyString(envelope.Msg, "hybrid ecommerce api failed"), 2, nil)
	}
	if target != nil {
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			return NewError(ErrorWorkspaceInvalid, "hybrid ecommerce response data is empty", 2, nil)
		}
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			return WrapError(ErrorWorkspaceInvalid, "decode hybrid ecommerce response data", 2, err)
		}
	}
	return nil
}

func (client hybridVPSClient) url(apiPath string) string {
	if strings.HasPrefix(apiPath, "http://") || strings.HasPrefix(apiPath, "https://") {
		return apiPath
	}
	return strings.TrimRight(client.baseURL, "/") + "/" + strings.TrimLeft(apiPath, "/")
}

func hybridStartRunPayload(run Envelope[RunData], template Envelope[TemplateData], remoteRunID string) map[string]any {
	input := cloneMap(run.Data.Input)
	payload := map[string]any{
		"runId": remoteRunID,
	}
	if values, ok := input["inputs"].([]map[string]any); ok {
		payload["inputs"] = values
	} else if raw, ok := input["inputs"]; ok {
		var values []map[string]any
		if data, err := json.Marshal(raw); err == nil && json.Unmarshal(data, &values) == nil {
			payload["inputs"] = values
		}
	}
	if _, ok := payload["inputs"]; !ok {
		payload["inputs"] = []map[string]any{}
	}
	for _, key := range []string{"productConcurrency", "maxRetries"} {
		if value, ok := input[key]; ok {
			payload[key] = value
		} else if template.Data.Settings != nil {
			if setting, ok := template.Data.Settings[key]; ok {
				payload[key] = setting
			}
		}
	}
	return payload
}

func hybridLocalRunStatus(overview hybridPDDOverview) string {
	status := strings.TrimSpace(overview.Run.Status)
	switch status {
	case "success", "completed":
		return RunStatusSuccess
	case "error", "failed":
		return RunStatusError
	case "canceled", "cancelled":
		return RunStatusCanceled
	}
	if overview.Run.Completed && overview.Run.FailedProducts == 0 {
		return RunStatusSuccess
	}
	if overview.Run.Completed && overview.Run.FailedProducts > 0 {
		return RunStatusError
	}
	return RunStatusRunning
}

func hybridNodeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "success", "completed":
		return RunStatusSuccess
	case "error", "failed":
		return RunStatusError
	case "running":
		return RunStatusRunning
	case "canceled", "cancelled":
		return RunStatusCanceled
	default:
		return RunStatusPending
	}
}

func patchHybridRunMetadata(workspace Workspace, runID string, patch map[string]any) (Envelope[RunData], error) {
	current, err := ReadRun(workspace, runID)
	if err != nil {
		return Envelope[RunData]{}, err
	}
	data := current.Data
	if data.Metadata == nil {
		data.Metadata = map[string]any{}
	}
	hybrid := hybridEcommerceRunMetadata(current)
	for key, value := range patch {
		if value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			hybrid[key] = value
		}
	}
	data.Metadata[hybridEcommerceKey] = hybrid
	next := nextEnvelopeRevision(current, data)
	if err := SaveRun(workspace, next, SaveRunOptions{}); err != nil {
		return Envelope[RunData]{}, err
	}
	return ReadRun(workspace, runID)
}

func hybridEcommerceRunMetadata(run Envelope[RunData]) map[string]any {
	if run.Data.Metadata == nil {
		return map[string]any{}
	}
	if values, ok := asMapStringAny(run.Data.Metadata[hybridEcommerceKey]); ok {
		return cloneMap(values)
	}
	return map[string]any{}
}

func normalizeHybridBaseURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", NewError(ErrorWorkspaceInvalid, "hybrid ecommerce baseUrl is invalid", 2, nil)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", NewError(ErrorWorkspaceInvalid, "hybrid ecommerce baseUrl scheme is not allowed", 2, nil)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func hybridSecretEnvRef(name string) *SecretRef {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return &SecretRef{Type: SecretRefTypeEnv, Name: name}
}

func secretRefFromMap(value any) (*SecretRef, error) {
	if value == nil {
		return nil, nil
	}
	var ref SecretRef
	data, err := json.Marshal(value)
	if err != nil {
		return nil, WrapError(ErrorWorkspaceInvalid, "encode secretRef", 2, err)
	}
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, WrapError(ErrorWorkspaceInvalid, "decode secretRef", 2, err)
	}
	if strings.TrimSpace(ref.Type) == "" {
		return nil, nil
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return &ref, nil
}

func asMapStringAny(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case json.RawMessage:
		var out map[string]any
		if json.Unmarshal(typed, &out) == nil {
			return out, true
		}
	}
	return nil, false
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func durationSecondsFromMap(values map[string]any, key string) time.Duration {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return time.Duration(typed) * time.Second
	case int64:
		return time.Duration(typed) * time.Second
	case float64:
		return time.Duration(typed) * time.Second
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return time.Duration(parsed) * time.Second
		}
	}
	return 0
}

func hybridAPIErrorMessage(body []byte) string {
	var parsed struct {
		Msg   string `json:"msg"`
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		return firstNonEmptyString(parsed.Msg, parsed.Error)
	}
	return ""
}
