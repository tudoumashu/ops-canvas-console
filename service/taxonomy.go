package service

import (
	"log"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

const (
	assetScopeLibrary = "library"

	assetPurposeGeneric           = "generic"
	assetPurposeStandardReference = "standard_reference"
	assetPurposeOfficialReference = "official_reference"
	assetPurposeSpecTemplate      = "spec_template"

	assetSourceCloud       = "cloud_asset"
	assetSourceLocalUpload = "local_upload"
	assetSourceAIGenerated = "ai_generated"

	assetCategoryGeneric           = "通用素材"
	assetCategoryGenericImage      = "通用图片"
	assetCategoryGenericText       = "文本素材"
	assetCategoryGenericVideo      = "视频素材"
	assetCategoryStandardReference = "角色参考图/标准参考图"
	assetCategoryOfficialReference = "角色参考图/官方参考图"
	assetCategorySpecTemplate      = "规格图模板"
)

var normalizeTaxonomyMu sync.Mutex
var normalizeTaxonomyDone bool

func ensureTaxonomyNormalized() {
	normalizeTaxonomyMu.Lock()
	defer normalizeTaxonomyMu.Unlock()
	if normalizeTaxonomyDone {
		return
	}
	ok := true
	if err := normalizeExistingAssets(); err != nil {
		log.Printf("[taxonomy] normalize assets failed: %v", err)
		ok = false
	}
	if err := normalizeExistingPrompts(); err != nil {
		log.Printf("[taxonomy] normalize prompts failed: %v", err)
		ok = false
	}
	if ok {
		normalizeTaxonomyDone = true
	}
}

func normalizeExistingAssets() error {
	for {
		items, err := repository.ListAssetsNeedingTaxonomy(model.MaxPageSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for _, item := range items {
			next := normalizeAssetTaxonomy(item)
			if !reflect.DeepEqual(next, item) {
				if _, err := repository.SaveAsset(next); err != nil {
					return err
				}
			}
		}
	}
}

func normalizeExistingPrompts() error {
	for {
		items, err := repository.ListPromptsNeedingTaxonomy(model.MaxPageSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for _, item := range items {
			next := normalizePromptTaxonomy(item)
			if !reflect.DeepEqual(next, item) {
				if _, err := repository.SavePrompt(next); err != nil {
					return err
				}
			}
		}
	}
}

func normalizeAssetTaxonomy(item model.Asset) model.Asset {
	if item.Type == "" {
		item.Type = model.AssetTypeText
	}
	if item.MediaType == "" {
		item.MediaType = string(item.Type)
	}
	if item.Scope == "" {
		item.Scope = assetScopeLibrary
	}
	if item.Purpose == "mockup_base" {
		item.Purpose = assetPurposeSpecTemplate
	}
	if item.CategoryPath == "Mockup底版" {
		item.CategoryPath = assetCategorySpecTemplate
	}
	if item.Category == "Mockup底版" {
		item.Category = assetCategorySpecTemplate
	}
	if item.Source == "" {
		item.Source = inferAssetSource(item)
	}
	item.Source = normalizeAssetSource(item.Source)
	if item.Purpose == "" {
		item.Purpose = inferAssetPurpose(item)
	}
	if item.CategoryPath == "" {
		item.CategoryPath = categoryPathForAsset(item)
	}
	if item.Category == "" || isLegacyAssetCategory(item.Category) {
		item.Category = item.CategoryPath
	}
	item.Metadata = normalizeAssetMetadata(item)
	item.Tags = normalizeAssetTags(item)
	return item
}

func inferAssetSource(item model.Asset) string {
	return assetSourceCloud
}

func normalizeAssetSource(value string) string {
	switch value {
	case "", "uploaded", "imported", "admin_created", "generated":
		return assetSourceCloud
	case assetSourceLocalUpload, assetSourceAIGenerated, assetSourceCloud:
		return value
	default:
		return assetSourceCloud
	}
}

func inferAssetPurpose(item model.Asset) string {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Category, item.Description, item.URL, strings.Join(item.Tags, " ")}, " "))
	switch {
	case strings.Contains(text, "标准参考图") || strings.Contains(text, "standard_reference") || strings.Contains(text, "/reference."):
		return assetPurposeStandardReference
	case strings.Contains(text, "官方参考图") || strings.Contains(text, "official_reference") || strings.Contains(text, "/official_references/"):
		return assetPurposeOfficialReference
	case strings.Contains(text, "规格图模板") || strings.Contains(text, "spec_template"):
		return assetPurposeSpecTemplate
	case strings.Contains(text, "mockup") || strings.Contains(text, "底版"):
		return assetPurposeSpecTemplate
	default:
		return assetPurposeGeneric
	}
}

