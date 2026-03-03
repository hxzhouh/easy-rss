package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
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
	httpClient  *http.Client
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
		httpClient:  &http.Client{Timeout: 30 * time.Second},
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

	// 2026年之前的文章不处理（跳过过旧的文章）
	cutoffDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

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

		// 跳过 2026 年之前发布的文章
		if publishedAt != nil && publishedAt.Before(cutoffDate) {
			continue
		}

		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}

		content := item.Content
		if content == "" {
			content = item.Description
		}

		// 如果内容太短，尝试抓取全文
		if len(content) < 1000 && item.Link != "" {
			fullContent, err := s.fetchArticleContent(ctx, item.Link)
			if err == nil && len(fullContent) > len(content) {
				content = fullContent
				s.logger.Debug("fetched full content",
					zap.String("title", item.Title),
					zap.Int("length", len(content)))
			}
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

// fetchArticleContent fetches the full article content from the original URL.
func (s *FetcherService) fetchArticleContent(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}

	// Try to find main content using common selectors
	selectors := []string{
		"article",
		"[role='main']",
		".post-content",
		".entry-content",
		".article-content",
		".content",
		"main",
		"#main-content",
		".post",
		".entry",
	}

	var content string
	for _, sel := range selectors {
		elem := doc.Find(sel).First()
		if elem.Length() > 0 {
			content = elem.Text()
			if len(content) > 500 {
				break
			}
		}
	}

	// Fallback to body if no content found
	if len(content) < 100 {
		content = doc.Find("body").Text()
	}

	// Clean up whitespace
	content = strings.Join(strings.Fields(content), " ")

	return content, nil
}
