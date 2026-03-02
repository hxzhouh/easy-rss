package repository

import (
	"github.com/hxzhouh/easy-rss/internal/model"
	"gorm.io/gorm"
)

type StageConfigRepo struct {
	db *gorm.DB
}

func NewStageConfigRepo(db *gorm.DB) *StageConfigRepo {
	return &StageConfigRepo{db: db}
}

func (r *StageConfigRepo) GetByStageName(name string) (*model.StageConfig, error) {
	var cfg model.StageConfig
	err := r.db.Where("stage_name = ?", name).First(&cfg).Error
	return &cfg, err
}

func (r *StageConfigRepo) List() ([]model.StageConfig, error) {
	var configs []model.StageConfig
	err := r.db.Order("stage_name ASC").Find(&configs).Error
	return configs, err
}

// Upsert creates or updates a stage config by stage_name.
func (r *StageConfigRepo) Upsert(cfg *model.StageConfig) error {
	var existing model.StageConfig
	err := r.db.Where("stage_name = ?", cfg.StageName).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.Create(cfg).Error
	}
	if err != nil {
		return err
	}
	existing.Provider = cfg.Provider
	existing.BaseURL = cfg.BaseURL
	existing.APIKey = cfg.APIKey
	existing.Model = cfg.Model
	existing.Enabled = cfg.Enabled
	existing.Prompt = cfg.Prompt
	return r.db.Save(&existing).Error
}

func (r *StageConfigRepo) Delete(stageName string) error {
	return r.db.Where("stage_name = ?", stageName).Delete(&model.StageConfig{}).Error
}

// SeedDefaults creates default stage configs from the global AI config if none exist.
func (r *StageConfigRepo) SeedDefaults(baseURL, apiKey, modelName string) error {
	defaults := []model.StageConfig{
		{StageName: "filter", Provider: "openai", BaseURL: baseURL, APIKey: apiKey, Model: modelName, Enabled: true},
		{StageName: "enrich", Provider: "openai", BaseURL: baseURL, APIKey: apiKey, Model: modelName, Enabled: true},
	}
	for _, d := range defaults {
		var count int64
		r.db.Model(&model.StageConfig{}).Where("stage_name = ?", d.StageName).Count(&count)
		if count == 0 {
			if err := r.db.Create(&d).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
