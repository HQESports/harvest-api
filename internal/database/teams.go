package database

import (
	"context"
	"harvest/internal/model"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TeamDatabase defines team-related database operations
type TeamDatabase interface {
	// Create a new team
	CreateTeam(ctx context.Context, team *model.Team) error

	// Get a team by ID
	GetTeamByID(ctx context.Context, id primitive.ObjectID) (*model.Team, error)

	// Update a team
	UpdateTeam(ctx context.Context, team *model.Team) error

	// List all teams
	ListTeams(ctx context.Context) ([]model.Team, error)

	// Delete a team by ID
	DeleteTeam(ctx context.Context, id primitive.ObjectID) error
}

// CreateTeam creates a new team in the database
func (m *mongoDB) CreateTeam(ctx context.Context, team *model.Team) error {
	// Ensure the team has a valid ID
	if team.ID.IsZero() {
		team.ID = primitive.NewObjectID()
	}

	// Insert the team
	_, err := m.teamsCol.InsertOne(ctx, team)
	if err != nil {
		log.Error().Err(err).Str("teamID", team.ID.Hex()).Msg("Failed to create team")
		return err
	}

	log.Debug().Str("teamID", team.ID.Hex()).Str("name", team.Name).Msg("Created new team")
	return nil
}

// GetTeamByID retrieves a team from the database by its ID
func (m *mongoDB) GetTeamByID(ctx context.Context, id primitive.ObjectID) (*model.Team, error) {
	var team model.Team

	filter := bson.M{"_id": id}
	err := m.teamsCol.FindOne(ctx, filter).Decode(&team)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Debug().Str("teamID", id.Hex()).Msg("Team not found")
			return nil, nil
		}
		log.Error().Err(err).Str("teamID", id.Hex()).Msg("Failed to get team")
		return nil, err
	}

	log.Debug().Str("teamID", id.Hex()).Msg("Retrieved team")
	return &team, nil
}

// UpdateTeam updates an existing team in the database
func (m *mongoDB) UpdateTeam(ctx context.Context, team *model.Team) error {
	// Ensure we have an ID
	if team.ID.IsZero() {
		log.Error().Msg("Cannot update team with zero ID")
		return mongo.ErrNoDocuments
	}

	filter := bson.M{"_id": team.ID}
	update := bson.M{"$set": team}

	result, err := m.teamsCol.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Str("teamID", team.ID.Hex()).Msg("Failed to update team")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().Str("teamID", team.ID.Hex()).Msg("Team not found for update")
		return mongo.ErrNoDocuments
	}

	log.Debug().Str("teamID", team.ID.Hex()).Str("name", team.Name).Msg("Updated team")
	return nil
}

// ListTeams retrieves all teams from the database, sorted by name
func (m *mongoDB) ListTeams(ctx context.Context) ([]model.Team, error) {
	// Create options to sort by name in ascending order
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})

	cursor, err := m.teamsCol.Find(ctx, bson.M{}, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list teams")
		return nil, err
	}
	defer cursor.Close(ctx)

	var teams []model.Team
	if err = cursor.All(ctx, &teams); err != nil {
		log.Error().Err(err).Msg("Failed to decode teams")
		return nil, err
	}

	log.Debug().Int("count", len(teams)).Msg("Retrieved teams list")
	return teams, nil
}

// DeleteTeam deletes a team from the database by its ID
func (m *mongoDB) DeleteTeam(ctx context.Context, id primitive.ObjectID) error {
	result, err := m.teamsCol.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		log.Error().Err(err).Str("teamID", id.Hex()).Msg("Failed to delete team")
		return err
	}

	if result.DeletedCount == 0 {
		log.Debug().Str("teamID", id.Hex()).Msg("Team not found for deletion")
		return mongo.ErrNoDocuments
	}

	log.Debug().Str("teamID", id.Hex()).Msg("Deleted team")
	return nil
}
