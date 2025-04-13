package processor

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/pkg/pubg"

	"github.com/rs/zerolog/log"
)

const MAX_MATCH_BATCH = 10

const (
	name        = "PUBG Plyaer Processor - Search and Expand Players"
	processType = "player-search-and-expand"
)

type playerProcessor struct {
	db         database.Database // Use for updating job progress as we go through batches and pulling current player IDs
	pubgClient *pubg.Client      // Used for access
}

// Operation implements BatchProcessor.

// Player IDs are a slice of 10 player IDs as strings
func (p playerProcessor) Operation(playerIDs []string) StatusError {
	plyaersResponse, err := p.pubgClient.GetPlayersByIDs(pubg.SteamPlatform, playerIDs)
	if err != nil {
		log.Error().Err(err).Str("Operation Name", p.Name()).Msg("Failed to get players by IDs")
		return NewFailureError(err)
	}

	// 1. Search through each player responses included match IDs and add them to a set
	matchMap := make(map[string]bool, len(playerIDs)*100)
	for _, player := range plyaersResponse.Data {
		for _, matchRelationships := range player.Relationships.Matches.Data {
			matchMap[matchRelationships.ID] = true
		}
	}

	matches := make([]string, 0, len(matchMap))
	for matchID := range matchMap {
		matches = append(matches, matchID)
	}

	// 2. Process each match ID and find the all the players in them to add to the databse
	ProcessBatch(matches, p.SearchMatchForPlayersOperation, MAX_MATCH_BATCH)

	return NewSuccessError("Sucessfully processed batch of player IDs")
}

func (p playerProcessor) SearchMatchForPlayersOperation(matchID string) StatusError {
	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)

	if err != nil {
		log.Error().Err(err).Msg("Search Match For Players operation failed")
		return NewFailureError(err)
	}

	players := make([]model.Entity, 0, len(match.Included))

	for _, player := range match.Included {
		if player.IsValidPlayer() {
			// Build entity since player is valid
			name, _ := player.GetName() // Ok value not needed as IsValidPlyaer checks for this already

			entity := model.Entity{
				ID:     player.ID,
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

	err = p.db.BulkUpsertEntities(context.Background(), "players", players)
	if err != nil {
		log.Error().Err(err).Msg("Could not bulk upsert players to database")
	}

	return NewSuccessError(fmt.Sprintf("Successfuly upserted %v new players", len(players)))
}

// Name implements BatchProcessor.
func (p playerProcessor) Name() string {
	return name
}

// Type implement BatchProcessor
func (p playerProcessor) Type() string {
	return processType
}

// ProcessBatch implements BatchProcessor.
func (p playerProcessor) ProcessBatch(ctx context.Context, job *model.Job) ([]model.JobResult, error) {
	playerEntitySlice, err := p.db.GetActivePlayers(ctx, -1)

	if err != nil {
		log.Error().Err(err).Msg("Error get active players")
		return nil, err
	}

	playerIDs := make([]string, 0, len(playerEntitySlice))
	for _, player := range playerEntitySlice {
		playerIDs = append(playerIDs, player.ID)
	}

	initialBatches := SplitIntoBatches(playerIDs, 10)
	ProcessStringSlicesBatch(initialBatches, p.Operation, job.BatchSize)

	return []model.JobResult{}, nil
}

// UpdateMetrics implements BatchProcessor.
func (p playerProcessor) UpdateMetrics(ctx context.Context, jobID string, metrics model.JobMetrics) {
	var progress int = 0

	// Check for division by zero
	if metrics.TotalItems > 0 {
		// Calculate percentage as float first
		progressFloat := float64(metrics.SuccessCount) / float64(metrics.TotalItems) * 100

		// Convert to int (truncates decimal portion)
		progress = int(progressFloat)
	}

	p.db.UpdateJobProgress(ctx, jobID, progress, metrics)
}

func NewPlayerProcessor(db database.Database, pubgClient *pubg.Client) StringSliceBatchProcessor {
	return playerProcessor{
		db:         db,
		pubgClient: pubgClient,
	}
}
