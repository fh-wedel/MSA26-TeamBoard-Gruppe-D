package config

import (
	"time"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	ServiceName string `env:"SERVICE_NAME" envDefault:"boardregistry-service"`
	Port        int    `env:"SERVICE_PORT" envDefault:"8007"`
	LogLevel    string `env:"LOG_LEVEL"    envDefault:"info"`

	DatabaseURL string `env:"DB_URL,required"`

	RabbitMQURL      string `env:"RABBITMQ_URL,required"`
	RabbitMQExchange string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`

	// Shared secret for HMAC service-to-service tokens (same scheme as project service).
	ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`

	OutboxInterval  time.Duration `env:"OUTBOX_INTERVAL"   envDefault:"2s"`
	OutboxBatchSize int32         `env:"OUTBOX_BATCH_SIZE" envDefault:"50"`
}

func Load() (Config, error) {
	var cfg Config
	return cfg, env.Parse(&cfg)
}
