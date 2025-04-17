package worker

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/orchestrator"
	"harvest/pkg/pubg"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	MATCH_EXPANDER_TYPE = "match_expander_worker"
	MATCH_EXPANDER_NAME = "Match Expander Worker - Search through stored players and bulk import their matches"
)

type MatchExpanderWorker struct {
	pubgClient *pubg.Client
	db         database.Database
	ctx        *context.Context
	cancelFunc *context.CancelFunc
	jobID      *primitive.ObjectID
	cancelled  int32 // Using atomic for thread-safe access
}

// Cancel implements job.BatchWorker.
func (p *MatchExpanderWorker) Cancel() error {
	if p.cancelFunc != nil {
		cancelFunc := *p.cancelFunc
		cancelFunc()
	}

	// Set the cancelled flag atomically
	atomic.StoreInt32(&p.cancelled, 1)

	// Then clear the fields
	p.ctx = nil
	p.cancelFunc = nil
	p.jobID = nil

	return nil
}

// IsActive implements job.BatchWorker.
func (p *MatchExpanderWorker) IsActive() bool {
	return atomic.LoadInt32(&p.cancelled) == 0 && p.ctx != nil
}

func (p *MatchExpanderWorker) ActiveJobID() *primitive.ObjectID {
	if !p.IsActive() {
		return nil
	}
	return p.jobID
}

// Name implements job.BatchWorker.
func (p *MatchExpanderWorker) Name() string {
	return MATCH_EXPANDER_NAME
}

// Type implements job.BatchWorker.
func (p *MatchExpanderWorker) Type() string {
	return MATCH_EXPANDER_TYPE
}

// isCancelled returns true if the worker has been cancelled
func (p *MatchExpanderWorker) isCancelled() bool {
	return atomic.LoadInt32(&p.cancelled) == 1 || p.ctx == nil
}

