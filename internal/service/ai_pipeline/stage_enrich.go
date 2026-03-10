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

// 英文版 Prompt
const enrichSystemPromptEN = `You are a professional content analyst and translator. Analyze the given article and provide:
1. Is it an advertisement or promotional content?
2. Is it meaningless content (spam, gibberish, clickbait with no substance)?
3. A quality score from 0 to 100 (based on depth, originality, clarity, and informativeness, < 75 will be filtered out)
4. A concise summary in English (2-3 sentences)
5. A concise summary translated to Chinese (2-3 sentences)
6. The article title translated to Chinese
7. Up to 5 relevant tags/keywords
8. Refined Chinese translation of the full content (accurate, fluent, and natural in Chinese)

Respond EXACTLY in this format. First provide valid JSON for metadata, followed by the delimiter '===CONTENT===', followed by the full translated text markdown.

{
  "is_ad": false,
  "is_meaningless": false,
  "quality_score": 85.0,
  "reason": "optional reason if quality < 75",
  "summary": "English summary here.",
  "summary_zh": "中文摘要。",
  "translated_title": "中文标题",
  "tags": ["tag1", "tag2"]
}
===CONTENT===
在这里输出文章全文的精校中文翻译（完整内容，Markdown 格式）。`

// 中文版 Prompt
const enrichSystemPromptZH = `你是一位专业的内容分析师和翻译家。请分析给定的文章并提供：
1. 是否为广告或宣传内容？
2. 是否为无意义内容（垃圾信息、乱码、无实质内容的标题党）？
3. 质量评分（0-100分，基于深度、原创性、清晰度和信息丰富度，<75分将被过滤）
4. 简洁的英文摘要（2-3句话）
5. 简洁的中文摘要（2-3句话）
6. 中文标题翻译
7. 最多5个相关标签/关键词
8. 文章全文的精校中文翻译（准确、流畅、符合中文阅读习惯）

请**严格**按照以下格式回复：首先提供用于元数据的合法 JSON，然后是分隔符 '===CONTENT==='，最后是完整的中文翻译 Markdown 文本。

{
  "is_ad": false,
  "is_meaningless": false,
  "quality_score": 85.0,
  "reason": "如果评分低于75，提供简短理由",
  "summary": "English summary here.",
  "summary_zh": "中文摘要。",
  "translated_title": "中文标题",
  "tags": ["标签1", "标签2"]
}
===CONTENT===
在这里输出文章全文的精校中文翻译（完整翻译，使用 Markdown 格式排版）。`

// 默认使用中文版 Prompt
const enrichSystemPrompt = enrichSystemPromptZH

// EnrichStage is the unified Stage: filters, scores, summarizes, and translates articles.
type EnrichStage struct {
	defaultClient   *aiutil.Client
	stageConfigRepo *repository.StageConfigRepo
	logger          *zap.Logger
	aiTimeout       interface{ Minutes() float64 }
}

type enrichResponse struct {
	IsAd            bool     `json:"is_ad"`
	IsMeaningless   bool     `json:"is_meaningless"`
	QualityScore    float64  `json:"quality_score"`
	Reason          string   `json:"reason"`
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

// GetEnrichPrompts returns all available prompt templates for the enrich stage
func GetEnrichPrompts() map[string]string {
	return map[string]string{
		"zh": enrichSystemPromptZH,
		"en": enrichSystemPromptEN,
	}
}

func (s *EnrichStage) Process(ctx context.Context, article *model.Article) (bool, error) {
	content := article.Content
	// Allow more content if we are skipping multiple stages
	if len(content) > 8000 {
		content = content[:8000]
	}

	userMsg := fmt.Sprintf("Title: %s\n\nContent:\n%s", article.Title, content)

	client, prompt := s.getClient()
	reply, err := client.Chat(ctx, prompt, userMsg)
	if err != nil {
		return false, fmt.Errorf("AI enrich call: %w", err)
	}

	jsonPart, markdownPart := extractTwoPartOutput(reply)

	var resp enrichResponse
	if err := json.Unmarshal([]byte(jsonPart), &resp); err != nil {
		s.logger.Warn("failed to parse enrich response JSON", zap.String("json_part", jsonPart), zap.Error(err))
		// Log the error but continue if possible, or fallback
		return false, fmt.Errorf("invalid json format")
	}

	// Save AI result
	aiResult := &model.AIResult{
		ArticleID:         article.ID,
		IsAd:              resp.IsAd,
		IsMeaningless:     resp.IsMeaningless,
		FilterReason:      resp.Reason,
		QualityScore:      resp.QualityScore,
		Summary:           resp.Summary,
		SummaryZh:         resp.SummaryZh,
		TranslatedTitle:   resp.TranslatedTitle,
		Tags:              resp.Tags,
		TranslatedContent: markdownPart,
	}
	article.AIResult = aiResult

	// Filter out if: ad, meaningless, or quality score < 75
	if resp.IsAd || resp.IsMeaningless || resp.QualityScore < 75 {
		if resp.QualityScore < 75 && resp.Reason == "" {
			aiResult.FilterReason = fmt.Sprintf("Quality score too low (%.1f < 75)", resp.QualityScore)
		}
		return false, nil
	}

	return true, nil
}

// extractTwoPartOutput splits the raw LLM response into JSON metadata and Markdown content
func extractTwoPartOutput(reply string) (string, string) {
	reply = strings.TrimSpace(reply)
	parts := strings.Split(reply, "===CONTENT===")

	jsonStr := strings.TrimSpace(parts[0])
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")
	jsonStr = strings.TrimSpace(jsonStr)

	markdownStr := ""
	if len(parts) > 1 {
		markdownStr = strings.TrimSpace(parts[1])
	}
	// Also strip markdown tags from the translated output if the LLM wrapped it
	markdownStr = strings.TrimPrefix(markdownStr, "```markdown")
	markdownStr = strings.TrimPrefix(markdownStr, "```")
	markdownStr = strings.TrimSuffix(markdownStr, "```")
	markdownStr = strings.TrimSpace(markdownStr)

	return jsonStr, markdownStr
}
