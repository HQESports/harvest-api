package pubg

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// PUBGPlayerResponse is the root structure for player data from PUBG API
type PUBGPlayerResponse struct {
	Data  []PlayerData           `json:"data"`
	Links Links                  `json:"links"`
	Meta  map[string]interface{} `json:"meta"`
}

// PlayerData represents information about a player
type PlayerData struct {
	Type          string              `json:"type"`
	ID            string              `json:"id"`
	Attributes    PlayerAttributes    `json:"attributes"`
	Relationships PlayerRelationships `json:"relationships"`
	Links         SelfLinks           `json:"links"`
}

// PlayerAttributes contains player details
type PlayerAttributes struct {
	BanType      string      `json:"banType"`
	ClanID       string      `json:"clanId"`
	Name         string      `json:"name"`
	Stats        interface{} `json:"stats"`
	TitleID      string      `json:"titleId"`
	ShardID      string      `json:"shardId"`
	PatchVersion string      `json:"patchVersion"`
}

// PlayerRelationships represents related data
type PlayerRelationships struct {
	Assets  RelationshipData `json:"assets"`
	Matches RelationshipData `json:"matches"`
}

// RelationshipData contains arrays of related objects
type RelationshipData struct {
	Data []RelatedItem `json:"data"`
}

// RelatedItem represents a reference to another object
type RelatedItem struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// SelfLinks contains links for the entity
type SelfLinks struct {
	Self   string `json:"self"`
	Schema string `json:"schema"`
}

// Links contains navigation links
type Links struct {
	Self string `json:"self"`
}

