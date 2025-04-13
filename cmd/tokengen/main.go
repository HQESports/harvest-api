package main

import (
	"context"
	"harvest/internal/config"
	"harvest/internal/controller"
	"harvest/internal/database"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Testing script to see if the rabbit MQ client functions correctly
func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
		os.Exit(1)
	}

	// Configure logging
	setupLogger(cfg.Logging)
	log.Info().Msg("Starting PUBG Harvest API")
	log.Info().Str("environment", cfg.Env).Int("port", cfg.Port).Msg("Configuration loaded")

	// Initialize MongoDB connection
	db, err := database.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database connection")
	}
	log.Info().Msg("Database connection established")

	tc := controller.NewToken(db)

	token, err := tc.GenerateInitialAdminToken(context.Background(), cfg.AppName)

	if err != nil {
		log.Error().Err(err).Msg("Error generating admin token")
		return
	}

	log.Info().Str("token", token).Msg("Generated initial admin token")
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
