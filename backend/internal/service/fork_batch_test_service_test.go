//go:build unit

// FORK: 二开「批量账号测试」的单测（新增文件，非上游改动）。
// 覆盖：按平台分组与共有模型交集、平台/配置不合格的排除、批量执行时
// 逐账号转发事件、并发上限。

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// forkBatchTestAccount 造一个参与批量测试的账号：已配协议复选 + 指定模型键。
func forkBatchTestAccount(id int64, platform string, models ...string) *Account {
	mapping := map[string]any{}
	for _, model := range models {
		mapping[model] = model
	}
	return &Account{
		ID:          id,
		Name:        "batch-" + platform,
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":           "sk-batch",
			"base_url":          "http://batch.example",
			"model_mapping":     mapping,
			"api_protocols":     []any{APIProtocolChatCompletions, APIProtocolResponses},
			"fallback_protocol": APIProtocolChatCompletions,
		},
	}
}

// forkBatchTestService 用多个账号装配一个测试用的 AccountTestService。
func forkBatchTestService(accounts ...*Account) (*AccountTestService, *protocolMatrixTestUpstream, *protocolSyncTestRepo) {
	byID := map[int64]*Account{}
	for _, account := range accounts {
		byID[account.ID] = account
	}
	repo := &protocolSyncTestRepo{
		openAIAccountTestRepo: openAIAccountTestRepo{
			mockAccountRepoForGemini: mockAccountRepoForGemini{accountsByID: byID},
		},
	}
	upstream := &protocolMatrixTestUpstream{
		responses: protocolMatrixResponses(
			func() *http.Response { return adaptiveCNChatTestResponse() },
			nil,
			func() *http.Response { return adaptiveCNResponsesTestResponse() },
		),
	}
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}, upstream, repo
}

// forkBatchTestEvents 把录制到的 SSE 正文拆成事件列表。
func forkBatchTestEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		events = append(events, event)
	}
	return events
}

// 按平台分组，每组算出共有模型；不支持协议探测的平台仍然列出但标记不合格。
func TestForkBatchTestPlanGroupsByPlatform(t *testing.T) {
	t.Parallel()

	svc, _, _ := forkBatchTestService(
		forkBatchTestAccount(1, PlatformOpenAI, "gpt-4o", "gpt-5"),
		forkBatchTestAccount(2, PlatformOpenAI, "gpt-4o", "gpt-5.4"),
		forkBatchTestAccount(3, PlatformDeepseek, "deepseek-chat"),
		forkBatchTestAccount(4, PlatformGrok, "grok-4"),
	)

	plan, err := svc.BuildBatchTestPlan(context.Background(), []int64{1, 2, 3, 4})
	require.NoError(t, err)
	require.Len(t, plan.Groups, 3, "按平台分组")

	require.Equal(t, PlatformDeepseek, plan.Groups[0].Platform)
	require.Equal(t, []string{"deepseek-chat"}, plan.Groups[0].CommonModels)

	require.Equal(t, PlatformGrok, plan.Groups[1].Platform)
	require.False(t, plan.Groups[1].Accounts[0].Eligible, "grok 不参与批量测试")
	require.NotEmpty(t, plan.Groups[1].Accounts[0].Reason)
	require.Empty(t, plan.Groups[1].CommonModels)

	require.Equal(t, PlatformOpenAI, plan.Groups[2].Platform)
	require.Equal(t, []int64{1, 2}, plan.Groups[2].AccountIDs)
	require.Equal(t, []string{"gpt-4o"}, plan.Groups[2].CommonModels, "只保留两个账号共有的模型")

	require.Equal(t, 1, plan.SkippedCount, "只统计不合格账号")
}

// 没配 model_mapping 的账号排除出交集，并给出原因。
func TestForkBatchTestPlanExcludesAccountWithoutModelMapping(t *testing.T) {
	t.Parallel()

	svc, _, _ := forkBatchTestService(forkBatchTestAccount(5, PlatformOpenAI))

	plan, err := svc.BuildBatchTestPlan(context.Background(), []int64{5})
	require.NoError(t, err)
	require.Len(t, plan.Groups, 1)
	require.False(t, plan.Groups[0].Accounts[0].Eligible)
	require.Contains(t, plan.Groups[0].Accounts[0].Reason, "model_mapping")
	require.Empty(t, plan.Groups[0].CommonModels)
	require.Equal(t, 1, plan.SkippedCount)
}

// 取不到的账号（已删除）跳过，不影响其它账号。
func TestForkBatchTestPlanSkipsMissingAccounts(t *testing.T) {
	t.Parallel()

	svc, _, _ := forkBatchTestService(forkBatchTestAccount(7, PlatformOpenAI, "gpt-4o"))

	plan, err := svc.BuildBatchTestPlan(context.Background(), []int64{7, 999})
	require.NoError(t, err)
	require.Len(t, plan.Groups, 1)
	require.Equal(t, 1, plan.SkippedCount)
}

