package repository

import (
	"github.com/hxzhouh/easy-rss/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ArticleRepo struct {
	db *gorm.DB
}

func NewArticleRepo(db *gorm.DB) *ArticleRepo {
	return &ArticleRepo{db: db}
}

// Upsert inserts a new article or skips if (feed_id, guid) already exists.
func (r *ArticleRepo) Upsert(article *model.Article) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "feed_id"}, {Name: "guid"}},
		DoNothing: true,
	}).Create(article).Error
}

func (r *ArticleRepo) Create(article *model.Article) error {
	return r.db.Create(article).Error
}

func (r *ArticleRepo) GetByGUID(feedID *int64, guid string) (*model.Article, error) {
	var article model.Article
	q := r.db
	if feedID != nil {
		q = q.Where("feed_id = ?", *feedID)
	}
	err := q.Where("guid = ?", guid).First(&article).Error
	return &article, err
}

func (r *ArticleRepo) SaveAIResult(aiResult *model.AIResult) error {
	return r.db.Save(aiResult).Error
}

func (r *ArticleRepo) GetByID(id int64) (*model.Article, error) {
	var article model.Article
	err := r.db.Preload("AIResult").Preload("Feed").First(&article, id).Error
	return &article, err
}

func (r *ArticleRepo) List(page, pageSize int, feedID *int64, aiStatus *int16, excludeFeedID *int64) ([]model.Article, int64, error) {
	var articles []model.Article
	var total int64

	q := r.db.Model(&model.Article{})
	if feedID != nil {
		q = q.Where("feed_id = ?", *feedID)
	}
	if excludeFeedID != nil {
		q = q.Where("feed_id != ?", *excludeFeedID)
	}
	if aiStatus != nil {
		q = q.Where("ai_status = ?", *aiStatus)
	}

	q.Count(&total)
	err := q.Preload("AIResult").Preload("Feed").
		Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&articles).Error

	return articles, total, err
}

func (r *ArticleRepo) Delete(id int64) error {
	return r.db.Delete(&model.Article{}, id).Error
}

func (r *ArticleRepo) UpdateAIStatus(id int64, status int16) error {
	return r.db.Model(&model.Article{}).Where("id = ?", id).Update("ai_status", status).Error
}

// ListByStatus returns articles with the given ai_status, limited by count.
func (r *ArticleRepo) ListByStatus(status int16, limit int) ([]model.Article, error) {
	var articles []model.Article
	err := r.db.Where("ai_status = ?", status).
		Order("created_at ASC").
		Limit(limit).
		Find(&articles).Error
	return articles, err
}

// CountByFeedAndStatus returns the count of articles for a feed with a given AI status.
func (r *ArticleRepo) CountByFeedAndStatus(feedID int64, status int16) (int64, error) {
	var count int64
	err := r.db.Model(&model.Article{}).
		Where("feed_id = ? AND ai_status = ?", feedID, status).
		Count(&count).Error
	return count, err
}

// CountByFeed returns total article count for a feed.
func (r *ArticleRepo) CountByFeed(feedID int64) (int64, error) {
	var count int64
	err := r.db.Model(&model.Article{}).Where("feed_id = ?", feedID).Count(&count).Error
	return count, err
}

// AvgQualityScoreByFeed returns the average quality score for enriched articles of a feed.
func (r *ArticleRepo) AvgQualityScoreByFeed(feedID int64) (float64, error) {
	var result struct {
		Avg float64
	}
	err := r.db.Model(&model.AIResult{}).
		Joins("JOIN articles ON articles.id = ai_results.article_id").
		Where("articles.feed_id = ? AND articles.ai_status = ?", feedID, model.AIStatusEnriched).
		Select("COALESCE(AVG(ai_results.quality_score), 0) as avg").
		Scan(&result).Error
	return result.Avg, err
}
