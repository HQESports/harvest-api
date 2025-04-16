package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// MetricsResponse represents the response for metrics operations
type MetricsResponse struct {
	TotalMatches     int64              `json:"totalMatches"`
	MapDistribution  map[string]int64   `json:"mapDistribution"`
	TypeDistribution map[string]int64   `json:"typeDistribution"`
	TimeRange        *TimeRangeResponse `json:"timeRange,omitempty"`
}

// TimeRangeResponse represents a time range in the response
type TimeRangeResponse struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// GetTotalMatchCountHandler returns the total count of matches
func (s *Server) GetTotalMatchCountHandler(c *gin.Context) {
	count, err := s.mc.GetTotalMatchCount(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get total match count: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"totalMatches": count})
}

// GetMatchDistributionHandler returns the distribution of matches by map and type
func (s *Server) GetMatchDistributionHandler(c *gin.Context) {
	// Get matches by map
	metrics, err := s.mc.GetMatchMetricsForTimeRange(c.Request.Context(), time.Time{}, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error aggregating metrics"})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// GetMatchMetricsForTimeRangeHandler returns metrics for a specific time range
func (s *Server) GetMatchMetricsForTimeRangeHandler(c *gin.Context) {
	// Parse start and end times from query parameters
	startTimeStr := c.Query("start")
	endTimeStr := c.Query("end")

	if startTimeStr == "" || endTimeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Both start and end time parameters are required"})
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start time format. Use RFC3339 format (e.g. 2023-01-01T00:00:00Z)"})
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end time format. Use RFC3339 format (e.g. 2023-01-01T00:00:00Z)"})
		return
	}

	if endTime.Before(startTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "End time must be after start time"})
		return
	}

	// Get metrics for the time range
	metrics, err := s.mc.GetMatchMetricsForTimeRange(c.Request.Context(), startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get match metrics: " + err.Error()})
		return
	}

	// Prepare response
	response := MetricsResponse{
		TotalMatches:     metrics.TotalMatches,
		MapDistribution:  metrics.MapDistribution,
		TypeDistribution: metrics.TypeDistribution,
		TimeRange: &TimeRangeResponse{
			Start: metrics.TimeRange.Start.Format(time.RFC3339),
			End:   metrics.TimeRange.End.Format(time.RFC3339),
		},
	}

	c.JSON(http.StatusOK, response)
}
