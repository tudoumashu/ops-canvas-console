package repository

import (
	"errors"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

// PromptCategories 返回内置提示词分类的副本。
func PromptCategories() []model.PromptCategory {
	result := make([]model.PromptCategory, len(promptCategories))
	copy(result, promptCategories)
	return result
}

// PromptCategoryByCode 根据分类编码查找内置提示词分类。
func PromptCategoryByCode(category string) (model.PromptCategory, bool) {
	for _, item := range promptCategories {
		if item.Category == category {
			return item, true
		}
	}
	return model.PromptCategory{}, false
}

// ListPromptCategories 返回内置提示词分类。
func ListPromptCategories() ([]model.PromptCategory, error) {
	return PromptCategories(), nil
}

// ListPrompts 按查询条件返回提示词分页列表。
func ListPrompts(q model.Query) ([]model.Prompt, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := applyPromptFilters(db.Model(&model.Prompt{}), q)

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []model.Prompt
	if err := tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	categories, _ := ListPromptCategories()
	githubURLs := map[string]string{}
	for _, item := range categories {
		githubURLs[item.Category] = item.GithubURL
	}
	for i := range items {
		items[i].GithubURL = githubURLs[items[i].Category]
	}
	return items, total, nil
}

// ListPromptTags 返回当前提示词查询条件下的全部标签。
func ListPromptTags(q model.Query) ([]string, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	q.Normalize()
	q.Tags = nil
	tx := applyPromptFilters(db.Model(&model.Prompt{}), q)

	var items []model.Prompt
	if err := tx.Select("tags").Find(&items).Error; err != nil {
		return nil, err
	}
	return promptTagsFromItems(items), nil
}

// ListPromptFacets 返回当前提示词查询条件下的结构化筛选项。
func ListPromptFacets(q model.Query) (model.PromptFacets, error) {
	db, err := DB()
	if err != nil {
		return model.PromptFacets{}, err
	}
	q.Normalize()
	q.Category = ""
	q.Stage = ""
	q.Provider = ""
	q.Model = ""
	q.Mode = ""
	q.InputType = ""
	q.OutputType = ""
	q.Status = ""
	q.Tags = nil
	tx := applyPromptFilters(db.Model(&model.Prompt{}), q)
	var items []model.Prompt
	if err := tx.Select("category", "domain", "stage", "provider", "model", "mode", "input_type", "output_type", "status").Find(&items).Error; err != nil {
		return model.PromptFacets{}, err
	}
	categories := map[string]bool{}
	domains := map[string]bool{}
	stages := map[string]bool{}
	providers := map[string]bool{}
	models := map[string]bool{}
	modes := map[string]bool{}
	inputTypes := map[string]bool{}
	outputTypes := map[string]bool{}
	statuses := map[string]bool{}
	for _, item := range items {
		categories[item.Category] = true
		domains[item.Domain] = true
		stages[item.Stage] = true
		providers[item.Provider] = true
		models[item.Model] = true
		modes[item.Mode] = true
		inputTypes[item.InputType] = true
		outputTypes[item.OutputType] = true
		statuses[item.Status] = true
	}
	return model.PromptFacets{
		Categories:  sortedKeys(categories),
		Domains:     sortedKeys(domains),
		Stages:      sortedKeys(stages),
		Providers:   sortedKeys(providers),
		Models:      sortedKeys(models),
		Modes:       sortedKeys(modes),
		InputTypes:  sortedKeys(inputTypes),
		OutputTypes: sortedKeys(outputTypes),
		Statuses:    sortedKeys(statuses),
	}, nil
}

// ListPromptsNeedingTaxonomy 返回仍缺少结构化分类字段的提示词。
func ListPromptsNeedingTaxonomy(limit int) ([]model.Prompt, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > model.MaxPageSize {
		limit = model.MaxPageSize
	}
	var items []model.Prompt
	err = db.Where(
		`domain = '' OR domain IS NULL OR
		domain NOT IN ? OR
		stage = '' OR stage IS NULL OR
		stage NOT IN ? OR
		provider = '' OR provider IS NULL OR
		model = '' OR model IS NULL OR
		mode = '' OR mode IS NULL OR
		input_type = '' OR input_type IS NULL OR
		output_type = '' OR output_type IS NULL OR
		status = '' OR status IS NULL OR
		(category IN ? AND (domain <> ? OR stage <> ? OR provider <> ? OR model <> ? OR mode <> ? OR input_type <> ? OR output_type <> ? OR status <> ?))`,
		[]string{"image", "text", "video"},
		[]string{"general", "repair", "main_image", "spec_image", "quality_review"},
		remotePromptCategoryCodes(),
		"image",
		"general",
		"openai",
		"gpt-image-2",
		"general",
		"text",
		"image",
		"production",
	).Order("id asc").Limit(limit).Find(&items).Error
	return items, err
}

// SavePrompt 保存提示词，并在更新时保留原创建时间。
func SavePrompt(item model.Prompt) (model.Prompt, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	if saved, ok, err := findPrompt(db, item.ID); err != nil {
		return item, err
	} else if ok && item.CreatedAt == "" {
		item.CreatedAt = saved.CreatedAt
	} else if !ok && item.CreatedAt == "" {
		item.CreatedAt = item.UpdatedAt
	}
	item.GithubURL = ""
	return item, db.Save(&item).Error
}

// DeletePrompt 删除指定提示词。
func DeletePrompt(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Prompt{}, "id = ?", id).Error
}

