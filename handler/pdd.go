package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

func PDDRuns(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListPDDRuns()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDRunOverview(w http.ResponseWriter, r *http.Request, runID string) {
	result, err := service.PDDRunOverview(runID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDRunProducts(w http.ResponseWriter, r *http.Request, runID string) {
	result, err := service.ListPDDProducts(runID, parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDProductDetail(w http.ResponseWriter, r *http.Request, runID string, productKey string) {
	result, err := service.PDDProductDetail(runID, productKey)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDCreativeCanvas(w http.ResponseWriter, r *http.Request, runID string, productKey string) {
	result, err := service.PDDCreativeCanvas(runID, productKey)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDSaveCreativeCanvas(w http.ResponseWriter, r *http.Request, runID string, productKey string) {
	var request model.PDDCreativeCanvasSaveRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.SavePDDCreativeCanvas(runID, productKey, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDCreativeCanvasAsset(w http.ResponseWriter, r *http.Request, runID string, productKey string) {
	var request model.PDDCreativeAssetRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.SavePDDCreativeCanvasAsset(runID, productKey, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDApplyCreativeCanvas(w http.ResponseWriter, r *http.Request, runID string, productKey string) {
	var request model.PDDCreativeCanvasApplyRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	if request.ProductKey == "" {
		request.ProductKey = productKey
	}
	result, err := service.ApplyPDDCreativeCanvasOutput(runID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDRunFile(w http.ResponseWriter, r *http.Request, runID string) {
	path, err := service.ResolvePDDRunFile(runID, r.URL.Query().Get("path"))
	if err != nil {
		FailError(w, err)
		return
	}
	http.ServeFile(w, r, path)
}

func PDDRunLogStream(w http.ResponseWriter, r *http.Request, runID string) {
	path, err := service.ResolvePDDRunFile(runID, "remote_workflow.log")
	if err != nil {
		path, err = service.ResolvePDDRunFile(runID, "logs/custom_workflow.log")
		if err != nil {
			FailError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		Fail(w, "当前服务不支持日志流")
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var offset int64
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			chunk, nextOffset := service.ReadFileTail(path, offset, 64*1024)
			offset = nextOffset
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			for _, line := range strings.Split(chunk, "\n") {
				fmt.Fprintf(w, "data: %s\n", strings.ReplaceAll(line, "\r", ""))
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
		}
	}
}

func PDDAction(w http.ResponseWriter, r *http.Request) {
	var request model.PDDActionRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	user, _ := service.UserFromContext(r.Context())
	result, err := service.RunPDDAction(request, user)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
