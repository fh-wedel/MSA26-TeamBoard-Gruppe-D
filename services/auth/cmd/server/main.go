package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"github.com/teamboard/services/auth/internal/api"
	"github.com/teamboard/services/auth/internal/config"
	"github.com/teamboard/services/auth/internal/domain"
	"github.com/teamboard/services/auth/internal/keys"
	"github.com/teamboard/services/auth/internal/mail"
	"github.com/teamboard/services/auth/internal/ratelimit"
	"github.com/teamboard/services/auth/internal/repository"
	"github.com/teamboard/shared/go/eventbus"
	"github.com/teamboard/shared/go/outbox"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Structured logger
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx := context.Background()

	// Database
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	slog.Info("postgres connected")

	// Redis
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	slog.Info("redis connected")

	// RabbitMQ
	amqpConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer amqpConn.Close()
	slog.Info("rabbitmq connected")

	// Infrastructure
	repo := repository.New(pool)
	limiter := ratelimit.NewRedisLimiter(redisClient)

	encKey, err := base64.StdEncoding.DecodeString(cfg.KeyEncryptionKey)
	if err != nil || len(encKey) != 32 {
		return fmt.Errorf("KEY_ENCRYPTION_KEY must be a base64-encoded 32-byte key (generate: openssl rand -base64 32)")
	}
	keyMgr := keys.NewManager(repo, encKey)

	mailSender, err := mail.NewSMTPSender(mail.Config{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		From:     cfg.SMTPFrom,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
	})
	if err != nil {
		return fmt.Errorf("setup mail sender: %w", err)
	}

	// Ensure a signing key exists on startup
	if err := keyMgr.EnsureActiveKey(ctx); err != nil {
		return fmt.Errorf("ensure signing key: %w", err)
	}

	// Domain service
	svc := domain.NewAuthService(repo, mailSender, limiter, keyMgr, domain.ServiceConfig{
		Issuer:           cfg.Issuer,
		Audience:         cfg.Audience,
		AccessTokenTTL:   cfg.AccessTokenTTL,
		RefreshTokenTTL:  cfg.RefreshTokenTTL,
		PasswordResetTTL: cfg.PasswordResetTTL,
		PasswordResetURL: cfg.PasswordResetURL,
		PasswordMinLen:   cfg.PasswordMinLen,
		PasswordMaxLen:   cfg.PasswordMaxLen,
	})

	// Outbox publisher goroutine (shared eventbus + outbox worker)
	pub, err := eventbus.NewPublisher(amqpConn, cfg.RabbitMQExchange)
	if err != nil {
		return fmt.Errorf("event publisher: %w", err)
	}
	defer pub.Close()
	worker := outbox.NewWorker(outbox.Config{
		Pool:         pool,
		Publisher:    pub,
		Producer:     "auth-service",
		PollInterval: cfg.OutboxPollInterval,
		BatchSize:    cfg.OutboxBatchSize,
	})
	pubCtx, cancelPub := context.WithCancel(ctx)
	defer cancelPub()
	go func() {
		if err := worker.Run(pubCtx); err != nil && pubCtx.Err() == nil {
			slog.Error("outbox worker stopped unexpectedly", "error", err)
		}
	}()

	// HTTP server
	router := api.NewRouter(svc, repo, pool, cfg.Issuer, cfg.Audience)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("auth service starting", "port", cfg.HTTPPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()

	<-quit
	slog.Info("shutting down gracefully")

	cancelPub()

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}
