package config

import "github.com/caarlos0/env/v10"

type Config struct {
	Port       int    `env:"PORT" envDefault:"8005"`
	InstanceID string `env:"INSTANCE_ID"` // auto-generated if empty

	DatabaseURL string `env:"DATABASE_URL,required"`

	RedisURL string `env:"REDIS_URL,required"`

	RabbitMQURL  string `env:"RABBITMQ_URL,required"`
	Exchange     string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
	QueueName    string `env:"RABBITMQ_QUEUE" envDefault:"notification-service"`

	ProjectServiceURL  string `env:"PROJECT_SERVICE_URL,required"`
	ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`

	JWKSUrl     string `env:"JWT_JWKS_URL,required"`
	JWTIssuer   string `env:"JWT_ISSUER,required"`
	JWTAudience string `env:"JWT_AUDIENCE" envDefault:"teamboard-api"`

	NotificationRetentionDays int `env:"NOTIFICATION_RETENTION_DAYS" envDefault:"90"`
}

func Load() (Config, error) {
	var cfg Config
	return cfg, env.Parse(&cfg)
}
