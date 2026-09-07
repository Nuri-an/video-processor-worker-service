package main

import (
	"fmt"

	"github.com/rabbitmq/amqp091-go"
)

type RabbitConfig struct {
	URL   string
	Queue string
}

type RabbitConsumer struct {
	config  RabbitConfig
	conn    *amqp091.Connection
	channel *amqp091.Channel
}

func NewRabbitConsumer(config RabbitConfig) (*RabbitConsumer, error) {
	conn, err := amqp091.Dial(config.URL)
	if err != nil {
		return nil, err
	}
	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &RabbitConsumer{config: config, conn: conn, channel: channel}, nil
}

func (q *RabbitConsumer) Consume() (<-chan amqp091.Delivery, error) {
	if _, err := q.channel.QueueDeclare(q.config.Queue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return q.channel.Consume(q.config.Queue, "video-processor-worker", false, false, false, false, nil)
}

func (q *RabbitConsumer) Close() {
	q.channel.Close()
	q.conn.Close()
}

func rabbitConfig() RabbitConfig {
	return RabbitConfig{
		URL: fmt.Sprintf("amqp://%s:%s@%s:%s/", envOr("RABBITMQ_USER", "video_processor"), envOr("RABBITMQ_PASSWORD", "video_processor"), envOr("RABBITMQ_HOST", "localhost"), envOr("RABBITMQ_PORT", "5672")),
		Queue: envOr("RABBITMQ_QUEUE", "video_jobs"),
	}
}
