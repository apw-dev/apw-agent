package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

type cliOptions struct {
	Config, RequestID, SessionID string
	Explicit                     bool
	Args                         []string
}

func parseOptions(command string, args []string, stderr io.Writer) (cliOptions, int) {
	o := cliOptions{Config: "./apw.json"}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.Config, "config", o.Config, "path to workspace configuration")
	switch command {
	case "run", "execute", "review", "preview", "verify":
		fs.StringVar(&o.RequestID, "request-id", "", "stable submission ID for safe retries")
		fs.StringVar(&o.SessionID, "session", "", "conversation ID")
	}
	var flags []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			o.Args = append(o.Args, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			o.Args = append(o.Args, arg)
			continue
		}
		name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		if name == "h" || name == "help" {
			_ = fs.Parse([]string{"-h"})
			return o, 2
		}
		if fs.Lookup(name) == nil {
			fmt.Fprintf(stderr, "unknown flag: %s\n", arg)
			return o, 2
		}
		if name == "config" {
			o.Explicit = true
		}
		flags = append(flags, arg)
		if !strings.Contains(arg, "=") {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintf(stderr, "flag %s requires a value\n", arg)
				return o, 2
			}
			i++
			flags = append(flags, args[i])
		}
	}
	if err := fs.Parse(flags); err != nil {
		return o, 2
	}
	if strings.TrimSpace(o.Config) == "" {
		fmt.Fprintln(stderr, "config path must not be empty")
		return o, 2
	}
	creatingConfig := command == "model" && len(o.Args) == 1 && o.Args[0] == "init"
	if o.Explicit && !creatingConfig {
		info, err := os.Stat(o.Config)
		if err != nil || info.IsDir() {
			fmt.Fprintf(stderr, "configuration is not a readable file: %s (%v)\n", o.Config, err)
			return o, 1
		}
	}
	return o, 0
}

// splitCommand handles quoted Windows paths without treating backslashes as shell escapes.
// It only tokenizes APW commands; nothing here invokes a shell.
func splitCommand(line string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	started := false
	for _, r := range line {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if (r == 39 || r == 34) && (!started || strings.HasSuffix(word.String(), "=")) {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote in APW command")
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}
