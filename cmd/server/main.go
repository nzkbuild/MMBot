package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mmbot/internal/config"
	apphttp "mmbot/internal/http"
	"mmbot/internal/integrations/openclaw"
	"mmbot/internal/integrations/telegram"
	"mmbot/internal/service/risk"
	storepkg "mmbot/internal/store"
	"mmbot/internal/store/memory"
	"mmbot/internal/store/postgres"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		log.Printf("failed to load .env: %v", err)
	}
	cfg := config.Load()
	var st storepkg.Store
	if cfg.StoreMode == "postgres" && cfg.DatabaseURL != "" {
		pgStore, err := postgres.NewStore(cfg.DatabaseURL, cfg.EATokenTTL, cfg.OAuthEncryptionKey)
		if err != nil {
			log.Printf("postgres store unavailable, falling back to memory store: %v", err)
			st = memory.NewStore(cfg.EATokenTTL)
		} else {
			st = pgStore
		}
	} else {
		st = memory.NewStore(cfg.EATokenTTL)
	}
	riskEngine := risk.NewEngine(
		cfg.MaxOpenPositions,
		cfg.MaxDailyLossPct,
		cfg.AIMinConfidence,
		cfg.MaxSpreadPips,
	)
	notifier := telegram.NewNotifier(cfg.TelegramBotToken, cfg.TelegramChatID)
	openClawClient := openclaw.NewClient(
		cfg.OpenClawWebhookURL,
		cfg.OpenClawTimeout,
		cfg.OpenClawMaxRetries,
		cfg.OpenClawRetryBase,
		cfg.OpenClawRetryMax,
	)

	srv := apphttp.NewServer(cfg, st, riskEngine, notifier, openClawClient)

	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("MMBot API listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
