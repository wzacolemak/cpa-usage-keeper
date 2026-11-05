package service

import (
	"context"
	"errors"
	"fmt"

	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"

	"gorm.io/gorm"
)

// ErrInvalidTimeWindowInput 表示分时段配置本身不合法（时段重叠、时区错误、区间格式错误等）。
var ErrInvalidTimeWindowInput = errors.New("invalid pricing time window input")

// GetPricingTimeWindows 返回当前分时段配置，供前端回显。
func (s *pricingService) GetPricingTimeWindows(ctx context.Context) (pricing.TimeWindowConfig, error) {
	if s == nil || s.db == nil {
		return pricing.TimeWindowConfig{}, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return repository.LoadPricingTimeWindowConfig(ctx, s.db)
}

// UpdatePricingTimeWindows 保存分时段配置并原子发布新的价格快照。
//
// 保存后所有成本计算立即按新时段生效；因为成本是查询时重算的，
// 历史数据的统计口径也会随之更新，无需回填。
func (s *pricingService) UpdatePricingTimeWindows(ctx context.Context, config pricing.TimeWindowConfig) (pricing.TimeWindowConfig, error) {
	if s == nil || s.db == nil {
		return pricing.TimeWindowConfig{}, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		_, saveErr := repository.SavePricingTimeWindowConfig(ctx, tx, config)
		return saveErr
	})
	if err != nil {
		if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
			return pricing.TimeWindowConfig{}, fmt.Errorf("%w: %v", ErrInvalidTimeWindowInput, err)
		}
		return pricing.TimeWindowConfig{}, err
	}
	return repository.LoadPricingTimeWindowConfig(ctx, s.db)
}

// ListPricingTimeWindowNames 返回已配置的时段名，供规则编辑界面做取值提示。
func (s *pricingService) ListPricingTimeWindowNames(ctx context.Context) []string {
	if s == nil || s.catalog == nil {
		return nil
	}
	return s.catalog.Snapshot().TimeWindows().Names()
}
