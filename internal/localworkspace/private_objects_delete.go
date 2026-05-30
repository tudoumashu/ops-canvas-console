package localworkspace

func DeleteTemplate(workspace Workspace, id string) error {
	repo := TemplateRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeleteTemplate(id)
		})
	})
}

func DeleteProfile(workspace Workspace, id string) error {
	repo := ProfileRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeleteProfile(id)
		})
	})
}

func DeleteProject(workspace Workspace, id string) error {
	repo := ProjectRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeleteProject(id)
		})
	})
}

func DeleteAsset(workspace Workspace, id string) error {
	repo := AssetRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeleteAsset(id)
		})
	})
}

func DeletePrompt(workspace Workspace, id string) error {
	repo := PromptRepository(workspace)
	if err := repo.validateID(id); err != nil {
		return err
	}
	return withWorkspaceLock(workspace, func() error {
		if err := repo.Delete(id); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			return index.DeletePrompt(id)
		})
	})
}
