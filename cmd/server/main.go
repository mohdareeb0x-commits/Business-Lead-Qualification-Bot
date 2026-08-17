// Package main wires the HTTP server, MongoDB, Telegram bot, and lead
// service together, handles graceful shutdown, and provides a single
// entrypoint for `go run` / Docker.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/telegramleadbot/telegram-lead-bot/internal/bot"
	"github.com/telegramleadbot/telegram-lead-bot/internal/config"
	"github.com/telegramleadbot/telegram-lead-bot/internal/database"
	"github.com/telegramleadbot/telegram-lead-bot/internal/health"
	"github.com/telegramleadbot/telegram-lead-bot/internal/lead"
	"github.com/telegramleadbot/telegram-lead-bot/internal/middleware"
	"github.com/telegramleadbot/telegram-lead-bot/internal/notification"
	"github.com/telegramleadbot/telegram-lead-bot/internal/scoring"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if cfg.GinMode != "" {
		gin.SetMode(cfg.GinMode)
	}

	// MongoDB.
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer connectCancel()
	mongo, err := database.Connect(connectCtx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Error("mongo connect", "err", err)
		os.Exit(1)
	}
	log.Info("mongo connected", "db", cfg.MongoDB)

	// Layers.
	repo := lead.NewMongoRepository(mongo)
	scorer := scoring.New()
	notifier := notification.NewTelegram(cfg.TelegramToken, cfg.AdminChatID)
	svc := lead.NewService(repo, scorer, notifier, log)

	store := bot.NewMemoryStore()
	botHandler := bot.NewHandler(cfg.TelegramToken, store, svc, log)
	botWebhook := bot.NewBot(botHandler, cfg.WebhookSecret, log)

	// HTTP router.
	r := gin.New()
	r.MaxMultipartMemory = 1 << 20
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logger(log))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", middleware.HeaderRequestID},
		ExposeHeaders:    []string{middleware.HeaderRequestID},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))
	// Apply rate limiting only to non-Telegram routes — Telegram's webhook
	// would be throttled otherwise.
	api := r.Group("/")
	api.Use(middleware.NewRateLimiter(20, 40).Middleware())

	health.New(mongo.Client).Register(r)
	lead.NewHandler(svc).Register(r.Group("/api/v1"))
	botWebhook.Register(r.Group("/api/v1"))

	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Run server.
	errCh := make(chan error, 1)
	go func() {
		log.Info("http server starting", "addr", srv.Addr, "env", cfg.ServerEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Error("http server crashed", "err", err)
	case sig := <-stop:
		log.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown", "err", err)
	}
	if err := mongo.Disconnect(shutdownCtx); err != nil {
		log.Warn("mongo disconnect", "err", err)
	}
	log.Info("bye")
}
