package proxy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/goclawgate/clawgate/config"
)

func newCfg(chatgpt bool) *config.Config {
	mode := "chatgpt"
	if !chatgpt {
		mode = "apikey"
	}
	return &config.Config{
		AuthMode:   mode,
		BigModel:   "gpt-5.4",
		MidModel:   "gpt-5.3-codex",
		SmallModel: "gpt-5.2-codex",
	}
}

func TestMapModelDispatch(t *testing.T) {
	cfg := newCfg(true)
	cases := map[string]string{
		"claude-3-5-sonnet-latest": "gpt-5.3-codex",
		"claude-haiku-4-5":         "gpt-5.2-codex",
		"claude-opus-4-6":          "gpt-5.4",
		"anthropic/claude-3-haiku": "gpt-5.2-codex",
		"some-unknown-model":       "gpt-5.4",
	}
	for in, want := range cases {
		if got := MapModel(in, cfg); got != want {
			t.Errorf("MapModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsReasoningModel(t *testing.T) {
	for _, m := range []string{"o1-preview", "o3-mini", "o4", "gpt-5.4", "gpt-5.2-codex", "gpt-5.1-codex"} {
		if !isReasoningModel(m) {
			t.Errorf("isReasoningModel(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"gpt-4", "gpt-4o", "chatgpt-4o-latest"} {
		if isReasoningModel(m) {
			t.Errorf("isReasoningModel(%q) = true, want false", m)
		}
	}
}

func TestReasoningEffortFromThinking(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want string
	}{
		{"nil", nil, ""},
		{"disabled", map[string]interface{}{"type": "disabled"}, ""},
		{"enabled no budget", map[string]interface{}{"type": "enabled"}, "medium"},
		{"xhigh: budget=128000", map[string]interface{}{"type": "enabled", "budget_tokens": float64(128000)}, "xhigh"},
		{"xhigh: budget=32000 (boundary)", map[string]interface{}{"type": "enabled", "budget_tokens": float64(32000)}, "xhigh"},
		{"high: budget=20000", map[string]interface{}{"type": "enabled", "budget_tokens": float64(20000)}, "high"},
		{"high: budget=10000 (boundary)", map[string]interface{}{"type": "enabled", "budget_tokens": float64(10000)}, "high"},
		{"medium: budget=8000", map[string]interface{}{"type": "enabled", "budget_tokens": float64(8000)}, "medium"},
		{"medium: budget=4000 (boundary)", map[string]interface{}{"type": "enabled", "budget_tokens": float64(4000)}, "medium"},
		{"low: budget=2000", map[string]interface{}{"type": "enabled", "budget_tokens": float64(2000)}, "low"},
		{"low: budget=1000 (boundary)", map[string]interface{}{"type": "enabled", "budget_tokens": float64(1000)}, "low"},
		{"minimal: budget=500", map[string]interface{}{"type": "enabled", "budget_tokens": float64(500)}, "minimal"},
		{"minimal: budget=1", map[string]interface{}{"type": "enabled", "budget_tokens": float64(1)}, "minimal"},
		{"none: budget=0", map[string]interface{}{"type": "enabled", "budget_tokens": float64(0)}, "none"},
		{"none: budget=-1", map[string]interface{}{"type": "enabled", "budget_tokens": float64(-1)}, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reasoningEffortFromThinking(tt.in); got != tt.want {
				t.Errorf("reasoningEffortFromThinking(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractToolResultContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain string", `"hello world"`, "hello world"},
		{"array of text", `[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]`, "line1\nline2"},
		{"single block", `{"type":"text","text":"only"}`, "only"},
		{"object", `{"foo":"bar"}`, `{"foo":"bar"}`},
		{"empty", ``, ""},
	}
	for _, c := range cases {
		got := extractToolResultContent(json.RawMessage(c.in))
		if got != c.want {
			t.Errorf("%s: extractToolResultContent(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestImageSourceToDataURL(t *testing.T) {
	got := imageSourceToDataURL(map[string]interface{}{
		"type":       "base64",
		"media_type": "image/png",
		"data":       "AAAA",
	})
	if got != "data:image/png;base64,AAAA" {
		t.Errorf("base64 image: got %q", got)
	}
	got = imageSourceToDataURL(map[string]interface{}{
		"type": "url",
		"url":  "https://example.com/foo.png",
	})
	if got != "https://example.com/foo.png" {
		t.Errorf("url image: got %q", got)
	}
	if imageSourceToDataURL(nil) != "" {
		t.Errorf("nil source should return empty")
	}
}

func TestTranslateRequestCodexCombinesUserText(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"text","text":"line A"},
				{"type":"text","text":"line B"}
			]`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex, ok := out.(*CodexRequest)
	if !ok {
		t.Fatalf("expected *CodexRequest, got %T", out)
	}
	// Should be ONE message with TWO content parts (not two messages).
	if len(codex.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(codex.Input))
	}
	msg, ok := codex.Input[0].(CodexReqMessage)
	if !ok {
		t.Fatalf("expected CodexReqMessage, got %T", codex.Input[0])
	}
	if len(msg.Content) != 2 {
		t.Errorf("expected 2 content parts, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != "input_text" || msg.Content[0].Text != "line A" {
		t.Errorf("first part wrong: %+v", msg.Content[0])
	}
}

func TestTranslateRequestCodexImage(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"text","text":"describe"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}
			]`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	if len(codex.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(codex.Input))
	}
	msg := codex.Input[0].(CodexReqMessage)
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.Content))
	}
	if msg.Content[1].Type != "input_image" {
		t.Errorf("expected input_image second part, got %+v", msg.Content[1])
	}
	if !strings.HasPrefix(msg.Content[1].ImageURL, "data:image/png;base64,") {
		t.Errorf("image url malformed: %q", msg.Content[1].ImageURL)
	}
}

func TestTranslateRequestCodexToolRoundtrip(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"call list_files"`)},
			{Role: "assistant", Content: json.RawMessage(`[
				{"type":"text","text":"calling tool"},
				{"type":"tool_use","id":"call_1","name":"list_files","input":{"path":"."}}
			]`)},
			{Role: "user", Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"call_1","content":"a.txt\nb.txt"}
			]`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	// Expect: user msg, assistant text, function_call, function_call_output
	if len(codex.Input) != 4 {
		for i, it := range codex.Input {
			t.Logf("input[%d] = %T %+v", i, it, it)
		}
		t.Fatalf("expected 4 input items, got %d", len(codex.Input))
	}
	if _, ok := codex.Input[2].(CodexFunctionCall); !ok {
		t.Errorf("expected CodexFunctionCall at [2], got %T", codex.Input[2])
	}
	out2, ok := codex.Input[3].(CodexFunctionCallOutput)
	if !ok {
		t.Errorf("expected CodexFunctionCallOutput at [3], got %T", codex.Input[3])
	}
	if out2.Output != "a.txt\nb.txt" {
		t.Errorf("expected tool output a.txt\\nb.txt, got %q", out2.Output)
	}
	if out2.CallID != "call_1" {
		t.Errorf("expected call_id call_1, got %q", out2.CallID)
	}
}

func TestResolveReasoningEffort_FlagOnly(t *testing.T) {
	cfg := newCfg(true)
	cfg.ReasoningEffort = "high"
	req := &AnthropicRequest{}
	if got := resolveReasoningEffort(cfg, req); got != "high" {
		t.Errorf("expected high, got %q", got)
	}
}

func TestResolveReasoningEffort_ThinkingHigherThanFlag(t *testing.T) {
	cfg := newCfg(true)
	cfg.ReasoningEffort = "low"
	req := &AnthropicRequest{
		Thinking: map[string]interface{}{"type": "enabled", "budget_tokens": float64(32000)},
	}
	if got := resolveReasoningEffort(cfg, req); got != "xhigh" {
		t.Errorf("expected thinking to win with xhigh, got %q", got)
	}
}

func TestResolveReasoningEffort_FlagHigherThanThinking(t *testing.T) {
	cfg := newCfg(true)
	cfg.ReasoningEffort = "xhigh"
	req := &AnthropicRequest{
		Thinking: map[string]interface{}{"type": "enabled", "budget_tokens": float64(10000)},
	}
	if got := resolveReasoningEffort(cfg, req); got != "xhigh" {
		t.Errorf("expected flag to win with xhigh, got %q", got)
	}
}

func TestResolveReasoningEffort_BothEqual(t *testing.T) {
	cfg := newCfg(true)
	cfg.ReasoningEffort = "high"
	req := &AnthropicRequest{
		Thinking: map[string]interface{}{"type": "enabled", "budget_tokens": float64(15000)},
	}
	if got := resolveReasoningEffort(cfg, req); got != "high" {
		t.Errorf("expected high, got %q", got)
	}
}

func TestEffortRank(t *testing.T) {
	tests := []struct {
		effort string
		rank   int
	}{
		{"none", 0},
		{"minimal", 1},
		{"low", 2},
		{"medium", 3},
		{"high", 4},
		{"xhigh", 5},
		{"", -1},
		{"unknown", -1},
	}
	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			if got := effortRank(tt.effort); got != tt.rank {
				t.Errorf("effortRank(%q) = %d, want %d", tt.effort, got, tt.rank)
			}
		})
	}
}

