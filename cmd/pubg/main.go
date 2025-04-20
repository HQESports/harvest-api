package main

import (
	"context"
	"fmt"
	"harvest/internal/cache"
	"harvest/internal/config"
	"harvest/pkg/pubg"

	"os"
	"time"

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
	log.Info().Msg("Starting PUBG client initialization")

	// Initialize Redis connection
	cache, err := cache.NewRedisCache(cfg.Redis)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize redis cache connection")
		return
	}
	log.Info().Msg("Redis connection established")

	// Initialize PUBG API client
	pubgClient := pubg.New(cfg.PUBG, cache)
	defer pubgClient.Close()
	log.Info().Msg("PUBG API client initialized")

	// Your code to use the pubgClient would go here
	const telmURL = "https://telemetry-cdn.pubg.com/bluehole-pubg/pc-tournament/2018/06/15/10/31/40f093c7-7087-11e8-b007-0a5864669f05-telemetry.json"
	telemData, err := pubgClient.ProcessTelemetryFromURL(context.Background(), telmURL)

	if err != nil {
		log.Error().Err(err).Msg("could not process telemetry data")
		return
	}

	telemData.GenerateCirclesImage("test.png", "erangel.png")
	fmt.Println(telemData.String())
}

func setupLogger(config config.LoggingConfig) {
	// Set global log level
	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure logger output
	switch config.Format {
	case "json":
		// JSON is the default for zerolog
	case "console", "combined":
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	}

	// Add timestamp
	log.Logger = log.With().Timestamp().Logger()
}
