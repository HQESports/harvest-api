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

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	PLAYER_EXPANDER_TYPE = "player_expander_worker"
	PLAYER_EXPANDER_NAME = "Player Expander Worker - Search through stored players and expand known players"
)

type PlayerExpanderWorker struct {
	pubgClient *pubg.Client
	db         database.Database
	ctx        *context.Context
	cancelFunc *context.CancelFunc
	jobID      *primitive.ObjectID
	cancelled  int32 // Using atomic for thread-safe access
}

// Cancel implements job.BatchWorker.
func (p *PlayerExpanderWorker) Cancel() error {
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
func (p *PlayerExpanderWorker) IsActive() bool {
	return atomic.LoadInt32(&p.cancelled) == 0 && p.ctx != nil
}

func (p *PlayerExpanderWorker) ActiveJobID() *primitive.ObjectID {
	if !p.IsActive() {
		return nil
	}
	return p.jobID
}

// Name implements job.BatchWorker.
func (p *PlayerExpanderWorker) Name() string {
	return PLAYER_EXPANDER_NAME
}

// Type implements job.BatchWorker.
func (p *PlayerExpanderWorker) Type() string {
	return PLAYER_EXPANDER_TYPE
}

// isCancelled returns true if the worker has been cancelled
func (p *PlayerExpanderWorker) isCancelled() bool {
	return atomic.LoadInt32(&p.cancelled) == 1 || p.ctx == nil
}

// SafeContext returns the current context if available or a cancelled context if not
func (p *PlayerExpanderWorker) SafeContext() context.Context {
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
func (p *PlayerExpanderWorker) StartWorker(job *model.Job) (bool, error) {
	// Reset the cancelled flag
	atomic.StoreInt32(&p.cancelled, 0)

	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = &ctx
	p.cancelFunc = &cancel
	p.jobID = &job.ID

	defer func() {
		// Clean up in case of panic or return
		if !p.isCancelled() {
			p.Cancel() // Use our Cancel() method which handles cleanup properly
		}
	}()

	playerEntities, err := p.db.GetActivePlayers(context.TODO(), -1)
	if err != nil {
		log.Error().Err(err).Msg("Error getting active players in order to start worker")
		return false, fmt.Errorf("failed to get active players: %w", err)
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

		if p.isCancelled() {
			log.Warn().Msg("Worker has been cancelled after processing batch")
			return true, nil
		}

		safeCtx = p.SafeContext()
		if err := p.db.IncrementJobBatchesComplete(safeCtx, job.ID, 1); err != nil {
			log.Error().Err(err).Msg("Error incrementing batches completed")
		}
	}

	// Clean up values that are not longer used
	p.Cancel() // Use our Cancel() method which handles cleanup properly

	return false, nil
}

func (p *PlayerExpanderWorker) ProcessBatch(batch []string, jobID primitive.ObjectID) error {
	if p.isCancelled() {
		return fmt.Errorf("worker cancelled")
	}

	matchIDs, err := p.pubgClient.GetMatchIDsForPlayers(pubg.SteamPlatform, batch)
	if err != nil {
		log.Error().Err(err).Msg("Error getting match IDs for players")
		return fmt.Errorf("failed to get match IDs: %w", err)
	}
	log.Info().Int("# Matches", len(matchIDs)).Msg("Found matches")

	matchIDBatches := orchestrator.SplitIntoBatches(matchIDs, 20)

	for _, matchIDBatch := range matchIDBatches {
		if p.isCancelled() {
			return fmt.Errorf("worker cancelled while processing match batches")
		}

		var wg sync.WaitGroup
		wg.Add(len(matchIDBatch))

		// Create metrics for this batch
		metrics := model.JobMetrics{}

		// Use mutex to protect concurrent access to metrics
		var mutex sync.Mutex

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

				matchMetrics, err := p.ProcessMatchID(id)
				if err != nil {
					log.Error().Err(err).Str("Match ID", id).Msg("Could not process match ID")
					mutex.Lock()
					metrics.FailureCount++
					mutex.Unlock()
					return
				}

				// Only lock if we have metrics to update
				if matchMetrics != nil {
					// Lock before updating shared metrics
					mutex.Lock()
					metrics.ProcessedItems += matchMetrics.ProcessedItems
					metrics.SuccessCount += matchMetrics.SuccessCount
					metrics.InvalidCount += matchMetrics.InvalidCount
					metrics.FailureCount += matchMetrics.FailureCount
					metrics.WarningCount += matchMetrics.WarningCount
					mutex.Unlock()
				}
			}(matchID) // Pass matchID as parameter to avoid closure issues
		}

		// Wait for all goroutines to complete before updating metrics
		wg.Wait()

		if p.isCancelled() {
			return fmt.Errorf("worker cancelled after processing matches")
		}

		// Get a safe context for database operations
		safeCtx := p.SafeContext()

		// Now it's safe to update the metrics in the database
		if err := p.db.UpdateJobMetrics(safeCtx, jobID, metrics); err != nil {
			log.Error().Err(err).Msg("Error updating job metrics")
		}
	}

	return nil
}

func (p *PlayerExpanderWorker) ProcessMatchID(matchID string) (*model.JobMetrics, error) {
	if p.isCancelled() {
		return nil, fmt.Errorf("worker cancelled")
	}

	metrics := model.JobMetrics{}
	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)

	if err != nil {
		log.Error().Err(err).Msg("Error getting match")
		return nil, fmt.Errorf("failed to get match: %w", err)
	}

	newPlayers := make([]model.Entity, 0, 100)
	cnt := 0

	for _, included := range match.Included {
		metrics.ProcessedItems++
		if included.IsValidPlayer() {
			ID, ok := included.GetAccountID()
			if !ok {
				metrics.InvalidCount++
				continue
			}
			name, ok := included.GetName()
			if !ok {
				metrics.InvalidCount++
				continue
			}

			player := model.Entity{
				Name:   name,
				ID:     ID,
				Active: true,
			}
			newPlayers = append(newPlayers, player)
			cnt++
		} else {
			metrics.InvalidCount++
		}
	}

	// Check for cancellation before database operation
	if p.isCancelled() {
		return nil, fmt.Errorf("worker cancelled before database update")
	}

	// Get a safe context for database operations
	safeCtx := p.SafeContext()

	result, err := p.db.BulkUpsertPlayers(safeCtx, newPlayers)
	if err != nil {
		log.Error().Err(err).Msg("Error bulk upserting players into the database")
		metrics.FailureCount += cnt
		return &metrics, fmt.Errorf("failed to bulk upsert players: %w", err)
	}

	if result == nil {
		log.Error().Msg("metrics is nil, returning a failure")
		return &model.JobMetrics{
			FailureCount:   cnt,
			ProcessedItems: metrics.ProcessedItems,
		}, nil
	}

	metrics.SuccessCount += int(result.UpsertedCount)
	metrics.WarningCount += int(result.MatchedCount)

	return &metrics, nil
}

func NewPlayerExpanderWorker(pubgClient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &PlayerExpanderWorker{
		pubgClient: pubgClient,
		db:         db,
		ctx:        nil,
		cancelFunc: nil,
		cancelled:  1, // Start in cancelled state
	}
}
