//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardAsAnthropic_TransportError_ReturnsFailoverError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{
		err: errors.New(`dial tcp 1.2.3.4:443: connect: connection refused`),
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "transport error should return UpstreamFailoverError for handler failover, got: %T", err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
}

func TestForwardAsAnthropic_AnthropicOnlyAccount_MarksMessagesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"claude-opus-5-5","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: &httpUpstreamRecorder{err: errors.New("connection refused")},
	}
	// 只勾选 anthropic 的聚合中转账号：入站 /v1/messages 必须记成原生端点，
	// 不能因为未勾选 responses 就被旧逻辑提前标成 /v1/chat/completions。
	account := rawChatCompletionsTestAccount()
	account.Platform = PlatformAggregateHub
	account.Credentials["api_protocols"] = []any{APIProtocolAnthropic}
	account.Credentials["fallback_protocol"] = APIProtocolAnthropic

	_, _ = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Equal(t, "/v1/messages", GetActualOpenAIUpstreamEndpoint(c))
}

func TestForwardAsAnthropic_TransportError_DoesNotWriteResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{
		err: errors.New(`read tcp: connection reset by peer`),
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, _ = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Equal(t, http.StatusOK, rec.Code, "transport error must not write HTTP response — handler owns the response for failover")
	require.Empty(t, rec.Body.String(), "response body must be empty so handler can write the correct error or failover")
}

func TestForwardAsAnthropic_TransportError_ClientCanceled_NoFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)).WithContext(cancelCtx)
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{
		err: context.Canceled,
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "client-canceled transport error should NOT trigger failover")
}
