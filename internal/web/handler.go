package web

import (
	"embed"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/repository"
)

//go:embed templates/*.html
var templateFS embed.FS

type WebHandler struct {
	articleRepo *repository.ArticleRepo
	feedRepo    *repository.FeedRepo
	tmpl        *template.Template
}

func NewWebHandler(articleRepo *repository.ArticleRepo, feedRepo *repository.FeedRepo) *WebHandler {
	funcMap := template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"formatDate": func(t time.Time) string {
			now := time.Now()
			diff := now.Sub(t)
			switch {
			case diff < time.Minute:
				return "刚刚"
			case diff < time.Hour:
				return strconv.Itoa(int(diff.Minutes())) + " 分钟前"
			case diff < 24*time.Hour:
				return strconv.Itoa(int(diff.Hours())) + " 小时前"
			case diff < 48*time.Hour:
				return "昨天"
			case t.Year() == now.Year():
				return t.Format("01-02")
			default:
				return t.Format("2006-01-02")
			}
		},
		"scoreColor": func(score float64) string {
			switch {
			case score >= 80:
				return "#16a34a" // green
			case score >= 60:
				return "#ca8a04" // yellow
			case score >= 40:
				return "#ea580c" // orange
			default:
				return "#dc2626" // red
			}
		},
		"scoreBg": func(score float64) string {
			switch {
			case score >= 80:
				return "#f0fdf4"
			case score >= 60:
				return "#fefce8"
			case score >= 40:
				return "#fff7ed"
			default:
				return "#fef2f2"
			}
		},
		"truncate": func(s string, n int) string {
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			return string(runes[:n]) + "..."
		},
		"pageRange": func(current, total int) []int {
			var pages []int
			start := current - 2
			end := current + 2
			if start < 1 {
				start = 1
			}
			if end > total {
				end = total
			}
			for i := start; i <= end; i++ {
				pages = append(pages, i)
			}
			return pages
		},
		"sub":      func(a, b int) int { return a - b },
		"add":      func(a, b int) int { return a + b },
		"scoreInt": func(f float64) int { return int(math.Round(f)) },
	}

	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

	return &WebHandler{
		articleRepo: articleRepo,
		feedRepo:    feedRepo,
		tmpl:        tmpl,
	}
}

type PageData struct {
	Articles   []ArticleView
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
	FeedID     *int64
	FeedName   string
}

type ArticleView struct {
	ID                int64
	Title             string
	TranslatedTitle   string
	Link              string
	Author            string
	Source            string
	FeedTitle         string
	SummaryZh         string
	Summary           string
	TranslatedContent string
	QualityScore      float64
	Tags              []string
	CreatedAt         time.Time
	AIStatus          int16
}

func (h *WebHandler) ServeAdmin(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(c.Writer, "admin.html", nil); err != nil {
		c.String(http.StatusInternalServerError, "Template error: "+err.Error())
	}
}

func (h *WebHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/", h.Index)
	r.GET("/article/:id", h.ArticleDetail)
	r.GET("/admin", h.ServeAdmin)
	r.GET("/admin/*path", h.ServeAdmin)
}

func (h *WebHandler) Index(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize := 20

	// Only show enriched articles (ai_status = 3)
	aiStatus := int16(3)
	var feedID *int64
	if fid := c.Query("feed_id"); fid != "" {
		id, err := strconv.ParseInt(fid, 10, 64)
		if err == nil {
			feedID = &id
		}
	}

	articles, total, err := h.articleRepo.List(page, pageSize, feedID, &aiStatus)
	if err != nil {
		c.String(500, "Internal Server Error")
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	var views []ArticleView
	for _, a := range articles {
		v := ArticleView{
			ID:        a.ID,
			Title:     a.Title,
			Link:      a.Link,
			Author:    a.Author,
			Source:    a.Source,
			CreatedAt: a.CreatedAt,
			AIStatus:  a.AIStatus,
		}
		if a.Feed != nil {
			v.FeedTitle = a.Feed.Title
		}
		if a.AIResult != nil {
			v.TranslatedTitle = a.AIResult.TranslatedTitle
			v.SummaryZh = a.AIResult.SummaryZh
			v.Summary = a.AIResult.Summary
			v.TranslatedContent = a.AIResult.TranslatedContent
			v.QualityScore = a.AIResult.QualityScore
			v.Tags = a.AIResult.Tags
		}
		views = append(views, v)
	}

	feedName := ""
	if feedID != nil {
		feed, err := h.feedRepo.GetByID(*feedID)
		if err == nil {
			feedName = feed.Title
		}
	}

	data := PageData{
		Articles:   views,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		FeedID:     feedID,
		FeedName:   feedName,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(c.Writer, "index.html", data); err != nil {
		c.String(500, "Template error: "+err.Error())
	}
}

func (h *WebHandler) ArticleDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid article ID")
		return
	}

	article, err := h.articleRepo.GetByID(id)
	if err != nil {
		c.String(http.StatusNotFound, "Article not found")
		return
	}

	// Build view model
	view := ArticleView{
		ID:        article.ID,
		Title:     article.Title,
		Link:      article.Link,
		Author:    article.Author,
		Source:    article.Source,
		CreatedAt: article.CreatedAt,
		AIStatus:  article.AIStatus,
	}
	if article.Feed != nil {
		view.FeedTitle = article.Feed.Title
	}
	if article.AIResult != nil {
		view.TranslatedTitle = article.AIResult.TranslatedTitle
		view.SummaryZh = article.AIResult.SummaryZh
		view.Summary = article.AIResult.Summary
		view.TranslatedContent = article.AIResult.TranslatedContent
		view.QualityScore = article.AIResult.QualityScore
		view.Tags = article.AIResult.Tags
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(c.Writer, "article.html", view); err != nil {
		c.String(500, "Template error: "+err.Error())
	}
}