// SafeContext returns the current context if available or a cancelled context if not
func (p *MatchExpanderWorker) SafeContext() context.Context {
	if p.isCancelled() || p.ctx == nil {
		// Return a cancelled context if worker is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return *p.ctx
}

// StartWorker implements job.BatchWorker.
// Pull in all players and create 10 batches where each batch contains 10 batches of player IDs
// Process each batch of 10 players ID concurrently
func (p *MatchExpanderWorker) StartWorker(job *model.Job) (bool, error) {
	// Reset the cancelled flag
	atomic.StoreInt32(&p.cancelled, 0)

	ctx, cancelFunc := context.WithCancel(context.Background())
	p.ctx = &ctx
	p.cancelFunc = &cancelFunc
	p.jobID = &job.ID

	defer func() {
		// Clean up in case of panic or return
		if !p.isCancelled() {
			p.ctx = nil
			p.cancelFunc = nil
			p.jobID = nil
		}
	}()

	playerEntities, err := p.db.GetActivePlayers(context.TODO(), -1)
	if err != nil {
		log.Error().Err(err).Msg("Error getting active jobs in order to start worker")
		return false, err
	}
	playerIDs := make([]string, 0, len(playerEntities))
	for _, player := range playerEntities {
		playerIDs = append(playerIDs, player.ID)
	}

	log.Info().Int("Batch Size", job.BatchSize).Msg("Starting job with batch size")
	initialBatches := orchestrator.SplitIntoBatches(playerIDs, 10)

	safeCtx := p.SafeContext()
	if err := p.db.SetJobTotalBatches(safeCtx, job.ID, len(initialBatches)); err != nil {
		log.Error().Err(err).Msg("Error setting job total batches")
	}

	for _, batch := range initialBatches {
		if p.isCancelled() {
			log.Warn().Msg("Worker has been cancelled, stopping process")
			return true, nil
		}

		if err := p.ProcessBatch(batch, job.ID); err != nil {
			log.Error().Err(err).Msg("Error processing batch")
			// Continue with other batches
		}

		safeCtx = p.SafeContext()
		if err := p.db.IncrementJobBatchesComplete(safeCtx, job.ID, 1); err != nil {
			log.Error().Err(err).Msg("Error incrementing batches completed")
		}
	}

	return false, nil
}

func (p *MatchExpanderWorker) ProcessBatch(batch []string, jobID primitive.ObjectID) error {
	if p.isCancelled() {
		return fmt.Errorf("worker cancelled")
	}

	matchIDs, err := p.pubgClient.GetMatchIDsForPlayers(pubg.SteamPlatform, batch)
	if err != nil {
		log.Error().Err(err).Msg("Error getting match IDs for players")
		return err
	}
	log.Info().Int("# Matches", len(matchIDs)).Msg("Found matches")

	matchIDBatches := orchestrator.SplitIntoBatches(matchIDs, 40)

	for _, matchIDBatch := range matchIDBatches {
		if p.isCancelled() {
			return fmt.Errorf("worker cancelled while processing match batch")
		}

		var wg sync.WaitGroup
		wg.Add(len(matchIDBatch))

		// Create metrics for this batch
		metrics := model.JobMetrics{}

		// Use mutex to protect concurrent access to metrics and matchDocumentSlice
		var mutex sync.Mutex

		// Match document out slice to bulk up
		matchDocumentSlice := make([]model.Match, 0, 300)

		for _, matchID := range matchIDBatch {
			if p.isCancelled() {
				// Adjust wait group count for remaining items
				wg.Add(-1)
				continue
			}

			go func(id string) {
				defer wg.Done()

				// Check for cancellation inside goroutine
				if p.isCancelled() {
					mutex.Lock()
					metrics.FailureCount++
					mutex.Unlock()
					return
				}

				matchDocument, valid, err := p.BuildMatchDocument(id)
				if err != nil {
					log.Error().Err(err).Str("Match ID", id).Msg("Could not process match ID")
					mutex.Lock()
					metrics.FailureCount++
					mutex.Unlock()
					return
				}

				if !valid {
					mutex.Lock()
					metrics.InvalidCount++
					log.Warn().Msg("Skipping invalid match")
					mutex.Unlock()
					return
				}

				mutex.Lock()
				matchDocumentSlice = append(matchDocumentSlice, *matchDocument)
				mutex.Unlock()
			}(matchID)
		}

		wg.Wait()

		if p.isCancelled() {
			return fmt.Errorf("worker cancelled after processing matches")
		}

		// Get a safe context for database operations
		safeCtx := p.SafeContext()

		// Only proceed with bulk import if we have documents to import
		if len(matchDocumentSlice) > 0 {
			bulkResult, err := p.db.BulkImportMatches(safeCtx, matchDocumentSlice)
			if err != nil {
				log.Error().Err(err).Msg("Could not bulk upsert matches")
				// Continue with metrics update
			} else {
				metrics.SuccessCount += bulkResult.SuccessCount
				metrics.WarningCount += bulkResult.DuplicateCount
			}
		}

		metrics.ProcessedItems += len(matchDocumentSlice) + metrics.FailureCount + metrics.InvalidCount

		// Update metrics in the database
		if err := p.db.UpdateJobMetrics(safeCtx, jobID, metrics); err != nil {
			log.Error().Err(err).Msg("Error updating job metrics")
		}
	}

	return nil
}

// Returns false if the match isn't "valid"
func (p *MatchExpanderWorker) BuildMatchDocument(matchID string) (*model.Match, bool, error) {
	if p.isCancelled() {
		return nil, true, fmt.Errorf("worker cancelled")
	}

	safeCtx := p.SafeContext()
	if safeCtx.Err() != nil {
		return nil, true, safeCtx.Err()
	}

	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)
	if err != nil {
		return nil, true, fmt.Errorf("could not get match: %w", err)
	}

	if !match.IsValidMatch() {
		return nil, false, nil
	}

	// Parse the match creation time
	createdAt, err := time.Parse(time.RFC3339, match.Data.Attributes.CreatedAt)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse match creation time")
		return nil, true, fmt.Errorf("failed to parse creation time: %w", err)
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

	return &matchDocument, true, nil
}

func NewMatchExpanderWorker(pubgClient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &MatchExpanderWorker{
		pubgClient: pubgClient,
		db:         db,
		ctx:        nil,
		cancelFunc: nil,
		cancelled:  1, // Start in cancelled state
	}
}
