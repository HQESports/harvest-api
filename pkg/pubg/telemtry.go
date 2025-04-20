package pubg

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Position represents a 2D coordinate in the game world
type Position struct {
	X float64 `json:"x" bson:"x"`
	Y float64 `json:"y" bson:"y"`
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
	SafetyZonePosition Position `json:"safetyZonePosition" bson:"safetyZonePosition"`
	SafetyZoneRadius   float64  `json:"safetyZoneRadius" bson:"safetyZoneRadius"`
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
}

// SafeZone represents data for a single circle phase
type SafeZone struct {
	Phase  int     `json:"phase" bson:"phase"`
	X      float64 `json:"x" bson:"x"`
	Y      float64 `json:"y" bson:"y"`
	Radius float64 `json:"radius" bson:"radius"`
}

// TelemetryData represents the extracted and processed telemetry data
type TelemetryData struct {
	SafeZones []SafeZone `json:"safeZones" bson:"safeZones"`
	PlanePath PlanePath  `json:"planePath" bson:"planePath"`
}

// ProcessTelemetry parses telemetry data from a JSON byte array and extracts relevant information
func ProcessTelemetry(data []byte) (*TelemetryData, error) {
	// Initialize result structure
	result := &TelemetryData{
		SafeZones: []SafeZone{},
	}

	// Use a map to track the index of each phase in our results array
	phaseIndexMap := make(map[int]int)

	// Unmarshal the entire array of events into a slice of maps
	var events []map[string]json.RawMessage
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal telemetry data: %w", err)
	}

	log.Info().Int("total_events", len(events)).Msg("Processing telemetry events")

	// Process each event based on its type
	for _, eventData := range events {
		// Extract event type
		var eventType string
		if typeData, ok := eventData["_T"]; ok {
			if err := json.Unmarshal(typeData, &eventType); err != nil {
				continue
			}
		} else {
			continue // Skip events without a type
		}

		// Process based on event type
		switch eventType {
		case "LogVehicleLeave": // Changed from LogPlayerPosition to LogVehicleRide to match the event type in your JSON
			// Check if this event has vehicle data
			vehicleData, hasVehicle := eventData["vehicle"]
			if !hasVehicle {
				continue
			}

			// Parse the vehicle data to check if it's the transport aircraft
			var vehicle struct {
				VehicleType string `json:"vehicleType"`
				VehicleId   string `json:"vehicleId"`
				Location    struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"location"`
			}

			if err := json.Unmarshal(vehicleData, &vehicle); err != nil {
				continue
			}

			// Check if this is the transport aircraft - only check vehicleType
			if vehicle.VehicleType == "TransportAircraft" {
				// Extract character location as the plane position
				var character struct {
					Location struct {
						X float64 `json:"x"`
						Y float64 `json:"y"`
						Z float64 `json:"z"`
					} `json:"location"`
				}

				characterData, hasCharacter := eventData["character"]
				if !hasCharacter {
					continue
				}

				if err := json.Unmarshal(characterData, &character); err != nil {
					continue
				}

				// If we don't have a start point yet, set it
				if result.PlanePath.StartPoint == (Position{}) {
					result.PlanePath.StartPoint = Position{
						X: character.Location.X,
						Y: character.Location.Y,
					}
					log.Info().
						Float64("x", character.Location.X).
						Float64("y", character.Location.Y).
						Msg("Set plane path start point")
				}

				// Always update the end point with each new position
				result.PlanePath.EndPoint = Position{
					X: character.Location.X,
					Y: character.Location.Y,
				}
				log.Debug().
					Float64("x", character.Location.X).
					Float64("y", character.Location.Y).
					Msg("Updated plane path end point")
			}

		case "LogGameStatePeriodic":
			var event LogGameStatePeriodic
			eventBytes, err := json.Marshal(eventData)
			if err != nil {
				continue
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				continue
			}

			// Get the isGame value
			isGame := event.Common.IsGame

			// Adjust phase number: isGame 2.0 is actually the first real zone
			var phase int
			if float64(int(isGame)) == isGame && isGame >= 2.0 {
				// Convert isGame to actual phase (isGame 2.0 -> Phase 1)
				phase = int(isGame) - 1

				// Skip processing if the coordinates are exactly center of map (408000, 408000)
				// which might indicate default values rather than actual gameplay position
				if phase == 1 && event.GameState.SafetyZonePosition.X == 408000.0 && event.GameState.SafetyZonePosition.Y == 408000.0 {
					log.Warn().Msg("Skipping Phase 1 with default center coordinates")
					continue
				}

				// Extract safe zone data for this phase
				safeZone := SafeZone{
					Phase:  phase,
					X:      event.GameState.SafetyZonePosition.X,
					Y:      event.GameState.SafetyZonePosition.Y,
					Radius: event.GameState.SafetyZoneRadius,
				}

				// Check if we've seen this phase before
				if index, found := phaseIndexMap[phase]; found {
					// Update the existing entry with the latest values
					result.SafeZones[index] = safeZone
				} else {
					// First time seeing this phase, add it to our results
					result.SafeZones = append(result.SafeZones, safeZone)
					// Store the index of this phase for future updates
					phaseIndexMap[phase] = len(result.SafeZones) - 1
				}
			}
		}
	}

	log.Info().
		Int("safe_zones_found", len(result.SafeZones)).
		Bool("plane_path_found", result.PlanePath != PlanePath{}).
		Msg("Telemetry processing completed")

	return result, nil
}

// ProcessTelemetryFromURL fetches and processes telemetry data from a URL
func (c *Client) ProcessTelemetryFromURL(ctx context.Context, telemetryURL string) (*TelemetryData, error) {
	log.Info().
		Str("url", telemetryURL).
		Msg("Processing telemetry data from URL")

	// Get the telemetry data using the existing client method
	telemetryData, err := c.GetTelemetry(ctx, telemetryURL, false)
	if err != nil {
		log.Error().
			Str("url", telemetryURL).
			Err(err).
			Msg("Failed to fetch telemetry data")
		return nil, fmt.Errorf("failed to fetch telemetry data: %w", err)
	}

	log.Info().
		Str("url", telemetryURL).
		Int("data_size_kb", len(telemetryData)/1024).
		Msg("Successfully downloaded telemetry data")

	// Process the telemetry data
	return ProcessTelemetry(telemetryData)
}

// GetMatchTelemetry retrieves and processes telemetry data for a specific match
func (c *Client) GetMatchTelemetry(shard string, matchID string) (*TelemetryData, error) {
	operationID := fmt.Sprintf("get_match_telemetry_%s_%d", matchID, time.Now().UnixNano())
	startTime := time.Now()

	log.Info().
		Str("operation", "GetMatchTelemetry").
		Str("operation_id", operationID).
		Str("shard", shard).
		Str("match_id", matchID).
		Msg("Starting telemetry retrieval")

	// Get match data
	matchResponse, err := c.GetMatch(shard, matchID)
	if err != nil {
		log.Error().
			Str("operation", "GetMatchTelemetry").
			Str("operation_id", operationID).
			Str("shard", shard).
			Str("match_id", matchID).
			Err(err).
			Msg("Error getting match data")
		return nil, fmt.Errorf("error getting match data: %w", err)
	}

	// Extract telemetry URL from match data
	telemetryURL, err := matchResponse.GetTelemetryURL()
	if err != nil {
		log.Error().
			Str("operation", "GetMatchTelemetry").
			Str("operation_id", operationID).
			Str("match_id", matchID).
			Err(err).
			Msg("Error extracting telemetry URL")
		return nil, fmt.Errorf("error extracting telemetry URL: %w", err)
	}

	// Process telemetry from the URL
	ctx := context.Background()
	processedData, err := c.ProcessTelemetryFromURL(ctx, telemetryURL)
	if err != nil {
		log.Error().
			Str("operation", "GetMatchTelemetry").
			Str("operation_id", operationID).
			Str("match_id", matchID).
			Err(err).
			Msg("Error processing telemetry data")
		return nil, fmt.Errorf("error processing telemetry data: %w", err)
	}

	// Log summary of results
	log.Info().
		Str("operation", "GetMatchTelemetry").
		Str("operation_id", operationID).
		Str("match_id", matchID).
		Str("map_name", matchResponse.Data.Attributes.MapName).
		Int("safe_zones_count", len(processedData.SafeZones)).
		Bool("has_plane_path", processedData.PlanePath != PlanePath{}).
		Dur("duration", time.Since(startTime)).
		Msg("Telemetry retrieval completed")

	return processedData, nil
}

// String returns a human-readable representation of TelemetryData
func (t *TelemetryData) String() string {
	var result strings.Builder

	// Add plane path information
	result.WriteString("Plane Path:\n")
	result.WriteString(fmt.Sprintf("  Start: X=%.2f, Y=%.2f\n", t.PlanePath.StartPoint.X, t.PlanePath.StartPoint.Y))
	result.WriteString(fmt.Sprintf("  End:   X=%.2f, Y=%.2f\n", t.PlanePath.EndPoint.X, t.PlanePath.EndPoint.Y))
	result.WriteString("\n")

	// Add safe zones information
	result.WriteString(fmt.Sprintf("Safe Zones (%d):\n", len(t.SafeZones)))
	for i, zone := range t.SafeZones {
		result.WriteString(fmt.Sprintf("  Zone %d (Phase %d):\n", i+1, zone.Phase))
		result.WriteString(fmt.Sprintf("    Center: X=%.2f, Y=%.2f\n", zone.X, zone.Y))
		result.WriteString(fmt.Sprintf("    Radius: %.2f\n", zone.Radius))
	}

	return result.String()
}

// GenerateCirclesImage creates a PNG image showing the safe zones and plane path
// from telemetry data and saves it to the specified output file
func (t *TelemetryData) GenerateCirclesImage(outputFile string, backgroundImagePath string) error {
	var img *image.RGBA

	// Check if background image is provided
	if backgroundImagePath != "" {
		// Load the background image
		bgFile, err := os.Open(backgroundImagePath)
		if err != nil {
			return fmt.Errorf("failed to open background image: %w", err)
		}
		defer bgFile.Close()

		bgImg, err := png.Decode(bgFile)
		if err != nil {
			return fmt.Errorf("failed to decode background image: %w", err)
		}

		// Create a new RGBA image with the same size as the background
		bounds := bgImg.Bounds()
		img = image.NewRGBA(bounds)

		// Copy the background image to our RGBA image
		for x := 0; x < bounds.Dx(); x++ {
			for y := 0; y < bounds.Dy(); y++ {
				img.Set(x, y, bgImg.At(x, y))
			}
		}
	} else {
		// No background provided, create a blank image with dark background
		img = image.NewRGBA(image.Rect(0, 0, 800, 800))

		// Fill with a dark background
		for x := 0; x < 800; x++ {
			for y := 0; y < 800; y++ {
				img.Set(x, y, color.RGBA{20, 20, 20, 255}) // Dark gray background
			}
		}
	}

	// Get image dimensions
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Define scaling factor - from 816000 units to image dimensions
	scaleX := float64(imgWidth) / 816000.0
	scaleY := float64(imgHeight) / 816000.0

	// Define colors for different elements
	planePathColor := color.RGBA{255, 255, 0, 255} // Yellow
	zoneColors := []color.RGBA{
		{255, 255, 255, 255}, // White - Phase 1
		{0, 0, 255, 255},     // Blue - Phase 2
		{0, 255, 0, 255},     // Green - Phase 3
		{255, 0, 0, 255},     // Red - Phase 4
		{255, 0, 255, 255},   // Magenta - Phase 5
		{0, 255, 255, 255},   // Cyan - Phase 6
		{255, 165, 0, 255},   // Orange - Phase 7
		{128, 0, 128, 255},   // Purple - Phase 8
	}

	// Convert game coordinates to image coordinates
	// With (0,0) at top-left of the image
	toImageX := func(gameX float64) int {
		return int(gameX * scaleX)
	}
	toImageY := func(gameY float64) int {
		return int(gameY * scaleY)
	}

	// Draw the safe zones
	for i, zone := range t.SafeZones {
		colorIndex := i % len(zoneColors)
		drawCircle(img, toImageX(zone.X), toImageY(zone.Y), int(zone.Radius*scaleX), zoneColors[colorIndex])
	}

	// Draw the plane path
	startX := toImageX(t.PlanePath.StartPoint.X)
	startY := toImageY(t.PlanePath.StartPoint.Y)
	endX := toImageX(t.PlanePath.EndPoint.X)
	endY := toImageY(t.PlanePath.EndPoint.Y)
	drawLine(img, startX, startY, endX, endY, planePathColor)

	// Save the image
	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	return nil
}

// drawCircle draws a circle on the image
func drawCircle(img *image.RGBA, x, y, radius int, c color.RGBA) {
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	// Draw only the outline of the circle
	for angle := 0.0; angle < 2*math.Pi; angle += 0.001 {
		px := x + int(float64(radius)*math.Cos(angle))
		py := y + int(float64(radius)*math.Sin(angle))

		// Check if point is within image boundaries
		if px >= 0 && px < imgWidth && py >= 0 && py < imgHeight {
			img.Set(px, py, c)
		}
	}
}

// drawLine draws a line on the image using Bresenham's algorithm
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	steep := math.Abs(float64(y1-y0)) > math.Abs(float64(x1-x0))

	if steep {
		x0, y0 = y0, x0
		x1, y1 = y1, x1
	}

	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}

	dx := x1 - x0
	dy := int(math.Abs(float64(y1 - y0)))
	err := dx / 2
	y := y0

	ystep := 1
	if y0 >= y1 {
		ystep = -1
	}

	for x := x0; x <= x1; x++ {
		if steep {
			if x >= 0 && x < imgHeight && y >= 0 && y < imgWidth {
				img.Set(y, x, c)
			}
		} else {
			if x >= 0 && x < imgWidth && y >= 0 && y < imgHeight {
				img.Set(x, y, c)
			}
		}

		err -= dy
		if err < 0 {
			y += ystep
			err += dx
		}
	}
}
