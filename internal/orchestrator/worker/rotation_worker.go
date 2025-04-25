package worker

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/orchestrator"
	"harvest/pkg/pubg"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	ROTATION_TYPE        = "tournament_expander_worker"
	ROTATION_NAME        = "Tournament Expander Worker"
	ROTATION_DESCRIPTION = "Pull PUBG tournaments from the PUBG API"
)

type rotationWorker struct {
	pubgClient *pubg.Client
	db         database.Database
	ctx        *context.Context
	cancelFunc *context.CancelFunc
	jobID      *primitive.ObjectID
	cancelled  int32 // Using atomic for thread-safe access
}

func NewRotationWorker(pubgClient *pubg.Client, db database.Database) orchestrator.BatchWorker {

	return &rotationWorker{
		pubgClient: pubgClient,
		db:         db,

		ctx:        nil,
		cancelFunc: nil,
		jobID:      nil,

		cancelled: 1,
	}
}

// ActiveJobID implements orchestrator.BatchWorker.
func (r *rotationWorker) ActiveJobID() *primitive.ObjectID {
	if !r.IsActive() {
		return nil
	}
	return r.jobID
}

// Cancel implements orchestrator.BatchWorker.
func (r *rotationWorker) Cancel() error {
	if r.cancelFunc != nil {
		cancelFunc := *r.cancelFunc
		cancelFunc()
	} else {
		return fmt.Errorf("job is not active")
	}

	// Set the cancelled flag atomically
	atomic.StoreInt32(&r.cancelled, 1)

	// Then clear the fields
	r.ctx = nil
	r.cancelFunc = nil
	r.jobID = nil

	return nil
}

// Description implements orchestrator.BatchWorker.
func (r *rotationWorker) Description() string {
	return ROTATION_DESCRIPTION
}

// IsActive implements orchestrator.BatchWorker.
func (r *rotationWorker) IsActive() bool {
	return atomic.LoadInt32(&r.cancelled) == 0 && r.ctx != nil
}

// Name implements orchestrator.BatchWorker.
func (r *rotationWorker) Name() string {
	return ROTATION_NAME
}

// StartWorker implements orchestrator.BatchWorker.
func (r *rotationWorker) StartWorker(job *model.Job) (bool, error) {
	// Reset the cancelled flag
	atomic.StoreInt32(&r.cancelled, 0)

	ctx, cancelFunc := context.WithCancel(context.Background())
	r.ctx = &ctx
	r.cancelFunc = &cancelFunc
	r.jobID = &job.ID

	defer func() {
		// Clean up in case of panic or return
		r.ctx = nil
		r.cancelFunc = nil
		r.jobID = nil
	}()

	teams, err := r.db.ListTeams(r.SafeContext())
	if err != nil {
		return false, err
	}

	for _, team := range teams {
		metrics := r.processTeam(team)

		r.db.UpdateJobMetrics(r.SafeContext(), job.ID, metrics)
	}

	return false, nil
}

func (r *rotationWorker) processTeam(team model.Team) model.JobMetrics {
	metrics := model.JobMetrics{}

	playerNames := make([]string, len(team.Players))

	for _, teamPlayer := range team.Players {
		playerNames = append(playerNames, teamPlayer.LiveServerIGN)
	}

	playersData, err := r.pubgClient.GetPlayersByNames(pubg.SteamPlatform, playerNames)

	if err != nil {
		log.Error().Err(err).Msg("could not pull player data by names")
		metrics.FailureCount += 1

		return metrics
	}

	matchIDMap := make(map[string]bool)

	for _, player := range playersData.Data {
		for _, playerMatch := range player.Relationships.Matches.Data {
			matchIDMap[playerMatch.ID] = true
		}
	}

	metrics.ProcessedItems += len(matchIDMap)

	

	return metrics
}

// Type implements orchestrator.BatchWorker.
func (r *rotationWorker) Type() string {
	return ROTATION_TYPE
}

func (r *rotationWorker) isCancelled() bool {
	return !r.IsActive()
}

// SafeContext returns the current context if available or a cancelled context if not
func (r *rotationWorker) SafeContext() context.Context {
	if r.isCancelled() || r.ctx == nil {
		// Return a cancelled context if worker is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return *r.ctx
}
