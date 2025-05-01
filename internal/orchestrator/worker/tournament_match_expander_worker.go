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
	TOURNAMENT_MATCH_EXPANDER_TYPE        = "tournament_match_expander_worker"
	TOURNAMENT_MATCH_EXPANDER_NAME        = "Tournament Match Expander Worker"
	TOURNAMENT_MATCH_EXPANDER_DESCRIPTION = "Search through pubg tournaments and bulk import their matche"
)

type tournamentMatchExpanderWorker struct {
	pubgClient *pubg.Client
	db         database.Database

	ctx        *context.Context
	cancelFunc *context.CancelFunc
	jobID      *primitive.ObjectID
	cancelled  int32 // Using atomic for thread-safe access
}

// Cancel implements job.BatchWorker.
func (t *tournamentMatchExpanderWorker) Cancel() error {
	if t.cancelFunc != nil {
		cancelFunc := *t.cancelFunc
		cancelFunc()
	}

	// Set the cancelled flag atomically
	atomic.StoreInt32(&t.cancelled, 1)

	// Then clear the fields
	t.ctx = nil
	t.cancelFunc = nil
	t.jobID = nil

	return nil
}

// Description implements orchestrator.BatchWorker.
func (t *tournamentMatchExpanderWorker) Description() string {
	return TOURNAMENT_MATCH_EXPANDER_DESCRIPTION
}

// IsActive implements job.BatchWorker.
func (t *tournamentMatchExpanderWorker) IsActive() bool {
	return atomic.LoadInt32(&t.cancelled) == 0 && t.ctx != nil
}

func (t *tournamentMatchExpanderWorker) ActiveJobID() *primitive.ObjectID {
	if !t.IsActive() {
		return nil
	}
	return t.jobID
}

// Name implements orchestrator.BatchWorker.
func (t *tournamentMatchExpanderWorker) Name() string {
	return TOURNAMENT_MATCH_EXPANDER_NAME
}

// StartWorker implements orchestrator.BatchWorker.
func (t *tournamentMatchExpanderWorker) StartWorker(job *model.Job) (bool, error) {
	atomic.StoreInt32(&t.cancelled, 0)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.ctx = &ctx
	t.cancelFunc = &cancelFunc
	t.jobID = &job.ID

	defer func() {
		// Clean up in case of panic or return
		if !t.isCancelled() {
			t.ctx = nil
			t.cancelFunc = nil
			t.jobID = nil
		}
	}()

	// Take all tournament IDs and parse through them to build matches
	safeCtx := t.SafeContext()
	tournaments, err := t.db.GetActiveTournaments(safeCtx, -1)
	if err != nil {
		log.Error().Err(err).Msg("could not get tournaments")
		return false, err
	}
	batches := orchestrator.SplitIntoBatches(tournaments, 9)

	t.db.SetJobTotalBatches(t.SafeContext(), job.ID, len(batches))

	for _, batch := range batches {
		if t.isCancelled() {
			return true, nil
		}
		metrics := t.processTournamentBatch(batch)
		// Batch complete
		metrics.BatchesComplete += 1

		err := t.db.UpdateJobMetrics(t.SafeContext(), job.ID, metrics)

		if err != nil {
			return false, err
		}
	}

	return false, nil
}

func (t *tournamentMatchExpanderWorker) processTournamentBatch(tournamentBatch []model.Entity) model.JobMetrics {
	metrics := model.JobMetrics{}

	var wg sync.WaitGroup
	var mutex sync.Mutex

	if t.isCancelled() {
		return metrics
	}

	wg.Add(len(tournamentBatch))
	for _, tournament := range tournamentBatch {
		go func(tournamentID string) {
			defer wg.Done()

			if t.isCancelled() {
				log.Warn().Msg("stoping process tournament batch, worker cancelled")
				return
			}

			tournamentMetrics, err := t.processTournament(tournamentID)

			if err != nil {
				log.Error().Err(err).Msg("could not process tournament")
				mutex.Lock()
				metrics.FailureCount += 1
				mutex.Unlock()
				return
			}

			mutex.Lock()
			metrics.ProcessedItems += tournamentMetrics.ProcessedItems
			metrics.SuccessCount += tournamentMetrics.SuccessCount
			metrics.WarningCount += tournamentMetrics.WarningCount
			metrics.InvalidCount += tournamentMetrics.InvalidCount
			metrics.FailureCount += tournamentMetrics.FailureCount
			mutex.Unlock()
		}(tournament.ID)
	}

	wg.Wait()

	return metrics
}

