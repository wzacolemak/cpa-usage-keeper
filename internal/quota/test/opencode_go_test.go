package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cpa-usage-keeper/internal/quota"
)

// TestFetchOpencodeGoUsageParsesWindows 验证 Keeper 能直连 OpenCode Go 的 usage 端点，
// 并按 5h/周/月 的固定顺序折算成 quota row。
func TestFetchOpencodeGoUsageParsesWindows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected bearer key, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{
			"monthly":{"status":"ok","percent":10,"resetsAt":"2026-12-04T08:17:26Z"},
			"rolling":{"status":"ok","percent":40,"resetsAt":"2026-11-04T13:29:43Z"},
			"weekly":{"status":"ok","percent":70,"resetsAt":"2026-11-09T00:00:00Z"}
		}}`))
	}))
	defer server.Close()

	payload, err := quota.FetchOpencodeGoUsage(context.Background(), server.URL, "test-key", 0)
	if err != nil {
		t.Fatalf("FetchOpencodeGoUsage returned error: %v", err)
	}
	if len(payload.Usage) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(payload.Usage))
	}
	if payload.Usage["rolling"].Percent != 40 {
		t.Fatalf("expected rolling percent 40, got %v", payload.Usage["rolling"].Percent)
	}

	rows := quota.NormalizeQuotaRows(quota.ProviderOutput{Provider: "opencode-go", Result: quota.OpencodeGoResult{Usage: payload}})
	if len(rows) != 3 {
		t.Fatalf("expected 3 quota rows, got %d", len(rows))
	}
	// 顺序固定为 5h → 周 → 月，且 percent 直接作为 usedPercent。
	wantKeys := []string{"opencode-go.rolling", "opencode-go.weekly", "opencode-go.monthly"}
	wantLabels := []string{"5h", "Weekly", "Monthly"}
	for i, row := range rows {
		if row.Key != wantKeys[i] || row.Label != wantLabels[i] {
			t.Fatalf("row %d: expected %s/%s, got %s/%s", i, wantKeys[i], wantLabels[i], row.Key, row.Label)
		}
		if row.UsedPercent == nil {
			t.Fatalf("row %d: expected usedPercent", i)
		}
	}
	if *rows[0].UsedPercent != 40 || *rows[1].UsedPercent != 70 || *rows[2].UsedPercent != 10 {
		t.Fatalf("unexpected usedPercent values: %v %v %v", *rows[0].UsedPercent, *rows[1].UsedPercent, *rows[2].UsedPercent)
	}
}

// TestFetchOpencodeGoUsageRequiresKey 验证缺少 key 时不发请求而是直接报错。
func TestFetchOpencodeGoUsageRequiresKey(t *testing.T) {
	if _, err := quota.FetchOpencodeGoUsage(context.Background(), "https://example.com", "", 0); err == nil {
		t.Fatal("expected error when api key is missing")
	}
}

// TestFetchOpencodeGoUsageRejectsNon2xx 验证非 2xx 不回显响应体（避免泄露内部信息）。
func TestFetchOpencodeGoUsageRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"secret-detail"}`))
	}))
	defer server.Close()

	_, err := quota.FetchOpencodeGoUsage(context.Background(), server.URL, "test-key", 0)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if contains(err.Error(), "secret-detail") {
		t.Fatalf("error must not echo response body, got %q", err.Error())
	}
}
