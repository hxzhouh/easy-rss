package scheduler

import (
	"context"

	"github.com/hxzhouh/easy-rss/internal/service"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type Scheduler struct {
	cron       *cron.Cron
	logger     *zap.Logger
	fetcherSvc *service.FetcherService
	aiSvc      *service.AIService
	qualitySvc *service.QualityService
	ctx        context.Context
	cancel     context.CancelFunc
}

func New(
	logger *zap.Logger,
	fetcherSvc *service.FetcherService,
	aiSvc *service.AIService,
	qualitySvc *service.QualityService,
) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:       cron.New(),
		logger:     logger,
		fetcherSvc: fetcherSvc,
		aiSvc:      aiSvc,
		qualitySvc: qualitySvc,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// CronIntervals holds the cron expressions for each scheduled task.
type CronIntervals struct {
	Fetch           string
	FetchHackerNews string
	Enrich          string
	Quality         string
}

func (s *Scheduler) Start(intervals CronIntervals) {
	// RSS Fetch (cron scheduled)
	_, err := s.cron.AddFunc(intervals.Fetch, func() {
		s.logger.Info("starting scheduled RSS fetch")
		s.fetcherSvc.FetchAll(context.Background())
	})
	if err != nil {
		s.logger.Fatal("failed to schedule RSS fetch", zap.Error(err))
	}

	// HackerNews Fetch (cron scheduled)
	if intervals.FetchHackerNews != "" {
		_, err = s.cron.AddFunc(intervals.FetchHackerNews, func() {
			s.logger.Info("starting scheduled HackerNews fetch")
			s.fetcherSvc.FetchByURL(context.Background(), "https://tg.i-c-a.su/rss/hacker_news_zh")
		})
		if err != nil {
			s.logger.Fatal("failed to schedule HackerNews fetch", zap.Error(err))
		}
	}

	// AI Enrich (cron scheduled)
	_, err = s.cron.AddFunc(intervals.Enrich, func() {
		s.logger.Info("starting scheduled AI enrich (Stage 2)")
		s.aiSvc.ProcessEnrich(context.Background())
	})
	if err != nil {
		s.logger.Fatal("failed to schedule AI enrich", zap.Error(err))
	}

	// Quality evaluation (cron scheduled)
	_, err = s.cron.AddFunc(intervals.Quality, func() {
		s.logger.Info("starting scheduled quality evaluation")
		s.qualitySvc.EvaluateAll()
	})
	if err != nil {
		s.logger.Fatal("failed to schedule quality evaluation", zap.Error(err))
	}

	s.cron.Start()
	s.logger.Info("scheduler started",
		zap.String("fetch", intervals.Fetch),
		zap.String("enrich", intervals.Enrich),
		zap.String("quality", intervals.Quality))
}

func (s *Scheduler) Stop() {
	s.cancel()
	s.cron.Stop()
	s.logger.Info("scheduler stopped")
}
