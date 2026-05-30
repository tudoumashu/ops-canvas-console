package localworkspace

import (
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	CanvasBackgroundDots  = "dots"
	CanvasBackgroundLines = "lines"
	CanvasBackgroundBlank = "blank"
)

type CanvasProjectData struct {
	Title          string                       `json:"title"`
	Nodes          []CanvasProjectNodeData      `json:"nodes"`
	Connections    []CanvasProjectConnection    `json:"connections"`
	ChatSessions   []CanvasProjectChatSession   `json:"chatSessions"`
	ActiveChatID   *string                      `json:"activeChatId"`
	BackgroundMode string                       `json:"backgroundMode"`
	ShowImageInfo  bool                         `json:"showImageInfo"`
	Viewport       CanvasProjectViewport        `json:"viewport"`
	Files          map[string]CanvasProjectFile `json:"files,omitempty"`
	Metadata       map[string]any               `json:"metadata,omitempty"`
}

type CanvasProjectFile struct {
	Role     string         `json:"role,omitempty"`
	NodeID   string         `json:"nodeId,omitempty"`
	MIME     string         `json:"mime,omitempty"`
	Path     string         `json:"path,omitempty"`
	Width    int            `json:"width,omitempty"`
	Height   int            `json:"height,omitempty"`
	Bytes    int64          `json:"bytes,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type CanvasProjectNodeData struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Position CanvasPosition `json:"position"`
	Width    float64        `json:"width"`
	Height   float64        `json:"height"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type CanvasPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type CanvasProjectConnection struct {
	ID         string `json:"id"`
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
}

type CanvasProjectChatSession struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Messages  []json.RawMessage `json:"messages"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
}

type CanvasProjectViewport struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	K float64 `json:"k"`
}

type CanvasProjectSummary struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	NodeCount       int    `json:"nodeCount"`
	ConnectionCount int    `json:"connectionCount"`
	FileCount       int    `json:"fileCount"`
	Revision        int    `json:"revision"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type CanvasProjectImportFile struct {
	Key         string
	File        multipart.File
	Header      *multipart.FileHeader
	ContentType string
}

func CanvasProjectRepository(workspace Workspace) Repository[CanvasProjectData] {
	return Repository[CanvasProjectData]{
		Workspace:  workspace,
		Collection: "canvas-projects",
		FileName:   "canvas-project.json",
		Kind:       KindCanvasProject,
		IDPrefix:   "canvas",
	}
}

func NewCanvasProject(workspace Workspace, data CanvasProjectData) (Envelope[CanvasProjectData], error) {
	prepared, err := prepareCanvasProjectData(data)
	if err != nil {
		return Envelope[CanvasProjectData]{}, err
	}
	return CanvasProjectRepository(workspace).New(prepared)
}

func WriteCanvasProject(workspace Workspace, document Envelope[CanvasProjectData]) error {
	data, err := prepareCanvasProjectData(document.Data)
	if err != nil {
		return err
	}
	document.Data = data
	if err := CanvasProjectRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		return writeCanvasProjectUnlocked(workspace, document)
	})
}

func SaveCanvasProject(workspace Workspace, document Envelope[CanvasProjectData], files []CanvasProjectImportFile) error {
	data, err := prepareCanvasProjectData(document.Data)
	if err != nil {
		return err
	}
	document.Data = data
	for _, file := range files {
		if err := validatePathComponent("canvas project file key", file.Key); err != nil {
			return err
		}
		relPath, err := canvasProjectUploadRelPath(file.Key, file.ContentType)
		if err != nil {
			return err
		}
		document.Data = upsertCanvasProjectFile(document.Data, file, relPath)
	}
	if err := validateCanvasProjectData(document.Data); err != nil {
		return err
	}
	if err := CanvasProjectRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	if err := withWorkspaceLock(workspace, func() error {
		for _, file := range files {
			meta, ok := document.Data.Files[file.Key]
			if !ok {
				return NewError(ErrorInvalidArgument, "canvas project file key is not referenced", 1, map[string]string{"fileKey": file.Key})
			}
			filePath := filepath.Join(CanvasProjectRepository(workspace).Dir(document.ID), filepath.FromSlash(meta.Path))
			if err := AtomicWriteFromReader(filePath, file.File, 0o600); err != nil {
				return err
			}
		}
		return writeCanvasProjectUnlocked(workspace, document)
	}); err != nil {
		if document.Revision == 1 {
			_ = os.RemoveAll(CanvasProjectRepository(workspace).Dir(document.ID))
		}
		return err
	}
	return nil
}

