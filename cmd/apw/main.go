package main

import (
	"fmt"
	"os"
)

const version = "0.1.0-alpha"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("apw version %s\n", version)
		return
	}

	fmt.Println("apw — Apiwat Agent (M0 Skeleton)")
	fmt.Println("Usage: apw <command> [arguments]")
	fmt.Println("\nAvailable commands:")
	fmt.Println("  version          Show version")
	fmt.Println("  daemon           Manage background agent daemon")
	fmt.Println("  browser login    Launch browser to authenticate model")
}
