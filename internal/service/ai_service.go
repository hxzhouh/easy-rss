package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/internal/service/ai_pipeline"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AIService struct {
	articleRepo *repository.ArticleRepo
	filterStage ai_pipeline.Stage
	enrichStage ai_pipeline.Stage
	logger      *zap.Logger
	db          *gorm.DB
	maxConc     int
}

func NewAIService(
	articleRepo *repository.ArticleRepo,
	filterStage ai_pipeline.Stage,
	enrichStage ai_pipeline.Stage,
	logger *zap.Logger,
	db *gorm.DB,
	maxConc int,
) *AIService {
	return &AIService{
		articleRepo: articleRepo,
		filterStage: filterStage,
		enrichStage: enrichStage,
		logger:      logger,
		db:          db,
		maxConc:     maxConc,
	}
}

// ProcessFilter picks up pending(0) articles and runs Stage 1 (filter).
// Articles pass → passed(2), articles fail → filtered_out(1).
func (s *AIService) ProcessFilter(ctx context.Context) {
	articles, err := s.articleRepo.ListByStatus(model.AIStatusPending, s.maxConc*2)
	if err != nil {
		s.logger.Error("failed to list pending articles for filter", zap.Error(err))
		return
	}
	if len(articles) == 0 {
		return
	}

	s.processBatch(ctx, articles, s.filterStage, model.AIStatusPassed, "filter")
}

// ProcessEnrich picks up passed(2) articles and runs Stage 2 (enrich).
// Articles complete → enriched(3).
func (s *AIService) ProcessEnrich(ctx context.Context) {
	articles, err := s.articleRepo.ListByStatus(model.AIStatusPassed, s.maxConc*2)
	if err != nil {
		s.logger.Error("failed to list passed articles for enrich", zap.Error(err))
		return
	}
	if len(articles) == 0 {
		return
	}

	s.processBatch(ctx, articles, s.enrichStage, model.AIStatusEnriched, "enrich")
}

func (s *AIService) processBatch(ctx context.Context, articles []model.Article, stage ai_pipeline.Stage, successStatus int16, stageName string) {
	sem := make(chan struct{}, s.maxConc)
	var wg sync.WaitGroup

	for _, article := range articles {
		wg.Add(1)
		sem <- struct{}{}
		go func(a model.Article) {
			defer wg.Done()
			defer func() { <-sem }()
			s.processStage(ctx, &a, stage, successStatus)
		}(article)
	}

	wg.Wait()
	s.logger.Info("AI stage cycle completed",
		zap.String("stage", stageName),
		zap.Int("processed", len(articles)))
}

func (s *AIService) processStage(ctx context.Context, article *model.Article, stage ai_pipeline.Stage, successStatus int16) {
	proceed, err := stage.Process(ctx, article)
	if err != nil {
		s.logger.Error("AI stage error",
			zap.String("stage", stage.Name()),
			zap.Int64("article_id", article.ID),
			zap.Error(err))
		return // leave as current status, retry next cycle
	}

	finalStatus := successStatus
	if !proceed {
		finalStatus = model.AIStatusFilteredOut
		s.logger.Info("article filtered out",
			zap.String("stage", stage.Name()),
			zap.Int64("article_id", article.ID))
	}

	// Save AI result and update status in a transaction.
	// Use UPSERT so reprocessing never hits the UNIQUE constraint on article_id.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if article.AIResult != nil {
			article.AIResult.ProcessedAt = time.Now()

			// DB-agnostic manual upsert to avoid dialect-specific ON CONFLICT issues
			var existing model.AIResult
			err := tx.Where("article_id = ?", article.ID).First(&existing).Error
			if err == nil {
				// Record exists, update it
				article.AIResult.ID = existing.ID
				if err := tx.Save(article.AIResult).Error; err != nil {
					return err
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				// Record doesn't exist, insert new
				if err := tx.Create(article.AIResult).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return tx.Model(&model.Article{}).
			Where("id = ?", article.ID).
			Update("ai_status", finalStatus).Error
	})

	if err != nil {
		s.logger.Error("failed to save AI result",
			zap.Int64("article_id", article.ID), zap.Error(err))
	}
}

// ReprocessArticle resets an article back to pending and lets the normal pipeline pick it up.
func (s *AIService) ReprocessArticle(ctx context.Context, articleID int64) error {
	article, err := s.articleRepo.GetByID(articleID)
	if err != nil {
		return err
	}

	// Delete existing AI result
	s.db.Where("article_id = ?", articleID).Delete(&model.AIResult{})

	// Reset to pending — filter cron will pick it up
	return s.db.Model(&model.Article{}).
		Where("id = ?", article.ID).
		Update("ai_status", model.AIStatusPending).Error
}