// DeletePrompts 批量删除提示词。
func DeletePrompts(ids []string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Prompt{}, "id IN ?", ids).Error
}

// DeleteManagedPromptLibraryPrompts 删除系统托管导入的公共提示词。
func DeleteManagedPromptLibraryPrompts() error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Prompt{}, "category IN ? OR id LIKE ?", remotePromptCategoryCodes(), "pdd-production-%").Error
}

// ReplacePromptCategory 用远程同步结果替换整个提示词分类。
func ReplacePromptCategory(category model.PromptCategory, items []model.Prompt) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("category = ?", category.Category).Delete(&model.Prompt{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].Category = category.Category
			items[i].GithubURL = ""
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		if category.Remote {
			return tx.Model(&model.Prompt{}).Where("category = ?", category.Category).Updates(remotePromptImageTaxonomy()).Error
		}
		return nil
	})
}

// applyPromptFilters 应用提示词列表的搜索条件。
func applyPromptFilters(tx *gorm.DB, q model.Query) *gorm.DB {
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("title LIKE ? OR prompt LIKE ?", like, like)
	}
	if isActivePromptOption(q.Category) {
		tx = tx.Where("category = ?", q.Category)
	}
	if isActivePromptOption(q.Domain) {
		if q.Domain == "image" {
			tx = tx.Where("domain = ? OR category IN ?", q.Domain, remotePromptCategoryCodes())
		} else {
			tx = tx.Where("domain = ?", q.Domain)
		}
	}
	if isActivePromptOption(q.Stage) {
		tx = tx.Where("stage = ?", q.Stage)
	}
	if isActivePromptOption(q.Provider) {
		tx = tx.Where("provider = ?", q.Provider)
	}
	if isActivePromptOption(q.Model) {
		tx = tx.Where("model = ?", q.Model)
	}
	if isActivePromptOption(q.Mode) {
		tx = tx.Where("mode = ?", q.Mode)
	}
	if isActivePromptOption(q.InputType) {
		tx = tx.Where("input_type = ?", q.InputType)
	}
	if isActivePromptOption(q.OutputType) {
		tx = tx.Where("output_type = ?", q.OutputType)
	}
	if isActivePromptOption(q.Status) {
		tx = tx.Where("status = ?", q.Status)
	}
	return applyPromptTagsFilter(tx, q.Tags)
}

// findPrompt 根据 ID 查询提示词。
func findPrompt(db *gorm.DB, id string) (model.Prompt, bool, error) {
	item := model.Prompt{}
	err := db.Where("id = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Prompt{}, false, nil
	}
	return item, err == nil, err
}

// applyPromptTagsFilter 应用 JSON 标签条件。
func applyPromptTagsFilter(tx *gorm.DB, tags []string) *gorm.DB {
	if len(tags) == 0 {
		return tx
	}
	for _, tag := range tags {
		tx = tx.Where(promptJSONTagsContains(tx), tag)
	}
	return tx
}

func promptTagsFromItems(items []model.Prompt) []string {
	seen := map[string]bool{}
	tags := []string{}
	for _, item := range items {
		for _, tag := range item.Tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// promptJSONTagsContains 返回提示词 tags 的 JSON 包含条件。
func promptJSONTagsContains(tx *gorm.DB) string {
	switch tx.Dialector.Name() {
	case "mysql":
		return "JSON_CONTAINS(tags, JSON_QUOTE(?))"
	case "postgres":
		return "jsonb_exists(tags::jsonb, ?)"
	default:
		return "EXISTS (SELECT 1 FROM json_each(tags) WHERE value = ?)"
	}
}

// isActivePromptOption 判断提示词筛选项有效状态。
func isActivePromptOption(value string) bool {
	return value != "" && value != "全部" && value != "all"
}

func remotePromptCategoryCodes() []string {
	result := []string{}
	for _, item := range promptCategories {
		if item.Remote {
			result = append(result, item.Category)
		}
	}
	return result
}

func remotePromptImageTaxonomy() map[string]any {
	return map[string]any{
		"domain":      "image",
		"stage":       "general",
		"provider":    "openai",
		"model":       "gpt-image-2",
		"mode":        "general",
		"input_type":  "text",
		"output_type": "image",
		"status":      "production",
	}
}
