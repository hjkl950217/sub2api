// FORK: 模型指纹检测（二开新增文件，不改上游）。
//
// 本文件放「样本库更新时间点」的解析与换算，供设置解析、保存校验、定时任务三处共用。
//
// 三个入口的分工：
//   - normalizeFingerprintBankUpdateTime：读取路径用。脏数据不报错，退回默认值，
//     保证设置页永远能打开。
//   - ValidateFingerprintBankUpdateTime：保存路径用。非法输入报错给管理员看，
//     而不是静默存一个默认值。
//   - fingerprintBankCronSpec：定时任务用。HH:mm → cron 五段表达式。
//
// 检测本身（exec CLI、解析报告）在 fingerprint_service.go；
// 样本库定时更新在 fingerprint_bank_service.go。
package service

import (
	"fmt"
	"strings"
	"time"
)

// FingerprintBankUpdateTimeDefault 是样本库每日自动更新的默认时间点。
// 与 setting_parse.go 默认值 map 里的字面量必须一致。
const FingerprintBankUpdateTimeDefault = "03:00"

// parseFingerprintBankUpdateTime 解析 HH:mm 或 HH:mm:ss。
// 用 time.Parse 做真解析，避免 "25:99" 这类边界值混进来 —— 定时任务靠这个
// 结果重建 cron，脏值会让任务静默不跑。
func parseFingerprintBankUpdateTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	layout := "15:04"
	if len(trimmed) == len("15:04:05") {
		layout = "15:04:05"
	}
	parsed, err := time.Parse(layout, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("时间点必须使用 HH:mm 格式，收到 %q", value)
	}
	return parsed, nil
}

// normalizeFingerprintBankUpdateTime 把用户输入规范成 HH:mm；空值或无法解析时
// 返回默认值。读取路径专用。
func normalizeFingerprintBankUpdateTime(value string) string {
	if strings.TrimSpace(value) == "" {
		return FingerprintBankUpdateTimeDefault
	}
	parsed, err := parseFingerprintBankUpdateTime(value)
	if err != nil {
		return FingerprintBankUpdateTimeDefault
	}
	return parsed.Format("15:04")
}

// ValidateFingerprintBankUpdateTime 校验保存路径收到的值。
// 空串表示「恢复默认」，不算错；非空但格式不对才算错。
func ValidateFingerprintBankUpdateTime(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	_, err := parseFingerprintBankUpdateTime(value)
	return err
}

// fingerprintBankCronSpec 把 HH:mm 换算成每天执行一次的 cron 五段表达式。
// 入参先过一遍 normalize，所以返回值一定合法，调用方的 AddFunc 不会因格式失败。
func fingerprintBankCronSpec(hhmm string) (string, error) {
	parsed, err := parseFingerprintBankUpdateTime(normalizeFingerprintBankUpdateTime(hhmm))
	if err != nil {
		return "", err
	}
	// cron 五段：分 时 日 月 周。每天在指定时刻跑一次。
	return fmt.Sprintf("%d %d * * *", parsed.Minute(), parsed.Hour()), nil
}
