package config

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// CodexModels lists the OpenAI models accepted by the ChatGPT Codex
// backend at https://chatgpt.com/backend-api/codex/responses. Anything
// outside this set is silently rejected upstream with an opaque error,
// so we warn at startup. Update this list as new Codex models ship.
var CodexModels = map[string]bool{
	"gpt-5.1-codex-max":  true,
	"gpt-5.1-codex-mini": true,
	"gpt-5.2-codex":      true,
	"gpt-5.3-codex":      true,
	"gpt-5.4":            true,
}

// ReasoningEfforts is the canonical set of reasoning effort levels
// accepted by the Codex / OpenAI reasoning models. Mirrors Codex CLI's
// `model_reasoning_effort` vocabulary verbatim. Soft-checked at startup
// so the catalog can grow without requiring a proxy release.
var ReasoningEfforts = map[string]bool{
	"none":    true,
	"minimal": true,
	"low":     true,
	"medium":  true,
	"high":    true,
	"xhigh":   true,
}

// Config holds all proxy configuration.
type Config struct {
	// Auth mode: "chatgpt" (OAuth) or "apikey" (OpenAI API key)
	AuthMode string

	// API key mode fields
	OpenAIAPIKey  string
	OpenAIBaseURL string

	// Model mapping
	BigModel   string
	MidModel   string
	SmallModel string

	// Fast mode — sends service_tier: "priority" in API requests
	FastMode bool

	// Reasoning effort — one of none|minimal|low|medium|high|xhigh.
	// Empty means "leave the field unset, let upstream use its default".
	// Mirrors Codex CLI's model_reasoning_effort vocabulary.
	ReasoningEffort string

	// Server
	Host string
	Port string

	// AccountName selects which stored account to use (ChatGPT mode).
	// Empty means "use the default account".
	AccountName string
}

// FlagOverrides carries values that were explicitly set on the command
// line. Any non-nil pointer takes precedence over the env/.env layer.
type FlagOverrides struct {
	AuthMode        *string
	OpenAIAPIKey    *string
	OpenAIBaseURL   *string
	BigModel        *string
	MidModel        *string
	SmallModel      *string
	FastMode        *bool
	ReasoningEffort *string
	Host            *string
	Port            *string
	AccountName     *string
}

// Load reads .env and environment variables, then layers CLI flag
// overrides on top. Precedence: flag > env > .env > default.
func Load(overrides FlagOverrides) *Config {
	loadDotEnv(".env")
	// REASON is the canonical env var; REASONING_EFFORT is accepted as a
	// discoverability alias (mirrors how some users learn the feature).
	reasonEnv := env("REASON", "")
	if reasonEnv == "" {
		reasonEnv = env("REASONING_EFFORT", "")
	}
	cfg := &Config{
		AuthMode:        env("AUTH_MODE", "chatgpt"),
		OpenAIAPIKey:    env("OPENAI_API_KEY", ""),
		OpenAIBaseURL:   env("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		BigModel:        env("BIG_MODEL", "gpt-5.4"),
		MidModel:        env("MID_MODEL", "gpt-5.3-codex"),
		SmallModel:      env("SMALL_MODEL", "gpt-5.2-codex"),
		FastMode:        envBool("FAST_MODE"),
		ReasoningEffort: reasonEnv,
		Host:            env("HOST", "127.0.0.1"),
		Port:            env("PORT", "8082"),
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
	if overrides.MidModel != nil && *overrides.MidModel != "" {
		cfg.MidModel = *overrides.MidModel
	}
	if overrides.SmallModel != nil && *overrides.SmallModel != "" {
		cfg.SmallModel = *overrides.SmallModel
	}
	if overrides.FastMode != nil {
		cfg.FastMode = *overrides.FastMode
	}
	if overrides.ReasoningEffort != nil && *overrides.ReasoningEffort != "" {
		cfg.ReasoningEffort = *overrides.ReasoningEffort
	}
	if overrides.Host != nil && *overrides.Host != "" {
		cfg.Host = *overrides.Host
	}
	if overrides.Port != nil && *overrides.Port != "" {
		cfg.Port = *overrides.Port
	}
	if overrides.AccountName != nil && *overrides.AccountName != "" {
		cfg.AccountName = *overrides.AccountName
	}

	// Normalise the reasoning effort: lowercased & trimmed so the
	// allowlist check and wire payload always agree on the form.
	cfg.ReasoningEffort = strings.ToLower(strings.TrimSpace(cfg.ReasoningEffort))

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

// CheckModels returns human-readable warnings for any configured model
// that the ChatGPT Codex backend will reject. It is a soft check — the
// allowlist may be stale, so warnings are printed but never fatal.
// Returns nil for API key mode (any model is fair game there).
func (c *Config) CheckModels() []string {
	if !c.IsChatGPT() {
		return nil
	}
	var warnings []string
	seen := map[string]bool{}
	check := func(flag, model string) {
		if model == "" || seen[model] || CodexModels[model] {
			return
		}
		seen[model] = true
		warnings = append(warnings, fmt.Sprintf(
			"%s=%q is not a known Codex model. ChatGPT mode only accepts: %s. Use --mode=api with an OpenAI API key for other models.",
			flag, model, codexModelList(),
		))
	}
	check("--bigModel", c.BigModel)
	check("--midModel", c.MidModel)
	check("--smallModel", c.SmallModel)
	return warnings
}

// CheckReasoning returns a single-element warning slice when the user
// has set --reason / REASON to a value the proxy does not recognise.
// The check applies in both auth modes (the value is forwarded
// regardless). Returns nil when unset or valid.
//
// Soft-only: the upstream catalog can grow (e.g. a future "ultra")
// without requiring a proxy release, so we never block startup.
func (c *Config) CheckReasoning() []string {
	if c.ReasoningEffort == "" {
		return nil
	}
	if ReasoningEfforts[c.ReasoningEffort] {
		return nil
	}
	return []string{fmt.Sprintf(
		"--reason=%q is not a known reasoning effort. Expected one of: %s. Forwarding anyway — upstream may reject it.",
		c.ReasoningEffort, reasoningEffortList(),
	)}
}

// reasoningEffortList returns the known efforts as a comma-separated
// string in canonical (low → high) order rather than alphabetical.
func reasoningEffortList() string {
	// Stable, intuitive order — not alphabetical.
	return "none, minimal, low, medium, high, xhigh"
}

// codexModelList returns the known Codex models as a sorted,
// comma-separated string suitable for embedding in user messages.
func codexModelList() string {
	models := make([]string, 0, len(CodexModels))
	for m := range CodexModels {
		models = append(models, m)
	}
	sort.Strings(models)
	return strings.Join(models, ", ")
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

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
