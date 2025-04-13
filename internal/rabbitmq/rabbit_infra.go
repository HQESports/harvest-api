package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

func (c *client) DeclareExchange(name, kind string) error {
	err := c.channel.ExchangeDeclare(
		name, kind, true, false, false, false, nil,
	)
	if err != nil {
		log.Error().Err(err).Str("exchange", name).Msg("Failed to declare exchange")
	} else {
		log.Info().Str("exchange", name).Str("type", kind).Msg("Declared exchange")
	}
	return err
}

func (c *client) DeclareQueue(name string) (amqp.Queue, error) {
	queue, err := c.channel.QueueDeclare(
		name, true, false, false, false, nil,
	)
	if err != nil {
		log.Error().Err(err).Str("queue", name).Msg("Failed to declare queue")
	} else {
		log.Info().Str("queue", name).Msg("Declared queue")
	}
	return queue, err
}

func (c *client) BindQueue(queueName, exchangeName, routingKey string) error {
	err := c.channel.QueueBind(
		queueName, routingKey, exchangeName, false, nil,
	)
	if err != nil {
		log.Error().
			Err(err).
			Str("queue", queueName).
			Str("exchange", exchangeName).
			Str("routingKey", routingKey).
			Msg("Failed to bind queue")
	} else {
		log.Info().
			Str("queue", queueName).
			Str("exchange", exchangeName).
			Str("routingKey", routingKey).
			Msg("Bound queue to exchange")
	}
	return err
}
