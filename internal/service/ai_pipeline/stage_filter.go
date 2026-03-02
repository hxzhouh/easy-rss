package ai_pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/pkg/aiutil"
	"go.uber.org/zap"
)

const filterSystemPrompt = `You are a content quality filter. Analyze the given article title and content to determine if it is:
1. An advertisement or promotional content
2. Meaningless content (spam, gibberish, clickbait with no substance)

Respond ONLY with valid JSON in the following format, no other text:
{"is_ad": false, "is_meaningless": false, "reason": ""}

Set is_ad to true if the content is primarily advertising/promotional.
Set is_meaningless to true if the content has no informational value.
If filtered, provide a brief reason.`

// FilterStage is Stage 1: filters out ads and meaningless content.
type FilterStage struct {
	defaultClient   *aiutil.Client
	stageConfigRepo *repository.StageConfigRepo
	articleRepo     *repository.ArticleRepo
	logger          *zap.Logger
	aiTimeout       interface{ Minutes() float64 }
}

type filterResponse struct {
	IsAd          bool   `json:"is_ad"`
	IsMeaningless bool   `json:"is_meaningless"`
	Reason        string `json:"reason"`
}

func NewFilterStage(
	defaultClient *aiutil.Client,
	stageConfigRepo *repository.StageConfigRepo,
	articleRepo *repository.ArticleRepo,
	logger *zap.Logger,
	aiTimeout interface{ Minutes() float64 },
) *FilterStage {
	return &FilterStage{
		defaultClient:   defaultClient,
		stageConfigRepo: stageConfigRepo,
		articleRepo:     articleRepo,
		logger:          logger,
		aiTimeout:       aiTimeout,
	}
}

func (s *FilterStage) Name() string {
	return "filter"
}

// getClient returns a per-stage AI client and prompt if configured, otherwise the default.
func (s *FilterStage) getClient() (*aiutil.Client, string) {
	cfg, err := s.stageConfigRepo.GetByStageName("filter")
	if err != nil || !cfg.Enabled {
		return s.defaultClient, filterSystemPrompt
	}
	prompt := cfg.Prompt
	if prompt == "" {
		prompt = filterSystemPrompt
	}
	return aiutil.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model, durationFromMinutes(s.aiTimeout)), prompt
}

func (s *FilterStage) Process(ctx context.Context, article *model.Article) (bool, error) {
	// Truncate content to avoid token limits
	content := article.Content
	if len(content) > 3000 {
		content = content[:3000]
	}

	userMsg := fmt.Sprintf("Title: %s\n\nContent:\n%s", article.Title, content)

	client, prompt := s.getClient()
	reply, err := client.Chat(ctx, prompt, userMsg)
	if err != nil {
		return false, fmt.Errorf("AI filter call: %w", err)
	}

	// Extract JSON from response (handle markdown code blocks)
	reply = extractJSON(reply)

	var resp filterResponse
	if err := json.Unmarshal([]byte(reply), &resp); err != nil {
		s.logger.Warn("failed to parse filter response", zap.String("reply", reply), zap.Error(err))
		// On parse failure, let the article through
		return true, nil
	}

	// Save filter result
	aiResult := &model.AIResult{
		ArticleID:     article.ID,
		IsAd:          resp.IsAd,
		IsMeaningless: resp.IsMeaningless,
		FilterReason:  resp.Reason,
	}

	// We'll create or update the AI result; handled by the AI service layer
	article.AIResult = aiResult

	if resp.IsAd || resp.IsMeaningless {
		return false, nil
	}

	return true, nil
}

// extractJSON tries to extract JSON from a string that may contain markdown code blocks.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Remove markdown code block markers
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	return s
}
