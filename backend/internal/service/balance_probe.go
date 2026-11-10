// FORK: 余额探测脚本登记表（二开新增文件）。探测脚本与平台解耦：
// 账号凭据 balance_probe_script 选择脚本，auto 表示首次探测时逐个尝试并记住命中项。
package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// balanceProbeSpec 一个探测脚本：完成取数与解析，返回统一的余额明细。
// 需要多个端点的脚本（New API 的 subscription + usage）自己发请求，共用
// CNProviderBalanceService.doBalanceGET 完成鉴权头、超时与出站 URL 校验。
type balanceProbeSpec struct {
	probe func(ctx context.Context, s *CNProviderBalanceService, account *Account, apiKey string) (*CNProviderBalanceResult, error)
}

const (
	balanceProbeEnabledKey = "balance_probe_enabled"
	balanceProbeScriptKey  = "balance_probe_script"

	balanceProbeURLKey       = "balance_probe_url"
	balanceProbePathKey      = "balance_probe_path"
	balanceProbeCurrencyKey  = "balance_probe_currency"
	balanceProbeDivisorKey   = "balance_probe_divisor"
	balanceProbeAvailableKey = "balance_probe_available_path"
	// FORK-ANCHOR: balance-probe-newapi-cookie (二开：New API/One API 用户余额探测的登录凭据)
	balanceProbeCookieKey = "balance_probe_cookie"
	balanceProbeUserIDKey = "balance_probe_user_id"

	balanceProbeScriptAuto         = "auto"
	balanceProbeScriptKimi         = "kimi"
	balanceProbeScriptDeepSeek     = "deepseek"
	balanceProbeScriptNewAPI       = "newapi"
	balanceProbeScriptNewAPICookie = "newapi_cookie"
	balanceProbeScriptSub2API      = "sub2api"
	balanceProbeScriptCustom       = "custom"

	defaultNewAPIBalancePath     = "data.quota"
	defaultNewAPIBalanceCurrency = "USD"
	defaultNewAPIBalanceDivisor  = 500000.0 // one-api/new-api：quota 500000 = $1
	// new-api 对无限额度 token 返回固定的 hard_limit_usd 哨兵值。
	newAPIUnlimitedHardLimit = 100000000.0
	newAPIUsageStartDate     = "2020-01-01"
)

// balanceProbeScripts 脚本登记表。新增脚本只加一条，探测流程（鉴权、出站校验、
// 快照落盘、错误处理）不复制。
var balanceProbeScripts = map[string]balanceProbeSpec{
	balanceProbeScriptKimi:         {probe: singleGETBalanceProbe(kimiBalanceURL, parseKimiBalance)},
	balanceProbeScriptDeepSeek:     {probe: singleGETBalanceProbe(deepseekBalanceURL, parseDeepSeekBalance)},
	balanceProbeScriptNewAPI:       {probe: fetchNewAPIBalance},
	balanceProbeScriptNewAPICookie: {probe: fetchNewAPICookieBalance},
	balanceProbeScriptSub2API:      {probe: singleGETBalanceProbe(sub2APIBalanceURL, parseSub2APIBalance)},
	balanceProbeScriptCustom:       {probe: singleGETBalanceProbe(customBalanceURL, parseCustomBalance)},
}

// balanceProbeAutoOrder auto 首次探测顺序：New API 最通用，Sub2API 其次，
// DeepSeek 官网最后（前两者对官网只会多两次 404 请求）。
var balanceProbeAutoOrder = []string{balanceProbeScriptNewAPI, balanceProbeScriptSub2API, balanceProbeScriptDeepSeek}

// balanceProbePlatformDefaults 未显式配置脚本时按平台回退，保持既有行为。
// 聚合中转不在此列：没有历史脚本的号按 auto 处理（auto 第一步就是 newapi，
// 与旧行为一致，但站点是 sub2api 时能自动命中）。
var balanceProbePlatformDefaults = map[string]string{
	PlatformKimi:     balanceProbeScriptKimi,
	PlatformDeepseek: balanceProbeScriptDeepSeek,
}

