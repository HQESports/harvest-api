package database

import (
	"context"
	"harvest/internal/config"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database interface {
	Health() error
	PubgDatabase
	TokenDatabase
	JobDatabase
	MatchMetricsDatabase
}

type mongoDB struct {
	client *mongo.Client
	db     *mongo.Database

	trainCol       *mongo.Collection
	playersCol     *mongo.Collection
	tournamentsCol *mongo.Collection
	matchesCol     *mongo.Collection
	tokensCol      *mongo.Collection
	jobsCol        *mongo.Collection
}

func New(config *config.Config) (Database, error) {
	clientOptions := options.Client().ApplyURI(config.MongoDB.URI)
	clientOptions.SetAuth(options.Credential{
		Username: config.MongoDB.Username,
		Password: config.MongoDB.Password,
	})

	client, err := mongo.Connect(context.TODO(), clientOptions)

	db := client.Database(config.MongoDB.DB)

	if err != nil {
		return nil, err
	}

	tokensCol := db.Collection("tokens")
	// Create unique indexes on the tokens collection
	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "token_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	jobsCol := db.Collection("jobs")
	// Create indexes for jobs collection
	jobIndexModels := []mongo.IndexModel{
		{
			// Index for status-based queries
			Keys:    bson.D{{Key: "status", Value: 1}},
			Options: options.Index(),
		},
		{
			// Index for user-based queries
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index(),
		},
		{
			// Compound index for status + user queries
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "user_id", Value: 1}},
			Options: options.Index(),
		},
		{
			// Index for job type queries
			Keys:    bson.D{{Key: "type", Value: 1}},
			Options: options.Index(),
		},
		{
			// Index for sorting by creation date
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index(),
		},
		{
			// TTL index to auto-delete old completed/failed jobs after 30 days
			Keys:    bson.D{{Key: "completed_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(60 * 60 * 24 * 30 * 6),
		},
	}

	_, err = tokensCol.Indexes().CreateMany(context.Background(), indexModels)

	if err != nil {
		log.Warn().Err(err).Str("Collection", "Tokens").Msg("Error creating indexes")
	}

	_, err = jobsCol.Indexes().CreateMany(context.Background(), jobIndexModels)
	if err != nil {
		log.Warn().Err(err).Str("Collection", "Jobs").Msg("Error creating indexes")
	}

	return &mongoDB{
		client:         client,
		db:             db,
		trainCol:       db.Collection("erangel_train"),
		playersCol:     db.Collection("players"),
		tournamentsCol: db.Collection("tournaments"),
		matchesCol:     db.Collection("matches"),
		jobsCol:        jobsCol,
		tokensCol:      tokensCol,
	}, nil
}

// Health implements Database interface
func (m *mongoDB) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := m.client.Ping(ctx, nil)

	if err != nil {
		log.Error().Msgf("Database health error: %v", err)
		return err
	}

	return nil
}
