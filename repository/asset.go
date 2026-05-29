package repository

import (
	"errors"
	"sort"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

// ListAssets 按查询条件返回素材分页列表。
func ListAssets(q model.Query) ([]model.Asset, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := applyAssetFilters(db.Model(&model.Asset{}), q)

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []model.Asset
	err = tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

// ListAssetTags 返回当前素材查询条件下的全部标签。
func ListAssetTags(q model.Query) ([]string, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	q.Normalize()
	q.Tags = nil
	tx := applyAssetFilters(db.Model(&model.Asset{}), q)

	var items []model.Asset
	if err := tx.Select("tags").Find(&items).Error; err != nil {
		return nil, err
	}
	return assetTagsFromItems(items), nil
}

// ListAssetFacets 返回当前素材查询条件下的结构化筛选项。
func ListAssetFacets(q model.Query) (model.AssetFacets, error) {
	db, err := DB()
	if err != nil {
		return model.AssetFacets{}, err
	}
	q.Normalize()
	q.CategoryPath = ""
	q.Purpose = ""
	q.Source = ""
	q.Tags = nil
	tx := applyAssetFilters(db.Model(&model.Asset{}), q)
	var items []model.Asset
	if err := tx.Select("type", "media_type", "category_path", "purpose", "source").Find(&items).Error; err != nil {
		return model.AssetFacets{}, err
	}
	facets := model.AssetFacets{}
	mediaTypes := map[string]bool{}
	categoryPaths := map[string]bool{}
	purposes := map[string]bool{}
	sources := map[string]bool{}
	for _, item := range items {
		if item.MediaType != "" {
			mediaTypes[item.MediaType] = true
		} else if item.Type != "" {
			mediaTypes[string(item.Type)] = true
		}
		if item.CategoryPath != "" {
			categoryPaths[item.CategoryPath] = true
		} else if item.Category != "" {
			categoryPaths[item.Category] = true
		}
		if item.Purpose != "" {
			purposes[item.Purpose] = true
		}
		if item.Source != "" {
			sources[item.Source] = true
		}
	}
	facets.MediaTypes = sortedKeys(mediaTypes)
	facets.CategoryPaths = sortedKeys(categoryPaths)
	facets.Purposes = sortedKeys(purposes)
	facets.Sources = sortedKeys(sources)
	return facets, nil
}

// ListAssetsNeedingTaxonomy 返回仍缺少结构化分类字段的素材。
func ListAssetsNeedingTaxonomy(limit int) ([]model.Asset, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > model.MaxPageSize {
		limit = model.MaxPageSize
	}
	var items []model.Asset
	err = db.Where(
		`media_type = '' OR media_type IS NULL OR
		scope = '' OR scope IS NULL OR
		category_path = '' OR category_path IS NULL OR
		purpose = '' OR purpose IS NULL OR
		source = '' OR source IS NULL OR
		purpose = ? OR
		category_path = ? OR category = ? OR
		source IN ?`,
		"mockup_base",
		"Mockup底版",
		"Mockup底版",
		[]string{"uploaded", "imported", "admin_created", "generated"},
	).Order("id asc").Limit(limit).Find(&items).Error
	return items, err
}

func GetAsset(id string) (model.Asset, error) {
	db, err := DB()
	if err != nil {
		return model.Asset{}, err
	}
	item, ok, err := findAsset(db, id)
	if err != nil {
		return model.Asset{}, err
	}
	if !ok {
		return model.Asset{}, gorm.ErrRecordNotFound
	}
	return item, nil
}

// SaveAsset 保存素材，并在更新时保留原创建时间。
func SaveAsset(item model.Asset) (model.Asset, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	if saved, ok, err := findAsset(db, item.ID); err != nil {
		return item, err
	} else if ok && item.CreatedAt == "" {
		item.CreatedAt = saved.CreatedAt
	} else if !ok && item.CreatedAt == "" {
		item.CreatedAt = item.UpdatedAt
	}
	return item, db.Save(&item).Error
}

// DeleteAsset 删除指定素材。
func DeleteAsset(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.Asset{}, "id = ?", id).Error
}

// applyAssetFilters 应用素材列表的搜索条件。
func applyAssetFilters(tx *gorm.DB, q model.Query) *gorm.DB {
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("title LIKE ? OR description LIKE ? OR content LIKE ?", like, like, like)
	}
	if isActiveAssetOption(q.Type) {
		tx = tx.Where("type = ?", q.Type)
	}
	if isActiveAssetOption(q.MediaType) {
		tx = tx.Where("media_type = ? OR type = ?", q.MediaType, q.MediaType)
	}
	if isActiveAssetOption(q.Scope) {
		tx = tx.Where("scope = ?", q.Scope)
	}
	if q.Category != "" && q.Category != "all" && q.Category != "全部" {
		tx = tx.Where("category = ?", q.Category)
	}
	if isActiveAssetOption(q.CategoryPath) {
		tx = tx.Where("category_path = ? OR category = ?", q.CategoryPath, q.CategoryPath)
	}
	if isActiveAssetOption(q.Purpose) {
		tx = tx.Where("purpose = ?", q.Purpose)
	}
	if isActiveAssetOption(q.Source) {
		tx = tx.Where("source = ?", q.Source)
	}
	return applyAssetTagsFilter(tx, q.Tags)
}

// findAsset 根据 ID 查询素材。
func findAsset(db *gorm.DB, id string) (model.Asset, bool, error) {
	item := model.Asset{}
	err := db.Where("id = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Asset{}, false, nil
	}
	return item, err == nil, err
}

// applyAssetTagsFilter 应用 JSON 标签条件。
func applyAssetTagsFilter(tx *gorm.DB, tags []string) *gorm.DB {
	if len(tags) == 0 {
		return tx
	}
	for _, tag := range tags {
		tx = tx.Where(assetJSONTagsContains(tx), tag)
	}
	return tx
}

func assetTagsFromItems(items []model.Asset) []string {
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

// assetJSONTagsContains 返回素材 tags 的 JSON 包含条件。
func assetJSONTagsContains(tx *gorm.DB) string {
	switch tx.Dialector.Name() {
	case "mysql":
		return "JSON_CONTAINS(tags, JSON_QUOTE(?))"
	case "postgres":
		return "jsonb_exists(tags::jsonb, ?)"
	default:
		return "EXISTS (SELECT 1 FROM json_each(tags) WHERE value = ?)"
	}
}

// isActiveAssetOption 判断素材筛选项有效状态。
func isActiveAssetOption(value string) bool {
	return value != "" && value != "全部" && value != "all"
}

func sortedKeys(items map[string]bool) []string {
	result := []string{}
	for item := range items {
		if item != "" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}
