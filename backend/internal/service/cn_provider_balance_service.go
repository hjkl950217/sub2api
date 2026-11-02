package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// 国产供应商 payg（按量付费）账号余额探测服务。
//
// 仅覆盖有公开余额端点的供应商：
//   - Kimi/Moonshot：GET https://api.moonshot.cn/v1/users/me/balance (Bearer) → data.available_balance
//   - DeepSeek：     GET https://api.deepseek.com/user/balance (Bearer) → balance_infos[].total_balance + is_available
//
// 智谱（zhipu）无公开余额端点（OpenAPI 规格验证），仅靠响应式 429/402（见
// ratelimit_cn_providers.go）。解析逻辑对齐 cc-switch services/balance.rs::query_deepseek。
const (
	cnBalanceUpstreamTimeout = 15 * time.Second
	cnBalanceMaxBodyBytes    = 256 * 1024

	// Extra 余额快照键后缀（加 provider 前缀）。
	cnBalanceExtraSuffixBalance   = "balance"
	cnBalanceExtraSuffixCurrency  = "balance_currency"
	cnBalanceExtraSuffixAvailable = "balance_available" // deepseek is_available 健康标记
	cnBalanceExtraSuffixUpdated   = "balance_updated_at"
	cnBalanceExtraSuffixBalances  = "balances"          // 多币种明细（deepseek USD+CNY）
	cnBalanceExtraSuffixUnlimited = "balance_unlimited" // 脚本明确返回「不限额度」（new-api 无限 token）
	cnBalanceExtraSuffixError     = "balance_error"     // 最近一次探测失败原因（页面直接展示）
	cnBalanceExtraSuffixErrorAt   = "balance_error_at"  // 失败时间（RFC3339）
)

// CNProviderBalanceEntry 是单一币种的余额明细。
type CNProviderBalanceEntry struct {
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	// Unlimited 表示上游明确返回「不限额度」而非具体余额（new-api 无限 token）。
	Unlimited bool `json:"unlimited,omitempty"`
}

// CNProviderBalanceResult 是余额探测的返回结构（管理端 + UI 消费）。
type CNProviderBalanceResult struct {
	Provider string `json:"provider"`
	Success  bool   `json:"success"`
	// Balance/Currency 为主币种（balance_infos 首条，兼容单币种消费方）；
	// 完整明细见 Balances（deepseek 双币种账号含 CNY + USD 两条）。
	Balance    float64                  `json:"balance"`
	Currency   string                   `json:"currency,omitempty"`
	Balances   []CNProviderBalanceEntry `json:"balances,omitempty"`
	Available  bool                     `json:"available"` // 健康标记（deepseek is_available；kimi 无此概念恒 true）
	StatusCode int                      `json:"status_code,omitempty"`
	FetchedAt  int64                    `json:"fetched_at"`
	Persisted  bool                     `json:"persisted"`
	Error      string                   `json:"error,omitempty"`
}

// CNProviderBalanceService 探测 Kimi / DeepSeek payg 账号的账户余额。
type CNProviderBalanceService struct {
	accountRepo  AccountRepository
	proxyRepo    ProxyRepository
	httpUpstream HTTPUpstream
	cfg          *config.Config
	flight       singleflight.Group
}

// NewCNProviderBalanceService 构造余额探测服务。
func NewCNProviderBalanceService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *CNProviderBalanceService {
	return &CNProviderBalanceService{
		accountRepo:  accountRepo,
		proxyRepo:    proxyRepo,
		httpUpstream: httpUpstream,
		cfg:          cfg,
	}
}

// QueryBalance 探测指定 payg 账号的余额并落 Extra 快照。
func (s *CNProviderBalanceService) QueryBalance(ctx context.Context, accountID int64) (*CNProviderBalanceResult, error) {
	account, err := s.loadPayGAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.QueryBalanceForAccount(ctx, account)
}

// QueryBalanceForAccount 探测已加载账号（配额监控 fetcher / 周期余额检测复用，
// 避免二次 GetByID）。singleflight key 与 QueryBalance 相同，按账号 ID 合并。
func (s *CNProviderBalanceService) QueryBalanceForAccount(ctx context.Context, account *Account) (*CNProviderBalanceResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_BALANCE_NOT_CONFIGURED", "cn provider balance service is not configured")
	}
	if err := validatePayGAccount(account); err != nil {
		return nil, err
	}
	key := "cn_balance:" + strconv.FormatInt(account.ID, 10)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		probeCtx, cancel := context.WithTimeout(context.Background(), cnBalanceUpstreamTimeout+5*time.Second)
		defer cancel()
		return s.queryBalanceForAccount(probeCtx, account)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		result, ok := flightResult.Val.(*CNProviderBalanceResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_BALANCE_PROBE_RESULT_INVALID", "invalid cn provider balance probe result")
		}
		cloned := *result
		return &cloned, nil
	}
}

