//go:build unit

// FORK: 二开「协议探测矩阵 + 一键更新支持协议」的单测。
// 覆盖三条语义：逐协议探测、按通过结果回写勾选集合、全部失败时保持原配置。
package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type protocolSyncTestRepo struct {
	openAIAccountTestRepo
	mu            sync.Mutex
	updated       *Account
	setErrorCalls int
}

func (r *protocolSyncTestRepo) Update(_ context.Context, account *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updated = account
	return nil
}

func (r *protocolSyncTestRepo) SetError(context.Context, int64, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setErrorCalls++
	return nil
}

type protocolMatrixTestUpstream struct {
	mu               sync.Mutex
	active           int
	maxActive        int
	expectedRequests int
	concurrent       bool
	ready            chan struct{}
	once             sync.Once
	requests         []*http.Request
	responses        map[string]func() *http.Response
}

func (u *protocolMatrixTestUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (u *protocolMatrixTestUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.mu.Lock()
	u.active++
	u.requests = append(u.requests, req)
	if u.active > u.maxActive {
		u.maxActive = u.active
	}
	requestCount := len(u.requests)
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.active--
		u.mu.Unlock()
	}()
	if u.concurrent {
		if requestCount >= u.expectedRequests {
			u.once.Do(func() { close(u.ready) })
		}
		<-u.ready
	}
	protocol := APIProtocolChatCompletions
	switch {
	case strings.Contains(req.URL.Path, "/v1/messages"):
		protocol = APIProtocolAnthropic
	case strings.HasSuffix(req.URL.Path, "/responses"):
		protocol = APIProtocolResponses
	}
	return u.responses[protocol](), nil
}

func protocolMatrixResponses(chat, anthropic, responses func() *http.Response) map[string]func() *http.Response {
	out := map[string]func() *http.Response{}
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

func protocolSyncTestAccount(id int64, platform string, protocols []any) *Account {
	return &Account{
		ID:          id,
		Name:        "protocol-sync-test",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Groups:      []*Group{{Name: "赛博羊毛-DS"}},
		Credentials: map[string]any{
			"api_key":           "sk-protocol-sync",
			"api_protocol":      APIProtocolAdaptive,
			"api_protocols":     protocols,
			"fallback_protocol": APIProtocolChatCompletions,
			"api_base_urls": map[string]any{
				APIProtocolChatCompletions: "http://chat.example/v1",
				APIProtocolAnthropic:       "http://anthropic.example",
				APIProtocolResponses:       "http://responses.example",
			},
		},
	}
}

func protocolSyncTestService(account *Account, responses map[string]func() *http.Response, concurrent ...bool) (*AccountTestService, *protocolMatrixTestUpstream, *protocolSyncTestRepo) {
	repo := &protocolSyncTestRepo{
		openAIAccountTestRepo: openAIAccountTestRepo{
			mockAccountRepoForGemini: mockAccountRepoForGemini{
				accountsByID: map[int64]*Account{account.ID: account},
			},
		},
	}
	upstream := &protocolMatrixTestUpstream{
		expectedRequests: len(responses),
		concurrent:       len(concurrent) > 0 && concurrent[0],
		ready:            make(chan struct{}),
		responses:        responses,
	}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}, upstream, repo
}

func protocolMatrixRequestBody(t *testing.T, request *http.Request) string {
	t.Helper()
	body, err := request.GetBody()
	require.NoError(t, err)
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	return string(payload)
}

// 语义：三个协议各测一次，测通的写回 api_protocols，兜底取 chat_completions 优先。
func TestForkProtocolSyncMatrixAppliesPassedProtocols(t *testing.T) {
	account := protocolSyncTestAccount(901, PlatformKimi, []any{
		APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses,
	})
	svc, upstream, repo := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return adaptiveCNChatTestResponse() },
		func() *http.Response {
			return newJSONResponse(http.StatusBadRequest, `{"error":"no anthropic endpoint"}`)
		},
		func() *http.Response { return adaptiveCNResponsesTestResponse() },
	))
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "kimi-k2", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3, "三个协议各探测一次")
	require.Equal(t, 1, upstream.maxActive, "非 DS 分组保留串行探测")
	paths := map[string]bool{}
	for _, request := range upstream.requests {
		paths[request.URL.Path] = true
	}
	require.True(t, paths["/v1/chat/completions"])
	require.True(t, paths["/v1/messages"])
	require.True(t, paths["/v1/responses"])

	require.NotNil(t, repo.updated, "探测结果必须回写账号")
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, repo.updated.Credentials["api_protocols"])
	require.Equal(t, APIProtocolChatCompletions, repo.updated.Credentials["fallback_protocol"])

	body := recorder.Body.String()
	require.Equal(t, 3, strings.Count(body, `"type":"protocol_result"`), "每个协议各一条结果事件")
	require.Contains(t, body, `"protocol":"anthropic","protocol_ok":false`)
	require.Contains(t, body, `"protocol":"responses","protocol_ok":true`)
	require.Contains(t, body, `"protocol_applied":true`)
	require.Equal(t, 1, strings.Count(body, `"type":"test_complete"`), "内层探针的 test_complete 不得冒泡")
	// 单个协议失败不再作为终止错误抛出（否则前端会显示测试失败）。
	require.NotContains(t, body, `"type":"error"`)
}

