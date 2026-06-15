package config

import "github.com/caarlos0/env/v10"

type Config struct {
	Port int `env:"PORT" envDefault:"8004"`

	DatabaseURL string `env:"DATABASE_URL,required"`

	RabbitMQURL  string `env:"RABBITMQ_URL,required"`
	Exchange     string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
	QueueName    string `env:"RABBITMQ_QUEUE" envDefault:"document-service"`

	ProjectServiceURL  string `env:"PROJECT_SERVICE_URL,required"`
	ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`

	// S3/MinIO
	S3Bucket           string `env:"S3_BUCKET" envDefault:"teamboard-documents"`
	S3Region           string `env:"S3_REGION" envDefault:"us-east-1"`
	S3AccessKeyID      string `env:"S3_ACCESS_KEY_ID,required"`
	S3SecretAccessKey  string `env:"S3_SECRET_ACCESS_KEY,required"`
	S3InternalEndpoint string `env:"S3_INTERNAL_ENDPOINT" envDefault:"http://minio:9000"`
	S3PublicEndpoint   string `env:"S3_PUBLIC_ENDPOINT" envDefault:"http://localhost:9000"`

	// Content-type allowlist (comma-separated, optional — uses default if empty)
	AllowedContentTypes []string `env:"ALLOWED_CONTENT_TYPES" envSeparator:","`
}

func Load() (Config, error) {
	var cfg Config
	return cfg, env.Parse(&cfg)
}
