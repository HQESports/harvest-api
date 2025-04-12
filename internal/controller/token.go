package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Role constants
const (
	RoleAdmin   = "ADMIN"   // Can manage tokens and access all API endpoints
	RoleService = "SERVICE" // Can only access API endpoints, not token management
)

// TokenController defines the contract for token operations
type TokenController interface {
	// GenerateToken creates a new token with the specified parameters
	GenerateToken(context.Context, string, string, *time.Time) (string, *model.APIToken, error)

	// VerifyToken checks if a token is valid and returns its details
	VerifyToken(context.Context, string) (*model.APIToken, error)

	// ListTokens retrieves all tokens from the database
	ListTokens(context.Context) ([]model.APIToken, error)

	// RevokeToken disables a token by ID
	RevokeToken(context.Context, string) error

	// GetTokenByID retrieves a token by its ID
	GetTokenByID(context.Context, string) (*model.APIToken, error)

	// GenerateInitialAdminToken creates the first admin token in the system
	GenerateInitialAdminToken(context.Context, string) (string, error)
}

// tokenController handles token operations
type tokenController struct {
	db database.Database
}

// NewTokenService creates a new token service
func NewToken(db database.Database) TokenController {
	return &tokenController{
		db: db,
	}
}

// GenerateToken creates a new secure token
func (s *tokenController) GenerateToken(ctx context.Context, name string, role string, expiresAt *time.Time) (string, *model.APIToken, error) {
	// Try up to 3 times to generate a unique token
	for attempts := 0; attempts < 3; attempts++ {
		// Generate 32 bytes of random data (will give us 64 hex characters)
		rawToken := make([]byte, 32)
		_, err := rand.Read(rawToken)
		if err != nil {
			return "", nil, fmt.Errorf("failed to generate random token: %w", err)
		}

		// Convert to hex string for user readability (64 characters)
		tokenString := hex.EncodeToString(rawToken)

		// Hash the token for storage
		hasher := sha256.New()
		hasher.Write([]byte(tokenString))
		tokenHash := hex.EncodeToString(hasher.Sum(nil))

		// Create token record
		now := time.Now()
		token := &model.APIToken{
			ID:        primitive.NewObjectID(),
			TokenHash: tokenHash,
			Name:      name,
			Role:      role,
			CreatedAt: now,
			LastUsed:  now,
			Revoked:   false,
		}

		// Set expiration if provided
		if expiresAt != nil {
			token.ExpiresAt = *expiresAt
		}

		// Try to create the token in the database
		err = s.db.CreateToken(ctx, token)
		if err != nil {
			// If it's a uniqueness error, try again
			if mongo.IsDuplicateKeyError(err) {
				log.Warn().Msg("Token hash collision detected, retrying generation")
				continue
			}
			// For other errors, return immediately
			return "", nil, err
		}

		// If successful, return the token
		return tokenString, token, nil
	}

	// If we've reached here, we couldn't generate a unique token after multiple attempts
	return "", nil, fmt.Errorf("failed to generate a unique token after multiple attempts")
}

// VerifyToken verifies if a token is valid
func (s *tokenController) VerifyToken(ctx context.Context, tokenString string) (*model.APIToken, error) {
	// Hash the provided token
	hasher := sha256.New()
	hasher.Write([]byte(tokenString))
	tokenHash := hex.EncodeToString(hasher.Sum(nil))

	// Verify token in database
	return s.db.VerifyToken(ctx, tokenHash)
}

// ListTokens lists all tokens
func (s *tokenController) ListTokens(ctx context.Context) ([]model.APIToken, error) {
	return s.db.ListTokens(ctx)
}

// RevokeToken revokes a token by ID
func (s *tokenController) RevokeToken(ctx context.Context, tokenID string) error {
	// Convert string ID to ObjectID
	id, err := primitive.ObjectIDFromHex(tokenID)
	if err != nil {
		return fmt.Errorf("invalid token ID format: %w", err)
	}

	return s.db.RevokeToken(ctx, id)
}

// GetTokenByID gets a token by ID
func (s *tokenController) GetTokenByID(ctx context.Context, tokenID string) (*model.APIToken, error) {
	// Convert string ID to ObjectID
	id, err := primitive.ObjectIDFromHex(tokenID)
	if err != nil {
		return nil, fmt.Errorf("invalid token ID format: %w", err)
	}

	return s.db.GetTokenByID(ctx, id)
}

// GenerateInitialAdminToken creates the first admin token in the system
// This should only be used during system initialization
func (s *tokenController) GenerateInitialAdminToken(ctx context.Context, appName string) (string, error) {
	// Set expiration for 1 year from now
	expiresAt := time.Now().AddDate(1, 0, 0)

	tokenString, token, err := s.GenerateToken(ctx, fmt.Sprintf("%s - Admin Token", appName), RoleAdmin, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to generate initial admin token: %w", err)
	}

	log.Info().
		Str("tokenID", token.ID.Hex()).
		Str("name", token.Name).
		Time("expiresAt", token.ExpiresAt).
		Msg("Initial admin token created")

	return tokenString, nil
}
