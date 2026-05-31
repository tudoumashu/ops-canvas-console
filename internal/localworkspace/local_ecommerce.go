package localworkspace

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	localEcommerceKey                = "localEcommerce"
	localEcommerceBackend            = "local_first"
	localEcommerceVersion            = 1
	localEcommerceMaterialLibrary    = "anime_ip"
	localEcommerceMaterialLibraryEnv = "OPSC_LOCAL_ECOMMERCE_MATERIAL_LIBRARY"
	defaultEcommerceProjectOutRoot   = "outputs/ecommerce"
)

type LocalEcommerceImportOptions struct {
	WorkspacePath       string
	BaseURL             string
	RemoteTemplateID    string
	ProfileID           string
	ChannelID           string
	ProjectID           string
	SecretEnv           string
	MaterialLibraryPath string
	HTTPClient          *http.Client
}

type LocalEcommerceImportResult struct {
	Created          bool                      `json:"created"`
	Template         Envelope[TemplateData]    `json:"template"`
	RemoteTemplateID string                    `json:"remoteTemplateId"`
	ProfileID        string                    `json:"profileId,omitempty"`
	ChannelID        string                    `json:"channelId,omitempty"`
	ProjectID        string                    `json:"projectId,omitempty"`
	Mode             string                    `json:"mode"`
	Warnings         []string                  `json:"warnings,omitempty"`
	SecretRef        *SecretRefSummary         `json:"secretRef,omitempty"`
	Remote           hybridRemoteTemplateBrief `json:"remote"`
}

func ImportLocalEcommerceTemplate(ctx context.Context, opts LocalEcommerceImportOptions) (LocalEcommerceImportResult, error) {
	workspace, err := Open(opts.WorkspacePath)
	if err != nil {
		return LocalEcommerceImportResult{}, err
	}
	opts.RemoteTemplateID = strings.TrimSpace(opts.RemoteTemplateID)
	if opts.RemoteTemplateID == "" {
		return LocalEcommerceImportResult{}, NewError(ErrorInvalidArgument, "remote template id is required", 1, nil)
	}
	client, summary, err := hybridEcommerceClient(*workspace, hybridEcommerceConfig{
		BaseURL:   opts.BaseURL,
		ProfileID: opts.ProfileID,
		ChannelID: opts.ChannelID,
		SecretRef: hybridSecretEnvRef(opts.SecretEnv),
	}, opts.HTTPClient)
	if err != nil {
		return LocalEcommerceImportResult{}, err
	}
	remote, err := client.getTemplate(ctx, opts.RemoteTemplateID)
	if err != nil {
		return LocalEcommerceImportResult{}, err
	}
	document, created, err := upsertLocalEcommerceTemplate(*workspace, remote, opts)
	if err != nil {
		return LocalEcommerceImportResult{}, err
	}
	result := LocalEcommerceImportResult{
		Created:          created,
		Template:         document,
		RemoteTemplateID: remote.ID,
		ProfileID:        strings.TrimSpace(opts.ProfileID),
		ChannelID:        strings.TrimSpace(opts.ChannelID),
		ProjectID:        strings.TrimSpace(opts.ProjectID),
		Mode:             localEcommerceBackend,
		Remote: hybridRemoteTemplateBrief{
			ID:        remote.ID,
			Title:     remote.Title,
			UpdatedAt: remote.UpdatedAt,
		},
	}
	if summary != nil {
		result.SecretRef = summary
	}
	if strings.TrimSpace(opts.ProfileID) == "" && strings.TrimSpace(opts.SecretEnv) != "" {
		result.Warnings = append(result.Warnings, "direct env secretRef is for CLI smoke diagnostics; use a workspace profile/channel secretRef for Web and watch worker runs")
	}
	return result, nil
}

