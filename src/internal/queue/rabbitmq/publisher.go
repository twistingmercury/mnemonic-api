// Package rabbitmq provides a RabbitMQ-backed implementation of queue.Publisher.
package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/google/uuid"
	"github.com/twistingmercury/mnemonic-api/internal/queue"
)

// PublisherConfig holds the connection parameters for the RabbitMQ publisher.
type PublisherConfig struct {
	Host           string
	Port           int
	User           string
	Password       string // #nosec G117
	VHost          string
	Queue          string
	ReconnectDelay time.Duration
}

// amqpURL builds the AMQP connection URL from the config fields.
func (c PublisherConfig) amqpURL() string {
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s", c.User, c.Password, c.Host, c.Port, c.VHost)
}

// RabbitMQPublisher implements queue.Publisher backed by a RabbitMQ broker.
type RabbitMQPublisher struct {
	cfg  PublisherConfig
	conn *amqp.Connection
	ch   *amqp.Channel
	mu   sync.Mutex
}

// NewPublisher dials the broker, opens a channel, and declares the destination
// queue. It returns an error if any step fails, cleaning up partial resources
// before returning.
func NewPublisher(cfg PublisherConfig) (queue.Publisher, error) {
	conn, err := amqp.Dial(cfg.amqpURL())
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}

	if _, err = ch.QueueDeclare(cfg.Queue, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare queue %q: %w", cfg.Queue, err)
	}

	return &RabbitMQPublisher{cfg: cfg, conn: conn, ch: ch}, nil
}

// jobPayload is the JSON body published for each enrichment job.
type jobPayload struct {
	JobID string `json:"job_id"`
}

// Publish serialises jobID into a JSON message and delivers it to the
// configured queue with persistent delivery mode. On publish failure it
// attempts a single reconnect and retry before returning the error.
func (p *RabbitMQPublisher) Publish(ctx context.Context, jobID uuid.UUID) error {
	body, err := json.Marshal(jobPayload{JobID: jobID.String()})
	if err != nil {
		return fmt.Errorf("rabbitmq: marshal payload: %w", err)
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         body,
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err = p.ch.PublishWithContext(ctx, "", p.cfg.Queue, false, false, msg); err != nil {
		if reconnErr := p.reconnect(ctx); reconnErr != nil {
			return fmt.Errorf("rabbitmq: publish failed and reconnect failed: %w", errors.Join(err, reconnErr))
		}
		if retryErr := p.ch.PublishWithContext(ctx, "", p.cfg.Queue, false, false, msg); retryErr != nil {
			return fmt.Errorf("rabbitmq: publish retry: %w", retryErr)
		}
	}

	return nil
}

// reconnect closes the stale channel and connection, then re-dials the broker
// and re-declares the queue. It replaces p.conn and p.ch on success.
func (p *RabbitMQPublisher) reconnect(ctx context.Context) error {
	// Ignore close errors on stale resources.
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}

	select {
	case <-time.After(p.cfg.ReconnectDelay):
	case <-ctx.Done():
		return fmt.Errorf("rabbitmq: reconnect cancelled: %w", ctx.Err())
	}

	conn, err := amqp.Dial(p.cfg.amqpURL())
	if err != nil {
		return fmt.Errorf("rabbitmq: reconnect dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("rabbitmq: reconnect open channel: %w", err)
	}

	if _, err = ch.QueueDeclare(p.cfg.Queue, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("rabbitmq: reconnect declare queue %q: %w", p.cfg.Queue, err)
	}

	p.conn = conn
	p.ch = ch
	return nil
}

// Close releases the channel and connection. Both close errors are joined and
// returned together so callers observe the full picture.
func (p *RabbitMQPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var chErr, connErr error
	if p.ch != nil {
		chErr = p.ch.Close()
	}
	if p.conn != nil {
		connErr = p.conn.Close()
	}
	return errors.Join(chErr, connErr)
}
