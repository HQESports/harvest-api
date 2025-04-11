package pubg

import (
	"encoding/json"
	"fmt"
	"time"

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

// GetMatch retrieves data for a specific match by ID
func (c *Client) GetMatch(shard string, matchID string) (*PUBGMatchResponse, error) {
	operationID := fmt.Sprintf("get_match_%s_%d", matchID, time.Now().UnixNano())
	startTime := time.Now()

	log.Info().
		Str("operation_id", operationID).
		Str("shard", shard).
		Str("match_id", matchID).
		Msg("Starting GetMatch operation")

	// Input validation phase
	validationStart := time.Now()
	if shard == "" {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "validation").
			Dur("duration", time.Since(validationStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetMatch validation failed: shard cannot be empty")
		return nil, fmt.Errorf("shard cannot be empty")
	}

	if matchID == "" {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "validation").
			Str("shard", shard).
			Dur("duration", time.Since(validationStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetMatch validation failed: matchID cannot be empty")
		return nil, fmt.Errorf("matchID cannot be empty")
	}
	validationDuration := time.Since(validationStart)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "validation").
		Str("shard", shard).
		Str("match_id", matchID).
		Dur("duration", validationDuration).
		Msg("GetMatch validation completed")

	// Request preparation phase
	endpoint := fmt.Sprintf("/shards/%s/matches/%s", shard, matchID)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "preparation").
		Str("endpoint", endpoint).
		Dur("prep_duration", time.Since(startTime)).
		Msg("GetMatch prepared API request")

	// API request phase
	apiStartTime := time.Now()
	respBody, err := c.RequestNonRateLimited(endpoint)
	apiDuration := time.Since(apiStartTime)

	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "api_request").
			Str("endpoint", endpoint).
			Err(err).
			Dur("validation_duration", validationDuration).
			Dur("api_duration", apiDuration).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetMatch API request failed")
		return nil, fmt.Errorf("error getting match data: %w", err)
	}

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "api_request").
		Str("endpoint", endpoint).
		Int("response_size", len(respBody)).
		Dur("validation_duration", validationDuration).
		Dur("api_duration", apiDuration).
		Msg("GetMatch API request completed successfully")

	// Response processing phase
	unmarshalStartTime := time.Now()

	var matchResponse PUBGMatchResponse
	if err := json.Unmarshal(respBody, &matchResponse); err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "unmarshaling").
			Str("endpoint", endpoint).
			Err(err).
			Int("response_size", len(respBody)).
			Dur("validation_duration", validationDuration).
			Dur("api_duration", apiDuration).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetMatch response unmarshaling failed")
		return nil, fmt.Errorf("error unmarshaling match data: %w", err)
	}

	unmarshalDuration := time.Since(unmarshalStartTime)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "unmarshaling").
		Dur("validation_duration", validationDuration).
		Dur("api_duration", apiDuration).
		Dur("unmarshal_duration", unmarshalDuration).
		Msg("GetMatch response unmarshaling completed")

	// Content analysis metrics
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
			Int("duration", matchResponse.Data.Attributes.Duration).
			Bool("is_custom_match", matchResponse.Data.Attributes.IsCustomMatch).
			Int("included_objects_count", len(matchResponse.Included)).
			Interface("included_object_types", includeTypeCount).
			Dur("validation_duration", validationDuration).
			Dur("api_duration", apiDuration).
			Dur("unmarshal_duration", unmarshalDuration).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetMatch retrieved match data successfully")
	} else {
		log.Warn().
			Str("operation_id", operationID).
			Str("phase", "analysis").
			Dur("validation_duration", validationDuration).
			Dur("api_duration", apiDuration).
			Dur("unmarshal_duration", unmarshalDuration).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetMatch retrieved match data with empty ID")
	}

	return &matchResponse, nil
}

