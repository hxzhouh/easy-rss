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
}

func New(
	logger *zap.Logger,
	fetcherSvc *service.FetcherService,
	aiSvc *service.AIService,
	qualitySvc *service.QualityService,
) *Scheduler {
	return &Scheduler{
		cron:       cron.New(),
		logger:     logger,
		fetcherSvc: fetcherSvc,
		aiSvc:      aiSvc,
		qualitySvc: qualitySvc,
	}
}

// CronIntervals holds the cron expressions for each scheduled task.
type CronIntervals struct {
	Fetch   string
	Filter  string
	Enrich  string
	Quality string
}

func (s *Scheduler) Start(intervals CronIntervals) {
	_, err := s.cron.AddFunc(intervals.Fetch, func() {
		s.logger.Info("starting scheduled RSS fetch")
		s.fetcherSvc.FetchAll(context.Background())
	})
	if err != nil {
		s.logger.Fatal("failed to schedule RSS fetch", zap.Error(err))
	}

	_, err = s.cron.AddFunc(intervals.Filter, func() {
		s.logger.Info("starting scheduled AI filter (Stage 1)")
		s.aiSvc.ProcessFilter(context.Background())
	})
	if err != nil {
		s.logger.Fatal("failed to schedule AI filter", zap.Error(err))
	}

	_, err = s.cron.AddFunc(intervals.Enrich, func() {
		s.logger.Info("starting scheduled AI enrich (Stage 2)")
		s.aiSvc.ProcessEnrich(context.Background())
	})
	if err != nil {
		s.logger.Fatal("failed to schedule AI enrich", zap.Error(err))
	}

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
		zap.String("filter", intervals.Filter),
		zap.String("enrich", intervals.Enrich),
		zap.String("quality", intervals.Quality))
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info("scheduler stopped")
}
