package service

import (
	"fmt"

	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/pkg/opml"
	"go.uber.org/zap"
)

type FeedService struct {
	repo   *repository.FeedRepo
	logger *zap.Logger
}

func NewFeedService(repo *repository.FeedRepo, logger *zap.Logger) *FeedService {
	return &FeedService{repo: repo, logger: logger}
}

func (s *FeedService) Create(feed *model.Feed) error {
	return s.repo.Create(feed)
}

func (s *FeedService) GetByID(id int64) (*model.Feed, error) {
	return s.repo.GetByID(id)
}

func (s *FeedService) List(page, pageSize int) ([]model.Feed, int64, error) {
	return s.repo.List(page, pageSize)
}

func (s *FeedService) Update(feed *model.Feed) error {
	return s.repo.Update(feed)
}

func (s *FeedService) Delete(id int64) error {
	return s.repo.Delete(id)
}

func (s *FeedService) ListByQuality(page, pageSize int) ([]model.Feed, error) {
	return s.repo.ListByQuality(page, pageSize)
}

// ImportOPML parses an OPML file and creates feeds from it.
func (s *FeedService) ImportOPML(filePath string) (int, error) {
	entries, err := opml.Parse(filePath)
	if err != nil {
		return 0, fmt.Errorf("parse OPML: %w", err)
	}

	imported := 0
	for _, entry := range entries {
		feed := &model.Feed{
			Title:    entry.Title,
			URL:      entry.URL,
			SiteURL:  entry.SiteURL,
			Category: entry.Category,
			Status:   model.FeedStatusActive,
		}
		if err := s.repo.Create(feed); err != nil {
			s.logger.Warn("skip duplicate feed", zap.String("url", entry.URL), zap.Error(err))
			continue
		}
		imported++
	}

	s.logger.Info("OPML import completed", zap.Int("imported", imported), zap.Int("total", len(entries)))
	return imported, nil
}
