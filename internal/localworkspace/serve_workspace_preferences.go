package localworkspace

import (
	"encoding/json"
	"net/http"
)

type workspacePreferencesWriteRequest struct {
	Revision    int             `json:"revision"`
	Preferences json.RawMessage `json:"preferences"`
}

func (api *serveAPI) handleWorkspacePreferences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		snapshot, err := ReadWorkspacePreferences(api.workspace)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, snapshot, nil)
	case http.MethodPut:
		api.updateWorkspacePreferences(w, r)
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
	}
}

func (api *serveAPI) updateWorkspacePreferences(w http.ResponseWriter, r *http.Request) {
	var payload workspacePreferencesWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "request body is invalid", 1, nil))
		return
	}
	if len(payload.Preferences) == 0 {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "preferences are required", 1, nil))
		return
	}
	if err := validateRawNoPlaintextSecrets(payload.Preferences, "workspace.preferences"); err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	var preferences WorkspacePreferences
	if err := json.Unmarshal(payload.Preferences, &preferences); err != nil {
		writeServeErrorFromError(w, WrapError(ErrorInvalidArgument, "preferences are invalid", 1, err))
		return
	}
	document, err := UpdateWorkspacePreferences(api.workspace, payload.Revision, preferences)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	api.mu.Lock()
	api.workspace.Document = document
	api.mu.Unlock()
	writeServeSuccess(w, workspacePreferencesSnapshot(document), nil)
}
