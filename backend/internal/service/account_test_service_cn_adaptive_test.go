//go:build unit

package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func adaptiveCNAccountTestAccount(id int64, platform string) *Account {
	return &Account{
		ID:          id,
		Name:        "adaptive-cn-test",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-adaptive-test",
			"api_protocol": APIProtocolAdaptive,
			"api_base_urls": map[string]any{
				APIProtocolChatCompletions: "http://chat.example/v1",
				APIProtocolAnthropic:       "http://anthropic.example",
				APIProtocolResponses:       "http://responses.example",
			},
		},
	}
}

// adaptiveCNResponseKey 把探测路径归到协议键上，供按协议预置响应。
func adaptiveCNResponseKey(path string) string {
	switch {
	case strings.Contains(path, "/v1/messages"):
		return APIProtocolAnthropic
	case strings.HasSuffix(path, "/responses"):
		return APIProtocolResponses
	default:
		return APIProtocolChatCompletions
	}
}

func adaptiveCNAccountTestService(account *Account, responses ...*http.Response) (*AccountTestService, *httpUpstreamRecorder) {
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	upstream := &httpUpstreamRecorder{responses: responses}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}, upstream
}

// adaptiveCNProtocolUpstream 按协议路径返回预置响应，可安全并发调用。
// 二开：adaptive/复选路径改为多协议并发后，不能再按到达顺序分配响应。
type adaptiveCNProtocolUpstream struct {
	mu        sync.Mutex
	requests  []*http.Request
	bodies    [][]byte
	responses map[string]*http.Response
}

func (u *adaptiveCNProtocolUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (u *adaptiveCNProtocolUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	key := adaptiveCNResponseKey(req.URL.Path)
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.bodies = append(u.bodies, body)
	resp := u.responses[key]
	u.mu.Unlock()
	if resp == nil {
		return nil, fmt.Errorf("no mocked response for %s", req.URL.Path)
	}
	return resp, nil
}

func (u *adaptiveCNProtocolUpstream) requestByProtocol(t *testing.T, protocol string) *http.Request {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, req := range u.requests {
		if adaptiveCNResponseKey(req.URL.Path) == protocol {
			return req
		}
	}
	require.Failf(t, "missing request", "protocol %s 没有发出请求", protocol)
	return nil
}

func (u *adaptiveCNProtocolUpstream) bodyByProtocol(t *testing.T, protocol string) []byte {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	for index, req := range u.requests {
		if adaptiveCNResponseKey(req.URL.Path) == protocol {
			return u.bodies[index]
		}
	}
	return nil
}

// adaptiveCNProtocolTestService 与 adaptiveCNAccountTestService 相同，但上游按协议预置响应，
// 供多协议并发探测的测试使用。
func adaptiveCNProtocolTestService(account *Account, responses map[string]*http.Response) (*AccountTestService, *adaptiveCNProtocolUpstream) {
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	upstream := &adaptiveCNProtocolUpstream{responses: responses}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}, upstream
}

// adaptiveCNResponses 组装按协议预置的响应；nil 表示该协议没有预置（探测会报错）。
func adaptiveCNResponses(chat, anthropic, responses *http.Response) map[string]*http.Response {
	out := map[string]*http.Response{}
	if chat != nil {
		out[APIProtocolChatCompletions] = chat
	}
	if anthropic != nil {
		out[APIProtocolAnthropic] = anthropic
	}
	if responses != nil {
		out[APIProtocolResponses] = responses
	}
	return out
}

func adaptiveCNChatTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"chat ok"},"finish_reason":"stop"}]}

data: [DONE]

`)),
	}
}

func adaptiveCNAnthropicTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"type":"content_block_delta","delta":{"text":"anthropic ok"}}

data: {"type":"message_stop"}

`)),
	}
}

func adaptiveCNResponsesTestResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(`data: {"type":"response.output_text.delta","delta":"responses ok"}

data: {"type":"response.completed"}

`)),
	}
}