func ReadCanvasProject(workspace Workspace, id string) (Envelope[CanvasProjectData], error) {
	document, err := CanvasProjectRepository(workspace).Read(id)
	if err != nil {
		return Envelope[CanvasProjectData]{}, err
	}
	data, err := prepareCanvasProjectData(document.Data)
	if err != nil {
		return Envelope[CanvasProjectData]{}, err
	}
	document.Data = data
	return document, nil
}

func ListCanvasProjects(workspace Workspace) ([]Envelope[CanvasProjectData], error) {
	documents, err := CanvasProjectRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for index, document := range documents {
		data, err := prepareCanvasProjectData(document.Data)
		if err != nil {
			return nil, err
		}
		documents[index].Data = data
	}
	return documents, nil
}

func ListCanvasProjectSummaries(workspace Workspace) ([]CanvasProjectSummary, error) {
	var summaries []CanvasProjectSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListCanvasProjectSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func DeleteCanvasProject(workspace Workspace, id string) error {
	repo := CanvasProjectRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeleteCanvasProject(id)
		})
	})
}

func CanvasProjectDocumentSummary(document Envelope[CanvasProjectData]) CanvasProjectSummary {
	return CanvasProjectSummary{
		ID:              document.ID,
		Title:           document.Data.Title,
		NodeCount:       len(document.Data.Nodes),
		ConnectionCount: len(document.Data.Connections),
		FileCount:       len(document.Data.Files),
		Revision:        document.Revision,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}
}

func prepareCanvasProjectData(data CanvasProjectData) (CanvasProjectData, error) {
	if strings.TrimSpace(data.Title) == "" {
		return CanvasProjectData{}, NewError(ErrorWorkspaceInvalid, "canvas project title is empty", 2, nil)
	}
	if strings.TrimSpace(data.BackgroundMode) == "" {
		data.BackgroundMode = CanvasBackgroundLines
	}
	if data.Viewport.K == 0 {
		data.Viewport.K = 1
	}
	if data.Nodes == nil {
		data.Nodes = []CanvasProjectNodeData{}
	}
	if data.Connections == nil {
		data.Connections = []CanvasProjectConnection{}
	}
	if data.ChatSessions == nil {
		data.ChatSessions = []CanvasProjectChatSession{}
	}
	if err := validateCanvasProjectData(data); err != nil {
		return CanvasProjectData{}, err
	}
	return data, nil
}

func validateCanvasProjectData(data CanvasProjectData) error {
	switch data.BackgroundMode {
	case CanvasBackgroundDots, CanvasBackgroundLines, CanvasBackgroundBlank:
	default:
		return NewError(ErrorWorkspaceInvalid, "canvas project backgroundMode is not allowed", 2, map[string]string{"backgroundMode": data.BackgroundMode})
	}
	if !finiteCanvasNumber(data.Viewport.X) || !finiteCanvasNumber(data.Viewport.Y) || !finiteCanvasNumber(data.Viewport.K) || data.Viewport.K <= 0 {
		return NewError(ErrorWorkspaceInvalid, "canvas project viewport is invalid", 2, nil)
	}
	nodeIDs := make(map[string]bool, len(data.Nodes))
	for _, node := range data.Nodes {
		if err := validatePathComponent("canvas node id", node.ID); err != nil {
			return err
		}
		if nodeIDs[node.ID] {
			return NewError(ErrorWorkspaceInvalid, "canvas node id is duplicated", 2, map[string]string{"id": node.ID})
		}
		nodeIDs[node.ID] = true
		switch node.Type {
		case "image", "text", "config", "video":
		default:
			return NewError(ErrorWorkspaceInvalid, "canvas node type is not allowed", 2, map[string]string{"type": node.Type})
		}
		if strings.TrimSpace(node.Title) == "" {
			return NewError(ErrorWorkspaceInvalid, "canvas node title is empty", 2, map[string]string{"id": node.ID})
		}
		if !finiteCanvasNumber(node.Position.X) || !finiteCanvasNumber(node.Position.Y) || !finiteCanvasNumber(node.Width) || !finiteCanvasNumber(node.Height) || node.Width <= 0 || node.Height <= 0 {
			return NewError(ErrorWorkspaceInvalid, "canvas node geometry is invalid", 2, map[string]string{"id": node.ID})
		}
	}
	connectionIDs := make(map[string]bool, len(data.Connections))
	for _, connection := range data.Connections {
		if err := validatePathComponent("canvas connection id", connection.ID); err != nil {
			return err
		}
		if connectionIDs[connection.ID] {
			return NewError(ErrorWorkspaceInvalid, "canvas connection id is duplicated", 2, map[string]string{"id": connection.ID})
		}
		connectionIDs[connection.ID] = true
		if err := validatePathComponent("canvas connection fromNodeId", connection.FromNodeID); err != nil {
			return err
		}
		if err := validatePathComponent("canvas connection toNodeId", connection.ToNodeID); err != nil {
			return err
		}
		if !nodeIDs[connection.FromNodeID] || !nodeIDs[connection.ToNodeID] {
			return NewError(ErrorWorkspaceInvalid, "canvas connection references missing node", 2, map[string]string{"id": connection.ID})
		}
	}
	chatSessionIDs := make(map[string]bool, len(data.ChatSessions))
	for _, session := range data.ChatSessions {
		if err := validatePathComponent("canvas chat session id", session.ID); err != nil {
			return err
		}
		if chatSessionIDs[session.ID] {
			return NewError(ErrorWorkspaceInvalid, "canvas chat session id is duplicated", 2, map[string]string{"id": session.ID})
		}
		chatSessionIDs[session.ID] = true
	}
	if err := validateCanvasProjectFiles(data.Files); err != nil {
		return err
	}
	return validateNoPlaintextSecrets(data, "canvas_project")
}