func categoryPathForAsset(item model.Asset) string {
	switch item.Purpose {
	case assetPurposeStandardReference:
		return assetCategoryStandardReference
	case assetPurposeOfficialReference:
		return assetCategoryOfficialReference
	case assetPurposeSpecTemplate:
		return assetCategorySpecTemplate
	}
	switch item.MediaType {
	case string(model.AssetTypeImage):
		return assetCategoryGenericImage
	case string(model.AssetTypeText):
		return assetCategoryGenericText
	case string(model.AssetTypeVideo):
		return assetCategoryGenericVideo
	default:
		return assetCategoryGeneric
	}
}

func normalizeAssetMetadata(item model.Asset) map[string]any {
	metadata := copyMetadata(item.Metadata)
	if rel := materialRelativePath(item); rel != "" {
		metadata["originPath"] = rel
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) >= 4 && parts[0] == "reference_library" && parts[1] == "anime_ip" {
			metadata["ip"] = parts[2]
			metadata["character"] = parts[3]
		}
	}
	return metadata
}

func normalizeAssetTags(item model.Asset) []string {
	metadata := item.Metadata
	if metadata == nil {
		metadata = normalizeAssetMetadata(item)
	}
	skip := map[string]bool{
		"":                   true,
		"图片":                 true,
		"文本":                 true,
		"视频":                 true,
		"素材库":                true,
		"PDD素材":              true,
		"PDD Mockup":         true,
		"Mockup底版":           true,
		"官方参考图":              true,
		"标准参考图":              true,
		"素材图片":               true,
		"sku_artwork":        true,
		item.Category:        true,
		item.CategoryPath:    true,
		item.Purpose:         true,
		item.Source:          true,
		string(item.Type):    true,
		item.MediaType:       true,
		assetScopeLibrary:    true,
		assetCategoryGeneric: true,
	}
	for _, key := range []string{"ip", "character"} {
		if value, ok := metadata[key].(string); ok {
			skip[value] = true
		}
	}
	return uniqueVisibleTags(item.Tags, skip)
}

func materialRelativePath(item model.Asset) string {
	for _, raw := range []string{item.URL, item.CoverURL} {
		parsed, err := url.Parse(raw)
		if err == nil {
			if path := parsed.Query().Get("path"); path != "" {
				return filepath.ToSlash(path)
			}
		}
	}
	if idx := strings.Index(item.Description, "："); idx >= 0 {
		return filepath.ToSlash(strings.TrimSpace(item.Description[idx+len("："):]))
	}
	return ""
}

func isLegacyAssetCategory(category string) bool {
	switch category {
	case "PDD素材", "pdd-mockup-assets":
		return true
	default:
		return false
	}
}

func normalizePromptTaxonomy(item model.Prompt) model.Prompt {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Category, item.Prompt, strings.Join(item.Tags, " ")}, " "))
	if isRemotePromptCategory(item.Category) {
		item.Domain = "image"
		item.Stage = "general"
		item.Provider = "openai"
		item.Model = "gpt-image-2"
		item.Mode = "general"
		item.InputType = "text"
		item.OutputType = "image"
		item.Status = "production"
	} else {
		item.Domain = normalizePromptDomain(item.Domain, item.Category, text)
		item.Stage = normalizePromptStage(item.Stage, text)
	}
	if item.Provider == "" {
		item.Provider = inferPromptProvider(text)
	}
	if item.Model == "" {
		item.Model = inferPromptModel(text)
	}
	if item.Mode == "" {
		item.Mode = inferPromptMode(text)
	}
	if item.InputType == "" {
		item.InputType = inferPromptInputType(item)
	}
	if item.OutputType == "" {
		item.OutputType = inferPromptOutputType(item)
	}
	if item.Status == "" {
		item.Status = "production"
	}
	item.Tags = normalizePromptTags(item)
	return item
}

func normalizePromptDomain(domain string, category string, text string) string {
	if isRemotePromptCategory(category) {
		return "image"
	}
	switch domain {
	case "image", "text", "video":
		return domain
	case "general", "":
		return inferPromptDomain(text)
	default:
		return inferPromptDomain(text)
	}
}

func normalizePromptStage(stage string, text string) string {
	switch stage {
	case "general", "repair", "main_image", "spec_image", "quality_review":
		return stage
	}
	switch {
	case strings.Contains(text, "规格") || strings.Contains(text, "sku") || strings.Contains(text, "spec"):
		return "spec_image"
	case strings.Contains(text, "quality") || strings.Contains(text, "review") || strings.Contains(text, "质检") || strings.Contains(text, "复检"):
		return "quality_review"
	case strings.Contains(text, "repair") || strings.Contains(text, "修复"):
		return "repair"
	case strings.Contains(text, "main") || strings.Contains(text, "主图"):
		return "main_image"
	default:
		return "general"
	}
}

