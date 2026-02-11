package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Kafka      KafkaConfig      `yaml:"kafka"`
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	Redis      RedisConfig      `yaml:"redis"`
	Batch      BatchConfig      `yaml:"batch"`
	Insights   InsightsConfig   `yaml:"insights"`
}

type InsightsConfig struct {
	RageClick      RageClickConfig      `yaml:"rage_click"`
	DeadClick      DeadClickConfig      `yaml:"dead_click"`
	ErrorClick     ErrorClickConfig     `yaml:"error_click"`
	ThrashedCursor ThrashedCursorConfig `yaml:"thrashed_cursor"`
	UTurn          UTurnConfig          `yaml:"u_turn"`
	SlowPage       SlowPageConfig       `yaml:"slow_page"`
}

type RageClickConfig struct {
	Enabled      bool  `yaml:"enabled"`
	MinClicks    int   `yaml:"min_clicks"`
	TimeWindowMs int64 `yaml:"time_window_ms"`
	RadiusPx     int   `yaml:"radius_px"`
}

type DeadClickConfig struct {
	Enabled             bool  `yaml:"enabled"`
	ObservationWindowMs int64 `yaml:"observation_window_ms"`
}

type ErrorClickConfig struct {
	Enabled       bool  `yaml:"enabled"`
	ErrorWindowMs int64 `yaml:"error_window_ms"`
}

type ThrashedCursorConfig struct {
	Enabled             bool  `yaml:"enabled"`
	MinDurationMs       int64 `yaml:"min_duration_ms"`
	MinDirectionChanges int   `yaml:"min_direction_changes"`
	MinVelocity         int   `yaml:"min_velocity"`
}

type UTurnConfig struct {
	Enabled       bool  `yaml:"enabled"`
	MaxTimeAwayMs int64 `yaml:"max_time_away_ms"`
}

type SlowPageConfig struct {
	Enabled         bool  `yaml:"enabled"`
	LCPThresholdMs  int64 `yaml:"lcp_threshold_ms"`
	TTFBThresholdMs int64 `yaml:"ttfb_threshold_ms"`
}

type KafkaConfig struct {
	Brokers       []string          `yaml:"brokers"`
	Topics        map[string]string `yaml:"topics"`
	ConsumerGroup string            `yaml:"consumer_group"`
}

type ClickHouseConfig struct {
	Addr         string `yaml:"addr"`
	Database     string `yaml:"database"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type BatchConfig struct {
	Size          int           `yaml:"size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Batch.Size == 0 {
		cfg.Batch.Size = 1000
	}
	if cfg.Batch.FlushInterval == 0 {
		cfg.Batch.FlushInterval = 5 * time.Second
	}
	if cfg.ClickHouse.MaxOpenConns == 0 {
		cfg.ClickHouse.MaxOpenConns = 10
	}
	if cfg.ClickHouse.MaxIdleConns == 0 {
		cfg.ClickHouse.MaxIdleConns = 5
	}

	// Set insights defaults
	if cfg.Insights.RageClick.MinClicks == 0 {
		cfg.Insights.RageClick.MinClicks = 5
	}
	if cfg.Insights.RageClick.TimeWindowMs == 0 {
		cfg.Insights.RageClick.TimeWindowMs = 2000
	}
	if cfg.Insights.RageClick.RadiusPx == 0 {
		cfg.Insights.RageClick.RadiusPx = 50
	}
	if cfg.Insights.DeadClick.ObservationWindowMs == 0 {
		cfg.Insights.DeadClick.ObservationWindowMs = 1000
	}
	if cfg.Insights.ErrorClick.ErrorWindowMs == 0 {
		cfg.Insights.ErrorClick.ErrorWindowMs = 1000
	}
	if cfg.Insights.ThrashedCursor.MinDurationMs == 0 {
		cfg.Insights.ThrashedCursor.MinDurationMs = 2000
	}
	if cfg.Insights.ThrashedCursor.MinDirectionChanges == 0 {
		cfg.Insights.ThrashedCursor.MinDirectionChanges = 10
	}
	if cfg.Insights.ThrashedCursor.MinVelocity == 0 {
		cfg.Insights.ThrashedCursor.MinVelocity = 500
	}
	if cfg.Insights.UTurn.MaxTimeAwayMs == 0 {
		cfg.Insights.UTurn.MaxTimeAwayMs = 10000
	}
	if cfg.Insights.SlowPage.LCPThresholdMs == 0 {
		cfg.Insights.SlowPage.LCPThresholdMs = 3000
	}
	if cfg.Insights.SlowPage.TTFBThresholdMs == 0 {
		cfg.Insights.SlowPage.TTFBThresholdMs = 800
	}

	return &cfg, nil
}
