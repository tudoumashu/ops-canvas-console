package handler

import (
	"encoding/json"
	"net/http"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

func PDDWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListWorkflowTemplates("pdd", parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDWorkflowTemplate(w http.ResponseWriter, r *http.Request, id string) {
	result, err := service.GetWorkflowTemplate(id)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDSaveWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	var request model.WorkflowTemplate
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.SaveWorkflowTemplate("pdd", request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDSaveWorkflowTemplateWithID(w http.ResponseWriter, r *http.Request, id string) {
	var request model.WorkflowTemplate
	_ = json.NewDecoder(r.Body).Decode(&request)
	request.ID = id
	result, err := service.SaveWorkflowTemplate("pdd", request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDDeleteWorkflowTemplate(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteWorkflowTemplate(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func PDDStartWorkflowTemplateRun(w http.ResponseWriter, r *http.Request, id string) {
	var request model.StartWorkflowTemplateRunRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.StartWorkflowTemplateRun(id, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDCustomWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListWorkflowRuns("pdd", parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDCustomWorkflowRun(w http.ResponseWriter, r *http.Request, id string) {
	result, err := service.GetWorkflowRun(id)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDWorkflowThemes(w http.ResponseWriter, r *http.Request) {
	result, err := service.LoadPDDWorkflowThemes()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDCreateManualEdit(w http.ResponseWriter, r *http.Request, runID string) {
	var request model.PDDManualEditRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.CreatePDDManualEdit(runID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PDDApplyManualEdit(w http.ResponseWriter, r *http.Request, runID string, editID string) {
	var request model.PDDManualEditApplyRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.ApplyPDDManualEdit(runID, editID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