// 语义：三个协议全失败时不改动账号配置，并明确告知未回写。
func TestForkProtocolSyncMatrixAllFailedKeepsConfig(t *testing.T) {
	account := protocolSyncTestAccount(902, PlatformDeepseek, []any{APIProtocolAnthropic})
	svc, upstream, repo := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return newJSONResponse(http.StatusBadRequest, `{"error":"chat down"}`) },
		func() *http.Response { return newJSONResponse(http.StatusBadRequest, `{"error":"anthropic down"}`) },
		func() *http.Response { return newJSONResponse(http.StatusBadRequest, `{"error":"responses down"}`) },
	), true)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-chat", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, 3, upstream.maxActive, "DS 分组同步测试也应并发探测")
	require.Nil(t, repo.updated, "全部失败时不得写库")
	require.Equal(t, []any{APIProtocolAnthropic}, account.Credentials["api_protocols"], "原勾选集合保持不变")

	body := recorder.Body.String()
	require.NotContains(t, body, `"protocol_applied":true`, "未回写时不得声称已更新")
	// success 带 omitempty：false 时不出现该字段，前端按"非 true 即失败"处理。
	require.Contains(t, body, `"type":"test_complete"`, "必须发最终完成事件")
	require.NotContains(t, body, `"success":true`, "全失败时不得报告成功")
	require.Equal(t, 3, strings.Count(body, `"type":"protocol_result"`))
	require.Equal(t, 3, strings.Count(body, `"protocol_ok":false`))
	require.NotContains(t, body, `"type":"error"`, "单协议失败不产生终止错误事件")
}

// 普通连接测试在赛博羊毛-DS 中也并发探测三协议，但不写回账号配置。
func TestForkDeepseekDSConnectionTestProbesAllProtocolsConcurrently(t *testing.T) {
	account := protocolSyncTestAccount(904, PlatformDeepseek, []any{APIProtocolChatCompletions})
	svc, upstream, repo := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return adaptiveCNChatTestResponse() },
		func() *http.Response { return newJSONResponse(http.StatusUnauthorized, `{"error":"invalid token"}`) },
		func() *http.Response { return adaptiveCNResponsesTestResponse() },
	), true)
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "deepseek-chat", "hello", AccountTestModeDefault)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, 3, upstream.maxActive, "三个协议请求应同时进行")
	for _, request := range upstream.requests {
		require.Contains(t, protocolMatrixRequestBody(t, request), cnProviderConnectionTestPrompt)
	}
	require.Nil(t, repo.updated, "普通连接测试不得写回协议配置")
	require.Zero(t, repo.setErrorCalls, "单协议认证失败不得把账号状态改成 error")
	require.Equal(t, 3, strings.Count(recorder.Body.String(), `"type":"protocol_result"`))
	require.Contains(t, recorder.Body.String(), `"protocol":"anthropic","protocol_ok":false`)
	require.NotContains(t, recorder.Body.String(), `"protocol_applied":true`)
}

func TestForkUpdateProbedCNProtocolsSavesResultsWithoutUpstreamRequests(t *testing.T) {
	account := protocolSyncTestAccount(905, PlatformDeepseek, []any{APIProtocolAnthropic})
	svc, upstream, repo := protocolSyncTestService(account, nil)

	err := svc.UpdateProbedCNProtocols(context.Background(), account.ID, []string{
		APIProtocolResponses, APIProtocolChatCompletions,
	})

	require.NoError(t, err)
	require.Empty(t, upstream.requests, "保存既有结果不得请求上游")
	require.NotNil(t, repo.updated)
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolResponses}, repo.updated.Credentials["api_protocols"])
	require.Equal(t, APIProtocolChatCompletions, repo.updated.Credentials["fallback_protocol"])
}

// 语义：平台没有原生 Responses 端点时跳过该协议，不发请求。
func TestForkProtocolSyncMatrixSkipsUnsupportedResponses(t *testing.T) {
	account := protocolSyncTestAccount(903, PlatformZhipu, []any{
		APIProtocolChatCompletions, APIProtocolAnthropic,
	})
	svc, upstream, repo := protocolSyncTestService(account, protocolMatrixResponses(
		func() *http.Response { return adaptiveCNChatTestResponse() },
		func() *http.Response { return adaptiveCNAnthropicTestResponse() },
		nil,
	))
	c, recorder := newTestContext()

	err := svc.TestAccountConnection(c, account.ID, "glm-4.7", "hello", AccountTestModeDefault, AccountTestOptions{SyncProtocols: true})

	require.NoError(t, err)
	require.Len(t, upstream.requests, 2, "zhipu 无原生 Responses，不发第三个请求")
	require.Equal(t, []string{APIProtocolChatCompletions, APIProtocolAnthropic}, repo.updated.Credentials["api_protocols"])
	body := recorder.Body.String()
	require.Contains(t, body, `"protocol":"responses","protocol_ok":false`)
	require.Contains(t, body, `"protocol_applied":true`)
}