// GetPlayersByNames retrieves data for up to 10 players by their in-game names from a specific shard
func (c *Client) GetPlayersByNames(shard string, playerNames []string) (*PUBGPlayerResponse, error) {
	operationID := fmt.Sprintf("get_players_by_names_%d", time.Now().Unix())
	startTime := time.Now()

	log.Info().
		Str("operation_id", operationID).
		Str("shard", shard).
		Strs("player_names", playerNames).
		Int("player_count", len(playerNames)).
		Msg("Starting GetPlayersByNames operation")

	// Check if we have any player names
	if len(playerNames) == 0 {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Dur("duration", time.Since(startTime)).
			Msg("No player names provided")
		return nil, fmt.Errorf("no player names provided")
	}

	// PUBG API limits to 10 players per request
	if len(playerNames) > 10 {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Int("player_count", len(playerNames)).
			Dur("duration", time.Since(startTime)).
			Msg("Too many player names: maximum is 10")
		return nil, fmt.Errorf("too many player names: maximum is 10, got %d", len(playerNames))
	}

	// Validate shard
	if shard == "" {
		log.Error().
			Str("operation_id", operationID).
			Dur("duration", time.Since(startTime)).
			Msg("Shard cannot be empty")
		return nil, fmt.Errorf("shard cannot be empty")
	}

	// Join player names with commas for the API query
	playerNamesParam := ""
	for i, name := range playerNames {
		if i > 0 {
			playerNamesParam += ","
		}
		playerNamesParam += name
	}

	endpoint := fmt.Sprintf("/shards/%s/players?filter[playerNames]=%s", shard, playerNamesParam)

	log.Debug().
		Str("operation_id", operationID).
		Str("endpoint", endpoint).
		Dur("prep_duration", time.Since(startTime)).
		Msg("Prepared API request for player names")

	apiStartTime := time.Now()
	respBody, err := c.RequestRateLimited(endpoint)
	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Str("endpoint", endpoint).
			Err(err).
			Dur("prep_duration", apiStartTime.Sub(startTime)).
			Dur("api_duration", time.Since(apiStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error getting player data from API")
		return nil, fmt.Errorf("error getting player data: %w", err)
	}

	unmarshalStartTime := time.Now()
	var playerResponse PUBGPlayerResponse
	if err := json.Unmarshal(respBody, &playerResponse); err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Str("endpoint", endpoint).
			Err(err).
			Int("response_size", len(respBody)).
			Dur("prep_duration", apiStartTime.Sub(startTime)).
			Dur("api_duration", unmarshalStartTime.Sub(apiStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error unmarshaling player data")
		return nil, fmt.Errorf("error unmarshaling player data: %w", err)
	}

	// Check if at least one player was found
	if len(playerResponse.Data) == 0 {
		log.Warn().
			Str("operation_id", operationID).
			Str("shard", shard).
			Strs("player_names", playerNames).
			Dur("prep_duration", apiStartTime.Sub(startTime)).
			Dur("api_duration", unmarshalStartTime.Sub(apiStartTime)).
			Dur("unmarshal_duration", time.Since(unmarshalStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("No players found for the given names")
		return nil, fmt.Errorf("no players found for the given names")
	}

	playerMatchCounts := make(map[string]int)
	for _, player := range playerResponse.Data {
		playerMatchCounts[player.Attributes.Name] = len(player.Relationships.Matches.Data)
	}

	log.Info().
		Str("operation_id", operationID).
		Str("shard", shard).
		Int("players_requested", len(playerNames)).
		Int("players_found", len(playerResponse.Data)).
		Interface("player_match_counts", playerMatchCounts).
		Dur("prep_duration", apiStartTime.Sub(startTime)).
		Dur("api_duration", unmarshalStartTime.Sub(apiStartTime)).
		Dur("unmarshal_duration", time.Since(unmarshalStartTime)).
		Dur("total_duration", time.Since(startTime)).
		Msg("Successfully retrieved player data by names")

	return &playerResponse, nil
}

// GetUniqueMatchIDs returns a slice of unique match IDs from a player response
func (p *PUBGPlayerResponse) GetUniqueMatchIDs() []string {
	startTime := time.Now()

	// Create a map to track unique match IDs
	uniqueMatches := make(map[string]bool)
	totalMatchReferences := 0

	// Go through each player in the response
	for _, player := range p.Data {
		// Go through each match in the player's matches relationship
		if matches := player.Relationships.Matches.Data; matches != nil {
			totalMatchReferences += len(matches)
			for _, match := range matches {
				// Only add match IDs of type "match"
				if match.Type == "match" {
					uniqueMatches[match.ID] = true
				}
			}
		}
	}

	// Convert map keys to a slice
	result := make([]string, 0, len(uniqueMatches))
	for id := range uniqueMatches {
		result = append(result, id)
	}

	log.Debug().
		Int("player_count", len(p.Data)).
		Int("total_match_references", totalMatchReferences).
		Int("unique_match_count", len(result)).
		Dur("duration", time.Since(startTime)).
		Msg("Extracted unique match IDs from player response")

	return result
}

// GetPlayersByIDs retrieves data for up to 10 players by their account IDs from a specific shard
func (c *Client) GetPlayersByIDs(shard string, playerIDs []string) (*PUBGPlayerResponse, error) {
	operationID := fmt.Sprintf("get_players_by_ids_%d", time.Now().Unix())
	startTime := time.Now()

	log.Info().
		Str("operation_id", operationID).
		Str("shard", shard).
		Strs("player_ids", playerIDs).
		Int("player_count", len(playerIDs)).
		Msg("Starting GetPlayersByIDs operation")

	// Check if we have any player IDs
	if len(playerIDs) == 0 {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Dur("duration", time.Since(startTime)).
			Msg("No player IDs provided")
		return nil, fmt.Errorf("no player IDs provided")
	}

	// PUBG API limits to 10 players per request
	if len(playerIDs) > 10 {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Int("player_count", len(playerIDs)).
			Dur("duration", time.Since(startTime)).
			Msg("Too many player IDs: maximum is 10")
		return nil, fmt.Errorf("too many player IDs: maximum is 10, got %d", len(playerIDs))
	}

	// Validate shard
	if shard == "" {
		log.Error().
			Str("operation_id", operationID).
			Dur("duration", time.Since(startTime)).
			Msg("Shard cannot be empty")
		return nil, fmt.Errorf("shard cannot be empty")
	}

	// Join player IDs with commas for the API query
	playerIDsParam := ""
	for i, id := range playerIDs {
		if i > 0 {
			playerIDsParam += ","
		}
		playerIDsParam += id
	}

	endpoint := fmt.Sprintf("/shards/%s/players?filter[playerIds]=%s", shard, playerIDsParam)

	log.Debug().
		Str("operation_id", operationID).
		Str("endpoint", endpoint).
		Dur("prep_duration", time.Since(startTime)).
		Msg("Prepared API request for player IDs")

	apiStartTime := time.Now()
	respBody, err := c.RequestRateLimited(endpoint)
	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Str("endpoint", endpoint).
			Err(err).
			Dur("prep_duration", apiStartTime.Sub(startTime)).
			Dur("api_duration", time.Since(apiStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error getting player data from API")
		return nil, fmt.Errorf("error getting player data: %w", err)
	}

	unmarshalStartTime := time.Now()
	var playerResponse PUBGPlayerResponse
	if err := json.Unmarshal(respBody, &playerResponse); err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Str("endpoint", endpoint).
			Err(err).
			Int("response_size", len(respBody)).
			Dur("prep_duration", apiStartTime.Sub(startTime)).
			Dur("api_duration", unmarshalStartTime.Sub(apiStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error unmarshaling player data")
		return nil, fmt.Errorf("error unmarshaling player data: %w", err)
	}

	// Check if at least one player was found
	if len(playerResponse.Data) == 0 {
		log.Warn().
			Str("operation_id", operationID).
			Str("shard", shard).
			Strs("player_ids", playerIDs).
			Dur("prep_duration", apiStartTime.Sub(startTime)).
			Dur("api_duration", unmarshalStartTime.Sub(apiStartTime)).
			Dur("unmarshal_duration", time.Since(unmarshalStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("No players found for the given IDs")
		return nil, fmt.Errorf("no players found for the given IDs")
	}

	playerMatchCounts := make(map[string]int)
	for _, player := range playerResponse.Data {
		playerMatchCounts[player.ID] = len(player.Relationships.Matches.Data)
	}

	log.Info().
		Str("operation_id", operationID).
		Str("shard", shard).
		Int("players_requested", len(playerIDs)).
		Int("players_found", len(playerResponse.Data)).
		Interface("player_match_counts", playerMatchCounts).
		Dur("prep_duration", apiStartTime.Sub(startTime)).
		Dur("api_duration", unmarshalStartTime.Sub(apiStartTime)).
		Dur("unmarshal_duration", time.Since(unmarshalStartTime)).
		Dur("total_duration", time.Since(startTime)).
		Msg("Successfully retrieved player data by IDs")

	return &playerResponse, nil
}

// GetPlayerIDs returns a map of player names to their IDs handling batches of 10 at a time
func (c *Client) GetPlayerIDs(shard string, playerNames []string) (map[string]string, error) {
	jobID := fmt.Sprintf("get_player_ids_%d", time.Now().Unix())
	startTime := time.Now()

	log.Info().
		Str("job_id", jobID).
		Str("shard", shard).
		Int("player_count", len(playerNames)).
		Msg("Starting GetPlayerIDs batch operation")

	// Validate inputs
	if shard == "" {
		log.Error().
			Str("job_id", jobID).
			Dur("duration", time.Since(startTime)).
			Msg("Shard cannot be empty")
		return nil, fmt.Errorf("shard cannot be empty")
	}

	if len(playerNames) == 0 {
		log.Error().
			Str("job_id", jobID).
			Str("shard", shard).
			Dur("duration", time.Since(startTime)).
			Msg("No player names provided")
		return nil, fmt.Errorf("no player names provided")
	}

	// Create a map to store player names and their IDs
	playerIDMap := make(map[string]string)
	totalBatches := (len(playerNames) + 9) / 10 // Ceiling division to get batch count

	log.Info().
		Str("job_id", jobID).
		Str("shard", shard).
		Int("total_players", len(playerNames)).
		Int("total_batches", totalBatches).
		Msg("Processing player names in batches")

	// Process player names in batches of 10
	for i := 0; i < len(playerNames); i += 10 {
		batchStartTime := time.Now()
		batchNum := i/10 + 1

		// Calculate the end index for current batch
		end := i + 10
		if end > len(playerNames) {
			end = len(playerNames)
		}

		// Get the current batch of player names
		batchNames := playerNames[i:end]

		log.Debug().
			Str("job_id", jobID).
			Int("batch_num", batchNum).
			Int("batch_size", len(batchNames)).
			Int("batch_start_idx", i).
			Int("batch_end_idx", end-1).
			Strs("batch_names", batchNames).
			Msg("Processing player name batch")

		// Use the existing GetPlayersByNames function to fetch player data for this batch
		playerResp, err := c.GetPlayersByNames(shard, batchNames)
		if err != nil {
			log.Error().
				Str("job_id", jobID).
				Int("batch_num", batchNum).
				Err(err).
				Int("batch_size", len(batchNames)).
				Int("batch_start_idx", i).
				Int("batch_end_idx", end-1).
				Dur("batch_duration", time.Since(batchStartTime)).
				Dur("total_duration", time.Since(startTime)).
				Msg("Error getting player data for batch")
			return nil, fmt.Errorf("error getting player data for batch %d-%d: %w", i, end-1, err)
		}

		// Populate the map with player names and their corresponding IDs from this batch
		batchFoundCount := 0
		for _, player := range playerResp.Data {
			playerIDMap[player.Attributes.Name] = player.ID
			batchFoundCount++
		}

		log.Info().
			Str("job_id", jobID).
			Int("batch_num", batchNum).
			Int("total_batches", totalBatches).
			Int("batch_size", len(batchNames)).
			Int("players_found", batchFoundCount).
			Float64("completion_percentage", float64(batchNum)/float64(totalBatches)*100).
			Dur("batch_duration", time.Since(batchStartTime)).
			Dur("elapsed_duration", time.Since(startTime)).
			Msg("Completed player name batch processing")
	}

	log.Info().
		Str("job_id", jobID).
		Str("shard", shard).
		Int("total_players_requested", len(playerNames)).
		Int("total_players_found", len(playerIDMap)).
		Dur("total_duration", time.Since(startTime)).
		Float64("players_per_second", float64(len(playerNames))/time.Since(startTime).Seconds()).
		Msg("Completed GetPlayerIDs batch operation")

	return playerIDMap, nil
}

// GetMatchIDsForPlayers retrieves all unique match IDs for a list of players
// by batching requests in groups of 10 players (PUBG API limit)
func (c *Client) GetMatchIDsForPlayers(shard string, playerIDs []string) ([]string, error) {
	jobID := fmt.Sprintf("get_match_ids_%d", time.Now().Unix())
	startTime := time.Now()

	log.Info().
		Str("job_id", jobID).
		Str("shard", shard).
		Int("player_count", len(playerIDs)).
		Msg("Starting GetMatchIDsForPlayers batch operation")

	// Validate shard
	if shard == "" {
		log.Error().
			Str("job_id", jobID).
			Dur("duration", time.Since(startTime)).
			Msg("Shard cannot be empty")
		return nil, fmt.Errorf("shard cannot be empty")
	}

	// Check if we have any player IDs
	if len(playerIDs) == 0 {
		log.Error().
			Str("job_id", jobID).
			Str("shard", shard).
			Dur("duration", time.Since(startTime)).
			Msg("No player IDs provided")
		return nil, fmt.Errorf("no player IDs provided")
	}

	// Create a map to track unique match IDs
	uniqueMatchIDs := make(map[string]struct{})
	totalBatches := (len(playerIDs) + 9) / 10 // Ceiling division to get batch count

	log.Info().
		Str("job_id", jobID).
		Str("shard", shard).
		Int("total_players", len(playerIDs)).
		Int("total_batches", totalBatches).
		Msg("Processing player IDs in batches")

	successfulBatches := 0
	skippedPlayers := 0
	totalMatchReferences := 0

	// Process player IDs in batches of 10
	for i := 0; i < len(playerIDs); i += 10 {
		batchStartTime := time.Now()
		batchNum := i/10 + 1

		// Calculate end index for current batch
		end := i + 10
		if end > len(playerIDs) {
			end = len(playerIDs)
		}

		// Create batch of up to 10 player IDs
		batch := playerIDs[i:end]

		log.Debug().
			Str("job_id", jobID).
			Int("batch_num", batchNum).
			Int("batch_size", len(batch)).
			Int("batch_start_idx", i).
			Int("batch_end_idx", end-1).
			Strs("batch_ids", batch).
			Msg("Processing player ID batch")

		// Get player data for the current batch using the rate-limited client
		response, err := c.GetPlayersByIDs(shard, batch)
		if err != nil {
			log.Error().
				Str("job_id", jobID).
				Int("batch_num", batchNum).
				Err(err).
				Int("batch_size", len(batch)).
				Int("batch_start_idx", i).
				Int("batch_end_idx", end-1).
				Dur("batch_duration", time.Since(batchStartTime)).
				Dur("total_duration", time.Since(startTime)).
				Msg("Error getting data for player ID batch")
			return nil, err
		}

		successfulBatches++
		batchMatchCount := 0
		playersWithNoMatches := 0

		// Extract match IDs from each player in the response
		for _, player := range response.Data {
			// Skip if the player has no relationships or matches
			if len(player.Relationships.Matches.Data) < 1 {
				playersWithNoMatches++
				skippedPlayers++
				continue
			}

			playerMatchCount := 0
			// Add each match ID to our unique set
			for _, match := range player.Relationships.Matches.Data {
				uniqueMatchIDs[match.ID] = struct{}{}
				playerMatchCount++
				batchMatchCount++
				totalMatchReferences++
			}

			log.Trace().
				Str("job_id", jobID).
				Str("player_id", player.ID).
				Str("player_name", player.Attributes.Name).
				Int("match_count", playerMatchCount).
				Msg("Processed player matches")
		}

		log.Info().
			Str("job_id", jobID).
			Int("batch_num", batchNum).
			Int("total_batches", totalBatches).
			Int("batch_size", len(batch)).
			Int("batch_match_count", batchMatchCount).
			Int("players_with_no_matches", playersWithNoMatches).
			Int("current_unique_matches", len(uniqueMatchIDs)).
			Float64("completion_percentage", float64(batchNum)/float64(totalBatches)*100).
			Dur("batch_duration", time.Since(batchStartTime)).
			Dur("elapsed_duration", time.Since(startTime)).
			Msg("Completed player ID batch processing")
	}

	// Convert map of unique match IDs to slice
	matchIDs := make([]string, 0, len(uniqueMatchIDs))
	for id := range uniqueMatchIDs {
		matchIDs = append(matchIDs, id)
	}

	log.Info().
		Str("job_id", jobID).
		Str("shard", shard).
		Int("total_players", len(playerIDs)).
		Int("players_with_matches", len(playerIDs)-skippedPlayers).
		Int("players_without_matches", skippedPlayers).
		Int("total_batches", totalBatches).
		Int("successful_batches", successfulBatches).
		Int("total_match_references", totalMatchReferences).
		Int("unique_match_ids", len(matchIDs)).
		Float64("duplication_rate", float64(totalMatchReferences-len(matchIDs))/float64(totalMatchReferences)).
		Dur("total_duration", time.Since(startTime)).
		Float64("players_per_second", float64(len(playerIDs))/time.Since(startTime).Seconds()).
		Float64("matches_per_second", float64(len(matchIDs))/time.Since(startTime).Seconds()).
		Msg("Completed GetMatchIDsForPlayers batch operation")

	return matchIDs, nil
}
