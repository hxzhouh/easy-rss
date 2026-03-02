package model

import "time"

// StageConfig stores per-stage AI model configuration, managed via admin API.
type StageConfig struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	StageName string    `json:"stage_name" gorm:"uniqueIndex;not null"` // e.g. "filter", "enrich"
	Provider  string    `json:"provider" gorm:"default:'openai'"`       // openai / deepseek / ...
	BaseURL   string    `json:"base_url" gorm:"not null"`
	APIKey    string    `json:"api_key" gorm:"not null"`
	Model     string    `json:"model" gorm:"not null"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	Prompt    string    `json:"prompt" gorm:"type:text"` // Custom system prompt, if empty uses default
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
