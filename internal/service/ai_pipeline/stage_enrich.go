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

// 英文版 Prompt
const enrichSystemPromptEN = `You are a professional content analyst and translator. Analyze the given article and provide:
1. A quality score from 0 to 100 (based on depth, originality, clarity, and informativeness)
2. A concise summary in English (2-3 sentences)
3. A concise summary translated to Chinese (2-3 sentences)
4. The article title translated to Chinese
5. Up to 5 relevant tags/keywords
6. Full article content translated to Chinese (complete translation, not summarized)
7. Refined Chinese translation of the content (polished and improved version of the translation in #6)

For the translation process:
- First, translate the full article content to Chinese accurately
- Then, refine the translation to improve fluency, fix any awkward phrasing, and ensure it reads naturally in Chinese

Respond ONLY with valid JSON in the following format, no other text:
{
  "quality_score": 85.0,
  "summary": "English summary here.",
  "summary_zh": "中文摘要。",
  "translated_title": "中文标题",
  "tags": ["tag1", "tag2"],
  "translated_content": "文章全文的初始中文翻译（完整内容）",
  "refined_translation": "精校后的中文翻译（润色优化后的版本）"
}`

// 中文版 Prompt
const enrichSystemPromptZH = `你是一位专业的内容分析师和翻译家。请分析给定的文章并提供：
1. 质量评分（0-100分，基于深度、原创性、清晰度和信息丰富度）
2. 简洁的英文摘要（2-3句话）
3. 简洁的中文摘要（2-3句话）
4. 中文标题翻译
5. 最多5个相关标签/关键词
6. 文章全文的中文翻译（完整翻译，不是摘要）
7. 精校后的中文翻译（对第6项的润色优化版本）

翻译流程要求：
- 第一步：准确地将文章全文翻译成中文
- 第二步：精校翻译结果，提升流畅度，修正生硬表达，确保符合中文阅读习惯

请只返回有效的 JSON 格式，不要包含其他文本：
{
  "quality_score": 85.0,
  "summary": "English summary here.",
  "summary_zh": "中文摘要。",
  "translated_title": "中文标题",
  "tags": ["标签1", "标签2"],
  "translated_content": "文章全文的初始中文翻译（完整内容）",
  "refined_translation": "精校后的中文翻译（润色优化后的版本）"
}`

// 默认使用中文版 Prompt
const enrichSystemPrompt = enrichSystemPromptZH

// EnrichStage is Stage 2: scores, summarizes, and translates articles.
type EnrichStage struct {
	defaultClient   *aiutil.Client
	stageConfigRepo *repository.StageConfigRepo
	logger          *zap.Logger
	aiTimeout       interface{ Minutes() float64 }
}

type enrichResponse struct {
	QualityScore         float64  `json:"quality_score"`
	Summary              string   `json:"summary"`
	SummaryZh            string   `json:"summary_zh"`
	TranslatedTitle      string   `json:"translated_title"`
	Tags                 []string `json:"tags"`
	TranslatedContent    string   `json:"translated_content"`
	RefinedTranslation   string   `json:"refined_translation"`
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
	// 优先使用精校版本，如果没有则使用初译版本
	if resp.RefinedTranslation != "" {
		article.AIResult.TranslatedContent = resp.RefinedTranslation
	} else {
		article.AIResult.TranslatedContent = resp.TranslatedContent
	}

	return true, nil
}
