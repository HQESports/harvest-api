package processor

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/pkg/pubg"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	MATCH_PROCESSOR_NAME = "PUBG Match Processor - Search Players and Insert Matches"
	MATCH_PROCESSOR_TYPE = "match-search-and-expand"
)

type matchProcessor struct {
	db         database.Database // Use for updating job progress as we go through batches and pulling current player IDs
	pubgClient *pubg.Client      // Used for access
	ctx        *context.Context  // Added context for consistency with playerProcessor
	cancelFunc *context.CancelFunc
}

// Add this method to the matchProcessor struct
func (p *matchProcessor) BulkInsertMatches(ctx context.Context, matchIDs []string, jobID primitive.ObjectID) StatusError {
	if len(matchIDs) == 0 {
		return NewSuccessError("No matches to process")
	}

	// Check if context is cancelled
	if ctx.Err() != nil {
		return NewWarningError("Context has error, cancelling bulk operation")
	}

	// Prepare to track metrics
	successCount := 0
	warningCount := 0
	failureCount := 0

	// Process matches in batches to avoid overwhelming the API
	matchBatches := SplitIntoBatches(matchIDs, MAX_MATCH_BATCH)

	for _, batch := range matchBatches {
		matches := make([]model.Match, 0, len(batch))

		// Fetch matches in the current batch
		for _, matchID := range batch {
			// Check for cancellation between API calls
			if ctx.Err() != nil {
				return NewWarningError("Context cancelled during bulk match processing")
			}

			match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)
			if err != nil {
				failureCount++
				log.Error().Err(err).Str("matchID", matchID).Msg("Failed to get match")
				continue
			}

			if !match.IsValidMatch() {
				warningCount++
				continue
			}

			// Parse match data similar to InsertMatch
			createdAt, err := time.Parse(time.RFC3339, match.Data.Attributes.CreatedAt)
			if err != nil {
				failureCount++
				log.Error().Err(err).Str("matchID", matchID).Msg("Failed to parse match creation time")
				continue
			}

			telemetryURL, _ := match.GetTelemetryURL()

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

			matchDocument := model.Match{
				MatchID:       matchID,
				ShardID:       pubg.SteamPlatform,
				MapName:       match.Data.Attributes.MapName,
				GameMode:      match.Data.Attributes.GameMode,
				Duration:      match.Data.Attributes.Duration,
				IsCustomMatch: match.Data.Attributes.IsCustomMatch,
				CreatedAt:     createdAt,
				MatchType:     match.GetMatchType(),

				// Processing metadata
				Processed:  false,
				ImportedAt: time.Now(),

				// Statistics and counts
				PlayerCount:  playerCount,
				TeamCount:    teamCount,
				TelemetryURL: telemetryURL,
			}

			matches = append(matches, matchDocument)
		}

		// Bulk import the matches for this batch
		if len(matches) > 0 {
			// Assuming we add a BulkImportMatches method to the database interface
			results, err := p.db.BulkImportMatches(ctx, matches)
			if err != nil {
				failureCount += len(matches)
				log.Error().Err(err).Int("count", len(matches)).Msg("Bulk import matches failed")
				continue
			}

			// Update counts based on results
			successCount += results.SuccessCount
			warningCount += results.DuplicateCount
			failureCount += results.FailureCount

			log.Debug().
				Int("success", results.SuccessCount).
				Int("duplicates", results.DuplicateCount).
				Int("failures", results.FailureCount).
				Msg("Bulk imported matches")
		}
	}

	// Return appropriate status based on overall results
	totalProcessed := successCount + warningCount + failureCount
	if failureCount == totalProcessed {
		return NewFailureError(fmt.Errorf("all matches failed to import"))
	} else if failureCount > 0 {
		return NewWarningError(fmt.Sprintf("Imported %d matches with %d failures", successCount, failureCount))
	}

	return NewSuccessError(fmt.Sprintf("Successfully processed %d matches (%d new, %d duplicates)",
		totalProcessed, successCount, warningCount))
}

