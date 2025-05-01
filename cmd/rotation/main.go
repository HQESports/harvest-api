package main

import (
	"context"
	"harvest/internal/cache"
	"harvest/internal/config"
	"harvest/pkg/pubg"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config/dev.config.json")
	if err != nil {
		log.Error().Err(err).Msg("Failed to load configuration")
		return
	}

	setupLogger(cfg.Logging)

	// Initialize Redis connectoin
	cache, err := cache.NewRedisCache(cfg.Redis)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize redis cache connection")
		return
	}
	log.Info().Msg("Redis connection established")

	// Initialize PUBG API client
	pubgClient := pubg.New(cfg.PUBG, cache)

	playerNames := []string{"hwinn", "TGLTN", "Shrimzy", "FLC_Kickstart"}

	matchID := "182912fc-37e4-4b4e-aa93-0cd7cd524746"

	match, err := pubgClient.GetMatch(pubg.SteamPlatform, matchID)
	if err != nil {
		log.Fatal().Err(err).Msg("could not get match")
	}

	ok, str := match.ArePlayersOnSameTeam(playerNames)
	if !ok {
		log.Fatal().Msg(str)
	}

	telemURL, err := match.GetTelemetryURL()
	if err != nil {
		log.Fatal().Err(err).Msg("could not get telemetry URL")
	}

	rotations, err := pubgClient.BuildRotationsFromTelemetryYRL(context.TODO(), playerNames, telemURL)
	if err != nil {
		log.Fatal().Err(err).Msg("could not build rotations from telemetry URL")
	}

	pubg.GenerateRotationsImage("rotations.png", *rotations, "erangel.png")

	log.Info().Bool("On same team", ok).Msg(str)

	if err != nil {
		log.Fatal().Err(err).Msg("could not get match")
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
