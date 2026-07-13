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

	"github.com/teamboard/services/task/internal/api"
	"github.com/teamboard/services/task/internal/config"
	"github.com/teamboard/services/task/internal/documentclient"
	"github.com/teamboard/services/task/internal/domain"
	"github.com/teamboard/services/task/internal/events"
	"github.com/teamboard/services/task/internal/projectclient"
	"github.com/teamboard/services/task/internal/repository"
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

	// RabbitMQ
	rabbitConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("rabbitmq connect failed", "error", err)
		os.Exit(1)
	}
	defer rabbitConn.Close()

	// Service-to-service token issuer (HS256, shared secret) + user-JWT source.
	stIssuer := servicetoken.NewIssuer(cfg.ServiceTokenSecret)
	jwks := authmiddleware.NewJWKSSource(cfg.JWKSURL)

	// Clients
	projClient := projectclient.New(cfg.ProjectServiceURL, stIssuer)
	docClient := documentclient.New(cfg.DocumentServiceURL, stIssuer)

	// Domain wiring
	repo := repository.New(pool)
	svc := domain.NewTaskService(repo, projClient, docClient)

	// Outbox publisher (shared eventbus + outbox worker)
	pub, err := eventbus.NewPublisher(rabbitConn, cfg.RabbitExchange)
	if err != nil {
		slog.Error("event publisher init failed", "error", err)
		os.Exit(1)
	}
	defer pub.Close()
	worker := outbox.NewWorker(outbox.Config{
		Pool:         pool,
		Publisher:    pub,
		Producer:     "task-service",
		PollInterval: 200 * time.Millisecond,
	})
	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("outbox worker stopped", "error", err)
		}
	}()

	// Event consumer
	cons, err := eventbus.NewConsumer(rabbitConn, eventbus.TopologyOptions{
		Exchange:    cfg.RabbitExchange,
		Queue:       cfg.RabbitQueue,
		BindingKeys: events.BindingKeys(),
	})
	if err != nil {
		slog.Error("event consumer init failed", "error", err)
		os.Exit(1)
	}
	defer cons.Close()
	handler := events.NewEventHandler(repo)
	store := eventbus.IdempotencyFuncs{Has: repo.WasEventProcessed, Mark: repo.MarkEventProcessed}
	go func() {
		if err := cons.Subscribe(ctx, eventbus.IdempotentHandler(handler.Handle, store)); err != nil && ctx.Err() == nil {
			slog.Error("event consumer stopped", "error", err)
		}
	}()

	// HTTP server
	router := api.NewRouter(svc, jwks, cfg.JWTIssuer, cfg.JWTAudience)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("task service starting", "port", cfg.HTTPPort)
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
	slog.Info("task service stopped")
}
