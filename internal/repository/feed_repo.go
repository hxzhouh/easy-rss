package repository

import (
	"github.com/hxzhouh/easy-rss/internal/model"
	"gorm.io/gorm"
)

type FeedRepo struct {
	db *gorm.DB
}

func NewFeedRepo(db *gorm.DB) *FeedRepo {
	return &FeedRepo{db: db}
}

func (r *FeedRepo) Create(feed *model.Feed) error {
	return r.db.Create(feed).Error
}

func (r *FeedRepo) GetByID(id int64) (*model.Feed, error) {
	var feed model.Feed
	err := r.db.First(&feed, id).Error
	return &feed, err
}

func (r *FeedRepo) GetByURL(url string) (*model.Feed, error) {
	var feed model.Feed
	err := r.db.Where("url = ?", url).First(&feed).Error
	return &feed, err
}

func (r *FeedRepo) List(page, pageSize int) ([]model.Feed, int64, error) {
	var feeds []model.Feed
	var total int64
	r.db.Model(&model.Feed{}).Count(&total)
	err := r.db.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&feeds).Error
	return feeds, total, err
}

func (r *FeedRepo) ListActive() ([]model.Feed, error) {
	var feeds []model.Feed
	err := r.db.Where("status = ?", model.FeedStatusActive).Find(&feeds).Error
	return feeds, err
}

func (r *FeedRepo) Update(feed *model.Feed) error {
	return r.db.Save(feed).Error
}

func (r *FeedRepo) Delete(id int64) error {
	return r.db.Delete(&model.Feed{}, id).Error
}

func (r *FeedRepo) UpdateQualityScore(id int64, score float64) error {
	return r.db.Model(&model.Feed{}).Where("id = ?", id).Update("quality_score", score).Error
}

func (r *FeedRepo) ListByQuality(page, pageSize int) ([]model.Feed, error) {
	var feeds []model.Feed
	err := r.db.Order("quality_score DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&feeds).Error
	return feeds, err
}

func (r *FeedRepo) DisableLowQuality(threshold float64, minArticles int) error {
	subQuery := r.db.Model(&model.Article{}).
		Select("feed_id").
		Group("feed_id").
		Having("COUNT(*) >= ?", minArticles)

	return r.db.Model(&model.Feed{}).
		Where("quality_score < ? AND status = ? AND id IN (?)", threshold, model.FeedStatusActive, subQuery).
		Update("status", model.FeedStatusDisabled).Error
}
