package proxy

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStreamBody wraps a strings.Reader to satisfy io.ReadCloser.
type fakeStreamBody struct{ *strings.Reader }

func (fakeStreamBody) Close() error { return nil }

func newFakeBody(s string) io.ReadCloser {
	return fakeStreamBody{strings.NewReader(s)}
}

// TestHandleStreamCodexParallelToolCalls ensures that when the Codex
// Responses API emits two interleaved function_call items, each
// argument delta is routed to the correct Anthropic content block by
// item_id. Before the fix the second item_added overwrote a single
// currentToolIdx pointer, so arguments for tool #1 were appended to
// tool #2's content_block_delta — corrupting both JSON blobs.
func TestHandleStreamCodexParallelToolCalls(t *testing.T) {
	// Two function_call items added back-to-back, then *interleaved*
	// argument deltas referencing each item by item_id.
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_A","call_id":"call_A","name":"read_file"}}`,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_B","call_id":"call_B","name":"read_file"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_A","delta":"{\"path\":\"a.txt\"}"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_B","delta":"{\"path\":\"b.txt\"}"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_A","delta":""}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)

	out := w.Body.String()

	// Two tool blocks opened — indexes 1 and 2 (text block is 0).
	if !strings.Contains(out, `"index":1`) || !strings.Contains(out, `"call_A"`) {
		t.Errorf("expected tool_use block for call_A at index 1:\n%s", out)
	}
	if !strings.Contains(out, `"index":2`) || !strings.Contains(out, `"call_B"`) {
		t.Errorf("expected tool_use block for call_B at index 2:\n%s", out)
	}

	// The key assertion: the content_block_delta carrying call_A's
	// arguments (a.txt) must target index 1, and call_B's (b.txt) must
	// target index 2. Before the fix both deltas bled into the last-
	// opened block (index 2). JSON field order is alphabetical from
	// encoding/json, so the emitted shape is
	//   {"delta":{"partial_json":"...","type":"input_json_delta"},"index":N,...}
	aDelta := `{"delta":{"partial_json":"{\"path\":\"a.txt\"}","type":"input_json_delta"},"index":1,"type":"content_block_delta"}`
	if !strings.Contains(out, aDelta) {
		t.Errorf("call_A argument delta must target block index 1, stream was:\n%s", out)
	}
	bDelta := `{"delta":{"partial_json":"{\"path\":\"b.txt\"}","type":"input_json_delta"},"index":2,"type":"content_block_delta"}`
	if !strings.Contains(out, bDelta) {
		t.Errorf("call_B argument delta must target block index 2, stream was:\n%s", out)
	}
}

// TestHandleStreamCodexReasoningStopReason ensures that a Codex
// response containing reasoning summary deltas (but no tool calls) is
// finalized with stop_reason: "end_turn", NOT "tool_use". Before the
// fix, reasoning blocks incremented lastAnthropicIdx which was used to
// decide the stop reason, so a reasoning-only response was incorrectly
// tagged as "tool_use" — leading Anthropic clients to expect a
// tool_result and break the conversation.
func TestHandleStreamCodexReasoningStopReason(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.reasoning_summary_text.delta","delta":"Thinking about the problem..."}`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":" more thoughts."}`,
		`data: {"type":"response.reasoning_summary_text.done"}`,
		`data: {"type":"response.output_text.delta","delta":"Here is the answer."}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
	out := w.Body.String()

	// Must contain thinking block (so we know reasoning was handled).
	if !strings.Contains(out, `"type":"thinking"`) {
		t.Errorf("expected thinking content block in stream:\n%s", out)
	}
	// Must NOT have stop_reason tool_use — reasoning is not a tool call.
	if strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Errorf("reasoning-only response must not have stop_reason=tool_use:\n%s", out)
	}
	// Must have end_turn stop reason.
	if !strings.Contains(out, `"stop_reason":"end_turn"`) {
		t.Errorf("expected stop_reason=end_turn for reasoning-only response:\n%s", out)
	}
}

