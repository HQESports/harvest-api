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
	TOURNAMENT_EXPANDER_TYPE        = "tournament_expander_worker"
	TOURNAMENT_EXPANDER_NAME        = "Tournament Expander Worker"
	TOURNAMENT_EXPANDER_DESCRIPTION = "Pull PUBG tournaments from the PUBG API"
)

type tournamentExpanderWorker struct {
	pubgClient *pubg.Client
	db         database.Database
	ctx        *context.Context
	cancelFunc *context.CancelFunc
	jobID      *primitive.ObjectID
	cancelled  int32 // Using atomic for thread-safe access
}

// ActiveJobID implements orchestrator.BatchWorker.
func (t *tournamentExpanderWorker) ActiveJobID() *primitive.ObjectID {
	if !t.IsActive() {
		return nil
	}
	return t.jobID
}

// Cancel implements orchestrator.BatchWorker.
func (t *tournamentExpanderWorker) Cancel() error {
	if t.cancelFunc != nil {
		cancelFunc := *t.cancelFunc
		cancelFunc()
	} else {
		return fmt.Errorf("job is not active")
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
func (t *tournamentExpanderWorker) Description() string {
	return TOURNAMENT_EXPANDER_DESCRIPTION
}

// IsActive implements orchestrator.BatchWorker.
func (t *tournamentExpanderWorker) IsActive() bool {
	return atomic.LoadInt32(&t.cancelled) == 0 && t.ctx != nil
}

// Name implements orchestrator.BatchWorker.
func (t *tournamentExpanderWorker) Name() string {
	return TOURNAMENT_EXPANDER_NAME
}

// StartWorker implements orchestrator.BatchWorker.
func (t *tournamentExpanderWorker) StartWorker(job *model.Job) (bool, error) {
	// Reset the cancelled flag
	atomic.StoreInt32(&t.cancelled, 0)

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.ctx = &ctx
	t.cancelFunc = &cancelFunc
	t.jobID = &job.ID

	defer func() {
		// Clean up in case of panic or return
		t.ctx = nil
		t.cancelFunc = nil
		t.jobID = nil
	}()
	tournaments, err := t.pubgClient.GetTournaments()
	log.Info().Int("Tournaments", len(tournaments.Data)).Msg("Tournaments found")
	if err != nil {
		return false, fmt.Errorf("error getting tournaments: %v", err)
	}
	//tournamens.Data

	batches := orchestrator.SplitIntoBatches(tournaments.Data, job.BatchSize)

	safeCtx := t.SafeContext()
	if err := t.db.SetJobTotalBatches(safeCtx, job.ID, len(batches)); err != nil {
		log.Error().Err(err).Msg("Error setting job total batches")
	}

	safeCtx = t.SafeContext()
	for _, batch := range batches {
		metrics := t.processBatch(batch)

		metrics.BatchesComplete = 1

		t.db.UpdateJobMetrics(safeCtx, job.ID, metrics)
	}

	return false, nil
}

func (t *tournamentExpanderWorker) processBatch(batch []pubg.TournamentData) model.JobMetrics {
	metrics := model.JobMetrics{}
	tournamentEntities := make([]model.Entity, 0, len(batch))

	var wg sync.WaitGroup
	var mutex sync.Mutex
	wg.Add(len(batch))

	for _, tournament := range batch {
		go func(tournament pubg.TournamentData) {
			defer wg.Done()
			if !t.IsActive() {
				log.Warn().Msg("cancelling batch due to job being cancelled")
				return
			}
			entity := t.processTournament(tournament)

			mutex.Lock()
			metrics.ProcessedItems++
			tournamentEntities = append(tournamentEntities, entity)
			mutex.Unlock()
		}(tournament)
	}

	wg.Wait()

	safeCtx := t.SafeContext()
	bulkResult, err := t.db.BulkUpsertTournaments(safeCtx, tournamentEntities)
	if err != nil {
		log.Error().Err(err).Msg("error bulk upserting tournamentss")
		metrics.FailureCount += 1
		return metrics
	}

	metrics.SuccessCount += int(bulkResult.UpsertedCount)
	metrics.WarningCount += int(bulkResult.MatchedCount)

	return metrics
}

func (t *tournamentExpanderWorker) processTournament(tournament pubg.TournamentData) model.Entity {
	tournamentEntity := model.Entity{
		Name:      pubg.BuildTournamentName(tournament.ID),
		ID:        tournament.ID,
		Active:    true,
		CreatedAt: tournament.Attributes.CreatedAt,
	}

	return tournamentEntity
}

// Type implements orchestrator.BatchWorker.
func (t *tournamentExpanderWorker) Type() string {
	return TOURNAMENT_EXPANDER_TYPE
}

func (t *tournamentExpanderWorker) isCancelled() bool {
	return !t.IsActive()
}

// SafeContext returns the current context if available or a cancelled context if not
func (t *tournamentExpanderWorker) SafeContext() context.Context {
	if t.isCancelled() || t.ctx == nil {
		// Return a cancelled context if worker is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return *t.ctx
}

func NewTournamentExpanderWorker(pubgClient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &tournamentExpanderWorker{
		pubgClient: pubgClient,
		db:         db,
		ctx:        nil,
		jobID:      nil,
		cancelFunc: nil,
		cancelled:  1,
	}
}
