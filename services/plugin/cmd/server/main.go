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

	"github.com/caarlos0/env/v10"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/teamboard/services/plugin/internal/api"
	"github.com/teamboard/services/plugin/internal/config"
	"github.com/teamboard/services/plugin/internal/delivery"
	"github.com/teamboard/services/plugin/internal/domain"
	"github.com/teamboard/services/plugin/internal/events"
	"github.com/teamboard/services/plugin/internal/projectclient"
	"github.com/teamboard/services/plugin/internal/repository"
	"github.com/teamboard/shared/go/eventbus"
	"github.com/teamboard/shared/go/outbox"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	var cfg config.Config
	if err := env.Parse(&cfg); err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Database
	pool, err := pgxpool.New(ctx, cfg.DB.URL)
	if err != nil {
		logger.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// RabbitMQ
	amqpConn, err := amqp.Dial(cfg.RabbitMQ.URL)
	if err != nil {
		logger.Error("failed to connect to rabbitmq", "err", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	// Wiring
	repo := repository.New(pool)
	permChecker := projectclient.New(
		cfg.ProjectService.URL,
		cfg.Security.ServiceTokenSecret,
		cfg.ProjectService.Timeout,
		cfg.ProjectService.PermissionCacheTTL,
	)
	webhookSvc := domain.NewWebhookService(
		repo, permChecker,
		cfg.Security.AllowInsecureHTTP,
		cfg.Security.AllowPrivateURLs,
		cfg.Security.AllowedPorts,
	)
	dispatcherSvc := domain.NewDispatcherService(repo)

	// Delivery worker
	httpClient := delivery.NewSSRFSafeClient(cfg.Delivery.HTTPConnectTimeout, cfg.Delivery.HTTPTotalTimeout)
	breakerReg := delivery.NewBreakerRegistry(cfg.Breaker.ConsecutiveFailures, cfg.Breaker.Timeout)
	worker := delivery.NewWorker(repo, httpClient, breakerReg, cfg.Delivery.UserAgent, logger)

	// Event handlers (shared eventbus + outbox worker)
	projHandler := events.NewProjectHandler(repo, logger)
	userHandler := events.NewUserHandler(logger)
	handler := events.NewEventHandler(dispatcherSvc, projHandler, userHandler)

	pub, err := eventbus.NewPublisher(amqpConn, cfg.RabbitMQ.Exchange)
	if err != nil {
		logger.Error("event publisher init failed", "err", err)
		os.Exit(1)
	}
	defer pub.Close()
	outboxWorker := outbox.NewWorker(outbox.Config{
		Pool:         pool,
		Publisher:    pub,
		Producer:     "plugin-service",
		PollInterval: 200 * time.Millisecond,
	})

	cons, err := eventbus.NewConsumer(amqpConn, eventbus.TopologyOptions{
		Exchange:    cfg.RabbitMQ.Exchange,
		Queue:       cfg.RabbitMQ.ConsumerQueue,
		BindingKeys: cfg.RabbitMQ.BindingKeys,
	})
	if err != nil {
		logger.Error("event consumer init failed", "err", err)
		os.Exit(1)
	}
	defer cons.Close()
	idemStore := eventbus.IdempotencyFuncs{Has: repo.WasEventProcessed, Mark: repo.MarkEventProcessed}

	// HTTP server
	router := api.NewRouter(webhookSvc, pool)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start background goroutines
	go func() {
		if err := cons.Subscribe(ctx, eventbus.IdempotentHandler(handler.Handle, idemStore)); err != nil && ctx.Err() == nil {
			logger.Error("consumer stopped", "err", err)
		}
	}()
	go func() {
		if err := outboxWorker.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("outbox worker stopped", "err", err)
		}
	}()
	for i := 0; i < cfg.Delivery.WorkerCount; i++ {
		go func() {
			if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("delivery worker stopped", "err", err)
			}
		}()
	}

	logger.Info("plugin service starting", "port", cfg.Port)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
