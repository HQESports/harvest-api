package processor

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/pkg/pubg"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	PLAYER_PROCESSOR_NAME = "PUBG Player Processor - Search and Expand Players" // Fixed typo in processor name
	PLAYER_PROCESSOR_TYPE = "player-search-and-expand"
	MAX_MATCH_BATCH       = 10 // Added constant for consistency
)

type playerProcessor struct {
	db         database.Database // Use for updating job progress as we go through batches and pulling current player IDs
	pubgClient *pubg.Client      // Used for access
	ctx        *context.Context
	cancelFunc *context.CancelFunc
}

// BulkSearchMatchForPlayers processes multiple matches in bulk to extract players
func (p *playerProcessor) BulkSearchMatchForPlayers(ctx context.Context, matchIDs []string, jobID primitive.ObjectID) StatusError {
	if len(matchIDs) == 0 {
		return NewSuccessError("No matches to process")
	}

	// Check if context is cancelled
	if ctx.Err() != nil {
		return NewWarningError("Context has error, cancelling bulk operation")
	}

	// Prepare to track metrics
	var successCount, warningCount, failureCount, totalPlayers int32

	// Process matches in batches to avoid overwhelming the API
	matchBatches := SplitIntoBatches(matchIDs, MAX_MATCH_BATCH)

	for _, batch := range matchBatches {
		// Collect all players from all matches in this batch
		allPlayers := make([]model.Entity, 0, len(batch)*50) // Estimate 50 players per match

		// Process each match to extract players
		for _, matchID := range batch {
			// Check for cancellation between API calls
			if ctx.Err() != nil {
				return NewWarningError("Context cancelled during bulk match processing")
			}

			match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)
			if err != nil {
				atomic.AddInt32(&failureCount, 1)
				log.Error().Err(err).Str("matchID", matchID).Msg("Failed to get match")
				continue
			}

			// Extract players from this match
			matchPlayers := make([]model.Entity, 0, len(match.Included))

			for _, player := range match.Included {
				if player.IsValidPlayer() {
					name, _ := player.GetName()
					playerID, _ := player.GetAccountID()

					entity := model.Entity{
						ID:     playerID,
						Name:   name,
						Active: true,
					}
					matchPlayers = append(matchPlayers, entity)
				}
			}

			if len(matchPlayers) == 0 {
				atomic.AddInt32(&warningCount, 1)
				log.Warn().Str("Match ID", matchID).Msg("No players found to expand with")
				continue
			}

			// Add these players to our batch collection
			allPlayers = append(allPlayers, matchPlayers...)
			atomic.AddInt32(&successCount, 1)
			atomic.AddInt32(&totalPlayers, int32(len(matchPlayers)))
		}

		// Check for cancellation before database update
		if ctx.Err() != nil {
			return NewWarningError("Context cancelled before database update")
		}

		// Bulk upsert all players from this batch of matches
		if len(allPlayers) > 0 {
			err := p.db.BulkUpsertEntities(ctx, "players", allPlayers)
			if err != nil {
				log.Error().Err(err).Int("playerCount", len(allPlayers)).Msg("Could not bulk upsert players to database")
				atomic.AddInt32(&failureCount, int32(len(batch)))
				atomic.StoreInt32(&successCount, 0) // Reset success count since the upsert failed
				continue
			}
		}
	}

	// Return appropriate status based on overall results
	failCount := int(failureCount)
	warnCount := int(warningCount)
	succCount := int(successCount)
	playerCount := int(totalPlayers)

	if failCount > 0 && succCount == 0 {
		return NewFailureError(fmt.Errorf("failed to process all %d matches", failCount))
	} else if failCount > 0 || warnCount > 0 {
		return NewWarningError(fmt.Sprintf("Processed %d matches with %d failures, %d warnings, found %d players",
			succCount, failCount, warnCount, playerCount))
	}

	return NewSuccessError(fmt.Sprintf("Successfully processed %d matches, found %d players", succCount, playerCount))
}

// Operation implements BatchProcessor.
// Player IDs are a slice of 10 player IDs as strings
func (p *playerProcessor) Operation(ctx context.Context, playerIDs []string, jobID primitive.ObjectID) StatusError {
	if ctx.Err() != nil { // Check if job has been cancelled
		return NewWarningError("Context has error, cancelling operation")
	}

	playersResponse, err := p.pubgClient.GetPlayersByIDs(pubg.SteamPlatform, playerIDs) // Fixed typo in variable name
	if err != nil {
		log.Error().Err(err).Str("Operation Name", p.Name()).Msg("Failed to get players by IDs")
		return NewFailureError(err)
	}

	// 1. Search through each player responses included match IDs and add them to a set
	matchMap := make(map[string]bool, len(playerIDs)*100)
	for _, player := range playersResponse.Data { // Fixed variable name
		for _, matchRelationships := range player.Relationships.Matches.Data {
			matchMap[matchRelationships.ID] = true
		}
	}

	matches := make([]string, 0, len(matchMap))
	for matchID := range matchMap {
		matches = append(matches, matchID)
	}

	// Add this cancellation check before nested batch processing
	if ctx.Err() != nil {
		log.Warn().Msg("Context cancelled before match processing could begin")
		return NewWarningError("Context cancelled before match processing")
	}

	// Use the bulk processing method instead of individual match processing
	return p.BulkSearchMatchForPlayers(ctx, matches, jobID)
}