func TestAccountTestService_AdaptiveChatOnlyProvidersTestChatAndAnthropicEndpoints(t *testing.T) {
	account := adaptiveCNAccountTestAccount(301, PlatformZhipu)
	svc, upstream := adaptiveCNProtocolTestService(
		account,
		adaptiveCNResponses(adaptiveCNChatTestResponse(), adaptiveCNAnthropicTestResponse(), nil),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "hello", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	chatReq := upstream.requestByProtocol(t, APIProtocolChatCompletions)
	anthropicReq := upstream.requestByProtocol(t, APIProtocolAnthropic)
	require.Equal(t, "http://chat.example/v1/chat/completions", chatReq.URL.String())
	require.Equal(t, "http://anthropic.example/v1/messages", anthropicReq.URL.String())
	require.Equal(t, "Bearer sk-adaptive-test", chatReq.Header.Get("Authorization"))
	require.Equal(t, "sk-adaptive-test", anthropicReq.Header.Get("x-api-key"))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_start"`))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_complete"`))
	// 二开：端点探测结果改为逐协议回传，正文与结论都在 protocol_result 上
	require.Contains(t, recorder.Body.String(), `"protocol":"anthropic","protocol_ok":true`)
}

func TestAccountTestService_AdaptiveDeepSeekAlsoTestsResponsesEndpoint(t *testing.T) {
	account := adaptiveCNAccountTestAccount(302, PlatformDeepseek)
	svc, upstream := adaptiveCNProtocolTestService(
		account,
		adaptiveCNResponses(adaptiveCNChatTestResponse(), adaptiveCNAnthropicTestResponse(), adaptiveCNResponsesTestResponse()),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-chat", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	responsesReq := upstream.requestByProtocol(t, APIProtocolResponses)
	require.Equal(t, "http://responses.example/responses", responsesReq.URL.String())
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(responsesReq.Context()))
	require.Equal(t, "Bearer sk-adaptive-test", responsesReq.Header.Get("Authorization"))
	responsesBody := upstream.bodyByProtocol(t, APIProtocolResponses)
	require.True(t, gjson.GetBytes(responsesBody, "stream").Bool())
	require.False(t, gjson.GetBytes(responsesBody, "store").Bool())
	require.False(t, gjson.GetBytes(responsesBody, "instructions").Exists())
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_complete"`))
	require.Contains(t, recorder.Body.String(), `"protocol":"responses","protocol_ok":true`)
}

func TestAccountTestService_AdaptiveKimiAlsoTestsResponsesEndpoint(t *testing.T) {
	account := adaptiveCNAccountTestAccount(306, PlatformKimi)
	svc, upstream := adaptiveCNProtocolTestService(
		account,
		adaptiveCNResponses(adaptiveCNChatTestResponse(), adaptiveCNAnthropicTestResponse(), adaptiveCNResponsesTestResponse()),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "k3-256k", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	responsesReq := upstream.requestByProtocol(t, APIProtocolResponses)
	require.Equal(t, "http://responses.example/v1/responses", responsesReq.URL.String())
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(responsesReq.Context()))
	require.Equal(t, "Bearer sk-adaptive-test", responsesReq.Header.Get("Authorization"))
	responsesBody := upstream.bodyByProtocol(t, APIProtocolResponses)
	require.True(t, gjson.GetBytes(responsesBody, "stream").Bool())
	require.False(t, gjson.GetBytes(responsesBody, "store").Bool())
	require.False(t, gjson.GetBytes(responsesBody, "instructions").Exists())
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_complete"`))
	require.Contains(t, recorder.Body.String(), `"protocol":"responses","protocol_ok":true`)
}

