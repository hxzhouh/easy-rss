package service

import (
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"go.uber.org/zap"
)

// QualityService evaluates feed quality scores.
type QualityService struct {
	feedRepo    *repository.FeedRepo
	articleRepo *repository.ArticleRepo
	logger      *zap.Logger
	minArticles int
	threshold   float64
}

func NewQualityService(
	feedRepo *repository.FeedRepo,
	articleRepo *repository.ArticleRepo,
	logger *zap.Logger,
	minArticles int,
	threshold float64,
) *QualityService {
	return &QualityService{
		feedRepo:    feedRepo,
		articleRepo: articleRepo,
		logger:      logger,
		minArticles: minArticles,
		threshold:   threshold,
	}
}

// EvaluateAll recalculates quality scores for all active feeds.
func (s *QualityService) EvaluateAll() {
	feeds, err := s.feedRepo.ListActive()
	if err != nil {
		s.logger.Error("failed to list feeds for quality evaluation", zap.Error(err))
		return
	}

	for _, feed := range feeds {
		s.evaluateFeed(feed)
	}

	// Auto-disable low quality feeds
	if err := s.feedRepo.DisableLowQuality(s.threshold, s.minArticles); err != nil {
		s.logger.Error("failed to disable low quality feeds", zap.Error(err))
	}

	s.logger.Info("quality evaluation completed", zap.Int("feeds_evaluated", len(feeds)))
}

func (s *QualityService) evaluateFeed(feed model.Feed) {
	total, err := s.articleRepo.CountByFeed(feed.ID)
	if err != nil || total < int64(s.minArticles) {
		return
	}

	// Pass rate (40%)
	enriched, _ := s.articleRepo.CountByFeedAndStatus(feed.ID, model.AIStatusEnriched)
	passed, _ := s.articleRepo.CountByFeedAndStatus(feed.ID, model.AIStatusPassed)
	passedTotal := enriched + passed
	passRate := float64(passedTotal) / float64(total) * 100.0

	// Average quality score (30%)
	avgScore, _ := s.articleRepo.AvgQualityScoreByFeed(feed.ID)

	// Update frequency score (15%) - simplified: based on total article count
	freqScore := float64(total)
	if freqScore > 100 {
		freqScore = 100
	}

	// Fetch success rate (15%) - simplified: based on whether last fetch had error
	fetchScore := 100.0
	if feed.FetchError != "" {
		fetchScore = 0
	}

	quality := passRate*0.4 + avgScore*0.3 + freqScore*0.15 + fetchScore*0.15

	if err := s.feedRepo.UpdateQualityScore(feed.ID, quality); err != nil {
		s.logger.Error("failed to update quality score",
			zap.Int64("feed_id", feed.ID), zap.Error(err))
	}
}
