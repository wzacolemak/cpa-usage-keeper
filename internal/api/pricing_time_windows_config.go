package api

import (
	"errors"
	"net/http"

	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/service"

	"github.com/gin-gonic/gin"
)

// timeWindowSpecResponse 是单个时段的 API 表示。
type timeWindowSpecResponse struct {
	Name     string   `json:"name"`
	Weekdays []int    `json:"weekdays"`
	Ranges   []string `json:"ranges"`
}

// timeWindowsResponse 是分时段配置的 API 响应。
type timeWindowsResponse struct {
	Timezone string                   `json:"timezone"`
	Windows  []timeWindowSpecResponse `json:"windows"`
}

type updateTimeWindowsRequest struct {
	Timezone string                   `json:"timezone"`
	Windows  []timeWindowSpecResponse `json:"windows"`
}

// registerPricingTimeWindowRoutes 注册分时段计价的读写接口。
//
// 时段本身只描述“几点到几点叫什么名字”；具体倍率仍由各模型的价格规则
// （time_window=<时段名>）决定，这样既复用了既有规则体系，也允许不同模型
// 对同一时段设置不同倍率。
func registerPricingTimeWindowRoutes(router gin.IRoutes, pricingProvider service.PricingProvider) {
	router.GET("/pricing/time-windows", func(c *gin.Context) {
		if pricingProvider == nil {
			c.JSON(http.StatusOK, timeWindowsResponse{Windows: []timeWindowSpecResponse{}})
			return
		}
		config, err := pricingProvider.GetPricingTimeWindows(c.Request.Context())
		if err != nil {
			writeInternalError(c, "get pricing time windows failed", err)
			return
		}
		c.JSON(http.StatusOK, timeWindowsResponseFromConfig(config))
	})

	router.PUT("/pricing/time-windows", func(c *gin.Context) {
		if pricingProvider == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pricing provider is not configured"})
			return
		}
		var request updateTimeWindowsRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		config, err := pricingProvider.UpdatePricingTimeWindows(c.Request.Context(), timeWindowConfigFromRequest(request))
		if err != nil {
			if errors.Is(err, service.ErrInvalidTimeWindowInput) || errors.Is(err, service.ErrInvalidPricingInput) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			writeInternalError(c, "update pricing time windows failed", err)
			return
		}
		c.JSON(http.StatusOK, timeWindowsResponseFromConfig(config))
	})
}

func timeWindowsResponseFromConfig(config pricing.TimeWindowConfig) timeWindowsResponse {
	windows := make([]timeWindowSpecResponse, 0, len(config.Windows))
	for _, spec := range config.Windows {
		weekdays := spec.Weekdays
		if weekdays == nil {
			weekdays = []int{}
		}
		ranges := spec.Ranges
		if ranges == nil {
			ranges = []string{}
		}
		windows = append(windows, timeWindowSpecResponse{
			Name:     spec.Name,
			Weekdays: weekdays,
			Ranges:   ranges,
		})
	}
	return timeWindowsResponse{Timezone: config.Timezone, Windows: windows}
}

func timeWindowConfigFromRequest(request updateTimeWindowsRequest) pricing.TimeWindowConfig {
	config := pricing.TimeWindowConfig{Timezone: request.Timezone}
	for _, spec := range request.Windows {
		config.Windows = append(config.Windows, pricing.TimeWindowSpec{
			Name:     spec.Name,
			Weekdays: spec.Weekdays,
			Ranges:   spec.Ranges,
		})
	}
	return config
}
