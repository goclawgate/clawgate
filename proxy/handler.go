package proxy

import (
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goclawgate/clawgate/auth"
	"github.com/goclawgate/clawgate/config"
)

// debug is evaluated once at startup so per-request hot paths don't
// pay the cost of re-reading the environment on every call.
var debug = os.Getenv("DEBUG") == "1"

// Handler holds the proxy HTTP handlers.
type Handler struct {
	Cfg         *config.Config
	Client      *http.Client
	mu          sync.RWMutex
	sessionOnce sync.Once
	session     string // lazily generated per-process session id
}

// randomSessionID returns a UUID-shaped random string suitable for use
// as the upstream `session_id` header.
func randomSessionID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	// Set version (4) and variant bits per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

// NewHandler creates a handler with a configured HTTP client.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		Cfg: cfg,
		Client: &http.Client{
			// Reasoning models can think for many minutes; rely on the
			// per-request context (cancelled when the client disconnects)
			// rather than a hard timeout.
			Timeout: 0,
		},
	}
}

// Register mounts all routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/messages", h.handleMessages)
	mux.HandleFunc("/v1/messages/count_tokens", h.handleTokenCount)
	mux.HandleFunc("/", h.handleRoot)
}

// getToken returns a valid access token, refreshing if needed (ChatGPT mode).
//
// Refresh is guarded by the handler's write lock with a double-check so
// that multiple concurrent requests arriving while the token is expired
// don't all call auth.Refresh — only the first goroutine to acquire the
// write lock actually talks to the OAuth server; the rest observe the
// freshly refreshed token on re-check and return immediately.
func (h *Handler) getToken() (string, string, error) {
	if !h.Cfg.IsChatGPT() {
		return h.Cfg.OpenAIAPIKey, "", nil
	}

	// Fast path: read the current token under the read lock.
	h.mu.RLock()
	token, err := auth.LoadToken()
	h.mu.RUnlock()
	if err != nil {
		return "", "", fmt.Errorf("no saved token — run './clawgate login' first")
	}

	if token.IsExpired() {
		// Slow path: acquire the write lock, then re-check. If another
		// goroutine already refreshed the token while we were waiting,
		// we'll see the fresh value here and skip the refresh.
		h.mu.Lock()
		token, err = auth.LoadToken()
		if err != nil {
			h.mu.Unlock()
			return "", "", fmt.Errorf("no saved token — run './clawgate login' first")
		}
		if token.IsExpired() {
			fmt.Println("🔄 Refreshing access token...")
			newToken, refreshErr := auth.Refresh(token)
			if refreshErr != nil {
				h.mu.Unlock()
				return "", "", fmt.Errorf("token refresh failed: %w", refreshErr)
			}
			if saveErr := auth.SaveToken(newToken); saveErr != nil {
				fmt.Printf("  ⚠️  Could not persist refreshed token: %v\n", saveErr)
			}
			token = newToken
		}
		h.Cfg.AccessToken = token.AccessToken
		h.Cfg.AccountID = token.AccountID
		h.mu.Unlock()
	}

	return token.AccessToken, token.AccountID, nil
}

// buildRequest creates the upstream HTTP request based on auth mode.
func (h *Handler) buildRequest(r *http.Request, oaiBody []byte) (*http.Request, error) {
	accessToken, accountID, err := h.getToken()
	if err != nil {
		return nil, err
	}

	var targetURL string
	if h.Cfg.IsChatGPT() {
		targetURL = auth.CodexEndpoint
	} else {
		targetURL = strings.TrimSuffix(h.Cfg.OpenAIBaseURL, "/") + "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(oaiBody))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	if h.Cfg.IsChatGPT() {
		// ChatGPT Codex-specific headers required by the Codex
		// Responses API endpoint.
		if accountID != "" {
			httpReq.Header.Set("ChatGPT-Account-Id", accountID)
		}
		httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
		httpReq.Header.Set("originator", "clawgate")
		httpReq.Header.Set("User-Agent", fmt.Sprintf("clawgate/%s (%s %s)", auth.Version, runtime.GOOS, runtime.GOARCH))
		// A stable session id helps backend rate-limiting bookkeeping.
		httpReq.Header.Set("session_id", h.sessionID())
	}

	return httpReq, nil
}

