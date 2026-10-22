package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountGrokNativeSearchCallsFromJSONBytes(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, countGrokNativeSearchCallsFromJSONBytes(nil))
	require.Equal(t, 0, countGrokNativeSearchCallsFromJSONBytes([]byte(`{"output":[]}`)))
	body := []byte(`{"output":[
		{"type":"web_search_call","id":"ws1","status":"completed"},
		{"type":"x_search_call","id":"xs1"},
		{"type":"function_call","name":"tool_search","call_id":"ts1"},
		{"type":"function_call","name":"lookup","call_id":"other"}
	]}`)
	require.Equal(t, 2, countGrokNativeSearchCallsFromJSONBytes(body))
}

func TestCountGrokNativeSearchCallsFromJSONBytes_PrefersNestedResponse(t *testing.T) {
	t.Parallel()
	body := []byte(`{"output":[{"type":"web_search_call","id":"duplicate"}],"response":{"output":[{"type":"web_search_call","id":"duplicate"},{"type":"x_search_call","id":"xs1"}]}}`)
	require.Equal(t, 2, countGrokNativeSearchCallsFromJSONBytes(body))
}

func TestCountGrokNativeSearchCallsFromJSONBytes_FallsBackWhenNestedOutputNull(t *testing.T) {
	t.Parallel()
	body := []byte(`{"output":[{"type":"web_search_call","id":"ws1"}],"response":{"output":null}}`)
	require.Equal(t, 1, countGrokNativeSearchCallsFromJSONBytes(body))
}

func TestCountGrokNativeSearchCallsFromSSEBodyDedups(t *testing.T) {
	t.Parallel()
	sse := stringsJoin(
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","call_id":"c1"}}`,
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","call_id":"c1"}}`,
		`data: {"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"ws1","call_id":"c1"},{"type":"x_search_call","id":"xs1","call_id":"c2"}]}}`,
	)
	require.Equal(t, 2, countGrokNativeSearchCallsFromSSEBody(sse))
}

func TestCountGrokNativeSearchCallsInSSEDataDedup_LiveStreamPath(t *testing.T) {
	t.Parallel()
	// Mirrors the live streaming accumulator: item.done then response.completed
	// for the same call_id must bill once (regression for ~2× surcharge).
	seen := make(map[string]struct{})
	done := []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws1","call_id":"c1"}}`)
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"ws1","call_id":"c1"},{"type":"x_search_call","id":"xs1","call_id":"c2"}]}}`)
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(done, seen))
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(completed, seen))
	// Raw (no-dedup) path still double-counts the same envelope pair.
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEData(done))
	require.Equal(t, 2, countGrokNativeSearchCallsInSSEData(completed))
}

func TestCountGrokNativeSearchCallsInSSEDataDedup_NoIDStillDedups(t *testing.T) {
	t.Parallel()
	// Upstream sometimes omits call_id/id; synthetic keys must still prevent 2×.
	seen := make(map[string]struct{})
	done := []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`)
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"web_search_call"}]}}`)
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(done, seen))
	require.Equal(t, 0, countGrokNativeSearchCallsInSSEDataDedup(completed, seen))
}

func TestCountGrokNativeSearchCallsInSSEDataDedup_MultipleNoIDCalls(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	firstDone := []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`)
	secondDone := []byte(`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`)
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"web_search_call"},{"type":"web_search_call"}]}}`)
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(firstDone, seen))
	require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(secondDone, seen))
	require.Equal(t, 0, countGrokNativeSearchCallsInSSEDataDedup(completed, seen))
}

func TestCountGrokNativeSearchCallsExcludesClientTools(t *testing.T) {
	t.Parallel()
	for _, item := range []string{
		`{"type":"function_call","name":"web_search","call_id":"client"}`,
		`{"type":"function_call","name":"x_search","call_id":"client"}`,
		`{"type":"function_call","name":"tool_search","call_id":"client"}`,
		`{"type":"custom_tool_call","name":"web_search","call_id":"client"}`,
		`{"type":"custom_tool_call","name":"x_search","call_id":"client"}`,
		`{"type":"custom_tool_call","name":"tool_search","call_id":"client"}`,
		`{"type":"tool_search_call","execution":"client","call_id":"client"}`,
	} {
		t.Run(item, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"output":[%s,{"type":"web_search_call","id":"server"}]}`, item))
			require.Equal(t, 1, countGrokNativeSearchCallsFromJSONBytes(body))
			done := []byte(fmt.Sprintf(`{"type":"response.output_item.done","item":%s}`, item))
			completed := []byte(fmt.Sprintf(`{"type":"response.completed","response":%s}`, body))
			seen := make(map[string]struct{})
			require.Zero(t, countGrokNativeSearchCallsInSSEDataDedup(done, seen))
			require.Equal(t, 1, countGrokNativeSearchCallsInSSEDataDedup(completed, seen))
			require.Zero(t, countGrokNativeSearchCallsInSSEDataDedup(completed, seen))
			require.Equal(t, 1, countGrokNativeSearchCallsFromSSEBody(stringsJoin("data: "+string(done), "data: "+string(completed))))
		})
	}
}

// xAI 按次计费的服务端工具都要计入：code_interpreter / file_search /
// collections_search / attachment_search（issue #7690 的漏计部分）。
func TestCountGrokNativeSearchCallsCountsServerSideTools(t *testing.T) {
	t.Parallel()
	body := []byte(`{"output":[
		{"type":"code_interpreter_call","id":"ci1"},
		{"type":"code_execution_call","id":"ce1"},
		{"type":"file_search_call","id":"fs1"},
		{"type":"collections_search_call","id":"cs1"},
		{"type":"attachment_search","id":"as1"}
	]}`)
	require.Equal(t, 5, countGrokNativeSearchCallsFromJSONBytes(body))
}

func stringsJoin(lines ...string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n\n"
	}
	return out
}
