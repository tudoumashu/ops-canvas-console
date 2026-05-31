package localworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WorkspaceIndex struct {
	db *gorm.DB
}

type IndexTemplateRecord struct {
	ID           string `gorm:"primaryKey;column:id"`
	Title        string `gorm:"column:title"`
	Description  string `gorm:"column:description"`
	WorkflowType string `gorm:"column:workflow_type;index"`
	Version      int    `gorm:"column:version"`
	Revision     int    `gorm:"column:revision"`
	CreatedAt    string `gorm:"column:created_at"`
	UpdatedAt    string `gorm:"column:updated_at;index"`
}

func (IndexTemplateRecord) TableName() string { return "index_templates" }

type IndexRunRecord struct {
	ID                  string `gorm:"primaryKey;column:id"`
	Status              string `gorm:"column:status;index"`
	TemplateID          string `gorm:"column:template_id;index"`
	ProfileID           string `gorm:"column:profile_id;index"`
	ProjectID           string `gorm:"column:project_id;index"`
	ArtifactCount       int    `gorm:"column:artifact_count"`
	LatestEventSequence int64  `gorm:"column:latest_event_sequence"`
	Revision            int    `gorm:"column:revision"`
	CreatedAt           string `gorm:"column:created_at"`
	UpdatedAt           string `gorm:"column:updated_at;index"`
}

func (IndexRunRecord) TableName() string { return "index_runs" }

type IndexArtifactRecord struct {
	ID              string   `gorm:"primaryKey;column:id"`
	Type            string   `gorm:"column:type;index"`
	MIME            string   `gorm:"column:mime"`
	Title           string   `gorm:"column:title"`
	SHA256          string   `gorm:"column:sha256"`
	Bytes           int64    `gorm:"column:bytes"`
	Width           int      `gorm:"column:width"`
	Height          int      `gorm:"column:height"`
	DurationSeconds *float64 `gorm:"column:duration_seconds"`
	Privacy         string   `gorm:"column:privacy;index"`
	Original        string   `gorm:"column:original"`
	Thumbnail       string   `gorm:"column:thumbnail"`
	Revision        int      `gorm:"column:revision"`
	CreatedAt       string   `gorm:"column:created_at"`
	UpdatedAt       string   `gorm:"column:updated_at;index"`
}

func (IndexArtifactRecord) TableName() string { return "index_artifacts" }

type IndexProfileRecord struct {
	ID           string `gorm:"primaryKey;column:id"`
	Name         string `gorm:"column:name"`
	Mode         string `gorm:"column:mode;index"`
	ChannelCount int    `gorm:"column:channel_count"`
	Revision     int    `gorm:"column:revision"`
	CreatedAt    string `gorm:"column:created_at"`
	UpdatedAt    string `gorm:"column:updated_at;index"`
}

func (IndexProfileRecord) TableName() string { return "index_profiles" }

type IndexProjectRecord struct {
	ID                      string `gorm:"primaryKey;column:id"`
	Name                    string `gorm:"column:name"`
	Kind                    string `gorm:"column:kind;index"`
	Adapter                 string `gorm:"column:adapter;index"`
	HasRootPath             bool   `gorm:"column:has_root_path"`
	RootFingerprint         string `gorm:"column:root_fingerprint;index"`
	CapabilityFSRead        bool   `gorm:"column:capability_fs_read"`
	CapabilityFSWrite       bool   `gorm:"column:capability_fs_write"`
	CapabilityProcessExec   bool   `gorm:"column:capability_process_exec"`
	CapabilityNetworkLocal  bool   `gorm:"column:capability_network_local"`
	CapabilityArtifactWrite bool   `gorm:"column:capability_artifact_write"`
	Revision                int    `gorm:"column:revision"`
	CreatedAt               string `gorm:"column:created_at"`
	UpdatedAt               string `gorm:"column:updated_at;index"`
}

func (IndexProjectRecord) TableName() string { return "index_projects" }

