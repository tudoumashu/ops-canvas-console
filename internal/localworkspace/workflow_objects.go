package localworkspace

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const KindRunArtifactRef = "run_artifact_ref"

type TemplateData struct {
	Title        string            `json:"title"`
	Description  string            `json:"description,omitempty"`
	WorkflowType string            `json:"workflowType,omitempty"`
	Version      int               `json:"version,omitempty"`
	Nodes        []json.RawMessage `json:"nodes,omitempty"`
	Edges        []json.RawMessage `json:"edges,omitempty"`
	Settings     map[string]any    `json:"settings,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

type RunData struct {
	TemplateID   string               `json:"templateId,omitempty"`
	Status       string               `json:"status"`
	ProfileID    string               `json:"profileId,omitempty"`
	ProjectID    string               `json:"projectId,omitempty"`
	Input        map[string]any       `json:"input,omitempty"`
	Output       map[string]any       `json:"output,omitempty"`
	ArtifactRefs []RunArtifactRefData `json:"artifactRefs,omitempty"`
	Metadata     map[string]any       `json:"metadata,omitempty"`
}

type SaveRunOptions struct {
	TemplateSnapshot *Envelope[TemplateData]
}

type RunArtifactRefData struct {
	ArtifactID string         `json:"artifactId"`
	Role       string         `json:"role,omitempty"`
	NodeID     string         `json:"nodeId,omitempty"`
	Slot       string         `json:"slot,omitempty"`
	Order      int            `json:"order,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type RunNodeStateData struct {
	NodeID     string         `json:"nodeId"`
	Status     string         `json:"status"`
	StartedAt  string         `json:"startedAt,omitempty"`
	FinishedAt string         `json:"finishedAt,omitempty"`
	Error      string         `json:"error,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ArtifactData struct {
	Type            string            `json:"type"`
	MIME            string            `json:"mime,omitempty"`
	Title           string            `json:"title,omitempty"`
	SHA256          string            `json:"sha256,omitempty"`
	Bytes           int64             `json:"bytes,omitempty"`
	Width           int               `json:"width,omitempty"`
	Height          int               `json:"height,omitempty"`
	DurationSeconds *float64          `json:"durationSeconds,omitempty"`
	Source          map[string]any    `json:"source,omitempty"`
	Privacy         string            `json:"privacy,omitempty"`
	Files           map[string]string `json:"files,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
}

type TemplateSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	WorkflowType string `json:"workflowType,omitempty"`
	Version      int    `json:"version,omitempty"`
	Revision     int    `json:"revision"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type RunSummary struct {
	ID                  string `json:"id"`
	Status              string `json:"status"`
	TemplateID          string `json:"templateId,omitempty"`
	ProfileID           string `json:"profileId,omitempty"`
	ProjectID           string `json:"projectId,omitempty"`
	ArtifactCount       int    `json:"artifactCount"`
	LatestEventSequence int64  `json:"latestEventSequence"`
	Revision            int    `json:"revision"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type ArtifactSummary struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	MIME            string   `json:"mime,omitempty"`
	Title           string   `json:"title,omitempty"`
	SHA256          string   `json:"sha256,omitempty"`
	Bytes           int64    `json:"bytes,omitempty"`
	Width           int      `json:"width,omitempty"`
	Height          int      `json:"height,omitempty"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
	Privacy         string   `json:"privacy,omitempty"`
	Original        string   `json:"original,omitempty"`
	Thumbnail       string   `json:"thumbnail,omitempty"`
	Revision        int      `json:"revision"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type RunArtifactSummary struct {
	Artifact ArtifactSummary    `json:"artifact"`
	Ref      RunArtifactRefData `json:"ref"`
}

