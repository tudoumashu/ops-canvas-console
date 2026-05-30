package localworkspace

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (api *serveAPI) handleCanvasProjectRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/canvas-projects/")
	if localPath == "/canvas-projects" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListCanvasProjectSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"canvasProjects": items}, nil)
		case http.MethodPost:
			api.createCanvasProject(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) == 3 && parts[1] == "files" && r.Method == http.MethodGet {
		api.handleCanvasProjectFileRoute(w, r, parts[0], parts[2])
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "canvas project route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadCanvasProject(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, document, nil)
	case http.MethodPut:
		api.updateCanvasProject(w, r, parts[0])
	case http.MethodDelete:
		api.deleteCanvasProject(w, parts[0])
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) createCanvasProject(w http.ResponseWriter, r *http.Request) {
	var data CanvasProjectData
	files, err := decodeCanvasProjectWriteRequest(r, "canvas_project", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	defer closeCanvasProjectFiles(files)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	document, err := NewCanvasProject(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := SaveCanvasProject(api.workspace, document, files); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	saved, err := ReadCanvasProject(api.workspace, document.ID)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, saved, nil)
}

func (api *serveAPI) updateCanvasProject(w http.ResponseWriter, r *http.Request, id string) {
	var data CanvasProjectData
	payload, files, err := decodeCanvasProjectUpdateRequest(r, "canvas_project", &data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	defer closeCanvasProjectFiles(files)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	existing, err := ReadCanvasProject(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := requireRevision(existing.Revision, payload.Revision); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if data.Files == nil {
		data.Files = existing.Data.Files
	}
	document := nextEnvelopeRevision(existing, data)
	if err := SaveCanvasProject(api.workspace, document, files); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	saved, err := ReadCanvasProject(api.workspace, document.ID)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, saved, nil)
}

func (api *serveAPI) deleteCanvasProject(w http.ResponseWriter, id string) {
	if err := DeleteCanvasProject(api.workspace, id); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{"deleted": true, "id": id}, nil)
}

func (api *serveAPI) handleCanvasProjectFileRoute(w http.ResponseWriter, r *http.Request, id string, fileKey string) {
	document, err := ReadCanvasProject(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	meta, ok := document.Data.Files[fileKey]
	if !ok || strings.TrimSpace(meta.Path) == "" {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "canvas project file ref not found", 2, nil))
		return
	}
	serveObjectFile(w, r, CanvasProjectRepository(api.workspace).Dir(document.ID), meta.Path, meta.MIME)
}

func decodeCanvasProjectWriteRequest(r *http.Request, scope string, data *CanvasProjectData) ([]CanvasProjectImportFile, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if _, err := decodeLocalObjectWriteRequest(r, scope, data); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, WrapError(ErrorInvalidArgument, "multipart request is invalid", 1, err)
	}
	raw := strings.TrimSpace(r.FormValue("data"))
	if raw == "" {
		return nil, NewError(ErrorInvalidArgument, "request data is required", 1, nil)
	}
	dataRaw := json.RawMessage(raw)
	if err := validateRawNoPlaintextSecrets(dataRaw, scope); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(dataRaw, data); err != nil {
		return nil, WrapError(ErrorInvalidArgument, "request data is invalid", 1, err)
	}
	files, err := readCanvasProjectImportFiles(r)
	if err != nil {
		closeCanvasProjectFiles(files)
		return nil, err
	}
	return files, nil
}

func decodeCanvasProjectUpdateRequest(r *http.Request, scope string, data *CanvasProjectData) (localObjectWriteRequest, []CanvasProjectImportFile, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		payload, err := decodeLocalObjectWriteRequest(r, scope, data)
		return payload, nil, err
	}
	files, err := decodeCanvasProjectWriteRequest(r, scope, data)
	if err != nil {
		return localObjectWriteRequest{}, nil, err
	}
	revisionValue := strings.TrimSpace(r.FormValue("revision"))
	if revisionValue == "" {
		closeCanvasProjectFiles(files)
		return localObjectWriteRequest{}, nil, NewError(ErrorInvalidArgument, "revision is required for update", 1, nil)
	}
	revision, err := strconv.Atoi(revisionValue)
	if err != nil {
		closeCanvasProjectFiles(files)
		return localObjectWriteRequest{}, nil, NewError(ErrorInvalidArgument, "revision is invalid", 1, nil)
	}
	return localObjectWriteRequest{Revision: revision}, files, nil
}

func readCanvasProjectImportFiles(r *http.Request) ([]CanvasProjectImportFile, error) {
	if r.MultipartForm == nil || len(r.MultipartForm.File) == 0 {
		return nil, nil
	}
	files := []CanvasProjectImportFile{}
	for name, headers := range r.MultipartForm.File {
		if !strings.HasPrefix(name, "file:") || len(headers) == 0 {
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(name, "file:"))
		if err := validatePathComponent("canvas project file key", key); err != nil {
			return files, err
		}
		file, err := readCanvasProjectImportFile(key, headers[0])
		if err != nil {
			return files, err
		}
		files = append(files, file)
	}
	return files, nil
}
