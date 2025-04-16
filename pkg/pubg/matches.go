package pubg

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/rs/zerolog/log"
)

// PUBGMatchResponse is the root structure for match data from PUBG API
type PUBGMatchResponse struct {
	Data     MatchData              `json:"data"`
	Included []IncludedObject       `json:"included"`
	Links    Links                  `json:"links"`
	Meta     map[string]interface{} `json:"meta"`
}

// MatchData represents information about a match
type MatchData struct {
	Type          string             `json:"type"`
	ID            string             `json:"id"`
	Attributes    MatchAttributes    `json:"attributes"`
	Relationships MatchRelationships `json:"relationships"`
	Links         SelfLinks          `json:"links"`
}

// MatchAttributes contains match details
type MatchAttributes struct {
	CreatedAt     string      `json:"createdAt"`
	Duration      int         `json:"duration"`
	TitleID       string      `json:"titleId"`
	IsCustomMatch bool        `json:"isCustomMatch"`
	MatchType     string      `json:"matchType"`
	SeasonState   string      `json:"seasonState"`
	Stats         interface{} `json:"stats"`
	GameMode      string      `json:"gameMode"`
	ShardID       string      `json:"shardId"`
	Tags          interface{} `json:"tags"`
	MapName       string      `json:"mapName"`
}

// MatchRelationships represents related data for a match
type MatchRelationships struct {
	Rosters RelationshipData `json:"rosters"`
	Assets  RelationshipData `json:"assets"`
}

// IncludedObject represents different types of objects included in the match data
type IncludedObject struct {
	Type          string                 `json:"type"`
	ID            string                 `json:"id"`
	Attributes    map[string]interface{} `json:"attributes"`
	Relationships map[string]interface{} `json:"relationships,omitempty"`
}

func (s IncludedObject) getStats() map[string]interface{} {
	stats, ok := s.Attributes["stats"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return stats
}

func (s IncludedObject) GetName() (string, bool) {
	stats := s.getStats()
	name, ok := stats["name"]
	if !ok {
		return "", false
	}

	// Convert to string, handling potential type conversions
	nameStr, ok := name.(string)
	if !ok {
		return "", false
	}

	if strings.TrimSpace(nameStr) == "" {
		return "", false // Name is empty
	}

	return nameStr, true
}

func (s IncludedObject) GetAccountID() (string, bool) {
	stats := s.getStats()

	accountID, ok := stats["playerId"]
	if !ok {
		return "", false
	}

	accountIDStr, ok := accountID.(string)
	if !ok {
		return "", false
	}

	return accountIDStr, true
}

// Checks to see if the incuded object has a valid player name (not starting with ai, not empty, exists)
func (s IncludedObject) IsValidPlayer() bool {
	if s.Type != "participant" {
		return false // Not a participant object so it cannot be a valid player
	}
	_, ok := s.GetName()
	if !ok {
		return false // Name not found in object return false to skip
	}

	accountID, ok := s.GetAccountID()
	if !ok {
		return false // Check if we can get player ID
	}

	if StartsWithAI(accountID) {
		return false // Bot account not valid for our purposes
	}

	return true
}

func StartsWithAI(s string) bool {
	// Trim leading whitespace
	trimmed := strings.TrimLeftFunc(s, unicode.IsSpace)

	// Check if the string starts with "ai." (at least 3 characters long)
	if len(trimmed) >= 3 && strings.ToLower(trimmed[:3]) == "ai." {
		return true
	}

	return false
}

// GetMatch retrieves data for a specific match by ID
func (c *Client) GetMatch(shard string, matchID string) (*PUBGMatchResponse, error) {
	operationID := fmt.Sprintf("get_match_%s_%d", matchID, time.Now().UnixNano())

	// Input validation
	if shard == "" {
		log.Error().
			Str("operation_id", operationID).
			Msg("GetMatch failed: shard cannot be empty")
		return nil, fmt.Errorf("shard cannot be empty")
	}

	if matchID == "" {
		log.Error().
			Str("operation_id", operationID).
			Str("shard", shard).
			Msg("GetMatch failed: matchID cannot be empty")
		return nil, fmt.Errorf("matchID cannot be empty")
	}

	// Request preparation and execution
	endpoint := fmt.Sprintf("/shards/%s/matches/%s", shard, matchID)
	respBody, err := c.RequestNonRateLimited(endpoint)

	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("endpoint", endpoint).
			Err(err).
			Msg("GetMatch API request failed")
		return nil, fmt.Errorf("error getting match data: %w", err)
	}

	// Response processing
	var matchResponse PUBGMatchResponse
	if err := json.Unmarshal(respBody, &matchResponse); err != nil {
		log.Error().
			Str("operation_id", operationID).
			Err(err).
			Msg("GetMatch response unmarshaling failed")
		return nil, fmt.Errorf("error unmarshaling match data: %w", err)
	}

	// Log success with important match details
	if matchResponse.Data.ID != "" {
		// Track included object types for metrics
		includeTypeCount := make(map[string]int)
		for _, obj := range matchResponse.Included {
			includeTypeCount[obj.Type]++
		}

		log.Info().
			Str("operation_id", operationID).
			Str("match_id", matchResponse.Data.ID).
			Str("game_mode", matchResponse.Data.Attributes.GameMode).
			Str("match_type", matchResponse.Data.Attributes.MatchType).
			Str("map_name", matchResponse.Data.Attributes.MapName).
			Int("included_objects_count", len(matchResponse.Included)).
			Msg("GetMatch retrieved match data successfully")
	}

	return &matchResponse, nil
}

