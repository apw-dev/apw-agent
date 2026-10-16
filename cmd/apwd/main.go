package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apiwat/apw-agent/internal/channels/telegram"
	"github.com/apiwat/apw-agent/internal/daemon"
)

const version = "0.1.0-alpha"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_ = daemon.WriteState(".", os.Getpid())
	defer daemon.ClearState(".")

	// Periodically refresh heartbeat state
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = daemon.WriteState(".", os.Getpid())
			}
		}
	}()

	if os.Getenv("APW_TELEGRAM_TOKEN") != "" && os.Getenv("APW_TELEGRAM_ALLOW") != "" {
		bot, err := telegram.NewFromEnv(version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize Telegram bot: %v\n", err)
			os.Exit(1)
		}
		if err := bot.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Telegram bot stopped with error: %v\n", err)
			os.Exit(1)
		}
	} else {
		<-ctx.Done()
	}
}
