package config

import "time"

type Config struct {
	ServiceName string `env:"SERVICE_NAME" envDefault:"plugin-service"`
	Port        int    `env:"SERVICE_PORT" envDefault:"8006"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

	DB struct {
		URL          string `env:"DB_URL,required"`
		MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
		MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
	}

	RabbitMQ struct {
		URL           string   `env:"RABBITMQ_URL,required"`
		Exchange      string   `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
		ConsumerQueue string   `env:"RABBITMQ_CONSUMER_QUEUE" envDefault:"plugin-service-queue"`
		BindingKeys   []string `env:"RABBITMQ_BINDING_KEYS" envSeparator:","`
	}

	Delivery struct {
		WorkerCount            int           `env:"DELIVERY_WORKER_COUNT" envDefault:"5"`
		HTTPConnectTimeout     time.Duration `env:"DELIVERY_HTTP_CONNECT_TIMEOUT" envDefault:"5s"`
		HTTPTotalTimeout       time.Duration `env:"DELIVERY_HTTP_TOTAL_TIMEOUT" envDefault:"30s"`
		ResponseBodyLimitBytes int64         `env:"DELIVERY_RESPONSE_BODY_LIMIT_BYTES" envDefault:"1048576"`
		MaxAttempts            int           `env:"MAX_DELIVERY_ATTEMPTS" envDefault:"8"`
		UserAgent              string        `env:"WEBHOOK_USER_AGENT" envDefault:"TeamBoard-Webhook/1.0"`
	}

	Security struct {
		AllowInsecureHTTP  bool  `env:"ALLOW_INSECURE_HTTP" envDefault:"false"`
		AllowPrivateURLs   bool  `env:"ALLOW_PRIVATE_URLS" envDefault:"false"`
		AllowedPorts       []int `env:"WEBHOOK_ALLOWED_PORTS" envSeparator:"," envDefault:"80,443,8080,8443"`
		ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`
	}

	Breaker struct {
		ConsecutiveFailures uint32        `env:"BREAKER_CONSECUTIVE_FAILURES" envDefault:"5"`
		Timeout             time.Duration `env:"BREAKER_TIMEOUT" envDefault:"30s"`
	}

	JWT struct {
		JWKSURL  string `env:"JWT_JWKS_URL,required"`
		Issuer   string `env:"JWT_ISSUER,required"`
		Audience string `env:"JWT_AUDIENCE,required"`
	}

	ProjectService struct {
		URL                string        `env:"PROJECT_SERVICE_URL,required"`
		Timeout            time.Duration `env:"PROJECT_SERVICE_TIMEOUT" envDefault:"200ms"`
		PermissionCacheTTL time.Duration `env:"PERMISSION_CACHE_TTL" envDefault:"30s"`
	}

	Cleanup struct {
		DeliveryRetentionDays int `env:"DELIVERY_RETENTION_DAYS" envDefault:"30"`
	}

	Observability struct {
		OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	}
}