func isRemotePromptCategory(category string) bool {
	switch category {
	case "gpt-image-2-prompts", "awesome-gpt-image", "awesome-gpt4o-image-prompts", "youmind-gpt-image-2", "youmind-nano-banana-pro", "davidwu-gpt-image2-prompts":
		return true
	default:
		return false
	}
}

func inferPromptDomain(text string) string {
	switch {
	case strings.Contains(text, "video") || strings.Contains(text, "sora"):
		return "video"
	case strings.Contains(text, "image") || strings.Contains(text, "图") || strings.Contains(text, "banana"):
		return "image"
	case strings.Contains(text, "json") || strings.Contains(text, "标题") || strings.Contains(text, "文案"):
		return "text"
	default:
		return "image"
	}
}

func inferPromptStage(text string) string {
	switch {
	case strings.Contains(text, "规格") || strings.Contains(text, "sku") || strings.Contains(text, "spec"):
		return "spec_image"
	case strings.Contains(text, "quality") || strings.Contains(text, "review") || strings.Contains(text, "质检") || strings.Contains(text, "复检"):
		return "quality_review"
	case strings.Contains(text, "repair") || strings.Contains(text, "修复"):
		return "repair"
	case strings.Contains(text, "main") || strings.Contains(text, "主图"):
		return "main_image"
	default:
		return "general"
	}
}

func inferPromptProvider(text string) string {
	switch {
	case strings.Contains(text, "gemini") || strings.Contains(text, "banana"):
		return "gemini"
	case strings.Contains(text, "chatgpt2api"):
		return "chatgpt2api"
	case strings.Contains(text, "openai") || strings.Contains(text, "gpt"):
		return "openai"
	default:
		return "other"
	}
}

func inferPromptModel(text string) string {
	switch {
	case strings.Contains(text, "gpt-image-2") || strings.Contains(text, "gpt image 2"):
		return "gpt-image-2"
	case strings.Contains(text, "gpt-4o") || strings.Contains(text, "gpt4o"):
		return "gpt-4o"
	case strings.Contains(text, "nano banana"):
		return "nano-banana"
	case strings.Contains(text, "gpt-5.5") || strings.Contains(text, "gpt-5-5"):
		return "gpt-5.5"
	case strings.Contains(text, "sora"):
		return "sora"
	default:
		return "other"
	}
}

func inferPromptMode(text string) string {
	switch {
	case strings.Contains(text, "white_bed_3d"):
		return "white_bed_3d"
	case strings.Contains(text, "white_bed_2d"):
		return "white_bed_2d"
	case strings.Contains(text, "themed_bed_3d"):
		return "themed_bed_3d"
	case strings.Contains(text, "themed_bed_2d"):
		return "themed_bed_2d"
	default:
		return "general"
	}
}

func inferPromptInputType(item model.Prompt) string {
	switch item.Stage {
	case "repair", "main_image", "spec_image", "quality_review":
		return "image"
	default:
		return "text"
	}
}

func inferPromptOutputType(item model.Prompt) string {
	switch item.Stage {
	case "quality_review":
		return "json"
	case "repair", "main_image", "spec_image":
		return "image"
	}
	if item.Domain == "image" {
		return "image"
	}
	if item.Domain == "video" {
		return "video"
	}
	return "text"
}

func normalizePromptTags(item model.Prompt) []string {
	skip := map[string]bool{
		"":                true,
		item.Category:     true,
		item.Domain:       true,
		item.Stage:        true,
		item.Provider:     true,
		item.Model:        true,
		item.Mode:         true,
		item.InputType:    true,
		item.OutputType:   true,
		item.Status:       true,
		"gpt4o":           true,
		"gpt-image-2":     true,
		"gpt image 2":     true,
		"nano banana":     true,
		"prompt":          true,
		"prompts":         true,
		"image":           true,
		"images":          true,
		"提示词":             true,
		"图像":              true,
		"图片":              true,
		"文案":              true,
		"json":            true,
		"self-gpt-image2": true,
	}
	return uniqueVisibleTags(item.Tags, skip)
}

func copyMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}
	next := map[string]any{}
	for key, value := range metadata {
		next[key] = value
	}
	return next
}

func uniqueVisibleTags(tags []string, skip map[string]bool) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		if skip[value] || skip[strings.ToLower(value)] || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
