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
		SmallModel: "gpt-5.4-mini",
	}
}

func TestMapModelDispatch(t *testing.T) {
	cfg := newCfg(true)
	cases := map[string]string{
		"claude-3-5-sonnet-latest": "gpt-5.4",
		"claude-haiku-4-5":         "gpt-5.4-mini",
		"claude-opus-4-6":          "gpt-5.4",
		"anthropic/claude-3-haiku": "gpt-5.4-mini",
		"some-unknown-model":       "gpt-5.4",
	}
	for in, want := range cases {
		if got := MapModel(in, cfg); got != want {
			t.Errorf("MapModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsReasoningModel(t *testing.T) {
	for _, m := range []string{"o1-preview", "o3-mini", "o4", "gpt-5.4", "gpt-5.4-mini", "gpt-5.1-codex"} {
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
	cases := []struct {
		in   map[string]interface{}
		want string
	}{
		{nil, ""},
		{map[string]interface{}{"type": "disabled"}, ""},
		{map[string]interface{}{"type": "enabled"}, "medium"},
		{map[string]interface{}{"type": "enabled", "budget_tokens": float64(1000)}, "low"},
		{map[string]interface{}{"type": "enabled", "budget_tokens": float64(8000)}, "medium"},
		{map[string]interface{}{"type": "enabled", "budget_tokens": float64(32000)}, "high"},
	}
	for _, c := range cases {
		if got := reasoningEffortFromThinking(c.in); got != c.want {
			t.Errorf("reasoningEffortFromThinking(%v) = %q, want %q", c.in, got, c.want)
		}
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
