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

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/teamboard/services/boardregistry/internal/api"
	"github.com/teamboard/services/boardregistry/internal/config"
	"github.com/teamboard/services/boardregistry/internal/domain"
	"github.com/teamboard/services/boardregistry/internal/repository"
	"github.com/teamboard/shared/go/authmiddleware"
	"github.com/teamboard/shared/go/eventbus"
	"github.com/teamboard/shared/go/outbox"
	"github.com/teamboard/shared/go/servicetoken"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

	rabbitConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("rabbitmq connect failed", "error", err)
		os.Exit(1)
	}
	defer rabbitConn.Close()

	repo := repository.New(pool)
	var svc domain.BoardTypeService = domain.NewService(repo)

	pub, err := eventbus.NewPublisher(rabbitConn, cfg.RabbitMQExchange)
	if err != nil {
		slog.Error("event publisher init failed", "error", err)
		os.Exit(1)
	}
	defer pub.Close()
	worker := outbox.NewWorker(outbox.Config{
		Pool:         pool,
		Publisher:    pub,
		Producer:     "boardregistry-service",
		PollInterval: cfg.OutboxInterval,
		BatchSize:    int(cfg.OutboxBatchSize),
	})
	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("outbox worker stopped", "error", err)
		}
	}()

	stVerifier := servicetoken.NewVerifier(cfg.ServiceTokenSecret, "internal")
	jwks := authmiddleware.NewJWKSSource(cfg.JWKSURL)
	router := api.NewRouter(svc, pool, jwks, cfg.JWTIssuer, cfg.JWTAudience, stVerifier)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("boardregistry service starting", "port", cfg.Port)
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
	slog.Info("boardregistry service stopped")
}
