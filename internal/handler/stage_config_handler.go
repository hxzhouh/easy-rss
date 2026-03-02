package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
)

type StageConfigHandler struct {
	repo *repository.StageConfigRepo
}

func NewStageConfigHandler(repo *repository.StageConfigRepo) *StageConfigHandler {
	return &StageConfigHandler{repo: repo}
}

func (h *StageConfigHandler) List(c *gin.Context) {
	configs, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

type upsertStageConfigRequest struct {
	StageName string `json:"stage_name" binding:"required"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url" binding:"required"`
	APIKey    string `json:"api_key" binding:"required"`
	Model     string `json:"model" binding:"required"`
	Enabled   *bool  `json:"enabled"`
	Prompt    string `json:"prompt"`
}

func (h *StageConfigHandler) Upsert(c *gin.Context) {
	var req upsertStageConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	provider := req.Provider
	if provider == "" {
		provider = "openai"
	}

	cfg := &model.StageConfig{
		StageName: req.StageName,
		Provider:  provider,
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		Model:     req.Model,
		Enabled:   enabled,
		Prompt:    req.Prompt,
	}

	if err := h.repo.Upsert(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "saved", "data": cfg})
}

func (h *StageConfigHandler) Delete(c *gin.Context) {
	stageName := c.Param("stage_name")
	if stageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stage_name required"})
		return
	}

	if err := h.repo.Delete(stageName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
