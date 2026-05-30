package localworkspace

import "strings"

const (
	WorkflowFolderKindArticle = "article"
	WorkflowFolderKindVideo   = "video"
	WorkflowFolderKindCustom  = "custom"
)

type WorkspacePreferencesSnapshot struct {
	Revision    int                  `json:"revision"`
	Preferences WorkspacePreferences `json:"preferences"`
}

func ReadWorkspacePreferences(workspace Workspace) (WorkspacePreferencesSnapshot, error) {
	document, err := readWorkspaceDocument(workspace.WorkspaceFilePath())
	if err != nil {
		return WorkspacePreferencesSnapshot{}, err
	}
	document.Data.Preferences = normalizeWorkspacePreferences(document.Data.Preferences)
	return workspacePreferencesSnapshot(document), nil
}

func UpdateWorkspacePreferences(workspace Workspace, revision int, preferences WorkspacePreferences) (WorkspaceDocument, error) {
	preferences = normalizeWorkspacePreferences(preferences)
	if err := validateWorkspacePreferences(preferences); err != nil {
		return WorkspaceDocument{}, err
	}
	var updated WorkspaceDocument
	if err := withWorkspaceLock(workspace, func() error {
		document, err := readWorkspaceDocument(workspace.WorkspaceFilePath())
		if err != nil {
			return err
		}
		if err := requireRevision(document.Revision, revision); err != nil {
			return err
		}
		document.Revision++
		document.UpdatedAt = timeNowRFC3339()
		document.Data.Preferences = preferences
		if err := AtomicWriteJSON(workspace.WorkspaceFilePath(), document, 0o600); err != nil {
			return err
		}
		updated = document
		return nil
	}); err != nil {
		return WorkspaceDocument{}, err
	}
	return updated, nil
}

func workspacePreferencesSnapshot(document WorkspaceDocument) WorkspacePreferencesSnapshot {
	return WorkspacePreferencesSnapshot{
		Revision:    document.Revision,
		Preferences: normalizeWorkspacePreferences(document.Data.Preferences),
	}
}

func normalizeWorkspacePreferences(preferences WorkspacePreferences) WorkspacePreferences {
	folders := make([]WorkflowFolderPreference, 0, len(preferences.WorkflowFolders))
	for _, folder := range preferences.WorkflowFolders {
		next := WorkflowFolderPreference{
			ID:          strings.TrimSpace(folder.ID),
			Title:       strings.TrimSpace(folder.Title),
			Description: strings.TrimSpace(folder.Description),
			Href:        strings.TrimSpace(folder.Href),
			Kind:        strings.TrimSpace(folder.Kind),
		}
		if next.Kind == "" {
			next.Kind = WorkflowFolderKindCustom
		}
		folders = append(folders, next)
	}
	return WorkspacePreferences{WorkflowFolders: folders}
}

func validateWorkspacePreferences(preferences WorkspacePreferences) error {
	if len(preferences.WorkflowFolders) > 100 {
		return NewError(ErrorWorkspaceInvalid, "too many workflow folders", 2, nil)
	}
	seen := map[string]bool{}
	for _, folder := range preferences.WorkflowFolders {
		if err := validateWorkflowFolderPreference(folder); err != nil {
			return err
		}
		if seen[folder.ID] {
			return NewError(ErrorWorkspaceInvalid, "workflow folder id is duplicated", 2, map[string]string{"id": folder.ID})
		}
		seen[folder.ID] = true
	}
	return nil
}

func validateWorkflowFolderPreference(folder WorkflowFolderPreference) error {
	if err := validatePathComponent("workflow folder id", folder.ID); err != nil {
		return err
	}
	if folder.ID == "pdd" {
		return NewError(ErrorWorkspaceInvalid, "workflow folder id is reserved", 2, map[string]string{"id": folder.ID})
	}
	if folder.Title == "" {
		return NewError(ErrorWorkspaceInvalid, "workflow folder title is empty", 2, map[string]string{"id": folder.ID})
	}
	if len([]rune(folder.Title)) > 120 {
		return NewError(ErrorWorkspaceInvalid, "workflow folder title is too long", 2, map[string]string{"id": folder.ID})
	}
	if len([]rune(folder.Description)) > 500 {
		return NewError(ErrorWorkspaceInvalid, "workflow folder description is too long", 2, map[string]string{"id": folder.ID})
	}
	switch folder.Kind {
	case WorkflowFolderKindArticle, WorkflowFolderKindVideo, WorkflowFolderKindCustom:
	default:
		return NewError(ErrorWorkspaceInvalid, "workflow folder kind is not allowed", 2, map[string]string{"kind": folder.Kind})
	}
	if folder.Href != "" && (!strings.HasPrefix(folder.Href, "/") || strings.HasPrefix(folder.Href, "//") || strings.Contains(folder.Href, "://")) {
		return NewError(ErrorWorkspaceInvalid, "workflow folder href must be an internal path", 2, map[string]string{"href": folder.Href})
	}
	return nil
}