// TestHandleStreamCodexReasoningThenToolInterleave verifies that when
// reasoning deltas arrive, then a function_call is opened, then more
// reasoning deltas arrive, the thinking deltas land on the reasoning
// block index (not the tool block) and the tool block is not stopped
// early by the reasoning done event.
func TestHandleStreamCodexReasoningThenToolInterleave(t *testing.T) {
	sse := strings.Join([]string{
		// Reasoning starts
		`data: {"type":"response.reasoning_summary_text.delta","delta":"thinking..."}`,
		// Tool call opens between reasoning deltas
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_T","call_id":"call_T","name":"read_file"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_T","delta":"{\"path\":\".\"}"}`,
		// More reasoning arrives (after tool block was opened)
		`data: {"type":"response.reasoning_summary_text.delta","delta":" still thinking."}`,
		// Reasoning finishes
		`data: {"type":"response.reasoning_summary_text.done"}`,
		// Completion
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
	out := w.Body.String()

	// Reasoning block should be at index 1 (after text block 0)
	if !strings.Contains(out, `"type":"thinking"`) {
		t.Errorf("expected thinking content block:\n%s", out)
	}

	// Tool block should be at index 2
	if !strings.Contains(out, `"call_T"`) {
		t.Errorf("expected tool_use block for call_T:\n%s", out)
	}

	// The thinking_delta for " still thinking." must target the reasoning
	// block (index 1), not the tool block (index 2).
	thinkDelta := `"index":1`
	stillThinking := `"thinking":" still thinking."`
	// Find the line with " still thinking." and check it targets index 1
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, stillThinking) {
			if !strings.Contains(line, thinkDelta) {
				t.Errorf("reasoning delta ' still thinking.' should target index 1, got:\n%s", line)
			}
		}
	}

	// The reasoning done event must NOT stop the tool block (index 2).
	// The tool block should still be open and stopped in finalize.
	// Final stop reason should be tool_use (we have a function_call).
	if !strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Errorf("expected stop_reason=tool_use for interleaved reasoning+tool:\n%s", out)
	}
}

// TestHandleStreamCodexIncompleteReasonMapping verifies that
// response.incomplete events parse incomplete_details.reason and map
// to the correct Anthropic stop_reason (not hard-coded max_tokens).
func TestHandleStreamCodexIncompleteReasonMapping(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		wantStop   string
	}{
		{"content_filter maps to end_turn", "content_filter", "end_turn"},
		{"max_output_tokens maps to max_tokens", "max_output_tokens", "max_tokens"},
		{"max_tokens maps to max_tokens", "max_tokens", "max_tokens"},
		{"unknown maps to end_turn", "something_else", "end_turn"},
		{"empty reason maps to end_turn", "", "end_turn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			incompleteDetails := ""
			if tt.reason != "" {
				incompleteDetails = `,"incomplete_details":{"reason":"` + tt.reason + `"}`
			}
			sse := strings.Join([]string{
				`data: {"type":"response.output_text.delta","delta":"partial"}`,
				`data: {"type":"response.incomplete","response":{"usage":{"input_tokens":10,"output_tokens":5}` + incompleteDetails + `}}`,
				``,
			}, "\n\n")

			w := httptest.NewRecorder()
			HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
			out := w.Body.String()

			want := `"stop_reason":"` + tt.wantStop + `"`
			if !strings.Contains(out, want) {
				t.Errorf("expected %s in output:\n%s", want, out)
			}
		})
	}
}

// TestHandleStreamCodexWebSearchCall verifies that a web_search_call
// output item is translated into Anthropic server_tool_use +
// web_search_tool_result content blocks in the SSE stream.
func TestHandleStreamCodexWebSearchCall(t *testing.T) {
	sse := strings.Join([]string{
		// Text before web search
		`data: {"type":"response.output_text.delta","delta":"Searching..."}`,
		`data: {"type":"response.output_item.added","item":{"type":"web_search_call","id":"ws_1","action":{"type":"web_search","query":"latest Go release"}}}`,
		`data: {"type":"response.web_search_call.searching","item_id":"ws_1"}`,
		`data: {"type":"response.web_search_call.completed","item_id":"ws_1"}`,
		// output_item.done carries action.sources when include was set
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_1","action":{"type":"web_search","query":"latest Go release","sources":[{"url":"https://go.dev/blog/go1.24","title":"Go 1.24 Release Notes"},{"url":"https://example.com","title":"Example"}]}}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":50,"output_tokens":20}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
	out := w.Body.String()

	// Must have server_tool_use block_start
	if !strings.Contains(out, `"type":"server_tool_use"`) {
		t.Errorf("expected server_tool_use block in stream:\n%s", out)
	}
	if !strings.Contains(out, `"name":"web_search"`) {
		t.Errorf("expected web_search name in stream:\n%s", out)
	}
	if !strings.Contains(out, `"query":"latest Go release"`) {
		t.Errorf("expected query in server_tool_use input:\n%s", out)
	}
	// Must have web_search_tool_result block with sources
	if !strings.Contains(out, `"type":"web_search_tool_result"`) {
		t.Errorf("expected web_search_tool_result block in stream:\n%s", out)
	}
	if !strings.Contains(out, `"type":"web_search_result"`) {
		t.Errorf("expected web_search_result entries in stream:\n%s", out)
	}
	if !strings.Contains(out, `https://go.dev/blog/go1.24`) {
		t.Errorf("expected source URL in stream:\n%s", out)
	}
	if !strings.Contains(out, `Go 1.24 Release Notes`) {
		t.Errorf("expected source title in stream:\n%s", out)
	}
	// web_search is server-side — stop_reason must be end_turn, not tool_use
	if !strings.Contains(out, `"stop_reason":"end_turn"`) {
		t.Errorf("expected stop_reason=end_turn:\n%s", out)
	}
	if strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Errorf("web_search must not produce stop_reason=tool_use:\n%s", out)
	}
	// Text before web search should appear
	if !strings.Contains(out, `Searching...`) {
		t.Errorf("expected text output in stream:\n%s", out)
	}
}

