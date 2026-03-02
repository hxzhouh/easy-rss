package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/hxzhouh/easy-rss/internal/config"
	"github.com/hxzhouh/easy-rss/internal/handler"
	"github.com/hxzhouh/easy-rss/internal/mcp_server"
	"github.com/hxzhouh/easy-rss/internal/middleware"
	"github.com/hxzhouh/easy-rss/internal/model"
	"github.com/hxzhouh/easy-rss/internal/repository"
	"github.com/hxzhouh/easy-rss/internal/scheduler"
	"github.com/hxzhouh/easy-rss/internal/service"
	"github.com/hxzhouh/easy-rss/internal/service/ai_pipeline"
	"github.com/hxzhouh/easy-rss/internal/web"
	"github.com/hxzhouh/easy-rss/pkg/aiutil"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	opmlPath := flag.String("opml", "", "path to OPML file for initial import")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Init logger
	var logger *zap.Logger
	if cfg.Server.Mode == "debug" {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	// Init database
	db, err := openDB(cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}
	logger.Info("database connected", zap.String("driver", cfg.Database.Driver))

	// AutoMigrate
	if err := db.AutoMigrate(
		&model.Feed{},
		&model.Article{},
		&model.AIResult{},
		&model.User{},
		&model.StageConfig{},
	); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}

	// Create unique composite index for articles (cross-DB compatible)
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_articles_feed_guid ON articles(feed_id, guid)")

	// Init repositories
	feedRepo := repository.NewFeedRepo(db)
	articleRepo := repository.NewArticleRepo(db)
	userRepo := repository.NewUserRepo(db)
	stageConfigRepo := repository.NewStageConfigRepo(db)

	// Seed admin user
	if err := userRepo.SeedAdmin(cfg.Auth.AdminUsername, cfg.Auth.AdminPassword); err != nil {
		logger.Fatal("failed to seed admin", zap.Error(err))
	}
	logger.Info("admin user ready", zap.String("username", cfg.Auth.AdminUsername))

	// Init services
	feedSvc := service.NewFeedService(feedRepo, logger)
	fetcherSvc := service.NewFetcherService(
		feedRepo, articleRepo, logger,
		cfg.Fetcher.UserAgent, cfg.Fetcher.Timeout, cfg.Fetcher.MaxConcurrent,
	)
	articleSvc := service.NewArticleService(articleRepo, logger)
	authSvc := service.NewAuthService(userRepo, cfg.Auth.JWTSecret, cfg.Auth.JWTExpireHours)

	// Init AI stages (decoupled)
	aiClient := aiutil.NewClient(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model, cfg.AI.Timeout)
	var filterStage ai_pipeline.Stage
	var enrichStage ai_pipeline.Stage
	if cfg.AI.Enabled {
		// Seed default stage configs from global AI settings
		if err := stageConfigRepo.SeedDefaults(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model); err != nil {
			logger.Warn("failed to seed stage configs", zap.Error(err))
		}
		filterStage = ai_pipeline.NewFilterStage(aiClient, stageConfigRepo, articleRepo, logger, cfg.AI.Timeout)
		enrichStage = ai_pipeline.NewEnrichStage(aiClient, stageConfigRepo, logger, cfg.AI.Timeout)
	}
	aiSvc := service.NewAIService(articleRepo, filterStage, enrichStage, logger, db, cfg.AI.FilterConcurrent)

	qualitySvc := service.NewQualityService(
		feedRepo, articleRepo, logger,
		cfg.Quality.MinArticlesForEval,
		cfg.Quality.AutoDisableThreshold,
	)

	// Import OPML if specified
	opml := *opmlPath
	if opml == "" {
		opml = cfg.Init.OPMLFile
	}
	if opml != "" {
		count, err := feedSvc.ImportOPML(opml)
		if err != nil {
			logger.Error("OPML import failed", zap.Error(err))
		} else {
			logger.Info("OPML import completed", zap.Int("feeds_imported", count))
		}
	}

	// Start scheduler
	sched := scheduler.New(logger, fetcherSvc, aiSvc, qualitySvc)
	sched.Start(scheduler.CronIntervals{
		Fetch:   durationToCron(cfg.Fetcher.Interval),
		Filter:  durationToCron(cfg.AI.FilterInterval),
		Enrich:  durationToCron(cfg.AI.EnrichInterval),
		Quality: durationToCron(cfg.Quality.EvaluationInterval),
	})
	defer sched.Stop()

	// Setup Gin
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// Init handlers
	authHandler := handler.NewAuthHandler(authSvc)
	feedHandler := handler.NewFeedHandler(feedSvc, fetcherSvc)
	articleHandler := handler.NewArticleHandler(articleSvc, aiSvc)
	importHandler := handler.NewImportHandler(articleSvc)
	stageConfigHandler := handler.NewStageConfigHandler(stageConfigRepo)

	// Routes
	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", authHandler.Login)

		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		{
			// Feeds
			auth.GET("/feeds", feedHandler.List)
			auth.POST("/feeds", feedHandler.Create)
			auth.GET("/feeds/:id", feedHandler.GetByID)
			auth.PUT("/feeds/:id", feedHandler.Update)
			auth.DELETE("/feeds/:id", feedHandler.Delete)
			auth.POST("/feeds/import/opml", feedHandler.ImportOPML)
			auth.POST("/feeds/:id/fetch", feedHandler.FetchNow)
			auth.GET("/feeds/quality", feedHandler.QualityRanking)

			// Articles
			auth.GET("/articles", articleHandler.List)
			auth.GET("/articles/:id", articleHandler.GetByID)
			auth.DELETE("/articles/:id", articleHandler.Delete)
			auth.POST("/articles/:id/reprocess", articleHandler.Reprocess)

			// Import
			auth.POST("/import/articles", importHandler.ImportArticles)

			// Stage Configs (per-stage AI model settings)
			auth.GET("/stage-configs", stageConfigHandler.List)
			auth.POST("/stage-configs", stageConfigHandler.Upsert)
			auth.DELETE("/stage-configs/:stage_name", stageConfigHandler.Delete)
			auth.POST("/stage-configs/test-connection", stageConfigHandler.TestConnection)
		}
	}

	// MCP Streamable HTTP endpoint
	mcpSrv := mcp_server.New(feedRepo, articleRepo, logger)
	r.Any("/mcp", gin.WrapH(mcpSrv.Handler()))
	logger.Info("MCP server mounted at /mcp")

	// Web routes (reader homepage + admin)
	webHandler := web.NewWebHandler(articleRepo, feedRepo)
	webHandler.RegisterRoutes(r)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("starting Easy-RSS server", zap.String("addr", addr))

	go func() {
		if err := r.Run(addr); err != nil {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")
}

// durationToCron converts a time.Duration to a cron expression.
func durationToCron(d interface{ Minutes() float64 }) string {
	minutes := int(d.Minutes())
	if minutes <= 0 {
		minutes = 30
	}
	if minutes < 60 {
		return fmt.Sprintf("@every %dm", minutes)
	}
	hours := minutes / 60
	return fmt.Sprintf("@every %dh", hours)
}

// openDB creates a GORM database connection based on the configured driver.
func openDB(dbCfg config.DatabaseConfig) (*gorm.DB, error) {
	switch dbCfg.Driver {
	case "sqlite":
		db, err := gorm.Open(sqlite.Open(dbCfg.Path), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		// Enable WAL mode for better concurrency
		db.Exec("PRAGMA journal_mode=WAL")
		db.Exec("PRAGMA foreign_keys=ON")
		return db, nil
	case "postgres":
		return gorm.Open(postgres.Open(dbCfg.DSN()), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported database driver: %s (use 'sqlite' or 'postgres')", dbCfg.Driver)
	}
}
