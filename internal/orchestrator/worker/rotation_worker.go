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
	ROTATION_TYPE        = "rotation_worker"
	ROTATION_NAME        = "Rotation Worker"
	ROTATION_DESCRIPTION = "Search through teams and pull live server rotations"
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

	r.db.SetJobTotalBatches(r.SafeContext(), job.ID, len(teams))
	for _, team := range teams {
		metrics := r.processTeam(team)

		r.db.UpdateJobMetrics(r.SafeContext(), job.ID, metrics)
		r.db.IncrementJobBatchesComplete(r.SafeContext(), job.ID, 1)

		if r.isCancelled() {
			return true, nil
		}
	}

	return false, nil
}

func (r *rotationWorker) processTeam(team model.Team) model.JobMetrics {
	metrics := model.JobMetrics{}
	if r.isCancelled() {
		return metrics
	}

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

	var wg sync.WaitGroup
	var mutex sync.Mutex

	wg.Add(len(matchIDMap))
	teamRotations := make([]model.TeamRotation, 0, len(matchIDMap))

	for matchID := range matchIDMap {
		if r.isCancelled() {
			return metrics
		}
		go func(matchID string) {
			if r.isCancelled() {
				return
			}
			defer wg.Done()

			match, err := r.pubgClient.GetMatch(pubg.SteamPlatform, matchID)
			matchType := match.GetMatchType(pubg.SteamPlatform)
			if matchType != "scrim" {
				log.Warn().Str("Match Type", matchType).Msg("invalid match type")
				metrics.InvalidCount += 1
				return
			}
			if err != nil {
				log.Error().Err(err).Msg("could not get pubg match")
			}
			matchMetrics, rotation := r.processMatch(team, match)

			mutex.Lock()
			if rotation != nil {
				teamRotations = append(teamRotations, *rotation)
			}
			metrics.FailureCount += matchMetrics.FailureCount
			metrics.InvalidCount += matchMetrics.InvalidCount
			mutex.Unlock()

		}(matchID)
	}

	wg.Wait()

	result, err := r.db.BulkCreateTeamRotations(r.SafeContext(), teamRotations)
	if err != nil {
		metrics.FailureCount += 1
		return metrics
	}

	metrics.SuccessCount += int(result.UpsertedCount)
	metrics.WarningCount += int(result.ModifiedCount)

	return metrics
}

func (r *rotationWorker) processMatch(team model.Team, match *pubg.PUBGMatchResponse) (model.JobMetrics, *model.TeamRotation) {
	metrics := model.JobMetrics{}
	if r.isCancelled() {
		return metrics, nil
	}
	matchDocument, err := orchestrator.BuildMatchDocument(pubg.SteamPlatform, *match)
	if err != nil {
		log.Error().Err(err).Msg("could not build match document")
		metrics.FailureCount += 1
		return metrics, nil
	}
	_, err = r.db.GetOrCreateMatch(r.SafeContext(), matchDocument)

	if err != nil {
		log.Error().Err(err).Msg("not able to build match document")
		metrics.FailureCount += 1
		return metrics, nil
	}

	// Succesfully created or found match

	playerNames := make([]string, 0, len(team.Players))
	for _, player := range team.Players {
		playerNames = append(playerNames, player.LiveServerIGN)
	}

	ok, _ := match.ArePlayersOnSameTeam(playerNames)

	if !ok {
		log.Warn().Msg("match found but players not on the same team")
		metrics.InvalidCount += 1
		return metrics, nil
	}

	if r.isCancelled() {
		return metrics, nil
	}

	rotations, err := r.pubgClient.BuildRotationsFromTelemetryYRL(r.SafeContext(), playerNames, matchDocument.TelemetryURL)
	if err != nil {
		metrics.FailureCount += 1
		return metrics, nil
	}

	teamRotation := model.TeamRotation{
		MatchID:         matchDocument.MatchID,
		TeamID:          team.ID,
		PlayerRotations: make([]model.PlayerRotation, 0, 4),
		CreatedAt:       time.Now(),
	}

	for playerName, rotation := range *rotations {
		playerPath := make([]model.Position, 0, len(rotation))
		for _, pos := range rotation {
			playerPath = append(playerPath, model.Position{
				X: int(pos.X),
				Y: int(pos.Y),
			})
		}
		playerRotation := model.PlayerRotation{
			Name:     playerName,
			Rotation: playerPath,
		}
		teamRotation.PlayerRotations = append(teamRotation.PlayerRotations, playerRotation)
	}

	return metrics, &teamRotation
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
