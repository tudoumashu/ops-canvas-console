package model

// Prompt 提示词记录。
type Prompt struct {
	ID         string         `json:"id" gorm:"primaryKey"`
	Title      string         `json:"title"`
	CoverURL   string         `json:"coverUrl"`
	Prompt     string         `json:"prompt"`
	Tags       []string       `json:"tags" gorm:"serializer:json"`
	Category   string         `json:"category" gorm:"index"`
	Domain     string         `json:"domain" gorm:"index"`
	Stage      string         `json:"stage" gorm:"index"`
	Provider   string         `json:"provider" gorm:"index"`
	Model      string         `json:"model" gorm:"index"`
	Mode       string         `json:"mode" gorm:"index"`
	InputType  string         `json:"inputType" gorm:"index"`
	OutputType string         `json:"outputType" gorm:"index"`
	Status     string         `json:"status" gorm:"index"`
	Metadata   map[string]any `json:"metadata,omitempty" gorm:"serializer:json"`
	GithubURL  string         `json:"githubUrl" gorm:"-"`
	Preview    string         `json:"preview"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
}

// PromptList 提示词分页结果。
type PromptList struct {
	Items      []Prompt     `json:"items"`
	Tags       []string     `json:"tags"`
	FreeTags   []string     `json:"freeTags"`
	Categories []string     `json:"categories"`
	Facets     PromptFacets `json:"facets"`
	Total      int          `json:"total"`
}

// PromptCategory 提示词分类。
type PromptCategory struct {
	Category    string `json:"category" gorm:"primaryKey"`
	Name        string `json:"name"`
	Description string `json:"description"`
	GithubURL   string `json:"githubUrl"`
	Remote      bool   `json:"remote"`
	UpdatedAt   string `json:"updatedAt"`
}

// PromptFacets 提示词结构化筛选项。
type PromptFacets struct {
	Categories  []string `json:"categories"`
	Domains     []string `json:"domains"`
	Stages      []string `json:"stages"`
	Providers   []string `json:"providers"`
	Models      []string `json:"models"`
	Modes       []string `json:"modes"`
	InputTypes  []string `json:"inputTypes"`
	OutputTypes []string `json:"outputTypes"`
	Statuses    []string `json:"statuses"`
}
