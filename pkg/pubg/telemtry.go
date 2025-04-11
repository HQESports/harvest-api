package pubg

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// Position represents a 3D coordinate in the game world
type Position struct {
	X float64 `json:"x" bson:"x"`
	Y float64 `json:"y" bson:"y"`
	Z float64 `json:"z" bson:"z"`
}

// Common represents shared fields across different event types
type Common struct {
	IsGame float64 `json:"isGame" bson:"isGame"`
}

// Character represents player information
type Character struct {
	Location Position `json:"location" bson:"location"`
}

// GameState represents the current state of game zones
type GameState struct {
	SafetyZonePosition       Position `json:"safetyZonePosition" bson:"safetyZonePosition"`
	SafetyZoneRadius         float64  `json:"safetyZoneRadius" bson:"safetyZoneRadius"`
	PoisonGasWarningPosition Position `json:"poisonGasWarningPosition" bson:"poisonGasWarningPosition"`
	PoisonGasWarningRadius   float64  `json:"poisonGasWarningRadius" bson:"poisonGasWarningRadius"`
}

// BaseEvent contains common fields for all event types
type BaseEvent struct {
	Type      string    `json:"_T" bson:"_T"`
	Timestamp time.Time `json:"_D" bson:"_D"`
}

// LogPlayerPosition represents a player position event
type LogPlayerPosition struct {
	BaseEvent
	Character Character `json:"character" bson:"character"`
	Common    Common    `json:"common" bson:"common"`
}

// LogGameStatePeriodic represents a game state update event
type LogGameStatePeriodic struct {
	BaseEvent
	Common    Common    `json:"common" bson:"common"`
	GameState GameState `json:"gameState" bson:"gameState"`
}

// PlanePath represents the airplane trajectory
type PlanePath struct {
	StartPoint Position `json:"startPoint" bson:"startPoint"`
	EndPoint   Position `json:"endPoint" bson:"endPoint"`
	Trajectory []Position
}

// CirclePhase represents data for a single circle phase
type CirclePhase struct {
	Phase          int       `json:"phase" bson:"phase"`
	Timestamp      time.Time `json:"timestamp" bson:"timestamp"`
	SafeZoneCenter Position  `json:"safeZoneCenter" bson:"safeZoneCenter"`
	SafeZoneRadius float64   `json:"safeZoneRadius" bson:"safeZoneRadius"`
	BlueZoneCenter Position  `json:"blueZoneCenter" bson:"blueZoneCenter"`
	BlueZoneRadius float64   `json:"blueZoneRadius" bson:"blueZoneRadius"`
}

// TelemetryData represents the extracted and processed telemetry data
type TelemetryData struct {
	Circles   []CirclePhase `json:"circles" bson:"circles"`
	PlanePath PlanePath     `json:"planePath" bson:"planePath"`
}

