package main

import (
	"context"
	"fmt"
	"os"

	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

const version = "0.1.0-alpha"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("apw version %s (Triad Engine: Architect + Surgeon + Reflex)\n", version)

	case "verify":
		workspace := "."
		if len(os.Args) > 2 {
			workspace = os.Args[2]
		}
		fmt.Printf("Running Reflex closed-loop verification on: %s...\n", workspace)
		v := reflex.NewVerifier(workspace)
		res := v.Verify(context.Background())
		if res.Passed {
			fmt.Printf("PASS: All checks (fmt, vet, build, test) passed in %v!\n", res.Duration)
		} else {
			fmt.Printf("FAIL: Verification failed at stage: %s\n", res.FailedStage)
			fmt.Println(reflex.FormatDiagnostics(res))
			os.Exit(1)
		}

	case "symbols":
		if len(os.Args) < 3 {
			fmt.Println("Usage: apw symbols <path/to/file.go>")
			os.Exit(1)
		}
		s := surgeon.NewSurgeon()
		syms, err := s.InspectGoSymbols(os.Args[2])
		if err != nil {
			fmt.Printf("Error inspecting symbols: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d symbols in %s:\n", len(syms), os.Args[2])
		for _, sym := range syms {
			fmt.Printf("  - [%s] %s (lines %d-%d)\n", sym.Kind, sym.Name, sym.StartLine, sym.EndLine)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Println("apw — Apiwat Agent CLI (Next-Gen Triad Architecture)")
	fmt.Println("Usage: apw <command> [arguments]")
	fmt.Println("\nAvailable commands:")
	fmt.Println("  version               Show version and architecture info")
	fmt.Println("  verify [dir]          Run Reflex closed-loop verification (fmt -> vet -> build -> test)")
	fmt.Println("  symbols <file.go>     Inspect AST code symbols using Surgeon inspector")
	fmt.Println("  daemon                Manage background agent daemon (not implemented)")
}
