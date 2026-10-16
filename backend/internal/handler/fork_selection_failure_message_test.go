// FORK: 二开用例——选号失败的响应文案要带上选号器给的原因，不放 `-` 或一句无法区分的兜底话。
// 与上游 no_account_error 相关测试分开放，减少合并冲突。
//go:build unit

package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// 二开：选号失败的文案要带上选号器给出的原因，且不能破坏已经判定的分类文案。
// 起因：使用记录→错误请求里 80% 的行账号列为空，兜底文案
// "Service temporarily unavailable" 与上游真实错误文案只差一个词，被读成上游错误。
func TestForkSelectionFailureMessage(t *testing.T) {
	fallback := noAccountErrorClassification{
		Status:  http.StatusServiceUnavailable,
		ErrType: "api_error",
		Message: "Service temporarily unavailable",
	}
	modelNotFound := noAccountErrorClassification{
		Status:        http.StatusNotFound,
		ErrType:       "model_not_found",
		Message:       `Model "deepseek-flash" is not supported by any configured account in this group`,
		ModelNotFound: true,
	}
	rateLimited := noAccountErrorClassification{
		Status:  http.StatusTooManyRequests,
		ErrType: "rate_limit_error",
		Message: "All available accounts are currently rate-limited. Please retry later.",
	}

	t.Run("选号器给了原因就接在文案后面", func(t *testing.T) {
		err := errors.New("no available OpenAI accounts supporting model: glm-5.3-flash (pool=21, filtered: temp_unschedulable=1)")
		got := selectionFailureMessage(fallback, err)
		require.Contains(t, got, "Service temporarily unavailable")
		require.Contains(t, got, "glm-5.3-flash")
		require.Contains(t, got, "temp_unschedulable=1")
	})

	t.Run("已判定的限流分类文案保留，原因追加在后面", func(t *testing.T) {
		err := errors.New("no available accounts supporting model: deepseek-flash (model_rate_limited=3)")
		got := selectionFailureMessage(rateLimited, err)
		require.Contains(t, got, "All available accounts are currently rate-limited. Please retry later.")
		require.Contains(t, got, "model_rate_limited=3")
	})

	t.Run("模型无账号的 404 不加原因", func(t *testing.T) {
		err := errors.New("no available accounts supporting model: deepseek-flash (total=21)")
		got := selectionFailureMessage(modelNotFound, err)
		require.Equal(t, modelNotFound.Message, got)
	})

	t.Run("没有错误原样返回", func(t *testing.T) {
		require.Equal(t, fallback.Message, selectionFailureMessage(fallback, nil))
	})

	t.Run("原因已在文案里不重复追加", func(t *testing.T) {
		err := errors.New("no available accounts")
		cls := noAccountErrorClassification{Message: "no available accounts (from selector)", ErrType: "api_error"}
		require.Equal(t, cls.Message, selectionFailureMessage(cls, err))
	})

	t.Run("分类文案为空时只回原因", func(t *testing.T) {
		err := errors.New("no available accounts")
		require.Equal(t, "no available accounts", selectionFailureMessage(noAccountErrorClassification{}, err))
	})
}
