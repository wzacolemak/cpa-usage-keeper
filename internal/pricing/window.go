package pricing

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultTimeWindowTimezone 是未显式配置时区时的默认值。
//
// 分时段计价的现实需求主要来自中国区模型（DeepSeek 的高峰/空闲时段按北京时间划分，
// 09:00-12:00 与 14:00-18:00 为高峰），因此默认用 Asia/Shanghai，避免用户不填时
// 静默按 UTC 计算导致峰谷反了。
const DefaultTimeWindowTimezone = "Asia/Shanghai"

// TimeWindowSpec 是一个命名的时段定义。
//
// Weekdays 用 0=周日 … 6=周六（与 time.Weekday 一致），留空表示每天都生效。
// Ranges 是 "HH:MM-HH:MM" 形式的半开区间列表，允许跨零点（如 "22:00-06:00"）。
type TimeWindowSpec struct {
	Name     string   `json:"name"`
	Weekdays []int    `json:"weekdays,omitempty"`
	Ranges   []string `json:"ranges"`
}

// TimeWindowConfig 是持久化的分时段配置。
type TimeWindowConfig struct {
	Timezone string           `json:"timezone"`
	Windows  []TimeWindowSpec `json:"windows"`
}

// compiledWindow 是解析完成后的单个时段。
type compiledWindow struct {
	name string
	// days 记录该时段在哪些星期生效，索引即 time.Weekday。
	days [7]bool
	// allDays 表示未限定星期，任意星期都生效。
	allDays bool
	// slots 记录该时段的分钟区间（半开）。
	slots []minuteRange
}

type minuteRange struct {
	start int // 含
	end   int // 不含；跨零点时由展开逻辑保证有序
}

// TimeWindowSet 是编译后的只读时段表，可安全并发使用。
type TimeWindowSet struct {
	loc     *time.Location
	windows []compiledWindow
	names   []string
}

// CompileTimeWindows 校验并编译分时段配置。
//
// 空配置返回 (nil, nil)，表示不做分时段计价；调用方据此跳过相关逻辑。
func CompileTimeWindows(config TimeWindowConfig) (*TimeWindowSet, error) {
	if len(config.Windows) == 0 {
		return nil, nil
	}

	loc, err := resolveTimeWindowLocation(config.Timezone)
	if err != nil {
		return nil, err
	}

	set := &TimeWindowSet{loc: loc}
	seenNames := make(map[string]struct{}, len(config.Windows))
	for index, spec := range config.Windows {
		compiled, err := compileTimeWindowSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("time window at index %d: %w", index, err)
		}
		if _, exists := seenNames[compiled.name]; exists {
			return nil, fmt.Errorf("duplicate time window %q", compiled.name)
		}
		seenNames[compiled.name] = struct{}{}
		set.windows = append(set.windows, compiled)
		set.names = append(set.names, compiled.name)
	}
	sort.Strings(set.names)
	if err := set.validateNoOverlap(); err != nil {
		return nil, err
	}
	return set, nil
}

func resolveTimeWindowLocation(name string) (*time.Location, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = DefaultTimeWindowTimezone
	}
	loc, err := time.LoadLocation(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid time window timezone %q: %w", trimmed, err)
	}
	return loc, nil
}

func compileTimeWindowSpec(spec TimeWindowSpec) (compiledWindow, error) {
	name := normalizeTimeWindowName(spec.Name)
	if name == "" {
		return compiledWindow{}, fmt.Errorf("window name is required")
	}
	compiled := compiledWindow{name: name}
	if len(spec.Weekdays) == 0 {
		compiled.allDays = true
	} else {
		for _, day := range spec.Weekdays {
			if day < 0 || day > 6 {
				return compiledWindow{}, fmt.Errorf("window %q weekday %d out of range 0-6", name, day)
			}
			compiled.days[day] = true
		}
	}
	if len(spec.Ranges) == 0 {
		return compiledWindow{}, fmt.Errorf("window %q requires at least one range", name)
	}
	for _, raw := range spec.Ranges {
		slots, err := parseTimeWindowRange(raw)
		if err != nil {
			return compiledWindow{}, fmt.Errorf("window %q: %w", name, err)
		}
		compiled.slots = append(compiled.slots, slots...)
	}
	return compiled, nil
}

