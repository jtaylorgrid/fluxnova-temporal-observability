package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Fluxnova FluxnovaConfig `yaml:"fluxnova"`
	Kafka    KafkaConfig    `yaml:"kafka"`
	Pipeline PipelineConfig `yaml:"pipeline"`
	LogLevel string         `yaml:"log_level"`
}

type FluxnovaConfig struct {
	BaseURL  string `yaml:"base_url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type KafkaConfig struct {
	Brokers        []string `yaml:"brokers"`
	EventsTopic    string   `yaml:"events_topic"`
	ProcessesTopic string   `yaml:"processes_topic"`
}

type PipelineConfig struct {
	PollInterval time.Duration `yaml:"poll_interval"`
	BatchSize    int           `yaml:"batch_size"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Fluxnova: FluxnovaConfig{
			BaseURL:  "http://localhost:8080/engine-rest",
			Username: "",
			Password: "",
		},
		Kafka: KafkaConfig{
			Brokers:        []string{"localhost:9092"},
			EventsTopic:    "fluxnova-events",
			ProcessesTopic: "fluxnova-processes",
		},
		Pipeline: PipelineConfig{
			PollInterval: 10 * time.Second,
			BatchSize:    100,
		},
		LogLevel: "info",
	}

	// Load from YAML if exists
	if data, err := os.ReadFile("config.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override from environment
	if v := os.Getenv("FLUXNOVA_BASE_URL"); v != "" {
		cfg.Fluxnova.BaseURL = v
	}
	if v := os.Getenv("FLUXNOVA_USERNAME"); v != "" {
		cfg.Fluxnova.Username = v
	}
	if v := os.Getenv("FLUXNOVA_PASSWORD"); v != "" {
		cfg.Fluxnova.Password = v
	}
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		cfg.Kafka.Brokers = []string{v}
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	return cfg, nil
}
