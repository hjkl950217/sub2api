package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// issue #7859：input_audio 不能在 Chat → Responses 转换时被静默丢弃。
// 音频以 audio/<format> 的 data URI 转成 input_image，走 Gemini/Antigravity
// 的 inline data 链路。
func TestChatCompletionsToResponsesPreservesInputAudio(t *testing.T) {
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gemini-3.7-flash-high",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"Transcribe this audio:"},
			{"type":"input_audio","input_audio":{"data":"QUJD","format":"ogg"}}
		]}]
	}`), &req))

	out, err := ChatCompletionsToResponses(&req)
	require.NoError(t, err)
	input := string(out.Input)
	require.Contains(t, input, `"type":"input_image"`)
	require.Contains(t, input, `"image_url":"data:audio/ogg;base64,QUJD"`)
}

func TestChatCompletionsToResponsesSkipsEmptyInputAudio(t *testing.T) {
	var req ChatCompletionsRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gemini-3.7-flash-high",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"hi"},
			{"type":"input_audio","input_audio":{"data":"","format":"ogg"}}
		]}]
	}`), &req))

	out, err := ChatCompletionsToResponses(&req)
	require.NoError(t, err)
	require.NotContains(t, string(out.Input), "input_image")
}
