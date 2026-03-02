package service

import (
	"context"
	"sync"
	"time"

	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
)

type FetcherService struct {
	feedRepo    *repository.FeedRepo
	articleRepo *repository.ArticleRepo
	logger      *zap.Logger
	userAgent   string
	timeout     time.Duration
	maxConc     int
}

func NewFetcherService(
	feedRepo *repository.FeedRepo,
	articleRepo *repository.ArticleRepo,
	logger *zap.Logger,
	userAgent string,
	timeout time.Duration,
	maxConc int,
) *FetcherService {
	return &FetcherService{
		feedRepo:    feedRepo,
		articleRepo: articleRepo,
		logger:      logger,
		userAgent:   userAgent,
		timeout:     timeout,
		maxConc:     maxConc,
	}
}

// FetchAll fetches new articles from all active feeds.
func (s *FetcherService) FetchAll(ctx context.Context) {
	feeds, err := s.feedRepo.ListActive()
	if err != nil {
		s.logger.Error("failed to list active feeds", zap.Error(err))
		return
	}

	sem := make(chan struct{}, s.maxConc)
	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		sem <- struct{}{}
		go func(f model.Feed) {
			defer wg.Done()
			defer func() { <-sem }()
			s.fetchFeed(ctx, &f)
		}(feed)
	}

	wg.Wait()
	s.logger.Info("fetch cycle completed", zap.Int("feeds", len(feeds)))
}

// FetchOne fetches a single feed by ID (manual trigger).
func (s *FetcherService) FetchOne(ctx context.Context, feedID int64) error {
	feed, err := s.feedRepo.GetByID(feedID)
	if err != nil {
		return err
	}
	s.fetchFeed(ctx, feed)
	return nil
}

func (s *FetcherService) fetchFeed(ctx context.Context, feed *model.Feed) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	parser := gofeed.NewParser()
	parser.UserAgent = s.userAgent

	parsed, err := parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		s.logger.Warn("fetch failed", zap.String("url", feed.URL), zap.Error(err))
		feed.FetchError = err.Error()
		_ = s.feedRepo.Update(feed)
		return
	}

	now := time.Now()
	feed.LastFetched = &now
	feed.FetchError = ""
	if feed.Title == "" || feed.Title == feed.URL {
		feed.Title = parsed.Title
	}
	if feed.Description == "" && parsed.Description != "" {
		feed.Description = parsed.Description
	}
	if feed.SiteURL == "" && parsed.Link != "" {
		feed.SiteURL = parsed.Link
	}
	_ = s.feedRepo.Update(feed)

	newCount := 0
	for _, item := range parsed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		var publishedAt *time.Time
		if item.PublishedParsed != nil {
			publishedAt = item.PublishedParsed
		}

		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}

		content := item.Content
		if content == "" {
			content = item.Description
		}

		article := &model.Article{
			FeedID:      &feed.ID,
			GUID:        guid,
			Title:       item.Title,
			Link:        item.Link,
			Author:      author,
			Content:     content,
			PublishedAt: publishedAt,
			Source:      "rss",
			AIStatus:    model.AIStatusPending,
		}

		if err := s.articleRepo.Upsert(article); err != nil {
			s.logger.Warn("upsert article failed",
				zap.String("guid", guid), zap.Error(err))
			continue
		}
		newCount++
	}

	s.logger.Debug("fetched feed",
		zap.String("title", feed.Title),
		zap.Int("items", len(parsed.Items)),
		zap.Int("new", newCount))
}
