package localworkspace

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ProfileModeLocal  = "local"
	ProfileModeCloud  = "cloud"
	ProfileModeHybrid = "hybrid"

	PrivacyPrivate = "private"
	PrivacyShared  = "shared"
	PrivacyPublic  = "public"
)

type ProfileData struct {
	Name     string           `json:"name"`
	Mode     string           `json:"mode,omitempty"`
	Channels []ProfileChannel `json:"channels,omitempty"`
	Metadata map[string]any   `json:"metadata,omitempty"`
}

type ProfileChannel struct {
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	Protocol  string         `json:"protocol,omitempty"`
	BaseURL   string         `json:"baseUrl,omitempty"`
	Models    []string       `json:"models,omitempty"`
	Weight    int            `json:"weight,omitempty"`
	Enabled   bool           `json:"enabled,omitempty"`
	SecretRef *SecretRef     `json:"secretRef,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ProjectCapabilities struct {
	FSRead        bool `json:"fs.read,omitempty"`
	FSWrite       bool `json:"fs.write,omitempty"`
	ProcessExec   bool `json:"process.exec,omitempty"`
	NetworkLocal  bool `json:"network.local,omitempty"`
	ArtifactWrite bool `json:"artifact.write,omitempty"`
}

type ProjectExecution struct {
	AllowGlobs []string       `json:"allowGlobs,omitempty"`
	DenyGlobs  []string       `json:"denyGlobs,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ProjectPathOperation string

const (
	ProjectPathRead  ProjectPathOperation = "read"
	ProjectPathWrite ProjectPathOperation = "write"
	ProjectPathExec  ProjectPathOperation = "exec"
)

var defaultProjectDenyGlobs = []string{
	"**/.env",
	"**/.env.*",
	"**/.git/**",
	"**/node_modules/**",
}

type ProjectData struct {
	Name            string               `json:"name"`
	Kind            string               `json:"kind,omitempty"`
	Adapter         string               `json:"adapter,omitempty"`
	RootPath        string               `json:"rootPath,omitempty"`
	RootFingerprint string               `json:"rootFingerprint,omitempty"`
	Capabilities    ProjectCapabilities  `json:"capabilities,omitempty"`
	Execution       ProjectExecution     `json:"execution,omitempty"`
	AdapterMetadata map[string]any       `json:"adapterMetadata,omitempty"`
	CredentialRefs  map[string]SecretRef `json:"credentialRefs,omitempty"`
	Metadata        map[string]any       `json:"metadata,omitempty"`
}

type AssetData struct {
	Type             string            `json:"type"`
	MIME             string            `json:"mime,omitempty"`
	Title            string            `json:"title,omitempty"`
	MediaType        string            `json:"mediaType,omitempty"`
	Category         string            `json:"category,omitempty"`
	CategoryPath     string            `json:"categoryPath,omitempty"`
	Purpose          string            `json:"purpose,omitempty"`
	Source           string            `json:"source,omitempty"`
	CoverURL         string            `json:"coverUrl,omitempty"`
	Description      string            `json:"description,omitempty"`
	Content          string            `json:"content,omitempty"`
	SourceArtifactID string            `json:"sourceArtifactId,omitempty"`
	Privacy          string            `json:"privacy,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Files            map[string]string `json:"files,omitempty"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
}

type PromptData struct {
	Title      string         `json:"title"`
	CoverURL   string         `json:"coverUrl,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Privacy    string         `json:"privacy,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Category   string         `json:"category,omitempty"`
	Domain     string         `json:"domain,omitempty"`
	Stage      string         `json:"stage,omitempty"`
	Provider   string         `json:"provider,omitempty"`
	Model      string         `json:"model,omitempty"`
	Mode       string         `json:"mode,omitempty"`
	InputType  string         `json:"inputType,omitempty"`
	OutputType string         `json:"outputType,omitempty"`
	Status     string         `json:"status,omitempty"`
	Preview    string         `json:"preview,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ProfileSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Mode         string `json:"mode,omitempty"`
	ChannelCount int    `json:"channelCount"`
	Revision     int    `json:"revision"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type ProjectSummary struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Kind            string              `json:"kind,omitempty"`
	Adapter         string              `json:"adapter,omitempty"`
	HasRootPath     bool                `json:"hasRootPath"`
	RootFingerprint string              `json:"rootFingerprint,omitempty"`
	Capabilities    ProjectCapabilities `json:"capabilities,omitempty"`
	Revision        int                 `json:"revision"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
}

type AssetSummary struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	MIME             string   `json:"mime,omitempty"`
	Title            string   `json:"title,omitempty"`
	MediaType        string   `json:"mediaType,omitempty"`
	CategoryPath     string   `json:"categoryPath,omitempty"`
	Purpose          string   `json:"purpose,omitempty"`
	Source           string   `json:"source,omitempty"`
	SourceArtifactID string   `json:"sourceArtifactId,omitempty"`
	Privacy          string   `json:"privacy,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Original         string   `json:"original,omitempty"`
	Thumbnail        string   `json:"thumbnail,omitempty"`
	Revision         int      `json:"revision"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

type PromptSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind,omitempty"`
	Category   string   `json:"category,omitempty"`
	Domain     string   `json:"domain,omitempty"`
	Stage      string   `json:"stage,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	Model      string   `json:"model,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	InputType  string   `json:"inputType,omitempty"`
	OutputType string   `json:"outputType,omitempty"`
	Status     string   `json:"status,omitempty"`
	Privacy    string   `json:"privacy,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	HasContent bool     `json:"hasContent"`
	Revision   int      `json:"revision"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
}

func ProfileRepository(workspace Workspace) Repository[ProfileData] {
	return Repository[ProfileData]{
		Workspace:  workspace,
		Collection: "profiles",
		FileName:   "profile.json",
		Kind:       KindProfile,
		IDPrefix:   "profile",
	}
}

func ProjectRepository(workspace Workspace) Repository[ProjectData] {
	return Repository[ProjectData]{
		Workspace:  workspace,
		Collection: "projects",
		FileName:   "project.json",
		Kind:       KindProject,
		IDPrefix:   "proj",
	}
}

func AssetRepository(workspace Workspace) Repository[AssetData] {
	return Repository[AssetData]{
		Workspace:  workspace,
		Collection: "assets",
		FileName:   "asset.json",
		Kind:       KindAsset,
		IDPrefix:   "asset",
	}
}

func PromptRepository(workspace Workspace) Repository[PromptData] {
	return Repository[PromptData]{
		Workspace:  workspace,
		Collection: "prompts",
		FileName:   "prompt.json",
		Kind:       KindPrompt,
		IDPrefix:   "prompt",
	}
}

func NewProfile(workspace Workspace, data ProfileData) (Envelope[ProfileData], error) {
	if strings.TrimSpace(data.Mode) == "" {
		data.Mode = ProfileModeLocal
	}
	if err := validateProfileData(data); err != nil {
		return Envelope[ProfileData]{}, err
	}
	return ProfileRepository(workspace).New(data)
}

func WriteProfile(workspace Workspace, document Envelope[ProfileData]) error {
	if err := validateProfileData(document.Data); err != nil {
		return err
	}
	if err := ProfileRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := ProfileRepository(workspace).Write(document); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertProfile(document)
		})
	})
}

