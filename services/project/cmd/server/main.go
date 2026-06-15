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
	"github.com/redis/go-redis/v9"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamboard/services/project/internal/api"
	"github.com/teamboard/services/project/internal/boardtypeclient"
	"github.com/teamboard/services/project/internal/cache"
	"github.com/teamboard/services/project/internal/config"
	"github.com/teamboard/services/project/internal/domain"
	"github.com/teamboard/services/project/internal/events"
	"github.com/teamboard/services/project/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	// Logger
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Postgres
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("postgres ping failed", "error", err)
		os.Exit(1)
	}

	// Redis
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("redis url parse failed", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	// RabbitMQ
	rabbitConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("rabbitmq connect failed", "error", err)
		os.Exit(1)
	}
	defer rabbitConn.Close()

	// Domain wiring
	repo := repository.New(pool)
	permCache := cache.NewRedis(redisClient)
	boardTypes := boardtypeclient.New(cfg.BoardRegistryURL, cfg.ServiceTokenSecret, cfg.BoardRegistryTimeout, cfg.BoardTypeCacheTTL)
	svc := domain.NewProjectService(repo, permCache, boardTypes)

	// Outbox publisher
	publisher := events.NewPublisher(repo, rabbitConn, cfg.RabbitExchange)
	go publisher.Run(ctx)

	// Event consumer (also invalidates the board-type cache on boardtype.* events)
	consumer := events.NewConsumer(repo, rabbitConn, cfg.RabbitExchange, cfg.RabbitQueue, boardTypes)
	go consumer.Run(ctx)

	// HTTP server
	router := api.NewRouter(svc, cfg.ServiceTokenSecret)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("project service starting", "port", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	slog.Info("project service stopped")
}
