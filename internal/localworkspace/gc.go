package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type GCPlanOptions struct{}

type GCPlan struct {
	WorkspaceID string        `json:"workspaceId"`
	Candidates  []GCCandidate `json:"candidates"`
	Warnings    []string      `json:"warnings,omitempty"`
}

type GCCandidate struct {
	Kind         string   `json:"kind"`
	ID           string   `json:"id,omitempty"`
	Path         string   `json:"path,omitempty"`
	Reason       string   `json:"reason"`
	Action       string   `json:"action"`
	ReferencedBy []string `json:"referencedBy,omitempty"`
}

func BuildGCPlan(workspace Workspace, _ GCPlanOptions) (GCPlan, error) {
	if strings.TrimSpace(workspace.Root) == "" {
		return GCPlan{}, NewError(ErrorInvalidArgument, "workspace root is empty", 1, nil)
	}
	plan := GCPlan{WorkspaceID: workspace.Document.ID}
	artifactDocs, err := ListArtifacts(workspace)
	if err != nil {
		return GCPlan{}, err
	}
	artifacts := map[string]Envelope[ArtifactData]{}
	for _, artifact := range artifactDocs {
		artifacts[artifact.ID] = artifact
	}
	referencedArtifacts := map[string]map[string]bool{}
	addReference := func(artifactID string, owner string) {
		artifactID = strings.TrimSpace(artifactID)
		if artifactID == "" {
			return
		}
		if referencedArtifacts[artifactID] == nil {
			referencedArtifacts[artifactID] = map[string]bool{}
		}
		referencedArtifacts[artifactID][owner] = true
	}
	addCandidate := func(candidate GCCandidate) {
		candidate.Path = filepath.ToSlash(candidate.Path)
		candidate.Action = "review"
		plan.Candidates = append(plan.Candidates, candidate)
	}

	runs, runWarnings := listRunsForGC(workspace)
	plan.Warnings = append(plan.Warnings, runWarnings...)
	for _, run := range runs {
		for _, artifactID := range run.ArtifactIDs {
			owner := "run:" + run.ID
			addReference(artifactID, owner)
			if _, ok := artifacts[artifactID]; !ok && strings.TrimSpace(artifactID) != "" {
				addCandidate(GCCandidate{
					Kind:         "run_artifact_ref",
					ID:           artifactID,
					Path:         filepath.Join("runs", run.ID, "run.json"),
					Reason:       "run artifactRefs contains missing artifact",
					ReferencedBy: []string{owner},
				})
			}
		}
		refDocs, err := listRunArtifactRefs(workspace, run.ID)
		if err != nil {
			plan.Warnings = append(plan.Warnings, "run artifact refs cannot be scanned: "+run.ID)
			continue
		}
		for _, ref := range refDocs {
			owner := "run:" + run.ID
			addReference(ref.Data.ArtifactID, owner)
			if _, ok := artifacts[ref.Data.ArtifactID]; !ok && strings.TrimSpace(ref.Data.ArtifactID) != "" {
				addCandidate(GCCandidate{
					Kind:         "run_artifact_ref",
					ID:           ref.Data.ArtifactID,
					Path:         filepath.Join("runs", run.ID, "artifacts", ref.ID+".ref.json"),
					Reason:       "run artifact ref points to missing artifact",
					ReferencedBy: []string{owner},
				})
			}
		}
	}

	assets, assetWarnings := listAssetsForGC(workspace)
	plan.Warnings = append(plan.Warnings, assetWarnings...)
	for _, asset := range assets {
		owner := "asset:" + asset.ID
		if strings.TrimSpace(asset.Data.SourceArtifactID) != "" {
			addReference(asset.Data.SourceArtifactID, owner)
			if _, ok := artifacts[asset.Data.SourceArtifactID]; !ok {
				addCandidate(GCCandidate{
					Kind:         "asset",
					ID:           asset.ID,
					Path:         filepath.Join("assets", asset.ID, "asset.json"),
					Reason:       "asset sourceArtifactId points to missing artifact",
					ReferencedBy: []string{owner},
				})
			}
		}
		for name, assetPath := range asset.Data.Files {
			if strings.TrimSpace(assetPath) == "" || !isWorkspaceRelativeFile(assetPath) {
				continue
			}
			path := filepath.Join("assets", asset.ID, filepath.FromSlash(assetPath))
			if stat, err := os.Stat(filepath.Join(workspace.Root, path)); err != nil {
				if os.IsNotExist(err) {
					addCandidate(GCCandidate{
						Kind:   "asset_file",
						ID:     asset.ID + "." + name,
						Path:   path,
						Reason: "asset file is missing",
					})
					continue
				}
				plan.Warnings = append(plan.Warnings, filepath.ToSlash(path)+": "+err.Error())
			} else if stat.IsDir() {
				addCandidate(GCCandidate{
					Kind:   "asset_file",
					ID:     asset.ID + "." + name,
					Path:   path,
					Reason: "asset file points to a directory",
				})
			}
		}
	}

	prompts, err := ListPrompts(workspace)
	if err != nil {
		return GCPlan{}, err
	}
	for _, prompt := range prompts {
		path := filepath.Join("prompts", prompt.ID, "content.md")
		if stat, err := os.Stat(filepath.Join(workspace.Root, path)); err != nil {
			if os.IsNotExist(err) {
				addCandidate(GCCandidate{
					Kind:   "prompt_content",
					ID:     prompt.ID,
					Path:   path,
					Reason: "prompt content.md is missing",
				})
				continue
			}
			plan.Warnings = append(plan.Warnings, filepath.ToSlash(path)+": "+err.Error())
		} else if stat.IsDir() {
			addCandidate(GCCandidate{
				Kind:   "prompt_content",
				ID:     prompt.ID,
				Path:   path,
				Reason: "prompt content.md is a directory",
			})
		}
	}

	canvasProjects, canvasWarnings := listCanvasProjectsForGC(workspace)
	plan.Warnings = append(plan.Warnings, canvasWarnings...)
	for _, project := range canvasProjects {
		for key, item := range project.Data.Files {
			if strings.TrimSpace(item.Path) == "" || !isWorkspaceRelativeFile(item.Path) {
				continue
			}
			path := filepath.Join("canvas-projects", project.ID, filepath.FromSlash(item.Path))
			if stat, err := os.Stat(filepath.Join(workspace.Root, path)); err != nil {
				if os.IsNotExist(err) {
					addCandidate(GCCandidate{
						Kind:   "canvas_project_file",
						ID:     project.ID + "." + key,
						Path:   path,
						Reason: "canvas project file is missing",
					})
					continue
				}
				plan.Warnings = append(plan.Warnings, filepath.ToSlash(path)+": "+err.Error())
			} else if stat.IsDir() {
				addCandidate(GCCandidate{
					Kind:   "canvas_project_file",
					ID:     project.ID + "." + key,
					Path:   path,
					Reason: "canvas project file points to a directory",
				})
			}
		}
	}

	workbenchLogs, workbenchWarnings := listWorkbenchLogsForGC(workspace)
	plan.Warnings = append(plan.Warnings, workbenchWarnings...)
	for _, log := range workbenchLogs {
		for _, media := range log.Data.Media {
			if strings.TrimSpace(media.Path) == "" || !isWorkspaceRelativeFile(media.Path) {
				continue
			}
			path := filepath.Join("workbench-logs", log.ID, filepath.FromSlash(media.Path))
			if stat, err := os.Stat(filepath.Join(workspace.Root, path)); err != nil {
				if os.IsNotExist(err) {
					addCandidate(GCCandidate{
						Kind:   "workbench_log_file",
						ID:     log.ID + "." + media.Key,
						Path:   path,
						Reason: "workbench log media file is missing",
					})
					continue
				}
				plan.Warnings = append(plan.Warnings, filepath.ToSlash(path)+": "+err.Error())
			} else if stat.IsDir() {
				addCandidate(GCCandidate{
					Kind:   "workbench_log_file",
					ID:     log.ID + "." + media.Key,
					Path:   path,
					Reason: "workbench log media file points to a directory",
				})
			}
		}
	}

	for _, artifact := range artifactDocs {
		if len(referencedArtifacts[artifact.ID]) > 0 {
			continue
		}
		addCandidate(GCCandidate{
			Kind:   "artifact",
			ID:     artifact.ID,
			Path:   filepath.Join("artifacts", artifact.ID, "artifact.json"),
			Reason: "artifact has no run or asset references",
		})
	}

	for i := range plan.Candidates {
		if len(plan.Candidates[i].ReferencedBy) > 1 {
			sort.Strings(plan.Candidates[i].ReferencedBy)
		}
	}
	sort.SliceStable(plan.Candidates, func(i int, j int) bool {
		if plan.Candidates[i].Kind != plan.Candidates[j].Kind {
			return plan.Candidates[i].Kind < plan.Candidates[j].Kind
		}
		if plan.Candidates[i].ID != plan.Candidates[j].ID {
			return plan.Candidates[i].ID < plan.Candidates[j].ID
		}
		return plan.Candidates[i].Path < plan.Candidates[j].Path
	})
	return plan, nil
}