func SaveProfile(workspace Workspace, document Envelope[ProfileData]) error {
	return WriteProfile(workspace, document)
}

func ReadProfile(workspace Workspace, id string) (Envelope[ProfileData], error) {
	document, err := ProfileRepository(workspace).Read(id)
	if err != nil {
		return Envelope[ProfileData]{}, err
	}
	if err := validateProfileData(document.Data); err != nil {
		return Envelope[ProfileData]{}, err
	}
	return document, nil
}

func ListProfiles(workspace Workspace) ([]Envelope[ProfileData], error) {
	documents, err := ProfileRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if err := validateProfileData(document.Data); err != nil {
			return nil, err
		}
	}
	return documents, nil
}

func ListProfileSummaries(workspace Workspace) ([]ProfileSummary, error) {
	var summaries []ProfileSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListProfileSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func NewProject(workspace Workspace, data ProjectData) (Envelope[ProjectData], error) {
	data, err := prepareProjectData(workspace, data)
	if err != nil {
		return Envelope[ProjectData]{}, err
	}
	if err := validateProjectData(data); err != nil {
		return Envelope[ProjectData]{}, err
	}
	return ProjectRepository(workspace).New(data)
}

func WriteProject(workspace Workspace, document Envelope[ProjectData]) error {
	data, err := prepareProjectData(workspace, document.Data)
	if err != nil {
		return err
	}
	document.Data = data
	if err := validateProjectData(document.Data); err != nil {
		return err
	}
	if err := ProjectRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := ProjectRepository(workspace).Write(document); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertProject(document)
		})
	})
}

