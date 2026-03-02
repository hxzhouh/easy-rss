package ai_pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/pkg/aiutil"
	"go.uber.org/zap"
)

const enrichSystemPrompt = `You are a content analyst. Analyze the given article and provide:
1. A quality score from 0 to 100 (based on depth, originality, clarity, and informativeness)
2. A concise summary in English (2-3 sentences)
3. A concise summary translated to Chinese (2-3 sentences)
4. The article title translated to Chinese
5. Up to 5 relevant tags/keywords

Respond ONLY with valid JSON in the following format, no other text:
{
  "quality_score": 85.0,
  "summary": "English summary here.",
  "summary_zh": "中文摘要。",
  "translated_title": "中文标题",
  "tags": ["tag1", "tag2"]
}`

// EnrichStage is Stage 2: scores, summarizes, and translates articles.
type EnrichStage struct {
	defaultClient   *aiutil.Client
	stageConfigRepo *repository.StageConfigRepo
	logger          *zap.Logger
	aiTimeout       interface{ Minutes() float64 }
}

type enrichResponse struct {
	QualityScore    float64  `json:"quality_score"`
	Summary         string   `json:"summary"`
	SummaryZh       string   `json:"summary_zh"`
	TranslatedTitle string   `json:"translated_title"`
	Tags            []string `json:"tags"`
}

func NewEnrichStage(
	defaultClient *aiutil.Client,
	stageConfigRepo *repository.StageConfigRepo,
	logger *zap.Logger,
	aiTimeout interface{ Minutes() float64 },
) *EnrichStage {
	return &EnrichStage{
		defaultClient:   defaultClient,
		stageConfigRepo: stageConfigRepo,
		logger:          logger,
		aiTimeout:       aiTimeout,
	}
}

func (s *EnrichStage) Name() string {
	return "enrich"
}

// getClient returns a per-stage AI client and prompt if configured, otherwise the default.
func (s *EnrichStage) getClient() (*aiutil.Client, string) {
	cfg, err := s.stageConfigRepo.GetByStageName("enrich")
	if err != nil || !cfg.Enabled {
		return s.defaultClient, enrichSystemPrompt
	}
	prompt := cfg.Prompt
	if prompt == "" {
		prompt = enrichSystemPrompt
	}
	return aiutil.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model, durationFromMinutes(s.aiTimeout)), prompt
}

func (s *EnrichStage) Process(ctx context.Context, article *model.Article) (bool, error) {
	content := article.Content
	if len(content) > 6000 {
		content = content[:6000]
	}

	userMsg := fmt.Sprintf("Title: %s\n\nContent:\n%s", article.Title, content)

	client, prompt := s.getClient()
	reply, err := client.Chat(ctx, prompt, userMsg)
	if err != nil {
		return false, fmt.Errorf("AI enrich call: %w", err)
	}

	reply = extractJSON(reply)

	var resp enrichResponse
	if err := json.Unmarshal([]byte(reply), &resp); err != nil {
		s.logger.Warn("failed to parse enrich response", zap.String("reply", reply), zap.Error(err))
		return true, nil
	}

	// Merge enrich results into the AI result
	if article.AIResult == nil {
		article.AIResult = &model.AIResult{ArticleID: article.ID}
	}
	article.AIResult.QualityScore = resp.QualityScore
	article.AIResult.Summary = resp.Summary
	article.AIResult.SummaryZh = resp.SummaryZh
	article.AIResult.TranslatedTitle = resp.TranslatedTitle
	article.AIResult.Tags = resp.Tags

	return true, nil
}