// GetTelemetryURL extracts the telemetry URL from a match response
func (m *PUBGMatchResponse) GetTelemetryURL() (string, error) {
	operationID := fmt.Sprintf("get_telemetry_url_%s_%d", m.Data.ID, time.Now().UnixNano())
	startTime := time.Now()

	log.Info().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Msg("Starting GetTelemetryURL operation")

	// First, find the asset IDs in the match data
	assetSearchStart := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "asset_search").
		Str("match_id", m.Data.ID).
		Msg("GetTelemetryURL searching for asset IDs in match relationships")

	var assetIDs []string
	for _, asset := range m.Data.Relationships.Assets.Data {
		if asset.Type == "asset" {
			assetIDs = append(assetIDs, asset.ID)

			log.Trace().
				Str("operation_id", operationID).
				Str("phase", "asset_search").
				Str("asset_id", asset.ID).
				Str("asset_type", asset.Type).
				Msg("GetTelemetryURL found potential asset ID")
		}
	}

	assetSearchDuration := time.Since(assetSearchStart)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "asset_search").
		Int("asset_count", len(assetIDs)).
		Dur("duration", assetSearchDuration).
		Msg("GetTelemetryURL completed asset ID search")

	if len(assetIDs) == 0 {
		log.Error().
			Str("operation_id", operationID).
			Str("match_id", m.Data.ID).
			Dur("asset_search_duration", assetSearchDuration).
			Dur("total_duration", time.Since(startTime)).
			Msg("GetTelemetryURL failed: no assets found in match data")
		return "", fmt.Errorf("no assets found in match data")
	}

	// Then look for the telemetry asset in the included objects
	telemetrySearchStart := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "telemetry_search").
		Int("included_objects_count", len(m.Included)).
		Int("asset_ids_count", len(assetIDs)).
		Msg("GetTelemetryURL searching for telemetry asset in included objects")

	var candidateAssets int
	for _, obj := range m.Included {
		if obj.Type == "asset" {
			// Check if this asset ID is one we're looking for
			found := false
			for _, assetID := range assetIDs {
				if obj.ID == assetID {
					found = true
					candidateAssets++

					log.Trace().
						Str("operation_id", operationID).
						Str("phase", "telemetry_search").
						Str("asset_id", obj.ID).
						Msg("GetTelemetryURL examining candidate asset")
					break
				}
			}

			if found {
				// Check if this is a telemetry asset
				attributes, ok := obj.Attributes["name"]
				if ok && attributes == "telemetry" {
					// Found the telemetry asset, extract the URL
					if url, ok := obj.Attributes["URL"].(string); ok {
						telemetrySearchDuration := time.Since(telemetrySearchStart)

						log.Info().
							Str("operation_id", operationID).
							Str("match_id", m.Data.ID).
							Str("telemetry_asset_id", obj.ID).
							Int("candidates_examined", candidateAssets).
							Int("total_included_objects", len(m.Included)).
							Dur("asset_search_duration", assetSearchDuration).
							Dur("telemetry_search_duration", telemetrySearchDuration).
							Dur("total_duration", time.Since(startTime)).
							Msg("GetTelemetryURL operation completed successfully")

						return url, nil
					} else {
						log.Warn().
							Str("operation_id", operationID).
							Str("match_id", m.Data.ID).
							Str("asset_id", obj.ID).
							Msg("GetTelemetryURL found telemetry asset but URL attribute is invalid or missing")
					}
				}
			}
		}
	}

	telemetrySearchDuration := time.Since(telemetrySearchStart)

	log.Error().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Int("candidates_examined", candidateAssets).
		Int("total_included_objects", len(m.Included)).
		Dur("asset_search_duration", assetSearchDuration).
		Dur("telemetry_search_duration", telemetrySearchDuration).
		Dur("total_duration", time.Since(startTime)).
		Msg("GetTelemetryURL failed: telemetry URL not found in match data")

	return "", fmt.Errorf("telemetry URL not found in match data")
}

func (m *PUBGMatchResponse) IsValidMatch() bool {
	operationID := fmt.Sprintf("is_valid_match_%s_%d", m.Data.ID, time.Now().UnixNano())
	startTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Msg("Starting IsValidMatch classification")

	matchType := m.GetMatchType()
	isValid := matchType != "invalid"

	log.Info().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Str("match_type", matchType).
		Bool("is_valid", isValid).
		Dur("duration", time.Since(startTime)).
		Msg("IsValidMatch classification completed")

	return isValid
}

func (m *PUBGMatchResponse) GetMatchType() string {
	operationID := fmt.Sprintf("get_match_type_%s_%d", m.Data.ID, time.Now().UnixNano())
	startTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Str("game_mode", m.Data.Attributes.GameMode).
		Str("match_type", m.Data.Attributes.MatchType).
		Bool("is_custom_match", m.Data.Attributes.IsCustomMatch).
		Msg("Starting GetMatchType classification")

	var matchType string

	if m.Data.Attributes.GameMode == "squad-fpp" && m.Data.Attributes.MatchType == "competitive" && !m.Data.Attributes.IsCustomMatch {
		matchType = "ranked"
	} else if m.Data.Attributes.GameMode == "esports-squad-fpp" && m.Data.Attributes.IsCustomMatch {
		matchType = "scrim"
	} else {
		matchType = "invalid"

		log.Debug().
			Str("operation_id", operationID).
			Str("match_id", m.Data.ID).
			Str("game_mode", m.Data.Attributes.GameMode).
			Str("match_type", m.Data.Attributes.MatchType).
			Bool("is_custom_match", m.Data.Attributes.IsCustomMatch).
			Msg("Match classified as invalid due to configuration mismatch")
	}

	log.Info().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Str("classification", matchType).
		Dur("duration", time.Since(startTime)).
		Msg("GetMatchType classification completed")

	return matchType
}

func (m *PUBGMatchResponse) IsMatchOldEnough(timeElapsed int) bool {
	operationID := fmt.Sprintf("is_match_old_enough_%s_%d", m.Data.ID, time.Now().UnixNano())
	startTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Int("current_duration", m.Data.Attributes.Duration).
		Int("required_duration", timeElapsed).
		Msg("Checking if match is old enough")

	isOldEnough := m.Data.Attributes.Duration >= timeElapsed

	log.Info().
		Str("operation_id", operationID).
		Str("match_id", m.Data.ID).
		Int("current_duration", m.Data.Attributes.Duration).
		Int("required_duration", timeElapsed).
		Bool("is_old_enough", isOldEnough).
		Dur("duration", time.Since(startTime)).
		Msg("IsMatchOldEnough check completed")

	return isOldEnough
}
