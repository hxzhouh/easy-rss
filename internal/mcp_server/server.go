package mcp_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// Server wraps an MCP server with Easy-RSS tools, exposed over Streamable HTTP.
type Server struct {
	mcpServer   *mcp.Server
	feedRepo    *repository.FeedRepo
	articleRepo *repository.ArticleRepo
	logger      *zap.Logger
}

func New(feedRepo *repository.FeedRepo, articleRepo *repository.ArticleRepo, logger *zap.Logger) *Server {
	s := &Server{
		feedRepo:    feedRepo,
		articleRepo: articleRepo,
		logger:      logger,
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "easy-rss",
		Version: "v1.0.0",
	}, nil)

	// Register tools
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "add_feed",
		Description: "添加一个 RSS 订阅源",
	}, s.addFeed)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "delete_feed",
		Description: "删除一个 RSS 订阅源",
	}, s.deleteFeed)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_feeds",
		Description: "列出所有 RSS 订阅源",
	}, s.listFeeds)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_articles",
		Description: "列出文章（可按 feed_id 和 ai_status 筛选）",
	}, s.listArticles)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "read_article",
		Description: "读取一篇文章的完整内容（包含 AI 处理结果）",
	}, s.readArticle)

	s.mcpServer = mcpServer
	return s
}

// Handler returns an http.Handler for Streamable HTTP MCP transport.
// Mount this on your HTTP router, e.g. at "/mcp".
func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)
}

// --- Tool Input/Output types ---

type AddFeedInput struct {
	URL      string `json:"url" jsonschema:"required,RSS 订阅源的 URL"`
	Title    string `json:"title" jsonschema:"订阅源标题（可选，抓取时自动获取）"`
	Category string `json:"category" jsonschema:"分类标签"`
}

type AddFeedOutput struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func (s *Server) addFeed(ctx context.Context, req *mcp.CallToolRequest, input AddFeedInput) (*mcp.CallToolResult, AddFeedOutput, error) {
	feed := &model.Feed{
		Title:    input.Title,
		URL:      input.URL,
		Category: input.Category,
		Status:   model.FeedStatusActive,
	}
	if feed.Title == "" {
		feed.Title = feed.URL
	}

	if err := s.feedRepo.Create(feed); err != nil {
		return errResult("添加订阅源失败: " + err.Error()), AddFeedOutput{}, nil
	}

	s.logger.Info("MCP: feed added", zap.String("url", feed.URL))
	return nil, AddFeedOutput{ID: feed.ID, Title: feed.Title, URL: feed.URL}, nil
}

type DeleteFeedInput struct {
	ID int64 `json:"id" jsonschema:"required,要删除的订阅源 ID"`
}

type DeleteFeedOutput struct {
	Message string `json:"message"`
}

func (s *Server) deleteFeed(ctx context.Context, req *mcp.CallToolRequest, input DeleteFeedInput) (*mcp.CallToolResult, DeleteFeedOutput, error) {
	if _, err := s.feedRepo.GetByID(input.ID); err != nil {
		return errResult("订阅源不存在"), DeleteFeedOutput{}, nil
	}

	if err := s.feedRepo.Delete(input.ID); err != nil {
		return errResult("删除失败: " + err.Error()), DeleteFeedOutput{}, nil
	}

	s.logger.Info("MCP: feed deleted", zap.Int64("id", input.ID))
	return nil, DeleteFeedOutput{Message: fmt.Sprintf("订阅源 #%d 已删除", input.ID)}, nil
}

type ListFeedsInput struct{}