func SaveProject(workspace Workspace, document Envelope[ProjectData]) error {
	return WriteProject(workspace, document)
}

func ReadProject(workspace Workspace, id string) (Envelope[ProjectData], error) {
	document, err := ProjectRepository(workspace).Read(id)
	if err != nil {
		return Envelope[ProjectData]{}, err
	}
	if err := validateProjectData(document.Data); err != nil {
		return Envelope[ProjectData]{}, err
	}
	return document, nil
}

func ListProjects(workspace Workspace) ([]Envelope[ProjectData], error) {
	documents, err := ProjectRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if err := validateProjectData(document.Data); err != nil {
			return nil, err
		}
	}
	return documents, nil
}

func ListProjectSummaries(workspace Workspace) ([]ProjectSummary, error) {
	var summaries []ProjectSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListProjectSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func NewAsset(workspace Workspace, data AssetData) (Envelope[AssetData], error) {
	if strings.TrimSpace(data.Privacy) == "" {
		data.Privacy = PrivacyPrivate
	}
	if err := validateAssetData(data); err != nil {
		return Envelope[AssetData]{}, err
	}
	return AssetRepository(workspace).New(data)
}

func WriteAsset(workspace Workspace, document Envelope[AssetData]) error {
	if err := validateAssetData(document.Data); err != nil {
		return err
	}
	if err := AssetRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if strings.TrimSpace(document.Data.SourceArtifactID) != "" {
			if _, err := ReadArtifact(workspace, document.Data.SourceArtifactID); err != nil {
				return err
			}
		}
		if err := AssetRepository(workspace).Write(document); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertAsset(document)
		})
	})
}

func SaveAsset(workspace Workspace, document Envelope[AssetData]) error {
	return WriteAsset(workspace, document)
}

func ReadAsset(workspace Workspace, id string) (Envelope[AssetData], error) {
	document, err := AssetRepository(workspace).Read(id)
	if err != nil {
		return Envelope[AssetData]{}, err
	}
	if err := validateAssetData(document.Data); err != nil {
		return Envelope[AssetData]{}, err
	}
	return document, nil
}

func ListAssets(workspace Workspace) ([]Envelope[AssetData], error) {
	documents, err := AssetRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if err := validateAssetData(document.Data); err != nil {
			return nil, err
		}
	}
	return documents, nil
}

func ListAssetSummaries(workspace Workspace) ([]AssetSummary, error) {
	var summaries []AssetSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListAssetSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func NewPrompt(workspace Workspace, data PromptData) (Envelope[PromptData], error) {
	if strings.TrimSpace(data.Privacy) == "" {
		data.Privacy = PrivacyPrivate
	}
	if err := validatePromptData(data); err != nil {
		return Envelope[PromptData]{}, err
	}
	return PromptRepository(workspace).New(data)
}

func WritePrompt(workspace Workspace, document Envelope[PromptData]) error {
	if err := validatePromptData(document.Data); err != nil {
		return err
	}
	if err := PromptRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := PromptRepository(workspace).Write(document); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertPrompt(workspace, document)
		})
	})
}

func SavePrompt(workspace Workspace, document Envelope[PromptData], content string) error {
	if err := validatePromptData(document.Data); err != nil {
		return err
	}
	if err := PromptRepository(workspace).validateDocument(document); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := PromptRepository(workspace).Write(document); err != nil {
			return err
		}
		if err := AtomicWriteFile(promptContentPath(workspace, document.ID), []byte(content), 0o600); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.UpsertPrompt(workspace, document)
		})
	})
}

func ReadPrompt(workspace Workspace, id string) (Envelope[PromptData], error) {
	document, err := PromptRepository(workspace).Read(id)
	if err != nil {
		return Envelope[PromptData]{}, err
	}
	if err := validatePromptData(document.Data); err != nil {
		return Envelope[PromptData]{}, err
	}
	return document, nil
}

