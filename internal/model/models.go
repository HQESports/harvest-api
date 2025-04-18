package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TODO: Ensure match get type uses these constants

const (
	// Match Types
	LIVE_SCRIM  = "LIVE_SCRIM"
	RANKED_GAME = "RANKED"
	EVENT_MATCH = "EVENT"

	// Job Status
	RUNNING   = "RUNNING"
	COMPLETED = "COMPLETED"
	FAILED    = "FAILED"
	CACNCELED = "CANCELED"

	// Job Types
	BUILD_MATCHES  = "BUILD_MATCHES"
	EXPAND_PLAYERS = "EXPAND_PLAYERS"
)

// Entity represents a common structure for players and tournaments
type Entity struct {
	ID        string    `json:"id" bson:"id"`
	Name      string    `json:"name" bson:"name"`
	Active    bool      `json:"active" bson:"active"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type Match struct {
	MatchID       string    `bson:"match_id"`
	ShardID       string    `bson:"shard_id"`        // From shardId in API
	MapName       string    `bson:"map_name"`        // From mapName in API
	GameMode      string    `bson:"game_mode"`       // From gameMode in API
	Duration      int       `bson:"duration"`        // Match duration in seconds
	IsCustomMatch bool      `bson:"is_custom_match"` // From isCustomMatch in API
	CreatedAt     time.Time `bson:"created_at"`      // From createdAt in API
	MatchType     string    `bson:"match_type"`      // Not from API either [LIVE_SCRIM, RANKED, EVENT]

	// Processing metadata
	Processed   bool      `bson:"processed"`    // Whether this match has been processed
	ProcessedAt time.Time `bson:"processed_at"` // When the match was processed
	ImportedAt  time.Time `bson:"imported_at"`  // When the match was imported to DB

	// Statistics and countsx
	PlayerCount int `bson:"player_count"` // Number of participants
	TeamCount   int `bson:"team_count"`   // Number of rosters/teams

	TelemetryURL string `bson:"telemetry_url"` // URL to telemetry data
}

// APIToken represents a service authentication token
type APIToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TokenHash string             `bson:"token_hash" json:"-" unique:"true"` // Hashed token value stored in DB
	Name      string             `bson:"name" json:"name" unique:"true"`    // Name/description of the token
	Role      string             `bson:"role" json:"role"`                  // Either "admin" or "service"
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	ExpiresAt time.Time          `bson:"expires_at,omitempty" json:"expires_at,omitempty"` // Optional expiration
	LastUsed  time.Time          `bson:"last_used,omitempty" json:"last_used,omitempty"`
	Revoked   bool               `bson:"revoked" json:"revoked"` // Whether the token has been revoked
}

// Add to your model package
type BulkImportResult struct {
	SuccessCount   int
	DuplicateCount int
	FailureCount   int
}
