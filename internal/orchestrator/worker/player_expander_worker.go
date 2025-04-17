package worker

import (
	"context"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/orchestrator"
	"harvest/pkg/pubg"
	"sync"

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

	jobID *primitive.ObjectID
}

// Cancel implements job.BatchWorker.
func (p *PlayerExpanderWorker) Cancel() error {
	cancelFunc := *p.cancelFunc
	cancelFunc()

	return nil
}

func (p *PlayerExpanderWorker) ActiveJobID() *primitive.ObjectID {
	return p.jobID
}

// IsActive implements job.BatchWorker.
func (p *PlayerExpanderWorker) IsActive() bool {
	return p.ctx != nil
}

// Name implements job.BatchWorker.
func (p *PlayerExpanderWorker) Name() string {
	return PLAYER_EXPANDER_NAME
}

// StartWorker implements job.BatchWorker.
// Pull in all players and create 10 batches where each batch contains 10 batches of player IDs
// Process each batch of 10 players ID concurrently
func (p *PlayerExpanderWorker) StartWorker(job *model.Job) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = &ctx
	p.cancelFunc = &cancel
	p.jobID = (&job.ID)

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
	p.db.SetJobTotalBatches(*p.ctx, job.ID, len(initialBatches))

	for _, batch := range initialBatches {
		cancelCtx := *p.ctx
		if cancelCtx.Err() != nil {
			log.Warn().Msg("worker has been canceled, stoping process")
			return true, nil
		}
		p.ProcessBatch(batch, job.ID)

		err = p.db.IncrementJobBatchesComplete(*p.ctx, job.ID, 1)
		if err != nil {
			log.Error().Err(err).Msg("Error incrementing batches completes")
		}
	}

	// Clean up values that are not longer used
	p.ctx = nil
	p.cancelFunc = nil
	p.jobID = nil

	return false, nil
}

func (p *PlayerExpanderWorker) ProcessBatch(batch []string, jobID primitive.ObjectID) error {
	matchIDs, err := p.pubgClient.GetMatchIDsForPlayers(pubg.SteamPlatform, batch)
	if err != nil {
		log.Error().Err(err).Msg("Error getting match IDs for players")
	}
	log.Info().Int("# Matches", len(matchIDs)).Msg("Found matches")

	matchIDBatches := orchestrator.SplitIntoBatches(matchIDs, 20)

	for _, matchIDBatch := range matchIDBatches {
		canceledCtx := *p.ctx
		if canceledCtx.Err() != nil {
			log.Warn().Msg("worker has been cancelled stopping batch processing")
			break
		}
		var wg sync.WaitGroup
		wg.Add(len(matchIDBatch))

		// Create metrics for this batch
		metrics := model.JobMetrics{}

		// Use mutex to protect concurrent access to metrics
		var mutex sync.Mutex

		for _, matchID := range matchIDBatch {
			go func(id string) {
				defer wg.Done()
				matchMetrics, err := p.ProcessMatchID(id)
				if err != nil {
					log.Error().Err(err).Str("Match ID", id).Msg("Could not process match ID")
				}

				// Lock before updating shared metrics
				mutex.Lock()
				metrics.ProcessedItems += matchMetrics.ProcessedItems
				metrics.SuccessCount += matchMetrics.SuccessCount
				metrics.FailureCount += matchMetrics.FailureCount
				metrics.WarningCount += matchMetrics.WarningCount
				mutex.Unlock()
			}(matchID) // Pass matchID as parameter to avoid closure issues
		}

		// Wait for all goroutines to complete before updating metrics
		wg.Wait()

		// Now it's safe to update the metrics in the database
		err := p.db.UpdateJobMetrics(*p.ctx, jobID, metrics)
		if err != nil {
			log.Error().Err(err).Msg("Error updating job metrics")
		}
	}

	return nil
}

func (p *PlayerExpanderWorker) ProcessMatchID(matchID string) (*model.JobMetrics, error) {
	metrics := model.JobMetrics{}
	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)

	if err != nil {
		log.Error().Err(err).Msg("Error getting match")
		return nil, err
	}

	newPlyaers := make([]model.Entity, 0, 100)
	cnt := 0

	for _, included := range match.Included {
		metrics.ProcessedItems++
		if included.IsValidPlayer() {
			ID, ok := included.GetAccountID()
			if !ok {
				continue
			}
			name, ok := included.GetName()
			if !ok {
				continue
			}

			player := model.Entity{
				Name:   name,
				ID:     ID,
				Active: true,
			}
			newPlyaers = append(newPlyaers, player)
			cnt++
		}
	}
	result, err := p.db.BulkUpsertPlayers(*p.ctx, newPlyaers)
	if err != nil {
		log.Error().Err(err).Msg("Error bulk upserting plyaers into the database")
		metrics.FailureCount += cnt
		return &metrics, err
	}

	if result == nil {
		log.Error().Msg("metrics is nil, returning a failure")
		return &model.JobMetrics{
			FailureCount: 0,
		}, nil
	}

	metrics.SuccessCount += int(result.UpsertedCount)
	metrics.WarningCount += int(result.MatchedCount)

	return &metrics, nil
}

// Type implements job.BatchWorker.
func (p *PlayerExpanderWorker) Type() string {
	return PLAYER_EXPANDER_TYPE
}

func NewPlayerExpanderWorker(pubclient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &PlayerExpanderWorker{
		pubgClient: pubclient,
		db:         db,
		ctx:        nil,
		cancelFunc: nil,
	}
}
