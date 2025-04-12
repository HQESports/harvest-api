package database

import (
	"context"
	"fmt"
	"harvest/internal/config"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database interface {
	Health() (string, error)
	PubgDatabase
	TokenDatabase
}

type mongoDB struct {
	client *mongo.Client
	db     *mongo.Database

	trainCol       *mongo.Collection
	playersCol     *mongo.Collection
	tournamentsCol *mongo.Collection
	matchesCol     *mongo.Collection
	tokensCol      *mongo.Collection
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

	_, err = tokensCol.Indexes().CreateMany(context.Background(), indexModels)

	if err != nil {
		return nil, fmt.Errorf("failed to create token indexes: %w", err)
	}

	return &mongoDB{
		client:         client,
		db:             db,
		trainCol:       db.Collection("erangel_train"),
		playersCol:     db.Collection("players"),
		tournamentsCol: db.Collection("tournaments"),
		matchesCol:     db.Collection("matches"),
		tokensCol:      tokensCol,
	}, nil
}

// Health implements Database interface
func (m *mongoDB) Health() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := m.client.Ping(ctx, nil)

	if err != nil {
		log.Error().Msgf("Database health error: %v", err)
		return "Database Offline", err
	}

	return "Database Online", nil
}