func listCanvasProjectsForGC(workspace Workspace) ([]Envelope[CanvasProjectData], []string) {
	dir := workspace.Path("canvas-projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Envelope[CanvasProjectData]{}, nil
		}
		return []Envelope[CanvasProjectData]{}, []string{"canvas projects cannot be scanned"}
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	documents := []Envelope[CanvasProjectData]{}
	warnings := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := validatePathComponent("canvas project id", id); err != nil {
			warnings = append(warnings, "canvas project id is not path safe")
			continue
		}
		document, err := readEnvelopeFile[CanvasProjectData](filepath.Join(dir, id, "canvas-project.json"))
		if err != nil {
			warnings = append(warnings, "canvas project document cannot be scanned: "+id)
			continue
		}
		documents = append(documents, document)
	}
	return documents, warnings
}

type gcRunDocument struct {
	ID          string
	ArtifactIDs []string
}

func listRunsForGC(workspace Workspace) ([]gcRunDocument, []string) {
	dir := workspace.Path("runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []gcRunDocument{}, []string{"runs directory cannot be scanned"}
		}
		return []gcRunDocument{}, []string{"runs cannot be scanned"}
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	documents := []gcRunDocument{}
	warnings := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := validatePathComponent("run id", id); err != nil {
			warnings = append(warnings, "run id is not path safe")
			continue
		}
		envelope, err := readDoctorEnvelope(filepath.Join(dir, id, "run.json"))
		if err != nil {
			warnings = append(warnings, "run document cannot be scanned: "+id)
			continue
		}
		var data doctorRunData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			warnings = append(warnings, "run artifactRefs cannot be scanned: "+id)
			continue
		}
		documents = append(documents, gcRunDocument{
			ID:          id,
			ArtifactIDs: runArtifactIDs(data.ArtifactRefs),
		})
	}
	return documents, warnings
}

