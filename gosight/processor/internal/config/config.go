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

	return &cfg, nil
}
