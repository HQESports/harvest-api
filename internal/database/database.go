package database

import (
	"context"
	"harvest/internal/config"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database interface {
	Health() error
	PubgDatabase
	TokenDatabase
	JobDatabase
	MatchMetricsDatabase
	TeamDatabase
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
	teamsCol       *mongo.Collection

	organizationsCol *mongo.Collection
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
	jobsCol := db.Collection("jobs")

	return &mongoDB{
		client:           client,
		db:               db,
		trainCol:         db.Collection("erangel_train"),
		playersCol:       db.Collection("players"),
		tournamentsCol:   db.Collection("tournaments"),
		matchesCol:       db.Collection("matches"),
		organizationsCol: db.Collection("organizations"),
		teamsCol:         db.Collection("teams"),
		jobsCol:          jobsCol,
		tokensCol:        tokensCol,
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
