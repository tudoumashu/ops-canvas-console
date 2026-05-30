package localworkspace

import (
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	WorkbenchModalityText  = "text"
	WorkbenchModalityImage = "image"
	WorkbenchModalityVideo = "video"

	WorkbenchLogStatusSuccess = "success"
	WorkbenchLogStatusError   = "error"
)

type WorkbenchLogData struct {
	Modality        string              `json:"modality"`
	Title           string              `json:"title,omitempty"`
	CreatedAtMillis int64               `json:"createdAtMillis,omitempty"`
	Status          string              `json:"status,omitempty"`
	Model           string              `json:"model,omitempty"`
	Prompt          string              `json:"prompt,omitempty"`
	Media           []WorkbenchLogMedia `json:"media,omitempty"`
	Payload         map[string]any      `json:"payload,omitempty"`
	Metrics         map[string]any      `json:"metrics,omitempty"`
	Metadata        map[string]any      `json:"metadata,omitempty"`
}

type WorkbenchLogMedia struct {
	Key        string         `json:"key"`
	Role       string         `json:"role,omitempty"`
	Name       string         `json:"name,omitempty"`
	MIME       string         `json:"mime,omitempty"`
	Path       string         `json:"path,omitempty"`
	Width      int            `json:"width,omitempty"`
	Height     int            `json:"height,omitempty"`
	Bytes      int64          `json:"bytes,omitempty"`
	DurationMs float64        `json:"durationMs,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type WorkbenchLogSummary struct {
	ID              string `json:"id"`
	Modality        string `json:"modality"`
	Title           string `json:"title,omitempty"`
	Status          string `json:"status,omitempty"`
	Model           string `json:"model,omitempty"`
	CreatedAtMillis int64  `json:"createdAtMillis,omitempty"`
	MediaCount      int    `json:"mediaCount"`
	Revision        int    `json:"revision"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type WorkbenchLogImportFile struct {
	Key         string
	File        multipart.File
	Header      *multipart.FileHeader
	ContentType string
}

func WorkbenchLogRepository(workspace Workspace) Repository[WorkbenchLogData] {
	return Repository[WorkbenchLogData]{
		Workspace:  workspace,
		Collection: "workbench-logs",
		FileName:   "workbench-log.json",
		Kind:       KindWorkbenchLog,
		IDPrefix:   "wblog",
	}
}

func NewWorkbenchLog(workspace Workspace, data WorkbenchLogData) (Envelope[WorkbenchLogData], error) {
	data = prepareWorkbenchLogData(data)
	if err := validateWorkbenchLogData(data); err != nil {
		return Envelope[WorkbenchLogData]{}, err
	}
	return WorkbenchLogRepository(workspace).New(data)
}

func WriteWorkbenchLog(workspace Workspace, document Envelope[WorkbenchLogData]) error {
	document.Data = prepareWorkbenchLogData(document.Data)
	if err := validateWorkbenchLogData(document.Data); err != nil {
		return err
	}
	if err := WorkbenchLogRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		return writeWorkbenchLogUnlocked(workspace, document)
	})
}

func SaveWorkbenchLog(workspace Workspace, document Envelope[WorkbenchLogData], files []WorkbenchLogImportFile) error {
	document.Data = prepareWorkbenchLogData(document.Data)
	for _, file := range files {
		if err := validatePathComponent("workbench log file key", file.Key); err != nil {
			return err
		}
		relPath, err := workbenchLogUploadRelPath(file.Key, file.ContentType)
		if err != nil {
			return err
		}
		document.Data = upsertWorkbenchLogMedia(document.Data, file, relPath)
	}
	if err := validateWorkbenchLogData(document.Data); err != nil {
		return err
	}
	if err := WorkbenchLogRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	if err := withWorkspaceLock(workspace, func() error {
		for _, file := range files {
			media, ok := workbenchLogMediaByKey(document.Data.Media, file.Key)
			if !ok {
				return NewError(ErrorInvalidArgument, "workbench log media file key is not referenced", 1, map[string]string{"fileKey": file.Key})
			}
			filePath := filepath.Join(WorkbenchLogRepository(workspace).Dir(document.ID), filepath.FromSlash(media.Path))
			if err := AtomicWriteFromReader(filePath, file.File, 0o600); err != nil {
				return err
			}
		}
		return writeWorkbenchLogUnlocked(workspace, document)
	}); err != nil {
		if document.Revision == 1 {
			_ = os.RemoveAll(WorkbenchLogRepository(workspace).Dir(document.ID))
		}
		return err
	}
	return nil
}

func ReadWorkbenchLog(workspace Workspace, id string) (Envelope[WorkbenchLogData], error) {
	document, err := WorkbenchLogRepository(workspace).Read(id)
	if err != nil {
		return Envelope[WorkbenchLogData]{}, err
	}
	document.Data = prepareWorkbenchLogData(document.Data)
	if err := validateWorkbenchLogData(document.Data); err != nil {
		return Envelope[WorkbenchLogData]{}, err
	}
	return document, nil
}

func ListWorkbenchLogs(workspace Workspace) ([]Envelope[WorkbenchLogData], error) {
	documents, err := WorkbenchLogRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for index := range documents {
		documents[index].Data = prepareWorkbenchLogData(documents[index].Data)
		if err := validateWorkbenchLogData(documents[index].Data); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(documents, func(i int, j int) bool {
		if documents[i].Data.CreatedAtMillis != documents[j].Data.CreatedAtMillis {
			return documents[i].Data.CreatedAtMillis > documents[j].Data.CreatedAtMillis
		}
		return documents[i].UpdatedAt > documents[j].UpdatedAt
	})
	return documents, nil
}

func ListWorkbenchLogSummaries(workspace Workspace, modality string) ([]WorkbenchLogSummary, error) {
	var summaries []WorkbenchLogSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListWorkbenchLogSummaries(modality)
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func DeleteWorkbenchLog(workspace Workspace, id string) error {
	repo := WorkbenchLogRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeleteWorkbenchLog(id)
		})
	})
}