// SearchMatchForPlayersOperation is kept for backward compatibility but marked as deprecated
// This can be removed in future versions once migration is complete
func (p *playerProcessor) SearchMatchForPlayersOperation(ctx context.Context, matchID string, jobID primitive.ObjectID) StatusError {
	if ctx.Err() != nil { // Check if job has been cancelled
		return NewWarningError("Context has error, cancelling operation")
	}

	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)

	if err != nil {
		log.Error().Err(err).Msg("Search Match For Players operation failed")
		return NewFailureError(err)
	}

	players := make([]model.Entity, 0, len(match.Included))

	for _, player := range match.Included {
		if player.IsValidPlayer() {
			// Build entity since player is valid
			name, _ := player.GetName() // Ok value not needed as IsValidPlayer checks for this already (fixed typo)
			playerID, _ := player.GetAccountID()

			entity := model.Entity{
				ID:     playerID,
				Name:   name,
				Active: true,
			}
			players = append(players, entity)
		}
	}

	if len(players) == 0 {
		log.Warn().Str("Match ID", matchID).Msg("No players found to expand with")
		return NewWarningError(fmt.Sprintf("%s match id has no players to expand with", matchID))
	}

	// Add a cancellation check before database operation
	if ctx.Err() != nil {
		return NewWarningError("Context cancelled before database update")
	}

	err = p.db.BulkUpsertEntities(ctx, "players", players)
	if err != nil {
		log.Error().Err(err).Msg("Could not bulk upsert players to database")
		return NewFailureError(err)
	}

	return NewSuccessError(fmt.Sprintf("Successfully upserted %v new players", len(players)))
}

// Name implements BatchProcessor.
func (p *playerProcessor) Name() string {
	return PLAYER_PROCESSOR_NAME
}

// Type implement BatchProcessor
func (p *playerProcessor) Type() string {
	return PLAYER_PROCESSOR_TYPE
}

// StartJob implements BatchProcessor.
func (p *playerProcessor) StartJob(job *model.Job) ([]model.JobResult, error) {
	// Create a cancelable context with an optional timeout
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour) // Example: 6-hour max runtime
	p.cancelFunc = &cancel
	p.ctx = &ctx

	playerEntitySlice, err := p.db.GetActivePlayers(*p.ctx, -1)

	if err != nil {
		log.Error().Err(err).Msg("Error getting active players")
		return nil, err
	}

	playerIDs := make([]string, 0, len(playerEntitySlice))
	for _, player := range playerEntitySlice {
		playerIDs = append(playerIDs, player.ID)
	}

	initialBatches := SplitIntoBatches(playerIDs, 10)

	// Initialize the total values in the job's metrics
	initialMetrics := model.JobMetrics{
		ProcessedItems:  0,
		SuccessCount:    0,
		WarningCount:    0,
		FailureCount:    0,
		BatchesComplete: 0,
	}

	// Update the job with initial metrics
	err = p.UpdateMetrics(*p.ctx, job.ID.Hex(), initialMetrics)
	if err != nil {
		log.Error().Err(err).Msg("Error initializing job metrics")
		return nil, err
	}

	// Use the enhanced ProcessBatch with metrics tracking
	ProcessBatch(*p.ctx, initialBatches, p.Operation, job.BatchSize, p, job.ID)

	p.ctx = nil
	p.cancelFunc = nil

	// For the job result, we'll return an empty slice
	// The actual results are being stored in the job document through the UpdateMetrics calls
	return []model.JobResult{}, nil
}

func (p *playerProcessor) IsActive() bool {
	return p.ctx != nil
}

// UpdateMetrics implements BatchProcessor.
func (p *playerProcessor) UpdateMetrics(ctx context.Context, jobID string, metrics model.JobMetrics) error {
	return p.db.UpdateJobProgress(ctx, jobID, metrics)
}

func (p *playerProcessor) Cancel() error {
	if p.cancelFunc != nil {
		cancel := *p.cancelFunc
		cancel()
	}
	return nil
}

func NewPlayerProcessor(db database.Database, pubgClient *pubg.Client) StringSliceBatchProcessor {
	return &playerProcessor{
		db:         db,
		pubgClient: pubgClient,
	}
}
