package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Println("Starting apwd (Apiwat Agent Daemon) M0...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println("Daemon running. Press Ctrl+C to stop.")
	<-sigChan

	fmt.Println("\nGracefully shutting down apwd...")
}