func TestResolveReasoningEffort_NeitherSet(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{}
	if got := resolveReasoningEffort(cfg, req); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestTranslateRequestCodex_ReasoningFlagApplied(t *testing.T) {
	cfg := newCfg(true)
	cfg.ReasoningEffort = "xhigh"
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex, ok := out.(*CodexRequest)
	if !ok {
		t.Fatalf("expected *CodexRequest, got %T", out)
	}
	if codex.Reasoning == nil {
		t.Fatal("expected codex.Reasoning to be set")
	}
	if codex.Reasoning.Effort != "xhigh" {
		t.Errorf("expected effort=xhigh, got %q", codex.Reasoning.Effort)
	}
	if codex.Reasoning.Summary != "auto" {
		t.Errorf("expected summary=auto, got %q", codex.Reasoning.Summary)
	}
}

func TestTranslateRequestCodex_ReasoningFlagIgnoredForNonReasoningModel(t *testing.T) {
	cfg := newCfg(false) // api key mode
	cfg.BigModel = "gpt-4o"
	cfg.ReasoningEffort = "high"
	req := &AnthropicRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	oai, ok := out.(*OpenAIRequest)
	if !ok {
		t.Fatalf("expected *OpenAIRequest, got %T", out)
	}
	if oai.ReasoningEffort != "" {
		t.Errorf("expected empty ReasoningEffort for non-reasoning model gpt-4o, got %q", oai.ReasoningEffort)
	}
}

func TestTranslateRequestOpenAI_ReasoningFlagApplied(t *testing.T) {
	cfg := newCfg(false) // api key mode
	cfg.BigModel = "gpt-5.4"
	cfg.ReasoningEffort = "medium"
	temp := 0.7
	topP := 0.9
	req := &AnthropicRequest{
		Model:       "claude-opus-4-6",
		MaxTokens:   1024,
		Temperature: &temp,
		TopP:        &topP,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	oai, ok := out.(*OpenAIRequest)
	if !ok {
		t.Fatalf("expected *OpenAIRequest, got %T", out)
	}
	if oai.ReasoningEffort != "medium" {
		t.Errorf("expected ReasoningEffort=medium, got %q", oai.ReasoningEffort)
	}
	// Reasoning models still drop temperature/top_p.
	if oai.Temperature != nil {
		t.Errorf("temperature should be nil for reasoning model, got %v", *oai.Temperature)
	}
	if oai.TopP != nil {
		t.Errorf("top_p should be nil for reasoning model, got %v", *oai.TopP)
	}
}

func TestTranslateRequestCodexReasoningDropsTemperature(t *testing.T) {
	cfg := newCfg(true)
	temp := 0.7
	req := &AnthropicRequest{
		Model:       "claude-3-5-sonnet",
		MaxTokens:   1024,
		Temperature: &temp,
		Thinking:    map[string]interface{}{"type": "enabled", "budget_tokens": float64(20000)},
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	if codex.Temperature != nil {
		t.Errorf("temperature should be nil for reasoning model, got %v", *codex.Temperature)
	}
	if codex.Reasoning == nil || codex.Reasoning.Effort != "high" {
		t.Errorf("expected reasoning.effort=high, got %+v", codex.Reasoning)
	}
}

func TestTranslateCodexResponseWithToolCall(t *testing.T) {
	resp := &CodexResponse{
		ID:    "resp_xyz",
		Model: "gpt-5.4",
		Output: []CodexOutputItem{
			{
				Type: "message",
				Role: "assistant",
				ID:   "msg_1",
				Content: []CodexOutputContent{
					{Type: "output_text", Text: "Sure, I'll help."},
				},
			},
			{
				Type:      "function_call",
				ID:        "fc_1",
				CallID:    "call_42",
				Name:      "list_files",
				Arguments: `{"path":"."}`,
			},
		},
		Usage: &CodexUsage{
			InputTokens:        100,
			OutputTokens:       42,
			InputTokensDetails: &CodexInputTokenDetails{CachedTokens: 30},
		},
	}
	out := TranslateCodexResponse(resp, "claude-3-5-sonnet")
	if out.ID != "resp_xyz" {
		t.Errorf("ID mismatch: %q", out.ID)
	}
	if out.Model != "claude-3-5-sonnet" {
		t.Errorf("Model not preserved")
	}
	if len(out.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(out.Content))
	}
	if out.Content[0]["type"] != "text" {
		t.Errorf("first block should be text")
	}
	if out.Content[1]["type"] != "tool_use" {
		t.Errorf("second block should be tool_use")
	}
	if out.StopReason == nil || *out.StopReason != "tool_use" {
		t.Errorf("stop_reason should be tool_use, got %v", out.StopReason)
	}
	// 100 - 30 cache = 70 fresh input
	if out.Usage.InputTokens != 70 {
		t.Errorf("expected fresh input 70, got %d", out.Usage.InputTokens)
	}
	if out.Usage.CacheReadInputTokens != 30 {
		t.Errorf("expected cache read 30, got %d", out.Usage.CacheReadInputTokens)
	}
}

func TestTranslateCodexResponseIncomplete(t *testing.T) {
	resp := &CodexResponse{
		ID:    "resp_x",
		Model: "gpt-5.4",
		Output: []CodexOutputItem{
			{Type: "message", Content: []CodexOutputContent{{Type: "output_text", Text: "partial"}}},
		},
		IncompleteDetails: &CodexIncompleteDet{Reason: "max_output_tokens"},
		Usage:             &CodexUsage{InputTokens: 10, OutputTokens: 5},
	}
	out := TranslateCodexResponse(resp, "claude-3-5-sonnet")
	if out.StopReason == nil || *out.StopReason != "max_tokens" {
		t.Errorf("expected stop_reason max_tokens, got %v", out.StopReason)
	}
}

func TestMapFinishReasonContentFilter(t *testing.T) {
	cases := map[string]string{
		"stop":           "end_turn",
		"length":         "max_tokens",
		"tool_calls":     "tool_use",
		"content_filter": "end_turn",
		"unknown":        "end_turn",
	}
	for in, want := range cases {
		if got := mapFinishReason(in); got != want {
			t.Errorf("mapFinishReason(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"1", 1 * time.Second},
		{"2.5", 2500 * time.Millisecond},
		{"0.5", 500 * time.Millisecond},
		{"", 0},
		{"invalid", 0},
		{"-1", 0},
		{"  3  ", 3 * time.Second},
	}
	for _, c := range cases {
		got := parseRetryAfter(c.in)
		if got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestExtractUpstreamErrorMessage(t *testing.T) {
	// OpenAI format
	oaiBody := []byte(`{"error":{"message":"Rate limit exceeded","code":"rate_limit"}}`)
	got := extractUpstreamErrorMessage(429, oaiBody)
	if got != "Rate limit exceeded" {
		t.Errorf("OpenAI format: got %q", got)
	}

	// Codex format
	codexBody := []byte(`{"detail":"Too many requests"}`)
	got = extractUpstreamErrorMessage(429, codexBody)
	if got != "Too many requests" {
		t.Errorf("Codex format: got %q", got)
	}

	// Fallback
	got = extractUpstreamErrorMessage(429, []byte(`not json`))
	if !strings.Contains(got, "Rate limited") {
		t.Errorf("Fallback: got %q", got)
	}

	got = extractUpstreamErrorMessage(503, []byte(`{}`))
	if !strings.Contains(got, "unavailable") {
		t.Errorf("503 fallback: got %q", got)
	}
}

func TestCodexParallelToolCallsEnabled(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"test"`)},
		},
		Tools: []AnthropicTool{
			{Name: "read_file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	if codex.ParallelToolCall == nil || !*codex.ParallelToolCall {
		t.Error("expected ParallelToolCall to be true when tools present")
	}
}

func TestCodexParallelToolCallsNilWithoutTools(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"test"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	if codex.ParallelToolCall != nil {
		t.Error("expected ParallelToolCall to be nil when no tools")
	}
}

func TestOpenAIImageContentParts(t *testing.T) {
	cfg := newCfg(false)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[
				{"type":"text","text":"describe this"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}
			]`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	oai := out.(*OpenAIRequest)

	// Find the user message with image content
	var found bool
	for _, m := range oai.Messages {
		if m.Role != "user" {
			continue
		}
		parts, ok := m.Content.([]map[string]interface{})
		if !ok {
			continue
		}
		found = true
		if len(parts) != 2 {
			t.Fatalf("expected 2 content parts, got %d", len(parts))
		}
		if parts[0]["type"] != "text" {
			t.Errorf("first part type = %v, want text", parts[0]["type"])
		}
		if parts[1]["type"] != "image_url" {
			t.Errorf("second part type = %v, want image_url", parts[1]["type"])
		}
		imgURL, ok := parts[1]["image_url"].(map[string]interface{})
		if !ok {
			t.Fatal("image_url not a map")
		}
		url, _ := imgURL["url"].(string)
		if !strings.HasPrefix(url, "data:image/png;base64,") {
			t.Errorf("image url malformed: %q", url)
		}
	}
	if !found {
		t.Error("did not find user message with image content parts")
	}
}

func TestMapModelMidTier(t *testing.T) {
	cfg := newCfg(true)
	// Sonnet should map to MidModel, not BigModel
	if got := MapModel("claude-sonnet-4-6", cfg); got != "gpt-5.3-codex" {
		t.Errorf("MapModel(sonnet) = %q, want gpt-5.3-codex", got)
	}
	// Opus should map to BigModel
	if got := MapModel("claude-opus-4-6", cfg); got != "gpt-5.4" {
		t.Errorf("MapModel(opus) = %q, want gpt-5.4", got)
	}
	// Haiku should map to SmallModel
	if got := MapModel("claude-haiku-4-5", cfg); got != "gpt-5.2-codex" {
		t.Errorf("MapModel(haiku) = %q, want gpt-5.2-codex", got)
	}
}

func TestTranslateRequestCodexFastMode(t *testing.T) {
	cfg := newCfg(true)
	cfg.FastMode = true
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex, ok := out.(*CodexRequest)
	if !ok {
		t.Fatalf("expected *CodexRequest, got %T", out)
	}
	if codex.ServiceTier != "priority" {
		t.Errorf("expected service_tier=priority, got %q", codex.ServiceTier)
	}
}

func TestTranslateRequestCodexNoFastMode(t *testing.T) {
	cfg := newCfg(true)
	// FastMode defaults to false
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex, ok := out.(*CodexRequest)
	if !ok {
		t.Fatalf("expected *CodexRequest, got %T", out)
	}
	if codex.ServiceTier != "" {
		t.Errorf("expected empty service_tier, got %q", codex.ServiceTier)
	}
}

// TestTranslateRequestCodexDoesNotForwardMaxTokens verifies that the
// Codex path never includes max_output_tokens in the serialized request.
// The ChatGPT backend-api/codex/responses endpoint rejects the field —
// matching the official Codex CLI and OpenCode's codex plugin (which
// strips it with "Match codex cli"). Marshalling to JSON catches both
// the field being present with a non-zero value and an omitempty zero.
func TestTranslateRequestCodexDoesNotForwardMaxTokens(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex, ok := out.(*CodexRequest)
	if !ok {
		t.Fatalf("expected *CodexRequest, got %T", out)
	}
	body, err := json.Marshal(codex)
	if err != nil {
		t.Fatalf("failed to marshal CodexRequest: %v", err)
	}
	if strings.Contains(string(body), "max_output_tokens") {
		t.Errorf("serialized CodexRequest must NOT contain max_output_tokens, got: %s", string(body))
	}
}

func TestIsAnthropicWebSearchTool(t *testing.T) {
	tests := []struct {
		name string
		tool AnthropicTool
		want bool
	}{
		{"typed web_search", AnthropicTool{Type: "web_search_20250305", Name: "web_search"}, true},
		{"future version", AnthropicTool{Type: "web_search_20260101", Name: "web_search"}, true},
		{"fallback no type", AnthropicTool{Name: "web_search"}, true},
		{"regular function", AnthropicTool{Name: "read_file", InputSchema: map[string]interface{}{"type": "object"}}, false},
		{"web_search with schema", AnthropicTool{Name: "web_search", InputSchema: map[string]interface{}{"type": "object"}}, false},
		{"custom type", AnthropicTool{Type: "custom", Name: "web_search"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnthropicWebSearchTool(tt.tool); got != tt.want {
				t.Errorf("isAnthropicWebSearchTool(%+v) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestTranslateCodexToolsWebSearch(t *testing.T) {
	tools := []AnthropicTool{
		{Type: "web_search_20250305", Name: "web_search"},
		{Name: "read_file", InputSchema: map[string]interface{}{"type": "object"}},
	}
	out := translateCodexTools(tools)
	if len(out) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(out))
	}
	if out[0].Type != "web_search" || out[0].Name != "" {
		t.Errorf("expected native web_search tool, got %+v", out[0])
	}
	if out[1].Type != "function" || out[1].Name != "read_file" {
		t.Errorf("expected function tool, got %+v", out[1])
	}
	// Verify JSON shape matches Codex CLI: {"type":"web_search"}
	b, _ := json.Marshal(out[0])
	if string(b) != `{"type":"web_search"}` {
		t.Errorf("web_search JSON = %s, want {\"type\":\"web_search\"}", string(b))
	}
}

func TestTranslateOpenAIToolsDropsWebSearch(t *testing.T) {
	tools := []AnthropicTool{
		{Type: "web_search_20250305", Name: "web_search"},
		{Name: "read_file", InputSchema: map[string]interface{}{"type": "object"}},
	}
	out := translateOpenAITools(tools)
	if len(out) != 1 {
		t.Fatalf("expected 1 tool (web_search dropped), got %d", len(out))
	}
	if out[0].Function.Name != "read_file" {
		t.Errorf("expected read_file, got %s", out[0].Function.Name)
	}
}

func TestTranslateCodexResponseWebSearchCall(t *testing.T) {
	resp := &CodexResponse{
		ID:    "resp_ws",
		Model: "gpt-5.4",
		Output: []CodexOutputItem{
			{
				Type: "web_search_call",
				ID:   "ws_1",
				Action: map[string]interface{}{
					"type":  "web_search",
					"query": "latest Go release",
					"sources": []interface{}{
						map[string]interface{}{"url": "https://go.dev/blog/go1.24", "title": "Go 1.24 Release Notes"},
						map[string]interface{}{"url": "https://example.com/go", "title": "Go News"},
					},
				},
			},
			{
				Type: "message",
				Role: "assistant",
				Content: []CodexOutputContent{
					{Type: "output_text", Text: "Go 1.24 was released."},
				},
			},
		},
		Usage: &CodexUsage{InputTokens: 50, OutputTokens: 20},
	}
	out := TranslateCodexResponse(resp, "claude-3-5-sonnet")
	// web_search is server-side — stop_reason must be end_turn, not tool_use
	if out.StopReason == nil || *out.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %v", out.StopReason)
	}
	// Should have: server_tool_use + web_search_tool_result + text = 3
	if len(out.Content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d: %+v", len(out.Content), out.Content)
	}
	if out.Content[0]["type"] != "server_tool_use" {
		t.Errorf("first block should be server_tool_use, got %v", out.Content[0]["type"])
	}
	if out.Content[0]["name"] != "web_search" {
		t.Errorf("server_tool_use name should be web_search, got %v", out.Content[0]["name"])
	}
	input, _ := out.Content[0]["input"].(map[string]interface{})
	if input["query"] != "latest Go release" {
		t.Errorf("query should be 'latest Go release', got %v", input["query"])
	}
	if out.Content[1]["type"] != "web_search_tool_result" {
		t.Errorf("second block should be web_search_tool_result, got %v", out.Content[1]["type"])
	}
	// Verify sources are populated in web_search_tool_result content
	resultContent, ok := out.Content[1]["content"].([]interface{})
	if !ok {
		t.Fatalf("web_search_tool_result content should be []interface{}, got %T", out.Content[1]["content"])
	}
	if len(resultContent) != 2 {
		t.Fatalf("expected 2 search results, got %d", len(resultContent))
	}
	first, _ := resultContent[0].(map[string]interface{})
	if first["type"] != "web_search_result" {
		t.Errorf("expected web_search_result type, got %v", first["type"])
	}
	if first["url"] != "https://go.dev/blog/go1.24" {
		t.Errorf("expected first url, got %v", first["url"])
	}
	if first["title"] != "Go 1.24 Release Notes" {
		t.Errorf("expected first title, got %v", first["title"])
	}
	if out.Content[2]["type"] != "text" {
		t.Errorf("third block should be text, got %v", out.Content[2]["type"])
	}
}

func TestTranslateCodexResponseWebSearchCallNoSources(t *testing.T) {
	resp := &CodexResponse{
		ID:    "resp_ws_ns",
		Model: "gpt-5.4",
		Output: []CodexOutputItem{
			{
				Type: "web_search_call",
				ID:   "ws_2",
				Action: map[string]interface{}{
					"type":  "web_search",
					"query": "test query",
				},
			},
		},
		Usage: &CodexUsage{InputTokens: 10, OutputTokens: 5},
	}
	out := TranslateCodexResponse(resp, "claude-3-5-sonnet")
	if len(out.Content) != 2 {
		t.Fatalf("expected 2 content blocks (server_tool_use + result), got %d", len(out.Content))
	}
	resultContent, ok := out.Content[1]["content"].([]interface{})
	if !ok {
		t.Fatalf("content should be []interface{}, got %T", out.Content[1]["content"])
	}
	if len(resultContent) != 0 {
		t.Errorf("expected empty content when no sources, got %d items", len(resultContent))
	}
}

func TestTranslateRequestCodexIncludeWebSearchSources(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"search for Go 1.24"`)},
		},
		Tools: []AnthropicTool{
			{Type: "web_search_20250305", Name: "web_search"},
			{Name: "read_file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	if len(codex.Include) != 1 || codex.Include[0] != "web_search_call.action.sources" {
		t.Errorf("expected include=[web_search_call.action.sources], got %v", codex.Include)
	}
}

func TestTranslateRequestCodexNoIncludeWithoutWebSearch(t *testing.T) {
	cfg := newCfg(true)
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"test"`)},
		},
		Tools: []AnthropicTool{
			{Name: "read_file", InputSchema: map[string]interface{}{"type": "object"}},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	codex := out.(*CodexRequest)
	if len(codex.Include) != 0 {
		t.Errorf("expected empty include without web_search, got %v", codex.Include)
	}
}

func TestTranslateRequestOpenAIFastMode(t *testing.T) {
	cfg := newCfg(false)
	cfg.FastMode = true
	req := &AnthropicRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}
	out, _, _ := TranslateRequest(req, cfg)
	oai, ok := out.(*OpenAIRequest)
	if !ok {
		t.Fatalf("expected *OpenAIRequest, got %T", out)
	}
	if oai.ServiceTier != "priority" {
		t.Errorf("expected service_tier=priority, got %q", oai.ServiceTier)
	}
}
