package localworkspace

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultArtifactUploadFileKey = "original"

type serveRunEventPayload struct {
	Type    string         `json:"type"`
	Level   string         `json:"level,omitempty"`
	Actor   RunEventActor  `json:"actor,omitempty"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func (api *serveAPI) createRun(w http.ResponseWriter, r *http.Request) {
	var data RunData
	if _, err := decodeLocalObjectWriteRequest(r, "run", &data); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewRun(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	saveOptions := SaveRunOptions{}
	if templateID := strings.TrimSpace(document.Data.TemplateID); templateID != "" {
		template, err := ReadTemplate(api.workspace, templateID)
		if err != nil {
			var workspaceErr *Error
			if !asLocalWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceNotFound {
				writeServeErrorFromError(w, err)
				return
			}
		} else {
			saveOptions.TemplateSnapshot = &template
		}
	}
	if err := SaveRun(api.workspace, document, saveOptions); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	saved, err := ReadRun(api.workspace, document.ID)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, saved, nil)
}

func (api *serveAPI) updateRun(w http.ResponseWriter, r *http.Request, id string) {
	var data RunData
	payload, err := decodeLocalObjectWriteRequest(r, "run", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadRun(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, data)
	if err := SaveRun(api.workspace, document, SaveRunOptions{}); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) appendRunEvent(w http.ResponseWriter, r *http.Request, runID string) {
	var request struct {
		Event json.RawMessage `json:"event"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "request body is invalid", 1, nil))
		return
	}
	if len(request.Event) == 0 {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "run event is required", 1, nil))
		return
	}
	if err := validateRawNoPlaintextSecrets(request.Event, "run.event"); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	var payload serveRunEventPayload
	if err := json.Unmarshal(request.Event, &payload); err != nil {
		writeServeErrorFromError(w, WrapError(ErrorInvalidArgument, "run event is invalid", 1, err))
		return
	}
	event, err := AppendRunEvent(api.workspace, runID, RunEventInput{
		Type:    payload.Type,
		Level:   payload.Level,
		Actor:   payload.Actor,
		Message: payload.Message,
		Data:    payload.Data,
	})
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, event, nil)
}

