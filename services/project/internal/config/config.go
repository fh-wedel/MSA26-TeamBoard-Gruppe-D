package config

import (
	"time"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	HTTPPort    int    `env:"HTTP_PORT"    envDefault:"8002"`
	DatabaseURL string `env:"DATABASE_URL" envRequired:"true"`
	RedisURL    string `env:"REDIS_URL"    envDefault:"redis://localhost:6379/0"`
	RabbitMQURL string `env:"RABBITMQ_URL" envDefault:"amqp://guest:guest@localhost:5672/"`

	RabbitExchange   string `env:"RABBIT_EXCHANGE"    envDefault:"teamboard.events"`
	RabbitQueue      string `env:"RABBIT_QUEUE"       envDefault:"project-service"`
	OutboxInterval   int    `env:"OUTBOX_INTERVAL_MS" envDefault:"2000"`
	OutboxBatchSize  int    `env:"OUTBOX_BATCH_SIZE"  envDefault:"50"`

	ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET" envRequired:"true"`

	// Board registry service (source of board-type definitions).
	BoardRegistryURL     string        `env:"BOARDREGISTRY_SERVICE_URL" envDefault:"http://boardregistry:8007"`
	BoardRegistryTimeout time.Duration `env:"BOARDREGISTRY_TIMEOUT"     envDefault:"500ms"`
	BoardTypeCacheTTL    time.Duration `env:"BOARDTYPE_CACHE_TTL"       envDefault:"60s"`

	LogLevel  string `env:"LOG_LEVEL"  envDefault:"info"`
	OTELEndpoint string `env:"OTEL_ENDPOINT" envDefault:""`
}

func Load() (Config, error) {
	var cfg Config
	return cfg, env.Parse(&cfg)
}