func (s *CNProviderBalanceService) queryBalanceForAccount(ctx context.Context, account *Account) (*CNProviderBalanceResult, error) {
	if account.IsCommandCode() {
		return s.queryCommandCodeBalance(ctx, account)
	}
	if account.IsCline() {
		return s.queryClineBalance(ctx, account)
	}
	// FORK-ANCHOR: balance-probe-registry (二开：余额端点与解析改由 balance_probe.go 的脚本登记表提供)
	script, spec, ok := lookupBalanceProbe(account)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_ENDPOINT", "account provider has no balance endpoint")
	}

	apiKey := balanceProbeAPIKey(account)
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_APIKEY", "account api_key is empty")
	}

	if script == balanceProbeScriptAuto {
		return s.probeAutoScript(ctx, account, apiKey)
	}
	return s.probeScript(ctx, account, apiKey, spec)
}

// probeAutoScript 按固定顺序逐个尝试脚本，命中后把脚本写回凭据，后续探测直接使用。
// 全部失败时返回最后一个结果（保留错误信息给 UI）。
func (s *CNProviderBalanceService) probeAutoScript(ctx context.Context, account *Account, apiKey string) (*CNProviderBalanceResult, error) {
	var lastResult *CNProviderBalanceResult
	var lastErr error
	for _, script := range balanceProbeAutoOrder {
		spec, ok := balanceProbeScripts[script]
		if !ok {
			continue
		}
		result, err := s.probeScript(ctx, account, apiKey, spec)
		if err == nil && result != nil && result.Success {
			s.rememberBalanceProbeScript(ctx, account, script)
			return result, nil
		}
		if result != nil {
			lastResult = result
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastResult != nil {
		return lastResult, nil
	}
	return nil, lastErr
}

// rememberBalanceProbeScript 把 auto 命中的脚本写回账号凭据。
func (s *CNProviderBalanceService) rememberBalanceProbeScript(ctx context.Context, account *Account, script string) {
	credentials := make(map[string]any, len(account.Credentials)+1)
	for key, value := range account.Credentials {
		credentials[key] = value
	}
	credentials[balanceProbeScriptKey] = script
	if err := persistAccountCredentials(ctx, s.accountRepo, account, credentials); err != nil {
		slog.Warn("balance_probe_script_persist_failed", "account_id", account.ID, "script", script, "error", err)
	}
}

// probeScript 执行一个脚本并落 Extra 快照。
func (s *CNProviderBalanceService) probeScript(ctx context.Context, account *Account, apiKey string, spec balanceProbeSpec) (*CNProviderBalanceResult, error) {
	result, err := spec.probe(ctx, s, account, apiKey)
	now := time.Now().UTC()
	if err != nil {
		// 出站校验/网络失败同样落盘，页面才能看到「取不到余额」的原因。
		s.persistBalanceError(ctx, account, err.Error(), now)
		return result, err
	}
	if result == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_BALANCE_PROBE_RESULT_INVALID", "empty balance probe result")
	}
	result.Provider = account.Platform
	result.FetchedAt = now.Unix()
	if !result.Success {
		// 失败也要落盘：页面需要显示「确实取不到余额」以及原因，而不是空白。
		result.Persisted = s.persistBalanceError(ctx, account, result.Error, now)
		return result, nil
	}
	updates := cnBalanceExtraUpdates(account.Platform, result, now)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_balance_persist_failed", "account_id", account.ID, "provider", account.Platform, "error", err)
	} else {
		result.Persisted = true
	}
	return result, nil
}

// persistBalanceError 把最近一次探测失败原因写入 Extra（页面直接展示）。
func (s *CNProviderBalanceService) persistBalanceError(ctx context.Context, account *Account, reason string, now time.Time) bool {
	updates := map[string]any{
		cnExtraKey(account.Platform, cnBalanceExtraSuffixError):   reason,
		cnExtraKey(account.Platform, cnBalanceExtraSuffixErrorAt): now.Format(time.RFC3339),
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_balance_persist_failed", "account_id", account.ID, "provider", account.Platform, "error", err)
		return false
	}
	return true
}

// balanceProbeAPIKey 余额探测使用的 key：先 CN 口径，再 OpenAI 口径，
// 最后对任意 apikey 账号直接读 api_key（脚本与平台解耦后需要）。
func balanceProbeAPIKey(account *Account) string {
	if key := strings.TrimSpace(account.GetCNAPIKey()); key != "" {
		return key
	}
	if key := strings.TrimSpace(account.GetOpenAIProtocolAPIKey()); key != "" {
		return key
	}
	if account.Type == AccountTypeAPIKey {
		return strings.TrimSpace(account.GetCredential("api_key"))
	}
	return ""
}