func (t *tournamentMatchExpanderWorker) processTournament(tournamentID string) (model.JobMetrics, error) {
	tournamentDetail, err := t.pubgClient.GetTournamentByID(tournamentID)
	metrics := model.JobMetrics{}

	if err != nil {
		log.Error().Err(err).Msg("could not get tournament details by ID")
		return metrics, err
	}

	matchIDs := make([]string, 0, 100)
	for _, match := range tournamentDetail.Data.Relationships.Matches.Data {
		matchIDs = append(matchIDs, match.ID)
	}

	matchIDBatches := orchestrator.SplitIntoBatches(matchIDs, 40)

	matchDocuments := make([]model.Match, 0, len(matchIDs))
	for _, matchIDBatch := range matchIDBatches {
		var wg sync.WaitGroup
		var mutex sync.Mutex

		wg.Add(len(matchIDBatch))

		for _, matchID := range matchIDBatch {
			if t.isCancelled() {
				return metrics, nil
			}
			go func(matchID string) {
				defer wg.Done()

				matchDocument, valid, err := t.BuildMatchDocument(matchID, pubg.EventPlatform)
				if err != nil {
					log.Error().Err(err).Msg("error building match")
					mutex.Lock()
					metrics.FailureCount++
					mutex.Unlock()
					return
				}

				mutex.Lock()
				metrics.ProcessedItems++
				if valid {
					matchDocuments = append(matchDocuments, *matchDocument)
				} else {
					metrics.InvalidCount++
				}
				mutex.Unlock()

			}(matchID)
		}
		wg.Wait()
	}

	bulkResult, err := t.db.BulkImportMatches(t.SafeContext(), matchDocuments)
	if err != nil {
		log.Error().Err(err).Msg("failed to bullk import matches")
		metrics.FailureCount += 1
		return metrics, err
	}

	metrics.SuccessCount += bulkResult.SuccessCount
	metrics.WarningCount += bulkResult.DuplicateCount
	metrics.FailureCount += bulkResult.FailureCount

	return metrics, nil
}

func (t *tournamentMatchExpanderWorker) BuildMatchDocument(matchID, shard string) (*model.Match, bool, error) {
	if t.isCancelled() {
		return nil, true, fmt.Errorf("worker cancelled")
	}

	safeCtx := t.SafeContext()
	if safeCtx.Err() != nil {
		return nil, true, safeCtx.Err()
	}

	match, err := t.pubgClient.GetMatch(shard, matchID)
	if err != nil {
		return nil, true, fmt.Errorf("could not get match: %w", err)
	}

	if !match.IsValidMatch(pubg.EventPlatform) {
		return nil, false, nil
	}

	matchDocument, err := orchestrator.BuildMatchDocument(shard, *match)
	if err != nil {
		log.Error().Err(err).Msg("could not build match document")
		return nil, true, err
	}

	return matchDocument, true, nil
}

// isCancelled returns true if the worker has been cancelled
func (t *tournamentMatchExpanderWorker) isCancelled() bool {
	return atomic.LoadInt32(&t.cancelled) == 1 || t.ctx == nil
}

// Type implements orchestrator.BatchWorker.
func (t *tournamentMatchExpanderWorker) Type() string {
	return TOURNAMENT_MATCH_EXPANDER_TYPE
}

// SafeContext returns the current context if available or a cancelled context if not
func (t *tournamentMatchExpanderWorker) SafeContext() context.Context {
	if t.isCancelled() || t.ctx == nil {
		// Return a cancelled context if worker is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return *t.ctx
}

func NewTournamentMatchExpanderWorker(pubgClient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &tournamentMatchExpanderWorker{
		pubgClient: pubgClient,
		db:         db,

		ctx:        nil,
		cancelFunc: nil,
		jobID:      nil,
		cancelled:  1,
	}
}
