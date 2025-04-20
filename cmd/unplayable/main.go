package main

import (
	"context"
	"harvest/internal/config"
	"harvest/internal/database"
	"harvest/internal/model"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Error().Err(err).Msg("Failed to load configuration")
		return
	}

	// Configure logging
	setupLogger(cfg.Logging)
	log.Info().Msg("Starting test script")

	// Initialize MongoDB connection
	db, err := database.New(cfg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize database connection")
		return
	}
	log.Info().Msg("Database connection established")

	// Your test code here
	// Use db for database operations
	matches, err := db.GetMatchesByFilters(context.TODO(), "Baltic_Main", []string{"event"}, nil, nil, 1000)
	if err != nil {
		log.Error().Err(err)
		return
	}

	err = GenerateUnplayableMap(matches, "unplayable.png")
	if err != nil {
		log.Error().Err(err)
		return
	}

	log.Info().Msg("Test script completed")
}

func setupLogger(config config.LoggingConfig) {
	// Set global log level
	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure logger output
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	// Add timestamp
	log.Logger = log.With().Timestamp().Logger()
}

// GenerateUnplayableMap creates a visualization of unplayable areas based on the model.Match data
// matches: List of Match objects from your model
// outPath: Path where the PNG image should be saved
func GenerateUnplayableMap(matches []model.Match, outPath string) error {
	// Constants
	const (
		MapSize   = 816000.0 // PUBG map size (816000x816000)
		ImageSize = 800      // Output image size (800x800 pixels)
	)

	// Initialize the heatmap grid
	heatmap := make([][]float64, ImageSize)
	for i := range heatmap {
		heatmap[i] = make([]float64, ImageSize)
	}

	// Process each match
	validMatches := 0
	for _, match := range matches {
		// Skip matches without telemetry data
		if match.TelemetryData == nil || len(match.TelemetryData.SafeZones) < 4 {
			continue
		}

		// Find phase 3 and 4 zones
		var phase3, phase4 *model.SafeZone
		for i := range match.TelemetryData.SafeZones {
			zone := &match.TelemetryData.SafeZones[i]
			if zone.Phase == 3 {
				phase3 = zone
			} else if zone.Phase == 4 {
				phase4 = zone
			}
		}

		if phase3 == nil || phase4 == nil {
			continue
		}

		// Calculate the shift vector from phase 3 to phase 4
		shiftVector := struct {
			x, y, magnitude float64
		}{
			x:         phase4.X - phase3.X,
			y:         phase4.Y - phase3.Y,
			magnitude: math.Sqrt(math.Pow(phase4.X-phase3.X, 2) + math.Pow(phase4.Y-phase3.Y, 2)),
		}

		// Calculate the opposite direction vector (pointing toward likely unplayable areas)
		oppositeDir := struct {
			x, y float64
		}{
			x: phase3.X - phase4.X,
			y: phase3.Y - phase4.Y,
		}

		// Normalize the opposite direction vector
		length := math.Sqrt(oppositeDir.x*oppositeDir.x + oppositeDir.y*oppositeDir.y)
		if length > 0 {
			oppositeDir.x /= length
			oppositeDir.y /= length
		}

		// Calculate maximum possible shift
		maxShift := phase3.Radius - phase4.Radius

		// Calculate shift utilization (how much of the maximum possible shift was used)
		shiftUtilization := shiftVector.magnitude / maxShift

		// The higher the utilization, the more likely there's unplayable area in opposite direction
		unplayableWeight := shiftUtilization * 2

		// Calculate cell size in world coordinates
		cellSize := MapSize / float64(ImageSize)

		// Update the heatmap for cells that are likely in the unplayable area
		for i := 0; i < ImageSize; i++ {
			for j := 0; j < ImageSize; j++ {
				// Convert grid cell to world coordinates (center of the cell)
				worldX := (float64(i) + 0.5) * cellSize
				worldY := (float64(j) + 0.5) * cellSize

				// Check if this point is inside phase3 circle but outside phase4 circle
				distToPhase3Center := math.Sqrt(
					math.Pow(worldX-phase3.X, 2) +
						math.Pow(worldY-phase3.Y, 2),
				)

				distToPhase4Center := math.Sqrt(
					math.Pow(worldX-phase4.X, 2) +
						math.Pow(worldY-phase4.Y, 2),
				)

				if distToPhase3Center <= phase3.Radius && distToPhase4Center > phase4.Radius {
					// This point is in the area that's in phase3 but not in phase4

					// Calculate vector from phase3 center to this point
					vectorToPoint := struct {
						x, y float64
					}{
						x: worldX - phase3.X,
						y: worldY - phase3.Y,
					}

					// Normalize the vector
					vectorLength := math.Sqrt(vectorToPoint.x*vectorToPoint.x + vectorToPoint.y*vectorToPoint.y)
					if vectorLength > 0 {
						vectorToPoint.x /= vectorLength
						vectorToPoint.y /= vectorLength
					}

					// Dot product gives alignment (-1 to 1, where 1 is perfect alignment)
					alignment := (vectorToPoint.x * oppositeDir.x) +
						(vectorToPoint.y * oppositeDir.y)

					// The higher the alignment and the further from phase4, the more likely this area is unplayable
					if alignment > 0 { // Only consider the half of the circle in the opposite direction
						// Calculate a weight based on alignment and distance from the edge of phase4
						distFromPhase4Edge := distToPhase4Center - phase4.Radius
						maxDistFromPhase4Edge := phase3.Radius - phase4.Radius
						distanceWeight := distFromPhase4Edge / maxDistFromPhase4Edge

						// Combine alignment, distance weight, and shift utilization for total weight
						weight := alignment * distanceWeight * unplayableWeight

						// Update the heatmap cell
						heatmap[i][j] += weight
					}
				}
			}
		}

		validMatches++
	}

	if validMatches == 0 {
		return nil
	}

	// Normalize the heatmap
	maxValue := 0.0
	for i := range heatmap {
		for j := range heatmap[i] {
			if heatmap[i][j] > maxValue {
				maxValue = heatmap[i][j]
			}
		}
	}

	if maxValue > 0 {
		for i := range heatmap {
			for j := range heatmap[i] {
				heatmap[i][j] = heatmap[i][j] / maxValue
			}
		}
	}

	// Create the visualization
	img := image.NewRGBA(image.Rect(0, 0, ImageSize, ImageSize))

	// Draw the heatmap using a red-yellow-green-blue gradient
	for i := 0; i < ImageSize; i++ {
		for j := 0; j < ImageSize; j++ {
			value := heatmap[i][j]

			var col color.RGBA
			u255 := uint8(255)
			if value < 0.25 {
				// Blue (cold) - least likely unplayable
				blue := uint8(128 + 127*value*4)
				col = color.RGBA{0, 0, blue, 255}
			} else if value < 0.5 {
				// Blue to green transition
				blue := uint8(255 - 255*(value-0.25)*4)
				green := uint8(255 * (value - 0.25) * 4)
				col = color.RGBA{0, green, blue, 255}
			} else if value < 0.75 {
				// Green to yellow transition
				green := u255
				red := uint8(255 * (value - 0.5) * 4)
				col = color.RGBA{red, green, 0, 255}
			} else {
				// Yellow to red transition
				red := u255
				green := uint8(255 - 255*(value-0.75)*4)
				col = color.RGBA{red, green, 0, 255}
			}

			img.Set(i, j, col)
		}
	}

	// Save the image
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}
