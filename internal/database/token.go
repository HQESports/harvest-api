package database

import (
	"context"
	"fmt"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TokenDatabase interface {
	CreateToken(context.Context, *model.APIToken) error
	VerifyToken(context.Context, string) (*model.APIToken, error)
	ListTokens(context.Context) ([]model.APIToken, error)
	RevokeToken(context.Context, primitive.ObjectID) error
	GetTokenByID(context.Context, primitive.ObjectID) (*model.APIToken, error)
	GetTokenByName(context.Context, string) (*model.APIToken, error)
}

// Create a new API token
func (m *mongoDB) CreateToken(ctx context.Context, token *model.APIToken) error {
	_, err := m.tokensCol.InsertOne(ctx, token)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			log.Error().Str("Name", token.Name).Msgf("Duplicate token detected")
			return fmt.Errorf("Duplicate token detected")
		}

		log.Error().Msgf("Failed to create token: %v", err)
		return err
	}
	return nil
}

// Verify an API token by its hash
func (m *mongoDB) VerifyToken(ctx context.Context, tokenHash string) (*model.APIToken, error) {
	filter := bson.M{
		"token_hash": tokenHash,
		"revoked":    false,
		"$or": []bson.M{
			{"expires_at": bson.M{"$exists": false}},
			{"expires_at": bson.M{"$gt": time.Now()}},
		},
	}

	var token model.APIToken
	err := m.tokensCol.FindOne(ctx, filter).Decode(&token)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("invalid or expired token")
		}
		log.Error().Msgf("Error verifying token: %v", err)
		return nil, err
	}

	// Update last used time
	update := bson.M{"$set": bson.M{"last_used": time.Now()}}
	_, err = m.tokensCol.UpdateOne(ctx, bson.M{"_id": token.ID}, update)
	if err != nil {
		log.Warn().Err(err).Msg("Error updating token last used time")
		// Non-critical error, so we don't return it
	}

	return &token, nil
}

// List all tokens
func (m *mongoDB) ListTokens(ctx context.Context) ([]model.APIToken, error) {
	var tokens []model.APIToken

	findOptions := options.Find()
	findOptions.SetSort(bson.M{"created_at": -1})

	cursor, err := m.tokensCol.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		log.Error().Msgf("Error retrieving tokens: %v", err)
		return nil, err
	}
	defer cursor.Close(ctx)

	if err = cursor.All(ctx, &tokens); err != nil {
		log.Error().Msgf("Error decoding tokens: %v", err)
		return nil, err
	}

	return tokens, nil
}

// Revoke a token by ID
func (m *mongoDB) RevokeToken(ctx context.Context, tokenID primitive.ObjectID) error {
	filter := bson.M{"_id": tokenID}
	update := bson.M{"$set": bson.M{"revoked": true}}

	_, err := m.tokensCol.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Msgf("Error revoking token: %v", err)
		return err
	}

	return nil
}

// Get a token by ID
func (m *mongoDB) GetTokenByID(ctx context.Context, tokenID primitive.ObjectID) (*model.APIToken, error) {
	var token model.APIToken

	err := m.tokensCol.FindOne(ctx, bson.M{"_id": tokenID}).Decode(&token)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("token not found")
		}
		log.Error().Msgf("Error retrieving token: %v", err)
		return nil, err
	}

	return &token, nil
}

func (m *mongoDB) GetTokenByName(ctx context.Context, name string) (*model.APIToken, error) {
	var token model.APIToken

	err := m.tokensCol.FindOne(ctx, bson.M{"name": name}).Decode(&token)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("token not found")
		}

		log.Error().Err(err).Msg("Error retrieving token")
		return nil, err
	}

	return &token, nil
}
