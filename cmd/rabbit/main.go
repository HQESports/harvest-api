package main

import (
	"encoding/json"
	"fmt"
	"harvest/internal/config"
	"harvest/internal/rabbitmq"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type DemoMessage struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	// Set up logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	// Load configuration
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize RabbitMQ client
	client, err := rabbitmq.NewClientFromConfig(cfg.RabbitMQ)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create RabbitMQ client")
	}
	defer client.Close()

	// Check RabbitMQ health
	if err := client.Health(); err != nil {
		log.Fatal().Err(err).Msg("RabbitMQ health check failed")
	}
	log.Info().Msg("RabbitMQ health check passed")

	// Get exchange name, queue name and routing key from config
	exchangeName := cfg.RabbitMQ.ExchangeName
	queueName := cfg.RabbitMQ.QueueName
	routingKey := cfg.RabbitMQ.RoutingKey

	// Set up consumer in a goroutine
	go func() {
		// Start consuming messages
		deliveries, err := client.Consume(queueName, "demo-consumer")
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to start consuming")
		}

		log.Info().Msg("Waiting for messages. Press CTRL+C to exit.")

		for delivery := range deliveries {
			var message DemoMessage
			if err := json.Unmarshal(delivery.Body, &message); err != nil {
				log.Error().Err(err).Msg("Failed to unmarshal message")
				continue
			}

			log.Info().
				Str("id", message.ID).
				Str("content", message.Content).
				Time("timestamp", message.Timestamp).
				Msg("Received message")

			// Access message headers if needed
			if delivery.Headers != nil {
				for key, value := range delivery.Headers {
					log.Info().
						Str("header_key", key).
						Interface("header_value", value).
						Msg("Message header")
				}
			}
		}
	}()

	// Wait a moment for consumer to start
	time.Sleep(1 * time.Second)

	// Publish test messages
	for i := 1; i <= 3; i++ {
		// Create a message
		message := DemoMessage{
			ID:        fmt.Sprintf("msg-%d-%d", time.Now().Unix(), i),
			Content:   fmt.Sprintf("Test message #%d", i),
			Timestamp: time.Now(),
		}

		// Convert message to JSON
		messageBytes, err := json.Marshal(message)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal message")
			continue
		}

		// Add custom headers
		headers := make(map[string]interface{})
		headers["source"] = "demo"
		headers["message_number"] = i

		// Publish the message
		if err := client.Publish(exchangeName, routingKey, messageBytes, headers); err != nil {
			log.Error().Err(err).Msg("Failed to publish message")
			continue
		}

		log.Info().Int("message_number", i).Msg("Published message")

		// Small delay between messages
		time.Sleep(500 * time.Millisecond)
	}

	log.Info().Msg("Published all test messages. Listening for incoming messages...")

	// Keep the application running until interrupted
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info().Msg("Shutting down...")
}
