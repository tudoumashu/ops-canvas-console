package service

import (
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func ListPrompts(q model.Query) (model.PromptList, error) {
	ensureTaxonomyNormalized()
	items, total, err := repository.ListPrompts(q)
	if err != nil {
		return model.PromptList{}, err
	}
	for i := range items {
		items[i] = normalizePromptTaxonomy(items[i])
	}
	tags, err := repository.ListPromptTags(q)
	if err != nil {
		return model.PromptList{}, err
	}
	facets, err := repository.ListPromptFacets(q)
	if err != nil {
		return model.PromptList{}, err
	}
	categories := promptCategoryCodes(ListPromptCategories())
	freeTags := uniqueVisibleTags(tags, map[string]bool{})
	return model.PromptList{Items: items, Tags: freeTags, FreeTags: freeTags, Categories: categories, Facets: facets, Total: int(total)}, nil
}

func ListPromptCategories() []model.PromptCategory {
	categories, _ := repository.ListPromptCategories()
	return categories
}

func SavePrompt(item model.Prompt) (model.Prompt, error) {
	now := time.Now().Format(time.RFC3339)
	if item.Category == "" {
		item.Category = repository.PromptCategories()[0].Category
	}
	if item.ID == "" {
		item.ID = newID(item.Category)
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	category, ok := repository.PromptCategoryByCode(item.Category)
	if !ok {
		category = repository.PromptCategories()[0]
		item.Category = category.Category
	}
	item.GithubURL = ""
	item = normalizePromptTaxonomy(item)
	return repository.SavePrompt(item)
}

func DeletePrompt(id string) error {
	return repository.DeletePrompt(id)
}

func DeletePrompts(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return repository.DeletePrompts(ids)
}

func RebuildManagedPromptLibrary() ([]model.PromptCategory, error) {
	if err := repository.DeleteManagedPromptLibraryPrompts(); err != nil {
		return nil, err
	}
	for _, category := range repository.PromptCategories() {
		if !category.Remote {
			continue
		}
		if _, err := SyncPromptCategory(category.Category); err != nil {
			return nil, err
		}
	}
	normalizeTaxonomyMu.Lock()
	normalizeTaxonomyDone = false
	normalizeTaxonomyMu.Unlock()
	ensureTaxonomyNormalized()
	return repository.ListPromptCategories()
}

func promptCategoryCodes(items []model.PromptCategory) []string {
	codes := []string{}
	for _, item := range items {
		if item.Category != "" {
			codes = append(codes, item.Category)
		}
	}
	return codes
}
