package main

import (
	"log"
	"os"
	"time"

	"github.com/hxzhouh/easy-rss/internal/config"
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/internal/service"
	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	cfg, _ := config.Load("configs/config.yaml")
	db, _ := gorm.Open(sqlite.Open(cfg.Database.Path), &gorm.Config{})
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	feedRepo := repository.NewFeedRepo(db)
	articleRepo := repository.NewArticleRepo(db)

	fetcherSvc := service.NewFetcherService(
		feedRepo, articleRepo, logger,
		cfg.Fetcher.UserAgent, cfg.Fetcher.Timeout, cfg.Fetcher.MaxConcurrent,
	)

	hnFeed, _ := feedRepo.GetByURL("https://tg.i-c-a.su/rss/hacker_news_zh")
	f, _ := os.Open("hacker_news_zh.rss")
	defer f.Close()

	parser := gofeed.NewParser()
	parsed, err := parser.Parse(f)
	if err != nil {
		log.Fatalf("Parse error: %v", err)
	}

	cutoffDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, item := range parsed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		var publishedAt *time.Time
		if item.PublishedParsed != nil {
			publishedAt = item.PublishedParsed
		}

		if publishedAt != nil && publishedAt.Before(cutoffDate) {
			continue
		}

		// Save article to DB using the updated fetcherSvc logic implicitly testable?
		// Actually, fetcherSvc.FetchByURL parses from URL. We need to parse from local file here but USE the same cleaning logic.
		// Since we want to test the `fetcher_service.go` logic, we should probably mock the HTTP response or just copy over the cleaning block.
		// Let's copy it here just for local seeding:

		content := item.Content
		if content == "" {
			content = item.Description
		}

		article := &model.Article{
			FeedID:      &hnFeed.ID,
			GUID:        guid,
			Title:       item.Title, // Will be cleaned in Upsert hook? No, fetcher does it. Let's just mock the HTTP Server.
			Link:        item.Link,
			Author:      "",
			Content:     content,
			PublishedAt: publishedAt,
			Source:      "rss",
			AIStatus:    model.AIStatusEnriched,
		}

		if err := articleRepo.Upsert(article); err != nil {
			continue
		}
	}
}
