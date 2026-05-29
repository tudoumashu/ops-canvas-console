package model

type WorkflowRunStatus string

const (
	WorkflowRunStatusIdle    WorkflowRunStatus = "idle"
	WorkflowRunStatusRunning WorkflowRunStatus = "running"
	WorkflowRunStatusSuccess WorkflowRunStatus = "success"
	WorkflowRunStatusError   WorkflowRunStatus = "error"
)

type WorkflowTemplate struct {
	ID           string               `json:"id" gorm:"primaryKey"`
	WorkflowType string               `json:"workflowType" gorm:"index"`
	Title        string               `json:"title"`
	Description  string               `json:"description"`
	Spec         WorkflowTemplateSpec `json:"spec" gorm:"serializer:json"`
	CreatedAt    string               `json:"createdAt"`
	UpdatedAt    string               `json:"updatedAt"`
}

type WorkflowTemplateSpec struct {
	Version  int                      `json:"version"`
	Nodes    []WorkflowTemplateNode   `json:"nodes"`
	Edges    []WorkflowTemplateEdge   `json:"edges"`
	Settings WorkflowTemplateSettings `json:"settings"`
}

type WorkflowTemplateSettings struct {
	ProductConcurrency int `json:"productConcurrency"`
	MaxRetries         int `json:"maxRetries"`
}

type WorkflowTemplateNode struct {
	ID             string                  `json:"id"`
	Type           string                  `json:"type"`
	Title          string                  `json:"title"`
	X              float64                 `json:"x"`
	Y              float64                 `json:"y"`
	Width          float64                 `json:"width"`
	Height         float64                 `json:"height"`
	Operation      string                  `json:"operation"`
	Model          string                  `json:"model"`
	Prompt         string                  `json:"prompt"`
	Count          int                     `json:"count"`
	Size           string                  `json:"size"`
	Quality        string                  `json:"quality"`
	Seconds        string                  `json:"seconds"`
	VideoQuality   string                  `json:"videoQuality"`
	Retry          *WorkflowNodeRetry      `json:"retry,omitempty"`
	OutputMappings []WorkflowOutputMapping `json:"outputMappings"`
	Extra          map[string]any          `json:"extra,omitempty"`
}

type WorkflowNodeRetry struct {
	Enabled         *bool `json:"enabled,omitempty"`
	RetryCount      int   `json:"retryCount"`
	IntervalSeconds int   `json:"intervalSeconds"`
}

const (
	WorkflowTextOutputFormatText = "text"
	WorkflowTextOutputFormatJSON = "json"
)

type WorkflowOutputMapping struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type WorkflowTemplateEdge struct {
	ID           string         `json:"id"`
	From         string         `json:"from"`
	To           string         `json:"to"`
	FromHandle   string         `json:"fromHandle,omitempty"`
	InputOrder   int            `json:"inputOrder,omitempty"`
	InputAlias   string         `json:"inputAlias,omitempty"`
	FileSelector string         `json:"fileSelector,omitempty"`
	Condition    map[string]any `json:"condition,omitempty"`
	Loop         map[string]any `json:"loop,omitempty"`
}

type WorkflowRun struct {
	ID             string               `json:"id" gorm:"primaryKey"`
	WorkflowType   string               `json:"workflowType" gorm:"index"`
	TemplateID     string               `json:"templateId" gorm:"index"`
	TemplateTitle  string               `json:"templateTitle"`
	Status         WorkflowRunStatus    `json:"status" gorm:"index"`
	RunDir         string               `json:"runDir"`
	InputCount     int                  `json:"inputCount"`
	CompletedCount int                  `json:"completedCount"`
	FailedCount    int                  `json:"failedCount"`
	Error          string               `json:"error,omitempty"`
	SpecSnapshot   WorkflowTemplateSpec `json:"specSnapshot" gorm:"serializer:json"`
	CreatedAt      string               `json:"createdAt"`
	UpdatedAt      string               `json:"updatedAt"`
}

type WorkflowTemplateList struct {
	Items []WorkflowTemplate `json:"items"`
	Total int                `json:"total"`
}

type WorkflowRunList struct {
	Items []WorkflowRun `json:"items"`
	Total int           `json:"total"`
}

type StartWorkflowTemplateRunRequest struct {
	RunID              string           `json:"runId"`
	Inputs             []map[string]any `json:"inputs"`
	ProductConcurrency int              `json:"productConcurrency"`
	MaxRetries         int              `json:"maxRetries"`
}

type StartWorkflowTemplateRunResult struct {
	RunID  string `json:"runId"`
	RunDir string `json:"runDir"`
}
