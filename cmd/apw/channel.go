package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/tui"
)

func runChannelCmdWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfgPath := "./apw.json"
	explicitCfg := false
	var rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "flag --config requires a value")
				return 2
			}
			i++
			cfgPath = args[i]
			explicitCfg = true
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			cfgPath = strings.TrimPrefix(arg, "--config=")
			explicitCfg = true
			continue
		}
		if arg == "-h" || arg == "--help" || arg == "help" {
			printChannelUsage(stdout)
			return 0
		}
		rest = append(rest, arg)
	}

	if explicitCfg {
		info, err := os.Stat(cfgPath)
		if err != nil || info.IsDir() {
			fmt.Fprintf(stderr, "configuration is not a readable file: %s (%v)\n", cfgPath, err)
			return 1
		}
	}

	cfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// Allow optional channel name prefix: e.g. `apw channel telegram ...` or `apw messaging telegram ...`
	explicitTelegram := false
	if len(rest) > 0 && strings.EqualFold(rest[0], "telegram") {
		explicitTelegram = true
		rest = rest[1:]
	}

	if len(rest) == 0 && stdin == os.Stdin {
		if explicitTelegram {
			if rc := tui.RunInteractiveTelegramMenu(cfgPath, stdin, stdout, stderr); rc >= 0 {
				return rc
			}
		} else {
			if rc := tui.RunInteractiveChannelMenu(cfgPath, stdin, stdout, stderr); rc >= 0 {
				return rc
			}
		}
	}

	credentialRoot := cfg.Workspace
	if !filepath.IsAbs(credentialRoot) {
		credentialRoot = filepath.Join(filepath.Dir(cfgPath), credentialRoot)
	}

	if len(rest) == 0 || strings.EqualFold(rest[0], "status") || strings.EqualFold(rest[0], "list") {
		printChannelStatus(stdout, cfg, credentialRoot)
		return 0
	}

	action := strings.ToLower(rest[0])
	subArgs := rest[1:]

	switch action {
	case "set":
		return runChannelSet(cfg, cfgPath, credentialRoot, subArgs, stdout, stderr)

	case "token":
		if len(subArgs) != 1 || strings.TrimSpace(subArgs[0]) == "" {
			fmt.Fprintln(stderr, "Usage: apw channel token <bot-token>")
			return 2
		}
		tok := strings.TrimSpace(subArgs[0])
		envName := cfg.Channels.Telegram.EffectiveTokenEnv()
		if err := config.SaveAuthKey(credentialRoot, envName, tok); err != nil {
			fmt.Fprintf(stderr, "Error saving token: %v\n", err)
			return 1
		}
		_ = os.Setenv(envName, tok)
		cfg.Channels.Telegram.TokenEnv = envName
		if len(cfg.Channels.Telegram.AllowUsers) > 0 {
			cfg.Channels.Telegram.Enabled = true
		}
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Saved Telegram Bot Token (%s) to .apw/auth.json (%s)\n", config.MaskSecret(tok), envName)
		return 0

	case "clear-token":
		envName := cfg.Channels.Telegram.EffectiveTokenEnv()
		if err := config.DeleteAuthKey(credentialRoot, envName); err != nil {
			fmt.Fprintf(stderr, "Error clearing token: %v\n", err)
			return 1
		}
		_ = os.Unsetenv(envName)
		cfg.Channels.Telegram.Enabled = false
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Cleared %s from .apw/auth.json and disabled Telegram channel.\n", envName)
		return 0

	case "allow":
		if len(subArgs) == 0 {
			fmt.Fprintln(stderr, "Usage: apw channel allow <user-id-1,user-id-2,...>")
			return 2
		}
		ids, err := tui.ParseAllowedUserIDs(strings.Join(subArgs, ","))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if len(ids) == 0 {
			fmt.Fprintln(stderr, "Usage: apw channel allow <user-id-1,user-id-2,...>")
			return 2
		}
		cfg.Channels.Telegram.AllowUsers = ids
		if cfg.Channels.Telegram.ResolveToken(credentialRoot) != "" {
			cfg.Channels.Telegram.Enabled = true
		}
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Updated Telegram allowed user IDs: %s\n", tui.FormatAllowedUserIDs(ids))
		return 0

	case "enable":
		if len(cfg.Channels.Telegram.AllowUsers) == 0 {
			fmt.Fprintln(stderr, "Cannot enable Telegram channel: no allowed user IDs configured. Run 'apw channel allow <user-id>' first.")
			return 1
		}
		if cfg.Channels.Telegram.ResolveToken(credentialRoot) == "" {
			fmt.Fprintln(stderr, "Cannot enable Telegram channel: no Bot Token configured. Run 'apw channel token <bot-token>' first.")
			return 1
		}
		cfg.Channels.Telegram.Enabled = true
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Enabled Telegram channel in %s\n", cfgPath)
		return 0

	case "disable":
		cfg.Channels.Telegram.Enabled = false
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Disabled Telegram channel in %s\n", cfgPath)
		return 0

	default:
		fmt.Fprintf(stderr, "unknown channel subcommand: %s\n", action)
		printChannelUsage(stderr)
		return 2
	}
}