func normalizeTimeWindowName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// parseTimeWindowRange 把 "HH:MM-HH:MM" 解析为分钟区间。
//
// 跨零点的区间（如 22:00-06:00）会拆成 [22:00,24:00) 与 [00:00,06:00) 两段，
// 这样查表时无需再处理环绕逻辑。
func parseTimeWindowRange(raw string) ([]minuteRange, error) {
	trimmed := strings.TrimSpace(raw)
	left, right, ok := strings.Cut(trimmed, "-")
	if !ok {
		return nil, fmt.Errorf("invalid range %q, expected HH:MM-HH:MM", trimmed)
	}
	start, err := parseClockMinutes(left)
	if err != nil {
		return nil, fmt.Errorf("invalid range %q: %w", trimmed, err)
	}
	end, err := parseClockMinutes(right)
	if err != nil {
		return nil, fmt.Errorf("invalid range %q: %w", trimmed, err)
	}
	if start == end {
		return nil, fmt.Errorf("invalid range %q: start equals end", trimmed)
	}
	if start < end {
		return []minuteRange{{start: start, end: end}}, nil
	}
	// 跨零点：拆成当天尾巴与当天开头两段。
	return []minuteRange{{start: start, end: 24 * 60}, {start: 0, end: end}}, nil
}

func parseClockMinutes(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	hourText, minuteText, ok := strings.Cut(trimmed, ":")
	if !ok {
		return 0, fmt.Errorf("invalid clock %q, expected HH:MM", trimmed)
	}
	hour, err := strconv.Atoi(strings.TrimSpace(hourText))
	if err != nil {
		return 0, fmt.Errorf("invalid hour in %q", trimmed)
	}
	minute, err := strconv.Atoi(strings.TrimSpace(minuteText))
	if err != nil {
		return 0, fmt.Errorf("invalid minute in %q", trimmed)
	}
	if hour < 0 || hour > 24 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("clock %q out of range", trimmed)
	}
	if hour == 24 && minute != 0 {
		return 0, fmt.Errorf("clock %q out of range", trimmed)
	}
	return hour*60 + minute, nil
}

// validateNoOverlap 拒绝互相覆盖的时段定义，避免同一次请求命中两条倍率不同的规则。
func (s *TimeWindowSet) validateNoOverlap() error {
	for day := 0; day < 7; day++ {
		for minute := 0; minute < 24*60; minute++ {
			first := ""
			for _, window := range s.windows {
				if !window.allDays && !window.days[day] {
					continue
				}
				if !window.coversMinute(minute) {
					continue
				}
				if first != "" {
					return fmt.Errorf("time windows %q and %q overlap on weekday %d at %02d:%02d", first, window.name, day, minute/60, minute%60)
				}
				first = window.name
			}
		}
	}
	return nil
}

func (w compiledWindow) coversMinute(minute int) bool {
	for _, slot := range w.slots {
		if minute >= slot.start && minute < slot.end {
			return true
		}
	}
	return false
}

// WindowAt 返回给定时刻命中的时段名；未命中任何时段时返回空字符串。
func (s *TimeWindowSet) WindowAt(at time.Time) string {
	if s == nil || at.IsZero() {
		return ""
	}
	local := at.In(s.loc)
	day := int(local.Weekday())
	minute := local.Hour()*60 + local.Minute()
	for _, window := range s.windows {
		if !window.allDays && !window.days[day] {
			continue
		}
		if window.coversMinute(minute) {
			return window.name
		}
	}
	return ""
}

// Names 返回已配置的时段名（稳定排序），供前端选择规则取值。
func (s *TimeWindowSet) Names() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.names...)
}

// Location 返回时段计算使用的时区。
func (s *TimeWindowSet) Location() *time.Location {
	if s == nil {
		return nil
	}
	return s.loc
}
