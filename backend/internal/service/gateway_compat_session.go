package service

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// applyCompatAnthropicSessionID 为 Anthropic API Key / 自定义 endpoint 的
// Chat Completions、Responses 兼容路径补齐 /v1/messages 所需的会话信号。
// 客户端已显式提供 X-Claude-Code-Session-Id 或 metadata.user_id 时原样保留。
func applyCompatAnthropicSessionID(body []byte, account *Account, c *gin.Context, promptCacheKey string) ([]byte, string) {
	if account == nil || account.Platform != PlatformAnthropic || account.IsOAuth() {
		return body, ""
	}
	if c != nil && c.Request != nil && strings.TrimSpace(c.Request.Header.Get("X-Claude-Code-Session-Id")) != "" {
		return body, ""
	}
	if strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String()) != "" {
		return body, ""
	}

	apiKeyID := int64(0)
	if c != nil {
		apiKeyID = getAPIKeyIDFromContext(c)
	}
	discriminator := strconv.FormatInt(apiKeyID, 10)
	if key := strings.TrimSpace(promptCacheKey); key != "" {
		discriminator += "\x00" + key
	}
	sessionID := generateSessionUUID(buildStableSessionSeed(account.ID, discriminator, extractFirstUserText(body)))
	updated, err := sjson.SetBytes(body, "metadata.user_id", FormatMetadataUserID("sub2api", "", sessionID, ""))
	if err != nil {
		return body, ""
	}
	return updated, sessionID
}
