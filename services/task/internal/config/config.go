package config

import "github.com/caarlos0/env/v10"

type Config struct {
	HTTPPort    int    `env:"HTTP_PORT"    envDefault:"8003"`
	DatabaseURL string `env:"DATABASE_URL" envRequired:"true"`
	RabbitMQURL string `env:"RABBITMQ_URL" envDefault:"amqp://guest:guest@localhost:5672/"`

	RabbitExchange  string `env:"RABBIT_EXCHANGE"    envDefault:"teamboard.events"`
	RabbitQueue     string `env:"RABBIT_QUEUE"       envDefault:"task-service"`
	OutboxBatchSize int32  `env:"OUTBOX_BATCH_SIZE"  envDefault:"50"`

	ProjectServiceURL  string `env:"PROJECT_SERVICE_URL"  envDefault:"http://project:8002"`
	DocumentServiceURL string `env:"DOCUMENT_SERVICE_URL" envDefault:"http://document:8004"`

	ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET" envRequired:"true"`

	// User-JWT validation (RS256 against the auth service's JWKS).
	// Defaults match the auth service's deployed JWT_ISSUER/JWT_AUDIENCE.
	JWKSURL     string `env:"JWT_JWKS_URL" envRequired:"true"`
	JWTIssuer   string `env:"JWT_ISSUER"   envDefault:"https://auth.teamboard.local"`
	JWTAudience string `env:"JWT_AUDIENCE" envDefault:"teamboard-api"`

	LogLevel     string `env:"LOG_LEVEL"     envDefault:"info"`
	OTELEndpoint string `env:"OTEL_ENDPOINT" envDefault:""`
}

func Load() (Config, error) {
	var cfg Config
	return cfg, env.Parse(&cfg)
}
