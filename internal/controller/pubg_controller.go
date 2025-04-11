package controller

import (
	"context"
	"fmt"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/pkg/pubg"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

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
	SearchAndExpandPlayers(int) (int, error)
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
	err = pc.db.BulkUpsertEntities(context.Background(), "players", entities)
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

// SearchAndExpandPlayers searches for matches and extracts new players
func (pc *pubgController) SearchAndExpandPlayers(limit int) (int, error) {
	jobID := fmt.Sprintf("expand_players_%d", time.Now().Unix())
	log.Info().Str("job_id", jobID).Msg("Starting search and expand players job")

	matchIDs, shard, err := pc.getLiveMatchIDsAndShard(limit)
	if err != nil {
		log.Error().Str("job_id", jobID).Err(err).Msg("Error getting live match IDs")
		return -1, err
	}

	metrics := NewJobMetrics(len(matchIDs))
	log.Info().
		Str("job_id", jobID).
		Int("match_count", len(matchIDs)).
		Str("shard", shard).
		Msg("Processing matches for player expansion")

	// Define concurrency limit
	const MaxConcurrentRequests = 20

	// Create semaphore to limit concurrency
	semaphore := make(chan struct{}, MaxConcurrentRequests)

	// Create wait group to wait for all goroutines
	var wg sync.WaitGroup

	// Error channel to collect errors
	errorCh := make(chan error, len(matchIDs))

	// Create a mutex for the player counter
	var mu sync.Mutex
	playerCnt := 0

	// Progress tracking ticker
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				metrics.LogProgress(fmt.Sprintf("%s_expand_players", jobID))
			case <-context.Background().Done():
				ticker.Stop()
				return
			}
		}
	}()

	// Process each match ID concurrently with controlled concurrency
	for _, id := range matchIDs {
		wg.Add(1)

		// Capture id in closure to avoid race conditions
		matchID := id

		go func() {
			defer wg.Done()

			// Acquire semaphore slot
			semaphore <- struct{}{}

			// Release semaphore when done
			defer func() { <-semaphore }()

			// Get match data
			match, err := pc.client.GetMatch(shard, matchID)
			if err != nil {
				log.Debug().Str("match_id", matchID).Err(err).Msg("Error getting match data")
				errorCh <- err
				metrics.FailedItems.Add(1)
				metrics.ProcessedItems.Add(1)
				return
			}

			// Process player data
			players := make([]model.Entity, 0, 100)
			for _, inc := range match.Included {
				if inc.Type == "participant" {
					//playerId
					var playerName, accountID string

					if name, ok := inc.Attributes["stats"].(map[string]interface{})["name"].(string); ok {
						playerName = name
					} else {
						continue
					}

					if id, ok := inc.Attributes["stats"].(map[string]interface{})["playerId"].(string); ok {
						accountID = id
					} else {
						continue
					}

					// Bot accounts start with the words "ai"
					if startsWithAI(playerName) {
						continue
					}

					if playerName == "" {
						continue
					}

					entity := model.Entity{
						ID:     accountID,
						Name:   playerName,
						Active: true,
					}
					players = append(players, entity)
				}
			}

			// Skip DB operation if no players found
			if len(players) == 0 {
				metrics.ProcessedItems.Add(1)
				return
			}

			// Save players to database
			err = pc.db.BulkUpsertEntities(context.Background(), "players", players)
			if err != nil {
				log.Error().Str("match_id", matchID).Err(err).Msg("Error bulk upserting players")
				errorCh <- err
				metrics.FailedItems.Add(1)
				metrics.ProcessedItems.Add(1)
				return
			}

			// Update player count with mutex to avoid race conditions
			mu.Lock()
			playerCnt += len(players)
			mu.Unlock()

			metrics.SuccessfulItems.Add(1)
			metrics.ProcessedItems.Add(1)
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Stop the ticker
	ticker.Stop()

	// Close error channel
	close(errorCh)

	// Check if there were any errors
	var lastErr error
	for err := range errorCh {
		lastErr = err
	}

	metrics.Complete(fmt.Sprintf("%s_expand_players", jobID))

	log.Info().
		Str("job_id", jobID).
		Int("total_players", playerCnt).
		Int("matches_processed", len(matchIDs)).
		Int("successful_matches", int(metrics.SuccessfulItems.Load())).
		Int("failed_matches", int(metrics.FailedItems.Load())).
		Dur("duration", time.Since(metrics.StartTime)).
		Msg("Search and expand players job completed")

	return playerCnt, lastErr
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
	err = pc.db.BulkUpsertEntities(context.Background(), "tournaments", entities)
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

// Helper methods
func (pc *pubgController) getMatchIDsAndShard(mf MatchFilter) ([]string, string, error) {
	// If match type isn't live, search for matchIDs across the tournaments
	if mf.MatchType == "event" {
		return pc.getEventMatchIDsAndShard(mf.Limit)
	}

	return pc.getLiveMatchIDsAndShard(mf.Limit)
}

func (pc *pubgController) getEventMatchIDsAndShard(limit int) ([]string, string, error) {
	tournaments, err := pc.db.GetActiveTournaments(context.Background(), limit)

	if err != nil {
		log.Error().Err(err).Msg("Error getting active tournaments")
		return nil, "N/A", err
	}

	tournamentIDs := make([]string, 0, len(tournaments))
	for _, tournament := range tournaments {
		tournamentIDs = append(tournamentIDs, tournament.ID)
	}

	matchIDs := make([]string, 0)
	for _, tournamentID := range tournamentIDs {
		tournamentMatchIDs, err := pc.client.GetMatchIDsByTournamentID(tournamentID)
		if err != nil {
			log.Debug().Str("tournament_id", tournamentID).Err(err).Msg("Error getting match IDs by tournament")
		}

		matchIDs = append(matchIDs, tournamentMatchIDs...)
	}

	return matchIDs, pubg.EventPlatform, nil
}

// Based on match filter return list of match IDs and the shard they correspond to
func (pc *pubgController) getLiveMatchIDsAndShard(limit int) ([]string, string, error) {
	players, err := pc.db.GetActivePlayers(context.Background(), limit)

	if err != nil {
		log.Error().Err(err).Msg("Error getting active players")
		return nil, "N/A", err
	}

	playerIDs := make([]string, 0, len(players))
	for _, player := range players {
		playerIDs = append(playerIDs, player.ID)
	}

	matchIDs, err := pc.client.GetMatchIDsForPlayers(pubg.SteamPlatform, playerIDs)
	if err != nil {
		log.Error().Err(err).Msg("Error getting matchIDs from PlayerIDs")
		return nil, "N/A", err
	}

	return matchIDs, pubg.SteamPlatform, nil
}

func startsWithAI(s string) bool {
	// Trim leading whitespace
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)

	// Check if the string starts with "ai" followed by a space or end of string
	if len(trimmed) >= 2 && strings.ToLower(trimmed[:2]) == "ai" {
		return true
	}

	return false
}
