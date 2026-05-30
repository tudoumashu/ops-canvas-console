package localworkspace

import (
	"fmt"
	"time"
)

const (
	SchemaVersion = "local-workspace-v1"

	WorkspaceFileName = "opsc-workspace.json"
	IndexFileName     = "index.sqlite"

	DefaultWorkspaceName = "Default Workspace"
	DefaultWorkspaceDir  = "OpsCanvas"

	EnvWorkspace = "OPSC_WORKSPACE"

	KindWorkspace     = "workspace"
	KindProfile       = "profile"
	KindProject       = "project"
	KindTemplate      = "template"
	KindRun           = "run"
	KindRunNode       = "run_node"
	KindArtifact      = "artifact"
	KindAsset         = "asset"
	KindPrompt        = "prompt"
	KindCanvasProject = "canvas_project"
	KindWorkbenchLog  = "workbench_log"

	RunStatusPending  = "pending"
	RunStatusRunning  = "running"
	RunStatusSuccess  = "success"
	RunStatusError    = "error"
	RunStatusCanceled = "canceled"

	ErrorInvalidArgument    = "invalid_argument"
	ErrorWorkspaceNotFound  = "workspace_not_found"
	ErrorWorkspaceInvalid   = "workspace_invalid"
	ErrorWorkspaceLocked    = "workspace_locked"
	ErrorWorkspaceUnhealthy = "workspace_unhealthy"
	ErrorAuthFailed         = "auth_failed"
	ErrorInternal           = "internal_error"
)

var RequiredDirs = []string{
	".opsc",
	"profiles",
	"projects",
	"templates",
	"runs",
	"artifacts",
	"assets",
	"prompts",
	"canvas-projects",
	"workbench-logs",
	"cache",
	"exports",
}

type PathSource string

const (
	PathSourceFlag    PathSource = "flag"
	PathSourceEnv     PathSource = "env"
	PathSourceDefault PathSource = "default"
)

type ResolvedPath struct {
	Path   string
	Source PathSource
}

type Envelope[T any] struct {
	SchemaVersion string `json:"schemaVersion"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Revision      int    `json:"revision"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	Data          T      `json:"data"`
}

type WorkspaceData struct {
	Name             string               `json:"name"`
	DefaultProfileID string               `json:"defaultProfileId,omitempty"`
	Preferences      WorkspacePreferences `json:"preferences,omitempty"`
}

type WorkspacePreferences struct {
	WorkflowFolders []WorkflowFolderPreference `json:"workflowFolders"`
}

type WorkflowFolderPreference struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Href        string `json:"href,omitempty"`
	Kind        string `json:"kind"`
}

type WorkspaceManifest = Envelope[WorkspaceData]
type WorkspaceDocument = WorkspaceManifest

type Workspace struct {
	Root     string
	Source   PathSource
	Document WorkspaceDocument
}

type InitOptions struct {
	Path string
	Name string
	Now  func() time.Time
}

type InitResult struct {
	Workspace Workspace `json:"workspace"`
	Created   bool      `json:"created"`
	Warnings  []string  `json:"warnings,omitempty"`
}

type WorkspaceInfo struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	SchemaVersion    string      `json:"schemaVersion"`
	Revision         int         `json:"revision"`
	DefaultProfileID string      `json:"defaultProfileId,omitempty"`
	Path             string      `json:"path,omitempty"`
	PathSource       PathSource  `json:"pathSource"`
	Directories      []string    `json:"directories"`
	Runtime          RuntimeInfo `json:"runtime"`
}

type RuntimeInfo struct {
	Active           bool   `json:"active"`
	PID              int    `json:"pid,omitempty"`
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	TokenFile        string `json:"tokenFile,omitempty"`
	LaunchSecretFile string `json:"launchSecretFile,omitempty"`
}

type RuntimeMetadata struct {
	SchemaVersion    string `json:"schemaVersion"`
	PID              int    `json:"pid"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	BaseURL          string `json:"baseUrl"`
	WorkspaceID      string `json:"workspaceId"`
	WorkspacePath    string `json:"workspacePath"`
	StartedAt        string `json:"startedAt"`
	TokenFile        string `json:"tokenFile"`
	LaunchSecretFile string `json:"launchSecretFile"`
}

type DoctorReport struct {
	OK            bool          `json:"ok"`
	Path          string        `json:"path,omitempty"`
	WorkspaceID   string        `json:"workspaceId,omitempty"`
	SchemaVersion string        `json:"schemaVersion,omitempty"`
	Checks        []DoctorCheck `json:"checks"`
	Warnings      []string      `json:"warnings,omitempty"`
	Errors        []string      `json:"errors,omitempty"`
}

type DoctorCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ScanOptions struct{}

type ScanEntry struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	FileName  string `json:"fileName"`
	Revision  int    `json:"revision"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type ScanResult struct {
	WorkspaceID string      `json:"workspaceId"`
	Entries     []ScanEntry `json:"entries"`
	Warnings    []string    `json:"warnings,omitempty"`
}

type Error struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode int    `json:"-"`
	Details  any    `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewError(code string, message string, exitCode int, details any) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		ExitCode: exitCode,
		Details:  details,
	}
}

func WrapError(code string, message string, exitCode int, err error) *Error {
	if err == nil {
		return NewError(code, message, exitCode, nil)
	}
	return NewError(code, fmt.Sprintf("%s: %v", message, err), exitCode, nil)
}