type RunNodeStateSummary struct {
	NodeID     string `json:"nodeId"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	Error      string `json:"error,omitempty"`
	Revision   int    `json:"revision"`
	UpdatedAt  string `json:"updatedAt"`
}

type RunStatusSnapshot struct {
	Run                 RunSummary            `json:"run"`
	Nodes               []RunNodeStateSummary `json:"nodes"`
	LatestEventSequence int64                 `json:"latestEventSequence"`
}

func TemplateRepository(workspace Workspace) Repository[TemplateData] {
	return Repository[TemplateData]{
		Workspace:  workspace,
		Collection: "templates",
		FileName:   "template.json",
		Kind:       KindTemplate,
		IDPrefix:   "tpl",
	}
}

func RunRepository(workspace Workspace) Repository[RunData] {
	return Repository[RunData]{
		Workspace:  workspace,
		Collection: "runs",
		FileName:   "run.json",
		Kind:       KindRun,
		IDPrefix:   "run",
	}
}

func ArtifactRepository(workspace Workspace) Repository[ArtifactData] {
	return Repository[ArtifactData]{
		Workspace:  workspace,
		Collection: "artifacts",
		FileName:   "artifact.json",
		Kind:       KindArtifact,
		IDPrefix:   "art",
	}
}

func NewTemplate(workspace Workspace, data TemplateData) (Envelope[TemplateData], error) {
	data = normalizeTemplateData(data)
	return TemplateRepository(workspace).New(data)
}

func WriteTemplate(workspace Workspace, document Envelope[TemplateData]) error {
	document.Data = normalizeTemplateData(document.Data)
	if err := TemplateRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := TemplateRepository(workspace).Write(document); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertTemplate(document)
		})
	})
}

func SaveTemplate(workspace Workspace, document Envelope[TemplateData]) error {
	return WriteTemplate(workspace, document)
}

func ReadTemplate(workspace Workspace, id string) (Envelope[TemplateData], error) {
	return TemplateRepository(workspace).Read(id)
}

func GetTemplate(workspace Workspace, id string) (Envelope[TemplateData], error) {
	return ReadTemplate(workspace, id)
}

func ListTemplates(workspace Workspace) ([]Envelope[TemplateData], error) {
	return TemplateRepository(workspace).List()
}

func ListTemplateSummaries(workspace Workspace) ([]TemplateSummary, error) {
	var summaries []TemplateSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListTemplateSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func NewRun(workspace Workspace, data RunData) (Envelope[RunData], error) {
	if strings.TrimSpace(data.Status) == "" {
		data.Status = RunStatusPending
	}
	if err := validateRunData(data); err != nil {
		return Envelope[RunData]{}, err
	}
	return RunRepository(workspace).New(data)
}

func WriteRun(workspace Workspace, document Envelope[RunData]) error {
	return SaveRun(workspace, document, SaveRunOptions{})
}

func SaveRun(workspace Workspace, document Envelope[RunData], opts SaveRunOptions) error {
	if err := RunRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	if err := validateRunData(document.Data); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		previous, previousErr := RunRepository(workspace).Read(document.ID)
		existing := previousErr == nil
		if previousErr != nil {
			var workspaceErr *Error
			if !asLocalWorkspaceError(previousErr, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceNotFound {
				return previousErr
			}
		}
		if err := ensureRunScaffold(workspace, document.ID); err != nil {
			return err
		}
		if err := writeRunTemplateSnapshot(workspace, document, opts.TemplateSnapshot); err != nil {
			return err
		}
		if err := RunRepository(workspace).Write(document); err != nil {
			return err
		}
		if !existing {
			sequence, err := nextRunEventSequence(workspace, document.ID)
			if err != nil {
				return err
			}
			event, err := buildRunEvent(document.ID, sequence, RunEventInput{
				Type:    "run.created",
				Level:   "info",
				Actor:   RunEventActor{Type: "system", ID: "localworkspace"},
				Message: "Run created",
				Data: map[string]any{
					"status": document.Data.Status,
				},
			})
			if err != nil {
				return err
			}
			if err := appendRunEventUnlocked(workspace, document.ID, event); err != nil {
				return err
			}
		} else if previous.Data.Status != document.Data.Status {
			sequence, err := nextRunEventSequence(workspace, document.ID)
			if err != nil {
				return err
			}
			event, err := buildRunEvent(document.ID, sequence, RunEventInput{
				Type:    "run.status_changed",
				Level:   runStatusEventLevel(document.Data.Status),
				Actor:   RunEventActor{Type: "system", ID: "localworkspace"},
				Message: "Run status changed",
				Data: map[string]any{
					"previousStatus": previous.Data.Status,
					"status":         document.Data.Status,
				},
			})
			if err != nil {
				return err
			}
			if err := appendRunEventUnlocked(workspace, document.ID, event); err != nil {
				return err
			}
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			events, err := ReadRunEvents(workspace, document.ID, 0)
			if err != nil {
				return err
			}
			for _, event := range events {
				if err := index.UpsertRunEvent(document.ID, event); err != nil {
					return err
				}
			}
			return index.UpsertRun(workspace, document)
		})
	})
}

func ReadRun(workspace Workspace, id string) (Envelope[RunData], error) {
	document, err := RunRepository(workspace).Read(id)
	if err != nil {
		return Envelope[RunData]{}, err
	}
	if err := validateRunData(document.Data); err != nil {
		return Envelope[RunData]{}, err
	}
	return document, nil
}

func ListRuns(workspace Workspace) ([]Envelope[RunData], error) {
	documents, err := RunRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if err := validateRunData(document.Data); err != nil {
			return nil, err
		}
	}
	return documents, nil
}

func ListRunSummaries(workspace Workspace) ([]RunSummary, error) {
	var summaries []RunSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListRunSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func GetRunStatus(workspace Workspace, runID string) (RunStatusSnapshot, error) {
	var snapshot RunStatusSnapshot
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		run, err := index.GetRunSummary(runID)
		if err != nil {
			return err
		}
		nodes, err := index.ListRunNodeStateSummaries(runID)
		if err != nil {
			return err
		}
		snapshot = RunStatusSnapshot{
			Run:                 run,
			Nodes:               nodes,
			LatestEventSequence: run.LatestEventSequence,
		}
		return nil
	})
	return snapshot, err
}

func NewArtifact(workspace Workspace, data ArtifactData) (Envelope[ArtifactData], error) {
	if strings.TrimSpace(data.Privacy) == "" {
		data.Privacy = "private"
	}
	if err := validateArtifactData(data); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	return ArtifactRepository(workspace).New(data)
}

func WriteArtifact(workspace Workspace, document Envelope[ArtifactData]) error {
	if err := validateArtifactData(document.Data); err != nil {
		return err
	}
	if err := ArtifactRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := ArtifactRepository(workspace).Write(document); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertArtifact(document)
		})
	})
}

func SaveArtifact(workspace Workspace, document Envelope[ArtifactData]) error {
	return WriteArtifact(workspace, document)
}

func ReadArtifact(workspace Workspace, id string) (Envelope[ArtifactData], error) {
	document, err := ArtifactRepository(workspace).Read(id)
	if err != nil {
		return Envelope[ArtifactData]{}, err
	}
	if err := validateArtifactData(document.Data); err != nil {
		return Envelope[ArtifactData]{}, err
	}
	return document, nil
}

func ListArtifacts(workspace Workspace) ([]Envelope[ArtifactData], error) {
	documents, err := ArtifactRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if err := validateArtifactData(document.Data); err != nil {
			return nil, err
		}
	}
	return documents, nil
}

func ListArtifactSummaries(workspace Workspace) ([]ArtifactSummary, error) {
	var summaries []ArtifactSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListArtifactSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func WriteRunArtifactRef(workspace Workspace, runID string, document Envelope[RunArtifactRefData]) error {
	if err := RunRepository(workspace).validateID(runID); err != nil {
		return err
	}
	if err := validateRunArtifactRefDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if _, err := ReadRun(workspace, runID); err != nil {
			return err
		}
		if _, err := ReadArtifact(workspace, document.Data.ArtifactID); err != nil {
			return err
		}
		if err := AtomicWriteJSON(runArtifactRefPath(workspace, runID, document.ID), document, 0o600); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			if err := index.UpsertRunArtifactRef(runID, document); err != nil {
				return err
			}
			run, err := ReadRun(workspace, runID)
			if err != nil {
				return err
			}
			return index.UpsertRun(workspace, run)
		})
	})
}

func ListRunArtifacts(workspace Workspace, runID string) ([]RunArtifactSummary, error) {
	if _, err := ReadRun(workspace, runID); err != nil {
		return nil, err
	}
	refs, err := listRunArtifactRefs(workspace, runID)
	if err != nil {
		return nil, err
	}
	items := []RunArtifactSummary{}
	for _, ref := range refs {
		artifact, err := ReadArtifact(workspace, ref.Data.ArtifactID)
		if err != nil {
			return nil, err
		}
		items = append(items, RunArtifactSummary{
			Artifact: ArtifactDocumentSummary(artifact),
			Ref:      ref.Data,
		})
	}
	sort.SliceStable(items, func(i int, j int) bool {
		if items[i].Ref.Order != items[j].Ref.Order {
			return items[i].Ref.Order < items[j].Ref.Order
		}
		return items[i].Artifact.ID < items[j].Artifact.ID
	})
	return items, nil
}

func TemplateDocumentSummary(document Envelope[TemplateData]) TemplateSummary {
	return TemplateSummary{
		ID:           document.ID,
		Title:        document.Data.Title,
		Description:  document.Data.Description,
		WorkflowType: document.Data.WorkflowType,
		Version:      document.Data.Version,
		Revision:     document.Revision,
		CreatedAt:    document.CreatedAt,
		UpdatedAt:    document.UpdatedAt,
	}
}

func normalizeTemplateData(data TemplateData) TemplateData {
	data.Title = strings.TrimSpace(data.Title)
	if data.Title == "" {
		data.Title = "未命名工作流模板"
	}
	data.Description = strings.TrimSpace(data.Description)
	data.WorkflowType = strings.TrimSpace(data.WorkflowType)
	if data.WorkflowType == "" {
		data.WorkflowType = "generic"
	}
	if data.Version <= 0 {
		data.Version = 1
	}
	if data.Settings == nil {
		data.Settings = map[string]any{}
	}
	return data
}

func RunDocumentSummary(document Envelope[RunData], artifactCount int) RunSummary {
	return RunSummary{
		ID:            document.ID,
		Status:        document.Data.Status,
		TemplateID:    document.Data.TemplateID,
		ProfileID:     document.Data.ProfileID,
		ProjectID:     document.Data.ProjectID,
		ArtifactCount: artifactCount,
		Revision:      document.Revision,
		CreatedAt:     document.CreatedAt,
		UpdatedAt:     document.UpdatedAt,
	}
}

func ArtifactDocumentSummary(document Envelope[ArtifactData]) ArtifactSummary {
	return ArtifactSummary{
		ID:              document.ID,
		Type:            document.Data.Type,
		MIME:            document.Data.MIME,
		Title:           document.Data.Title,
		SHA256:          document.Data.SHA256,
		Bytes:           document.Data.Bytes,
		Width:           document.Data.Width,
		Height:          document.Data.Height,
		DurationSeconds: document.Data.DurationSeconds,
		Privacy:         document.Data.Privacy,
		Original:        document.Data.Files["original"],
		Thumbnail:       document.Data.Files["thumbnail"],
		Revision:        document.Revision,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}
}

func RunArtifactCount(workspace Workspace, runID string) (int, error) {
	refs, err := listRunArtifactRefs(workspace, runID)
	if err != nil {
		return 0, err
	}
	return len(refs), nil
}

func ListRunArtifactSummaries(workspace Workspace, runID string) ([]RunArtifactSummary, error) {
	var summaries []RunArtifactSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListRunArtifactSummaries(runID)
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func NewRunNodeState(nodeID string, data RunNodeStateData) (Envelope[RunNodeStateData], error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return Envelope[RunNodeStateData]{}, NewError(ErrorInvalidArgument, "run node id is empty", 1, nil)
	}
	if strings.TrimSpace(data.NodeID) == "" {
		data.NodeID = nodeID
	}
	if data.NodeID != nodeID {
		return Envelope[RunNodeStateData]{}, NewError(ErrorInvalidArgument, "run node state id mismatch", 1, map[string]string{"id": nodeID, "nodeId": data.NodeID})
	}
	now := timeNowRFC3339()
	return Envelope[RunNodeStateData]{
		SchemaVersion: SchemaVersion,
		Kind:          KindRunNode,
		ID:            nodeID,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Data:          data,
	}, nil
}

func WriteRunNodeState(workspace Workspace, runID string, document Envelope[RunNodeStateData]) error {
	if err := RunRepository(workspace).validateID(runID); err != nil {
		return err
	}
	if err := validateRunNodeStateDocument(document); err != nil {
		return err
	}
	fileName := runNodeStateFileName(document.Data.NodeID)
	return withWorkspaceLock(workspace, func() error {
		if _, err := ReadRun(workspace, runID); err != nil {
			return err
		}
		if err := AtomicWriteJSON(runNodeStatePath(workspace, runID, document.Data.NodeID), document, 0o600); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertRunNodeState(runID, fileName, document)
		})
	})
}

func ListRunNodeStates(workspace Workspace, runID string) ([]Envelope[RunNodeStateData], error) {
	return listRunNodeStates(workspace, runID)
}

func listRunArtifactRefs(workspace Workspace, runID string) ([]Envelope[RunArtifactRefData], error) {
	if err := RunRepository(workspace).validateID(runID); err != nil {
		return nil, err
	}
	dir := filepath.Join(workspace.Root, "runs", runID, "artifacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Envelope[RunArtifactRefData]{}, nil
		}
		return nil, WrapError(ErrorInternal, "read run artifact refs", 5, err)
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	refs := []Envelope[RunArtifactRefData]{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ref.json") {
			continue
		}
		document, err := readEnvelopeFile[RunArtifactRefData](filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if err := validateRunArtifactRefDocument(document); err != nil {
			return nil, err
		}
		expectedName := document.ID + ".ref.json"
		if entry.Name() != expectedName {
			return nil, NewError(ErrorWorkspaceInvalid, "run artifact ref file name mismatch", 2, map[string]string{"fileName": entry.Name(), "id": document.ID})
		}
		refs = append(refs, document)
	}
	return refs, nil
}

func listRunNodeStates(workspace Workspace, runID string) ([]Envelope[RunNodeStateData], error) {
	if err := RunRepository(workspace).validateID(runID); err != nil {
		return nil, err
	}
	dir := filepath.Join(workspace.Root, "runs", runID, "nodes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Envelope[RunNodeStateData]{}, nil
		}
		return nil, WrapError(ErrorInternal, "read run node states", 5, err)
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	states := []Envelope[RunNodeStateData]{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		document, err := readEnvelopeFile[RunNodeStateData](filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if err := validateRunNodeStateDocument(document); err != nil {
			return nil, err
		}
		expectedName := runNodeStateFileName(document.Data.NodeID)
		if entry.Name() != expectedName {
			return nil, NewError(ErrorWorkspaceInvalid, "run node state file name mismatch", 2, map[string]string{"fileName": entry.Name(), "nodeId": document.Data.NodeID})
		}
		states = append(states, document)
	}
	return states, nil
}

func readEnvelopeFile[T any](filePath string) (Envelope[T], error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Envelope[T]{}, NewError(ErrorWorkspaceNotFound, "object document not found", 2, nil)
		}
		return Envelope[T]{}, WrapError(ErrorInternal, "read object document", 5, err)
	}
	var document Envelope[T]
	if err := json.Unmarshal(data, &document); err != nil {
		return Envelope[T]{}, WrapError(ErrorWorkspaceInvalid, "parse object document", 2, err)
	}
	return document, nil
}

func runArtifactRefPath(workspace Workspace, runID string, artifactID string) string {
	return filepath.Join(workspace.Root, "runs", runID, "artifacts", artifactID+".ref.json")
}

func runNodeStatePath(workspace Workspace, runID string, nodeID string) string {
	return filepath.Join(workspace.Root, "runs", runID, "nodes", runNodeStateFileName(nodeID))
}

func runNodeStateFileName(nodeID string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(nodeID))
	return encoded + ".json"
}

func validateRunData(data RunData) error {
	switch data.Status {
	case RunStatusPending, RunStatusRunning, RunStatusSuccess, RunStatusError, RunStatusCanceled:
		return nil
	default:
		return NewError(ErrorWorkspaceInvalid, "run status is not allowed", 2, map[string]string{"status": data.Status})
	}
}

func validateRunArtifactRefDocument(document Envelope[RunArtifactRefData]) error {
	if err := validateScannedEnvelope(envelopeHeader{
		SchemaVersion: document.SchemaVersion,
		Kind:          document.Kind,
		ID:            document.ID,
		Revision:      document.Revision,
		CreatedAt:     document.CreatedAt,
		UpdatedAt:     document.UpdatedAt,
	}, scanSpec{Kind: KindRunArtifactRef, IDPrefix: "art"}, document.ID); err != nil {
		return err
	}
	if strings.TrimSpace(document.Data.ArtifactID) == "" {
		return NewError(ErrorWorkspaceInvalid, "run artifact ref artifactId is empty", 2, nil)
	}
	if document.ID != document.Data.ArtifactID {
		return NewError(ErrorWorkspaceInvalid, "run artifact ref id must match artifactId", 2, map[string]string{"id": document.ID, "artifactId": document.Data.ArtifactID})
	}
	return nil
}

func validateRunNodeStateDocument(document Envelope[RunNodeStateData]) error {
	if document.SchemaVersion != SchemaVersion {
		return NewError(ErrorWorkspaceInvalid, "run node state schema version mismatch", 2, map[string]string{"schemaVersion": document.SchemaVersion})
	}
	if document.Kind != KindRunNode {
		return NewError(ErrorWorkspaceInvalid, "run node state kind mismatch", 2, map[string]string{"kind": document.Kind})
	}
	if strings.TrimSpace(document.ID) == "" {
		return NewError(ErrorWorkspaceInvalid, "run node state id is empty", 2, nil)
	}
	if strings.TrimSpace(document.Data.NodeID) == "" {
		return NewError(ErrorWorkspaceInvalid, "run node state nodeId is empty", 2, nil)
	}
	if document.ID != document.Data.NodeID {
		return NewError(ErrorWorkspaceInvalid, "run node state id must match nodeId", 2, map[string]string{"id": document.ID, "nodeId": document.Data.NodeID})
	}
	if strings.TrimSpace(document.Data.Status) == "" {
		return NewError(ErrorWorkspaceInvalid, "run node state status is empty", 2, map[string]string{"nodeId": document.Data.NodeID})
	}
	if document.Revision < 1 {
		return NewError(ErrorWorkspaceInvalid, "run node state revision must be at least 1", 2, map[string]string{"id": document.ID})
	}
	if _, err := time.Parse(time.RFC3339, document.CreatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse run node state createdAt", 2, err)
	}
	if _, err := time.Parse(time.RFC3339, document.UpdatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse run node state updatedAt", 2, err)
	}
	if strings.TrimSpace(document.Data.StartedAt) != "" {
		if _, err := time.Parse(time.RFC3339, document.Data.StartedAt); err != nil {
			return WrapError(ErrorWorkspaceInvalid, "parse run node state startedAt", 2, err)
		}
	}
	if strings.TrimSpace(document.Data.FinishedAt) != "" {
		if _, err := time.Parse(time.RFC3339, document.Data.FinishedAt); err != nil {
			return WrapError(ErrorWorkspaceInvalid, "parse run node state finishedAt", 2, err)
		}
	}
	return nil
}

func validateArtifactData(data ArtifactData) error {
	if err := validateArtifactType(data.Type); err != nil {
		return err
	}
	if strings.TrimSpace(data.Privacy) != "" {
		switch data.Privacy {
		case "private", "public", "shared":
		default:
			return NewError(ErrorWorkspaceInvalid, "artifact privacy is not allowed", 2, map[string]string{"privacy": data.Privacy})
		}
	}
	for name, filePath := range data.Files {
		if strings.TrimSpace(filePath) == "" {
			continue
		}
		if !isWorkspaceRelativeFile(filePath) {
			return NewError(ErrorWorkspaceInvalid, "artifact file path must stay inside artifact directory", 2, map[string]string{"file": name})
		}
	}
	return nil
}

func validateArtifactType(value string) error {
	switch value {
	case "image", "video", "text", "audio", "file":
		return nil
	default:
		return NewError(ErrorWorkspaceInvalid, "artifact type is not allowed", 2, map[string]string{"type": value})
	}
}

func isWorkspaceRelativeFile(value string) bool {
	normalized := strings.ReplaceAll(value, "\\", "/")
	if path.IsAbs(normalized) {
		return false
	}
	clean := path.Clean(normalized)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func ensureRunScaffold(workspace Workspace, runID string) error {
	for _, dir := range []string{
		filepath.Join(workspace.Root, "runs", runID, "nodes"),
		filepath.Join(workspace.Root, "runs", runID, "artifacts"),
		filepath.Join(workspace.Root, "runs", runID, "creative_canvas"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return WrapError(ErrorInternal, "create run directory", 5, err)
		}
	}
	eventsPath := runEventsPath(workspace, runID)
	file, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return WrapError(ErrorInternal, "create run events", 5, err)
	}
	if err := file.Close(); err != nil {
		return WrapError(ErrorInternal, "close run events", 5, err)
	}
	return nil
}

func writeRunTemplateSnapshot(workspace Workspace, run Envelope[RunData], snapshot *Envelope[TemplateData]) error {
	if snapshot == nil && strings.TrimSpace(run.Data.TemplateID) != "" {
		if document, err := ReadTemplate(workspace, run.Data.TemplateID); err == nil {
			snapshot = &document
		}
	}
	if snapshot == nil {
		return nil
	}
	if err := TemplateRepository(workspace).validateDocument(*snapshot); err != nil {
		return err
	}
	return AtomicWriteJSON(filepath.Join(workspace.Root, "runs", run.ID, "template.snapshot.json"), snapshot, 0o600)
}

func runStatusEventLevel(status string) string {
	if status == RunStatusError || status == RunStatusCanceled {
		return "warn"
	}
	return "info"
}

func timeNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func withWorkspaceLock(workspace Workspace, fn func() error) error {
	if strings.TrimSpace(workspace.Root) == "" {
		return NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	lockPath, err := workspaceWriteLockPath(workspace)
	if err != nil {
		return err
	}
	if err := ensurePrivateStateDir(filepath.Dir(lockPath)); err != nil {
		return err
	}
	lock, err := acquireWorkspaceWriteLock(lockPath, 2*time.Second)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

func acquireWorkspaceWriteLock(path string, timeout time.Duration) (*Lock, error) {
	deadline := time.Now().Add(timeout)
	for {
		lock, err := AcquireLock(path)
		if err == nil {
			return lock, nil
		}
		var workspaceErr *Error
		if !asLocalWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceLocked || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func asLocalWorkspaceError(err error, target **Error) bool {
	if err == nil {
		return false
	}
	typed, ok := err.(*Error)
	if !ok {
		return false
	}
	*target = typed
	return true
}
