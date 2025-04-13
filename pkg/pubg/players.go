package pubg

import (
	"encoding/json"
	"fmt"

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
	// Validate inputs
	if len(playerNames) == 0 {
		return nil, fmt.Errorf("no player names provided")
	}

	if len(playerNames) > 10 {
		return nil, fmt.Errorf("too many player names: maximum is 10, got %d", len(playerNames))
	}

	if shard == "" {
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

	log.Info().
		Str("shard", shard).
		Int("player_count", len(playerNames)).
		Msg("Getting players by names")

	respBody, err := c.RequestRateLimited(endpoint)
	if err != nil {
		log.Error().
			Str("shard", shard).
			Err(err).
			Msg("Error getting player data")
		return nil, fmt.Errorf("error getting player data: %w", err)
	}

	var playerResponse PUBGPlayerResponse
	if err := json.Unmarshal(respBody, &playerResponse); err != nil {
		log.Error().
			Str("shard", shard).
			Err(err).
			Msg("Error unmarshaling player data")
		return nil, fmt.Errorf("error unmarshaling player data: %w", err)
	}

	// Check if at least one player was found
	if len(playerResponse.Data) == 0 {
		log.Warn().
			Str("shard", shard).
			Strs("player_names", playerNames).
			Msg("No players found for the given names")
		return nil, fmt.Errorf("no players found for the given names")
	}

	log.Info().
		Str("shard", shard).
		Int("players_found", len(playerResponse.Data)).
		Msg("Retrieved player data by names")

	return &playerResponse, nil
}

// GetUniqueMatchIDs returns a slice of unique match IDs from a player response
func (p *PUBGPlayerResponse) GetUniqueMatchIDs() []string {
	// Create a map to track unique match IDs
	uniqueMatches := make(map[string]bool)

	// Go through each player in the response
	for _, player := range p.Data {
		// Go through each match in the player's matches relationship
		if matches := player.Relationships.Matches.Data; matches != nil {
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

	return result
}

// GetPlayersByIDs retrieves data for up to 10 players by their account IDs from a specific shard
func (c *Client) GetPlayersByIDs(shard string, playerIDs []string) (*PUBGPlayerResponse, error) {
	// Validate inputs
	if len(playerIDs) == 0 {
		return nil, fmt.Errorf("no player IDs provided")
	}

	if len(playerIDs) > 10 {
		return nil, fmt.Errorf("too many player IDs: maximum is 10, got %d", len(playerIDs))
	}

	if shard == "" {
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

	log.Info().
		Str("shard", shard).
		Int("player_count", len(playerIDs)).
		Msg("Getting players by IDs")

	respBody, err := c.RequestRateLimited(endpoint)
	if err != nil {
		log.Error().
			Str("shard", shard).
			Err(err).
			Msg("Error getting player data")
		return nil, fmt.Errorf("error getting player data: %w", err)
	}

	var playerResponse PUBGPlayerResponse
	if err := json.Unmarshal(respBody, &playerResponse); err != nil {
		log.Error().
			Str("shard", shard).
			Err(err).
			Msg("Error unmarshaling player data")
		return nil, fmt.Errorf("error unmarshaling player data: %w", err)
	}

	// Check if at least one player was found
	if len(playerResponse.Data) == 0 {
		log.Warn().
			Str("shard", shard).
			Strs("player_ids", playerIDs).
			Msg("No players found for the given IDs")
		return nil, fmt.Errorf("no players found for the given IDs")
	}

	log.Info().
		Str("shard", shard).
		Int("players_found", len(playerResponse.Data)).
		Msg("Retrieved player data by IDs")

	return &playerResponse, nil
}

// GetPlayerIDs returns a map of player names to their IDs handling batches of 10 at a time
func (c *Client) GetPlayerIDs(shard string, playerNames []string) (map[string]string, error) {
	// Validate inputs
	if shard == "" {
		return nil, fmt.Errorf("shard cannot be empty")
	}

	if len(playerNames) == 0 {
		return nil, fmt.Errorf("no player names provided")
	}

	log.Info().
		Str("shard", shard).
		Int("total_players", len(playerNames)).
		Msg("Getting player IDs in batches")

	// Create a map to store player names and their IDs
	playerIDMap := make(map[string]string)

	// Process player names in batches of 10
	for i := 0; i < len(playerNames); i += 10 {
		// Calculate the end index for current batch
		end := i + 10
		if end > len(playerNames) {
			end = len(playerNames)
		}

		// Get the current batch of player names
		batchNames := playerNames[i:end]

		// Use the existing GetPlayersByNames function to fetch player data for this batch
		playerResp, err := c.GetPlayersByNames(shard, batchNames)
		if err != nil {
			log.Error().
				Str("shard", shard).
				Err(err).
				Msg("Error getting player data for batch")
			return nil, fmt.Errorf("error getting player data for batch %d-%d: %w", i, end-1, err)
		}

		// Populate the map with player names and their corresponding IDs from this batch
		for _, player := range playerResp.Data {
			playerIDMap[player.Attributes.Name] = player.ID
		}
	}

	log.Info().
		Str("shard", shard).
		Int("players_found", len(playerIDMap)).
		Msg("Completed GetPlayerIDs operation")

	return playerIDMap, nil
}

// GetMatchIDsForPlayers retrieves all unique match IDs for a list of players
// by batching requests in groups of 10 players (PUBG API limit)
func (c *Client) GetMatchIDsForPlayers(shard string, playerIDs []string) ([]string, error) {
	// Validate inputs
	if shard == "" {
		return nil, fmt.Errorf("shard cannot be empty")
	}

	if len(playerIDs) == 0 {
		return nil, fmt.Errorf("no player IDs provided")
	}

	log.Info().
		Str("shard", shard).
		Int("total_players", len(playerIDs)).
		Msg("Getting match IDs for players in batches")

	// Create a map to track unique match IDs
	uniqueMatchIDs := make(map[string]struct{})
	skippedPlayers := 0

	// Process player IDs in batches of 10
	for i := 0; i < len(playerIDs); i += 10 {
		// Calculate end index for current batch
		end := i + 10
		if end > len(playerIDs) {
			end = len(playerIDs)
		}

		// Create batch of up to 10 player IDs
		batch := playerIDs[i:end]

		// Get player data for the current batch using the rate-limited client
		response, err := c.GetPlayersByIDs(shard, batch)
		if err != nil {
			log.Error().
				Str("shard", shard).
				Err(err).
				Msg("Error getting data for player ID batch")
			return nil, err
		}

		// Extract match IDs from each player in the response
		for _, player := range response.Data {
			// Skip if the player has no relationships or matches
			if len(player.Relationships.Matches.Data) < 1 {
				skippedPlayers++
				continue
			}

			// Add each match ID to our unique set
			for _, match := range player.Relationships.Matches.Data {
				uniqueMatchIDs[match.ID] = struct{}{}
			}
		}
	}

	// Convert map of unique match IDs to slice
	matchIDs := make([]string, 0, len(uniqueMatchIDs))
	for id := range uniqueMatchIDs {
		matchIDs = append(matchIDs, id)
	}

	log.Info().
		Str("shard", shard).
		Int("total_players", len(playerIDs)).
		Int("players_with_no_matches", skippedPlayers).
		Int("unique_match_ids", len(matchIDs)).
		Msg("Completed GetMatchIDsForPlayers operation")

	return matchIDs, nil
}