type IndexAssetRecord struct {
	ID               string `gorm:"primaryKey;column:id"`
	Type             string `gorm:"column:type;index"`
	MIME             string `gorm:"column:mime"`
	Title            string `gorm:"column:title"`
	MediaType        string `gorm:"column:media_type;index"`
	CategoryPath     string `gorm:"column:category_path;index"`
	Purpose          string `gorm:"column:purpose;index"`
	Source           string `gorm:"column:source;index"`
	SourceArtifactID string `gorm:"column:source_artifact_id;index"`
	Privacy          string `gorm:"column:privacy;index"`
	TagsJSON         string `gorm:"column:tags_json"`
	Original         string `gorm:"column:original"`
	Thumbnail        string `gorm:"column:thumbnail"`
	Revision         int    `gorm:"column:revision"`
	CreatedAt        string `gorm:"column:created_at"`
	UpdatedAt        string `gorm:"column:updated_at;index"`
}

func (IndexAssetRecord) TableName() string { return "index_assets" }

type IndexPromptRecord struct {
	ID         string `gorm:"primaryKey;column:id"`
	Title      string `gorm:"column:title"`
	Kind       string `gorm:"column:kind;index"`
	Category   string `gorm:"column:category;index"`
	Domain     string `gorm:"column:domain;index"`
	Stage      string `gorm:"column:stage;index"`
	Provider   string `gorm:"column:provider;index"`
	Model      string `gorm:"column:model;index"`
	Mode       string `gorm:"column:mode;index"`
	InputType  string `gorm:"column:input_type;index"`
	OutputType string `gorm:"column:output_type;index"`
	Status     string `gorm:"column:status;index"`
	Privacy    string `gorm:"column:privacy;index"`
	TagsJSON   string `gorm:"column:tags_json"`
	HasContent bool   `gorm:"column:has_content"`
	Revision   int    `gorm:"column:revision"`
	CreatedAt  string `gorm:"column:created_at"`
	UpdatedAt  string `gorm:"column:updated_at;index"`
}

func (IndexPromptRecord) TableName() string { return "index_prompts" }

type IndexCanvasProjectRecord struct {
	ID              string `gorm:"primaryKey;column:id"`
	Title           string `gorm:"column:title"`
	NodeCount       int    `gorm:"column:node_count"`
	ConnectionCount int    `gorm:"column:connection_count"`
	FileCount       int    `gorm:"column:file_count"`
	Revision        int    `gorm:"column:revision"`
	CreatedAt       string `gorm:"column:created_at"`
	UpdatedAt       string `gorm:"column:updated_at;index"`
}

func (IndexCanvasProjectRecord) TableName() string { return "index_canvas_projects" }

type IndexWorkbenchLogRecord struct {
	ID              string `gorm:"primaryKey;column:id"`
	Modality        string `gorm:"column:modality;index"`
	Title           string `gorm:"column:title"`
	Status          string `gorm:"column:status;index"`
	Model           string `gorm:"column:model;index"`
	CreatedAtMillis int64  `gorm:"column:created_at_millis;index"`
	MediaCount      int    `gorm:"column:media_count"`
	Revision        int    `gorm:"column:revision"`
	CreatedAt       string `gorm:"column:created_at"`
	UpdatedAt       string `gorm:"column:updated_at;index"`
}

func (IndexWorkbenchLogRecord) TableName() string { return "index_workbench_logs" }

type IndexRunArtifactRefRecord struct {
	RunID      string `gorm:"primaryKey;column:run_id"`
	ArtifactID string `gorm:"primaryKey;column:artifact_id"`
	Role       string `gorm:"column:role"`
	NodeID     string `gorm:"column:node_id;index"`
	Slot       string `gorm:"column:slot"`
	RefOrder   int    `gorm:"column:ref_order;index"`
	Revision   int    `gorm:"column:revision"`
	CreatedAt  string `gorm:"column:created_at"`
	UpdatedAt  string `gorm:"column:updated_at"`
}

func (IndexRunArtifactRefRecord) TableName() string { return "index_run_artifact_refs" }