func upsertLocalEcommerceTemplate(workspace Workspace, remote hybridPDDTemplate, opts LocalEcommerceImportOptions) (Envelope[TemplateData], bool, error) {
	remote.ID = strings.TrimSpace(remote.ID)
	if remote.ID == "" {
		return Envelope[TemplateData]{}, false, NewError(ErrorWorkspaceInvalid, "remote template id is empty", 2, nil)
	}
	settings := cloneMap(remote.Spec.Settings)
	if settings == nil {
		settings = map[string]any{}
	}
	if strings.TrimSpace(opts.ProfileID) != "" {
		settings["defaultProfileId"] = strings.TrimSpace(opts.ProfileID)
	}
	if strings.TrimSpace(opts.ProjectID) != "" {
		settings["defaultProjectId"] = strings.TrimSpace(opts.ProjectID)
	}
	nodes, err := localizeEcommerceTemplateNodes(remote.Spec.Nodes, strings.TrimSpace(opts.MaterialLibraryPath))
	if err != nil {
		return Envelope[TemplateData]{}, false, err
	}
	metadata := map[string]any{
		"source": "local_ecommerce_vps_template",
		localEcommerceKey: map[string]any{
			"version":                localEcommerceVersion,
			"backend":                localEcommerceBackend,
			"sourceRemoteTemplateId": remote.ID,
			"remoteTitle":            remote.Title,
			"remoteUpdatedAt":        remote.UpdatedAt,
			"importedAt":             timeNowRFC3339(),
			"sourceFingerprint":      hybridRemoteTemplateFingerprint(remote),
			"profileId":              strings.TrimSpace(opts.ProfileID),
			"channelId":              strings.TrimSpace(opts.ChannelID),
			"projectId":              strings.TrimSpace(opts.ProjectID),
			"materialLibrary":        localEcommerceMaterialLibrary,
			"projectOutputRoot":      defaultEcommerceProjectOutRoot,
		},
	}
	if pathValue := strings.TrimSpace(opts.MaterialLibraryPath); pathValue != "" {
		metadata[localEcommerceKey].(map[string]any)["materialLibraryPath"] = pathValue
	}
	data := TemplateData{
		Title:        nonEmptyString(remote.Title, "Local Ecommerce Template"),
		Description:  remote.Description,
		WorkflowType: firstNonEmptyString(remote.WorkflowType, "pdd"),
		Version:      remote.Spec.Version,
		Nodes:        nodes,
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
	existing, ok, err := findLocalEcommerceTemplate(workspace, remote.ID)
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

func localizeEcommerceTemplateNodes(rawNodes []json.RawMessage, materialLibraryPath string) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(rawNodes))
	for _, raw := range rawNodes {
		var node map[string]any
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, WrapError(ErrorWorkspaceInvalid, "parse ecommerce template node", 2, err)
		}
		extra, _ := asStringAnyMap(node["extra"])
		if extra == nil {
			extra = map[string]any{}
		}
		operation := strings.TrimSpace(stringFromAny(firstNonNil(node["operation"], node["type"])))
		nodeID := strings.TrimSpace(stringFromAny(node["id"]))
		switch operation {
		case "material_lookup", "material":
			assetID := strings.TrimSpace(stringFromAny(extra["assetId"]))
			if assetID == "" {
				extra["assetMode"] = "auto"
				extra["materialLibrary"] = localEcommerceMaterialLibrary
				if strings.TrimSpace(materialLibraryPath) != "" {
					extra["materialLibraryPath"] = strings.TrimSpace(materialLibraryPath)
				}
			} else if assetID == builtinPDDMockupBaseAssetID {
				extra["assetMode"] = "fixed"
				extra["fallback"] = "builtin_pdd_mockup_base"
			}
		case "script":
			switch nodeID {
			case "package":
				extra["executor"] = "local"
				extra["localEcommerceAction"] = "package"
				if strings.TrimSpace(stringFromAny(extra["outputRoot"])) == "" {
					extra["outputRoot"] = defaultEcommerceProjectOutRoot
				}
			case "sync_local":
				extra["executor"] = "local"
				extra["localEcommerceAction"] = "sync_local"
				if strings.TrimSpace(stringFromAny(extra["outputRoot"])) == "" {
					extra["outputRoot"] = defaultEcommerceProjectOutRoot
				}
			}
		}
		node["extra"] = extra
		encoded, err := json.Marshal(node)
		if err != nil {
			return nil, WrapError(ErrorInternal, "encode ecommerce template node", 5, err)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func findLocalEcommerceTemplate(workspace Workspace, remoteTemplateID string) (Envelope[TemplateData], bool, error) {
	templates, err := ListTemplates(workspace)
	if err != nil {
		return Envelope[TemplateData]{}, false, err
	}
	for _, template := range templates {
		config, ok := localEcommerceConfigFromTemplate(template)
		if ok && config.SourceRemoteTemplateID == remoteTemplateID {
			return template, true, nil
		}
	}
	return Envelope[TemplateData]{}, false, nil
}

type localEcommerceConfig struct {
	Backend                string
	SourceRemoteTemplateID string
	ProfileID              string
	ChannelID              string
	MaterialLibraryPath    string
	ProjectOutputRoot      string
}

func localEcommerceConfigFromTemplate(template Envelope[TemplateData]) (localEcommerceConfig, bool) {
	raw, ok := template.Data.Metadata[localEcommerceKey]
	if !ok {
		raw, ok = template.Data.Settings[localEcommerceKey]
	}
	values, ok := asMapStringAny(raw)
	if !ok {
		return localEcommerceConfig{}, false
	}
	backend := firstNonEmptyString(stringFromMap(values, "backend"), localEcommerceBackend)
	if backend != localEcommerceBackend {
		return localEcommerceConfig{}, false
	}
	return localEcommerceConfig{
		Backend:                backend,
		SourceRemoteTemplateID: firstNonEmptyString(stringFromMap(values, "sourceRemoteTemplateId"), stringFromMap(values, "remoteTemplateId")),
		ProfileID:              stringFromMap(values, "profileId"),
		ChannelID:              stringFromMap(values, "channelId"),
		MaterialLibraryPath:    stringFromMap(values, "materialLibraryPath"),
		ProjectOutputRoot:      firstNonEmptyString(stringFromMap(values, "projectOutputRoot"), defaultEcommerceProjectOutRoot),
	}, true
}

func CreateEcommerceRun(ctx context.Context, opts HybridEcommerceRunOptions) (HybridEcommerceRunResult, error) {
	workspace, err := Open(opts.WorkspacePath)
	if err != nil {
		return HybridEcommerceRunResult{}, err
	}
	templateID := strings.TrimSpace(opts.TemplateID)
	if templateID == "" {
		return HybridEcommerceRunResult{}, NewError(ErrorInvalidArgument, "template id is required", 1, nil)
	}
	template, err := ReadTemplate(*workspace, templateID)
	if err != nil {
		return HybridEcommerceRunResult{}, err
	}
	if _, ok, err := hybridEcommerceConfigFromTemplate(template); err != nil {
		return HybridEcommerceRunResult{}, err
	} else if ok {
		return CreateHybridEcommerceRun(ctx, opts)
	}
	config, ok := localEcommerceConfigFromTemplate(template)
	if !ok {
		return HybridEcommerceRunResult{}, NewError(ErrorWorkspaceInvalid, "template is not an ecommerce executable template", 2, map[string]string{"templateId": template.ID})
	}
	return createLocalEcommerceRun(*workspace, template, opts, config)
}

func createLocalEcommerceRun(workspace Workspace, template Envelope[TemplateData], opts HybridEcommerceRunOptions, config localEcommerceConfig) (HybridEcommerceRunResult, error) {
	runInput := hybridRunInputWithTemplateDefaults(opts.Input, template)
	if err := validateNoPlaintextSecrets(runInput, "local_ecommerce.run.input"); err != nil {
		return HybridEcommerceRunResult{}, err
	}
	profileID := firstNonEmptyString(strings.TrimSpace(opts.ProfileID), config.ProfileID, stringFromMap(template.Data.Settings, "defaultProfileId"), stringFromMap(template.Data.Settings, "profileId"))
	channelID := firstNonEmptyString(strings.TrimSpace(opts.ChannelID), config.ChannelID)
	projectID := firstNonEmptyString(strings.TrimSpace(opts.ProjectID), stringFromMap(template.Data.Settings, "defaultProjectId"), stringFromMap(template.Data.Settings, "projectId"))
	run, err := NewRun(workspace, RunData{
		TemplateID: template.ID,
		Status:     RunStatusPending,
		ProfileID:  profileID,
		ProjectID:  projectID,
		Input:      runInput,
		Metadata: map[string]any{
			"source":           "opsc_ecommerce_cli",
			"workflowType":     firstNonEmptyString(template.Data.WorkflowType, "pdd"),
			"templateTitle":    template.Data.Title,
			"templateRevision": template.Revision,
			"executor":         "opsc",
			localEcommerceKey: map[string]any{
				"backend":                localEcommerceBackend,
				"sourceRemoteTemplateId": config.SourceRemoteTemplateID,
				"profileId":              profileID,
				"channelId":              channelID,
				"projectId":              projectID,
				"projectOutputRoot":      firstNonEmptyString(config.ProjectOutputRoot, defaultEcommerceProjectOutRoot),
			},
		},
	})
	if err != nil {
		return HybridEcommerceRunResult{}, err
	}
	if err := SaveRun(workspace, run, SaveRunOptions{TemplateSnapshot: &template}); err != nil {
		return HybridEcommerceRunResult{}, err
	}
	if err := writeLocalEcommerceTemplatePendingNodeStates(workspace, run.ID, template); err != nil {
		return HybridEcommerceRunResult{}, err
	}
	if _, err := AppendRunEvent(workspace, run.ID, RunEventInput{
		Type:    "run.waiting_for_executor",
		Level:   "info",
		Actor:   RunEventActor{Type: "cli", ID: "opsc"},
		Message: "Local ecommerce run created, waiting for executor.",
		Data: map[string]any{
			"templateId":   template.ID,
			"workflowType": firstNonEmptyString(template.Data.WorkflowType, "pdd"),
			"mode":         localEcommerceBackend,
		},
	}); err != nil {
		return HybridEcommerceRunResult{}, err
	}
	saved, err := ReadRun(workspace, run.ID)
	if err != nil {
		return HybridEcommerceRunResult{}, err
	}
	return HybridEcommerceRunResult{
		Run:        saved,
		TemplateID: template.ID,
		ProfileID:  profileID,
		ChannelID:  channelID,
		ProjectID:  projectID,
		Mode:       localEcommerceBackend,
	}, nil
}

func writeLocalEcommerceTemplatePendingNodeStates(workspace Workspace, runID string, template Envelope[TemplateData]) error {
	for _, raw := range template.Data.Nodes {
		var node struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Type      string `json:"type"`
			Operation string `json:"operation"`
		}
		if err := json.Unmarshal(raw, &node); err != nil || strings.TrimSpace(node.ID) == "" {
			continue
		}
		state, err := NewRunNodeState(node.ID, RunNodeStateData{
			NodeID: node.ID,
			Status: RunStatusPending,
			Metadata: map[string]any{
				"source":    "local_ecommerce_template",
				"title":     node.Title,
				"type":      node.Type,
				"operation": node.Operation,
			},
		})
		if err != nil {
			return err
		}
		if err := WriteRunNodeState(workspace, runID, state); err != nil {
			return err
		}
	}
	return nil
}