func validateCanvasProjectFiles(files map[string]CanvasProjectFile) error {
	for key, item := range files {
		if err := validatePathComponent("canvas project file key", key); err != nil {
			return err
		}
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		if !isWorkspaceRelativeFile(item.Path) || !strings.HasPrefix(path.Clean(item.Path), "files/") {
			return NewError(ErrorWorkspaceInvalid, "canvas project file path escapes files directory", 2, map[string]string{"file": key})
		}
		if item.NodeID != "" {
			if err := validatePathComponent("canvas project file nodeId", item.NodeID); err != nil {
				return err
			}
		}
		if item.Width < 0 || item.Height < 0 || item.Bytes < 0 {
			return NewError(ErrorInvalidArgument, "canvas project file numeric fields must not be negative", 1, map[string]string{"file": key})
		}
	}
	return nil
}

func writeCanvasProjectUnlocked(workspace Workspace, document Envelope[CanvasProjectData]) error {
	if err := CanvasProjectRepository(workspace).Write(document); err != nil {
		return err
	}
	return withIndex(workspace, func(index *WorkspaceIndex) error {
		return index.UpsertCanvasProject(document)
	})
}

func upsertCanvasProjectFile(data CanvasProjectData, file CanvasProjectImportFile, relPath string) CanvasProjectData {
	if data.Files == nil {
		data.Files = map[string]CanvasProjectFile{}
	}
	meta := data.Files[file.Key]
	meta.Path = relPath
	if meta.MIME == "" {
		meta.MIME = file.ContentType
	}
	if file.Header != nil {
		if meta.Bytes <= 0 {
			meta.Bytes = file.Header.Size
		}
		if meta.Metadata == nil {
			meta.Metadata = map[string]any{}
		}
		meta.Metadata["fileName"] = filepath.Base(file.Header.Filename)
	}
	data.Files[file.Key] = meta
	return data
}

func canvasProjectUploadRelPath(fileKey string, contentType string) (string, error) {
	if err := validatePathComponent("canvas project file key", fileKey); err != nil {
		return "", err
	}
	return path.Join("files", fileKey+extensionForContentType(contentType)), nil
}

func closeCanvasProjectFiles(files []CanvasProjectImportFile) {
	for _, file := range files {
		if file.File != nil {
			_ = file.File.Close()
		}
	}
}

func readCanvasProjectImportFile(key string, header *multipart.FileHeader) (CanvasProjectImportFile, error) {
	file, err := header.Open()
	if err != nil {
		return CanvasProjectImportFile{}, WrapError(ErrorInvalidArgument, "open canvas project file", 1, err)
	}
	contentType, err := sniffMultipartFile(file, header)
	if err != nil {
		_ = file.Close()
		return CanvasProjectImportFile{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return CanvasProjectImportFile{}, WrapError(ErrorInvalidArgument, "rewind canvas project file", 1, err)
	}
	return CanvasProjectImportFile{Key: key, File: file, Header: header, ContentType: contentType}, nil
}

func finiteCanvasNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