// 批量执行把每个账号的事件都打上 account_id，并以 batch_test_complete 收尾。
func TestForkBatchTestRunForwardsEventsWithAccountID(t *testing.T) {
	svc, _, _ := forkBatchTestService(
		forkBatchTestAccount(11, PlatformOpenAI, "gpt-4o"),
		forkBatchTestAccount(12, PlatformOpenAI, "gpt-4o"),
	)
	c, recorder := newTestContext()

	err := svc.RunAccountBatchTest(c, BatchTestRunOptions{Targets: []BatchTestTarget{
		{AccountID: 11, ModelID: "gpt-4o", Prompt: "hi"},
		{AccountID: 12, ModelID: "gpt-4o", Prompt: "hi"},
	}})
	require.NoError(t, err)

	events := forkBatchTestEvents(t, recorder.Body.String())
	require.NotEmpty(t, events)

	results := map[int64]int{}
	completed := map[int64]map[string]any{}
	for _, event := range events {
		accountID, ok := event["account_id"].(float64)
		require.True(t, ok, "每条事件都要带 account_id")
		if event["type"] == "protocol_result" {
			results[int64(accountID)]++
		}
		if event["type"] == "batch_test_complete" {
			completed[int64(accountID)] = event
		}
	}
	require.Equal(t, 2, results[11], "账号 11 的 chat 与 responses 各一条结果")
	require.Equal(t, 2, results[12])
	require.Contains(t, completed, int64(11), "账号 11 有收尾事件")
	require.Contains(t, completed, int64(12))
	require.Equal(t, true, completed[11]["success"], "全部协议通过时判通过")
	require.Equal(t, true, completed[12]["success"])
}

// 账号测不通时同样要有收尾事件，前端才不会一直停在"测试中"。
func TestForkBatchTestRunCompletesFailedTarget(t *testing.T) {
	svc, upstream, _ := forkBatchTestService(forkBatchTestAccount(21, PlatformOpenAI, "gpt-4o"))
	upstream.responses = protocolMatrixResponses(
		func() *http.Response { return newJSONResponse(http.StatusBadRequest, `{"error":"nope"}`) },
		nil,
		func() *http.Response { return newJSONResponse(http.StatusBadRequest, `{"error":"nope"}`) },
	)
	c, recorder := newTestContext()

	err := svc.RunAccountBatchTest(c, BatchTestRunOptions{Targets: []BatchTestTarget{
		{AccountID: 21, ModelID: "gpt-4o", Prompt: "hi"},
	}})
	require.NoError(t, err)

	events := forkBatchTestEvents(t, recorder.Body.String())
	var completion map[string]any
	probeResults := []map[string]any{}
	for _, event := range events {
		if event["type"] == "batch_test_complete" {
			completion = event
		}
		if event["type"] == "protocol_result" {
			probeResults = append(probeResults, event)
		}
	}
	require.NotNil(t, completion, "失败账号也要有收尾事件")

	// 回归：协议矩阵在两个协议都失败时仍返回 nil，早期实现只看返回值，
	// 结果是「协议全红却标通过」。判定必须取 test_complete 的结论。
	require.Len(t, probeResults, 2)
	for _, result := range probeResults {
		require.Equal(t, false, result["protocol_ok"], "两个协议都应探测失败")
	}
	require.NotEqual(t, true, completion["success"], "全部协议失败时不能判通过")
	require.Nil(t, completion["error"], "具体失败原因由逐协议结果给出，收尾事件不重复带错误文本")
}

// 账号取不到（已删除）时判失败，不给没有结论的账号标「通过」。
func TestForkBatchTestRunFailsWhenAccountMissing(t *testing.T) {
	t.Parallel()

	svc, _, _ := forkBatchTestService()
	c, recorder := newTestContext()

	err := svc.RunAccountBatchTest(c, BatchTestRunOptions{Targets: []BatchTestTarget{
		{AccountID: 999, ModelID: "gpt-4o", Prompt: "hi"},
	}})
	require.NoError(t, err)

	events := forkBatchTestEvents(t, recorder.Body.String())
	require.NotEmpty(t, events, "取不到账号也要有收尾事件")
	last := events[len(events)-1]
	require.Equal(t, "batch_test_complete", last["type"])
	require.NotEqual(t, true, last["success"], "取不到账号不能判通过")
}

// 候选查询带上账号当前的调度开关，弹窗逐行改完后回写。
func TestForkBatchTestPlanCarriesSchedulable(t *testing.T) {
	t.Parallel()

	on := forkBatchTestAccount(6, PlatformOpenAI, "gpt-4o")
	on.Schedulable = true
	off := forkBatchTestAccount(7, PlatformOpenAI, "gpt-4o")
	svc, _, _ := forkBatchTestService(on, off)

	plan, err := svc.BuildBatchTestPlan(context.Background(), []int64{6, 7})
	require.NoError(t, err)
	require.Len(t, plan.Groups, 1)
	require.True(t, plan.Groups[0].Accounts[0].Schedulable)
	require.False(t, plan.Groups[0].Accounts[1].Schedulable)
}

// forkBatchSlowUpstream 在转发前等待一小段时间，让并发上限可被观察到。
type forkBatchSlowUpstream struct {
	*protocolMatrixTestUpstream
	delay time.Duration
}

func (u *forkBatchSlowUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	time.Sleep(u.delay)
	return u.protocolMatrixTestUpstream.DoWithTLS(req, proxyURL, accountID, concurrency, profile)
}

// 并发上限生效：并发设为 1 时，两个账号不会同时打上游。
func TestForkBatchTestRunRespectsConcurrencyLimit(t *testing.T) {
	svc, upstream, _ := forkBatchTestService(
		forkBatchTestAccount(31, PlatformOpenAI, "gpt-4o"),
		forkBatchTestAccount(32, PlatformOpenAI, "gpt-4o"),
	)
	svc.httpUpstream = &forkBatchSlowUpstream{
		protocolMatrixTestUpstream: upstream,
		delay:                      60 * time.Millisecond,
	}
	c, _ := newTestContext()

	err := svc.RunAccountBatchTest(c, BatchTestRunOptions{
		Concurrency: 1,
		Targets: []BatchTestTarget{
			{AccountID: 31, ModelID: "gpt-4o", Prompt: "hi"},
			{AccountID: 32, ModelID: "gpt-4o", Prompt: "hi"},
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, upstream.maxActive, "并发 1 时同一时刻只有一个账号在测")
}
