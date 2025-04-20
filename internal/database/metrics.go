package database

import (
	"context"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MatchMetricsDatabase defines match metrics-related database operations
type MatchMetricsDatabase interface {
	// Get total count of matches
	GetTotalMatchCount(ctx context.Context) (int64, error)

	// Get count of matches by map name
	GetMatchCountByMap(ctx context.Context) (map[string]int64, error)

	// Get count of matches by match type
	GetMatchCountByType(ctx context.Context) (map[string]int64, error)

	// Get metrics for a specific time range
	GetMatchMetricsForTimeRange(ctx context.Context, startTime, endTime time.Time) (*model.MatchMetrics, error)
}

// GetTotalMatchCount returns the total number of matches in the database
func (m *mongoDB) GetTotalMatchCount(ctx context.Context) (int64, error) {
	count, err := m.matchesCol.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to count total matches")
		return 0, err
	}

	return count, nil
}

// GetMatchCountByMap returns the count of matches for each map name
func (m *mongoDB) GetMatchCountByMap(ctx context.Context) (map[string]int64, error) {
	// Use MongoDB aggregation to group by map_name and count
	pipeline := mongo.Pipeline{
		{{
			Key: "$group",
			Value: bson.D{
				{Key: "_id", Value: "$map_name"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			},
		}},
	}

	cursor, err := m.matchesCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate matches by map")
		return nil, err
	}
	defer cursor.Close(ctx)

	// Process results
	mapCounts := make(map[string]int64)
	for cursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			log.Error().Err(err).Msg("Failed to decode map count result")
			return nil, err
		}
		mapCounts[result.ID] = result.Count
	}

	if err := cursor.Err(); err != nil {
		log.Error().Err(err).Msg("Error iterating map count results")
		return nil, err
	}

	return mapCounts, nil
}

// GetMatchCountByType returns the count of matches for each match type
func (m *mongoDB) GetMatchCountByType(ctx context.Context) (map[string]int64, error) {
	// Use MongoDB aggregation to group by match_type and count
	pipeline := mongo.Pipeline{
		{{
			Key: "$group",
			Value: bson.D{
				{Key: "_id", Value: "$match_type"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			},
		}},
	}

	cursor, err := m.matchesCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate matches by type")
		return nil, err
	}
	defer cursor.Close(ctx)

	// Process results
	typeCounts := make(map[string]int64)
	for cursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			log.Error().Err(err).Msg("Failed to decode match type count result")
			return nil, err
		}
		typeCounts[result.ID] = result.Count
	}

	if err := cursor.Err(); err != nil {
		log.Error().Err(err).Msg("Error iterating match type count results")
		return nil, err
	}

	return typeCounts, nil
}

// GetMatchMetricsForTimeRange returns comprehensive match metrics for a specific time range
func (m *mongoDB) GetMatchMetricsForTimeRange(ctx context.Context, startTime, endTime time.Time) (*model.MatchMetrics, error) {
	// Create a filter for the time range
	filter := bson.M{
		"created_at": bson.M{
			"$gte": startTime,
			"$lte": endTime,
		},
	}

	// Get total count for the time range
	totalCount, err := m.matchesCol.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to count matches in time range")
		return nil, err
	}

	// Get total count of players
	playerCount, err := m.playersCol.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to count players")
		return nil, err
	}

	// Get tournament count
	touramentCount, err := m.tournamentsCol.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to count players")
		return nil, err
	}

	// Group by map_name for the time range
	mapPipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{
			Key: "$group",
			Value: bson.D{
				{Key: "_id", Value: "$map_name"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			},
		}},
	}

	mapCursor, err := m.matchesCol.Aggregate(ctx, mapPipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate matches by map in time range")
		return nil, err
	}
	defer mapCursor.Close(ctx)

	// Process map results
	mapCounts := make(map[string]int64)
	for mapCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := mapCursor.Decode(&result); err != nil {
			log.Error().Err(err).Msg("Failed to decode map count result")
			return nil, err
		}
		mapCounts[result.ID] = result.Count
	}

	// Group by match_type for the time range
	typePipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{
			Key: "$group",
			Value: bson.D{
				{Key: "_id", Value: "$match_type"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			},
		}},
	}

	typeCursor, err := m.matchesCol.Aggregate(ctx, typePipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate matches by type in time range")
		return nil, err
	}
	defer typeCursor.Close(ctx)

	// Process type results
	typeCounts := make(map[string]int64)
	for typeCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := typeCursor.Decode(&result); err != nil {
			log.Error().Err(err).Msg("Failed to decode match type count result")
			return nil, err
		}
		typeCounts[result.ID] = result.Count
	}

	// Group by processed status for the time range
	processedPipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{
			Key: "$group",
			Value: bson.D{
				{Key: "_id", Value: "$processed"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			},
		}},
	}

	processedCursor, err := m.matchesCol.Aggregate(ctx, processedPipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate matches by processed status in time range")
		return nil, err
	}
	defer processedCursor.Close(ctx)

	// Process processed status results
	processedCounts := make(map[bool]int64)
	for processedCursor.Next(ctx) {
		var result struct {
			ID    bool  `bson:"_id"`
			Count int64 `bson:"count"`
		}
		if err := processedCursor.Decode(&result); err != nil {
			log.Error().Err(err).Msg("Failed to decode processed status count result")
			return nil, err
		}
		processedCounts[result.ID] = result.Count
	}

	// Group by shard for the time range
	shardPipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{
			Key: "$group",
			Value: bson.D{
				{Key: "_id", Value: "$shard_id"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			},
		}},
	}

	shardCursor, err := m.matchesCol.Aggregate(ctx, shardPipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate matches by shard in time range")
		return nil, err
	}
	defer shardCursor.Close(ctx)

	// Process shard results
	shardCounts := make(map[string]int64)
	for shardCursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := shardCursor.Decode(&result); err != nil {
			log.Error().Err(err).Msg("Failed to decode shard count result")
			return nil, err
		}
		shardCounts[result.ID] = result.Count
	}

	proccessedDistro := map[string]int64{
		"true":  processedCounts[true],
		"false": processedCounts[false],
	}

	// Compile all metrics into a single result
	metrics := &model.MatchMetrics{
		TotalMatches:          totalCount,
		TotalPlayers:          playerCount,
		TotalTournaments:      touramentCount,
		MapDistribution:       mapCounts,
		TypeDistribution:      typeCounts,
		ProcessedDistribution: proccessedDistro,
		ShardDistribution:     shardCounts,
		TimeRange: model.TimeRange{
			Start: startTime,
			End:   endTime,
		},
	}

	return metrics, nil
}