// ProcessTelemetry parses telemetry data from a JSON byte array and extracts relevant information
func ProcessTelemetry(data []byte) (*TelemetryData, error) {
	operationID := fmt.Sprintf("process_telemetry_%d", time.Now().UnixNano())
	startTime := time.Now()

	log.Info().
		Str("operation_id", operationID).
		Int("data_size_bytes", len(data)).
		Msg("Starting ProcessTelemetry operation")

	// Initialize result structure
	result := &TelemetryData{
		Circles:   []CirclePhase{},
		PlanePath: PlanePath{Trajectory: []Position{}},
	}

	// Use a map to track the first occurrence of each circle phase
	phaseMap := make(map[int]bool)

	// Unmarshal phase
	unmarshalStartTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "unmarshal").
		Msg("Unmarshaling telemetry data array")

	// Unmarshal the entire array of events into a slice of maps
	var events []map[string]json.RawMessage
	if err := json.Unmarshal(data, &events); err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "unmarshal").
			Err(err).
			Int("data_size_bytes", len(data)).
			Dur("operation_duration", time.Since(startTime)).
			Msg("Failed to unmarshal telemetry data")
		return nil, fmt.Errorf("failed to unmarshal telemetry data: %w", err)
	}

	unmarshalDuration := time.Since(unmarshalStartTime)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "unmarshal").
		Int("event_count", len(events)).
		Dur("duration", unmarshalDuration).
		Msg("Successfully unmarshaled telemetry events array")

	// Initialize processing metrics
	processingStartTime := time.Now()
	processedEvents := 0
	errorEvents := 0
	skippedEvents := 0
	playerPositionEvents := 0
	gameStateEvents := 0
	planePathPoints := 0
	circlePhasesFound := 0

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "processing").
		Int("total_events", len(events)).
		Msg("Starting event processing loop")

	// Process each event based on its type
	for i, eventData := range events {
		eventStartTime := time.Now()

		// Extract event type
		var eventType string
		if typeData, ok := eventData["_T"]; ok {
			if err := json.Unmarshal(typeData, &eventType); err != nil {
				log.Error().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("event_index", i).
					Err(err).
					Dur("event_duration", time.Since(eventStartTime)).
					Msg("Failed to unmarshal event type")
				errorEvents++
				continue
			}
		} else {
			log.Trace().
				Str("operation_id", operationID).
				Str("phase", "processing").
				Int("event_index", i).
				Msg("Skipping event without _T field")
			skippedEvents++
			continue // Skip events without a type
		}

		// Process based on event type
		switch eventType {
		case "LogPlayerPosition":
			playerPositionEvents++

			log.Trace().
				Str("operation_id", operationID).
				Str("phase", "processing").
				Int("event_index", i).
				Str("event_type", eventType).
				Msg("Processing player position event")

			var event LogPlayerPosition
			eventBytes, err := json.Marshal(eventData)
			if err != nil {
				log.Warn().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("event_index", i).
					Str("event_type", eventType).
					Err(err).
					Msg("Failed to marshal event data for unmarshaling")
				errorEvents++
				continue
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				log.Error().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("event_index", i).
					Str("event_type", eventType).
					Err(err).
					Msg("Error unmarshalling LogPlayerPosition event")
				errorEvents++
				continue
			}

			// Check if this is a plane path event
			if event.Common.IsGame <= 0.2 && event.Common.IsGame >= 0.1 {
				planePathPoints++

				log.Trace().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("event_index", i).
					Float64("is_game", event.Common.IsGame).
					Interface("position", event.Character.Location).
					Str("timestamp", event.Timestamp.Format(time.RFC3339)).
					Int("total_trajectory_points", len(result.PlanePath.Trajectory)).
					Msg("Identified plane path position")

				// Add this point to the trajectory
				result.PlanePath.Trajectory = append(result.PlanePath.Trajectory, event.Character.Location)

				// If this is the first point, set it as the start point
				if len(result.PlanePath.Trajectory) == 1 {
					result.PlanePath.StartPoint = event.Character.Location

					log.Debug().
						Str("operation_id", operationID).
						Str("phase", "processing").
						Interface("start_point", event.Character.Location).
						Str("timestamp", event.Timestamp.Format(time.RFC3339)).
						Msg("Set plane path start point")
				}

				// Update the end point with each new position (will eventually be the last position)
				result.PlanePath.EndPoint = event.Character.Location
			}

		case "LogGameStatePeriodic":
			gameStateEvents++

			log.Trace().
				Str("operation_id", operationID).
				Str("phase", "processing").
				Int("event_index", i).
				Str("event_type", eventType).
				Msg("Processing game state event")

			var event LogGameStatePeriodic
			eventBytes, err := json.Marshal(eventData)
			if err != nil {
				log.Warn().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("event_index", i).
					Str("event_type", eventType).
					Err(err).
					Msg("Failed to marshal event data for unmarshaling")
				errorEvents++
				continue
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				log.Warn().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("event_index", i).
					Str("event_type", eventType).
					Err(err).
					Msg("Error unmarshalling LogGameStatePeriodic event")
				errorEvents++
				continue
			}

			// Check if this represents a new circle phase (whole number)
			isGame := event.Common.IsGame
			phase := int(isGame)

			// Only process events that represent new circle formations (whole numbers like 1.0, 2.0, etc.)
			// Also check that we haven't already processed this phase
			if float64(phase) == isGame && phase > 0 && !phaseMap[phase] {
				// Mark this phase as processed
				phaseMap[phase] = true
				circlePhasesFound++

				// Extract circle data for this phase
				circlePhase := CirclePhase{
					Phase:          phase,
					Timestamp:      event.Timestamp,
					SafeZoneCenter: event.GameState.SafetyZonePosition,
					SafeZoneRadius: event.GameState.SafetyZoneRadius,
					BlueZoneCenter: event.GameState.PoisonGasWarningPosition,
					BlueZoneRadius: event.GameState.PoisonGasWarningRadius,
				}

				result.Circles = append(result.Circles, circlePhase)

				log.Debug().
					Str("operation_id", operationID).
					Str("phase", "processing").
					Int("circle_phase", phase).
					Float64("safe_zone_radius", event.GameState.SafetyZoneRadius).
					Float64("blue_zone_radius", event.GameState.PoisonGasWarningRadius).
					Interface("safe_zone_center", event.GameState.SafetyZonePosition).
					Interface("blue_zone_center", event.GameState.PoisonGasWarningPosition).
					Str("timestamp", event.Timestamp.Format(time.RFC3339)).
					Msg("Extracted new circle phase data")
			}
		}

		processedEvents++

		if processedEvents%10000 == 0 {
			log.Info().
				Str("operation_id", operationID).
				Str("phase", "processing").
				Int("processed_events", processedEvents).
				Int("total_events", len(events)).
				Float64("completion_percentage", float64(processedEvents)/float64(len(events))*100).
				Int("player_position_events", playerPositionEvents).
				Int("game_state_events", gameStateEvents).
				Int("error_events", errorEvents).
				Int("plane_path_points", planePathPoints).
				Int("circle_phases_found", circlePhasesFound).
				Dur("elapsed_duration", time.Since(startTime)).
				Msg("Telemetry processing progress update")
		}
	}

	processingDuration := time.Since(processingStartTime)

	// Only log the plane path if we have valid data
	if len(result.PlanePath.Trajectory) > 0 {
		log.Info().
			Str("operation_id", operationID).
			Str("phase", "results").
			Interface("plane_start", result.PlanePath.StartPoint).
			Interface("plane_end", result.PlanePath.EndPoint).
			Int("trajectory_points", len(result.PlanePath.Trajectory)).
			Msg("Extracted plane path data")
	} else {
		log.Warn().
			Str("operation_id", operationID).
			Str("phase", "results").
			Msg("No plane path data found in telemetry")
	}

	// Log circle phases summary
	circlePhaseNumbers := make([]int, 0, len(result.Circles))
	for _, circle := range result.Circles {
		circlePhaseNumbers = append(circlePhaseNumbers, circle.Phase)
	}

	log.Info().
		Str("operation_id", operationID).
		Str("phase", "results").
		Int("circle_count", len(result.Circles)).
		Ints("detected_phases", circlePhaseNumbers).
		Msg("Extracted circle phase data")

	// Final completion statistics
	log.Info().
		Str("operation_id", operationID).
		Int("total_events", len(events)).
		Int("processed_events", processedEvents).
		Int("skipped_events", skippedEvents).
		Int("error_events", errorEvents).
		Int("player_position_events", playerPositionEvents).
		Int("game_state_events", gameStateEvents).
		Int("plane_path_points", planePathPoints).
		Int("circle_phases", len(result.Circles)).
		Dur("unmarshal_duration", unmarshalDuration).
		Dur("processing_duration", processingDuration).
		Dur("total_duration", time.Since(startTime)).
		Float64("events_per_second", float64(processedEvents)/processingDuration.Seconds()).
		Msg("ProcessTelemetry operation completed")

	return result, nil
}