func ReadPromptContent(workspace Workspace, id string) (string, error) {
	if err := PromptRepository(workspace).validateID(id); err != nil {
		return "", err
	}
	data, err := os.ReadFile(promptContentPath(workspace, id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", NewError(ErrorWorkspaceNotFound, "prompt content not found", 2, map[string]string{"id": id})
		}
		return "", WrapError(ErrorInternal, "read prompt content", 5, err)
	}
	return string(data), nil
}

func ListPrompts(workspace Workspace) ([]Envelope[PromptData], error) {
	documents, err := PromptRepository(workspace).List()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if err := validatePromptData(document.Data); err != nil {
			return nil, err
		}
	}
	return documents, nil
}

func ListPromptSummaries(workspace Workspace) ([]PromptSummary, error) {
	var summaries []PromptSummary
	err := withIndex(workspace, func(index *WorkspaceIndex) error {
		items, err := index.ListPromptSummaries()
		if err != nil {
			return err
		}
		summaries = items
		return nil
	})
	return summaries, err
}

func ProfileDocumentSummary(document Envelope[ProfileData]) ProfileSummary {
	return ProfileSummary{
		ID:           document.ID,
		Name:         document.Data.Name,
		Mode:         document.Data.Mode,
		ChannelCount: len(document.Data.Channels),
		Revision:     document.Revision,
		CreatedAt:    document.CreatedAt,
		UpdatedAt:    document.UpdatedAt,
	}
}

func ProjectDocumentSummary(document Envelope[ProjectData]) ProjectSummary {
	return ProjectSummary{
		ID:              document.ID,
		Name:            document.Data.Name,
		Kind:            document.Data.Kind,
		Adapter:         document.Data.Adapter,
		HasRootPath:     strings.TrimSpace(document.Data.RootPath) != "",
		RootFingerprint: document.Data.RootFingerprint,
		Capabilities:    document.Data.Capabilities,
		Revision:        document.Revision,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}
}

func AssetDocumentSummary(document Envelope[AssetData]) AssetSummary {
	return AssetSummary{
		ID:               document.ID,
		Type:             document.Data.Type,
		MIME:             document.Data.MIME,
		Title:            document.Data.Title,
		MediaType:        document.Data.MediaType,
		CategoryPath:     document.Data.CategoryPath,
		Purpose:          document.Data.Purpose,
		Source:           document.Data.Source,
		SourceArtifactID: document.Data.SourceArtifactID,
		Privacy:          document.Data.Privacy,
		Tags:             append([]string{}, document.Data.Tags...),
		Original:         document.Data.Files["original"],
		Thumbnail:        document.Data.Files["thumbnail"],
		Revision:         document.Revision,
		CreatedAt:        document.CreatedAt,
		UpdatedAt:        document.UpdatedAt,
	}
}

