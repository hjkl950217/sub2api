//go:build unit

// FORK: 模型指纹检测的纯函数测试（二开新增）。
//
// 覆盖三块最容易出错、又不用起服务就能验证的逻辑：
//   - 样本库更新时间点的规范化与 cron 换算（脏值会让定时任务静默不跑）
//   - base_url 规范化（fpd 遇到带完整端点路径的地址会直接报错）
//   - fpd 报告解析与失败原因提取（网络失败时它仍输出合法 JSON，靠 scored_rounds 判定）
package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeFingerprintBankUpdateTime(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"标准 HH:mm", "03:00", "03:00"},
		{"补零前的写法", "3:00", "03:00"},
		{"带秒", "23:45:00", "23:45"},
		{"首尾空格", " 07:30 ", "07:30"},
		{"空串退回默认", "", FingerprintBankUpdateTimeDefault},
		{"只有空格退回默认", "   ", FingerprintBankUpdateTimeDefault},
		{"小时越界退回默认", "25:00", FingerprintBankUpdateTimeDefault},
		{"分钟越界退回默认", "03:99", FingerprintBankUpdateTimeDefault},
		{"乱写退回默认", "凌晨三点", FingerprintBankUpdateTimeDefault},
		{"日期时间退回默认", "2026-10-15 03:00", FingerprintBankUpdateTimeDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeFingerprintBankUpdateTime(tc.input))
		})
	}
}

func TestValidateFingerprintBankUpdateTime(t *testing.T) {
	// 空串表示「恢复默认」，不算错。
	require.NoError(t, ValidateFingerprintBankUpdateTime(""))
	require.NoError(t, ValidateFingerprintBankUpdateTime("   "))
	require.NoError(t, ValidateFingerprintBankUpdateTime("00:00"))
	require.NoError(t, ValidateFingerprintBankUpdateTime("23:59"))
	require.NoError(t, ValidateFingerprintBankUpdateTime("03:00:00"))

	require.Error(t, ValidateFingerprintBankUpdateTime("24:00"))
	require.Error(t, ValidateFingerprintBankUpdateTime("12:60"))
	require.Error(t, ValidateFingerprintBankUpdateTime("中午"))
	require.Error(t, ValidateFingerprintBankUpdateTime("03-00"))
}

func TestFingerprintBankCronSpec(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"03:00", "0 3 * * *"},
		{"00:00", "0 0 * * *"},
		{"23:59", "59 23 * * *"},
		{"07:05", "5 7 * * *"},
		// 脏值走规范化后退回默认 03:00，仍然产出合法表达式。
		{"乱写", "0 3 * * *"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			spec, err := fingerprintBankCronSpec(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, spec)
		})
	}
}

func TestNormalizeFingerprintBaseURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"裸域名", "https://yunshu.best", "https://yunshu.best"},
		{"带尾斜杠", "https://yunshu.best/", "https://yunshu.best"},
		{"保留 /v1", "https://yunshu.best/v1", "https://yunshu.best/v1"},
		{"带 v1 端点", "https://yunshu.best/v1/chat/completions", "https://yunshu.best/v1"},
		{"带 messages 端点", "https://x.com/v1/messages", "https://x.com/v1"},
		{"带 responses 端点", "https://x.com/v1/responses", "https://x.com/v1"},
		{"无 v1 的端点", "https://x.com/chat/completions", "https://x.com"},
		{"端口与路径", "http://192.168.1.5:8080/api/v1", "http://192.168.1.5:8080/api/v1"},
		{"首尾空格", "  https://x.com/v1/  ", "https://x.com/v1"},
		{"空串", "", ""},
		{"只有空格", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeFingerprintBaseURL(tc.input))
		})
	}
}

