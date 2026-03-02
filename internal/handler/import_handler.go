package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/service"
)

type ImportHandler struct {
	articleSvc *service.ArticleService
}

func NewImportHandler(articleSvc *service.ArticleService) *ImportHandler {
	return &ImportHandler{articleSvc: articleSvc}
}

type importRequest struct {
	Articles []service.ImportArticle `json:"articles" binding:"required,dive"`
}

func (h *ImportHandler) ImportArticles(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	count, err := h.articleSvc.ImportBatch(req.Articles)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"imported": count,
		"total":    len(req.Articles),
	})
}