type FeedItem struct {
	ID           int64   `json:"id"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	Category     string  `json:"category"`
	Status       int16   `json:"status"`
	QualityScore float64 `json:"quality_score"`
}

type ListFeedsOutput struct {
	Feeds []FeedItem `json:"feeds"`
	Total int        `json:"total"`
}

func (s *Server) listFeeds(ctx context.Context, req *mcp.CallToolRequest, input ListFeedsInput) (*mcp.CallToolResult, ListFeedsOutput, error) {
	feeds, total, err := s.feedRepo.List(1, 100)
	if err != nil {
		return errResult("获取订阅源列表失败: " + err.Error()), ListFeedsOutput{}, nil
	}

	items := make([]FeedItem, 0, len(feeds))
	for _, f := range feeds {
		items = append(items, FeedItem{
			ID:           f.ID,
			Title:        f.Title,
			URL:          f.URL,
			Category:     f.Category,
			Status:       f.Status,
			QualityScore: f.QualityScore,
		})
	}

	return nil, ListFeedsOutput{Feeds: items, Total: int(total)}, nil
}

type ListArticlesInput struct {
	FeedID   *int64 `json:"feed_id" jsonschema:"按订阅源 ID 筛选"`
	AIStatus *int16 `json:"ai_status" jsonschema:"按 AI 处理状态筛选: 0=pending, 1=filtered_out, 2=passed, 3=enriched"`
	Page     int    `json:"page" jsonschema:"页码，默认 1"`
	PageSize int    `json:"page_size" jsonschema:"每页数量，默认 20"`
}

type ArticleItem struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	Link            string  `json:"link"`
	Author          string  `json:"author"`
	AIStatus        int16   `json:"ai_status"`
	QualityScore    float64 `json:"quality_score,omitempty"`
	SummaryZh       string  `json:"summary_zh,omitempty"`
	TranslatedTitle string  `json:"translated_title,omitempty"`
	Source          string  `json:"source"`
}

type ListArticlesOutput struct {
	Articles []ArticleItem `json:"articles"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
}

func (s *Server) listArticles(ctx context.Context, req *mcp.CallToolRequest, input ListArticlesInput) (*mcp.CallToolResult, ListArticlesOutput, error) {
	page := input.Page
	if page <= 0 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	articles, total, err := s.articleRepo.List(page, pageSize, input.FeedID, input.AIStatus, nil)
	if err != nil {
		return errResult("获取文章列表失败: " + err.Error()), ListArticlesOutput{}, nil
	}

	items := make([]ArticleItem, 0, len(articles))
	for _, a := range articles {
		item := ArticleItem{
			ID:       a.ID,
			Title:    a.Title,
			Link:     a.Link,
			Author:   a.Author,
			AIStatus: a.AIStatus,
			Source:   a.Source,
		}
		if a.AIResult != nil {
			item.QualityScore = a.AIResult.QualityScore
			item.SummaryZh = a.AIResult.SummaryZh
			item.TranslatedTitle = a.AIResult.TranslatedTitle
		}
		items = append(items, item)
	}

	return nil, ListArticlesOutput{Articles: items, Total: total, Page: page}, nil
}

type ReadArticleInput struct {
	ID int64 `json:"id" jsonschema:"required,文章 ID"`
}

type ReadArticleOutput struct {
	ID              int64    `json:"id"`
	Title           string   `json:"title"`
	TranslatedTitle string   `json:"translated_title,omitempty"`
	Link            string   `json:"link"`
	Author          string   `json:"author"`
	Content         string   `json:"content"`
	Source          string   `json:"source"`
	AIStatus        int16    `json:"ai_status"`
	QualityScore    float64  `json:"quality_score,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	SummaryZh       string   `json:"summary_zh,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	IsAd            bool     `json:"is_ad,omitempty"`
	FilterReason    string   `json:"filter_reason,omitempty"`
	FeedTitle       string   `json:"feed_title,omitempty"`
}

func (s *Server) readArticle(ctx context.Context, req *mcp.CallToolRequest, input ReadArticleInput) (*mcp.CallToolResult, ReadArticleOutput, error) {
	article, err := s.articleRepo.GetByID(input.ID)
	if err != nil {
		return errResult("文章不存在"), ReadArticleOutput{}, nil
	}

	out := ReadArticleOutput{
		ID:       article.ID,
		Title:    article.Title,
		Link:     article.Link,
		Author:   article.Author,
		Content:  article.Content,
		Source:   article.Source,
		AIStatus: article.AIStatus,
	}

	if article.AIResult != nil {
		out.QualityScore = article.AIResult.QualityScore
		out.Summary = article.AIResult.Summary
		out.SummaryZh = article.AIResult.SummaryZh
		out.TranslatedTitle = article.AIResult.TranslatedTitle
		out.Tags = article.AIResult.Tags
		out.IsAd = article.AIResult.IsAd
		out.FilterReason = article.AIResult.FilterReason
	}
	if article.Feed != nil {
		out.FeedTitle = article.Feed.Title
	}

	return nil, out, nil
}

// errResult creates an error tool result.
func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// jsonString marshals v to a JSON string for readable output.
func jsonString(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
