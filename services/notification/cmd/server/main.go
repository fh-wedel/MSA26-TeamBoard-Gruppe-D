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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/teamboard/services/notification/internal/api"
	"github.com/teamboard/services/notification/internal/cleanup"
	"github.com/teamboard/services/notification/internal/config"
	"github.com/teamboard/services/notification/internal/domain"
	"github.com/teamboard/services/notification/internal/events"
	"github.com/teamboard/services/notification/internal/projectclient"
	"github.com/teamboard/services/notification/internal/push"
	"github.com/teamboard/services/notification/internal/repository"
	"github.com/teamboard/services/notification/internal/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	if cfg.InstanceID == "" {
		cfg.InstanceID = uuid.New().String()
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

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdbOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("redis url parse failed", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(rdbOpts)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("redis ping failed", "error", err)
		os.Exit(1)
	}

	// ── RabbitMQ ──────────────────────────────────────────────────────────────
	amqpConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("rabbitmq connect failed", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	// ── Application wiring ────────────────────────────────────────────────────
	repo := repository.New(pool)
	projClient := projectclient.New(cfg.ProjectServiceURL, cfg.ServiceTokenSecret)

	hub := ws.NewHub()
	registry := push.NewRegistry(rdb, cfg.InstanceID)
	backplane := push.NewBackplane(rdb, hub)
	pushSvc := push.NewService(hub, registry, backplane, projClient)

	notifSvc := domain.NewNotificationService(repo)
	dispatcher := events.NewDispatcher(repo, pushSvc)

	publisher := events.NewPublisher(repo, amqpConn, cfg.Exchange)
	consumer := events.NewConsumer(repo, dispatcher, amqpConn, cfg.QueueName, cfg.Exchange)
	cleanupWorker := cleanup.NewWorker(repo, cfg.NotificationRetentionDays)

	go publisher.Run(ctx)
	go consumer.Run(ctx)
	go cleanupWorker.Run(ctx)
	go backplane.RunSubscriber(ctx)

	// ── HTTP server ───────────────────────────────────────────────────────────
	router := api.NewRouter(notifSvc, pushSvc, hub)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second, // longer for WebSocket
	}

	go func() {
		slog.Info("notification service listening", "port", cfg.Port, "instance_id", cfg.InstanceID)
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
