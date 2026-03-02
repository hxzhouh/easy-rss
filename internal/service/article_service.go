package service

import (
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"go.uber.org/zap"
)

type ArticleService struct {
	repo   *repository.ArticleRepo
	logger *zap.Logger
}

func NewArticleService(repo *repository.ArticleRepo, logger *zap.Logger) *ArticleService {
	return &ArticleService{repo: repo, logger: logger}
}

func (s *ArticleService) GetByID(id int64) (*model.Article, error) {
	return s.repo.GetByID(id)
}

func (s *ArticleService) List(page, pageSize int, feedID *int64, aiStatus *int16) ([]model.Article, int64, error) {
	return s.repo.List(page, pageSize, feedID, aiStatus)
}

func (s *ArticleService) Delete(id int64) error {
	return s.repo.Delete(id)
}

// ImportArticle represents a single article from an external source.
type ImportArticle struct {
	Title   string `json:"title" binding:"required"`
	Link    string `json:"link" binding:"required"`
	Content string `json:"content"`
	Author  string `json:"author"`
	Source  string `json:"source"`
}

// ImportBatch creates articles from external sources.
func (s *ArticleService) ImportBatch(articles []ImportArticle) (int, error) {
	imported := 0
	for _, a := range articles {
		source := a.Source
		if source == "" {
			source = "import"
		}
		article := &model.Article{
			GUID:     a.Link, // Use link as GUID for imported articles
			Title:    a.Title,
			Link:     a.Link,
			Author:   a.Author,
			Content:  a.Content,
			Source:   source,
			AIStatus: model.AIStatusPending,
		}
		if err := s.repo.Create(article); err != nil {
			s.logger.Warn("import article failed", zap.String("link", a.Link), zap.Error(err))
			continue
		}
		imported++
	}
	return imported, nil
}