func (api *serveAPI) writeRunNodeState(w http.ResponseWriter, r *http.Request, runID string, nodeID string) {
	var data RunNodeStateData
	payload, err := decodeLocalObjectWriteRequest(r, "run.node", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if strings.TrimSpace(data.NodeID) == "" {
		data.NodeID = nodeID
	}
	if data.NodeID != nodeID {
		writeServeErrorFromError(w, NewError(ErrorInvalidArgument, "run node state id mismatch", 1, map[string]string{"id": nodeID, "nodeId": data.NodeID}))
		return
	}
	document, err := NewRunNodeState(nodeID, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if payload.Revision > 0 {
		existing, err := readEnvelopeFile[RunNodeStateData](runNodeStatePath(api.workspace, runID, nodeID))
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		if err := requireRevision(existing.Revision, payload.Revision); err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		document = nextEnvelopeRevision(existing, data)
	}
	if err := WriteRunNodeState(api.workspace, runID, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) attachRunArtifact(w http.ResponseWriter, r *http.Request, runID string) {
	var data RunArtifactRefData
	payload, err := decodeLocalObjectWriteRequest(r, "run.artifact_ref", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if strings.TrimSpace(data.ArtifactID) == "" {
		writeServeErrorFromError(w, NewError(ErrorInvalidArgument, "artifactId is required", 1, nil))
		return
	}
	document := Envelope[RunArtifactRefData]{
		SchemaVersion: SchemaVersion,
		Kind:          KindRunArtifactRef,
		ID:            data.ArtifactID,
		Revision:      1,
		CreatedAt:     timeNowRFC3339(),
		UpdatedAt:     timeNowRFC3339(),
		Data:          data,
	}
	if payload.Revision > 0 {
		existing, err := readEnvelopeFile[RunArtifactRefData](runArtifactRefPath(api.workspace, runID, data.ArtifactID))
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		if err := requireRevision(existing.Revision, payload.Revision); err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		document = nextEnvelopeRevision(existing, data)
	}
	if err := WriteRunArtifactRef(api.workspace, runID, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) handleArtifacts(w http.ResponseWriter) {
	items, err := ListArtifactSummaries(api.workspace)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"artifacts": items}, nil)
}

func (api *serveAPI) createArtifact(w http.ResponseWriter, r *http.Request) {
	var data ArtifactData
	if _, err := decodeLocalObjectWriteRequest(r, "artifact", &data); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewArtifact(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteArtifact(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) updateArtifact(w http.ResponseWriter, r *http.Request, id string) {
	var data ArtifactData
	payload, err := decodeLocalObjectWriteRequest(r, "artifact", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadArtifact(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document := nextEnvelopeRevision(existing, data)
	if err := WriteArtifact(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) createArtifactImport(w http.ResponseWriter, r *http.Request) {
	data, file, header, fileKey, contentType, err := decodeArtifactImportRequest(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	defer file.Close()
	document, err := NewArtifact(api.workspace, normalizeImportedArtifactData(data, fileKey, contentType, header))
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	relPath, err := artifactUploadRelPath(fileKey, contentType)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document.Data.Files[fileKey] = relPath
	filePath := filepath.Join(ArtifactRepository(api.workspace).Dir(document.ID), filepath.FromSlash(relPath))
	if err := AtomicWriteFromReader(filePath, file, 0o600); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteArtifact(api.workspace, document); err != nil {
		_ = os.RemoveAll(ArtifactRepository(api.workspace).Dir(document.ID))
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) updateArtifactImport(w http.ResponseWriter, r *http.Request, id string) {
	data, file, header, fileKey, contentType, err := decodeArtifactImportRequest(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	defer file.Close()
	revision, err := parseArtifactImportRevision(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	existing, err := ReadArtifact(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	data = normalizeImportedArtifactData(data, fileKey, contentType, header)
	if data.Files == nil {
		data.Files = map[string]string{}
	}
	for key, value := range existing.Data.Files {
		if _, ok := data.Files[key]; !ok {
			data.Files[key] = value
		}
	}
	relPath, err := artifactUploadRelPath(fileKey, contentType)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	data.Files[fileKey] = relPath
	document := nextEnvelopeRevision(existing, data)
	filePath := filepath.Join(ArtifactRepository(api.workspace).Dir(document.ID), filepath.FromSlash(relPath))
	if err := AtomicWriteFromReader(filePath, file, 0o600); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := WriteArtifact(api.workspace, document); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func decodeArtifactImportRequest(r *http.Request) (ArtifactData, multipart.File, *multipart.FileHeader, string, string, error) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return ArtifactData{}, nil, nil, "", "", WrapError(ErrorInvalidArgument, "multipart request is invalid", 1, err)
	}
	raw := strings.TrimSpace(r.FormValue("data"))
	if raw == "" {
		return ArtifactData{}, nil, nil, "", "", NewError(ErrorInvalidArgument, "request data is required", 1, nil)
	}
	dataRaw := json.RawMessage(raw)
	if err := validateRawNoPlaintextSecrets(dataRaw, "artifact"); err != nil {
		return ArtifactData{}, nil, nil, "", "", err
	}
	var data ArtifactData
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return ArtifactData{}, nil, nil, "", "", WrapError(ErrorInvalidArgument, "request data is invalid", 1, err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return ArtifactData{}, nil, nil, "", "", NewError(ErrorInvalidArgument, "artifact file is required", 1, nil)
	}
	contentType, err := sniffMultipartFile(file, header)
	if err != nil {
		_ = file.Close()
		return ArtifactData{}, nil, nil, "", "", err
	}
	fileKey := strings.TrimSpace(r.FormValue("fileKey"))
	if fileKey == "" {
		fileKey = defaultArtifactUploadFileKey
	}
	if err := validatePathComponent("artifact file key", fileKey); err != nil {
		_ = file.Close()
		return ArtifactData{}, nil, nil, "", "", err
	}
	return data, file, header, fileKey, contentType, nil
}

func normalizeImportedArtifactData(data ArtifactData, fileKey string, contentType string, header *multipart.FileHeader) ArtifactData {
	if strings.TrimSpace(data.Type) == "" {
		data.Type = artifactTypeForContentType(contentType)
	}
	if strings.TrimSpace(data.MIME) == "" {
		data.MIME = contentType
	}
	if strings.TrimSpace(data.Privacy) == "" {
		data.Privacy = PrivacyPrivate
	}
	if data.Files == nil {
		data.Files = map[string]string{}
	}
	if data.Metadata == nil {
		data.Metadata = map[string]any{}
	}
	if header != nil {
		data.Metadata["fileName"] = filepath.Base(header.Filename)
		if data.Bytes <= 0 {
			data.Bytes = header.Size
		}
	}
	data.Metadata["fileKey"] = fileKey
	return data
}

func artifactUploadRelPath(fileKey string, contentType string) (string, error) {
	if err := validatePathComponent("artifact file key", fileKey); err != nil {
		return "", err
	}
	return path.Join("files", fileKey+extensionForContentType(contentType)), nil
}

func artifactTypeForContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	case strings.HasPrefix(contentType, "text/") || contentType == "application/json":
		return "text"
	default:
		return "file"
	}
}

func parseArtifactImportRevision(r *http.Request) (int, error) {
	value := strings.TrimSpace(r.FormValue("revision"))
	if value == "" {
		return 0, NewError(ErrorInvalidArgument, "revision is required for update", 1, nil)
	}
	revision, err := strconv.Atoi(value)
	if err != nil {
		return 0, NewError(ErrorInvalidArgument, "revision is invalid", 1, nil)
	}
	return revision, nil
}
