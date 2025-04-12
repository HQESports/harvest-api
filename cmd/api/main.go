package main

import (
	"context"
	"fmt"
	"harvest/internal/config"
	"harvest/internal/database"
	"harvest/internal/server"
	"harvest/pkg/pubg"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
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

	// Initialize PUBG API client
	pubgClient := pubg.New(
		cfg.PUBG.APIKey,
		cfg.PUBG.BaseURL,
		cfg.PUBG.OldEnoughCap,
		cfg.PUBG.RequestsPerMinute,
	)
	defer pubgClient.Close()
	log.Info().Msg("PUBG API client initialized")

	// Create and start HTTP server
	srv := server.New(*cfg, db, *pubgClient)

	// Start the server in a goroutine to avoid blocking
	go func() {
		log.Info().Int("port", cfg.Port).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil {
			if err.Error() != "http: Server closed" {
				log.Error().Err(err).Msg("HTTP server error")
			}
		}
	}()

	// Set up graceful shutdown
	gracefulShutdown(srv)
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

func gracefulShutdown(srv *http.Server) {
	// Create channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("Shutting down server")

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt to gracefully shut down the server
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
}
