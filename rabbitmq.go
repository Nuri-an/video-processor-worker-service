package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type RabbitConfig struct {
	URL            string
	Queue          string
	DLX            string
	Prefetch       int
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ConfirmTimeout time.Duration
}

type RabbitConsumer struct {
	config  RabbitConfig
	conn    *amqp091.Connection
	channel *amqp091.Channel
	confirms <-chan amqp091.Confirmation
	mu      sync.Mutex
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
	if err := channel.Confirm(false); err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("habilitar publisher confirms: %w", err)
	}
	return &RabbitConsumer{
		config:   config,
		conn:     conn,
		channel:  channel,
		confirms: channel.NotifyPublish(make(chan amqp091.Confirmation, 16)),
	}, nil
}

func (q *RabbitConsumer) Consume() (<-chan amqp091.Delivery, error) {
	if err := q.declareTopology(); err != nil {
		return nil, err
	}
	if err := q.channel.Qos(q.config.Prefetch, 0, false); err != nil {
		return nil, err
	}
	return q.channel.Consume(q.config.Queue, "video-processor-worker", false, false, false, false, nil)
}

func (q *RabbitConsumer) declareTopology() error {
	if _, err := q.channel.ExchangeDeclare(q.config.DLX, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	deadQueue := q.config.Queue + ".dead"
	if _, err := q.channel.QueueDeclare(deadQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := q.channel.QueueBind(deadQueue, deadQueue, q.config.DLX, false, nil); err != nil {
		return err
	}
	if _, err := q.channel.QueueDeclare(q.config.Queue, true, false, false, false, amqp091.Table{
		"x-dead-letter-exchange":    q.config.DLX,
		"x-dead-letter-routing-key": deadQueue,
	}); err != nil {
		return err
	}
	for attempt := 1; attempt < q.config.MaxAttempts; attempt++ {
		name := q.retryQueue(attempt)
		_, err := q.channel.QueueDeclare(name, true, false, false, false, amqp091.Table{
			"x-message-ttl":             int(q.retryDelay(attempt).Milliseconds()),
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": q.config.Queue,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (q *RabbitConsumer) retryQueue(attempt int) string {
	return fmt.Sprintf("%s.retry.%d", q.config.Queue, attempt)
}

func (q *RabbitConsumer) retryDelay(attempt int) time.Duration {
	return q.config.RetryBaseDelay * time.Duration(1<<(attempt-1))
}

func (q *RabbitConsumer) PublishRetry(message amqp091.Delivery, attempt int) error {
	return q.publish(q.retryQueue(attempt), message.Body, amqp091.Table{"x-retry-attempt": attempt})
}

func (q *RabbitConsumer) PublishDeadLetter(message amqp091.Delivery) error {
	return q.publishWithExchange(q.config.DLX, q.config.Queue+".dead", message.Body, nil)
}

func (q *RabbitConsumer) publish(routingKey string, body []byte, headers amqp091.Table) error {
	return q.publishWithExchange("", routingKey, body, headers)
}

func (q *RabbitConsumer) publishWithExchange(exchange, routingKey string, body []byte, headers amqp091.Table) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.channel.Publish(exchange, routingKey, false, false, amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		Headers:      headers,
		Body:         body,
	}); err != nil {
		return err
	}
	select {
	case confirmation := <-q.confirms:
		if !confirmation.Ack {
			return fmt.Errorf("rabbitmq rejeitou a mensagem")
		}
		return nil
	case <-time.After(q.config.ConfirmTimeout):
		return fmt.Errorf("timeout aguardando confirmacao rabbitmq")
	}
}

func (q *RabbitConsumer) Close() {
	q.channel.Close()
	q.conn.Close()
}

func rabbitConfig() RabbitConfig {
	maxAttempts := envInt("WORKER_MAX_ATTEMPTS", 3)
	if maxAttempts > 10 {
		maxAttempts = 10
	}
	return RabbitConfig{
		URL:            fmt.Sprintf("amqp://%s:%s@%s:%s/", envOr("RABBITMQ_USER", ""), envOr("RABBITMQ_PASSWORD", ""), envOr("RABBITMQ_HOST", "localhost"), envOr("RABBITMQ_PORT", "5672")),
		Queue:          envOr("RABBITMQ_QUEUE", "video_jobs"),
		DLX:            envOr("RABBITMQ_DLX", "video_jobs.dlx"),
		Prefetch:       envInt("WORKER_PREFETCH", envInt("WORKER_CONCURRENCY", 2)),
		MaxAttempts:    maxAttempts,
		RetryBaseDelay: envDuration("WORKER_RETRY_BASE_DELAY", 5*time.Second),
		ConfirmTimeout: envDuration("RABBITMQ_CONFIRM_TIMEOUT", 5*time.Second),
	}
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}
