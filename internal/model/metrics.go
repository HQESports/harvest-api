package model

import "time"

// TimeRange represents a time range for metrics
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// DurationStats represents statistics about match durations
type DurationStats struct {
	AvgDuration int
	MaxDuration int
	MinDuration int
}

// PlayerStats represents statistics about players in matches
type PlayerStats struct {
	AvgPlayers   float64
	MaxPlayers   int
	TotalPlayers int
}

type MatchMetrics struct {
	TotalMatches          int64            `json:"total_matches"`
	TotalPlayers          int64            `json:"total_players"`
	TotalTournaments      int64            `json:"total_tournaments"`
	MapDistribution       map[string]int64 `json:"map_distribution"`
	TypeDistribution      map[string]int64 `json:"type_distribution"`
	ProcessedDistribution map[string]int64 `json:"processed_distribution"`
	ShardDistribution     map[string]int64 `json:"shard_distribution"`
	TimeRange             TimeRange        `json:"time_range"`
}
