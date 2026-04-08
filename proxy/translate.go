package proxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/goclawgate/clawgate/config"
)

// ── Anthropic request types ──────────────────────────────────────────

type AnthropicRequest struct {
	Model         string                 `json:"model"`
	MaxTokens     int                    `json:"max_tokens"`
	Messages      []AnthropicMessage     `json:"messages"`
	System        json.RawMessage        `json:"system,omitempty"`
	StopSequences []string               `json:"stop_sequences,omitempty"`
	Stream        bool                   `json:"stream,omitempty"`
	Temperature   *float64               `json:"temperature,omitempty"`
	TopP          *float64               `json:"top_p,omitempty"`
	Tools         []AnthropicTool        `json:"tools,omitempty"`
	ToolChoice    map[string]interface{} `json:"tool_choice,omitempty"`
	Thinking      map[string]interface{} `json:"thinking,omitempty"`
}

type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type ContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   json.RawMessage        `json:"content,omitempty"`
	Source    map[string]interface{} `json:"source,omitempty"`
}

// ── OpenAI Chat Completions request types ────────────────────────────

type OpenAIRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Stop                []string        `json:"stop,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	Tools               []OpenAITool    `json:"tools,omitempty"`
	ToolChoice          interface{}     `json:"tool_choice,omitempty"`
	ServiceTier         string          `json:"service_tier,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ── Codex Responses API request types ────────────────────────────────

type CodexRequest struct {
	Model            string          `json:"model"`
	Instructions     string          `json:"instructions,omitempty"`
	Store            *bool           `json:"store,omitempty"`
	Input            []interface{}   `json:"input"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Tools            []CodexTool     `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	Reasoning        *CodexReasoning `json:"reasoning,omitempty"`
	ParallelToolCall *bool           `json:"parallel_tool_calls,omitempty"`
	ServiceTier      string          `json:"service_tier,omitempty"`
}

type CodexReasoning struct {
	Effort  string `json:"effort,omitempty"`  // "none" | "minimal" | "low" | "medium" | "high" | "xhigh"
	Summary string `json:"summary,omitempty"` // "auto" | "concise" | "detailed"
}

type CodexTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type CodexReqMessage struct {
	Role    string            `json:"role"`
	Content []CodexReqContent `json:"content"`
}