type IndexRunNodeStateRecord struct {
	RunID        string `gorm:"primaryKey;column:run_id"`
	NodeID       string `gorm:"primaryKey;column:node_id"`
	FileName     string `gorm:"column:file_name"`
	Status       string `gorm:"column:status;index"`
	StartedAt    string `gorm:"column:started_at"`
	FinishedAt   string `gorm:"column:finished_at"`
	Error        string `gorm:"column:error"`
	OutputJSON   string `gorm:"column:output_json"`
	MetadataJSON string `gorm:"column:metadata_json"`
	Revision     int    `gorm:"column:revision"`
	UpdatedAt    string `gorm:"column:updated_at;index"`
}

func (IndexRunNodeStateRecord) TableName() string { return "index_run_node_states" }

type IndexRunEventRecord struct {
	RunID     string `gorm:"primaryKey;column:run_id"`
	Sequence  int64  `gorm:"primaryKey;column:sequence;autoIncrement:false"`
	EventID   string `gorm:"column:event_id;uniqueIndex"`
	Type      string `gorm:"column:type;index"`
	Level     string `gorm:"column:level;index"`
	Message   string `gorm:"column:message"`
	CreatedAt string `gorm:"column:created_at;index"`
}

func (IndexRunEventRecord) TableName() string { return "index_run_events" }

type IndexMetadataRecord struct {
	Key       string `gorm:"primaryKey;column:key"`
	Value     string `gorm:"column:value"`
	UpdatedAt string `gorm:"column:updated_at"`
}

func (IndexMetadataRecord) TableName() string { return "index_metadata" }

type SQLiteIndexRebuilder struct{}

func OpenIndex(workspace Workspace) (*WorkspaceIndex, error) {
	if workspace.Root == "" {
		return nil, NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	db, err := gorm.Open(sqlite.Open(workspace.Path(IndexFileName)), &gorm.Config{})
	if err != nil {
		return nil, WrapError(ErrorWorkspaceInvalid, "open workspace index", 2, err)
	}
	index := &WorkspaceIndex{db: db}
	if err := index.migrate(); err != nil {
		index.Close()
		return nil, err
	}
	return index, nil
}

func (i *WorkspaceIndex) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	db, err := i.db.DB()
	if err != nil {
		return WrapError(ErrorInternal, "open raw index db", 5, err)
	}
	if err := db.Close(); err != nil {
		return WrapError(ErrorInternal, "close workspace index", 5, err)
	}
	return nil
}

func (i *WorkspaceIndex) migrate() error {
	if err := i.db.AutoMigrate(
		&IndexTemplateRecord{},
		&IndexRunRecord{},
		&IndexArtifactRecord{},
		&IndexProfileRecord{},
		&IndexProjectRecord{},
		&IndexAssetRecord{},
		&IndexPromptRecord{},
		&IndexCanvasProjectRecord{},
		&IndexWorkbenchLogRecord{},
		&IndexRunArtifactRefRecord{},
		&IndexRunNodeStateRecord{},
		&IndexRunEventRecord{},
		&IndexMetadataRecord{},
	); err != nil {
		return WrapError(ErrorInternal, "migrate workspace index", 5, err)
	}
	return nil
}

