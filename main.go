package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
			token, err := auth.Login()
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Login failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\n  Login complete. Account ID: %s\n", token.AccountID)
			fmt.Println("  Run './clawgate' to start the proxy.")
			fmt.Println()
			return
		case "logout":
			auth.Logout()
			return
		case "status":
			token, err := auth.LoadToken()
			if err != nil {
				fmt.Println("❌ Not logged in. Run './clawgate login'")
				os.Exit(1)
			}
			fmt.Println("✅ Logged in")
			fmt.Printf("   Account ID: %s\n", token.AccountID)
			if token.IsExpired() {
				fmt.Println("   Token: expired (will auto-refresh)")
			} else {
				fmt.Println("   Token: valid")
			}
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
		flagPort       = fs.String("port", "", "Server port")
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

	overrides := config.FlagOverrides{
		AuthMode:        flagMode,
		OpenAIAPIKey:    flagAPIKey,
		OpenAIBaseURL:   flagBaseURL,
		BigModel:        flagBigModel,
		MidModel:        flagMidModel,
		SmallModel:      flagSmallModel,
		FastMode:        flagFast,
		ReasoningEffort: flagReason,
		Port:            flagPort,
	}
	cfg := config.Load(overrides)
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
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

	// In ChatGPT mode, verify token exists
	if cfg.IsChatGPT() {
		token, err := auth.LoadToken()
		if err != nil {
			fmt.Println("❌ Not logged in. Run './clawgate login' first.")
			os.Exit(1)
		}
		cfg.AccessToken = token.AccessToken
		cfg.AccountID = token.AccountID
	}

	handler := proxy.NewHandler(cfg)
	mux := http.NewServeMux()
	handler.Register(mux)

	addr := ":" + cfg.Port
	mode := "API Key"
	if cfg.IsChatGPT() {
		mode = "ChatGPT Codex (OAuth)"
	}

	listen := "http://localhost:" + cfg.Port
	envLine := "ANTHROPIC_BASE_URL=" + listen
	fmt.Println("┌────────────────────────────────────────────────────┐")
	fmt.Println("│        clawgate — Anthropic ↔ OpenAI Proxy         │")
	fmt.Println("├────────────────────────────────────────────────────┤")
	fmt.Printf("│  Mode:        %-37s│\n", mode)
	fmt.Printf("│  Listening:   %-37s│\n", listen)
	fmt.Printf("│  Big Model:   %-37s│\n", cfg.BigModel)
	fmt.Printf("│  Mid Model:   %-37s│\n", cfg.MidModel)
	fmt.Printf("│  Small Model: %-37s│\n", cfg.SmallModel)
	if cfg.FastMode {
		fmt.Printf("│  Fast Mode:   %-37s│\n", "enabled")
	}
	if cfg.ReasoningEffort != "" {
		fmt.Printf("│  Reasoning:   %-37s│\n", cfg.ReasoningEffort)
	}
	fmt.Println("├────────────────────────────────────────────────────┤")
	fmt.Println("│  Connect Claude Code with:                         │")
	fmt.Printf("│    %-48s│\n", envLine+" \\")
	fmt.Println("│    claude                                          │")
	fmt.Println("└────────────────────────────────────────────────────┘")

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

func printHelp() {
	fmt.Println(`clawgate — Anthropic ↔ OpenAI Proxy

Usage:
  clawgate [flags]            Start the proxy server
  clawgate <command>          Run a subcommand

Commands:
  login     Login with your ChatGPT account (OAuth)
  logout    Remove saved credentials
  status    Check login status
  help      Show this help

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
  --port         Server port (default: 8082)
  --help, -h     Show this help

Examples:
  # ChatGPT Codex mode (default) — uses your ChatGPT subscription
  clawgate

  # OpenAI API key mode
  clawgate --mode=api --apiKey=sk-xxx

  # Custom base URL (Azure, local vLLM, etc.)
  clawgate --mode=api --apiKey=sk-xxx --baseUrl=https://my.endpoint/v1 --port=9000

Environment variables are honoured as a fallback (useful for CI/containers):
  AUTH_MODE, OPENAI_API_KEY, OPENAI_BASE_URL, BIG_MODEL, MID_MODEL, SMALL_MODEL,
  FAST_MODE, REASON, PORT
  (REASONING_EFFORT is also accepted as an alias for REASON.)`)
}