// Modify the Operation method to use bulk operations
func (p *matchProcessor) Operation(ctx context.Context, playerIDs []string, jobID primitive.ObjectID) StatusError {
	if ctx.Err() != nil {
		return NewWarningError("Context has error, cancelling operation")
	}

	playersResponse, err := p.pubgClient.GetPlayersByIDs(pubg.SteamPlatform, playerIDs)
	if err != nil {
		log.Error().Err(err).Str("Operation Name", p.Name()).Msg("Failed to get players by IDs")
		return NewFailureError(err)
	}

	// Collect all unique match IDs
	matchMap := make(map[string]bool, len(playerIDs)*100)
	for _, player := range playersResponse.Data {
		for _, matchRelationships := range player.Relationships.Matches.Data {
			matchMap[matchRelationships.ID] = true
		}
	}

	matches := make([]string, 0, len(matchMap))
	for matchID := range matchMap {
		matches = append(matches, matchID)
	}

	// Use bulk processing instead of individual processing
	return p.BulkInsertMatches(ctx, matches, jobID)
}

func (p *matchProcessor) InsertMatch(ctx context.Context, matchID string, jobID primitive.ObjectID) StatusError {
	if ctx.Err() != nil { // Check if job has been cancelled
		return NewWarningError("Context has error, cancelling operation")
	}

	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)
	if err != nil {
		log.Error().Err(err).Msg("Search Match For Players operation failed")
		return NewFailureError(err)
	}

	if !match.IsValidMatch() {
		return NewWarningError("Not a valid match")
	}

	// Parse the match creation time
	createdAt, err := time.Parse(time.RFC3339, match.Data.Attributes.CreatedAt)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse match creation time")
		return NewFailureError(err)
	}

	// Get telemetry URL
	telemetryURL, err := match.GetTelemetryURL()
	if err != nil {
		log.Warn().Err(err).Msg("Could not retrieve telemetry URL")
		// Continue processing even if telemetry URL cannot be retrieved
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

	matchDocument := model.Match{
		MatchID:       matchID,
		ShardID:       pubg.SteamPlatform,
		MapName:       match.Data.Attributes.MapName,
		GameMode:      match.Data.Attributes.GameMode,
		Duration:      match.Data.Attributes.Duration,
		IsCustomMatch: match.Data.Attributes.IsCustomMatch,
		CreatedAt:     createdAt,
		MatchType:     match.GetMatchType(),

		// Processing metadata
		Processed:  false,
		ImportedAt: time.Now(),

		// Statistics and counts
		PlayerCount:  playerCount,
		TeamCount:    teamCount,
		TelemetryURL: telemetryURL,
	}

	// Save match to database
	ok, err := p.db.ImportMatch(ctx, matchDocument)
	if err != nil {
		return NewFailureError(err)
	}

	if !ok {
		return NewWarningError("Match ID already exists")
	}

	return NewSuccessError(fmt.Sprintf("Successfully upserted match %s with %v players and %v teams", matchID, playerCount, teamCount))
}

// Name implements BatchProcessor.
func (p *matchProcessor) Name() string {
	return MATCH_PROCESSOR_NAME
}

// Type implement BatchProcessor
func (p *matchProcessor) Type() string {
	return MATCH_PROCESSOR_TYPE
}

// StartJob implements BatchProcessor.
func (p *matchProcessor) StartJob(job *model.Job) ([]model.JobResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
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
	// Removed unused finalMetrics variable
	ProcessBatch(*p.ctx, initialBatches, p.Operation, job.BatchSize, p, job.ID)

	// For the job result, we'll return an empty slice
	// The actual results are being stored in the job document through the UpdateMetrics calls
	p.ctx = nil
	p.cancelFunc = nil

	return []model.JobResult{}, nil
}

func (p *matchProcessor) IsActive() bool {
	return p.ctx != nil
}

// UpdateMetrics implements BatchProcessor.
func (p *matchProcessor) UpdateMetrics(ctx context.Context, jobID string, metrics model.JobMetrics) error {
	return p.db.UpdateJobProgress(ctx, jobID, metrics)
}

// Cancel implements BaseBatchProcessor
func (p *matchProcessor) Cancel() error {
	if p.cancelFunc != nil {
		cancel := *p.cancelFunc
		cancel()
	}
	return nil
}

func NewMatchProcessor(db database.Database, pubgClient *pubg.Client) StringSliceBatchProcessor {
	return &matchProcessor{
		db:         db,
		pubgClient: pubgClient,
	}
}