// GetTelemetryURL extracts the telemetry URL from a match response
func (m *PUBGMatchResponse) GetTelemetryURL() (string, error) {
	operationID := fmt.Sprintf("get_telemetry_url_%s_%d", m.Data.ID, time.Now().UnixNano())

	// First, find the asset IDs in the match data
	var assetIDs []string
	for _, asset := range m.Data.Relationships.Assets.Data {
		if asset.Type == "asset" {
			assetIDs = append(assetIDs, asset.ID)
		}
	}

	if len(assetIDs) == 0 {
		log.Error().
			Str("operation_id", operationID).
			Str("match_id", m.Data.ID).
			Msg("GetTelemetryURL failed: no assets found in match data")
		return "", fmt.Errorf("no assets found in match data")
	}

	// Then look for the telemetry asset in the included objects
	for _, obj := range m.Included {
		if obj.Type == "asset" {
			// Check if this asset ID is one we're looking for
			for _, assetID := range assetIDs {
				if obj.ID == assetID {
					// Check if this is a telemetry asset
					attributes, ok := obj.Attributes["name"]
					if ok && attributes == "telemetry" {
						// Found the telemetry asset, extract the URL
						if url, ok := obj.Attributes["URL"].(string); ok {
							return url, nil
						}
					}
					break
				}
			}
		}
	}

	log.Error().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Msg("GetTelemetryURL failed: telemetry URL not found in match data")

	return "", fmt.Errorf("telemetry URL not found in match data")
}

func (m *PUBGMatchResponse) IsValidMatch() bool {
	matchType := m.GetMatchType()
	return matchType != "invalid"
}

func (m *PUBGMatchResponse) GetMatchType() string {
	var matchType string

	if m.Data.Attributes.GameMode == "squad-fpp" && m.Data.Attributes.MatchType == "competitive" && !m.Data.Attributes.IsCustomMatch {
		matchType = "ranked"
	} else if m.Data.Attributes.GameMode == "esports-squad-fpp" && m.Data.Attributes.IsCustomMatch {
		matchType = "scrim"
	} else {
		matchType = "invalid"
	}

	return matchType
}

func (m *PUBGMatchResponse) IsMatchOldEnough(timeElapsed int) bool {
	return m.Data.Attributes.Duration >= timeElapsed
}
