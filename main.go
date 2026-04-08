package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/goclawgate/clawgate/auth"
	"github.com/goclawgate/clawgate/config"
	"github.com/goclawgate/clawgate/proxy"
)

func main() {
	// Handle CLI subcommands (must come before flag parsing so e.g.
	// `./clawgate login` is never confused for a run invocation).
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			cmdLogin(os.Args[2:])
			return
		case "logout":
			cmdLogout(os.Args[2:])
			return
		case "status":
			cmdStatus(os.Args[2:])
			return
		case "account":
			cmdAccount(os.Args[2:])
			return
		case "help":
			printHelp()
			return
		}
	}

	// Parse run-mode flags. Go's stdlib `flag` natively accepts both
	// `-flag` and `--flag`, and both `--flag=value` and `--flag value`.
	fs := flag.NewFlagSet("clawgate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		flagMode       = fs.String("mode", "", "Auth mode: chatgpt (default) or api")
		flagAPIKey     = fs.String("apiKey", "", "OpenAI API key (required when --mode=api)")
		flagBaseURL    = fs.String("baseUrl", "", "Custom OpenAI-compatible base URL (api mode only)")
		flagBigModel   = fs.String("bigModel", "", "Model for opus requests")
		flagMidModel   = fs.String("midModel", "", "Model for sonnet requests")
		flagSmallModel = fs.String("smallModel", "", "Model for haiku requests")
		flagFast       = fs.Bool("fast", false, "Enable fast mode (service_tier: priority)")
		flagReason     = fs.String("reason", "", "Reasoning effort: none|minimal|low|medium|high|xhigh (reasoning models only)")
		flagHost       = fs.String("host", "", "Bind address (default: 127.0.0.1)")
		flagPort       = fs.String("port", "", "Server port")
		flagAccount    = fs.String("account", "", "Account name to use for this run (ChatGPT mode)")
		flagHelp       = fs.Bool("help", false, "Show help and exit")
	)
	// Silence the default usage dump — we print our own help.
	fs.Usage = func() {}
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			printHelp()
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printHelp()
		os.Exit(1)
	}
	if *flagHelp {
		printHelp()
		return
	}

	// Only override FastMode when --fast was explicitly passed on the
	// command line. fs.Bool always returns a non-nil pointer (defaulting
	// to false), so passing it unconditionally would clobber the env/.env
	// FAST_MODE=1 setting every time clawgate is started without --fast.
	var fastOverride *bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "fast" {
			fastOverride = flagFast
		}
	})

	overrides := config.FlagOverrides{
		AuthMode:        flagMode,
		OpenAIAPIKey:    flagAPIKey,
		OpenAIBaseURL:   flagBaseURL,
		BigModel:        flagBigModel,
		MidModel:        flagMidModel,
		SmallModel:      flagSmallModel,
		FastMode:        fastOverride,
		ReasoningEffort: flagReason,
		Host:            flagHost,
		Port:            flagPort,
		AccountName:     flagAccount,
	}
	cfg := config.Load(overrides)
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if warnings := cfg.CheckModels(); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  WARNING: %s\n", w)
		}
	}
	if warnings := cfg.CheckReasoning(); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  WARNING: %s\n", w)
		}
	}

	// In ChatGPT mode, verify an account exists and pin the name.
	mode := "API Key"
	if cfg.IsChatGPT() {
		store, err := auth.LoadStore()
		if err != nil {
			fmt.Println("Not logged in. Run 'clawgate login' first.")
			os.Exit(1)
		}
		acct, err := store.ResolveAccount(cfg.AccountName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		cfg.AccountName = acct.Name
		mode = fmt.Sprintf("ChatGPT Codex (%s)", acct.Name)
	}

	handler := proxy.NewHandler(cfg)
	mux := http.NewServeMux()
	handler.Register(mux)

	addr := cfg.Host + ":" + cfg.Port

	listen := "http://" + displayHost(cfg.Host) + ":" + cfg.Port
	envLine := "ANTHROPIC_BASE_URL=" + listen
	fmt.Println("┌─────────────────────────────────────────────────────────┐")
	fmt.Println("│        clawgate — Anthropic ↔ OpenAI Proxy              │")
	fmt.Println("├─────────────────────────────────────────────────────────┤")
	fmt.Printf("│  Mode:        %-42s│\n", mode)
	fmt.Printf("│  Listening:   %-42s│\n", listen)
	fmt.Printf("│  Big Model:   %-42s│\n", cfg.BigModel)
	fmt.Printf("│  Mid Model:   %-42s│\n", cfg.MidModel)
	fmt.Printf("│  Small Model: %-42s│\n", cfg.SmallModel)
	if cfg.FastMode {
		fmt.Printf("│  Fast Mode:   %-42s│\n", "enabled")
	}
	if cfg.ReasoningEffort != "" {
		fmt.Printf("│  Reasoning:   %-42s│\n", cfg.ReasoningEffort)
	}
	fmt.Println("├─────────────────────────────────────────────────────────┤")
	fmt.Println("│  Connect Claude Code with:                              │")
	fmt.Printf("│    %-53s│\n", envLine+" \\")
	fmt.Println("│    claude                                               │")
	fmt.Println("└─────────────────────────────────────────────────────────┘")

	srv := &http.Server{Addr: addr, Handler: mux}

	// Graceful shutdown on SIGINT/SIGTERM
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
			os.Exit(1)
		}
	}()

	<-done
	fmt.Println("\nShutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Shutdown error: %v\n", err)
	}
}

