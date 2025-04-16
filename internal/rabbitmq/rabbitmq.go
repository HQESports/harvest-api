package rabbitmq

import (
	"context"
	"fmt"
	"harvest/internal/config"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

type Client interface {
	Close() error

	DeclareExchange(name, kind string) error
	DeclareQueue(name string) (amqp.Queue, error)
	BindQueue(queueName, exchangeName, routingKey string) error

	Publish(exchange, routingKey string, body []byte, headers amqp.Table) error
	Consume(queueName string, consumerTag string) (<-chan amqp.Delivery, error)

	Health() error
}

type client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	config  config.RabbitMQConfig
}

func NewClientFromConfig(cfg config.RabbitMQConfig) (Client, error) {
	amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.VHost,
	)

	conn, err := amqp.DialConfig(amqpURL, amqp.Config{
		Heartbeat: 30 * time.Second, // Set heartbeat interval
		Locale:    "en_US",
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to RabbitMQ")
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open RabbitMQ channel")
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	if cfg.PrefetchCount > 0 {
		if err := ch.Qos(cfg.PrefetchCount, 0, false); err != nil {
			log.Error().Err(err).Msg("Failed to set channel QoS")
			conn.Close()
			return nil, fmt.Errorf("failed to set QoS: %w", err)
		}
	}

	log.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("vhost", cfg.VHost).
		Msg("RabbitMQ connection established")

	return &client{conn: conn, channel: ch, config: cfg}, nil
}

func (c *client) Health() error {
	if c.conn == nil || c.channel == nil {
		log.Error().Msg("RabbitMQ health check failed: nil connection or channel")
		return fmt.Errorf("nil connection or channel")
	}

	if c.conn.IsClosed() {
		log.Error().Msg("RabbitMQ connection is closed")
		return fmt.Errorf("connection is closed")
	}

	// Try a passive declare to validate channel health
	err := c.channel.ExchangeDeclarePassive(
		c.config.ExchangeName, // doesn't need to exist
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		log.Error().Err(err).Msg("RabbitMQ health check failed on passive exchange declare")
		return err
	}

	log.Debug().Msg("RabbitMQ is healthy")
	return nil
}

func (c *client) Close() error {
	if err := c.channel.Close(); err != nil {
		log.Error().Err(err).Msg("Failed to close RabbitMQ channel")
		return fmt.Errorf("channel close error: %w", err)
	}
	if err := c.conn.Close(); err != nil {
		log.Error().Err(err).Msg("Failed to close RabbitMQ connection")
		return fmt.Errorf("connection close error: %w", err)
	}
	log.Info().Msg("RabbitMQ connection and channel closed")
	return nil
}

func (c *client) Publish(exchange, routingKey string, body []byte, headers amqp.Table) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
		Headers:     headers,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("exchange", exchange).
			Str("routingKey", routingKey).
			Msg("Failed to publish message")
	} else {
		log.Info().
			Str("exchange", exchange).
			Str("routingKey", routingKey).
			Int("size", len(body)).
			Msg("Published message")
	}
	return err
}

func (c *client) Consume(queueName string, consumerTag string) (<-chan amqp.Delivery, error) {
	deliveries, err := c.channel.Consume(
		queueName, consumerTag, false, false, false, false, nil,
	)
	if err != nil {
		log.Error().
			Err(err).
			Str("queue", queueName).
			Str("consumerTag", consumerTag).
			Msg("Failed to start consuming")
		return nil, fmt.Errorf("consume error: %w", err)
	}

	log.Info().
		Str("queue", queueName).
		Str("consumerTag", consumerTag).
		Msg("Started consuming messages")

	return deliveries, nil
}
