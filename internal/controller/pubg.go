package controller

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/pkg/pubg"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// MatchFilter represents the filtering criteria for PUBG matches
type MatchFilter struct {
	MapName   string `json:"mapName"`
	MatchType string `json:"matchType"`
	Limit     int    `json:"limit"`
	// Future fields can be added here
}

// JobMetrics tracks metrics for long-running operations
type JobMetrics struct {
	TotalItems      int
	ProcessedItems  atomic.Int32
	SuccessfulItems atomic.Int32
	FailedItems     atomic.Int32
	StartTime       time.Time
	EndTime         time.Time
}

// NewJobMetrics creates a new JobMetrics instance
func NewJobMetrics(totalItems int) *JobMetrics {
	return &JobMetrics{
		TotalItems: totalItems,
		StartTime:  time.Now(),
	}
}

// LogProgress logs the current progress of the job
func (jm *JobMetrics) LogProgress(jobName string) {
	processed := jm.ProcessedItems.Load()
	successful := jm.SuccessfulItems.Load()
	failed := jm.FailedItems.Load()

	elapsed := time.Since(jm.StartTime)
	var itemsPerSecond float64
	if elapsed.Seconds() > 0 {
		itemsPerSecond = float64(processed) / elapsed.Seconds()
	}

	percentComplete := 0.0
	if jm.TotalItems > 0 {
		percentComplete = float64(processed) / float64(jm.TotalItems) * 100
	}

	log.Info().
		Str("job", jobName).
		Int32("processed", processed).
		Int32("successful", successful).
		Int32("failed", failed).
		Int("total", jm.TotalItems).
		Float64("percent_complete", percentComplete).
		Float64("items_per_second", itemsPerSecond).
		Msg("Job progress")
}

// Complete marks the job as complete and logs final metrics
func (jm *JobMetrics) Complete(jobName string) {
	jm.EndTime = time.Now()
	elapsed := jm.EndTime.Sub(jm.StartTime)

	processed := jm.ProcessedItems.Load()
	successful := jm.SuccessfulItems.Load()
	failed := jm.FailedItems.Load()

	log.Info().
		Str("job", jobName).
		Int32("processed", processed).
		Int32("successful", successful).
		Int32("failed", failed).
		Int("total", jm.TotalItems).
		Dur("duration", elapsed).
		Float64("items_per_second", float64(processed)/elapsed.Seconds()).
		Msg("Job completed")
}

type PubgController interface {
	CreatePlayers([]string) (int, error)
	CreateTournaments() (int, error)
}

type pubgController struct {
	db     database.Database
	client pubg.Client
}

func NewPUBG(db database.Database, client pubg.Client) PubgController {
	return &pubgController{
		db:     db,
		client: client,
	}
}

func (pc *pubgController) CreatePlayers(names []string) (int, error) {
	jobID := fmt.Sprintf("create_players_%d", time.Now().Unix())
	log.Info().
		Str("job_id", jobID).
		Int("player_count", len(names)).
		Msg("Starting player creation job")

	startTime := time.Now()

	// Get player IDs from the PUBG API
	idMap, err := pc.client.GetPlayerIDs(pubg.SteamPlatform, names)
	if err != nil {
		log.Error().Str("job_id", jobID).Err(err).Msg("Failed to get player IDs")
		return 0, err
	}

	// Create entities for bulk insertion
	entities := make([]model.Entity, 0, len(idMap))
	for player, id := range idMap {
		entities = append(entities, model.Entity{
			ID:     id,
			Name:   player,
			Active: true,
		})
	}

	// Save the entities to the database
	_, err = pc.db.BulkUpsertPlayers(context.Background(), entities)
	if err != nil {
		log.Error().Str("job_id", jobID).Err(err).Msg("Failed to save player entities")
		return 0, err
	}

	log.Info().
		Str("job_id", jobID).
		Int("players_created", len(entities)).
		Dur("duration", time.Since(startTime)).
		Msg("Successfully created/updated players")

	return len(entities), nil
}

// CreateTournaments fetches tournament details and creates entity records
func (pc *pubgController) CreateTournaments() (int, error) {
	jobID := fmt.Sprintf("create_tournaments_%d", time.Now().Unix())
	log.Info().Str("job_id", jobID).Msg("Starting tournament creation job")
	startTime := time.Now()

	// Get tournaments from the PUBG API
	tournaments, err := pc.client.GetTournaments()
	if err != nil {
		log.Error().Str("job_id", jobID).Err(err).Msg("Failed to get tournaments")
		return 0, err
	}

	// Create entities for bulk insertion
	entities := make([]model.Entity, 0, len(tournaments.Data))
	for _, tournament := range tournaments.Data {
		// Use the helper function to build the tournament name
		tournamentName := pubg.BuildTournamentName(tournament.ID)

		entities = append(entities, model.Entity{
			ID:     tournament.ID,
			Name:   tournamentName,
			Active: true,
		})
	}

	// Save the entities to the database
	_, err = pc.db.BulkUpsertTournaments(context.Background(), entities)
	if err != nil {
		log.Error().Str("job_id", jobID).Err(err).Msg("Failed to save tournament entities")
		return 0, err
	}

	log.Info().
		Str("job_id", jobID).
		Int("tournaments_created", len(entities)).
		Dur("duration", time.Since(startTime)).
		Msg("Successfully created/updated tournaments")

	return len(entities), nil
}