// ── Subcommands ─────────────────────────────────────────────────────

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	nameFlag := fs.String("name", "", "Account name (default: \"default\")")
	defaultFlag := fs.Bool("default", false, "Set this account as the default")
	replaceFlag := fs.Bool("replace", false, "Replace existing account with the same name")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	name := strings.ToLower(strings.TrimSpace(*nameFlag))
	if name == "" {
		// Re-authenticate the current default, or create "default".
		store, _ := auth.LoadStore()
		if store != nil && store.GetDefault() != "" {
			name = store.GetDefault()
		} else {
			name = "default"
		}
	}

	if err := auth.ValidateAccountName(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	token, err := auth.Login()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(1)
	}

	store, _ := auth.LoadStore()
	if store == nil {
		store = &auth.Store{Version: 2}
	}

	acct := auth.StoredAccount{Name: name}
	acct.FromToken(token)

	if err := store.UpsertAccount(acct, *replaceFlag); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *defaultFlag || store.GetDefault() == "" {
		store.SetDefault(name)
	}

	if err := auth.SaveStore(store); err != nil {
		fmt.Fprintf(os.Stderr, "Could not save token: %v\n", err)
		fmt.Println("Authenticated (token NOT persisted -- will need re-login)")
		return
	}

	fmt.Println("\n  Authenticated successfully!")
	fmt.Printf("  Account: %s\n", name)
	if token.AccountID != "" {
		fmt.Printf("  Account ID: %s\n", token.AccountID)
	}
	if store.GetDefault() == name {
		fmt.Println("  (set as default)")
	}
	fmt.Println("  Run 'clawgate' to start the proxy.")
	fmt.Println()
}

func cmdLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	accountFlag := fs.String("account", "", "Account to remove")
	allFlag := fs.Bool("all", false, "Remove all accounts")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *allFlag {
		os.Remove(auth.TokenPath())
		fmt.Println("Logged out -- all accounts removed")
		return
	}

	name := *accountFlag
	if name == "" {
		// Remove the default account (or all if only one).
		store, err := auth.LoadStore()
		if err != nil {
			fmt.Println("Not logged in.")
			return
		}
		if len(store.Accounts) == 1 {
			os.Remove(auth.TokenPath())
			fmt.Println("Logged out -- token removed")
			return
		}
		if store.GetDefault() == "" {
			fmt.Fprintf(os.Stderr, "Multiple accounts saved. Specify --account NAME or --all.\n")
			os.Exit(1)
		}
		name = store.GetDefault()
	}

	store, err := auth.LoadStore()
	if err != nil {
		fmt.Println("Not logged in.")
		return
	}
	if err := store.RemoveAccount(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(store.Accounts) == 0 {
		os.Remove(auth.TokenPath())
		fmt.Printf("Logged out -- account %q removed (no accounts remaining)\n", name)
		return
	}
	if err := auth.SaveStore(store); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving store: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Account %q removed\n", name)
	if store.GetDefault() == "" {
		fmt.Println("  No default account set. Run 'clawgate account use NAME' to set one.")
	}
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	accountFlag := fs.String("account", "", "Account to check")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	store, err := auth.LoadStore()
	if err != nil {
		fmt.Println("Not logged in. Run 'clawgate login'")
		os.Exit(1)
	}

	acct, err := store.ResolveAccount(*accountFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	isDefault := store.GetDefault() == acct.Name
	fmt.Println("Logged in")
	fmt.Printf("   Account:    %s", acct.Name)
	if isDefault {
		fmt.Print(" (default)")
	}
	fmt.Println()
	if acct.AccountID != "" {
		fmt.Printf("   Account ID: %s\n", acct.AccountID)
	}
	tok := acct.Token()
	if tok.IsExpired() {
		fmt.Println("   Token: expired (will auto-refresh)")
	} else {
		fmt.Println("   Token: valid")
	}

	if len(store.Accounts) > 1 {
		fmt.Printf("   Total accounts: %d\n", len(store.Accounts))
	}
}

func cmdAccount(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: clawgate account <list|use>\n")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		store, err := auth.LoadStore()
		if err != nil {
			fmt.Println("No accounts saved. Run 'clawgate login'")
			os.Exit(1)
		}
		if len(store.Accounts) == 0 {
			fmt.Println("No accounts saved. Run 'clawgate login'")
			return
		}
		for _, name := range store.AccountNames() {
			if name == store.GetDefault() {
				fmt.Printf("  * %s (default)\n", name)
			} else {
				fmt.Printf("    %s\n", name)
			}
		}

	case "use":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: clawgate account use NAME\n")
			os.Exit(1)
		}
		name := strings.ToLower(strings.TrimSpace(args[1]))
		store, err := auth.LoadStore()
		if err != nil {
			fmt.Println("No accounts saved. Run 'clawgate login'")
			os.Exit(1)
		}
		if err := store.SetDefault(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := auth.SaveStore(store); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Default account set to %q\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown account subcommand: %s\nUsage: clawgate account <list|use>\n", args[0])
		os.Exit(1)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

// displayHost returns the host string to show in the connection hint.
// When the server binds to all interfaces (0.0.0.0, ::, or empty),
// users should connect via localhost, not the wildcard address.
func displayHost(host string) string {
	switch host {
	case "0.0.0.0", "::", "":
		return "localhost"
	}
	return host
}

func printHelp() {
	fmt.Println(`clawgate -- Anthropic <-> OpenAI Proxy

Usage:
  clawgate [flags]            Start the proxy server
  clawgate <command>          Run a subcommand

Commands:
  login [flags]               Login with your ChatGPT account (OAuth)
    --name NAME               Account name (default: "default")
    --default                 Set as the default account
    --replace                 Overwrite existing account with same name
  logout [flags]              Remove saved credentials
    --account NAME            Account to remove
    --all                     Remove all accounts
  status [flags]              Check login status
    --account NAME            Account to check
  account list                List saved accounts
  account use NAME            Set the default account
  help                        Show this help

Flags:
  --mode         Auth mode: chatgpt (default) or api
  --apiKey       OpenAI API key (required when --mode=api)
  --baseUrl      OpenAI-compatible base URL (api mode only,
                 default: https://api.openai.com/v1)
  --bigModel     Model for opus requests (default: gpt-5.4)
  --midModel     Model for sonnet requests (default: gpt-5.3-codex)
  --smallModel   Model for haiku requests (default: gpt-5.2-codex)
  --fast         Enable fast mode (sends service_tier: priority)
  --reason       Reasoning effort for reasoning models. One of:
                 none, minimal, low, medium, high, xhigh.
                 Per-request Anthropic 'thinking' field still wins.
  --account      Account to use for this proxy run (ChatGPT mode)
  --host         Bind address (default: 127.0.0.1)
  --port         Server port (default: 8082)
  --help, -h     Show this help

Examples:
  # ChatGPT Codex mode (default)
  clawgate login
  clawgate

  # Multiple accounts
  clawgate login --name work
  clawgate login --name personal --default
  clawgate account list
  clawgate account use work
  clawgate --account personal

  # OpenAI API key mode
  clawgate --mode=api --apiKey=sk-xxx

  # Custom base URL (Azure, local vLLM, etc.)
  clawgate --mode=api --apiKey=sk-xxx --baseUrl=https://my.endpoint/v1 --port=9000

Environment variables are honoured as a fallback (useful for CI/containers):
  AUTH_MODE, OPENAI_API_KEY, OPENAI_BASE_URL, BIG_MODEL, MID_MODEL, SMALL_MODEL,
  FAST_MODE, REASON, HOST, PORT
  (REASONING_EFFORT is also accepted as an alias for REASON.)`)
}
