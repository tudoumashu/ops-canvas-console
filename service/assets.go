package service

import (
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func ListAssets(q model.Query) (model.AssetList, error) {
	ensureTaxonomyNormalized()
	items, total, err := repository.ListAssets(q)
	if err != nil {
		return model.AssetList{}, err
	}
	for i := range items {
		items[i] = normalizeAssetTaxonomy(items[i])
	}
	tags, err := repository.ListAssetTags(q)
	if err != nil {
		return model.AssetList{}, err
	}
	facets, err := repository.ListAssetFacets(q)
	if err != nil {
		return model.AssetList{}, err
	}
	freeTags := uniqueVisibleTags(tags, map[string]bool{})
	return model.AssetList{Items: items, Tags: freeTags, FreeTags: freeTags, Facets: facets, Total: int(total)}, nil
}

func SaveAsset(item model.Asset) (model.Asset, error) {
	now := time.Now().Format(time.RFC3339)
	if item.Type == "" {
		item.Type = model.AssetTypeText
	}
	if item.ID == "" {
		item.ID = newID("asset")
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.CoverURL == "" {
		item.CoverURL = assetCoverURL(item)
	}
	item = normalizeAssetTaxonomy(item)
	return repository.SaveAsset(item)
}

func DeleteAsset(id string) error {
	return repository.DeleteAsset(id)
}

func assetCoverURL(item model.Asset) string {
	if item.CoverURL != "" {
		return item.CoverURL
	}
	if item.Type == model.AssetTypeImage {
		return item.URL
	}
	return ""
}
