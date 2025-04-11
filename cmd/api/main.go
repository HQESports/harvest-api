package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"harvest/internal/config"
	"harvest/internal/model"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Configure logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log := zerolog.New(os.Stderr).With().Timestamp().Logger()

	if len(os.Args) < 4 {
		fmt.Println("Usage: create_token <config_path> <token_name> <expires_in_days>")
		fmt.Println("Example: create_token config.json \"Initial Admin Token\" 365")
		os.Exit(1)
	}

	configPath := os.Args[1]
	tokenName := os.Args[2]
	expiresInDays, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Fatal().Msgf("Invalid expires_in_days value: %v", err)
	}

	// Load configuration
	config, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Msgf("Failed to load configuration: %v", err)
	}

	// Connect to MongoDB using the same pattern as in mongo.go
	clientOptions := options.Client().ApplyURI(config.MongoDB.URI)
	clientOptions.SetAuth(options.Credential{
		Username: config.MongoDB.Username,
		Password: config.MongoDB.Password,
	})

	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal().Msgf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(context.TODO())

	// Test the connection
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal().Msgf("Failed to ping MongoDB: %v", err)
	}

	db := client.Database(config.MongoDB.DB)
	apiTokensCol := db.Collection("api_tokens")

	// Generate a secure random token
	b := make([]byte, 32) // 256 bits of randomness
	_, err = rand.Read(b)
	if err != nil {
		log.Fatal().Msgf("Failed to generate random token: %v", err)
	}
	rawToken := base64.URLEncoding.EncodeToString(b)

	// Hash the token for storage
	h := sha256.New()
	h.Write([]byte(rawToken))
	tokenHash := hex.EncodeToString(h.Sum(nil))

	// Create token document
	token := model.APIToken{
		ID:        primitive.NewObjectID(),
		TokenHash: tokenHash,
		Name:      tokenName,
		Role:      model.RoleAdmin, // Create as admin token
		CreatedAt: time.Now(),
		Revoked:   false,
	}

	// Set expiration if provided
	if expiresInDays > 0 {
		token.ExpiresAt = time.Now().AddDate(0, 0, expiresInDays)
	}

	// Insert token into database
	_, err = apiTokensCol.InsertOne(context.TODO(), token)
	if err != nil {
		log.Fatal().Msgf("Failed to insert token: %v", err)
	}

	fmt.Println("Admin token created successfully!")
	fmt.Println("Token:", rawToken)
	fmt.Println("IMPORTANT: Save this token securely. It won't be shown again.")
}
