package localworkspace

import (
	"encoding/json"
	"net/http"
	"strings"
)

type localObjectWriteRequest struct {
	Revision int             `json:"revision"`
	Data     json.RawMessage `json:"data"`
	Content  *string         `json:"content,omitempty"`
}

type sanitizedEnvelope[T any] struct {
	SchemaVersion string `json:"schemaVersion"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Revision      int    `json:"revision"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	Data          T      `json:"data"`
}

type sanitizedProfileData struct {
	Name     string                    `json:"name"`
	Mode     string                    `json:"mode,omitempty"`
	Channels []sanitizedProfileChannel `json:"channels,omitempty"`
	Metadata map[string]any            `json:"metadata,omitempty"`
}

type sanitizedProfileChannel struct {
	ID        string            `json:"id"`
	Name      string            `json:"name,omitempty"`
	Protocol  string            `json:"protocol,omitempty"`
	BaseURL   string            `json:"baseUrl,omitempty"`
	Models    []string          `json:"models,omitempty"`
	Weight    int               `json:"weight,omitempty"`
	Enabled   bool              `json:"enabled,omitempty"`
	SecretRef *SecretRefSummary `json:"secretRef,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

type sanitizedProjectData struct {
	Name            string                      `json:"name"`
	Kind            string                      `json:"kind,omitempty"`
	Adapter         string                      `json:"adapter,omitempty"`
	RootPath        string                      `json:"rootPath,omitempty"`
	HasRootPath     bool                        `json:"hasRootPath"`
	RootFingerprint string                      `json:"rootFingerprint,omitempty"`
	Capabilities    ProjectCapabilities         `json:"capabilities,omitempty"`
	Execution       ProjectExecution            `json:"execution,omitempty"`
	AdapterMetadata map[string]any              `json:"adapterMetadata,omitempty"`
	CredentialRefs  map[string]SecretRefSummary `json:"credentialRefs,omitempty"`
	Metadata        map[string]any              `json:"metadata,omitempty"`
}

func (api *serveAPI) handleProfileRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/profiles/")
	if localPath == "/profiles" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListProfileSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"profiles": items}, nil)
		case http.MethodPost:
			api.createProfile(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "profile route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadProfile(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, sanitizeProfile(document), nil)
	case http.MethodPut:
		api.updateProfile(w, r, parts[0])
	case http.MethodDelete:
		api.deleteProfile(w, parts[0])
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) handleProjectRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/projects/")
	showPaths := parseBoolQuery(r, "showPaths")
	if localPath == "/projects" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListProjectSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"projects": items}, nil)
		case http.MethodPost:
			api.createProject(w, r, showPaths)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "project route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadProject(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, sanitizeProject(document, showPaths), nil)
	case http.MethodPut:
		api.updateProject(w, r, parts[0], showPaths)
	case http.MethodDelete:
		api.deleteProject(w, parts[0])
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) handleTemplateRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/templates/")
	if localPath == "/templates" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListTemplateSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"templates": items}, nil)
		case http.MethodPost:
			api.createTemplate(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "template route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadTemplate(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, document, nil)
	case http.MethodPut:
		api.updateTemplate(w, r, parts[0])
	case http.MethodDelete:
		api.deleteTemplate(w, parts[0])
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) handleAssetRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/assets/")
	if localPath == "/assets/import" && r.Method == http.MethodPost {
		api.createAssetImport(w, r)
		return
	}
	if len(parts) == 3 && parts[1] == "files" && r.Method == http.MethodGet {
		api.handleAssetFileRoute(w, r, localPath)
		return
	}
	if len(parts) == 2 && parts[1] == "import" && r.Method == http.MethodPut {
		api.updateAssetImport(w, r, parts[0])
		return
	}
	if localPath == "/assets" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListAssetSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"assets": items}, nil)
		case http.MethodPost:
			api.createAsset(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "asset route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadAsset(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, document, nil)
	case http.MethodPut:
		api.updateAsset(w, r, parts[0])
	case http.MethodDelete:
		api.deleteAsset(w, parts[0])
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) handlePromptRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/prompts/")
	if len(parts) == 2 && parts[1] == "content" && r.Method == http.MethodGet {
		api.handlePromptContentRoute(w, r, localPath)
		return
	}
	if localPath == "/prompts" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListPromptSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"prompts": items}, nil)
		case http.MethodPost:
			api.createPrompt(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "prompt route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadPrompt(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, document, nil)
	case http.MethodPut:
		api.updatePrompt(w, r, parts[0])
	case http.MethodDelete:
		api.deletePrompt(w, parts[0])
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) createTemplate(w http.ResponseWriter, r *http.Request) {
	var data TemplateData
	if _, err := decodeLocalObjectWriteRequest(r, "template", &data); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewTemplate(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteTemplate(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) updateTemplate(w http.ResponseWriter, r *http.Request, id string) {
	var data TemplateData
	payload, err := decodeLocalObjectWriteRequest(r, "template", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadTemplate(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, normalizeTemplateData(data))
	if err := WriteTemplate(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) createProfile(w http.ResponseWriter, r *http.Request) {
	var data ProfileData
	if _, err := decodeLocalObjectWriteRequest(r, "profile", &data); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewProfile(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteProfile(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, sanitizeProfile(document), nil)
}

func (api *serveAPI) updateProfile(w http.ResponseWriter, r *http.Request, id string) {
	var data ProfileData
	payload, err := decodeLocalObjectWriteRequest(r, "profile", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadProfile(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, data)
	if err := WriteProfile(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, sanitizeProfile(document), nil)
}

func (api *serveAPI) createProject(w http.ResponseWriter, r *http.Request, showPaths bool) {
	var data ProjectData
	if _, err := decodeLocalObjectWriteRequest(r, "project", &data); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewProject(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteProject(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, sanitizeProject(document, showPaths), nil)
}

func (api *serveAPI) updateProject(w http.ResponseWriter, r *http.Request, id string, showPaths bool) {
	var data ProjectData
	payload, err := decodeLocalObjectWriteRequest(r, "project", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadProject(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, data)
	if err := WriteProject(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, sanitizeProject(document, showPaths), nil)
}

func (api *serveAPI) createAsset(w http.ResponseWriter, r *http.Request) {
	var data AssetData
	if _, err := decodeLocalObjectWriteRequest(r, "asset", &data); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewAsset(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteAsset(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) updateAsset(w http.ResponseWriter, r *http.Request, id string) {
	var data AssetData
	payload, err := decodeLocalObjectWriteRequest(r, "asset", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadAsset(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, data)
	if err := WriteAsset(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) createPrompt(w http.ResponseWriter, r *http.Request) {
	var data PromptData
	payload, err := decodeLocalObjectWriteRequest(r, "prompt", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewPrompt(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if payload.Content != nil {
		err = SavePrompt(api.workspace, document, *payload.Content)
	} else {
		err = WritePrompt(api.workspace, document)
	}
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) updatePrompt(w http.ResponseWriter, r *http.Request, id string) {
	var data PromptData
	payload, err := decodeLocalObjectWriteRequest(r, "prompt", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadPrompt(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, data)
	if payload.Content != nil {
		err = SavePrompt(api.workspace, document, *payload.Content)
	} else {
		err = WritePrompt(api.workspace, document)
	}
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) deleteProfile(w http.ResponseWriter, id string) {
	if err := DeleteProfile(api.workspace, id); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"deleted": true, "id": id}, nil)
}

func (api *serveAPI) deleteTemplate(w http.ResponseWriter, id string) {
	if err := DeleteTemplate(api.workspace, id); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"deleted": true, "id": id}, nil)
}

func (api *serveAPI) deleteProject(w http.ResponseWriter, id string) {
	if err := DeleteProject(api.workspace, id); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"deleted": true, "id": id}, nil)
}

func (api *serveAPI) deleteAsset(w http.ResponseWriter, id string) {
	if err := DeleteAsset(api.workspace, id); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"deleted": true, "id": id}, nil)
}

func (api *serveAPI) deletePrompt(w http.ResponseWriter, id string) {
	if err := DeletePrompt(api.workspace, id); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"deleted": true, "id": id}, nil)
}

func decodeLocalObjectWriteRequest[T any](r *http.Request, scope string, data *T) (localObjectWriteRequest, error) {
	var payload localObjectWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return payload, NewError(ErrorInvalidArgument, "request body is invalid", 1, nil)
	}
	if len(payload.Data) == 0 {
		return payload, NewError(ErrorInvalidArgument, "request data is required", 1, nil)
	}
	if err := validateRawNoPlaintextSecrets(payload.Data, scope); err != nil {
		return payload, err
	}
	if err := json.Unmarshal(payload.Data, data); err != nil {
		return payload, WrapError(ErrorInvalidArgument, "request data is invalid", 1, err)
	}
	return payload, nil
}

func requireRevision(current int, requested int) error {
	if requested <= 0 {
		return NewError(ErrorInvalidArgument, "revision is required for update", 1, nil)
	}
	if requested != current {
		return NewError(ErrorWorkspaceInvalid, "revision conflict", 2, map[string]int{"current": current, "requested": requested})
	}
	return nil
}

func nextEnvelopeRevision[T any](existing Envelope[T], data T) Envelope[T] {
	existing.Revision++
	existing.UpdatedAt = timeNowRFC3339()
	existing.Data = data
	return existing
}

func sanitizeProfile(document Envelope[ProfileData]) sanitizedEnvelope[sanitizedProfileData] {
	channels := make([]sanitizedProfileChannel, 0, len(document.Data.Channels))
	for _, channel := range document.Data.Channels {
		channels = append(channels, sanitizedProfileChannel{
			ID:        channel.ID,
			Name:      channel.Name,
			Protocol:  channel.Protocol,
			BaseURL:   channel.BaseURL,
			Models:    append([]string{}, channel.Models...),
			Weight:    channel.Weight,
			Enabled:   channel.Enabled,
			SecretRef: secretRefSummary(channel.SecretRef),
			Metadata:  channel.Metadata,
		})
	}
	return sanitizedEnvelope[sanitizedProfileData]{
		SchemaVersion: document.SchemaVersion,
		Kind:          document.Kind,
		ID:            document.ID,
		Revision:      document.Revision,
		CreatedAt:     document.CreatedAt,
		UpdatedAt:     document.UpdatedAt,
		Data: sanitizedProfileData{
			Name:     document.Data.Name,
			Mode:     document.Data.Mode,
			Channels: channels,
			Metadata: document.Data.Metadata,
		},
	}
}

func sanitizeProject(document Envelope[ProjectData], showPaths bool) sanitizedEnvelope[sanitizedProjectData] {
	refs := map[string]SecretRefSummary{}
	for name, ref := range document.Data.CredentialRefs {
		refs[name] = ref.Summary()
	}
	data := sanitizedProjectData{
		Name:            document.Data.Name,
		Kind:            document.Data.Kind,
		Adapter:         document.Data.Adapter,
		HasRootPath:     strings.TrimSpace(document.Data.RootPath) != "",
		RootFingerprint: document.Data.RootFingerprint,
		Capabilities:    document.Data.Capabilities,
		Execution:       document.Data.Execution,
		AdapterMetadata: document.Data.AdapterMetadata,
		CredentialRefs:  refs,
		Metadata:        document.Data.Metadata,
	}
	if showPaths {
		data.RootPath = document.Data.RootPath
	}
	return sanitizedEnvelope[sanitizedProjectData]{
		SchemaVersion: document.SchemaVersion,
		Kind:          document.Kind,
		ID:            document.ID,
		Revision:      document.Revision,
		CreatedAt:     document.CreatedAt,
		UpdatedAt:     document.UpdatedAt,
		Data:          data,
	}
}

func secretRefSummary(ref *SecretRef) *SecretRefSummary {
	if ref == nil {
		return nil
	}
	summary := ref.Summary()
	return &summary
}

func parseBoolQuery(r *http.Request, name string) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(name)))
	return value == "1" || value == "true" || value == "yes"
}