// DumpTelemetryData outputs extracted data for debugging
func DumpTelemetryData(data *TelemetryData) {
	operationID := fmt.Sprintf("dump_telemetry_%d", time.Now().UnixNano())

	log.Debug().
		Str("operation_id", operationID).
		Int("circle_phases", len(data.Circles)).
		Int("trajectory_points", len(data.PlanePath.Trajectory)).
		Msg("Starting DumpTelemetryData operation")

	fmt.Println("Extracted PUBG Telemetry Data:")

	// Dump circle phases
	fmt.Println("\nCircle Phases:")
	for i, circle := range data.Circles {
		fmt.Printf("Phase %d (Time: %s):\n", circle.Phase, circle.Timestamp.Format(time.RFC3339))
		fmt.Printf("  White Circle: Center(%.2f, %.2f, %.2f), Radius: %.2f\n",
			circle.SafeZoneCenter.X, circle.SafeZoneCenter.Y, circle.SafeZoneCenter.Z, circle.SafeZoneRadius)
		fmt.Printf("  Blue Circle: Center(%.2f, %.2f, %.2f), Radius: %.2f\n",
			circle.BlueZoneCenter.X, circle.BlueZoneCenter.Y, circle.BlueZoneCenter.Z, circle.BlueZoneRadius)

		if i < len(data.Circles)-1 {
			fmt.Println("")
		}
	}

	// Dump plane path information
	fmt.Println("\nPlane Path:")
	fmt.Printf("  Start: (%.2f, %.2f, %.2f)\n",
		data.PlanePath.StartPoint.X, data.PlanePath.StartPoint.Y, data.PlanePath.StartPoint.Z)
	fmt.Printf("  End: (%.2f, %.2f, %.2f)\n",
		data.PlanePath.EndPoint.X, data.PlanePath.EndPoint.Y, data.PlanePath.EndPoint.Z)
	fmt.Printf("  Points: %d\n", len(data.PlanePath.Trajectory))

	log.Debug().
		Str("operation_id", operationID).
		Msg("DumpTelemetryData operation completed")
}

