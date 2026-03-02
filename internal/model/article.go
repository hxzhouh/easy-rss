package model

import "time"

// Article represents a single article/entry from an RSS feed or external import.
type Article struct {
	ID          int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	FeedID      *int64     `json:"feed_id" gorm:"index"`
	GUID        string     `json:"guid" gorm:"not null"`
	Title       string     `json:"title" gorm:"not null"`
	Link        string     `json:"link" gorm:"not null"`
	Author      string     `json:"author" gorm:"default:''"`
	Content     string     `json:"content" gorm:"type:text;default:''"`
	PublishedAt *time.Time `json:"published_at"`
	Source      string     `json:"source" gorm:"default:'rss'"` // rss / import / api
	AIStatus    int16      `json:"ai_status" gorm:"default:0;index"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Feed     *Feed     `json:"feed,omitempty" gorm:"foreignKey:FeedID"`
	AIResult *AIResult `json:"ai_result,omitempty" gorm:"foreignKey:ArticleID"`
}

const (
	AIStatusPending     int16 = 0
	AIStatusFilteredOut int16 = 1
	AIStatusPassed      int16 = 2
	AIStatusEnriched    int16 = 3
)

// UniqueIndex for (feed_id, guid) is defined via migration hook.
