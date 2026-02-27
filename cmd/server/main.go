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
	"mmbot/internal/store/memory"
)

func main() {
	cfg := config.Load()
	store := memory.NewStore(cfg.EATokenTTL)
	riskEngine := risk.NewEngine(
		cfg.MaxOpenPositions,
		cfg.MaxDailyLossPct,
		cfg.AIMinConfidence,
		cfg.MaxSpreadPips,
	)
	notifier := telegram.NewNotifier(cfg.TelegramBotToken, cfg.TelegramChatID)
	openClawClient := openclaw.NewClient(cfg.OpenClawWebhookURL, cfg.OpenClawTimeout)

	srv := apphttp.NewServer(cfg, store, riskEngine, notifier, openClawClient)

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