// fpd 在两种情况下都输出 fpd-detection-v1：真正测出结果，以及上游完全不通。
// 靠 scored_rounds 区分，这里把两种形态都钉住。
func TestParseFingerprintReport(t *testing.T) {
	ok := []byte(`{
		"schema": "fpd-detection-v1",
		"created_at": "2026-10-15T03:10:53.204Z",
		"request": {"model": "claude-opus-5-5"},
		"bank": {"reference_sha256": "abc", "models": 59},
		"requested_rounds": 1,
		"completed_rounds": 1,
		"scored_rounds": 1,
		"analysis": {
			"prediction": "mimo-v2.5",
			"prediction_name": "mimo-v2.5",
			"probability": 0.467,
			"results": [{"model": "mimo-v2.5", "display_name": "mimo-v2.5", "probability": 0.467}],
			"evidence": {"insufficient": true, "label": "Calibrated ranking"}
		}
	}`)

	report, err := parseFingerprintReport(ok)
	require.NoError(t, err)
	require.Equal(t, "fpd-detection-v1", report.Schema)
	require.Equal(t, 1, report.ScoredRounds)
	require.Equal(t, 59, report.Bank.Models)
	require.NotNil(t, report.Analysis)
	require.Equal(t, "mimo-v2.5", report.Analysis.PredictionName)
	// 实测坑：成功检测时 insufficient 也是 true，它表达「证据不足以证明身份」。
	require.True(t, report.Analysis.Evidence.Insufficient)
	require.Len(t, report.Analysis.Results, 1)

	// 前后空白要能容忍：CLI 用 stdout.write 一次性输出，末尾可能带换行。
	padded := append([]byte("\n  "), append(append([]byte{}, ok...), []byte("\n\n")...)...)
	_, err = parseFingerprintReport(padded)
	require.NoError(t, err)

	t.Run("空输出", func(t *testing.T) {
		_, err := parseFingerprintReport(nil)
		require.Error(t, err)
	})
	t.Run("不是 JSON", func(t *testing.T) {
		_, err := parseFingerprintReport([]byte("Error: Cannot read valid JSON"))
		require.Error(t, err)
	})
	t.Run("未知 schema", func(t *testing.T) {
		_, err := parseFingerprintReport([]byte(`{"schema":"something-else"}`))
		require.ErrorContains(t, err, "未知的报告格式")
	})
}

// 失败报告的形状：网络不通时 fpd 照样输出合法 JSON，错误藏在 rounds 里。
func TestFingerprintFailureErrorExtractsReasons(t *testing.T) {
	raw := []byte(`{
		"schema": "fpd-detection-v1",
		"requested_rounds": 1,
		"completed_rounds": 1,
		"scored_rounds": 0,
		"rounds": [{
			"error": "No valid samples. This round cannot be scored.",
			"samples": [{"error": "fetch failed"}, {"error": "fetch failed"}]
		}]
	}`)

	report, err := parseFingerprintReport(raw)
	require.NoError(t, err)
	require.Equal(t, 0, report.ScoredRounds)

	failure := fingerprintFailureError(raw, report, "")
	message := failure.Error()
	require.Contains(t, message, "0 轮可评分")
	require.Contains(t, message, "No valid samples")
	require.Contains(t, message, "fetch failed")
	// 同样的原因只出现一次，不因为三个样本都失败就刷三遍。
	require.Equal(t, 1, strings.Count(message, "fetch failed"))
}

// stderr 为空、rounds 里也没有原因时，至少要给出一句可读的话，不能是空错误。
func TestFingerprintFailureErrorFallsBack(t *testing.T) {
	raw := []byte(`{"schema":"fpd-detection-v1","scored_rounds":0,"rounds":[]}`)
	report, err := parseFingerprintReport(raw)
	require.NoError(t, err)

	failure := fingerprintFailureError(raw, report, "")
	require.NotEmpty(t, failure.Error())
	require.Contains(t, failure.Error(), "上游没有返回可用的样本")
}

func TestFingerprintProcessErrorIncludesStderr(t *testing.T) {
	failure := fingerprintProcessError(assertAnError{}, "Error: boom")
	require.Contains(t, failure.Error(), "boom")
	require.Contains(t, failure.Error(), "lm-detector 执行失败")
}

// assertAnError 提供固定的错误文本，避免测试依赖具体错误类型。
type assertAnError struct{}

func (assertAnError) Error() string { return "exit status 1" }

// 完整性兜底：确保报告结构体与前端 types 的字段名一致（改了字段没同步就红）。
func TestFingerprintReportJSONShape(t *testing.T) {
	report := FingerprintReport{
		Schema:          "fpd-detection-v1",
		CreatedAt:       "2026-10-15T03:10:53.204Z",
		Bank:            FingerprintBankInfo{ReferenceSHA256: "abc", Models: 59},
		RequestedRounds: 1,
		CompletedRounds: 1,
		ScoredRounds:    1,
		Analysis: &FingerprintAnalysis{
			PredictionName: "mimo-v2.5",
			Probability:    0.467,
			Results: []FingerprintCandidate{
				{Model: "mimo-v2.5", DisplayName: "mimo-v2.5", Probability: 0.467},
			},
			Evidence: &FingerprintEvidence{Insufficient: true, Label: "Calibrated ranking"},
		},
	}
	encoded, err := json.Marshal(report)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	for _, key := range []string{
		"schema", "created_at", "request", "bank", "requested_rounds",
		"completed_rounds", "scored_rounds", "analysis",
	} {
		require.Contains(t, decoded, key)
	}
	analysis, ok := decoded["analysis"].(map[string]any)
	require.True(t, ok)
	for _, key := range []string{
		"prediction_name", "probability", "results", "evidence", "probability_status",
		"family_prediction_name", "used_outputs", "diagnostics",
	} {
		require.Contains(t, analysis, key)
	}
}
