package test

import (
	"testing"
	"time"

	"cpa-usage-keeper/internal/pricing"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %s: %v", name, err)
	}
	return loc
}

func TestCompileTimeWindowsRejectsInvalidInput(t *testing.T) {
	cases := map[string]pricing.TimeWindowConfig{
		"empty name": {Windows: []pricing.TimeWindowSpec{{Name: "  ", Ranges: []string{"09:00-12:00"}}}},
		"no ranges":  {Windows: []pricing.TimeWindowSpec{{Name: "peak"}}},
		"bad range":  {Windows: []pricing.TimeWindowSpec{{Name: "peak", Ranges: []string{"9-12"}}}},
		"bad clock":  {Windows: []pricing.TimeWindowSpec{{Name: "peak", Ranges: []string{"25:00-26:00"}}}},
		"same bounds": {Windows: []pricing.TimeWindowSpec{{Name: "peak", Ranges: []string{
			"09:00-09:00",
		}}}},
		"bad weekday": {Windows: []pricing.TimeWindowSpec{{Name: "peak", Weekdays: []int{7}, Ranges: []string{"09:00-12:00"}}}},
		"bad timezone": {Timezone: "Not/AZone", Windows: []pricing.TimeWindowSpec{
			{Name: "peak", Ranges: []string{"09:00-12:00"}},
		}},
		"duplicate name": {Windows: []pricing.TimeWindowSpec{
			{Name: "peak", Ranges: []string{"09:00-12:00"}},
			{Name: "Peak", Ranges: []string{"14:00-18:00"}},
		}},
		"overlapping": {Windows: []pricing.TimeWindowSpec{
			{Name: "peak", Ranges: []string{"09:00-12:00"}},
			{Name: "late-morning", Ranges: []string{"11:00-13:00"}},
		}},
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := pricing.CompileTimeWindows(config); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestCompileTimeWindowsEmptyDisablesFeature(t *testing.T) {
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set != nil {
		t.Fatalf("expected nil set, got %#v", set)
	}
	if got := set.WindowAt(time.Now()); got != "" {
		t.Fatalf("nil set must resolve to empty window, got %q", got)
	}
}

func TestTimeWindowSetResolvesDeepSeekPeakHours(t *testing.T) {
	// DeepSeek 官方高峰时段：北京时间周一至周五 09:00-12:00、14:00-18:00。
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{
		Timezone: "Asia/Shanghai",
		Windows: []pricing.TimeWindowSpec{{
			Name:     "peak",
			Weekdays: []int{1, 2, 3, 4, 5},
			Ranges:   []string{"09:00-12:00", "14:00-18:00"},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	loc := mustLoadLocation(t, "Asia/Shanghai")
	cases := []struct {
		label string
		t     time.Time
		want  string
	}{
		{"周一早高峰起点", time.Date(2026, 11, 2, 9, 0, 0, 0, loc), "peak"},
		{"周一早高峰中段", time.Date(2026, 11, 2, 10, 30, 0, 0, loc), "peak"},
		{"周一午间空闲", time.Date(2026, 11, 2, 12, 30, 0, 0, loc), ""},
		{"周一早高峰右开区间", time.Date(2026, 11, 2, 12, 0, 0, 0, loc), ""},
		{"周一午后高峰", time.Date(2026, 11, 2, 14, 0, 0, 0, loc), "peak"},
		{"周一晚间高峰结束", time.Date(2026, 11, 2, 18, 0, 0, 0, loc), ""},
		{"周六全天空闲", time.Date(2026, 11, 7, 10, 0, 0, 0, loc), ""},
		{"周日全天空闲", time.Date(2026, 11, 8, 15, 0, 0, 0, loc), ""},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if got := set.WindowAt(tc.t); got != tc.want {
				t.Fatalf("WindowAt(%s) = %q, want %q", tc.t.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}

func TestTimeWindowSetConvertsAcrossTimezones(t *testing.T) {
	// UTC 01:00-04:00 等价于北京时间 09:00-12:00，两种写法必须解析一致。
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{
		Timezone: "Asia/Shanghai",
		Windows: []pricing.TimeWindowSpec{{
			Name:     "peak",
			Weekdays: []int{1},
			Ranges:   []string{"09:00-12:00"},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// 2026-11-02 是周一，UTC 02:00 = 北京 10:00。
	utcTime := time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	if got := set.WindowAt(utcTime); got != "peak" {
		t.Fatalf("UTC 02:00 should be Beijing peak, got %q", got)
	}
	// UTC 05:00 = 北京 13:00，属于空闲。
	if got := set.WindowAt(time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC)); got != "" {
		t.Fatalf("UTC 05:00 should be off-peak, got %q", got)
	}
}

func TestTimeWindowSetSupportsOvernightRange(t *testing.T) {
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{
		Timezone: "UTC",
		Windows: []pricing.TimeWindowSpec{{
			Name:   "night",
			Ranges: []string{"22:00-06:00"},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, tc := range []struct {
		hour int
		want string
	}{{23, "night"}, {0, "night"}, {5, "night"}, {6, ""}, {12, ""}, {21, ""}} {
		at := time.Date(2026, 11, 2, tc.hour, 0, 0, 0, time.UTC)
		if got := set.WindowAt(at); got != tc.want {
			t.Fatalf("hour %d = %q, want %q", tc.hour, got, tc.want)
		}
	}
}

func TestTimeWindowSetDefaultsToShanghai(t *testing.T) {
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{
		Windows: []pricing.TimeWindowSpec{{
			Name:   "peak",
			Ranges: []string{"09:00-12:00"},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if name := set.Location().String(); name != "Asia/Shanghai" {
		t.Fatalf("default timezone = %q, want Asia/Shanghai", name)
	}
}

func TestTimeWindowSetNamesAreSorted(t *testing.T) {
	set, err := pricing.CompileTimeWindows(pricing.TimeWindowConfig{
		Timezone: "UTC",
		Windows: []pricing.TimeWindowSpec{
			{Name: "peak", Ranges: []string{"09:00-12:00"}},
			{Name: "off_peak", Ranges: []string{"12:00-13:00"}},
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	names := set.Names()
	if len(names) != 2 || names[0] != "off_peak" || names[1] != "peak" {
		t.Fatalf("names = %v, want [off_peak peak]", names)
	}
}
