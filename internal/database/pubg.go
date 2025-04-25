package database

import (
	"context"
	"fmt"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PubgDatabase interface {
	BulkUpsertEntities(context.Context, *mongo.Collection, []model.Entity) (*mongo.BulkWriteResult, error)
	BulkUpsertPlayers(context.Context, []model.Entity) (*mongo.BulkWriteResult, error)
	BulkUpsertTournaments(context.Context, []model.Entity) (*mongo.BulkWriteResult, error)

	GetActivePlayers(context.Context, int) ([]model.Entity, error)
	GetActiveTournaments(context.Context, int) ([]model.Entity, error)
	ImportMatch(context.Context, model.Match) (bool, error)
	ImportMatches(context.Context, []model.Match) (int, error)
	GetProcessedMatchIDs(context.Context) ([]string, error)
	GetUnProcessedMatches(ctx context.Context, minDuration int) ([]model.Match, error)
	GetMatchesByType(context.Context, string, int) ([]model.Match, error)
	MarkMatchAsProcessed(context.Context, string) error
	BulkImportMatches(ctx context.Context, matches []model.Match) (model.BulkImportResult, error)

	UpdateMatchesWithTelemetryData(context.Context, map[string]*model.TelemetryData) (int, error)
	GetMatchesByFilters(ctx context.Context, mapName string, matchTypes []string, startDate *time.Time, endDate *time.Time, limit int) ([]model.Match, error)

	AddTeamRotationToMatch(context.Context, string, model.TeamRotation) error
	GetMatchByID(context.Context, string) (*model.Match, error)
}

func (m *mongoDB) BulkUpsertPlayers(ctx context.Context, entities []model.Entity) (*mongo.BulkWriteResult, error) {
	return m.BulkUpsertEntities(ctx, m.playersCol, entities)
}

func (m *mongoDB) BulkUpsertTournaments(ctx context.Context, entities []model.Entity) (*mongo.BulkWriteResult, error) {
	return m.BulkUpsertEntities(ctx, m.tournamentsCol, entities)
}

// BulkUpsertEntities adds or updates multiple entities in the specified collection
func (m *mongoDB) BulkUpsertEntities(ctx context.Context, col *mongo.Collection, entities []model.Entity) (*mongo.BulkWriteResult, error) {
	emptyResult := &mongo.BulkWriteResult{
		InsertedCount: 0,
		MatchedCount:  0,
		ModifiedCount: 0,
		DeletedCount:  0,
		UpsertedCount: 0,
		UpsertedIDs:   make(map[int64]interface{}),
	}

	if len(entities) == 0 {
		return emptyResult, nil
	}

	// Create a slice of write models for bulk operation
	var models []mongo.WriteModel

	for _, entity := range entities {
		// Ensure entity has Active set to true by default if not specified
		if !entity.Active {
			entity.Active = true
		}

		// Create filter for this entity
		filter := bson.M{"id": entity.ID}

		// Create update model for this entity
		update := bson.M{"$set": entity}

		// Create an UpdateOneModel with upsert option
		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)

		models = append(models, model)
	}

	// Set options for bulk write
	opts := options.BulkWrite().SetOrdered(false)

	// Execute bulk write operation
	writeResult, err := col.BulkWrite(ctx, models, opts)
	if err != nil {
		log.Error().Msgf("Failed to bulk upsert entities: %v", err)
		return emptyResult, err
	}

	return writeResult, nil
}

