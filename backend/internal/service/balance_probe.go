package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// 余额探测适配器登记表。
//
// 每个适配器只描述三件事：该账号是否启用探测、余额端点、返回体里怎么取数。
// 新增供应商时在这里加一条，探测流程（鉴权、出站 URL 校验、快照落盘、错误处理）
// 复用 cn_provider_balance_service.go，不再复制。
type balanceProbeSpec struct {
	// enabled 为 nil 表示平台默认启用；非 nil 时由账号配置决定。
	enabled func(*Account) bool
	url     func(*Account) string
	parse   func(*Account, []byte) ([]CNProviderBalanceEntry, bool, error)
}

var balanceProbeSpecs = map[string]balanceProbeSpec{
	PlatformKimi:     {url: kimiBalanceURL, parse: parseKimiBalance},
	PlatformDeepseek: {url: deepseekBalanceURL, parse: parseDeepSeekBalance},
	// 聚合中转没有统一余额端点：默认按 one-api/new-api 的 /api/user/self 约定探测，
	// 账号可用 balance_probe_* 凭据覆盖端点与 JSON 路径。默认关闭，避免对不支持的站点发请求。
	PlatformAggregateHub: {
		enabled: aggregateHubBalanceProbeEnabled,
		url:     aggregateHubBalanceURL,
		parse:   parseAggregateHubBalance,
	},
}

func lookupBalanceProbe(account *Account) (balanceProbeSpec, bool) {
	if account == nil {
		return balanceProbeSpec{}, false
	}
	spec, ok := balanceProbeSpecs[account.Platform]
	return spec, ok
}

func kimiBalanceURL(*Account) string {
	return "https://api.moonshot.cn/v1/users/me/balance"
}

func deepseekBalanceURL(account *Account) string {
	// Anthropic 协议账号的凭证 base_url 指向 /anthropic 端点，余额探测需回退
	// 到 OpenAI 格式 base（协议感知）再拼接 /user/balance。
	return strings.TrimRight(account.GetOpenAIFormatBaseURL(), "/") + "/user/balance"
}

func parseKimiBalance(_ *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
	// Moonshot：code==0 成功；data.available_balance（number），单币种 CNY。
	balance, _ := cnParseF64(gjson.GetBytes(body, "data.available_balance").Value())
	return []CNProviderBalanceEntry{{Currency: "CNY", Balance: balance}}, true, nil
}

func parseDeepSeekBalance(_ *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
	// is_available 缺省视为 true（健康）；显式存在时取其值。
	available := true
	if v := gjson.GetBytes(body, "is_available"); v.Exists() {
		available = v.Bool()
	}
	// balance_infos 逐条解析：双币种账号同时返回 CNY + USD（数组顺序即
	// 主次序，首条为主币种）。
	balanceInfos := gjson.GetBytes(body, "balance_infos")
	if !balanceInfos.Exists() || !balanceInfos.IsArray() {
		return nil, available, fmt.Errorf("missing balance_infos")
	}
	var entries []CNProviderBalanceEntry
	balanceInfos.ForEach(func(_, info gjson.Result) bool {
		currency := strings.ToUpper(strings.TrimSpace(info.Get("currency").String()))
		totalBalance := info.Get("total_balance")
		balance, ok := cnParseF64(totalBalance.Value())
		if !totalBalance.Exists() || !ok {
			return true
		}
		if currency == "" {
			currency = "CNY"
		}
		entries = append(entries, CNProviderBalanceEntry{Currency: currency, Balance: balance})
		return true
	})
	if len(entries) == 0 {
		return nil, available, fmt.Errorf("no valid balance entries")
	}
	return entries, available, nil
}

// --- 聚合中转 ---

const (
	aggregateHubBalanceProbeEnabledKey = "balance_probe_enabled"
	aggregateHubBalanceURLKey          = "balance_probe_url"
	aggregateHubBalancePathKey         = "balance_probe_path"
	aggregateHubBalanceCurrencyKey     = "balance_probe_currency"
	aggregateHubBalanceDivisorKey      = "balance_probe_divisor"
	aggregateHubBalanceAvailableKey    = "balance_probe_available_path"

	defaultAggregateHubBalanceEndpoint = "/api/user/self"
	defaultAggregateHubBalancePath     = "data.quota"
	defaultAggregateHubBalanceCurrency = "USD"
	defaultAggregateHubBalanceDivisor  = 500000.0 // one-api/new-api：quota 500000 = $1
)

func aggregateHubBalanceProbeEnabled(account *Account) bool {
	enabled, _ := account.Credentials[aggregateHubBalanceProbeEnabledKey].(bool)
	return enabled
}

func aggregateHubBalanceURL(account *Account) string {
	if raw := strings.TrimSpace(account.GetCredential(aggregateHubBalanceURLKey)); raw != "" {
		return raw
	}
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIFormatBaseURL()), "/")
	if base == "" {
		return ""
	}
	return base + defaultAggregateHubBalanceEndpoint
}

func parseAggregateHubBalance(account *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
	path := strings.TrimSpace(account.GetCredential(aggregateHubBalancePathKey))
	if path == "" {
		path = defaultAggregateHubBalancePath
	}
	raw := gjson.GetBytes(body, path)
	if !raw.Exists() {
		return nil, true, fmt.Errorf("balance path %q not found", path)
	}
	value, ok := cnParseF64(raw.Value())
	if !ok {
		return nil, true, fmt.Errorf("balance path %q is not a number", path)
	}
	divisor := defaultAggregateHubBalanceDivisor
	if v, ok := credentialNumber(account, aggregateHubBalanceDivisorKey); ok && v > 0 {
		divisor = v
	}
	currency := strings.ToUpper(strings.TrimSpace(account.GetCredential(aggregateHubBalanceCurrencyKey)))
	if currency == "" {
		currency = defaultAggregateHubBalanceCurrency
	}
	available := true
	if path := strings.TrimSpace(account.GetCredential(aggregateHubBalanceAvailableKey)); path != "" {
		if v := gjson.GetBytes(body, path); v.Exists() {
			available = v.Bool()
		}
	}
	return []CNProviderBalanceEntry{{Currency: currency, Balance: value / divisor}}, available, nil
}

// credentialNumber 读取凭据里的数值（兼容 JSON 反序列化后的 float64 与整数）。
func credentialNumber(account *Account, key string) (float64, bool) {
	switch v := account.Credentials[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	default:
		return 0, false
	}
}