// TestHandleStreamCodexWebSearchCallDoneOnly verifies the safety path
// where response.output_item.done closes a web_search_call block that
// was not closed by web_search_call.completed.
func TestHandleStreamCodexWebSearchCallDoneOnly(t *testing.T) {
	// When web_search_call.completed is never sent, output_item.done
	// should still close the block and emit web_search_tool_result.
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"web_search_call","id":"ws_2","action":{"type":"web_search","query":"Go 1.24"}}}`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_2","action":{"type":"web_search","query":"Go 1.24","sources":[{"url":"https://go.dev","title":"Go"}]}}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
	out := w.Body.String()

	if !strings.Contains(out, `"type":"server_tool_use"`) {
		t.Errorf("expected server_tool_use block:\n%s", out)
	}
	if !strings.Contains(out, `"type":"web_search_tool_result"`) {
		t.Errorf("expected web_search_tool_result block:\n%s", out)
	}
	if !strings.Contains(out, `https://go.dev`) {
		t.Errorf("expected source URL in web_search_tool_result:\n%s", out)
	}
	if !strings.Contains(out, `"stop_reason":"end_turn"`) {
		t.Errorf("expected stop_reason=end_turn:\n%s", out)
	}
}

// TestHandleStreamCodexWebSearchNoSources verifies that when
// output_item.done has no sources, an empty content array is emitted.
func TestHandleStreamCodexWebSearchNoSources(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"web_search_call","id":"ws_3","action":{"type":"web_search","query":"test"}}}`,
		`data: {"type":"response.web_search_call.completed","item_id":"ws_3"}`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_3","action":{"type":"web_search","query":"test"}}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
	out := w.Body.String()

	if !strings.Contains(out, `"type":"web_search_tool_result"`) {
		t.Errorf("expected web_search_tool_result block:\n%s", out)
	}
	// Should have empty content array (no web_search_result entries)
	if strings.Contains(out, `"type":"web_search_result"`) {
		t.Errorf("should not have web_search_result when no sources:\n%s", out)
	}
}

// TestHandleStreamCodexSingleToolCall is a sanity check that the
// item_id routing still works for the common single-tool case.
func TestHandleStreamCodexSingleToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_X","call_id":"call_X","name":"ls"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_X","delta":"{\"path\":\".\"}"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
		``,
	}, "\n\n")

	w := httptest.NewRecorder()
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true, 100)
	out := w.Body.String()

	if !strings.Contains(out, `{"delta":{"partial_json":"{\"path\":\".\"}","type":"input_json_delta"},"index":1,"type":"content_block_delta"}`) {
		t.Errorf("expected single-tool delta on index 1:\n%s", out)
	}
}