// GetActiveEntities retrieves all active entities from the specified collection
func (m *mongoDB) getActiveEntities(ctx context.Context, col *mongo.Collection, limit int) ([]model.Entity, error) {
	filter := bson.M{"active": true}

	findOptions := options.Find()

	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		log.Error().Msgf("Error retrieving active entities: %v", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var entities []model.Entity
	if err = cursor.All(ctx, &entities); err != nil {
		log.Error().Msgf("Error decoding entities: %v", err)
		return nil, err
	}

	return entities, nil
}

func (m *mongoDB) GetActivePlayers(ctx context.Context, limit int) ([]model.Entity, error) {
	return m.getActiveEntities(ctx, m.playersCol, limit)
}

func (m *mongoDB) GetActiveTournaments(ctx context.Context, limit int) ([]model.Entity, error) {
	return m.getActiveEntities(ctx, m.tournamentsCol, limit)
}

// ImportMatches imports matches into the database if they don't already exist
func (m *mongoDB) ImportMatches(ctx context.Context, matches []model.Match) (int, error) {
	if len(matches) == 0 {
		return 0, nil
	}

	// Create a slice of write models for bulk operation
	var models []mongo.WriteModel

	for _, match := range matches {
		// Set import timestamp
		match.ImportedAt = time.Now()

		// Create filter to check if match already exists
		filter := bson.M{"match_id": match.MatchID}

		// Create an InsertOneModel with a check for existence
		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(bson.M{"$setOnInsert": match}).
			SetUpsert(true)

		models = append(models, model)
	}

	// Set options for bulk write
	opts := options.BulkWrite().SetOrdered(false)

	// Execute bulk write operation
	result, err := m.matchesCol.BulkWrite(ctx, models, opts)
	if err != nil {
		log.Error().Msgf("Failed to import matches: %v", err)
		return 0, err
	}

	// Return the number of newly inserted documents
	return int(result.UpsertedCount), nil
}

// ImportMatch imports a single match if it doesn't already exist
func (m *mongoDB) ImportMatch(ctx context.Context, match model.Match) (bool, error) {
	// Set import timestamp
	match.ImportedAt = time.Now()

	// Create filter to check if match already exists
	filter := bson.M{"match_id": match.MatchID}

	// Set options for update
	opts := options.Update().SetUpsert(true)

	// Use $setOnInsert to only insert if the document doesn't exist
	result, err := m.matchesCol.UpdateOne(
		ctx,
		filter,
		bson.M{"$setOnInsert": match},
		opts,
	)

	if err != nil {
		log.Error().Msgf("Failed to import match: %v", err)
		return false, err
	}

	// Return true if a new document was inserted, false if it already existed
	return result.UpsertedCount > 0, nil
}

// GetProcessedMatchIDs returns a list of match IDs that have been processed
func (m *mongoDB) GetProcessedMatchIDs(ctx context.Context) ([]string, error) {
	filter := bson.M{"processed": true}

	// Only select the match_id field
	projection := bson.M{"match_id": 1, "_id": 0}
	findOptions := options.Find().SetProjection(projection)

	cursor, err := m.matchesCol.Find(ctx, filter, findOptions)
	if err != nil {
		log.Error().Msgf("Error retrieving processed match IDs: %v", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	// Use a slice of structs to capture the results
	var matches []struct {
		MatchID string `bson:"match_id"`
	}

	if err = cursor.All(ctx, &matches); err != nil {
		log.Error().Msgf("Error decoding match IDs: %v", err)
		return nil, err
	}

	// Convert to a slice of strings
	matchIDs := make([]string, len(matches))
	for i, match := range matches {
		matchIDs[i] = match.MatchID
	}

	return matchIDs, nil
}

// GetMatchesByType returns a list of matches of a specific type
func (m *mongoDB) GetMatchesByType(ctx context.Context, matchType string, limit int) ([]model.Match, error) {
	filter := bson.M{"match_type": matchType}

	findOptions := options.Find().SetLimit(int64(limit))

	cursor, err := m.matchesCol.Find(ctx, filter, findOptions)
	if err != nil {
		log.Error().Msgf("Error retrieving matches by type: %v", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	var matches []model.Match
	if err = cursor.All(ctx, &matches); err != nil {
		log.Error().Msgf("Error decoding matches: %v", err)
		return nil, err
	}

	return matches, nil
}

// MarkMatchAsProcessed marks a match as processed
func (m *mongoDB) MarkMatchAsProcessed(ctx context.Context, matchID string) error {
	filter := bson.M{"match_id": matchID}

	update := bson.M{
		"$set": bson.M{
			"processed":    true,
			"processed_at": time.Now(),
		},
	}

	_, err := m.matchesCol.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Msgf("Error marking match as processed: %v", err)
		return err
	}

	return nil
}

// Example implementation - you would add this to your database implementation file
func (m *mongoDB) BulkImportMatches(ctx context.Context, matches []model.Match) (model.BulkImportResult, error) {
	if len(matches) == 0 {
		return model.BulkImportResult{}, nil
	}

	// Create bulk write operation
	var operations []mongo.WriteModel

	for _, match := range matches {
		// Use upsert with filter on matchID and shardID
		filter := bson.M{
			"match_id": match.MatchID,
			"shard_id": match.ShardID,
		}

		// Create upsert model
		update := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(bson.M{"$setOnInsert": match}).
			SetUpsert(true)

		operations = append(operations, update)
	}

	// Execute bulk write
	result, err := m.matchesCol.BulkWrite(ctx, operations)
	if err != nil {
		return model.BulkImportResult{}, err
	}

	// Return results
	return model.BulkImportResult{
		SuccessCount:   int(result.UpsertedCount),
		DuplicateCount: int(result.MatchedCount),
		FailureCount:   0, // BulkWrite doesn't track individual failures this way
	}, nil
}

func (m *mongoDB) GetUnProcessedMatches(ctx context.Context, minDuration int) ([]model.Match, error) {
	// Define filter for unprocessed matches with minimum duration
	filter := bson.M{
		"processed": bson.M{"$ne": true},
		"duration":  bson.M{"$gt": minDuration},
	}

	// Execute the find operation
	cursor, err := m.matchesCol.Find(ctx, filter)
	if err != nil {
		log.Error().Msgf("Error retrieving unprocessed matches: %v", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	// Decode results into match objects
	var matches []model.Match
	if err = cursor.All(ctx, &matches); err != nil {
		log.Error().Msgf("Error decoding matches: %v", err)
		return nil, err
	}

	return matches, nil
}

// UpdateMatchesWithTelemetryData updates multiple matches with processed telemetry data in a batch
func (m *mongoDB) UpdateMatchesWithTelemetryData(ctx context.Context, updates map[string]*model.TelemetryData) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}

	// Create a slice of write models for bulk operation
	var models []mongo.WriteModel

	for matchID, telemetryData := range updates {
		// Create filter for this match
		filter := bson.M{"match_id": matchID}

		// Create update model for this match
		update := bson.M{
			"$set": bson.M{
				"telemetry_data": telemetryData,
				"processed":      true,
				"processed_at":   time.Now(),
			},
		}

		// Create an UpdateOneModel
		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update)

		models = append(models, model)
	}

	// Set options for bulk write
	opts := options.BulkWrite().SetOrdered(false)

	// Execute bulk write operation
	result, err := m.matchesCol.BulkWrite(ctx, models, opts)
	if err != nil {
		log.Error().Msgf("Failed to bulk update matches with telemetry data: %v", err)
		return 0, err
	}

	return int(result.ModifiedCount), nil
}

// GetMatchesByFilters retrieves matches based on map name, match types, and date range
func (m *mongoDB) GetMatchesByFilters(ctx context.Context, mapName string, matchTypes []string, startDate *time.Time, endDate *time.Time, limit int) ([]model.Match, error) {
	// Start with an empty filter
	filter := bson.M{
		"processed": true, // Only return processed matches
	}

	// Add map name filter if provided
	if mapName != "" {
		filter["map_name"] = mapName
	}

	// Add match type filter if provided - handle multiple match types
	if len(matchTypes) > 0 {
		if len(matchTypes) == 1 {
			filter["match_type"] = matchTypes[0]
		} else {
			filter["match_type"] = bson.M{"$in": matchTypes}
		}
	}

	// Add created_at date range filter if either start or end date is provided
	if startDate != nil || endDate != nil {
		dateFilter := bson.M{}

		if startDate != nil {
			dateFilter["$gte"] = *startDate
		}

		if endDate != nil {
			dateFilter["$lte"] = *endDate
		}

		if len(dateFilter) > 0 {
			filter["created_at"] = dateFilter
		}
	}

	// Configure find options
	findOptions := options.Find()
	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	// Log the filter being used (helpful for debugging)
	log.Debug().Interface("filter", filter).Int("limit", limit).Msg("Querying matches with filters")

	// Execute the query
	cursor, err := m.matchesCol.Find(ctx, filter, findOptions)
	if err != nil {
		log.Error().Err(err).Msg("Error retrieving matches by filters")
		return nil, err
	}
	defer cursor.Close(ctx)

	// Decode the results
	var matches []model.Match
	if err = cursor.All(ctx, &matches); err != nil {
		log.Error().Err(err).Msg("Error decoding matches")
		return nil, err
	}

	log.Debug().Int("match_count", len(matches)).Msg("Retrieved filtered matches")
	return matches, nil
}

// GetMatchByID retrieves a match by its match ID
func (m *mongoDB) GetMatchByID(ctx context.Context, matchID string) (*model.Match, error) {
	// Create filter for this match
	filter := bson.M{"match_id": matchID}

	// Execute the find operation
	var match model.Match
	err := m.matchesCol.FindOne(ctx, filter).Decode(&match)

	// Check if the error is "no documents found"
	if err == mongo.ErrNoDocuments {
		// Return nil match and nil error to indicate match doesn't exist
		return nil, nil
	}

	// If there was another error, return it
	if err != nil {
		log.Error().Msgf("Error retrieving match by ID: %v", err)
		return nil, err
	}

	// Match was found, return it
	return &match, nil
}

// AddOrUpdateTeamRotation adds a team's rotation data to a match or updates it if it already exists
func (m *mongoDB) AddOrUpdateTeamRotation(ctx context.Context, matchID string, rotation model.TeamRotation) error {
	// Create filter for this match
	filter := bson.M{"match_id": matchID}

	// First check if this team already has a rotation in this match
	teamFilter := bson.M{
		"match_id":               matchID,
		"team_rotations.team_id": rotation.TeamID,
	}

	// Check if the team rotation already exists
	count, err := m.matchesCol.CountDocuments(ctx, teamFilter)
	if err != nil {
		log.Error().Msgf("Error checking team rotation existence: %v", err)
		return err
	}

	var result *mongo.UpdateResult

	if count > 0 {
		// Update existing team rotation
		update := bson.M{
			"$set": bson.M{
				"team_rotations.$[elem]": rotation,
			},
		}

		// Set up array filter to match the team ID
		arrayFilters := options.ArrayFilters{
			Filters: []interface{}{
				bson.M{"elem.team_id": rotation.TeamID},
			},
		}

		opts := options.Update().SetArrayFilters(arrayFilters)

		// Execute the update
		result, err = m.matchesCol.UpdateOne(ctx, filter, update, opts)
	} else {
		// Add new team rotation
		update := bson.M{
			"$push": bson.M{
				"team_rotations": rotation,
			},
		}

		// Execute the update
		result, err = m.matchesCol.UpdateOne(ctx, filter, update)
	}

	if err != nil {
		log.Error().Msgf("Error adding/updating team rotation: %v", err)
		return err
	}

	// Check if the match was found
	if result.MatchedCount == 0 {
		return fmt.Errorf("match with ID %s not found", matchID)
	}

	return nil
}
