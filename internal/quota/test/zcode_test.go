package test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/quota"
)

// zcodeSummaryStub 模拟 cpa.Client 的 summary 读取行为。
type zcodeSummaryStub struct {
	raw json.RawMessage
	err error
}

func (s zcodeSummaryStub) FetchZcodeSummary(context.Context) (json.RawMessage, error) {
	return s.raw, s.err
}

// zcodeSummaryCallerStub 只实现 zcode summary 读取，其余管理接口保持不可用。
type zcodeSummaryCallerStub struct {
	zcodeSummaryStub
}

func (s zcodeSummaryCallerStub) CallManagementAPI(context.Context, apicall.Request) (*apicall.Response, error) {
	return nil, errors.New("not implemented")
}

// 插件被改名后端点会 404。ruo 错误被原样透传，前端只会看到通用文案
// "Quota refresh failed. Please try again later."，所以这里必须提升成
// ProviderHTTPError，把真实状态码带到任务记录里。
func TestZcodeProviderSurfacesHTTPStatusFromEndpointError(t *testing.T) {
	provider := quota.NewZcodeProvider(zcodeSummaryCallerStub{
		zcodeSummaryStub: zcodeSummaryStub{
			err: errors.New("management zcode summary request returned status 404"),
		},
	})

	_, err := provider.Check(context.Background(), quota.ProviderInput{})
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	var httpErr quota.ProviderHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected ProviderHTTPError so the UI shows the real status, got %T: %v", err, err)
	}
	if httpErr.StatusCode != 404 {
		t.Fatalf("expected status 404, got %d", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Error(), "插件") {
		t.Fatalf("expected an actionable 404 hint, got %q", httpErr.Error())
	}
	// 前端读取的最终展示文案必须是真实的 HTTP 状态，而不是通用兜底。
	if !strings.HasPrefix(httpErr.Error(), "HTTP 404") {
		t.Fatalf("expected message to start with \"HTTP 404\", got %q", httpErr.Error())
	}
}

// 无法解析状态码的传输层错误不应被伪装成 HTTP 错误。
func TestZcodeProviderKeepsNonHTTPErrorsIntact(t *testing.T) {
	provider := quota.NewZcodeProvider(zcodeSummaryCallerStub{
		zcodeSummaryStub: zcodeSummaryStub{
			err: errors.New("request management zcode summary: dial tcp: connection refused"),
		},
	})

	_, err := provider.Check(context.Background(), quota.ProviderInput{})
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	var httpErr quota.ProviderHTTPError
	if errors.As(err, &httpErr) {
		t.Fatalf("expected a plain error, got ProviderHTTPError %v", httpErr)
	}
}
