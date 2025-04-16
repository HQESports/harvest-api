package controller

import (
	"context"
	"harvest/internal/database"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
)

// MetricsController handles match metrics operations
type MetricsController interface {
	// GetTotalMatchCount gets the total number of matches
	GetTotalMatchCount(ctx context.Context) (int64, error)

	// GetMatchCountsByMap gets distribution of matches by map
	GetMatchCountsByMap(ctx context.Context) (map[string]int64, error)

	// GetMatchCountsByType gets distribution of matches by match type
	GetMatchCountsByType(ctx context.Context) (map[string]int64, error)

	// GetMatchMetricsForTimeRange gets comprehensive metrics for a time range
	GetMatchMetricsForTimeRange(ctx context.Context, startTime, endTime time.Time) (*model.MatchMetrics, error)
}

// metricsController implements MetricsController
type metricsController struct {
	db database.MatchMetricsDatabase
}

// NewMetricsController creates a new metrics controller
func NewMetricsController(db database.MatchMetricsDatabase) MetricsController {
	return &metricsController{
		db: db,
	}
}

// GetTotalMatchCount gets the total number of matches
func (c *metricsController) GetTotalMatchCount(ctx context.Context) (int64, error) {
	count, err := c.db.GetTotalMatchCount(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get total match count")
		return 0, err
	}

	return count, nil
}

// GetMatchCountsByMap gets distribution of matches by map
func (c *metricsController) GetMatchCountsByMap(ctx context.Context) (map[string]int64, error) {
	mapCounts, err := c.db.GetMatchCountByMap(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get match counts by map")
		return nil, err
	}

	return mapCounts, nil
}

// GetMatchCountsByType gets distribution of matches by match type
func (c *metricsController) GetMatchCountsByType(ctx context.Context) (map[string]int64, error) {
	typeCounts, err := c.db.GetMatchCountByType(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get match counts by type")
		return nil, err
	}

	return typeCounts, nil
}

// GetMatchMetricsForTimeRange gets comprehensive metrics for a time range
func (c *metricsController) GetMatchMetricsForTimeRange(ctx context.Context, startTime, endTime time.Time) (*model.MatchMetrics, error) {
	metrics, err := c.db.GetMatchMetricsForTimeRange(ctx, startTime, endTime)
	if err != nil {
		log.Error().Err(err).
			Time("startTime", startTime).
			Time("endTime", endTime).
			Msg("Failed to get match metrics for time range")
		return nil, err
	}

	return metrics, nil
}