// resolveBalanceProbeScript 返回账号使用的脚本 ID（可能是 auto 或空）。
func resolveBalanceProbeScript(account *Account) string {
	if account == nil {
		return ""
	}
	if raw := strings.ToLower(strings.TrimSpace(account.GetCredential(balanceProbeScriptKey))); raw != "" {
		if raw == balanceProbeScriptAuto {
			return raw
		}
		if _, ok := balanceProbeScripts[raw]; ok {
			return raw
		}
	}
	// 存量聚合中转账号：手填过端点/字段的按自定义脚本处理，否则沿用 one-api 约定。
	if account.Platform == PlatformAggregateHub && hasCustomBalanceProbeConfig(account) {
		return balanceProbeScriptCustom
	}
	if script := balanceProbePlatformDefaults[account.Platform]; script != "" {
		return script
	}
	// 显式开启但没有脚本（老数据/直接改库）：按 auto 处理，首次探测会记住命中项。
	if balanceProbeEnabled(account) {
		return balanceProbeScriptAuto
	}
	return ""
}

// balanceProbeEnabled 余额探测开关：显式布尔值优先；未配置时仅 kimi/deepseek
// 默认开启（保持接入脚本前的既有行为）。
func balanceProbeEnabled(account *Account) bool {
	if account == nil {
		return false
	}
	if v, ok := account.Credentials[balanceProbeEnabledKey].(bool); ok {
		return v
	}
	return account.Platform == PlatformKimi || account.Platform == PlatformDeepseek
}

func hasCustomBalanceProbeConfig(account *Account) bool {
	for _, key := range []string{balanceProbeURLKey, balanceProbePathKey, balanceProbeCurrencyKey, balanceProbeDivisorKey, balanceProbeAvailableKey} {
		if _, ok := account.Credentials[key]; ok {
			return true
		}
	}
	return false
}

// lookupBalanceProbe 解析账号的脚本与脚本实现。auto 返回空 spec（由调用方遍历）。
func lookupBalanceProbe(account *Account) (string, balanceProbeSpec, bool) {
	if account == nil {
		return "", balanceProbeSpec{}, false
	}
	script := resolveBalanceProbeScript(account)
	if script == "" {
		return "", balanceProbeSpec{}, false
	}
	if script == balanceProbeScriptAuto {
		return script, balanceProbeSpec{}, true
	}
	spec, ok := balanceProbeScripts[script]
	return script, spec, ok
}

