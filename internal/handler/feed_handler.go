package handler

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/service"
)

type FeedHandler struct {
	feedSvc    *service.FeedService
	fetcherSvc *service.FetcherService
}

func NewFeedHandler(feedSvc *service.FeedService, fetcherSvc *service.FetcherService) *FeedHandler {
	return &FeedHandler{feedSvc: feedSvc, fetcherSvc: fetcherSvc}
}

type createFeedRequest struct {
	Title    string `json:"title"`
	URL      string `json:"url" binding:"required"`
	Category string `json:"category"`
}

func (h *FeedHandler) Create(c *gin.Context) {
	var req createFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	feed := &model.Feed{
		Title:    req.Title,
		URL:      req.URL,
		Category: req.Category,
		Status:   model.FeedStatusActive,
	}

	if err := h.feedSvc.Create(feed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, feed)
}

func (h *FeedHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	feeds, total, err := h.feedSvc.List(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      feeds,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *FeedHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	feed, err := h.feedSvc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}

	c.JSON(http.StatusOK, feed)
}

type updateFeedRequest struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Category string `json:"category"`
	Status   *int16 `json:"status"`
}

func (h *FeedHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	feed, err := h.feedSvc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}

	var req updateFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title != "" {
		feed.Title = req.Title
	}
	if req.URL != "" {
		feed.URL = req.URL
	}
	if req.Category != "" {
		feed.Category = req.Category
	}
	if req.Status != nil {
		feed.Status = *req.Status
	}

	if err := h.feedSvc.Update(feed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, feed)
}

func (h *FeedHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.feedSvc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *FeedHandler) ImportOPML(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}

	// Open multipart file and read content directly
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open upload: " + err.Error()})
		return
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read upload: " + err.Error()})
		return
	}

	// Write to temp file
	tmp, err := os.CreateTemp("", "opml_upload_*.opml")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp file: " + err.Error()})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write temp file: " + err.Error()})
		return
	}
	tmp.Close()

	count, err := h.feedSvc.ImportOPML(tmpPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"imported": count})
}

func (h *FeedHandler) FetchNow(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.fetcherSvc.FetchOne(context.Background(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "fetch triggered"})
}

func (h *FeedHandler) QualityRanking(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	feeds, err := h.feedSvc.ListByQuality(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": feeds})
}
