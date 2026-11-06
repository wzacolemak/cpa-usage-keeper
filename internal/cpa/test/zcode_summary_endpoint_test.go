package cpa_test

import (
	"context"
	. "cpa-usage-keeper/internal/cpa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 插件 ID 被运维改名（zcode -> zcode-oauth）后，keeper 必须自己发现新端点，
// 而不是继续打旧路径拿到 404，否则前端只会看到 "Quota refresh failed"。
func TestFetchZcodeSummaryDiscoversRenamedPluginID(t *testing.T) {
	var oldPathHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/plugins":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plugins":[
				{"id":"dsml-fixer","enabled":false},
				{"id":"zcode-oauth","registered":true,"enabled":true,"effective_enabled":true,"oauth_provider":"zcode-oauth"}
			]}`))
		case "/v0/resource/plugins/zcode-oauth/summary":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"logged_in":true,"account":"pmmmtox5","level":"MAX","windows":[]}`))
		case "/v0/resource/plugins/zcode/summary":
			// 旧 ID 端点不应该被优先请求；即使被请求也必须失败。
			oldPathHits.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "management-secret", 2*time.Second, false)
	raw, err := client.FetchZcodeSummary(context.Background())
	if err != nil {
		t.Fatalf("expected summary from renamed plugin, got error: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected non-empty summary payload")
	}
	if got := oldPathHits.Load(); got != 0 {
		t.Fatalf("expected discovery to avoid the stale plugin id, but it was requested %d times", got)
	}
}

// 发现结果必须被缓存：连续刷新不能每次都回退到插件列表接口。
func TestFetchZcodeSummaryCachesDiscoveredEndpoint(t *testing.T) {
	var pluginListHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/plugins":
			pluginListHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plugins":[{"id":"zcode-oauth","enabled":true}]}`))
		case "/v0/resource/plugins/zcode-oauth/summary":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"logged_in":true,"windows":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "management-secret", 2*time.Second, false)
	for i := 0; i < 3; i++ {
		if _, err := client.FetchZcodeSummary(context.Background()); err != nil {
			t.Fatalf("refresh %d failed: %v", i, err)
		}
	}
	if got := pluginListHits.Load(); got != 1 {
		t.Fatalf("expected plugin discovery to be cached once, got %d list requests", got)
	}
}

// 插件列表接口不可用时（老版本 CPA）仍要按历史 ID 兜底。
func TestFetchZcodeSummaryFallsBackToLegacyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/resource/plugins/zcode/summary" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"logged_in":true,"windows":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "management-secret", 2*time.Second, false)
	raw, err := client.FetchZcodeSummary(context.Background())
	if err != nil {
		t.Fatalf("expected legacy endpoint fallback to succeed, got: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("expected JSON summary, got %q: %v", raw, err)
	}
}

// 插件再次改名时，缓存里的旧端点会 404；此时必须丢弃缓存并重新发现，
// 而不是一直打到 TTL 过期为止。
func TestFetchZcodeSummaryRediscoversAfterCachedEndpointGoesStale(t *testing.T) {
	var currentID atomic.Value
	currentID.Store("zcode-oauth")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := currentID.Load().(string)
		switch r.URL.Path {
		case "/v0/management/plugins":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plugins":[{"id":"` + id + `","enabled":true}]}`))
		case "/v0/resource/plugins/" + id + "/summary":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"logged_in":true,"windows":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "management-secret", 2*time.Second, false)
	if _, err := client.FetchZcodeSummary(context.Background()); err != nil {
		t.Fatalf("initial refresh failed: %v", err)
	}

	// 模拟运维再次重命名插件文件。
	currentID.Store("zcode-oauth-v2")
	if _, err := client.FetchZcodeSummary(context.Background()); err != nil {
		t.Fatalf("expected rediscovery after rename, got: %v", err)
	}
}