// singleGETBalanceProbe 单请求脚本：取 URL → 共享请求 → 脚本解析。
func singleGETBalanceProbe(
	urlOf func(*Account) string,
	parse func(*Account, []byte) ([]CNProviderBalanceEntry, bool, error),
) func(context.Context, *CNProviderBalanceService, *Account, string) (*CNProviderBalanceResult, error) {
	return func(ctx context.Context, s *CNProviderBalanceService, account *Account, apiKey string) (*CNProviderBalanceResult, error) {
		return s.runBalanceGET(ctx, account, apiKey, urlOf(account), parse)
	}
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

// --- Sub2API ---

func sub2APIBalanceURL(account *Account) string {
	base := strings.TrimSpace(account.GetOpenAIFormatBaseURL())
	if base == "" {
		return ""
	}
	// Sub2API 的 key 级余额端点：GET /v1/usage → {balance, remaining, unit}。
	return buildOpenAIEndpointURL(base, "/v1/usage")
}

func parseSub2APIBalance(account *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
	raw := gjson.GetBytes(body, "balance")
	if !raw.Exists() {
		// 配额受限的 key 走 quota_limited 模式，余额字段是 remaining。
		raw = gjson.GetBytes(body, "remaining")
	}
	if !raw.Exists() {
		return nil, true, fmt.Errorf("missing balance")
	}
	value, ok := cnParseF64(raw.Value())
	if !ok {
		return nil, true, fmt.Errorf("balance is not a number")
	}
	currency := strings.ToUpper(strings.TrimSpace(account.GetCredential(balanceProbeCurrencyKey)))
	if currency == "" {
		currency = "USD"
	}
	available := true
	if v := gjson.GetBytes(body, "isValid"); v.Exists() {
		available = v.Bool()
	}
	return []CNProviderBalanceEntry{{Currency: currency, Balance: value}}, available, nil
}

// --- New API / One API ---
//
// API key 不能读 /api/user/self（那是用户 access token 的口径）。Key 可用的
// OpenAI 兼容计费接口有两层：subscription 给总额度，usage 给已用额度。
// hard_limit_usd 是无限额度哨兵时直接标记 unlimited。

// newAPIBalanceURL 供测试与调试使用：New API 的总额度端点。
func newAPIBalanceURL(account *Account) string {
	base := strings.TrimSpace(account.GetOpenAIFormatBaseURL())
	if base == "" {
		return ""
	}
	return buildOpenAIEndpointURL(base, "/v1/dashboard/billing/subscription")
}

func fetchNewAPIBalance(ctx context.Context, s *CNProviderBalanceService, account *Account, apiKey string) (*CNProviderBalanceResult, error) {
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIFormatBaseURL()), "/")
	if base == "" {
		return nil, fmt.Errorf("base_url is empty")
	}
	currency := strings.ToUpper(strings.TrimSpace(account.GetCredential(balanceProbeCurrencyKey)))
	if currency == "" {
		currency = defaultNewAPIBalanceCurrency
	}
	result, err := s.runBalanceGET(ctx, account, apiKey, newAPIBalanceURL(account), func(_ *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
		hard, ok := cnParseF64(gjson.GetBytes(body, "hard_limit_usd").Value())
		if !ok {
			return nil, true, fmt.Errorf("missing hard_limit_usd")
		}
		if hard >= newAPIUnlimitedHardLimit {
			return []CNProviderBalanceEntry{{Currency: currency, Unlimited: true}}, true, nil
		}
		return []CNProviderBalanceEntry{{Currency: currency, Balance: hard}}, true, nil
	})
	if err != nil || !result.Success {
		return result, err
	}
	if result.Balances[0].Unlimited {
		return result, nil
	}
	// 剩余 = 总额度 - 已用。usage 的时间范围按全量取（token 口径本身就是累计值）。
	end := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	usageURL := fmt.Sprintf("%s/v1/dashboard/billing/usage?start_date=%s&end_date=%s", base, newAPIUsageStartDate, end)
	usage, err := s.runBalanceGET(ctx, account, apiKey, usageURL, func(_ *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
		used, ok := cnParseF64(gjson.GetBytes(body, "total_usage").Value())
		if !ok {
			return nil, true, fmt.Errorf("missing total_usage")
		}
		// FORK-ANCHOR: balance-probe-usage-cents (二开：OpenAI 计费口径的 total_usage 以美分为单位)
		return []CNProviderBalanceEntry{{Currency: currency, Balance: used / 100}}, true, nil
	})
	if err != nil || !usage.Success {
		return usage, err
	}
	remaining := result.Balance - usage.Balance
	if remaining < 0 {
		remaining = 0
	}
	result.Balance = remaining
	result.Balances = []CNProviderBalanceEntry{{Currency: currency, Balance: remaining}}
	return result, nil
}

// --- New API / One API + Cookie ---
//
// API Key 读不到用户余额（/api/user/self 走用户鉴权），但登录态可以：
// GET /api/user/self + Cookie + New-Api-User → data.quota / data.used_quota。
// 与 newapi 脚本的区别：这里是账户余额，不是令牌额度；受限的 API 令牌
// 会把 subscription 的 hard_limit_usd 写成无限哨兵，账户余额仍可读。
//
// Cookie 随每次探测刷新：响应若带 Set-Cookie 就合并回凭据，4xx 时保留旧值
// 并把失败原因落盘，等下次手动更新 Cookie。

func newAPICookieBalanceURL(account *Account) string {
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIFormatBaseURL()), "/")
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return ""
	}
	return base + "/api/user/self"
}

