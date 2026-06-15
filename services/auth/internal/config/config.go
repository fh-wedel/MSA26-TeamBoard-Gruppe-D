package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	// HTTP
	HTTPPort int `env:"HTTP_PORT" envDefault:"8001"`

	// Database
	DatabaseURL string `env:"DATABASE_URL,required"`

	// Redis
	RedisURL string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`

	// RabbitMQ
	RabbitMQURL      string `env:"RABBITMQ_URL"      envDefault:"amqp://guest:guest@localhost:5672/"`
	RabbitMQExchange string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`

	// SMTP
	SMTPHost     string `env:"SMTP_HOST"     envDefault:"localhost"`
	SMTPPort     int    `env:"SMTP_PORT"     envDefault:"1025"`
	SMTPFrom     string `env:"SMTP_FROM"     envDefault:"no-reply@teamboard.local"`
	SMTPUsername string `env:"SMTP_USERNAME"`
	SMTPPassword string `env:"SMTP_PASSWORD"`

	// JWT / token lifetimes
	Issuer           string        `env:"JWT_ISSUER"          envDefault:"teamboard-auth"`
	Audience         string        `env:"JWT_AUDIENCE"        envDefault:"teamboard"`
	AccessTokenTTL   time.Duration `env:"ACCESS_TOKEN_TTL"   envDefault:"15m"`
	RefreshTokenTTL  time.Duration `env:"REFRESH_TOKEN_TTL"  envDefault:"720h"`
	PasswordResetTTL time.Duration `env:"PASSWORD_RESET_TTL" envDefault:"1h"`
	PasswordResetURL string        `env:"PASSWORD_RESET_URL" envDefault:"http://localhost:5173/reset-password"`

	// Password policy
	PasswordMinLen int `env:"PASSWORD_MIN_LEN" envDefault:"8"`
	PasswordMaxLen int `env:"PASSWORD_MAX_LEN" envDefault:"128"`

	// Observability
	OtelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	LogLevel     string `env:"LOG_LEVEL"     envDefault:"info"`
	ServiceName  string `env:"SERVICE_NAME"  envDefault:"auth"`

	// Key encryption
	KeyEncryptionKey string `env:"KEY_ENCRYPTION_KEY,required"`

	// Outbox worker
	OutboxPollInterval time.Duration `env:"OUTBOX_POLL_INTERVAL" envDefault:"1s"`
	OutboxBatchSize    int           `env:"OUTBOX_BATCH_SIZE"    envDefault:"50"`
}

// Load parses Config from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return cfg, nil
}