func PromptDocumentSummary(workspace Workspace, document Envelope[PromptData]) PromptSummary {
	return PromptSummary{
		ID:         document.ID,
		Title:      document.Data.Title,
		Kind:       document.Data.Kind,
		Category:   document.Data.Category,
		Domain:     document.Data.Domain,
		Stage:      document.Data.Stage,
		Provider:   document.Data.Provider,
		Model:      document.Data.Model,
		Mode:       document.Data.Mode,
		InputType:  document.Data.InputType,
		OutputType: document.Data.OutputType,
		Status:     document.Data.Status,
		Privacy:    document.Data.Privacy,
		Tags:       append([]string{}, document.Data.Tags...),
		HasContent: promptContentExists(workspace, document.ID),
		Revision:   document.Revision,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}

func promptContentPath(workspace Workspace, id string) string {
	return filepath.Join(workspace.Root, "prompts", id, "content.md")
}

func promptContentExists(workspace Workspace, id string) bool {
	stat, err := os.Stat(promptContentPath(workspace, id))
	return err == nil && !stat.IsDir()
}

func validateProfileData(data ProfileData) error {
	if strings.TrimSpace(data.Name) == "" {
		return NewError(ErrorWorkspaceInvalid, "profile name is empty", 2, nil)
	}
	switch strings.TrimSpace(data.Mode) {
	case ProfileModeLocal, ProfileModeCloud, ProfileModeHybrid:
	default:
		return NewError(ErrorWorkspaceInvalid, "profile mode is not allowed", 2, map[string]string{"mode": data.Mode})
	}
	for _, channel := range data.Channels {
		if strings.TrimSpace(channel.ID) == "" {
			return NewError(ErrorWorkspaceInvalid, "profile channel id is empty", 2, nil)
		}
		if err := validatePathComponent("profile channel id", channel.ID); err != nil {
			return err
		}
		if channel.SecretRef != nil {
			if err := channel.SecretRef.Validate(); err != nil {
				return err
			}
		}
	}
	return validateNoPlaintextSecrets(data, "profile")
}

func prepareProjectData(workspace Workspace, data ProjectData) (ProjectData, error) {
	if strings.TrimSpace(data.Adapter) == "" {
		data.Adapter = "filesystem"
	}
	data.Execution.DenyGlobs = mergeDefaultProjectDenyGlobs(data.Execution.DenyGlobs)
	if strings.TrimSpace(data.RootPath) != "" {
		fingerprint, err := ProjectRootFingerprint(workspace, data.RootPath)
		if err != nil {
			return ProjectData{}, err
		}
		data.RootFingerprint = fingerprint
	}
	return data, nil
}

func validateProjectData(data ProjectData) error {
	if strings.TrimSpace(data.Name) == "" {
		return NewError(ErrorWorkspaceInvalid, "project name is empty", 2, nil)
	}
	if strings.TrimSpace(data.RootPath) != "" && !filepath.IsAbs(data.RootPath) {
		return NewError(ErrorWorkspaceInvalid, "project rootPath must be absolute", 2, nil)
	}
	for _, glob := range append(append([]string{}, data.Execution.AllowGlobs...), data.Execution.DenyGlobs...) {
		if strings.TrimSpace(glob) == "" {
			continue
		}
		normalized := strings.ReplaceAll(glob, "\\", "/")
		if strings.HasPrefix(normalized, "/") || normalized == ".." || strings.Contains(normalized, "../") || strings.Contains(normalized, "/..") {
			return NewError(ErrorWorkspaceInvalid, "project execution glob must stay project relative", 2, map[string]string{"glob": glob})
		}
	}
	for name, ref := range data.CredentialRefs {
		if err := validatePathComponent("project credential ref name", name); err != nil {
			return err
		}
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return validateNoPlaintextSecrets(data, "project")
}

func validateAssetData(data AssetData) error {
	if err := validateArtifactType(data.Type); err != nil {
		return err
	}
	if err := validatePrivacy(data.Privacy, "asset privacy"); err != nil {
		return err
	}
	if strings.TrimSpace(data.SourceArtifactID) != "" {
		if err := validateScannedID(data.SourceArtifactID, "art"); err != nil {
			return err
		}
	}
	for name, filePath := range data.Files {
		if strings.TrimSpace(filePath) == "" {
			continue
		}
		if !isWorkspaceRelativeFile(filePath) {
			return NewError(ErrorWorkspaceInvalid, "asset file path must stay inside asset directory", 2, map[string]string{"file": name})
		}
	}
	return validateNoPlaintextSecrets(data, "asset")
}

func validatePromptData(data PromptData) error {
	if strings.TrimSpace(data.Title) == "" {
		return NewError(ErrorWorkspaceInvalid, "prompt title is empty", 2, nil)
	}
	if err := validatePrivacy(data.Privacy, "prompt privacy"); err != nil {
		return err
	}
	return validateNoPlaintextSecrets(data, "prompt")
}

func validatePrivacy(value string, label string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	switch value {
	case PrivacyPrivate, PrivacyShared, PrivacyPublic:
		return nil
	default:
		return NewError(ErrorWorkspaceInvalid, label+" is not allowed", 2, map[string]string{"privacy": value})
	}
}

func validateNoPlaintextSecrets(value any, scope string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return WrapError(ErrorInternal, "encode object for secret inspection", 5, err)
	}
	return validateRawNoPlaintextSecrets(data, scope)
}

func validateRawNoPlaintextSecrets(raw json.RawMessage, scope string) error {
	if len(raw) == 0 {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := object[key]
			if key == "secretRef" {
				var ref SecretRef
				if err := json.Unmarshal(value, &ref); err != nil {
					return NewError(ErrorWorkspaceInvalid, "secretRef is not valid json", 2, map[string]string{"scope": scope})
				}
				if err := ref.Validate(); err != nil {
					return err
				}
				continue
			}
			if isPlaintextSecretKey(key) {
				return NewError(ErrorWorkspaceInvalid, "plaintext secret field is not allowed", 2, map[string]string{"scope": scope + "." + key})
			}
			if err := validateRawNoPlaintextSecrets(value, scope+"."+key); err != nil {
				return err
			}
		}
		return nil
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		for _, value := range array {
			if err := validateRawNoPlaintextSecrets(value, scope+"[]"); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeDefaultProjectDenyGlobs(values []string) []string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(defaultProjectDenyGlobs)+len(values))
	for _, value := range append(defaultProjectDenyGlobs, values...) {
		value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		merged = append(merged, value)
	}
	return merged
}

func ProjectRootFingerprint(workspace Workspace, rootPath string) (string, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return "", nil
	}
	if !filepath.IsAbs(rootPath) {
		return "", NewError(ErrorWorkspaceInvalid, "project rootPath must be absolute", 2, nil)
	}
	normalized, err := normalizeProjectRoot(rootPath)
	if err != nil {
		return "", err
	}
	salt, err := workspaceRootSalt(workspace, true)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(string(salt) + "\x00" + normalized))
	return "rootfp_" + hex.EncodeToString(sum[:]), nil
}

