package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds all proxy configuration.
type Config struct {
	// Auth mode: "chatgpt" (OAuth) or "apikey" (OpenAI API key)
	AuthMode string

	// API key mode fields
	OpenAIAPIKey  string
	OpenAIBaseURL string

	// Model mapping
	BigModel   string
	SmallModel string

	// Server
	Port string

	// Runtime: set after auth
	AccessToken string
	AccountID   string
}

// FlagOverrides carries values that were explicitly set on the command
// line. Any non-nil pointer takes precedence over the env/.env layer.
type FlagOverrides struct {
	AuthMode      *string
	OpenAIAPIKey  *string
	OpenAIBaseURL *string
	BigModel      *string
	SmallModel    *string
	Port          *string
}

// Load reads .env and environment variables, then layers CLI flag
// overrides on top. Precedence: flag > env > .env > default.
func Load(overrides FlagOverrides) *Config {
	loadDotEnv(".env")
	cfg := &Config{
		AuthMode:      env("AUTH_MODE", "chatgpt"),
		OpenAIAPIKey:  env("OPENAI_API_KEY", ""),
		OpenAIBaseURL: env("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		BigModel:      env("BIG_MODEL", "gpt-5.4"),
		SmallModel:    env("SMALL_MODEL", "gpt-5.4-mini"),
		Port:          env("PORT", "8082"),
	}

	// Layer flag overrides on top of env/defaults.
	if overrides.AuthMode != nil && *overrides.AuthMode != "" {
		cfg.AuthMode = *overrides.AuthMode
	}
	if overrides.OpenAIAPIKey != nil && *overrides.OpenAIAPIKey != "" {
		cfg.OpenAIAPIKey = *overrides.OpenAIAPIKey
	}
	if overrides.OpenAIBaseURL != nil && *overrides.OpenAIBaseURL != "" {
		cfg.OpenAIBaseURL = *overrides.OpenAIBaseURL
	}
	if overrides.BigModel != nil && *overrides.BigModel != "" {
		cfg.BigModel = *overrides.BigModel
	}
	if overrides.SmallModel != nil && *overrides.SmallModel != "" {
		cfg.SmallModel = *overrides.SmallModel
	}
	if overrides.Port != nil && *overrides.Port != "" {
		cfg.Port = *overrides.Port
	}

	// Normalise auth mode: accept "api" as a friendly alias for the
	// internal "apikey" value so all downstream checks keep working.
	switch strings.ToLower(strings.TrimSpace(cfg.AuthMode)) {
	case "api", "apikey":
		cfg.AuthMode = "apikey"
	case "chatgpt", "":
		cfg.AuthMode = "chatgpt"
	default:
		// Leave as-is; Validate() will complain.
	}

	return cfg
}

// Validate returns a human-readable error when required fields for the
// selected mode are missing. The error message names the CLI flag so
// the user knows exactly what to pass.
func (c *Config) Validate() error {
	switch c.AuthMode {
	case "chatgpt":
		// No secret validation at config time — main.go checks the
		// saved OAuth token separately.
		return nil
	case "apikey":
		if c.OpenAIAPIKey == "" {
			return fmt.Errorf("API key is required in api mode: pass --apiKey=sk-... (or set OPENAI_API_KEY)")
		}
		return nil
	default:
		return fmt.Errorf("unknown --mode %q: expected \"chatgpt\" or \"api\"", c.AuthMode)
	}
}

func (c *Config) IsChatGPT() bool { return c.AuthMode == "chatgpt" }

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
