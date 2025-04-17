package worker

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/orchestrator"
	"harvest/pkg/pubg"
	"sync"
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
}

// Cancel implements job.BatchWorker.
func (p *MatchExpanderWorker) Cancel() error {
	cancelFunc := *p.cancelFunc
	cancelFunc()

	p.ctx = nil
	p.cancelFunc = nil

	return nil
}

// IsActive implements job.BatchWorker.
func (p *MatchExpanderWorker) IsActive() bool {
	return p.ctx != nil
}

func (p *MatchExpanderWorker) ActiveJobID() *primitive.ObjectID {
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

// StartWorker implements job.BatchWorker.
// Pull in all players and create 10 batches where each batch contains 10 batches of player IDs
// Process each batch of 10 players ID concurrently
func (p *MatchExpanderWorker) StartWorker(job *model.Job) (bool, error) {
	ctx, cacnelFunc := context.WithCancel(context.Background())
	p.ctx = &ctx
	p.cancelFunc = &cacnelFunc
	p.jobID = &job.ID

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
		if cancelCtx == nil {
			log.Warn().Msg("worker has been canceled, stoping process")
			return true, nil
		}
		p.ProcessBatch(batch, job.ID)

		err = p.db.IncrementJobBatchesComplete(*p.ctx, job.ID, 1)
		if err != nil {
			log.Error().Err(err).Msg("Error incrementing batches completes")
		}
	}

	p.ctx = nil
	p.cancelFunc = nil
	p.jobID = nil

	return false, nil
}

func (p *MatchExpanderWorker) ProcessBatch(batch []string, jobID primitive.ObjectID) error {
	matchIDs, err := p.pubgClient.GetMatchIDsForPlayers(pubg.SteamPlatform, batch)
	if err != nil {
		log.Error().Err(err).Msg("Error getting match IDs for players")
	}
	log.Info().Int("# Matches", len(matchIDs)).Msg("Found matches")

	matchIDBatches := orchestrator.SplitIntoBatches(matchIDs, 40)

	for _, matchIDBatch := range matchIDBatches {
		var wg sync.WaitGroup
		wg.Add(len(matchIDBatch))

		// Create metrics for this batch
		metrics := model.JobMetrics{}

		// Use mutex to protect concurrent access to metrics
		var mutex sync.Mutex

		// Match document out slice to bulk up
		matchDocumentSlice := make([]model.Match, 0, 300)

		for _, matchID := range matchIDBatch {
			canceledCtx := *p.ctx
			if canceledCtx == nil {
				log.Warn().Msg("worker has been cancelled stopping batch processing")
				break
			}
			go func(id string) {
				defer wg.Done()
				matchDocument, err := p.BuildMatchDocument(id)
				if err != nil {
					log.Error().Err(err).Str("Match ID", id).Msg("Could not process match ID")
					mutex.Lock()
					metrics.FailureCount++
					mutex.Unlock()
					return
				}

				mutex.Lock()
				matchDocumentSlice = append(matchDocumentSlice, *matchDocument)
				mutex.Unlock()

			}(matchID) // Pass matchID as parameter to avoid closure issues
		}

		wg.Wait()
		ctx := *p.ctx
		if ctx == nil {
			log.Error().Msg("context is nil, worker cancelled")
			return nil
		}

		bulkResult, err := p.db.BulkImportMatches(ctx, matchDocumentSlice)
		if err != nil {
			log.Error().Err(err).Msg("could not bulk upsert matches")
		}

		metrics.ProcessedItems += len(matchDocumentSlice) + metrics.FailureCount
		metrics.SuccessCount += bulkResult.SuccessCount
		metrics.WarningCount += bulkResult.DuplicateCount

		// Now it's safe to update the metrics in the database
		err = p.db.UpdateJobMetrics(ctx, jobID, metrics)
		if err != nil {
			log.Error().Err(err).Msg("Error updating job metrics")
		}
	}

	return nil
}

func (p *MatchExpanderWorker) BuildMatchDocument(matchID string) (*model.Match, error) {
	ctx := *p.ctx
	if ctx.Err() != nil { // Check if job has been cancelled
		return nil, ctx.Err()
	}

	match, err := p.pubgClient.GetMatch(pubg.SteamPlatform, matchID)
	if err != nil {
		return nil, fmt.Errorf("could not get match: %v", err)
	}

	if !match.IsValidMatch() {
		return nil, fmt.Errorf("not a valid match")
	}

	// Parse the match creation time
	createdAt, err := time.Parse(time.RFC3339, match.Data.Attributes.CreatedAt)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse match creation time")
		return nil, err
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

	return &matchDocument, nil
}

func NewMatchExpanderWorker(pubclient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &MatchExpanderWorker{
		pubgClient: pubclient,
		db:         db,
		ctx:        nil,
		cancelFunc: nil,
	}
}
