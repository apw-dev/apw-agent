package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/apiwat/apw-agent/internal/channels/telegram"
)

const version = "0.1.0-alpha"

func main() {
	fmt.Println("Starting apwd (Apiwat Agent Daemon) M0...")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if os.Getenv("APW_TELEGRAM_TOKEN") != "" && os.Getenv("APW_TELEGRAM_ALLOW") != "" {
		bot, err := telegram.NewFromEnv(version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize Telegram bot: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Telegram long-polling active. Press Ctrl+C to stop.")
		if err := bot.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Telegram bot stopped with error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Daemon running (set APW_TELEGRAM_TOKEN and APW_TELEGRAM_ALLOW to enable Telegram). Press Ctrl+C to stop.")
		<-ctx.Done()
	}

	fmt.Println("\nGracefully shutting down apwd...")
}