func listWorkbenchLogsForGC(workspace Workspace) ([]Envelope[WorkbenchLogData], []string) {
	dir := workspace.Path("workbench-logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Envelope[WorkbenchLogData]{}, nil
		}
		return []Envelope[WorkbenchLogData]{}, []string{"workbench logs cannot be scanned"}
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	documents := []Envelope[WorkbenchLogData]{}
	warnings := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := validatePathComponent("workbench log id", id); err != nil {
			warnings = append(warnings, "workbench log id is not path safe")
			continue
		}
		document, err := readEnvelopeFile[WorkbenchLogData](filepath.Join(dir, id, "workbench-log.json"))
		if err != nil {
			warnings = append(warnings, "workbench log document cannot be scanned: "+id)
			continue
		}
		documents = append(documents, document)
	}
	return documents, warnings
}

func listAssetsForGC(workspace Workspace) ([]Envelope[AssetData], []string) {
	dir := workspace.Path("assets")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Envelope[AssetData]{}, nil
		}
		return []Envelope[AssetData]{}, []string{"assets cannot be scanned"}
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	documents := []Envelope[AssetData]{}
	warnings := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := validatePathComponent("asset id", id); err != nil {
			warnings = append(warnings, "asset id is not path safe")
			continue
		}
		document, err := readEnvelopeFile[AssetData](filepath.Join(dir, id, "asset.json"))
		if err != nil {
			warnings = append(warnings, "asset document cannot be scanned: "+id)
			continue
		}
		documents = append(documents, document)
	}
	return documents, warnings
}
