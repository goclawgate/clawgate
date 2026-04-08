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
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true)

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
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true)
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
	HandleStream(w, newFakeBody(sse), "claude-3-5-sonnet", true)
	out := w.Body.String()

	if !strings.Contains(out, `{"delta":{"partial_json":"{\"path\":\".\"}","type":"input_json_delta"},"index":1,"type":"content_block_delta"}`) {
		t.Errorf("expected single-tool delta on index 1:\n%s", out)
	}
}
