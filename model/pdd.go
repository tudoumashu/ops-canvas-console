package model

type PDDRunStatus string

const (
	PDDRunStatusIdle    PDDRunStatus = "idle"
	PDDRunStatusRunning PDDRunStatus = "running"
	PDDRunStatusSuccess PDDRunStatus = "success"
	PDDRunStatusError   PDDRunStatus = "error"
)

type PDDRunItem struct {
	RunID             string       `json:"runId"`
	Status            PDDRunStatus `json:"status"`
	RunDir            string       `json:"runDir"`
	UpdatedAt         string       `json:"updatedAt"`
	CustomWorkflow    bool         `json:"customWorkflow,omitempty"`
	StartedAt         string       `json:"startedAt,omitempty"`
	FinishedAt        string       `json:"finishedAt,omitempty"`
	Completed         bool         `json:"completed"`
	HasLogs           bool         `json:"hasLogs"`
	ProductTotal      int          `json:"productTotal"`
	CompletedProducts int          `json:"completedProducts"`
	FailedProducts    int          `json:"failedProducts"`
	RunningProducts   int          `json:"runningProducts"`
	RecentError       string       `json:"recentError,omitempty"`
}

type PDDRunList struct {
	Items []PDDRunItem `json:"items"`
	Root  string       `json:"root"`
}

type PDDRunOverview struct {
	Run          PDDRunItem          `json:"run"`
	Stages       []PDDStageNode      `json:"stages"`
	Edges        []PDDGraphEdge      `json:"edges"`
	Products     []PDDProductSummary `json:"products"`
	RecentErrors []string            `json:"recentErrors"`
}

type PDDStageNode struct {
	ID              string       `json:"id"`
	Title           string       `json:"title"`
	Type            string       `json:"type,omitempty"`
	Status          PDDRunStatus `json:"status"`
	Total           int          `json:"total"`
	Success         int          `json:"success"`
	Failed          int          `json:"failed"`
	Running         int          `json:"running"`
	Idle            int          `json:"idle"`
	Skipped         int          `json:"skipped"`
	X               float64      `json:"x,omitempty"`
	Y               float64      `json:"y,omitempty"`
	Width           float64      `json:"width,omitempty"`
	Height          float64      `json:"height,omitempty"`
	DurationSeconds float64      `json:"durationSeconds,omitempty"`
	RecentError     string       `json:"recentError,omitempty"`
}

type PDDProductSummary struct {
	Key              string       `json:"key"`
	SourceProduct    string       `json:"sourceProduct"`
	GeneratedProduct string       `json:"generatedProduct,omitempty"`
	Product          string       `json:"product"`
	ThemeName        string       `json:"themeName"`
	Status           PDDRunStatus `json:"status"`
	RawStatus        string       `json:"rawStatus"`
	StartedAt        string       `json:"startedAt,omitempty"`
	FinishedAt       string       `json:"finishedAt,omitempty"`
	Error            string       `json:"error,omitempty"`
	GeneratedImages  int          `json:"generatedImages"`
	SpecImages       int          `json:"specImages"`
	MainImages       int          `json:"mainImages"`
	ArtifactCount    int          `json:"artifactCount,omitempty"`
}

type PDDProductDetail struct {
	RunID   string            `json:"runId"`
	Product PDDProductSummary `json:"product"`
	Nodes   []PDDGraphNode    `json:"nodes"`
	Edges   []PDDGraphEdge    `json:"edges"`
	Files   []PDDDetailFile   `json:"files"`
}

type PDDCreativeCanvas struct {
	RunID          string                 `json:"runId"`
	ProductKey     string                 `json:"productKey"`
	Product        PDDProductSummary      `json:"product"`
	Nodes          []PDDCreativeNode      `json:"nodes"`
	Edges          []PDDCreativeEdge      `json:"edges"`
	Viewport       map[string]float64     `json:"viewport,omitempty"`
	BackgroundMode string                 `json:"backgroundMode,omitempty"`
	ShowImageInfo  bool                   `json:"showImageInfo"`
	Saved          bool                   `json:"saved"`
	UpdatedAt      string                 `json:"updatedAt,omitempty"`
	Context        map[string]interface{} `json:"context,omitempty"`
}

type PDDCreativeNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Position map[string]float64     `json:"position"`
	Width    float64                `json:"width"`
	Height   float64                `json:"height"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type PDDCreativeEdge struct {
	ID         string `json:"id"`
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
}

type PDDCreativeCanvasSaveRequest struct {
	Nodes          []PDDCreativeNode  `json:"nodes"`
	Edges          []PDDCreativeEdge  `json:"edges"`
	Viewport       map[string]float64 `json:"viewport,omitempty"`
	BackgroundMode string             `json:"backgroundMode,omitempty"`
	ShowImageInfo  bool               `json:"showImageInfo"`
}

type PDDCreativeAssetRequest struct {
	ProductKey string `json:"productKey,omitempty"`
	NodeID     string `json:"nodeId"`
	FileName   string `json:"fileName,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Content    string `json:"content"`
}

type PDDCreativeAsset struct {
	URL      string  `json:"url"`
	Path     string  `json:"path"`
	FileName string  `json:"fileName"`
	MimeType string  `json:"mimeType"`
	Bytes    int64   `json:"bytes"`
	Width    float64 `json:"width,omitempty"`
	Height   float64 `json:"height,omitempty"`
}

type PDDCreativeCanvasApplyRequest struct {
	ProductKey      string `json:"productKey,omitempty"`
	SourceNodeID    string `json:"sourceNodeId"`
	TargetNodeID    string `json:"targetNodeId"`
	ArtifactPath    string `json:"artifactPath,omitempty"`
	Content         string `json:"content,omitempty"`
	MimeType        string `json:"mimeType,omitempty"`
	RerunDownstream bool   `json:"rerunDownstream"`
}

type PDDGraphNode struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Title           string          `json:"title"`
	Status          PDDRunStatus    `json:"status"`
	X               float64         `json:"x"`
	Y               float64         `json:"y"`
	Width           float64         `json:"width"`
	Height          float64         `json:"height"`
	Summary         string          `json:"summary,omitempty"`
	Config          map[string]any  `json:"config,omitempty"`
	DurationSeconds float64         `json:"durationSeconds,omitempty"`
	Artifacts       []PDDArtifact   `json:"artifacts,omitempty"`
	Files           []PDDDetailFile `json:"files,omitempty"`
}

type PDDGraphEdge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type PDDArtifact struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	MimeType string `json:"mimeType,omitempty"`
}

type PDDDetailFile struct {
	Title string `json:"title"`
	Path  string `json:"path"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

type PDDActionRequest struct {
	Action        string         `json:"action"`
	RunID         string         `json:"runId"`
	CountPerTheme int            `json:"countPerTheme"`
	ExtraArgs     []string       `json:"extraArgs"`
	ConsoleSpec   map[string]any `json:"consoleSpec,omitempty"`
}

type PDDActionResult struct {
	Action string `json:"action"`
	Output string `json:"output"`
	RunID  string `json:"runId,omitempty"`
}

type PDDManualEditRequest struct {
	ProductKey      string `json:"productKey"`
	NodeID          string `json:"nodeId"`
	ArtifactPath    string `json:"artifactPath"`
	MaskPath        string `json:"maskPath"`
	MaskDataURL     string `json:"maskDataUrl"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	Count           int    `json:"count"`
	Size            string `json:"size"`
	Quality         string `json:"quality"`
	Apply           bool   `json:"apply"`
	RerunDownstream bool   `json:"rerunDownstream"`
}

type PDDManualEditApplyRequest struct {
	ProductKey      string `json:"productKey"`
	NodeID          string `json:"nodeId"`
	RerunDownstream bool   `json:"rerunDownstream"`
}

type PDDManualEditResult struct {
	EditID          string        `json:"editId"`
	ProductKey      string        `json:"productKey"`
	NodeID          string        `json:"nodeId"`
	Artifacts       []PDDArtifact `json:"artifacts"`
	Applied         bool          `json:"applied"`
	RerunDownstream bool          `json:"rerunDownstream"`
	Output          string        `json:"output,omitempty"`
}