func writeWorkbenchLogUnlocked(workspace Workspace, document Envelope[WorkbenchLogData]) error {
	if err := WorkbenchLogRepository(workspace).Write(document); err != nil {
		return err
	}
	return withIndex(workspace, func(index *WorkspaceIndex) error {
		return index.UpsertWorkbenchLog(document)
	})
}

func WorkbenchLogDocumentSummary(document Envelope[WorkbenchLogData]) WorkbenchLogSummary {
	return WorkbenchLogSummary{
		ID:              document.ID,
		Modality:        document.Data.Modality,
		Title:           document.Data.Title,
		Status:          document.Data.Status,
		Model:           document.Data.Model,
		CreatedAtMillis: document.Data.CreatedAtMillis,
		MediaCount:      len(document.Data.Media),
		Revision:        document.Revision,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}
}

func prepareWorkbenchLogData(data WorkbenchLogData) WorkbenchLogData {
	data.Modality = strings.TrimSpace(data.Modality)
	data.Status = strings.TrimSpace(data.Status)
	if data.Status == "" {
		data.Status = WorkbenchLogStatusSuccess
	}
	data.Title = strings.TrimSpace(data.Title)
	data.Model = strings.TrimSpace(data.Model)
	data.Prompt = strings.TrimSpace(data.Prompt)
	if data.Media == nil {
		data.Media = []WorkbenchLogMedia{}
	}
	return data
}

func validateWorkbenchLogData(data WorkbenchLogData) error {
	switch data.Modality {
	case WorkbenchModalityText, WorkbenchModalityImage, WorkbenchModalityVideo:
	default:
		return NewError(ErrorInvalidArgument, "workbench log modality is not allowed", 1, map[string]string{"modality": data.Modality})
	}
	switch data.Status {
	case WorkbenchLogStatusSuccess, WorkbenchLogStatusError:
	default:
		return NewError(ErrorInvalidArgument, "workbench log status is not allowed", 1, map[string]string{"status": data.Status})
	}
	seen := map[string]bool{}
	for _, media := range data.Media {
		if strings.TrimSpace(media.Key) == "" {
			return NewError(ErrorInvalidArgument, "workbench log media key is empty", 1, nil)
		}
		if err := validatePathComponent("workbench log media key", media.Key); err != nil {
			return err
		}
		if seen[media.Key] {
			return NewError(ErrorInvalidArgument, "workbench log media key is duplicated", 1, map[string]string{"key": media.Key})
		}
		seen[media.Key] = true
		if strings.TrimSpace(media.Path) != "" {
			if !isWorkspaceRelativeFile(media.Path) || !strings.HasPrefix(path.Clean(media.Path), "files/") {
				return NewError(ErrorWorkspaceInvalid, "workbench log media path escapes files directory", 2, map[string]string{"key": media.Key})
			}
		}
		if media.Width < 0 || media.Height < 0 || media.Bytes < 0 || media.DurationMs < 0 {
			return NewError(ErrorInvalidArgument, "workbench log media numeric fields must not be negative", 1, map[string]string{"key": media.Key})
		}
	}
	return validateNoPlaintextSecrets(data, "workbench_log")
}

func upsertWorkbenchLogMedia(data WorkbenchLogData, file WorkbenchLogImportFile, relPath string) WorkbenchLogData {
	for index := range data.Media {
		if data.Media[index].Key != file.Key {
			continue
		}
		data.Media[index].Path = relPath
		if data.Media[index].MIME == "" {
			data.Media[index].MIME = file.ContentType
		}
		if file.Header != nil {
			data.Media[index].Name = filepath.Base(file.Header.Filename)
			if data.Media[index].Bytes <= 0 {
				data.Media[index].Bytes = file.Header.Size
			}
		}
		return data
	}
	media := WorkbenchLogMedia{
		Key:  file.Key,
		MIME: file.ContentType,
		Path: relPath,
	}
	if file.Header != nil {
		media.Name = filepath.Base(file.Header.Filename)
		media.Bytes = file.Header.Size
	}
	data.Media = append(data.Media, media)
	return data
}

func workbenchLogMediaByKey(items []WorkbenchLogMedia, key string) (WorkbenchLogMedia, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return WorkbenchLogMedia{}, false
}

func workbenchLogUploadRelPath(fileKey string, contentType string) (string, error) {
	if err := validatePathComponent("workbench log file key", fileKey); err != nil {
		return "", err
	}
	return path.Join("files", fileKey+extensionForContentType(contentType)), nil
}

func closeWorkbenchLogFiles(files []WorkbenchLogImportFile) {
	for _, file := range files {
		if file.File != nil {
			_ = file.File.Close()
		}
	}
}

func readWorkbenchLogImportFile(key string, header *multipart.FileHeader) (WorkbenchLogImportFile, error) {
	file, err := header.Open()
	if err != nil {
		return WorkbenchLogImportFile{}, WrapError(ErrorInvalidArgument, "open workbench log file", 1, err)
	}
	contentType, err := sniffMultipartFile(file, header)
	if err != nil {
		_ = file.Close()
		return WorkbenchLogImportFile{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return WorkbenchLogImportFile{}, WrapError(ErrorInvalidArgument, "rewind workbench log file", 1, err)
	}
	return WorkbenchLogImportFile{Key: key, File: file, Header: header, ContentType: contentType}, nil
}