func ExistingProjectRootFingerprint(workspace Workspace, rootPath string) (string, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return "", nil
	}
	if !filepath.IsAbs(rootPath) {
		return "", NewError(ErrorWorkspaceInvalid, "project rootPath must be absolute", 2, nil)
	}
	normalized, err := normalizeProjectRoot(rootPath)
	if err != nil {
		return "", err
	}
	salt, err := workspaceRootSalt(workspace, false)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(string(salt) + "\x00" + normalized))
	return "rootfp_" + hex.EncodeToString(sum[:]), nil
}

func normalizeProjectRoot(rootPath string) (string, error) {
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve project rootPath", 1, err)
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	return abs, nil
}

func workspaceRootSalt(workspace Workspace, create bool) ([]byte, error) {
	if strings.TrimSpace(workspace.Root) == "" {
		return nil, NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	path := workspace.Path(".opsc", "project-root.salt")
	if data, err := os.ReadFile(path); err == nil {
		value := strings.TrimSpace(string(data))
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) < 32 {
			return nil, NewError(ErrorWorkspaceInvalid, "project root salt is invalid", 2, nil)
		}
		return decoded, nil
	} else if !os.IsNotExist(err) {
		return nil, WrapError(ErrorInternal, "read project root salt", 5, err)
	}
	if !create {
		return nil, NewError(ErrorWorkspaceNotFound, "project root salt not found", 2, nil)
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, WrapError(ErrorInternal, "generate project root salt", 5, err)
	}
	if err := AtomicWriteFile(path, []byte(hex.EncodeToString(salt)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return salt, nil
}

func projectRootSaltExists(workspace Workspace) bool {
	stat, err := os.Stat(workspace.Path(".opsc", "project-root.salt"))
	return err == nil && !stat.IsDir()
}

type ProjectPathRequest struct {
	Operation ProjectPathOperation
	Path      string
}

type ProjectPathResult struct {
	Path         string `json:"path,omitempty"`
	RelativePath string `json:"relativePath"`
	Operation    string `json:"operation"`
}

func ResolveProjectPath(workspace Workspace, project Envelope[ProjectData], request ProjectPathRequest) (ProjectPathResult, error) {
	_ = workspace
	data := project.Data
	operation := request.Operation
	if operation == "" {
		operation = ProjectPathRead
	}
	if err := projectCapabilityAllows(data.Capabilities, operation); err != nil {
		return ProjectPathResult{}, err
	}
	root := strings.TrimSpace(data.RootPath)
	if root == "" {
		return ProjectPathResult{}, NewError(ErrorWorkspaceInvalid, "project rootPath is empty", 2, map[string]string{"projectId": project.ID})
	}
	if !filepath.IsAbs(root) {
		return ProjectPathResult{}, NewError(ErrorWorkspaceInvalid, "project rootPath must be absolute", 2, map[string]string{"projectId": project.ID})
	}
	rel, err := cleanProjectRelativePath(request.Path)
	if err != nil {
		return ProjectPathResult{}, err
	}
	if projectPathDenied(data.Execution, rel) {
		return ProjectPathResult{}, NewError(ErrorWorkspaceInvalid, "project path is denied by execution policy", 2, map[string]string{"path": rel})
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	resolved, err := resolveProjectTarget(root, target, operation)
	if err != nil {
		return ProjectPathResult{}, err
	}
	return ProjectPathResult{
		Path:         resolved,
		RelativePath: rel,
		Operation:    string(operation),
	}, nil
}

func projectCapabilityAllows(capabilities ProjectCapabilities, operation ProjectPathOperation) error {
	switch operation {
	case ProjectPathRead:
		if capabilities.FSRead {
			return nil
		}
		return NewError(ErrorWorkspaceInvalid, "project capability fs.read is disabled", 2, nil)
	case ProjectPathWrite:
		if capabilities.FSWrite {
			return nil
		}
		return NewError(ErrorWorkspaceInvalid, "project capability fs.write is disabled", 2, nil)
	case ProjectPathExec:
		if capabilities.ProcessExec {
			return nil
		}
		return NewError(ErrorWorkspaceInvalid, "project capability process.exec is disabled", 2, nil)
	default:
		return NewError(ErrorInvalidArgument, "project path operation is not allowed", 1, map[string]string{"operation": string(operation)})
	}
}

func cleanProjectRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if value == "" || cleaned == "." {
		return "", NewError(ErrorInvalidArgument, "project path is empty", 1, nil)
	}
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", NewError(ErrorWorkspaceInvalid, "project path escapes root", 2, nil)
	}
	return filepath.ToSlash(cleaned), nil
}

func projectPathDenied(execution ProjectExecution, rel string) bool {
	denies := mergeDefaultProjectDenyGlobs(execution.DenyGlobs)
	for _, pattern := range denies {
		if matchProjectGlob(pattern, rel) {
			return true
		}
	}
	allowGlobs := normalizedProjectGlobs(execution.AllowGlobs)
	if len(allowGlobs) == 0 {
		return false
	}
	for _, pattern := range allowGlobs {
		if matchProjectGlob(pattern, rel) {
			return false
		}
	}
	return true
}

func normalizedProjectGlobs(values []string) []string {
	globs := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
		if value != "" {
			globs = append(globs, value)
		}
	}
	return globs
}

