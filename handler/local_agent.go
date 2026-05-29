package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/basketikun/infinite-canvas/service"
)

func LocalAgentClaimJob(w http.ResponseWriter, r *http.Request) {
	job, ok, err := service.ClaimLocalAgentJob(localAgentToken(r))
	if err != nil {
		FailError(w, err)
		return
	}
	if !ok {
		OK(w, map[string]any{"job": nil})
		return
	}
	OK(w, map[string]any{"job": job})
}

func LocalAgentCompleteJob(w http.ResponseWriter, r *http.Request, id string) {
	var request struct {
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	if err := service.CompleteLocalAgentJob(localAgentToken(r), id, request.Output, request.Error); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func LocalAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if err := service.ValidateLocalAgentToken(localAgentToken(r)); err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"status": "ok"})
}

func localAgentToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-Local-Agent-Token"))
}
