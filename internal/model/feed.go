package model

import "time"

// Feed represents an RSS/Atom subscription source.
type Feed struct {
	ID           int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	Title        string     `json:"title" gorm:"not null"`
	URL          string     `json:"url" gorm:"uniqueIndex;not null"`
	SiteURL      string     `json:"site_url"`
	Description  string     `json:"description"`
	Category     string     `json:"category" gorm:"default:''"`
	ETag         string     `json:"-" gorm:"default:''"`
	LastModified string     `json:"-" gorm:"default:''"`
	LastFetched  *time.Time `json:"last_fetched"`
	FetchError   string     `json:"fetch_error" gorm:"default:''"`
	QualityScore float64    `json:"quality_score" gorm:"default:0"`
	Status       int16      `json:"status" gorm:"default:1"` // 1=active, 0=paused, -1=disabled
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Articles     []Article  `json:"articles,omitempty" gorm:"foreignKey:FeedID"`
}

const (
	FeedStatusActive   int16 = 1
	FeedStatusPaused   int16 = 0
	FeedStatusDisabled int16 = -1
)