// GetMatchTelemetry retrieves and processes telemetry data for a specific match
func (c *Client) GetMatchTelemetry(shard string, matchID string) (*TelemetryData, error) {
	operationID := fmt.Sprintf("get_match_telemetry_%s_%d", matchID, time.Now().UnixNano())
	startTime := time.Now()

	log.Info().
		Str("operation_id", operationID).
		Str("shard", shard).
		Str("match_id", matchID).
		Msg("Starting GetMatchTelemetry operation")

	// Get match data
	matchStartTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "match_retrieval").
		Str("shard", shard).
		Str("match_id", matchID).
		Msg("Retrieving match data")

	matchResponse, err := c.GetMatch(shard, matchID)
	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "match_retrieval").
			Str("shard", shard).
			Str("match_id", matchID).
			Err(err).
			Dur("match_duration", time.Since(matchStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error getting match data")
		return nil, fmt.Errorf("error getting match data: %w", err)
	}

	matchDuration := time.Since(matchStartTime)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "match_retrieval").
		Str("match_id", matchID).
		Str("game_mode", matchResponse.Data.Attributes.GameMode).
		Str("map_name", matchResponse.Data.Attributes.MapName).
		Dur("duration", matchDuration).
		Msg("Successfully retrieved match data")

	// Extract telemetry URL from match data
	urlStartTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "url_extraction").
		Str("match_id", matchID).
		Msg("Extracting telemetry URL from match data")

	telemetryURL, err := matchResponse.GetTelemetryURL()
	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "url_extraction").
			Str("match_id", matchID).
			Err(err).
			Dur("match_duration", matchDuration).
			Dur("url_duration", time.Since(urlStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error extracting telemetry URL")
		return nil, fmt.Errorf("error extracting telemetry URL: %w", err)
	}

	urlDuration := time.Since(urlStartTime)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "url_extraction").
		Str("match_id", matchID).
		Str("telemetry_url", telemetryURL).
		Dur("duration", urlDuration).
		Msg("Successfully extracted telemetry URL")

	// Get telemetry data
	downloadStartTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "download").
		Str("match_id", matchID).
		Str("telemetry_url", telemetryURL).
		Msg("Downloading telemetry data")

	telemetryData, err := c.GetTelemetryNonRateLimited(telemetryURL)
	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "download").
			Str("match_id", matchID).
			Str("telemetry_url", telemetryURL).
			Err(err).
			Dur("match_duration", matchDuration).
			Dur("url_duration", urlDuration).
			Dur("download_duration", time.Since(downloadStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error downloading telemetry data")
		return nil, fmt.Errorf("error getting telemetry data: %w", err)
	}

	downloadDuration := time.Since(downloadStartTime)

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "download").
		Str("match_id", matchID).
		Int("data_size_bytes", len(telemetryData)).
		Dur("duration", downloadDuration).
		Msg("Successfully downloaded telemetry data")

	// Process telemetry data
	processStartTime := time.Now()

	log.Debug().
		Str("operation_id", operationID).
		Str("phase", "processing").
		Str("match_id", matchID).
		Int("data_size_bytes", len(telemetryData)).
		Msg("Processing telemetry data")

	processedData, err := ProcessTelemetry(telemetryData)
	if err != nil {
		log.Error().
			Str("operation_id", operationID).
			Str("phase", "processing").
			Str("match_id", matchID).
			Err(err).
			Int("data_size_bytes", len(telemetryData)).
			Dur("match_duration", matchDuration).
			Dur("url_duration", urlDuration).
			Dur("download_duration", downloadDuration).
			Dur("processing_duration", time.Since(processStartTime)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error processing telemetry data")
		return nil, fmt.Errorf("error processing telemetry data: %w", err)
	}

	processingDuration := time.Since(processStartTime)

	// Log summary of results
	log.Info().
		Str("operation_id", operationID).
		Str("phase", "completion").
		Str("match_id", matchID).
		Str("map_name", matchResponse.Data.Attributes.MapName).
		Int("data_size_mb", len(telemetryData)/1024/1024).
		Int("circle_phases", len(processedData.Circles)).
		Int("plane_path_points", len(processedData.PlanePath.Trajectory)).
		Bool("has_plane_path", len(processedData.PlanePath.Trajectory) > 0).
		Dur("match_duration", matchDuration).
		Dur("url_duration", urlDuration).
		Dur("download_duration", downloadDuration).
		Dur("processing_duration", processingDuration).
		Dur("total_duration", time.Since(startTime)).
		Float64("mb_per_second", float64(len(telemetryData))/1024/1024/time.Since(startTime).Seconds()).
		Msg("GetMatchTelemetry operation completed successfully")

	return processedData, nil
}
