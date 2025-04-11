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

	// Role Types for Authentication Tokenss
	RoleAdmin   = "ADMIN"   // Can manage tokens and access all API endpoints
	RoleService = "SERVICE" // Can only access API endpoints, not token management
)

// Entity represents a common structure for players and tournaments
type Entity struct {
	ID     string `json:"id" bson:"id"`
	Name   string `json:"name" bson:"name"`
	Active bool   `json:"active" bson:"active"`
}

type Match struct {
	MatchID       string    `bson:"match_id,omitempty"`
	ShardID       string    `bson:"shard_id,omitempty"`        // From shardId in API
	MapName       string    `bson:"map_name,omitempty"`        // From mapName in API
	GameMode      string    `bson:"game_mode,omitempty"`       // From gameMode in API
	Duration      int       `bson:"duration,omitempty"`        // Match duration in seconds
	IsCustomMatch bool      `bson:"is_custom_match,omitempty"` // From isCustomMatch in API
	CreatedAt     time.Time `bson:"created_at,omitempty"`      // From createdAt in API
	MatchType     string    `bson:"match_type,omitempty"`      // Not from API either [LIVE_SCRIM, RANKED, EVENT]

	// Processing metadata
	Processed   bool      `bson:"processed,omitempty"`    // Whether this match has been processed
	ProcessedAt time.Time `bson:"processed_at,omitempty"` // When the match was processed
	ImportedAt  time.Time `bson:"imported_at,omitempty"`  // When the match was imported to DB

	// Statistics and counts
	PlayerCount int `bson:"player_count,omitempty"` // Number of participants
	TeamCount   int `bson:"team_count,omitempty"`   // Number of rosters/teams

	TelemetryURL string `bson:"telemetry_url,omitempty"` // URL to telemetry data
}

type Job struct {
	ID     string `bson:"_id,omitempty"`
	Type   string `bson:"type,omitempty"`   // e.g., "match_processing"
	Status string `bson:"status,omitempty"` // "queued", "running", "completed", "failed", "canceled"

	CreatedAt   time.Time `bson:"created_at,omitempty"`
	StartedAt   time.Time `bson:"started_at,omitempty"`
	CompletedAt time.Time `bson:"completed_at,omitempty"`

	Progress JobProgress `bson:"progress,omitempty"`

	Error string `bson:"error,omitempty"`
}

type JobProgress struct {
	Total     int `bson:"total,omitempty"`
	Processed int `bson:"processed,omitempty"`
	Failed    int `bson:"failed,omitempty"`
}

// APIToken represents a service authentication token
type APIToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TokenHash string             `bson:"token_hash" json:"-"` // Hashed token value stored in DB
	Name      string             `bson:"name" json:"name"`    // Name/description of the token
	Role      string             `bson:"role" json:"role"`    // Either "admin" or "service"
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	ExpiresAt time.Time          `bson:"expires_at,omitempty" json:"expires_at,omitempty"` // Optional expiration
	LastUsed  time.Time          `bson:"last_used,omitempty" json:"last_used,omitempty"`
	Revoked   bool               `bson:"revoked" json:"revoked"` // Whether the token has been revoked
}
