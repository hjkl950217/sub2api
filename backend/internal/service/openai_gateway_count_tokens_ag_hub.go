// FORK: ag_hub count_tokens 先试上游，404/501 回落本地估算（二开新增文件）
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// forwardAnthropicCountTokensWithFallback 为 ag_hub 平台实现「先试上游 /v1/messages/count_tokens，
// 404/501 回落本地 tiktoken 估算」。上游支持此端点的中转站（如云枢、artbloom）能拿到精确计数；
// 不支持的站会回 404/501，此时用本地估算兜底，不影响 Claude Code 调用。
//
// 设计理由：
//  1. 中转站能力不统一：云枢/artbloom 等提供 count_tokens，但琦玉/anyrouter 可能不提供；
//  2. Claude Code 高频调用此端点，404 常态化会污染日志；
//  3. 本地估算是既有能力（CN 供应商与 Grok 已用），回落无风险。
//
// 回落条件：仅 404（端点不存在）与 501（未实现）触发，其他错误（401/403/429/500/502/503/504）
// 不回落——那些是鉴权、限流或服务故障，回落会掩盖真实问题。
func (s *OpenAIGatewayService) forwardAnthropicCountTokensWithFallback(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) error {
	if account == nil || account.Platform != PlatformAggregateHub {
		// 防御：不是 ag_hub 账号不该走这条路径
		writeAnthropicCountTokensError(c, http.StatusServiceUnavailable, "api_error", "Invalid account")
		return fmt.Errorf("forwardAnthropicCountTokensWithFallback: not ag_hub account")
	}

	prepared, err := prepareOpenAIInputTokensCountRequest(body, account, defaultMappedModel)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return err
	}

	upstreamBody, err := marshalOpenAIUpstreamJSON(prepared.Request)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return fmt.Errorf("marshal openai input_tokens body: %w", err)
	}

	logger.L().Debug("ag_hub count_tokens: trying upstream first",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", prepared.OriginalModel),
		zap.String("upstream_model", prepared.UpstreamModel),
	)

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		// token 获取失败直接报错，不回落（鉴权问题）
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return fmt.Errorf("get access token: %w", err)
	}

	upstreamReq, err := s.buildInputTokensUpstreamRequest(ctx, c, account, upstreamBody, token)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return fmt.Errorf("build input_tokens request: %w", err)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	if err != nil {
		// 网络错误不回落（可能是临时故障）
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return fmt.Errorf("ag_hub count_tokens upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		return fmt.Errorf("read input_tokens response: %w", err)
	}

	// 404/501 回落本地估算
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNotImplemented {
		logger.L().Info("ag_hub count_tokens: upstream not supported, falling back to local estimate",
			zap.Int64("account_id", account.ID),
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_preview", truncate(string(respBody), 200)),
		)
		estimated, err := estimateAnthropicCountTokensLocally(body)
		if err != nil {
			writeAnthropicCountTokensError(c, http.StatusBadRequest, "invalid_request_error", "Failed to estimate locally")
			return fmt.Errorf("ag_hub count_tokens: fallback estimate failed: %w", err)
		}
		logger.L().Debug("ag_hub count_tokens: local estimate used",
			zap.Int64("account_id", account.ID),
			zap.Int("estimated_input_tokens", estimated),
		)
		c.JSON(http.StatusOK, gin.H{
			"input_tokens": estimated,
		})
		return nil
	}

	// 其他非 2xx 错误不回落，透传给客户端
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		upstreamMsg, errMsg := extractUpstreamErrorMessage(respBody), extractUpstreamErrorMessage(respBody)
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, "")
		writeAnthropicCountTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("ag_hub count_tokens upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("ag_hub count_tokens upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	// 成功：解析 input_tokens 并返回
	inputTokensResult := gjson.GetBytes(respBody, "input_tokens")
	if !inputTokensResult.Exists() {
		// 响应格式不对，回落本地估算（宽容处理）
		logger.L().Warn("ag_hub count_tokens: upstream success but missing input_tokens, falling back",
			zap.Int64("account_id", account.ID),
			zap.String("response_preview", truncate(string(respBody), 200)),
		)
		estimated, err := estimateAnthropicCountTokensLocally(body)
		if err != nil {
			writeAnthropicCountTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response invalid")
			return fmt.Errorf("ag_hub count_tokens: response missing input_tokens, fallback estimate failed: %w", err)
		}
		c.JSON(http.StatusOK, gin.H{
			"input_tokens": estimated,
		})
		return nil
	}

	logger.L().Debug("ag_hub count_tokens: upstream success",
		zap.Int64("account_id", account.ID),
		zap.Int64("input_tokens", inputTokensResult.Int()),
	)
	c.JSON(http.StatusOK, gin.H{
		"input_tokens": int(inputTokensResult.Int()),
	})
	return nil
}
