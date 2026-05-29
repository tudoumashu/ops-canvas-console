package service

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

type pddPromptModeFile struct {
	Mode    string         `json:"mode"`
	Prompts map[string]any `json:"prompts"`
}

func SyncPDDPromptLibrary() {
	root := strings.TrimSpace(config.Cfg.PDDPromptsRoot)
	if root == "" {
		return
	}
	modeRoot := filepath.Join(root, "mode")
	entries, err := os.ReadDir(modeRoot)
	if err != nil {
		log.Printf("[pdd-prompts] skip sync: %v", err)
		return
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if imported, err := syncPDDPromptModeFile(filepath.Join(modeRoot, entry.Name())); err != nil {
			log.Printf("[pdd-prompts] sync failed file=%s err=%v", entry.Name(), err)
		} else {
			count += imported
		}
	}
	if count > 0 {
		log.Printf("[pdd-prompts] synced %d production prompts", count)
	}
}

func syncPDDPromptModeFile(path string) (int, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var modeFile pddPromptModeFile
	if err := json.Unmarshal(body, &modeFile); err != nil {
		return 0, err
	}
	mode := strings.TrimSpace(modeFile.Mode)
	if mode == "" {
		mode = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	now := time.Now().Format(time.RFC3339)
	count := 0
	for key, value := range modeFile.Prompts {
		prompt := strings.TrimSpace(pddPromptValueText(value))
		if prompt == "" {
			continue
		}
		stage, domain, inputType, outputType := pddPromptTaxonomy(key)
		item := model.Prompt{
			ID:         "pdd-production-" + safePromptID(mode+"-"+key),
			Title:      pddPromptTitle(mode, key),
			Prompt:     prompt,
			Tags:       []string{"生产提示词", "工作流", "pdd", mode, key},
			Category:   "manual-prompts",
			Domain:     domain,
			Stage:      stage,
			Provider:   pddPromptProvider(domain),
			Model:      pddPromptModel(domain, stage),
			Mode:       mode,
			InputType:  inputType,
			OutputType: outputType,
			Status:     "production",
			Metadata: map[string]any{
				"source":     "pdd-prompts",
				"sourceFile": path,
				"promptKey":  key,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := repository.SavePrompt(item); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func pddPromptValueText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		lines := make([]string, 0, len(typed))
		for _, line := range typed {
			lines = append(lines, anyToString(line))
		}
		return strings.Join(lines, "\n")
	default:
		body, _ := json.MarshalIndent(typed, "", "  ")
		return string(body)
	}
}

func pddPromptTaxonomy(key string) (stage string, domain string, inputType string, outputType string) {
	name := strings.ToLower(key)
	switch {
	case strings.Contains(name, "quality") || strings.Contains(name, "review"):
		return "quality_review", "text", "image", "json"
	case strings.Contains(name, "title"):
		return "title", "text", "image", "json"
	case strings.Contains(name, "repair"):
		return "repair", "image", "image", "image"
	case strings.Contains(name, "main"):
		return "main_image", "image", "image", "image"
	case strings.Contains(name, "reference"):
		return "general", "image", "image", "image"
	default:
		return "general", "image", "text", "image"
	}
}

func pddPromptProvider(domain string) string {
	if domain == "text" {
		return "openai"
	}
	return "openai"
}

func pddPromptModel(domain string, stage string) string {
	if domain == "text" || stage == "quality_review" || stage == "title" {
		return "gpt-5.5"
	}
	return "gpt-image-2"
}

func pddPromptTitle(mode string, key string) string {
	title := strings.ReplaceAll(key, "_", " ")
	return strings.TrimSpace(mode + " / " + title)
}

func safePromptID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-", "!", "-", "?", "-")
	value = replacer.Replace(value)
	parts := []rune{}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			parts = append(parts, r)
		}
	}
	result := strings.Trim(string(parts), "-")
	if result == "" {
		return "prompt"
	}
	return result
}
