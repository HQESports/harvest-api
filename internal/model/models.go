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

	// Statistics and counts
	PlayerCount int `bson:"player_count"` // Number of participants
	TeamCount   int `bson:"team_count"`   // Number of rosters/teams

	TelemetryURL string `bson:"telemetry_url"` // URL to telemetry data

	// Telemetry data organized under an umbrella field
	TelemetryData *TelemetryData `bson:"telemetry_data,omitempty"` // All extracted telemetry data
}

// TelemetryData contains all extracted data from telemetry
type TelemetryData struct {
	SafeZones []SafeZone `bson:"safe_zones,omitempty"` // Array of safety zones, starting with phase 1
	PlanePath PlanePath  `bson:"plane_path,omitempty"` // Plane path coordinates
}

// SafeZone represents data for a single circle phase
type SafeZone struct {
	Phase  int     `bson:"phase"`  // Phase number (1-indexed)
	X      float64 `bson:"x"`      // X coordinate of center
	Y      float64 `bson:"y"`      // Y coordinate of center
	Radius float64 `bson:"radius"` // Circle radius
}

// PlanePath represents the airplane trajectory
type PlanePath struct {
	StartX float64 `bson:"start_x"` // X coordinate of start point
	StartY float64 `bson:"start_y"` // Y coordinate of start point
	EndX   float64 `bson:"end_x"`   // X coordinate of end point
	EndY   float64 `bson:"end_y"`   // Y coordinate of end point
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

type Organization struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	ImageURL  string             `bson:"image_url" json:"imageUrl"`
	CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updatedAt"`
}