// doBalanceGET 共享探测请求：出站 URL 校验、鉴权头、超时、代理与 header 覆写。
func (s *CNProviderBalanceService) doBalanceGET(ctx context.Context, account *Account, apiKey, targetURL string) (int, []byte, error) {
	// 探测发起前过出站 URL 安全策略（与网关转发/Grok 探测同一套校验）：
	// 端点由账号 base_url 衍生，不得把 API key 发往策略外主机。
	validatedURL, err := cnValidateProbeURL(s.cfg, targetURL)
	if err != nil {
		return 0, nil, infraerrors.New(http.StatusForbidden, "CN_BALANCE_URL_REJECTED", err.Error())
	}
	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, cnBalanceUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, validatedURL, nil)
	if err != nil {
		return 0, nil, infraerrors.Newf(http.StatusInternalServerError, "CN_BALANCE_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return 0, nil, infraerrors.Newf(http.StatusBadGateway, "CN_BALANCE_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, cnBalanceMaxBodyBytes))
	return resp.StatusCode, bodyBytes, nil
}

// runBalanceGET 单端点脚本的公共流程：请求 → 状态判定 → 解析。
func (s *CNProviderBalanceService) runBalanceGET(
	ctx context.Context,
	account *Account,
	apiKey, targetURL string,
	parse func(*Account, []byte) ([]CNProviderBalanceEntry, bool, error),
) (*CNProviderBalanceResult, error) {
	status, body, err := s.doBalanceGET(ctx, account, apiKey, targetURL)
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
	entries, available, parseErr := parse(account, body)
	result.Available = available
	if parseErr != nil {
		result.Error = "Invalid balance response: " + parseErr.Error()
		return result, nil
	}
	result.Balances = entries
	result.Balance = entries[0].Balance
	result.Currency = entries[0].Currency
	result.Success = true
	return result, nil
}

// cnBalanceExtraUpdates 构造 provider 维度的余额快照更新。
func cnBalanceExtraUpdates(provider string, result *CNProviderBalanceResult, now time.Time) map[string]any {
	balanceUpdates := make([]any, 0, len(result.Balances))
	unlimited := false
	for _, entry := range result.Balances {
		balanceUpdates = append(balanceUpdates, map[string]any{
			"currency":  entry.Currency,
			"balance":   entry.Balance,
			"unlimited": entry.Unlimited,
		})
		if entry.Unlimited {
			unlimited = true
		}
	}
	return map[string]any{
		cnExtraKey(provider, cnBalanceExtraSuffixBalance):   result.Balance,
		cnExtraKey(provider, cnBalanceExtraSuffixCurrency):  result.Currency,
		cnExtraKey(provider, cnBalanceExtraSuffixAvailable): result.Available,
		cnExtraKey(provider, cnBalanceExtraSuffixUpdated):   now.Format(time.RFC3339),
		cnExtraKey(provider, cnBalanceExtraSuffixBalances):  balanceUpdates,
		cnExtraKey(provider, cnBalanceExtraSuffixUnlimited): unlimited,
		// 成功即清除失败原因。
		cnExtraKey(provider, cnBalanceExtraSuffixError):   "",
		cnExtraKey(provider, cnBalanceExtraSuffixErrorAt): "",
		// 余额探测成功即清除响应式 402/429 写下的 balance_low 标记。
		cnExtraKey(provider, cnBalanceExtraSuffixLow): false,
	}
}

// loadPayGAccount 加载 payg 模式的国产供应商账号（余额仅对 payg 有意义；coding 走额度）。
func (s *CNProviderBalanceService) loadPayGAccount(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "CN_BALANCE_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if err := validatePayGAccount(account); err != nil {
		return nil, err
	}
	return account, nil
}

// validatePayGAccount 加载后的非 DB 校验（ForAccount 入口同样复用，
// 保证直传 account 也不绕过平台/模式检查）。
func validatePayGAccount(account *Account) error {
	if account == nil {
		return infraerrors.New(http.StatusNotFound, "CN_BALANCE_ACCOUNT_NOT_FOUND", "account not found")
	}
	// Cline 积分余额只查官方主机。
	if account.IsCline() {
		if !account.clineAccountAPISupported() {
			return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_ENDPOINT", "cline balance is only available for api key accounts on the official host")
		}
		return nil
	}
	// Command Code 积分余额与套餐窗口同源，只查官方主机。
	if account.IsCommandCode() {
		if !account.commandCodeUsageSupported() {
			return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_NO_ENDPOINT", "command code balance is only available for api key accounts on the official host")
		}
		return nil
	}
	// FORK-ANCHOR: balance-probe-platform-gate (二开：余额脚本与平台解耦，按账号开关探测)
	if _, _, ok := lookupBalanceProbe(account); !ok {
		return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_INVALID_PLATFORM", "account has no balance probe script")
	}
	if !balanceProbeEnabled(account) {
		return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_DISABLED", "balance probe is disabled for this account")
	}
	// coding 账号走额度探测，余额端点不适用。DeepSeek 没有 coding 套餐，
	// 存量账号上残留的 coding 标记不应挡住余额查询。
	if account.IsCodingPlan() && account.Platform != PlatformDeepseek {
		return infraerrors.New(http.StatusBadRequest, "CN_BALANCE_CODING_PLAN", "coding plan account has no balance endpoint; use quota probe")
	}
	return nil
}

func (s *CNProviderBalanceService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s != nil && s.proxyRepo != nil {
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			account.Proxy = proxy
			return proxy.URL()
		}
	}
	return ""
}
