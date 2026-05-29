package model

type AssetType string

const (
	AssetTypeText  AssetType = "text"
	AssetTypeImage AssetType = "image"
	AssetTypeVideo AssetType = "video"
)

// Asset 素材记录。
type Asset struct {
	ID           string         `json:"id" gorm:"primaryKey"`
	Title        string         `json:"title"`
	Type         AssetType      `json:"type"`
	MediaType    string         `json:"mediaType" gorm:"index"`
	Scope        string         `json:"scope" gorm:"index"`
	Category     string         `json:"category"`
	CategoryPath string         `json:"categoryPath" gorm:"index"`
	Purpose      string         `json:"purpose" gorm:"index"`
	Source       string         `json:"source" gorm:"index"`
	CoverURL     string         `json:"coverUrl"`
	Tags         []string       `json:"tags" gorm:"serializer:json"`
	Description  string         `json:"description"`
	Content      string         `json:"content,omitempty"`
	URL          string         `json:"url,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty" gorm:"serializer:json"`
	CreatedAt    string         `json:"createdAt"`
	UpdatedAt    string         `json:"updatedAt"`
}

// AssetList 素材分页结果。
type AssetList struct {
	Items    []Asset     `json:"items"`
	Tags     []string    `json:"tags"`
	FreeTags []string    `json:"freeTags"`
	Facets   AssetFacets `json:"facets"`
	Total    int         `json:"total"`
}

// AssetFacets 素材结构化筛选项。
type AssetFacets struct {
	MediaTypes    []string `json:"mediaTypes"`
	CategoryPaths []string `json:"categoryPaths"`
	Purposes      []string `json:"purposes"`
	Sources       []string `json:"sources"`
}