// sessionID returns the (lazily generated) per-process session id used
// in upstream Codex requests.
func (h *Handler) sessionID() string {
	h.sessionOnce.Do(func() { h.session = randomSessionID() })
	return h.session
}

// ── POST /v1/messages ────────────────────────────────────────────────

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if debug {
		incomingBody, _ := json.MarshalIndent(req, "", "  ")
		fmt.Printf("\n--- [DEBUG] INCOMING ANTHROPIC REQUEST ---\n%s\n------------------------------------------\n", string(incomingBody))
	}

	oaiReq, originalModel, mappedModel := TranslateRequest(&req, h.Cfg)

	// Log
	origDisplay := originalModel
	for _, p := range []string{"anthropic/", "openai/", "gemini/"} {
		origDisplay = strings.TrimPrefix(origDisplay, p)
	}
	mode := "API Key"
	if h.Cfg.IsChatGPT() {
		mode = "Codex"
	}
	fmt.Printf("\033[1mPOST /v1/messages\033[0m \033[92m✓\033[0m [%s]\n", mode)
	fmt.Printf("\033[96m%s\033[0m → \033[92m%s\033[0m\n", origDisplay, mappedModel)

	oaiBody, err := json.Marshal(oaiReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if debug {
		pretty, _ := json.MarshalIndent(oaiReq, "", "  ")
		fmt.Printf("\n--- [DEBUG] OUTGOING REQUEST TO MODEL ---\n%s\n---------------------------------------\n", string(pretty))
	}

	const maxRetries = 3
	const baseBackoff = 500 * time.Millisecond
	var resp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		httpReq, err := h.buildRequest(r, oaiBody)
		if err != nil {
			writeAnthropicError(w, http.StatusUnauthorized, err.Error())
			return
		}

		resp, err = h.Client.Do(httpReq)
		if err != nil {
			writeAnthropicError(w, http.StatusBadGateway, "upstream request failed: "+err.Error())
			return
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt == maxRetries {
			break
		}

		// 429 — retry with backoff
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		backoff := parseRetryAfter(resp.Header.Get("Retry-After"))
		if backoff == 0 {
			backoff = baseBackoff << uint(attempt) // 500ms, 1s, 2s
		}
		fmt.Printf("  429 rate limited, retry %d/%d in %v\n", attempt+1, maxRetries, backoff)
		_ = respBody // consumed and discarded

		select {
		case <-time.After(backoff):
			// continue to next attempt
		case <-r.Context().Done():
			writeAnthropicError(w, http.StatusGatewayTimeout, "client disconnected during retry backoff")
			return
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		msg := extractUpstreamErrorMessage(resp.StatusCode, respBody)
		writeAnthropicError(w, resp.StatusCode, msg)
		return
	}

	// Streaming
	if req.Stream {
		HandleStream(w, resp.Body, originalModel, h.Cfg.IsChatGPT())
		return
	}

	// Non-streaming
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "failed to read response")
		return
	}

	if debug {
		fmt.Printf("\n--- [DEBUG] UPSTREAM RESPONSE ---\n%s\n----------------------------------\n", string(respBody))
	}

	w.Header().Set("Content-Type", "application/json")

	if h.Cfg.IsChatGPT() {
		// Codex Responses API uses an `output[]` shape, NOT `choices[]`.
		var codexResp CodexResponse
		if err := json.Unmarshal(respBody, &codexResp); err != nil {
			writeAnthropicError(w, http.StatusBadGateway, "failed to parse codex response: "+err.Error())
			return
		}
		if codexResp.Error != nil {
			writeAnthropicError(w, http.StatusBadGateway, fmt.Sprintf("codex error: %s — %s", codexResp.Error.Code, codexResp.Error.Message))
			return
		}
		anthResp := TranslateCodexResponse(&codexResp, originalModel)
		json.NewEncoder(w).Encode(anthResp)
		return
	}

	var oaiResp OpenAIResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "failed to parse response: "+err.Error())
		return
	}

	anthResp := TranslateResponse(&oaiResp, originalModel)
	json.NewEncoder(w).Encode(anthResp)
}