func (i *WorkspaceIndex) UpsertTemplate(document Envelope[TemplateData]) error {
	record := IndexTemplateRecord{
		ID:           document.ID,
		Title:        document.Data.Title,
		Description:  document.Data.Description,
		WorkflowType: document.Data.WorkflowType,
		Version:      document.Data.Version,
		Revision:     document.Revision,
		CreatedAt:    document.CreatedAt,
		UpdatedAt:    document.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertRun(workspace Workspace, document Envelope[RunData]) error {
	count, err := RunArtifactCount(workspace, document.ID)
	if err != nil {
		return err
	}
	latest, err := LatestRunEventSequence(workspace, document.ID)
	if err != nil {
		return err
	}
	record := IndexRunRecord{
		ID:                  document.ID,
		Status:              document.Data.Status,
		TemplateID:          document.Data.TemplateID,
		ProfileID:           document.Data.ProfileID,
		ProjectID:           document.Data.ProjectID,
		ArtifactCount:       count,
		LatestEventSequence: latest,
		Revision:            document.Revision,
		CreatedAt:           document.CreatedAt,
		UpdatedAt:           document.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertArtifact(document Envelope[ArtifactData]) error {
	summary := ArtifactDocumentSummary(document)
	record := IndexArtifactRecord{
		ID:              summary.ID,
		Type:            summary.Type,
		MIME:            summary.MIME,
		Title:           summary.Title,
		SHA256:          summary.SHA256,
		Bytes:           summary.Bytes,
		Width:           summary.Width,
		Height:          summary.Height,
		DurationSeconds: summary.DurationSeconds,
		Privacy:         summary.Privacy,
		Original:        summary.Original,
		Thumbnail:       summary.Thumbnail,
		Revision:        summary.Revision,
		CreatedAt:       summary.CreatedAt,
		UpdatedAt:       summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertProfile(document Envelope[ProfileData]) error {
	summary := ProfileDocumentSummary(document)
	record := IndexProfileRecord{
		ID:           summary.ID,
		Name:         summary.Name,
		Mode:         summary.Mode,
		ChannelCount: summary.ChannelCount,
		Revision:     summary.Revision,
		CreatedAt:    summary.CreatedAt,
		UpdatedAt:    summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertProject(document Envelope[ProjectData]) error {
	summary := ProjectDocumentSummary(document)
	record := IndexProjectRecord{
		ID:                      summary.ID,
		Name:                    summary.Name,
		Kind:                    summary.Kind,
		Adapter:                 summary.Adapter,
		HasRootPath:             summary.HasRootPath,
		RootFingerprint:         summary.RootFingerprint,
		CapabilityFSRead:        summary.Capabilities.FSRead,
		CapabilityFSWrite:       summary.Capabilities.FSWrite,
		CapabilityProcessExec:   summary.Capabilities.ProcessExec,
		CapabilityNetworkLocal:  summary.Capabilities.NetworkLocal,
		CapabilityArtifactWrite: summary.Capabilities.ArtifactWrite,
		Revision:                summary.Revision,
		CreatedAt:               summary.CreatedAt,
		UpdatedAt:               summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertAsset(document Envelope[AssetData]) error {
	summary := AssetDocumentSummary(document)
	record := IndexAssetRecord{
		ID:               summary.ID,
		Type:             summary.Type,
		MIME:             summary.MIME,
		Title:            summary.Title,
		MediaType:        summary.MediaType,
		CategoryPath:     summary.CategoryPath,
		Purpose:          summary.Purpose,
		Source:           summary.Source,
		SourceArtifactID: summary.SourceArtifactID,
		Privacy:          summary.Privacy,
		TagsJSON:         encodeStringList(summary.Tags),
		Original:         summary.Original,
		Thumbnail:        summary.Thumbnail,
		Revision:         summary.Revision,
		CreatedAt:        summary.CreatedAt,
		UpdatedAt:        summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertPrompt(workspace Workspace, document Envelope[PromptData]) error {
	summary := PromptDocumentSummary(workspace, document)
	record := IndexPromptRecord{
		ID:         summary.ID,
		Title:      summary.Title,
		Kind:       summary.Kind,
		Category:   summary.Category,
		Domain:     summary.Domain,
		Stage:      summary.Stage,
		Provider:   summary.Provider,
		Model:      summary.Model,
		Mode:       summary.Mode,
		InputType:  summary.InputType,
		OutputType: summary.OutputType,
		Status:     summary.Status,
		Privacy:    summary.Privacy,
		TagsJSON:   encodeStringList(summary.Tags),
		HasContent: summary.HasContent,
		Revision:   summary.Revision,
		CreatedAt:  summary.CreatedAt,
		UpdatedAt:  summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertCanvasProject(document Envelope[CanvasProjectData]) error {
	summary := CanvasProjectDocumentSummary(document)
	record := IndexCanvasProjectRecord{
		ID:              summary.ID,
		Title:           summary.Title,
		NodeCount:       summary.NodeCount,
		ConnectionCount: summary.ConnectionCount,
		FileCount:       summary.FileCount,
		Revision:        summary.Revision,
		CreatedAt:       summary.CreatedAt,
		UpdatedAt:       summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertWorkbenchLog(document Envelope[WorkbenchLogData]) error {
	summary := WorkbenchLogDocumentSummary(document)
	record := IndexWorkbenchLogRecord{
		ID:              summary.ID,
		Modality:        summary.Modality,
		Title:           summary.Title,
		Status:          summary.Status,
		Model:           summary.Model,
		CreatedAtMillis: summary.CreatedAtMillis,
		MediaCount:      summary.MediaCount,
		Revision:        summary.Revision,
		CreatedAt:       summary.CreatedAt,
		UpdatedAt:       summary.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) DeleteProfile(id string) error {
	return i.deleteByID(&IndexProfileRecord{}, id)
}

func (i *WorkspaceIndex) DeleteTemplate(id string) error {
	return i.deleteByID(&IndexTemplateRecord{}, id)
}

func (i *WorkspaceIndex) DeleteProject(id string) error {
	return i.deleteByID(&IndexProjectRecord{}, id)
}

func (i *WorkspaceIndex) DeleteAsset(id string) error {
	return i.deleteByID(&IndexAssetRecord{}, id)
}

func (i *WorkspaceIndex) DeletePrompt(id string) error {
	return i.deleteByID(&IndexPromptRecord{}, id)
}

func (i *WorkspaceIndex) DeleteCanvasProject(id string) error {
	return i.deleteByID(&IndexCanvasProjectRecord{}, id)
}

func (i *WorkspaceIndex) DeleteWorkbenchLog(id string) error {
	return i.deleteByID(&IndexWorkbenchLogRecord{}, id)
}

func (i *WorkspaceIndex) deleteByID(record any, id string) error {
	if err := i.db.Where("id = ?", id).Delete(record).Error; err != nil {
		return WrapError(ErrorInternal, "delete workspace index record", 5, err)
	}
	return nil
}

func (i *WorkspaceIndex) UpsertRunArtifactRef(runID string, document Envelope[RunArtifactRefData]) error {
	record := IndexRunArtifactRefRecord{
		RunID:      runID,
		ArtifactID: document.Data.ArtifactID,
		Role:       document.Data.Role,
		NodeID:     document.Data.NodeID,
		Slot:       document.Data.Slot,
		RefOrder:   document.Data.Order,
		Revision:   document.Revision,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertRunNodeState(runID string, fileName string, document Envelope[RunNodeStateData]) error {
	record := IndexRunNodeStateRecord{
		RunID:        runID,
		NodeID:       document.Data.NodeID,
		FileName:     fileName,
		Status:       document.Data.Status,
		StartedAt:    document.Data.StartedAt,
		FinishedAt:   document.Data.FinishedAt,
		Error:        document.Data.Error,
		OutputJSON:   encodeMap(document.Data.Output),
		MetadataJSON: encodeMap(document.Data.Metadata),
		Revision:     document.Revision,
		UpdatedAt:    document.UpdatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) UpsertRunEvent(runID string, event RunEventEnvelope) error {
	record := IndexRunEventRecord{
		RunID:     runID,
		Sequence:  event.Sequence,
		EventID:   event.ID,
		Type:      event.Type,
		Level:     event.Level,
		Message:   event.Message,
		CreatedAt: event.CreatedAt,
	}
	return i.upsert(&record)
}

func (i *WorkspaceIndex) ListTemplateSummaries() ([]TemplateSummary, error) {
	var records []IndexTemplateRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query template index", 5, err)
	}
	items := make([]TemplateSummary, 0, len(records))
	for _, record := range records {
		items = append(items, TemplateSummary{
			ID:           record.ID,
			Title:        record.Title,
			Description:  record.Description,
			WorkflowType: record.WorkflowType,
			Version:      record.Version,
			Revision:     record.Revision,
			CreatedAt:    record.CreatedAt,
			UpdatedAt:    record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListRunSummaries() ([]RunSummary, error) {
	var records []IndexRunRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query run index", 5, err)
	}
	items := make([]RunSummary, 0, len(records))
	for _, record := range records {
		items = append(items, runSummaryFromIndex(record))
	}
	return items, nil
}

func (i *WorkspaceIndex) GetRunSummary(runID string) (RunSummary, error) {
	var record IndexRunRecord
	err := i.db.Where("id = ?", runID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RunSummary{}, NewError(ErrorWorkspaceNotFound, "run index record not found", 2, map[string]string{"id": runID})
	}
	if err != nil {
		return RunSummary{}, WrapError(ErrorInternal, "query run index", 5, err)
	}
	return runSummaryFromIndex(record), nil
}

func (i *WorkspaceIndex) ListArtifactSummaries() ([]ArtifactSummary, error) {
	var records []IndexArtifactRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query artifact index", 5, err)
	}
	items := make([]ArtifactSummary, 0, len(records))
	for _, record := range records {
		items = append(items, artifactSummaryFromIndex(record))
	}
	return items, nil
}

func (i *WorkspaceIndex) ListProfileSummaries() ([]ProfileSummary, error) {
	var records []IndexProfileRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query profile index", 5, err)
	}
	items := make([]ProfileSummary, 0, len(records))
	for _, record := range records {
		items = append(items, ProfileSummary{
			ID:           record.ID,
			Name:         record.Name,
			Mode:         record.Mode,
			ChannelCount: record.ChannelCount,
			Revision:     record.Revision,
			CreatedAt:    record.CreatedAt,
			UpdatedAt:    record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListProjectSummaries() ([]ProjectSummary, error) {
	var records []IndexProjectRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query project index", 5, err)
	}
	items := make([]ProjectSummary, 0, len(records))
	for _, record := range records {
		items = append(items, ProjectSummary{
			ID:              record.ID,
			Name:            record.Name,
			Kind:            record.Kind,
			Adapter:         record.Adapter,
			HasRootPath:     record.HasRootPath,
			RootFingerprint: record.RootFingerprint,
			Capabilities: ProjectCapabilities{
				FSRead:        record.CapabilityFSRead,
				FSWrite:       record.CapabilityFSWrite,
				ProcessExec:   record.CapabilityProcessExec,
				NetworkLocal:  record.CapabilityNetworkLocal,
				ArtifactWrite: record.CapabilityArtifactWrite,
			},
			Revision:  record.Revision,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListAssetSummaries() ([]AssetSummary, error) {
	var records []IndexAssetRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query asset index", 5, err)
	}
	items := make([]AssetSummary, 0, len(records))
	for _, record := range records {
		items = append(items, AssetSummary{
			ID:               record.ID,
			Type:             record.Type,
			MIME:             record.MIME,
			Title:            record.Title,
			MediaType:        record.MediaType,
			CategoryPath:     record.CategoryPath,
			Purpose:          record.Purpose,
			Source:           record.Source,
			SourceArtifactID: record.SourceArtifactID,
			Privacy:          record.Privacy,
			Tags:             decodeStringList(record.TagsJSON),
			Original:         record.Original,
			Thumbnail:        record.Thumbnail,
			Revision:         record.Revision,
			CreatedAt:        record.CreatedAt,
			UpdatedAt:        record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListPromptSummaries() ([]PromptSummary, error) {
	var records []IndexPromptRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query prompt index", 5, err)
	}
	items := make([]PromptSummary, 0, len(records))
	for _, record := range records {
		items = append(items, PromptSummary{
			ID:         record.ID,
			Title:      record.Title,
			Kind:       record.Kind,
			Category:   record.Category,
			Domain:     record.Domain,
			Stage:      record.Stage,
			Provider:   record.Provider,
			Model:      record.Model,
			Mode:       record.Mode,
			InputType:  record.InputType,
			OutputType: record.OutputType,
			Status:     record.Status,
			Privacy:    record.Privacy,
			Tags:       decodeStringList(record.TagsJSON),
			HasContent: record.HasContent,
			Revision:   record.Revision,
			CreatedAt:  record.CreatedAt,
			UpdatedAt:  record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListCanvasProjectSummaries() ([]CanvasProjectSummary, error) {
	var records []IndexCanvasProjectRecord
	if err := i.db.Order("updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query canvas project index", 5, err)
	}
	items := make([]CanvasProjectSummary, 0, len(records))
	for _, record := range records {
		items = append(items, CanvasProjectSummary{
			ID:              record.ID,
			Title:           record.Title,
			NodeCount:       record.NodeCount,
			ConnectionCount: record.ConnectionCount,
			FileCount:       record.FileCount,
			Revision:        record.Revision,
			CreatedAt:       record.CreatedAt,
			UpdatedAt:       record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListWorkbenchLogSummaries(modality string) ([]WorkbenchLogSummary, error) {
	var records []IndexWorkbenchLogRecord
	query := i.db
	if modality != "" {
		query = query.Where("modality = ?", modality)
	}
	if err := query.Order("created_at_millis desc, updated_at desc, id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query workbench log index", 5, err)
	}
	items := make([]WorkbenchLogSummary, 0, len(records))
	for _, record := range records {
		items = append(items, WorkbenchLogSummary{
			ID:              record.ID,
			Modality:        record.Modality,
			Title:           record.Title,
			Status:          record.Status,
			Model:           record.Model,
			CreatedAtMillis: record.CreatedAtMillis,
			MediaCount:      record.MediaCount,
			Revision:        record.Revision,
			CreatedAt:       record.CreatedAt,
			UpdatedAt:       record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListRunArtifactSummaries(runID string) ([]RunArtifactSummary, error) {
	var refs []IndexRunArtifactRefRecord
	if err := i.db.Where("run_id = ?", runID).Order("ref_order asc, artifact_id asc").Find(&refs).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query run artifact ref index", 5, err)
	}
	items := make([]RunArtifactSummary, 0, len(refs))
	for _, ref := range refs {
		var artifact IndexArtifactRecord
		err := i.db.Where("id = ?", ref.ArtifactID).First(&artifact).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, NewError(ErrorWorkspaceInvalid, "run artifact index ref points to missing artifact", 2, map[string]string{"artifactId": ref.ArtifactID})
		}
		if err != nil {
			return nil, WrapError(ErrorInternal, "query run artifact index", 5, err)
		}
		items = append(items, RunArtifactSummary{
			Artifact: artifactSummaryFromIndex(artifact),
			Ref: RunArtifactRefData{
				ArtifactID: ref.ArtifactID,
				Role:       ref.Role,
				NodeID:     ref.NodeID,
				Slot:       ref.Slot,
				Order:      ref.RefOrder,
			},
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) ListRunNodeStateSummaries(runID string) ([]RunNodeStateSummary, error) {
	var records []IndexRunNodeStateRecord
	if err := i.db.Where("run_id = ?", runID).Order("node_id asc").Find(&records).Error; err != nil {
		return nil, WrapError(ErrorInternal, "query run node state index", 5, err)
	}
	items := make([]RunNodeStateSummary, 0, len(records))
	for _, record := range records {
		items = append(items, RunNodeStateSummary{
			NodeID:     record.NodeID,
			Status:     record.Status,
			StartedAt:  record.StartedAt,
			FinishedAt: record.FinishedAt,
			Error:      record.Error,
			Output:     decodeMap(record.OutputJSON),
			Metadata:   decodeMap(record.MetadataJSON),
			Revision:   record.Revision,
			UpdatedAt:  record.UpdatedAt,
		})
	}
	return items, nil
}

func (i *WorkspaceIndex) upsert(value any) error {
	if err := i.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(value).Error; err != nil {
		return WrapError(ErrorInternal, "upsert workspace index", 5, err)
	}
	return nil
}

func (SQLiteIndexRebuilder) Rebuild(ctx context.Context, workspace Workspace, scan ScanResult) error {
	_ = scan
	if err := ctx.Err(); err != nil {
		return err
	}
	index, err := OpenIndex(workspace)
	if err != nil {
		return err
	}
	defer index.Close()

	return index.db.Transaction(func(tx *gorm.DB) error {
		if err := clearIndexTables(tx); err != nil {
			return err
		}
		rebuild := &WorkspaceIndex{db: tx}
		if err := rebuildProfiles(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildProjects(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildTemplates(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildArtifacts(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildAssets(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildPrompts(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildCanvasProjects(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildWorkbenchLogs(ctx, workspace, rebuild); err != nil {
			return err
		}
		if err := rebuildRuns(ctx, workspace, rebuild); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		record := IndexMetadataRecord{
			Key:       "last_rebuild",
			Value:     now,
			UpdatedAt: now,
		}
		return rebuild.upsert(&record)
	})
}

func clearIndexTables(tx *gorm.DB) error {
	tables := []any{
		&IndexTemplateRecord{},
		&IndexRunRecord{},
		&IndexArtifactRecord{},
		&IndexProfileRecord{},
		&IndexProjectRecord{},
		&IndexAssetRecord{},
		&IndexPromptRecord{},
		&IndexCanvasProjectRecord{},
		&IndexWorkbenchLogRecord{},
		&IndexRunArtifactRefRecord{},
		&IndexRunNodeStateRecord{},
		&IndexRunEventRecord{},
		&IndexMetadataRecord{},
	}
	for _, table := range tables {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(table).Error; err != nil {
			return WrapError(ErrorInternal, "clear workspace index", 5, err)
		}
	}
	return nil
}

func rebuildProfiles(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListProfiles(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertProfile(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildProjects(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListProjects(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertProject(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildTemplates(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListTemplates(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertTemplate(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildArtifacts(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListArtifacts(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertArtifact(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildAssets(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListAssets(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertAsset(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildPrompts(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListPrompts(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertPrompt(workspace, document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildCanvasProjects(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListCanvasProjects(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertCanvasProject(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildWorkbenchLogs(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	documents, err := ListWorkbenchLogs(workspace)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := index.UpsertWorkbenchLog(document); err != nil {
			return err
		}
	}
	return nil
}

func rebuildRuns(ctx context.Context, workspace Workspace, index *WorkspaceIndex) error {
	runs, err := ListRuns(workspace)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		refs, err := listRunArtifactRefs(workspace, run.ID)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if err := index.UpsertRunArtifactRef(run.ID, ref); err != nil {
				return err
			}
		}
		nodes, err := listRunNodeStates(workspace, run.ID)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			fileName := runNodeStateFileName(node.Data.NodeID)
			if err := index.UpsertRunNodeState(run.ID, fileName, node); err != nil {
				return err
			}
		}
		events, err := ReadRunEvents(workspace, run.ID, 0)
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := index.UpsertRunEvent(run.ID, event); err != nil {
				return err
			}
		}
		if err := index.UpsertRun(workspace, run); err != nil {
			return err
		}
	}
	return nil
}

func runSummaryFromIndex(record IndexRunRecord) RunSummary {
	return RunSummary{
		ID:                  record.ID,
		Status:              record.Status,
		TemplateID:          record.TemplateID,
		ProfileID:           record.ProfileID,
		ProjectID:           record.ProjectID,
		ArtifactCount:       record.ArtifactCount,
		LatestEventSequence: record.LatestEventSequence,
		Revision:            record.Revision,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}

func artifactSummaryFromIndex(record IndexArtifactRecord) ArtifactSummary {
	return ArtifactSummary{
		ID:              record.ID,
		Type:            record.Type,
		MIME:            record.MIME,
		Title:           record.Title,
		SHA256:          record.SHA256,
		Bytes:           record.Bytes,
		Width:           record.Width,
		Height:          record.Height,
		DurationSeconds: record.DurationSeconds,
		Privacy:         record.Privacy,
		Original:        record.Original,
		Thumbnail:       record.Thumbnail,
		Revision:        record.Revision,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func encodeStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeStringList(value string) []string {
	if value == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return []string{}
	}
	return values
}

func encodeMap(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeMap(value string) map[string]any {
	if strings.TrimSpace(value) == "" || value == "{}" {
		return nil
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func withIndex(workspace Workspace, fn func(*WorkspaceIndex) error) error {
	index, err := OpenIndex(workspace)
	if err != nil {
		return err
	}
	defer index.Close()
	return fn(index)
}
