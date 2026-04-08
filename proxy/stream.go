package proxy

import (
	"bufio"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamChunk represents a single SSE chunk from OpenAI's streaming response.
type StreamChunk struct {
	ID      string              `json:"id"`
	Choices []StreamChunkChoice `json:"choices"`
	Usage   *OpenAIUsage        `json:"usage,omitempty"`
}

type StreamChunkChoice struct {
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type StreamDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []StreamToolCall `json:"tool_calls,omitempty"`
}

type StreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function StreamFunctionCall `json:"function"`
}

type StreamFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// HandleStream reads OpenAI SSE stream and writes Anthropic-format SSE events.
// estimatedInputTokens is a best-effort prompt token count derived from the
// original Anthropic request; it is emitted in the message_start event so
// that clients (e.g. Claude Code) can display per-request usage immediately
// rather than waiting for the final message_delta.
func HandleStream(w http.ResponseWriter, body io.ReadCloser, originalModel string, isChatGPT bool, estimatedInputTokens int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	messageID := fmt.Sprintf("msg_%s", randomHex(12))

	// message_start
	writeSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         originalModel,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":                estimatedInputTokens,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
				"output_tokens":               0,
			},
		},
	})

	// content_block_start for text
	writeSSE(w, flusher, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]interface{}{"type": "text", "text": ""},
	})

	// ping
	writeSSE(w, flusher, "ping", map[string]interface{}{"type": "ping"})

	var (
		textBlockClosed     bool
		currentToolIdx      *int
		lastAnthropicIdx    int
		toolBlockCount      int            // number of tool_use blocks opened (for stop reason)
		inputTokens         int
		outputTokens        int
		cachedTokens        int
		hasSentStop         bool
		reasoningBlockOpen  bool           // tracks whether a reasoning block is currently open
		reasoningBlockIdx   int            // Anthropic block index for the current reasoning block
		toolIndexMap        map[int]int    // OpenAI tool index → Anthropic block index
		codexItemToBlockIdx map[string]int // Codex item_id → Anthropic block index (parallel tool calls)
		lastCodexBlockIdx   int            // most recently opened Codex tool block (fallback routing)
		stoppedBlocks       map[int]bool   // blocks already stopped (avoid double-stop)
	)

	scanner := bufio.NewScanner(body)
	// Reasoning models can emit very large argument deltas / annotations
	// in a single SSE line. Allow up to 4 MiB per line.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		if isChatGPT {
			if debug {
				fmt.Println("CODEX STRM:", data)
			}
			// We translate Codex SSE chunk to Anthropic SSE format
			var cChunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &cChunk); err != nil {
				continue
			}

			cType, _ := cChunk["type"].(string)
			switch cType {
			case "response.output_text.delta":
				deltaStr, _ := cChunk["delta"].(string)
				if !textBlockClosed {
					writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": 0,
						"delta": map[string]interface{}{"type": "text_delta", "text": deltaStr},
					})
				}
			case "response.output_item.added":
				item, ok := cChunk["item"].(map[string]interface{})
				if !ok {
					continue
				}
				if item["type"] == "function_call" {
					if !textBlockClosed {
						textBlockClosed = true
						writeSSE(w, flusher, "content_block_stop", map[string]interface{}{
							"type": "content_block_stop", "index": 0,
						})
					}
					callId, _ := item["call_id"].(string)
					name, _ := item["name"].(string)
					// The Responses API puts the per-item identifier on
					// `id`; `call_id` is the separate tool-use id Anthropic
					// clients echo back on tool_result.
					itemID, _ := item["id"].(string)
					lastAnthropicIdx++
					toolBlockCount++
					if codexItemToBlockIdx == nil {
						codexItemToBlockIdx = map[string]int{}
					}
					if itemID != "" {
						codexItemToBlockIdx[itemID] = lastAnthropicIdx
					}
					lastCodexBlockIdx = lastAnthropicIdx
					writeSSE(w, flusher, "content_block_start", map[string]interface{}{
						"type":  "content_block_start",
						"index": lastAnthropicIdx,
						"content_block": map[string]interface{}{
							"type":  "tool_use",
							"id":    callId,
							"name":  name,
							"input": map[string]interface{}{},
						},
					})
				}
			case "response.function_call_arguments.delta":
				deltaStr, _ := cChunk["delta"].(string)
				// Route by item_id so parallel tool calls don't bleed
				// into each other. Fall back to the most recently opened
				// tool block for malformed streams so we still produce
				// *something* rather than silently drop the delta.
				itemID, _ := cChunk["item_id"].(string)
				blockIdx := -1
				if itemID != "" {
					if idx, ok := codexItemToBlockIdx[itemID]; ok {
						blockIdx = idx
					}
				}
				if blockIdx < 0 && lastCodexBlockIdx > 0 {
					blockIdx = lastCodexBlockIdx
				}
				if blockIdx > 0 {
					writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": blockIdx,
						"delta": map[string]interface{}{
							"type":         "input_json_delta",
							"partial_json": deltaStr,
						},
					})
				}
			case "response.error", "response.failed":
				// Upstream model error — surface to user and finalize
				errMsg := "unknown error"
				if errObj, ok := cChunk["error"].(map[string]interface{}); ok {
					if m, ok := errObj["message"].(string); ok && m != "" {
						errMsg = m
					}
				} else if respObj, ok := cChunk["response"].(map[string]interface{}); ok {
					if errObj, ok := respObj["error"].(map[string]interface{}); ok {
						if m, ok := errObj["message"].(string); ok && m != "" {
							errMsg = m
						}
					}
				}
				if !textBlockClosed {
					writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": 0,
						"delta": map[string]interface{}{"type": "text_delta", "text": "\n\n[Error: " + errMsg + "]"},
					})
				}
				if !hasSentStop {
					hasSentStop = true
					finalizeStream(w, flusher, textBlockClosed, lastAnthropicIdx, "end_turn", inputTokens, outputTokens, cachedTokens, stoppedBlocks)
				}
				return

			case "response.function_call_arguments.done":
				if debug {
					fmt.Println("CODEX STRM: function_call_arguments.done (complete)")
				}

			case "response.reasoning_summary_text.delta":
				deltaStr, _ := cChunk["delta"].(string)
				if deltaStr != "" {
					if !reasoningBlockOpen {
						// Open a new thinking block on the first delta only.
						// Track its index separately so that tool blocks
						// opened between reasoning deltas don't cause the
						// done event to stop the wrong block.
						reasoningBlockOpen = true
						lastAnthropicIdx++
						reasoningBlockIdx = lastAnthropicIdx
						writeSSE(w, flusher, "content_block_start", map[string]interface{}{
							"type":  "content_block_start",
							"index": reasoningBlockIdx,
							"content_block": map[string]interface{}{
								"type":     "thinking",
								"thinking": "",
							},
						})
					}
					writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": reasoningBlockIdx,
						"delta": map[string]interface{}{"type": "thinking_delta", "thinking": deltaStr},
					})
				}

			case "response.reasoning_summary_text.done":
				if reasoningBlockOpen {
					reasoningBlockOpen = false
					if stoppedBlocks == nil {
						stoppedBlocks = map[int]bool{}
					}
					stoppedBlocks[reasoningBlockIdx] = true
					writeSSE(w, flusher, "content_block_stop", map[string]interface{}{
						"type": "content_block_stop", "index": reasoningBlockIdx,
					})
				}

			case "response.completed", "response.incomplete":
				if !hasSentStop {
					hasSentStop = true

					respData, _ := cChunk["response"].(map[string]interface{})
					if respData != nil {
						if usage, ok := respData["usage"].(map[string]interface{}); ok {
							if v, ok2 := usage["output_tokens"].(float64); ok2 {
								outputTokens = int(v)
							}
							if v, ok2 := usage["input_tokens"].(float64); ok2 {
								inputTokens = int(v)
							}
							if details, ok2 := usage["input_tokens_details"].(map[string]interface{}); ok2 {
								if v, ok3 := details["cached_tokens"].(float64); ok3 {
									cachedTokens = int(v)
								}
							}
						}
					}

					reason := "end_turn"
					if cType == "response.incomplete" {
						// Mirror TranslateCodexResponse: parse the actual
						// incomplete reason instead of hard-coding max_tokens.
						incReason := ""
						if respData != nil {
							if det, ok := respData["incomplete_details"].(map[string]interface{}); ok {
								incReason, _ = det["reason"].(string)
							}
						}
						switch incReason {
						case "max_output_tokens", "max_tokens":
							reason = "max_tokens"
						default:
							reason = "end_turn"
						}
					} else if toolBlockCount > 0 {
						reason = "tool_use"
					}

					finalizeStream(w, flusher, textBlockClosed, lastAnthropicIdx, reason, inputTokens, outputTokens, cachedTokens, stoppedBlocks)
					return
				}
			}
			continue
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Track usage
		if chunk.Usage != nil {
			outputTokens = chunk.Usage.CompletionTokens
			inputTokens = chunk.Usage.PromptTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Text content
		if delta.Content != nil && *delta.Content != "" {
			if currentToolIdx == nil && !textBlockClosed {
				writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]interface{}{"type": "text_delta", "text": *delta.Content},
				})
			}
		}

		// Tool calls
		if len(delta.ToolCalls) > 0 {
			// Close text block on first tool call
			if currentToolIdx == nil && !textBlockClosed {
				textBlockClosed = true
				writeSSE(w, flusher, "content_block_stop", map[string]interface{}{
					"type": "content_block_stop", "index": 0,
				})
			}
			if toolIndexMap == nil {
				toolIndexMap = map[int]int{}
			}

			for _, tc := range delta.ToolCalls {
				anthIdx, known := toolIndexMap[tc.Index]
				if !known {
					// New tool call — assign an Anthropic block index
					lastAnthropicIdx++
					toolBlockCount++
					anthIdx = lastAnthropicIdx
					toolIndexMap[tc.Index] = anthIdx
					currentToolIdx = &tc.Index

					toolID := tc.ID
					if toolID == "" {
						toolID = fmt.Sprintf("toolu_%s", randomHex(12))
					}

					writeSSE(w, flusher, "content_block_start", map[string]interface{}{
						"type":  "content_block_start",
						"index": anthIdx,
						"content_block": map[string]interface{}{
							"type":  "tool_use",
							"id":    toolID,
							"name":  tc.Function.Name,
							"input": map[string]interface{}{},
						},
					})
				}

				// Send argument fragments to the correct block
				if tc.Function.Arguments != "" {
					writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": anthIdx,
						"delta": map[string]interface{}{
							"type":         "input_json_delta",
							"partial_json": tc.Function.Arguments,
						},
					})
				}
			}
		}

		// Finish
		if choice.FinishReason != nil && !hasSentStop {
			hasSentStop = true
			stopReason := mapFinishReason(*choice.FinishReason)
			finalizeStream(w, flusher, textBlockClosed, lastAnthropicIdx, stopReason, inputTokens, outputTokens, cachedTokens, stoppedBlocks)
			return
		}
	}

	// If the scanner encountered an error (e.g. connection reset mid-stream),
	// surface it so the user sees it instead of silent truncation.
	if err := scanner.Err(); err != nil && !hasSentStop {
		if !textBlockClosed {
			writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{"type": "text_delta", "text": "\n\n[Error: stream interrupted: " + err.Error() + "]"},
			})
		}
	}

	// Fallback close if no finish_reason received
	if !hasSentStop {
		finalizeStream(w, flusher, textBlockClosed, lastAnthropicIdx, "end_turn", inputTokens, outputTokens, cachedTokens, stoppedBlocks)
	}
}