// ── POST /v1/messages/count_tokens ───────────────────────────────────

// handleTokenCount returns an *estimate* of the prompt token count for
// an Anthropic request. We deliberately do not proxy this to upstream:
// ChatGPT Codex has no count_tokens endpoint, and round-tripping an
// OpenAI request purely to satisfy a budgeting call is wasteful. The
// estimate walks the request's actual text payload (system, message
// content text blocks, tool definitions) rather than counting raw JSON
// bytes, which would otherwise include heavy punctuation and base64
// image overhead.
func (h *Handler) handleTokenCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	estimated := estimateAnthropicTokens(body)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"input_tokens": estimated})
}

// estimateAnthropicTokens returns a best-effort token count for an
// Anthropic request body. If the body does not parse as AnthropicRequest
// we fall back to the crude len(body)/4 heuristic so existing (admittedly
// broken) callers don't break.
func estimateAnthropicTokens(body []byte) int {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		n := len(body) / 4
		if n < 1 {
			n = 1
		}
		return n
	}

	chars := 0

	// System prompt (string or array of text blocks).
	if len(req.System) > 0 {
		chars += len(extractSystemText(req.System))
	}

	// Messages — walk text blocks (ignore images, which are counted
	// differently upstream and aren't meaningful to a char heuristic).
	const perMessageOverhead = 4 // rough "role framing" allowance per tiktoken
	for _, msg := range req.Messages {
		chars += perMessageOverhead * 4 // multiply by 4 so /4 below gives +4 tokens
		blocks := parseContentBlocks(msg.Content)
		if blocks == nil {
			chars += len(extractStringContent(msg.Content))
			continue
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				chars += len(b.Text)
			case "tool_use":
				chars += len(b.Name)
				if argsJSON, err := json.Marshal(b.Input); err == nil {
					chars += len(argsJSON)
				}
			case "tool_result":
				chars += len(extractToolResultContent(b.Content))
			}
		}
	}

	// Tool definitions — name + description + serialized schema.
	for _, t := range req.Tools {
		chars += len(t.Name) + len(t.Description)
		if schemaJSON, err := json.Marshal(t.InputSchema); err == nil {
			chars += len(schemaJSON)
		}
	}

	estimated := chars / 4
	if estimated < 1 {
		estimated = 1
	}
	return estimated
}

// ── GET / ────────────────────────────────────────────────────────────

func (h *Handler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "clawgate — Anthropic ↔ OpenAI Proxy",
	})
}

func writeAnthropicError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    anthropicErrorType(status),
			"message": msg,
		},
	})
}

// anthropicErrorType maps HTTP status codes to Anthropic error type strings.
func anthropicErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	default:
		if status >= 500 {
			return "api_error"
		}
		return "api_error"
	}
}

// parseRetryAfter parses a Retry-After header value (seconds) into a
// time.Duration. Returns 0 if the header is empty or unparseable.
func parseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(header, 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// extractUpstreamErrorMessage attempts to parse the upstream JSON error body
// and return a human-readable message. Falls back to a generic description
// if parsing fails.
func extractUpstreamErrorMessage(statusCode int, body []byte) string {
	// Try OpenAI-style: {"error": {"message": "...", "code": "..."}}
	var oaiErr struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &oaiErr); err == nil && oaiErr.Error.Message != "" {
		return oaiErr.Error.Message
	}

	// Try Codex / Responses API style: {"detail": "..."}
	var detailErr struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &detailErr); err == nil && detailErr.Detail != "" {
		return detailErr.Detail
	}

	// Fallback: generic message per status code
	switch statusCode {
	case http.StatusTooManyRequests:
		return "Rate limited by upstream provider. Please wait and try again."
	case http.StatusUnauthorized:
		return "Authentication failed with upstream provider."
	case http.StatusForbidden:
		return "Access denied by upstream provider."
	case http.StatusServiceUnavailable:
		return "Upstream provider is temporarily unavailable."
	default:
		return fmt.Sprintf("Upstream request failed with status %d.", statusCode)
	}
}
