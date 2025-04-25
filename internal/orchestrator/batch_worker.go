package orchestrator

import (
	"fmt"
	"harvest/internal/model"
	"harvest/pkg/pubg"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BatchWorker interface {
	// ProcessBatch processes a job and returns results
	StartWorker(*model.Job) (bool, error)

	// Cancels the job
	Cancel() error

	// Name returns the processor name
	Name() string

	// Is Active
	IsActive() bool

	// Type returns the type of the processor as a string
	Type() string

	// Description
	Description() string

	//
	ActiveJobID() *primitive.ObjectID
}

// SplitIntoBatches is a generic function that divides a slice of items
// into batches of the specified size
func SplitIntoBatches[T any](items []T, batchSize int) [][]T {
	// Handle edge cases
	if batchSize <= 0 {
		return nil
	}

	if len(items) == 0 {
		return [][]T{}
	}

	// Calculate the number of batches needed
	numBatches := (len(items) + batchSize - 1) / batchSize

	// Create the result slice with the right capacity
	batches := make([][]T, 0, numBatches)

	// Split the items into batches
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize

		// Handle the last batch which might be smaller
		if end > len(items) {
			end = len(items)
		}

		// Create a new batch with the current slice of items
		batches = append(batches, items[i:end])
	}

	return batches
}

func BuildMatchDocument(shard, matchID string, match pubg.PUBGMatchResponse) (model.Match, error) {
	createdAt, err := time.Parse(time.RFC3339, match.Data.Attributes.CreatedAt)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse match creation time")
		return model.Match{}, fmt.Errorf("failed to parse creation time: %w", err)
	}

	// Calculate player and team counts
	playerCount := 0
	teamCount := 0

	for _, obj := range match.Included {
		if obj.Type == "participant" && obj.IsValidPlayer() {
			playerCount++
		} else if obj.Type == "roster" {
			teamCount++
		}
	}

	// Get telemetry URL
	telemetryURL, err := match.GetTelemetryURL()
	if err != nil {
		log.Warn().Err(err).Msg("Could not retrieve telemetry URL")
		// Continue processing even if telemetry URL cannot be retrieved
		return model.Match{}, err
	}

	return model.Match{
		MatchID:       matchID,
		ShardID:       shard,
		MapName:       match.Data.Attributes.MapName,
		GameMode:      match.Data.Attributes.GameMode,
		Duration:      match.Data.Attributes.Duration,
		IsCustomMatch: match.Data.Attributes.IsCustomMatch,
		CreatedAt:     createdAt,
		MatchType:     match.GetMatchType(pubg.EventPlatform),

		// Processing metadata
		Processed:  false,
		ImportedAt: time.Now(),

		// Statistics and counts
		PlayerCount:  playerCount,
		TeamCount:    teamCount,
		TelemetryURL: telemetryURL,
	}, nil
}