func runChannelSet(cfg *config.Config, cfgPath, credentialRoot string, args []string, stdout, stderr io.Writer) int {
	var token string
	var hasToken bool
	var allowRaw string
	var hasAllow bool
	var enableFlag, disableFlag bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--token" || arg == "-token":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "flag --token requires a value")
				return 2
			}
			i++
			token = strings.TrimSpace(args[i])
			hasToken = true
		case strings.HasPrefix(arg, "--token="):
			token = strings.TrimSpace(strings.TrimPrefix(arg, "--token="))
			hasToken = true
		case arg == "--allow" || arg == "-allow" || arg == "--allow-users":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "flag --allow requires a value")
				return 2
			}
			i++
			allowRaw = args[i]
			hasAllow = true
		case strings.HasPrefix(arg, "--allow="):
			allowRaw = strings.TrimPrefix(arg, "--allow=")
			hasAllow = true
		case strings.HasPrefix(arg, "--allow-users="):
			allowRaw = strings.TrimPrefix(arg, "--allow-users=")
			hasAllow = true
		case arg == "--enable" || arg == "-enable":
			enableFlag = true
		case arg == "--disable" || arg == "-disable":
			disableFlag = true
		default:
			fmt.Fprintf(stderr, "unknown flag for channel set: %s\n", arg)
			printChannelUsage(stderr)
			return 2
		}
	}

	if enableFlag && disableFlag {
		fmt.Fprintln(stderr, "cannot specify both --enable and --disable")
		return 2
	}
	if !hasToken && !hasAllow && !enableFlag && !disableFlag {
		fmt.Fprintln(stderr, "Usage: apw channel set [--token <bot-token>] [--allow <id1,id2>] [--enable|--disable]")
		return 2
	}

	if hasToken {
		if token == "" {
			fmt.Fprintln(stderr, "token must not be empty (use 'apw channel clear-token' to remove)")
			return 2
		}
		envName := cfg.Channels.Telegram.EffectiveTokenEnv()
		if err := config.SaveAuthKey(credentialRoot, envName, token); err != nil {
			fmt.Fprintf(stderr, "Error saving token: %v\n", err)
			return 1
		}
		_ = os.Setenv(envName, token)
		cfg.Channels.Telegram.TokenEnv = envName
	}

	if hasAllow {
		ids, err := tui.ParseAllowedUserIDs(allowRaw)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if len(ids) == 0 {
			fmt.Fprintln(stderr, "--allow requires at least one positive Telegram user ID")
			return 2
		}
		cfg.Channels.Telegram.AllowUsers = ids
	}

	if disableFlag {
		cfg.Channels.Telegram.Enabled = false
	} else if enableFlag || (hasToken || hasAllow) {
		if len(cfg.Channels.Telegram.AllowUsers) > 0 && cfg.Channels.Telegram.ResolveToken(credentialRoot) != "" {
			cfg.Channels.Telegram.Enabled = true
		} else if enableFlag {
			fmt.Fprintln(stderr, "Cannot enable Telegram channel: both a Bot Token and at least one allowed user ID are required.")
			return 1
		}
	}

	if err := cfg.Save(cfgPath); err != nil {
		fmt.Fprintf(stderr, "Error saving config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Updated Telegram channel configuration in %s (enabled=%v, allow_users=%s)\n",
		cfgPath, cfg.Channels.Telegram.Enabled, tui.FormatAllowedUserIDs(cfg.Channels.Telegram.AllowUsers))
	return 0
}

func printChannelStatus(stdout io.Writer, cfg *config.Config, credentialRoot string) {
	tg := cfg.Channels.Telegram
	tok := tg.ResolveToken(credentialRoot)
	state := "disabled"
	if tg.Enabled {
		state = "enabled"
	}
	tokenDisplay := "not configured"
	if tok != "" {
		tokenDisplay = fmt.Sprintf("configured (%s via %s)", config.MaskSecret(tok), tg.EffectiveTokenEnv())
	}
	fmt.Fprintln(stdout, "APW Messaging & Channel Gateway:")
	fmt.Fprintf(stdout, "  Telegram Bot Gateway : %s\n", state)
	fmt.Fprintf(stdout, "  Bot Token            : %s\n", tokenDisplay)
	fmt.Fprintf(stdout, "  Allowed User IDs     : %s\n", tui.FormatAllowedUserIDs(tg.AllowUsers))
	fmt.Fprintln(stdout, "\nCommands:")
	fmt.Fprintln(stdout, "  apw channel                                        Open interactive channel setup menu")
	fmt.Fprintln(stdout, "  apw channel status                                 Show current channel configuration")
	fmt.Fprintln(stdout, "  apw channel set --token <tok> --allow <id1,id2>    Configure & enable Telegram bot")
	fmt.Fprintln(stdout, "  apw channel token <tok>                            Save Telegram Bot Token to .apw/auth.json")
	fmt.Fprintln(stdout, "  apw channel allow <id1,id2>                        Set allowed Telegram numeric user IDs")
	fmt.Fprintln(stdout, "  apw channel enable | disable                       Toggle Telegram gateway on/off")
}

func printChannelUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: apw channel (or apw messaging) [--config path] [command]")
	fmt.Fprintln(w, "\nSubcommands:")
	fmt.Fprintln(w, "  status                                             Show messaging channel status")
	fmt.Fprintln(w, "  set [--token <tok>] [--allow <ids>] [--enable]     Update Telegram token, allowed users, and state")
	fmt.Fprintln(w, "  token <bot-token>                                  Save Telegram Bot Token in .apw/auth.json")
	fmt.Fprintln(w, "  clear-token                                        Remove stored Telegram Bot Token")
	fmt.Fprintln(w, "  allow <id1,id2,...>                                Set allowed Telegram numeric user IDs")
	fmt.Fprintln(w, "  enable                                             Enable the Telegram gateway")
	fmt.Fprintln(w, "  disable                                            Disable the Telegram gateway")
}
