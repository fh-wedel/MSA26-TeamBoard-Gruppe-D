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

	"github.com/teamboard/services/domain/notification/internal/api"
	"github.com/teamboard/services/domain/notification/internal/cleanup"
	"github.com/teamboard/services/domain/notification/internal/config"
	"github.com/teamboard/services/domain/notification/internal/domain"
	"github.com/teamboard/services/domain/notification/internal/events"
	"github.com/teamboard/services/domain/notification/internal/projectclient"
	"github.com/teamboard/services/domain/notification/internal/push"
	"github.com/teamboard/services/domain/notification/internal/repository"
	"github.com/teamboard/services/domain/notification/internal/ws"
	"github.com/teamboard/shared/go/authmiddleware"
	"github.com/teamboard/shared/go/eventbus"
	"github.com/teamboard/shared/go/outbox"
	"github.com/teamboard/shared/go/servicetoken"
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
	stIssuer := servicetoken.NewIssuer(cfg.ServiceTokenSecret)
	jwks := authmiddleware.NewJWKSSource(cfg.JWKSUrl)

	repo := repository.New(pool)
	projClient := projectclient.New(cfg.ProjectServiceURL, stIssuer)

	hub := ws.NewHub()
	registry := push.NewRegistry(rdb, cfg.InstanceID)
	backplane := push.NewBackplane(rdb, hub)
	pushSvc := push.NewService(hub, registry, backplane, projClient)

	notifSvc := domain.NewNotificationService(repo)
	dispatcher := events.NewDispatcher(repo, pushSvc)

	pub, err := eventbus.NewPublisher(amqpConn, cfg.Exchange)
	if err != nil {
		slog.Error("event publisher init failed", "error", err)
		os.Exit(1)
	}
	defer pub.Close()
	worker := outbox.NewWorker(outbox.Config{
		Pool:         pool,
		Publisher:    pub,
		Producer:     "notification-service",
		PollInterval: 200 * time.Millisecond,
	})

	cons, err := eventbus.NewConsumer(amqpConn, eventbus.TopologyOptions{
		Exchange:    cfg.Exchange,
		Queue:       cfg.QueueName,
		BindingKeys: events.BindingKeys(),
	})
	if err != nil {
		slog.Error("event consumer init failed", "error", err)
		os.Exit(1)
	}
	defer cons.Close()
	handler := events.NewEventHandler(dispatcher)
	idemStore := eventbus.IdempotencyFuncs{Has: repo.WasEventProcessed, Mark: repo.MarkEventProcessed}

	cleanupWorker := cleanup.NewWorker(repo, cfg.NotificationRetentionDays)

	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("outbox worker stopped", "error", err)
		}
	}()
	go func() {
		if err := cons.Subscribe(ctx, eventbus.IdempotentHandler(handler.Handle, idemStore)); err != nil && ctx.Err() == nil {
			slog.Error("event consumer stopped", "error", err)
		}
	}()
	go cleanupWorker.Run(ctx)
	go backplane.RunSubscriber(ctx)

	// ── HTTP server ───────────────────────────────────────────────────────────
	router := api.NewRouter(notifSvc, pushSvc, hub, jwks, cfg.JWTIssuer, cfg.JWTAudience)
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
