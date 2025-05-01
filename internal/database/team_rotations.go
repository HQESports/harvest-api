package database

import (
	"context"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// TeamRotationDatabase defines team rotation related database operations
type TeamRotationDatabase interface {
	// Get all rotations for a specific team
	GetTeamRotations(ctx context.Context, teamID string, startDate time.Time, endDate time.Time) ([]model.TeamRotationTiny, error)

	// Get a specific rotation by ID
	GetRotationByID(ctx context.Context, id primitive.ObjectID) (*model.TeamRotation, error)

	// Bulk create team rotations
	BulkCreateTeamRotations(ctx context.Context, rotations []model.TeamRotation) (*mongo.BulkWriteResult, error)
}

// GetTeamRotations retrieves all rotations for a specific team with populated match data
func (r *mongoDB) GetTeamRotations(ctx context.Context, teamID string, startDate time.Time, endDate time.Time) ([]model.TeamRotationTiny, error) {
	objID, err := primitive.ObjectIDFromHex(teamID)
	if err != nil {
		log.Error().Err(err).Str("teamID", teamID).Msg("Invalid team ID format")
		return nil, err
	}

	// If start and end dates are provided, add a date range filter
	matchStage := bson.D{{Key: "$match", Value: bson.M{
		"team_id": objID,
		"match.created_at": bson.M{
			"$gte": startDate, // Greater than or equal to start date
			"$lte": endDate,   // Less than or equal to end date
		},
	}}}

	// Create an aggregation pipeline to populate match data
	pipeline := mongo.Pipeline{

		// Lookup match data from the matches collection
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "matches",
			"localField":   "match_id",
			"foreignField": "match_id",
			"as":           "match_data",
		}}},
		// Unwind the match array to get a single match object
		bson.D{{Key: "$unwind", Value: bson.M{
			"path":                       "$match_data",
			"preserveNullAndEmptyArrays": true,
		}}},
		// Add the match field to the document
		bson.D{{Key: "$addFields", Value: bson.M{
			"match": "$match_data",
		}}},
		// Match documents for the specified team
		matchStage,
		// Remove the temporary match_data field
		bson.D{{Key: "$project", Value: bson.M{
			"match_data": 0,
		}}},
		// Sort by match created_at date in descending order
		bson.D{{Key: "$sort", Value: bson.D{{Key: "match.created_at", Value: -1}}}},
	}

	cursor, err := r.teamRotationsCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Str("teamID", teamID).Msg("Failed to get team rotations")
		return nil, err
	}
	defer cursor.Close(ctx)

	var rotations []model.TeamRotationTiny
	if err = cursor.All(ctx, &rotations); err != nil {
		log.Error().Err(err).Str("teamID", teamID).Msg("Failed to decode team rotations")
		return nil, err
	}

	log.Debug().Str("teamID", teamID).Int("count", len(rotations)).Msg("Retrieved team rotations")
	return rotations, nil
}

// GetRotationByID retrieves a specific rotation by its ID with populated match data
func (r *mongoDB) GetRotationByID(ctx context.Context, id primitive.ObjectID) (*model.TeamRotation, error) {
	// Create an aggregation pipeline
	pipeline := mongo.Pipeline{
		// Match the specific rotation document
		bson.D{{Key: "$match", Value: bson.M{"_id": id}}},
		// Lookup match data from the matches collection
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "matches",
			"localField":   "match_id",
			"foreignField": "match_id",
			"as":           "match_data",
		}}},
		// Unwind the match array to get a single match object
		bson.D{{Key: "$unwind", Value: bson.M{
			"path":                       "$match_data",
			"preserveNullAndEmptyArrays": true,
		}}},
		// Add the match field to the document
		bson.D{{Key: "$addFields", Value: bson.M{
			"match": "$match_data",
		}}},
		// Remove the temporary match_data field
		bson.D{{Key: "$project", Value: bson.M{
			"match_data": 0,
		}}},
	}

	cursor, err := r.teamRotationsCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Str("rotationID", id.Hex()).Msg("Failed to get team rotation")
		return nil, err
	}
	defer cursor.Close(ctx)

	var rotations []model.TeamRotation
	if err = cursor.All(ctx, &rotations); err != nil {
		log.Error().Err(err).Str("rotationID", id.Hex()).Msg("Failed to decode team rotation")
		return nil, err
	}

	if len(rotations) == 0 {
		log.Debug().Str("rotationID", id.Hex()).Msg("Team rotation not found")
		return nil, nil
	}

	log.Debug().Str("rotationID", id.Hex()).Msg("Retrieved team rotation")
	return &rotations[0], nil
}

// BulkCreateTeamRotations creates multiple team rotations in the database
func (r *mongoDB) BulkCreateTeamRotations(ctx context.Context, rotations []model.TeamRotation) (*mongo.BulkWriteResult, error) {
	if len(rotations) == 0 {
		log.Debug().Msg("No rotations to create")
		return nil, nil
	}

	now := time.Now()
	bulkOperations := make([]mongo.WriteModel, len(rotations))

	// Prepare bulk operations for each rotation
	for i := range rotations {
		// Set creation timestamp for new documents
		rotations[i].CreatedAt = now
		// For updates, set the updated timestamp
		rotations[i].UpdatedAt = now

		// Create filter by match ID
		filter := bson.M{"match_id": rotations[i].MatchID}

		// Prepare the update document
		update := bson.M{
			"$set": rotations[i],
		}

		// Create an upsert operation
		upsertOp := mongo.NewUpdateOneModel()
		upsertOp.SetFilter(filter)
		upsertOp.SetUpdate(update)
		upsertOp.SetUpsert(true)

		bulkOperations[i] = upsertOp
	}

	// Execute the bulk write operation
	result, err := r.teamRotationsCol.BulkWrite(ctx, bulkOperations)
	if err != nil {
		log.Error().Err(err).Int("count", len(rotations)).Msg("Failed to bulk upsert team rotations")
		return nil, err
	}

	log.Debug().
		Int64("inserted", result.InsertedCount).
		Int64("modified", result.ModifiedCount).
		Int64("upserted", result.UpsertedCount).
		Msg("Bulk upserted team rotations")

	return result, nil
}
