package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teamboard/services/document/internal/api"
	"github.com/teamboard/services/document/internal/cleanup"
	"github.com/teamboard/services/document/internal/config"
	"github.com/teamboard/services/document/internal/domain"
	"github.com/teamboard/services/document/internal/events"
	"github.com/teamboard/services/document/internal/projectclient"
	"github.com/teamboard/services/document/internal/repository"
	"github.com/teamboard/services/document/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Database ──────────────────────────────────────────────────────────────
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("db ping failed", "error", err)
		os.Exit(1)
	}

	// ── RabbitMQ ──────────────────────────────────────────────────────────────
	amqpConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("rabbitmq connect failed", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	// ── Storage ───────────────────────────────────────────────────────────────
	store, err := storage.NewS3Storage(ctx, storage.Config{
		Bucket:           cfg.S3Bucket,
		InternalEndpoint: cfg.S3InternalEndpoint,
		PublicEndpoint:   cfg.S3PublicEndpoint,
		Region:           cfg.S3Region,
		AccessKeyID:      cfg.S3AccessKeyID,
		SecretAccessKey:  cfg.S3SecretAccessKey,
	})
	if err != nil {
		slog.Error("storage init failed", "error", err)
		os.Exit(1)
	}

	// ── Repository, clients, service ──────────────────────────────────────────
	repo := repository.New(pool)
	projClient := projectclient.New(cfg.ProjectServiceURL, cfg.ServiceTokenSecret)
	svc := domain.NewService(repo, store, projClient, cfg.AllowedContentTypes)

	// ── Background workers ────────────────────────────────────────────────────
	publisher := events.NewPublisher(repo, amqpConn, cfg.Exchange)
	consumer := events.NewConsumer(repo, amqpConn, cfg.QueueName, cfg.Exchange)
	cleanupWorker := cleanup.NewWorker(repo, store)

	go publisher.Run(ctx)
	go consumer.Run(ctx)
	go cleanupWorker.Run(ctx)

	// ── HTTP server ───────────────────────────────────────────────────────────
	router := api.NewRouter(svc)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("document service listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