// finalizeStream closes all open content blocks and sends the final
// message events. Token counts are forwarded so Anthropic clients can
// display usage information; cachedTokens is reported separately as
// `cache_read_input_tokens` to match the Anthropic schema.
func finalizeStream(w http.ResponseWriter, f http.Flusher, textBlockClosed bool, lastAnthropicIdx int, stopReason string, inputTokens, outputTokens, cachedTokens int, stoppedBlocks map[int]bool) {
	if !textBlockClosed {
		writeSSE(w, f, "content_block_stop", map[string]interface{}{
			"type": "content_block_stop", "index": 0,
		})
	}
	for i := 1; i <= lastAnthropicIdx; i++ {
		if stoppedBlocks[i] {
			continue
		}
		writeSSE(w, f, "content_block_stop", map[string]interface{}{
			"type": "content_block_stop", "index": i,
		})
	}
	usage := map[string]interface{}{
		"output_tokens": outputTokens,
		"input_tokens":  normalizeCachedInputTokens(inputTokens, cachedTokens),
	}
	if cachedTokens > 0 {
		usage["cache_read_input_tokens"] = cachedTokens
	}
	writeSSE(w, f, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usage,
	})
	writeSSE(w, f, "message_stop", map[string]interface{}{"type": "message_stop"})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	f.Flush()
}

func writeSSE(w http.ResponseWriter, f http.Flusher, event string, data interface{}) {
	b, err := json.Marshal(data)
	if err != nil {
		return // skip this event; don't send malformed data
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
	f.Flush()
}

func randomHex(n int) string {
	b := make([]byte, (n+1)/2)
	crand.Read(b)
	return hex.EncodeToString(b)[:n]
}