func TestAccountTestService_AdaptiveStopsAndNamesFailingEndpoint(t *testing.T) {
	account := adaptiveCNAccountTestAccount(303, PlatformDeepseek)
	svc, upstream := adaptiveCNProtocolTestService(
		account,
		adaptiveCNResponses(
			adaptiveCNChatTestResponse(),
			newJSONResponse(http.StatusNotFound, `{"error":{"message":"missing messages route"}}`),
			adaptiveCNResponsesTestResponse(),
		),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-chat", "", AccountTestModeDefault)

	require.Error(t, err)
	require.Contains(t, err.Error(), "Adaptive Anthropic endpoint returned 404")
	// 二开：并发探测不再首错即停，三个端点都会发出去
	require.Len(t, upstream.requests, 3)
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
	require.NotContains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_AdaptiveRejectsInvalidAnthropicSuccessBody(t *testing.T) {
	account := adaptiveCNAccountTestAccount(305, PlatformKimi)
	svc, upstream := adaptiveCNProtocolTestService(
		account,
		adaptiveCNResponses(
			adaptiveCNChatTestResponse(),
			newJSONResponse(http.StatusOK, `<html>not an Anthropic stream</html>`),
			adaptiveCNResponsesTestResponse(),
		),
	)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "kimi-k2.5", "", AccountTestModeDefault)

	require.Error(t, err)
	require.Contains(t, err.Error(), "Adaptive Anthropic stream ended before message_stop")
	require.Len(t, upstream.requests, 3)
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
	require.NotContains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_FixedCNChatProtocolStillTestsOnlyChatEndpoint(t *testing.T) {
	account := adaptiveCNAccountTestAccount(304, PlatformZhipu)
	account.Credentials["api_protocol"] = APIProtocolChatCompletions
	account.Credentials["base_url"] = "http://fixed-chat.example/v1"
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNChatTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "http://fixed-chat.example/v1/chat/completions", upstream.requests[0].URL.String())
	require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"test_complete"`))
}

func anthropicProtocolCNAccount(id int64, platform string, credentials map[string]any) *Account {
	base := map[string]any{
		"api_key":      "sk-anthropic-test",
		"api_protocol": APIProtocolAnthropic,
	}
	for key, value := range credentials {
		base[key] = value
	}
	return &Account{
		ID:          id,
		Name:        "anthropic-protocol-cn-test",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: base,
	}
}

func TestAccountTestService_AnthropicProtocolProbesNativeEndpointWithoutBetaQuery(t *testing.T) {
	account := anthropicProtocolCNAccount(311, PlatformZhipu, map[string]any{
		"base_url": "https://open.bigmodel.cn/api/anthropic",
	})
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNAnthropicTestResponse())
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	// Native Anthropic path without the ?beta=true suffix the generic Claude tester appends.
	require.Equal(t, "https://open.bigmodel.cn/api/anthropic/v1/messages", req.URL.String())
	require.Empty(t, req.URL.RawQuery)
	require.Equal(t, "sk-anthropic-test", req.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", req.Header.Get("anthropic-version"))
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_AnthropicProtocolFallsBackToProviderDefaultNotAnthropicDotCom(t *testing.T) {
	// base_url intentionally absent: forwarding resolves the per-platform default
	// Anthropic endpoint. The old fall-through probed https://api.anthropic.com
	// with the provider's API key.
	account := anthropicProtocolCNAccount(312, PlatformZhipu, nil)
	svc, upstream := adaptiveCNAccountTestService(account, adaptiveCNAnthropicTestResponse())
	c, _ := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://open.bigmodel.cn/api/anthropic/v1/messages", upstream.requests[0].URL.String())
}

func TestAccountTestService_AnthropicProtocolRejectsOpenAICompatBaseURL(t *testing.T) {
	account := anthropicProtocolCNAccount(313, PlatformZhipu, map[string]any{
		"base_url": "https://open.bigmodel.cn/api/paas/v4",
	})
	svc, upstream := adaptiveCNAccountTestService(account)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "", AccountTestModeDefault)

	require.Error(t, err)
	// Fails fast locally: no upstream request with the wrong endpoint shape.
	require.Empty(t, upstream.requests)
	require.Contains(t, recorder.Body.String(), "looks like an OpenAI-compatible endpoint")
	require.Contains(t, recorder.Body.String(), "https://open.bigmodel.cn/api/anthropic")
}

func TestAccountTestService_AnthropicProtocol401MarksAccountError(t *testing.T) {
	account := anthropicProtocolCNAccount(314, PlatformKimi, map[string]any{
		"base_url": "https://api.moonshot.cn/anthropic",
	})
	svc, _ := adaptiveCNAccountTestService(
		account,
		newJSONResponse(http.StatusUnauthorized, `{"error":{"message":"invalid key"}}`),
	)
	c, _ := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "kimi-k2.5", "", AccountTestModeDefault)

	require.Error(t, err)
	require.Contains(t, err.Error(), "Anthropic endpoint returned 401")
	repo := svc.accountRepo.(*openAIAccountTestRepo)
	require.Equal(t, account.ID, repo.setErrorID)
}