func fetchNewAPICookieBalance(ctx context.Context, s *CNProviderBalanceService, account *Account, _ string) (*CNProviderBalanceResult, error) {
	targetURL := newAPICookieBalanceURL(account)
	if targetURL == "" {
		return nil, fmt.Errorf("base_url is empty")
	}
	cookie := strings.TrimSpace(account.GetCredential(balanceProbeCookieKey))
	if cookie == "" {
		return nil, fmt.Errorf("missing balance_probe_cookie")
	}
	userID := strings.TrimSpace(account.GetCredential(balanceProbeUserIDKey))
	if userID == "" {
		return nil, fmt.Errorf("missing balance_probe_user_id")
	}
	headers := http.Header{}
	headers.Set("Cookie", cookie)
	headers.Set("New-Api-User", userID)
	status, respHeaders, body, err := s.doBalanceGETWithHeaders(ctx, account, "", targetURL, headers)
	if err != nil {
		return nil, err
	}
	result := &CNProviderBalanceResult{StatusCode: status, Available: true}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		result.Error = fmt.Sprintf("Authentication failed (HTTP %d)", status)
		return result, nil
	}
	if status < 200 || status >= 300 {
		result.Error = fmt.Sprintf("API error (HTTP %d): %s", status, truncate(strings.TrimSpace(string(body)), 240))
		return result, nil
	}
	raw := gjson.GetBytes(body, "data.quota")
	if !raw.Exists() {
		result.Error = `balance path "data.quota" not found`
		return result, nil
	}
	value, ok := cnParseF64(raw.Value())
	if !ok {
		result.Error = `balance path "data.quota" is not a number`
		return result, nil
	}
	divisor := defaultNewAPIBalanceDivisor
	if v, ok := credentialNumber(account, balanceProbeDivisorKey); ok && v > 0 {
		divisor = v
	}
	currency := strings.ToUpper(strings.TrimSpace(account.GetCredential(balanceProbeCurrencyKey)))
	if currency == "" {
		currency = defaultNewAPIBalanceCurrency
	}
	balance := value / divisor
	result.Balance = balance
	result.Currency = currency
	result.Balances = []CNProviderBalanceEntry{{Currency: currency, Balance: balance}}
	result.Success = true

	if fresh := mergeSetCookieHeader(cookie, respHeaders.Values("Set-Cookie")); fresh != cookie {
		s.persistBalanceProbeCookie(ctx, account, fresh)
	}
	return result, nil
}

// mergeSetCookieHeader 把响应 Set-Cookie 的 name=value 合并进现有 Cookie 头；
// 只保留请求头形态，丢弃 Path/Expires 等属性。新值覆盖同名旧值。
func mergeSetCookieHeader(current string, setCookies []string) string {
	pairs := make([]string, 0, len(setCookies)+4)
	index := make(map[string]int, len(setCookies)+4)
	upsert := func(name, value string) {
		if i, ok := index[name]; ok {
			pairs[i] = name + "=" + value
			return
		}
		index[name] = len(pairs)
		pairs = append(pairs, name+"="+value)
	}
	for _, pair := range strings.Split(current, ";") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) != "" {
			upsert(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}
	for _, raw := range setCookies {
		first := strings.SplitN(strings.TrimSpace(raw), ";", 2)[0]
		kv := strings.SplitN(first, "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) != "" {
			upsert(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}
	return strings.Join(pairs, "; ")
}

// --- 自定义（沿用聚合中转的手填端点/JSON 路径） ---

func customBalanceURL(account *Account) string {
	if raw := strings.TrimSpace(account.GetCredential(balanceProbeURLKey)); raw != "" {
		return raw
	}
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIFormatBaseURL()), "/")
	// one-api/new-api 的 OpenAI 端点通常带 /v1，余额接口在站点根路径上。
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return ""
	}
	return base + "/api/user/self"
}

func parseCustomBalance(account *Account, body []byte) ([]CNProviderBalanceEntry, bool, error) {
	path := strings.TrimSpace(account.GetCredential(balanceProbePathKey))
	if path == "" {
		path = defaultNewAPIBalancePath
	}
	raw := gjson.GetBytes(body, path)
	if !raw.Exists() {
		return nil, true, fmt.Errorf("balance path %q not found", path)
	}
	value, ok := cnParseF64(raw.Value())
	if !ok {
		return nil, true, fmt.Errorf("balance path %q is not a number", path)
	}
	divisor := defaultNewAPIBalanceDivisor
	if v, ok := credentialNumber(account, balanceProbeDivisorKey); ok && v > 0 {
		divisor = v
	}
	currency := strings.ToUpper(strings.TrimSpace(account.GetCredential(balanceProbeCurrencyKey)))
	if currency == "" {
		currency = defaultNewAPIBalanceCurrency
	}
	available := true
	if availablePath := strings.TrimSpace(account.GetCredential(balanceProbeAvailableKey)); availablePath != "" {
		if v := gjson.GetBytes(body, availablePath); v.Exists() {
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
