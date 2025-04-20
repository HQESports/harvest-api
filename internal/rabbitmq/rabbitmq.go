package rabbitmq

import (
	"context"
	"fmt"
	"harvest/internal/config"
	"sync"
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
	conn         *amqp.Connection
	channel      *amqp.Channel
	config       config.RabbitMQConfig
	mu           sync.Mutex
	reconnecting bool
	notifyClose  chan *amqp.Error
}

func NewClientFromConfig(cfg config.RabbitMQConfig) (Client, error) {
	c := &client{
		config:       cfg,
		reconnecting: false,
	}

	if err := c.connect(); err != nil {
		return nil, err
	}

	// Setup reconnection handling
	c.setupReconnect()

	return c, nil
}

func (c *client) connect() error {
	amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		c.config.Username,
		c.config.Password,
		c.config.Host,
		c.config.Port,
		c.config.VHost,
	)

	conn, err := amqp.DialConfig(amqpURL, amqp.Config{
		Heartbeat: 30 * time.Second, // Set heartbeat interval
		Locale:    "en_US",
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to RabbitMQ")
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open RabbitMQ channel")
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	if c.config.PrefetchCount > 0 {
		if err := ch.Qos(c.config.PrefetchCount, 0, false); err != nil {
			log.Error().Err(err).Msg("Failed to set channel QoS")
			conn.Close()
			return fmt.Errorf("failed to set QoS: %w", err)
		}
	}

	c.conn = conn
	c.channel = ch

	log.Info().
		Str("host", c.config.Host).
		Int("port", c.config.Port).
		Str("vhost", c.config.VHost).
		Msg("RabbitMQ connection established")

	return nil
}

func (c *client) setupReconnect() {
	c.notifyClose = c.conn.NotifyClose(make(chan *amqp.Error))

	// Start a goroutine to handle connection failures
	go func() {
		for err := range c.notifyClose {
			log.Warn().
				Str("reason", err.Reason).
				Int("code", err.Code).
				Bool("recover", err.Recover).
				Msg("RabbitMQ connection closed, attempting to reconnect...")

			// Begin reconnection attempts
			c.doReconnect()
		}
	}()
}

func (c *client) doReconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.reconnecting {
		return
	}

	c.reconnecting = true
	defer func() { c.reconnecting = false }()

	// Close existing resources if they're still open
	if c.channel != nil {
		c.channel.Close()
	}

	if c.conn != nil && !c.conn.IsClosed() {
		c.conn.Close()
	}

	// Attempt reconnection with backoff
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		log.Info().Dur("backoff", backoff).Msg("Attempting to reconnect to RabbitMQ")

		if err := c.connect(); err != nil {
			log.Error().Err(err).Msg("Failed to reconnect to RabbitMQ")

			// Exponential backoff with cap
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Setup the notification channel again
		c.notifyClose = c.conn.NotifyClose(make(chan *amqp.Error))

		log.Info().Msg("Successfully reconnected to RabbitMQ")
		return
	}
}

func (c *client) Health() error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close RabbitMQ channel")
			return fmt.Errorf("channel close error: %w", err)
		}
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close RabbitMQ connection")
			return fmt.Errorf("connection close error: %w", err)
		}
	}

	log.Info().Msg("RabbitMQ connection and channel closed")
	return nil
}

func (c *client) Publish(exchange, routingKey string, body []byte, headers amqp.Table) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connection check and auto-reconnect
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() {
		if err := c.connect(); err != nil {
			return fmt.Errorf("failed to reconnect before publishing: %w", err)
		}

		// Re-setup the reconnect hooks
		c.setupReconnect()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent, // Make messages persistent
		Body:         body,
		Headers:      headers,
	})

	if err != nil {
		log.Error().
			Err(err).
			Str("exchange", exchange).
			Str("routingKey", routingKey).
			Msg("Failed to publish message")

		// If publish fails due to connection issues, try to reconnect once and retry
		if err.Error() == "Exception (504) Reason: \"channel/connection is not open\"" {
			if err := c.connect(); err == nil {
				c.setupReconnect()

				// Try publishing again after successful reconnection
				retryErr := c.channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
					ContentType:  "application/json",
					DeliveryMode: amqp.Persistent,
					Body:         body,
					Headers:      headers,
				})

				if retryErr == nil {
					log.Info().
						Str("exchange", exchange).
						Str("routingKey", routingKey).
						Int("size", len(body)).
						Msg("Published message after reconnection")
					return nil
				}
			}
		}

		return err
	}

	log.Info().
		Str("exchange", exchange).
		Str("routingKey", routingKey).
		Int("size", len(body)).
		Msg("Published message")

	return nil
}

func (c *client) Consume(queueName string, consumerTag string) (<-chan amqp.Delivery, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connection check and auto-reconnect
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() {
		if err := c.connect(); err != nil {
			return nil, fmt.Errorf("failed to reconnect before consuming: %w", err)
		}

		// Re-setup the reconnect hooks
		c.setupReconnect()
	}

	deliveries, err := c.channel.Consume(
		queueName,   // queue
		consumerTag, // consumer
		false,       // auto-ack (important: set to false for manual ack)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
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

// Implement remaining interface methods
func (c *client) DeclareExchange(name, kind string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connection check and auto-reconnect
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() {
		if err := c.connect(); err != nil {
			return fmt.Errorf("failed to reconnect before declaring exchange: %w", err)
		}

		// Re-setup the reconnect hooks
		c.setupReconnect()
	}

	return c.channel.ExchangeDeclare(
		name,  // name
		kind,  // type
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
}

func (c *client) DeclareQueue(name string) (amqp.Queue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connection check and auto-reconnect
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() {
		if err := c.connect(); err != nil {
			return amqp.Queue{}, fmt.Errorf("failed to reconnect before declaring queue: %w", err)
		}

		// Re-setup the reconnect hooks
		c.setupReconnect()
	}

	return c.channel.QueueDeclare(
		name,  // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
}

func (c *client) BindQueue(queueName, exchangeName, routingKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Connection check and auto-reconnect
	if c.conn == nil || c.channel == nil || c.conn.IsClosed() {
		if err := c.connect(); err != nil {
			return fmt.Errorf("failed to reconnect before binding queue: %w", err)
		}

		// Re-setup the reconnect hooks
		c.setupReconnect()
	}

	return c.channel.QueueBind(
		queueName,    // queue name
		routingKey,   // routing key
		exchangeName, // exchange
		false,        // no-wait
		nil,          // arguments
	)
}
