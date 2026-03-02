package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/service"
)

type ArticleHandler struct {
	articleSvc *service.ArticleService
	aiSvc      *service.AIService
}

func NewArticleHandler(articleSvc *service.ArticleService, aiSvc *service.AIService) *ArticleHandler {
	return &ArticleHandler{articleSvc: articleSvc, aiSvc: aiSvc}
}

func (h *ArticleHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var feedID *int64
	if fid := c.Query("feed_id"); fid != "" {
		id, err := strconv.ParseInt(fid, 10, 64)
		if err == nil {
			feedID = &id
		}
	}

	var aiStatus *int16
	if status := c.Query("ai_status"); status != "" {
		s, err := strconv.ParseInt(status, 10, 16)
		if err == nil {
			s16 := int16(s)
			aiStatus = &s16
		}
	}

	articles, total, err := h.articleSvc.List(page, pageSize, feedID, aiStatus)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      articles,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *ArticleHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	article, err := h.articleSvc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
		return
	}

	c.JSON(http.StatusOK, article)
}

func (h *ArticleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.articleSvc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *ArticleHandler) Reprocess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.aiSvc.ReprocessArticle(context.Background(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "reprocessing triggered"})
}
