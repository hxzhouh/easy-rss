package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Fetcher  FetcherConfig  `mapstructure:"fetcher"`
	AI       AIConfig       `mapstructure:"ai"`
	Quality  QualityConfig  `mapstructure:"quality"`
	Init     InitConfig     `mapstructure:"init"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Driver   string `mapstructure:"driver"` // sqlite / postgres
	Path     string `mapstructure:"path"`   // SQLite file path
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + itoa(d.Port) +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.DBName +
		" sslmode=" + d.SSLMode
}

type AuthConfig struct {
	AdminUsername  string `mapstructure:"admin_username"`
	AdminPassword  string `mapstructure:"admin_password"`
	JWTSecret      string `mapstructure:"jwt_secret"`
	JWTExpireHours int    `mapstructure:"jwt_expire_hours"`
}

type FetcherConfig struct {
	Interval      time.Duration `mapstructure:"interval"`
	Timeout       time.Duration `mapstructure:"timeout"`
	UserAgent     string        `mapstructure:"user_agent"`
	MaxConcurrent int           `mapstructure:"max_concurrent"`
}

type AIConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Provider         string        `mapstructure:"provider"`
	APIKey           string        `mapstructure:"api_key"`
	BaseURL          string        `mapstructure:"base_url"`
	Model            string        `mapstructure:"model"`
	Timeout          time.Duration `mapstructure:"timeout"`
	FilterInterval   time.Duration `mapstructure:"filter_interval"`
	FilterConcurrent int           `mapstructure:"filter_concurrent"`
	EnrichInterval   time.Duration `mapstructure:"enrich_interval"`
	EnrichConcurrent int           `mapstructure:"enrich_concurrent"`
}

type QualityConfig struct {
	EvaluationInterval   time.Duration `mapstructure:"evaluation_interval"`
	AutoDisableThreshold float64       `mapstructure:"auto_disable_threshold"`
	MinArticlesForEval   int           `mapstructure:"min_articles_for_eval"`
}

type InitConfig struct {
	OPMLFile string `mapstructure:"opml_file"`
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Default driver
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.Database.Driver == "sqlite" && cfg.Database.Path == "" {
		cfg.Database.Path = "easyrss.db"
	}

	return &cfg, nil
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
