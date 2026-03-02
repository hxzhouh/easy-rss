package ai_pipeline

import (
	"context"

	"github.com/hxzhouh/easy-rss/internal/model"
	"go.uber.org/zap"
)

// Stage is the interface for a single step in the AI processing pipeline.
type Stage interface {
	Name() string
	Process(ctx context.Context, article *model.Article) (proceed bool, err error)
}

// Pipeline orchestrates multiple stages for article processing.
type Pipeline struct {
	stages []Stage
	logger *zap.Logger
}

func NewPipeline(logger *zap.Logger) *Pipeline {
	return &Pipeline{logger: logger}
}

func (p *Pipeline) RegisterStage(stage Stage) {
	p.stages = append(p.stages, stage)
	p.logger.Info("registered AI pipeline stage", zap.String("stage", stage.Name()))
}

// Run processes an article through all registered stages.
// Returns the final AI status for the article.
func (p *Pipeline) Run(ctx context.Context, article *model.Article) int16 {
	for _, stage := range p.stages {
		proceed, err := stage.Process(ctx, article)
		if err != nil {
			p.logger.Error("pipeline stage error",
				zap.String("stage", stage.Name()),
				zap.Int64("article_id", article.ID),
				zap.Error(err))
			return model.AIStatusPending // retry later
		}
		if !proceed {
			p.logger.Info("article filtered out",
				zap.String("stage", stage.Name()),
				zap.Int64("article_id", article.ID))
			return model.AIStatusFilteredOut
		}
	}
	return model.AIStatusEnriched
}
