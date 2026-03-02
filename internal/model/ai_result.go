package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// StringSlice is a JSON-serialized string slice that works with both PostgreSQL and SQLite.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *StringSlice) Scan(src interface{}) error {
	if src == nil {
		*s = StringSlice{}
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("unsupported type for StringSlice: %T", src)
	}
	return json.Unmarshal(data, s)
}

// AIResult stores the AI processing output for an article.
type AIResult struct {
	ID                  int64       `json:"id" gorm:"primaryKey;autoIncrement"`
	ArticleID           int64       `json:"article_id" gorm:"uniqueIndex;not null"`
	IsAd                bool        `json:"is_ad" gorm:"default:false"`
	IsMeaningless       bool        `json:"is_meaningless" gorm:"default:false"`
	FilterReason        string      `json:"filter_reason" gorm:"default:''"`
	QualityScore        float64     `json:"quality_score" gorm:"default:0"`
	Summary             string      `json:"summary" gorm:"type:text;default:''"`
	SummaryZh           string      `json:"summary_zh" gorm:"type:text;default:''"`
	TranslatedTitle     string      `json:"translated_title" gorm:"default:''"`
	TranslatedContent   string      `json:"translated_content" gorm:"type:text;default:''"`
	Tags                StringSlice `json:"tags" gorm:"type:text"`
	ProcessedAt         time.Time   `json:"processed_at" gorm:"autoCreateTime"`
}
