package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/pkg/aiutil"
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

// TestConnectionRequest 测试 API 连接的请求参数
type TestConnectionRequest struct {
	BaseURL string `json:"base_url" binding:"required"`
	APIKey  string `json:"api_key" binding:"required"`
	Model   string `json:"model" binding:"required"`
}

// TestConnection 测试 API 配置是否有效
func (h *StageConfigHandler) TestConnection(c *gin.Context) {
	var req TestConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建临时客户端测试连接（10分钟超时，适配慢速 API）
	client := aiutil.NewClient(req.BaseURL, req.APIKey, req.Model, 10*time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := client.ValidateAPIKey(ctx); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"valid":   false,
			"message": "连接失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "连接成功，API Key 有效",
	})
}
