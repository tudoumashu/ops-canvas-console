package localworkspace

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (api *serveAPI) handleWorkbenchLogRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/workbench-logs/")
	if len(parts) == 3 && parts[1] == "files" && r.Method == http.MethodGet {
		api.handleWorkbenchLogFileRoute(w, r, parts[0], parts[2])
		return
	}
	if localPath == "/workbench-logs" {
		switch r.Method {
		case http.MethodGet:
			modality := strings.TrimSpace(r.URL.Query().Get("modality"))
			items, err := ListWorkbenchLogSummaries(api.workspace, modality)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"workbenchLogs": items}, nil)
		case http.MethodPost:
			api.createWorkbenchLog(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 1 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "workbench log route not found", 2, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		document, err := ReadWorkbenchLog(api.workspace, parts[0])
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, document, nil)
	case http.MethodDelete:
		if err := DeleteWorkbenchLog(api.workspace, parts[0]); err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, map[string]any{"deleted": true, "id": parts[0]}, nil)
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) createWorkbenchLog(w http.ResponseWriter, r *http.Request) {
	var data WorkbenchLogData
	var files []WorkbenchLogImportFile
	var err error
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		data, files, err = decodeWorkbenchLogMultipartRequest(r)
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		defer closeWorkbenchLogFiles(files)
	} else {
		if _, err = decodeLocalObjectWriteRequest(r, "workbench_log", &data); err != nil {
			writeServeErrorFromError(w, err)
			return
		}
	}
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err := NewWorkbenchLog(api.workspace, data)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	if err := SaveWorkbenchLog(api.workspace, document, files); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	document, err = ReadWorkbenchLog(api.workspace, document.ID)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, document, nil)
}

func (api *serveAPI) handleWorkbenchLogFileRoute(w http.ResponseWriter, r *http.Request, id string, fileKey string) {
	document, err := ReadWorkbenchLog(api.workspace, id)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	media, ok := workbenchLogMediaByKey(document.Data.Media, fileKey)
	if !ok {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "workbench log file ref not found", 2, nil))
		return
	}
	serveObjectFile(w, r, WorkbenchLogRepository(api.workspace).Dir(document.ID), media.Path, media.MIME)
}

func decodeWorkbenchLogMultipartRequest(r *http.Request) (WorkbenchLogData, []WorkbenchLogImportFile, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return WorkbenchLogData{}, nil, WrapError(ErrorInvalidArgument, "multipart request is invalid", 1, err)
	}
	raw := strings.TrimSpace(r.FormValue("data"))
	if raw == "" {
		return WorkbenchLogData{}, nil, NewError(ErrorInvalidArgument, "request data is required", 1, nil)
	}
	dataRaw := json.RawMessage(raw)
	if err := validateRawNoPlaintextSecrets(dataRaw, "workbench_log"); err != nil {
		return WorkbenchLogData{}, nil, err
	}
	var data WorkbenchLogData
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return WorkbenchLogData{}, nil, WrapError(ErrorInvalidArgument, "request data is invalid", 1, err)
	}
	files := []WorkbenchLogImportFile{}
	if r.MultipartForm == nil {
		return data, files, nil
	}
	for fieldName, headers := range r.MultipartForm.File {
		fileKey, ok := strings.CutPrefix(fieldName, "file:")
		if !ok {
			continue
		}
		fileKey = strings.TrimSpace(fileKey)
		if err := validatePathComponent("workbench log file key", fileKey); err != nil {
			closeWorkbenchLogFiles(files)
			return WorkbenchLogData{}, nil, err
		}
		if len(headers) != 1 {
			closeWorkbenchLogFiles(files)
			return WorkbenchLogData{}, nil, NewError(ErrorInvalidArgument, "workbench log file key must map to one file", 1, map[string]string{"fileKey": fileKey})
		}
		file, err := readWorkbenchLogImportFile(fileKey, headers[0])
		if err != nil {
			closeWorkbenchLogFiles(files)
			return WorkbenchLogData{}, nil, err
		}
		files = append(files, file)
	}
	return data, files, nil
}
