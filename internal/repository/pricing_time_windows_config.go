package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"

	"gorm.io/gorm"
)

// PricingTimeWindowSettingKey 是分时段配置在 app_settings 里的键名。
const PricingTimeWindowSettingKey = "pricing_time_windows"

// LoadPricingTimeWindows 读取并编译分时段配置。
//
// 未配置时返回 (nil, nil)，表示不启用分时段计价。
func LoadPricingTimeWindows(ctx context.Context, db *gorm.DB) (*pricing.TimeWindowSet, error) {
	config, err := LoadPricingTimeWindowConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	set, err := pricing.CompileTimeWindows(config)
	if err != nil {
		return nil, fmt.Errorf("compile pricing time windows: %w", err)
	}
	return set, nil
}

// LoadPricingTimeWindowConfig 读取原始分时段配置（不做编译，便于回显给前端）。的
func LoadPricingTimeWindowConfig(ctx context.Context, db *gorm.DB) (pricing.TimeWindowConfig, error) {
	if db == nil {
		return pricing.TimeWindowConfig{}, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	setting, found, err := GetAppSetting(ctx, db, PricingTimeWindowSettingKey)
	if err != nil {
		return pricing.TimeWindowConfig{}, err
	}
	if !found || setting.Value == nil || strings.TrimSpace(*setting.Value) == "" {
		return pricing.TimeWindowConfig{}, nil
	}
	var config pricing.TimeWindowConfig
	if err := json.Unmarshal([]byte(*setting.Value), &config); err != nil {
		return pricing.TimeWindowConfig{}, fmt.Errorf("decode %s: %w", PricingTimeWindowSettingKey, err)
	}
	return config, nil
}

// SavePricingTimeWindowConfig 持久化分时段配置，并先编译校验避免写入坏配置。
func SavePricingTimeWindowConfig(ctx context.Context, db *gorm.DB, config pricing.TimeWindowConfig) (pricing.TimeWindowConfig, error) {
	if db == nil {
		return pricing.TimeWindowConfig{}, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := pricing.CompileTimeWindows(config); err != nil {
		return pricing.TimeWindowConfig{}, err
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return pricing.TimeWindowConfig{}, fmt.Errorf("encode %s: %w", PricingTimeWindowSettingKey, err)
	}
	value := string(payload)
	if _, err := UpsertAppSetting(ctx, db, entities.AppSetting{
		SettingKey: PricingTimeWindowSettingKey,
		Value:      &value,
		ValueType:  entities.AppSettingValueTypeJSON,
	}); err != nil {
		return pricing.TimeWindowConfig{}, err
	}
	return config, nil
}