type CodexReqContent struct {
	Type     string `json:"type"` // "input_text" | "output_text" | "input_image"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// Codex Responses API structured tool call types (top-level input items)
type CodexFunctionCall struct {
	Type      string `json:"type"` // "function_call"
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type CodexFunctionCallOutput struct {
	Type   string `json:"type"` // "function_call_output"
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// ── Codex Responses API response types (non-streaming) ───────────────

type CodexResponse struct {
	ID                string              `json:"id"`
	Model             string              `json:"model"`
	Output            []CodexOutputItem   `json:"output"`
	Usage             *CodexUsage         `json:"usage,omitempty"`
	IncompleteDetails *CodexIncompleteDet `json:"incomplete_details,omitempty"`
	Error             *CodexError         `json:"error,omitempty"`
}

type CodexOutputItem struct {
	Type string `json:"type"`
	// Common
	ID string `json:"id,omitempty"`
	// message
	Role    string               `json:"role,omitempty"`
	Content []CodexOutputContent `json:"content,omitempty"`
	// function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// reasoning
	Summary          []CodexReasoningSummary `json:"summary,omitempty"`
	EncryptedContent string                  `json:"encrypted_content,omitempty"`
}

type CodexOutputContent struct {
	Type string `json:"type"` // "output_text"
	Text string `json:"text,omitempty"`
}

type CodexReasoningSummary struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type CodexUsage struct {
	InputTokens         int                      `json:"input_tokens"`
	OutputTokens        int                      `json:"output_tokens"`
	TotalTokens         int                      `json:"total_tokens,omitempty"`
	InputTokensDetails  *CodexInputTokenDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *CodexOutputTokenDetails `json:"output_tokens_details,omitempty"`
}

type CodexInputTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type CodexOutputTokenDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type CodexIncompleteDet struct {
	Reason string `json:"reason"`
}

type CodexError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ── OpenAI response types ────────────────────────────────────────────

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"`
}

type OpenAIChoice struct {
	Message      OpenAIRespMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type OpenAIRespMessage struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ── Anthropic response types ─────────────────────────────────────────

type AnthropicResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Model        string                   `json:"model"`
	Content      []map[string]interface{} `json:"content"`
	StopReason   *string                  `json:"stop_reason"`
	StopSequence *string                  `json:"stop_sequence"`
	Usage        AnthropicUsage           `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// maxCompletionTokens is the ceiling applied to the Anthropic max_tokens
// field before forwarding upstream. OpenAI models generally cap at 16384
// output tokens; higher values are silently clamped by upstream anyway.
const maxCompletionTokens = 16384

// Budget-to-effort thresholds for mapping Anthropic thinking.budget_tokens
// to Codex reasoning effort levels.
const (
	budgetXHigh   = 32000
	budgetHigh    = 10000
	budgetMedium  = 4000
	budgetLow     = 1000
)

// ── Translation functions ────────────────────────────────────────────

// MapModel maps Anthropic model names to OpenAI model names.
func MapModel(model string, cfg *config.Config) string {
	clean := model
	for _, prefix := range []string{"anthropic/", "openai/", "gemini/"} {
		clean = strings.TrimPrefix(clean, prefix)
	}
	lower := strings.ToLower(clean)
	if strings.Contains(lower, "haiku") {
		return cfg.SmallModel
	}
	if strings.Contains(lower, "sonnet") {
		return cfg.MidModel
	}
	if strings.Contains(lower, "opus") {
		return cfg.BigModel
	}
	return cfg.BigModel // default to big model
}

// isReasoningModel returns true for OpenAI reasoning models that
// reject temperature/top_p and accept the `reasoning` field. This
// covers o1/o3/o4 family and the GPT-5 reasoning lines (codex/codex-max
// variants are reasoning-capable).
func isReasoningModel(model string) bool {
	m := strings.ToLower(model)
	switch {
	case strings.HasPrefix(m, "o1"),
		strings.HasPrefix(m, "o3"),
		strings.HasPrefix(m, "o4"),
		strings.HasPrefix(m, "gpt-5"):
		return true
	}
	return false
}

// effortRank returns a numeric rank for a reasoning effort string.
// Higher rank = more reasoning. Unknown/empty returns -1.
func effortRank(effort string) int {
	switch effort {
	case "none":
		return 0
	case "minimal":
		return 1
	case "low":
		return 2
	case "medium":
		return 3
	case "high":
		return 4
	case "xhigh":
		return 5
	default:
		return -1
	}
}

// resolveReasoningEffort returns the effort level to send, or "" to
// leave the field unset. When both per-request thinking and the global
// --reason flag are set, the higher of the two wins.
func resolveReasoningEffort(cfg *config.Config, req *AnthropicRequest) string {
	thinkingEffort := reasoningEffortFromThinking(req.Thinking)
	flagEffort := cfg.ReasoningEffort

	if thinkingEffort == "" {
		return flagEffort
	}
	if flagEffort == "" {
		return thinkingEffort
	}
	// Both set — return whichever ranks higher.
	if effortRank(flagEffort) > effortRank(thinkingEffort) {
		return flagEffort
	}
	return thinkingEffort
}

// reasoningEffortFromThinking maps Anthropic's `thinking` field
// (extended thinking) to a Codex reasoning effort level. budget_tokens
// is used as the heuristic when present.
func reasoningEffortFromThinking(thinking map[string]interface{}) string {
	if thinking == nil {
		return ""
	}
	if t, _ := thinking["type"].(string); t != "" && t != "enabled" {
		// "disabled" or unknown — no reasoning override
		return ""
	}
	if budget, ok := thinking["budget_tokens"].(float64); ok {
		switch {
		case budget >= budgetXHigh:
			return "xhigh"
		case budget >= budgetHigh:
			return "high"
		case budget >= budgetMedium:
			return "medium"
		case budget >= budgetLow:
			return "low"
		case budget > 0:
			return "minimal"
		default:
			return "none"
		}
	}
	// thinking enabled with no budget — default medium
	return "medium"
}

// TranslateRequest converts an Anthropic request to OpenAI or Codex format.
func TranslateRequest(req *AnthropicRequest, cfg *config.Config) (interface{}, string, string) {
	originalModel := req.Model
	mappedModel := MapModel(req.Model, cfg)

	var sysText string
	if len(req.System) > 0 {
		sysText = extractSystemText(req.System)
	}

	maxTokens := req.MaxTokens
	if maxTokens > maxCompletionTokens {
		maxTokens = maxCompletionTokens
	}

	// ── ChatGPT Codex Format ─────────────────────────────────────
	if cfg.IsChatGPT() {
		// NOTE: maxTokens is intentionally NOT forwarded to the Codex
		// request. The ChatGPT backend-api/codex/responses endpoint
		// rejects max_output_tokens — matching the official Codex CLI
		// and OpenCode's codex plugin which explicitly strips the field
		// with the comment "Match codex cli".
		f := false
		codexReq := &CodexRequest{
			Model:        mappedModel,
			Instructions: sysText, // may be empty; Codex accepts empty
			Store:        &f,
			Stream:       req.Stream,
		}

		// Reasoning models (gpt-5*, o1, o3, o4...) reject temperature
		// and top_p but accept a `reasoning` field.
		reasoning := isReasoningModel(mappedModel)
		if !reasoning {
			codexReq.Temperature = req.Temperature
			codexReq.TopP = req.TopP
		} else if effort := resolveReasoningEffort(cfg, req); effort != "" {
			codexReq.Reasoning = &CodexReasoning{Effort: effort, Summary: "auto"}
		}

		// Input messages — properly handle tool_use and tool_result as
		// top-level function_call / function_call_output items, and
		// combine multi-part user message content into a single message.
		for _, msg := range req.Messages {
			blocks := parseContentBlocks(msg.Content)

			if blocks == nil {
				// Simple string content
				s := extractStringContent(msg.Content)
				if s == "" {
					s = "..."
				}
				contentType := "input_text"
				if msg.Role == "assistant" {
					contentType = "output_text"
				}
				codexReq.Input = append(codexReq.Input, CodexReqMessage{
					Role:    msg.Role,
					Content: []CodexReqContent{{Type: contentType, Text: s}},
				})
				continue
			}

			if msg.Role == "user" {
				// Collect all user content parts (text + image) into a
				// single input message; tool_results emit separate
				// top-level function_call_output items.
				var parts []CodexReqContent
				for _, b := range blocks {
					switch b.Type {
					case "text":
						if b.Text != "" {
							parts = append(parts, CodexReqContent{
								Type: "input_text",
								Text: b.Text,
							})
						}
					case "image":
						if url := imageSourceToDataURL(b.Source); url != "" {
							parts = append(parts, CodexReqContent{
								Type:     "input_image",
								ImageURL: url,
							})
						}
					case "tool_result":
						codexReq.Input = append(codexReq.Input, CodexFunctionCallOutput{
							Type:   "function_call_output",
							CallID: b.ToolUseID,
							Output: extractToolResultContent(b.Content),
						})
					}
				}
				if len(parts) > 0 {
					codexReq.Input = append(codexReq.Input, CodexReqMessage{
						Role:    "user",
						Content: parts,
					})
				}
				continue
			}

			// assistant message: each text part becomes its own message
			// (matching OpenCode reference behavior); tool_use parts
			// become top-level function_call items.
			for _, b := range blocks {
				switch b.Type {
				case "text":
					if b.Text == "" {
						continue
					}
					codexReq.Input = append(codexReq.Input, CodexReqMessage{
						Role: "assistant",
						Content: []CodexReqContent{{
							Type: "output_text",
							Text: b.Text,
						}},
					})
				case "tool_use":
					argsJSON, _ := json.Marshal(b.Input)
					if len(argsJSON) == 0 || string(argsJSON) == "null" {
						argsJSON = []byte("{}")
					}
					codexReq.Input = append(codexReq.Input, CodexFunctionCall{
						Type:      "function_call",
						CallID:    b.ID,
						Name:      b.Name,
						Arguments: string(argsJSON),
					})
				}
			}
		}

		// Tools
		codexReq.Tools = translateCodexTools(req.Tools)

		if len(codexReq.Tools) > 0 {
			t := true
			codexReq.ParallelToolCall = &t
		}

		// Tool choice
		if choice := translateToolChoice(req.ToolChoice, true); choice != nil {
			codexReq.ToolChoice = choice
		}

		if cfg.FastMode {
			codexReq.ServiceTier = "priority"
		}

		return codexReq, originalModel, mappedModel
	}

	// ── Standard OpenAI Chat Completions Format ─────────────────
	var messages []OpenAIMessage
	if sysText != "" {
		messages = append(messages, OpenAIMessage{Role: "system", Content: sysText})
	}

	for _, msg := range req.Messages {
		blocks := parseContentBlocks(msg.Content)

		if blocks == nil {
			// Simple string content
			s := extractStringContent(msg.Content)
			if s == "" {
				s = "..."
			}
			messages = append(messages, OpenAIMessage{Role: msg.Role, Content: s})
			continue
		}

		if msg.Role == "assistant" {
			// Collect text and tool_use blocks
			var textParts []string
			var toolCalls []OpenAIToolCall

			for _, b := range blocks {
				switch b.Type {
				case "text":
					textParts = append(textParts, b.Text)
				case "tool_use":
					argsJSON, _ := json.Marshal(b.Input)
					toolCalls = append(toolCalls, OpenAIToolCall{
						ID:   b.ID,
						Type: "function",
						Function: OpenAIFunctionCall{
							Name:      b.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}

			text := strings.Join(textParts, "\n")
			if len(toolCalls) > 0 {
				messages = append(messages, OpenAIMessage{
					Role:      "assistant",
					Content:   text,
					ToolCalls: toolCalls,
				})
			} else {
				if text == "" {
					text = "..."
				}
				messages = append(messages, OpenAIMessage{Role: "assistant", Content: text})
			}
		} else {
			// User message — emit tool_result as "tool" role messages,
			// then any remaining text/image as a "user" message.
			var textParts []string
			var contentParts []map[string]interface{}
			hasImages := false

			for _, b := range blocks {
				switch b.Type {
				case "tool_result":
					messages = append(messages, OpenAIMessage{
						Role:       "tool",
						ToolCallID: b.ToolUseID,
						Content:    extractToolResultContent(b.Content),
					})
				case "text":
					textParts = append(textParts, b.Text)
					contentParts = append(contentParts, map[string]interface{}{
						"type": "text",
						"text": b.Text,
					})
				case "image":
					hasImages = true
					if url := imageSourceToDataURL(b.Source); url != "" {
						contentParts = append(contentParts, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": url,
							},
						})
					}
				}
			}

			if hasImages && len(contentParts) > 0 {
				// Use content parts array for vision support
				messages = append(messages, OpenAIMessage{Role: "user", Content: contentParts})
			} else {
				text := strings.Join(textParts, "\n")
				if text != "" {
					messages = append(messages, OpenAIMessage{Role: "user", Content: text})
				}
			}
		}
	}

	oaiReq := &OpenAIRequest{
		Model:               mappedModel,
		Messages:            messages,
		MaxCompletionTokens: maxTokens,
		Stream:              req.Stream,
	}
	// Reasoning models reject temperature/top_p — only set them for
	// non-reasoning models. Reasoning models, conversely, accept the
	// `reasoning_effort` field, so forward whatever the user picked
	// (higher of thinking / --reason flag via resolveReasoningEffort).
	if !isReasoningModel(mappedModel) {
		oaiReq.Temperature = req.Temperature
		oaiReq.TopP = req.TopP
	} else if effort := resolveReasoningEffort(cfg, req); effort != "" {
		oaiReq.ReasoningEffort = effort
	}
	if cfg.FastMode {
		oaiReq.ServiceTier = "priority"
	}

	if req.Stream {
		oaiReq.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	if len(req.StopSequences) > 0 {
		oaiReq.Stop = req.StopSequences
	}

	oaiReq.Tools = translateOpenAITools(req.Tools)

	if choice := translateToolChoice(req.ToolChoice, false); choice != nil {
		oaiReq.ToolChoice = choice
	}

	return oaiReq, originalModel, mappedModel
}

// TranslateResponse converts an OpenAI response to Anthropic format.
func TranslateResponse(oaiResp *OpenAIResponse, originalModel string) *AnthropicResponse {
	var content []map[string]interface{}

	if len(oaiResp.Choices) > 0 {
		choice := oaiResp.Choices[0]

		// Text content
		if choice.Message.Content != nil && *choice.Message.Content != "" {
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": *choice.Message.Content,
			})
		}

		// Tool calls
		for _, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]interface{}{"raw": tc.Function.Arguments}
			}
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": args,
			})
		}
	}

	if len(content) == 0 {
		content = append(content, map[string]interface{}{"type": "text", "text": ""})
	}

	// Map finish reason
	var stopReason *string
	if len(oaiResp.Choices) > 0 {
		sr := mapFinishReason(oaiResp.Choices[0].FinishReason)
		stopReason = &sr
	}

	usage := AnthropicUsage{}
	if oaiResp.Usage != nil {
		usage.InputTokens = oaiResp.Usage.PromptTokens
		usage.OutputTokens = oaiResp.Usage.CompletionTokens
	}

	return &AnthropicResponse{
		ID:         oaiResp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      originalModel,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// TranslateCodexResponse converts a non-streaming Codex Responses API
// response (which uses an `output[]` array, NOT `choices[]`) to
// Anthropic format.
func TranslateCodexResponse(resp *CodexResponse, originalModel string) *AnthropicResponse {
	var content []map[string]interface{}
	hasFunctionCall := false
	stoppedIncomplete := false
	incompleteReason := ""

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" && c.Text != "" {
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": c.Text,
					})
				}
			}
		case "function_call":
			hasFunctionCall = true
			var args map[string]interface{}
			if item.Arguments != "" {
				if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
					args = map[string]interface{}{"raw": item.Arguments}
				}
			}
			if args == nil {
				args = map[string]interface{}{}
			}
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"id":    item.CallID,
				"name":  item.Name,
				"input": args,
			})
		case "reasoning":
			// Optional: surface reasoning summary as a thinking block.
			// Anthropic clients ignore unknown types so this is safe.
			for _, s := range item.Summary {
				if s.Text != "" {
					content = append(content, map[string]interface{}{
						"type":     "thinking",
						"thinking": s.Text,
					})
				}
			}
		}
	}

	if resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason != "" {
		stoppedIncomplete = true
		incompleteReason = resp.IncompleteDetails.Reason
	}

	if len(content) == 0 {
		content = append(content, map[string]interface{}{"type": "text", "text": ""})
	}

	stopReasonStr := "end_turn"
	if stoppedIncomplete {
		switch incompleteReason {
		case "max_output_tokens", "max_tokens":
			stopReasonStr = "max_tokens"
		default:
			stopReasonStr = "end_turn"
		}
	} else if hasFunctionCall {
		stopReasonStr = "tool_use"
	}

	usage := AnthropicUsage{}
	if resp.Usage != nil {
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens
		if resp.Usage.InputTokensDetails != nil {
			usage.CacheReadInputTokens = resp.Usage.InputTokensDetails.CachedTokens
			usage.InputTokens = normalizeCachedInputTokens(usage.InputTokens, usage.CacheReadInputTokens)
		}
	}

	return &AnthropicResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      originalModel,
		Content:    content,
		StopReason: &stopReasonStr,
		Usage:      usage,
	}
}

// ── Shared tool / tool-choice translation helpers ────────────────────

// translateCodexTools converts Anthropic tool definitions into Codex
// Responses API tool definitions. Returns nil when the input is empty.
func translateCodexTools(tools []AnthropicTool) []CodexTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]CodexTool, 0, len(tools))
	for i, t := range tools {
		name := t.Name
		if name == "" {
			name = fmt.Sprintf("unnamed_tool_%d", i)
		}
		out = append(out, CodexTool{
			Type:        "function",
			Name:        name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	return out
}

// translateOpenAITools converts Anthropic tool definitions into OpenAI
// Chat Completions tool definitions. Returns nil when the input is empty.
func translateOpenAITools(tools []AnthropicTool) []OpenAITool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]OpenAITool, 0, len(tools))
	for i, t := range tools {
		name := t.Name
		if name == "" {
			name = fmt.Sprintf("unnamed_tool_%d", i)
		}
		out = append(out, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return out
}

// translateToolChoice maps an Anthropic tool_choice object to the
// upstream representation. The shape of the "tool" case differs between
// the Codex Responses API (flat name at top level) and Chat Completions
// (nested under "function"), so isChatGPT selects the output form.
// Returns nil when tc is nil (caller should leave field unset).
func translateToolChoice(tc map[string]interface{}, isChatGPT bool) interface{} {
	if tc == nil {
		return nil
	}
	switch tc["type"] {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		name, ok := tc["name"].(string)
		if !ok || name == "" {
			return "auto"
		}
		if isChatGPT {
			return map[string]interface{}{
				"type": "function",
				"name": name,
			}
		}
		return map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": name},
		}
	default:
		return "auto"
	}
}

// normalizeCachedInputTokens subtracts cachedTokens from inputTokens to
// give Anthropic clients a "fresh" input count (Codex returns the
// inclusive total). Used by both TranslateCodexResponse and finalizeStream.
func normalizeCachedInputTokens(inputTokens, cachedTokens int) int {
	if cachedTokens > 0 && inputTokens >= cachedTokens {
		return inputTokens - cachedTokens
	}
	return inputTokens
}

// ── Helpers ──────────────────────────────────────────────────────────

// imageSourceToDataURL converts an Anthropic image content block source
// (`{type: base64, media_type, data}` or `{type: url, url}`) into a URL
// usable for the Codex `input_image` content type.
func imageSourceToDataURL(src map[string]interface{}) string {
	if src == nil {
		return ""
	}
	if t, _ := src["type"].(string); t == "url" {
		if u, _ := src["url"].(string); u != "" {
			return u
		}
		return ""
	}
	mediaType, _ := src["media_type"].(string)
	data, _ := src["data"].(string)
	if mediaType == "" || data == "" {
		return ""
	}
	return fmt.Sprintf("data:%s;base64,%s", mediaType, data)
}

func extractSystemText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// extractStringContent tries to unmarshal raw as a plain string.
func extractStringContent(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// parseContentBlocks tries to parse raw as an array of content blocks.
// Returns nil if the content is a plain string (not an array).
func parseContentBlocks(raw json.RawMessage) []ContentBlock {
	// If it's a string, return nil to signal "plain string"
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

func extractToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 1. Plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// 2. Array of content blocks (the canonical Anthropic shape)
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil && len(blocks) > 0 {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
				continue
			}
			if b.Type == "image" {
				parts = append(parts, "[image]")
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	// 3. Single content block object
	var block ContentBlock
	if err := json.Unmarshal(raw, &block); err == nil && block.Text != "" {
		return block.Text
	}
	// 4. Arbitrary JSON object — re-serialize compactly
	var anyVal interface{}
	if err := json.Unmarshal(raw, &anyVal); err == nil {
		if b, err := json.Marshal(anyVal); err == nil {
			return string(b)
		}
	}
	return string(raw)
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}