func matchProjectGlob(pattern string, rel string) bool {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if pattern == "" || rel == "" {
		return false
	}
	if ok, _ := filepath.Match(pattern, rel); ok {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		rest := strings.TrimPrefix(pattern, "**/")
		if strings.HasSuffix(rest, "/**") {
			dir := strings.TrimSuffix(rest, "/**")
			return rel == dir || strings.HasPrefix(rel, dir+"/") || strings.Contains(rel, "/"+dir+"/")
		}
		parts := strings.Split(rel, "/")
		for i := range parts {
			suffix := strings.Join(parts[i:], "/")
			if ok, _ := filepath.Match(rest, suffix); ok {
				return true
			}
		}
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	return false
}

func resolveProjectTarget(root string, target string, operation ProjectPathOperation) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve project root", 1, err)
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", WrapError(ErrorWorkspaceInvalid, "resolve project root symlinks", 2, err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", WrapError(ErrorInvalidArgument, "resolve project path", 1, err)
	}
	checkPath := targetAbs
	if resolved, err := filepath.EvalSymlinks(targetAbs); err == nil {
		checkPath = resolved
	} else if operation == ProjectPathWrite {
		parent := filepath.Dir(targetAbs)
		resolvedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr != nil {
			return "", WrapError(ErrorWorkspaceInvalid, "resolve project path parent symlinks", 2, parentErr)
		}
		checkPath = filepath.Join(resolvedParent, filepath.Base(targetAbs))
	} else {
		return "", WrapError(ErrorWorkspaceInvalid, "resolve project path symlinks", 2, err)
	}
	if !pathInsideRoot(rootResolved, checkPath) {
		return "", NewError(ErrorWorkspaceInvalid, "project path escapes root", 2, nil)
	}
	return filepath.Clean(checkPath), nil
}

func pathInsideRoot(root string, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}
