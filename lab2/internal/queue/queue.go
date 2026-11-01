package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"lab2/internal/domain"
)

// MaxPriority is the highest priority the channel queues support.
const MaxPriority = 10

// Priorities used for the two notification priority levels.
const (
	priorityLow  = 1
	priorityHigh = 9
)

// Channels lists the delivery channels; each one gets its own queue.
var Channels = []string{"email", "push"}

// QueueName returns the name of the queue serving the given channel.
func QueueName(channel string) string {
	return "notifications." + channel
}

// RetryQueueName returns the delay queue of the given channel. Messages wait
// there until they expire and are then dead-lettered back to the main queue.
func RetryQueueName(channel string) string {
	return QueueName(channel) + ".retry"
}

type message struct {
	ID int64 `json:"id"`
}

// Broker holds a RabbitMQ connection and a confirm-mode channel for publishing.
type Broker struct {
	conn *amqp.Connection
	mu   sync.Mutex // amqp channels are not safe for concurrent publishing
	pub  *amqp.Channel
}

// Connect dials RabbitMQ and declares the queues of every channel. Declaring
// is idempotent, so the API and every worker can call it on start.
func Connect(url string) (*Broker, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	pub, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := pub.Confirm(false); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	for _, c := range Channels {
		_, err := pub.QueueDeclare(QueueName(c), true, false, false, false,
			amqp.Table{"x-max-priority": int32(MaxPriority)})
		if err == nil {
			_, err = pub.QueueDeclare(RetryQueueName(c), true, false, false, false, amqp.Table{
				"x-dead-letter-exchange":    "",
				"x-dead-letter-routing-key": QueueName(c),
			})
		}
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("declare queues of %s: %w", c, err)
		}
	}
	return &Broker{conn: conn, pub: pub}, nil
}

// Ping reports whether the connection to RabbitMQ is still open.
func (b *Broker) Ping(_ context.Context) error {
	if b.conn.IsClosed() {
		return errors.New("rabbitmq connection closed")
	}
	return nil
}

func (b *Broker) Close() error {
	return b.conn.Close()
}

// Publish sends a notification id to the queue of its channel and returns once
// the broker has confirmed it.
func (b *Broker) Publish(ctx context.Context, channel string, id int64, priority domain.Priority) error {
	return b.publish(ctx, QueueName(channel), id, priority, "")
}

// Retry sends the id to the delay queue; it returns to the main queue after delay.
func (b *Broker) Retry(ctx context.Context, channel string, id int64, priority domain.Priority, delay time.Duration) error {
	return b.publish(ctx, RetryQueueName(channel), id, priority,
		strconv.FormatInt(delay.Milliseconds(), 10))
}

func (b *Broker) publish(ctx context.Context, queue string, id int64, priority domain.Priority, expiration string) error {
	body, err := json.Marshal(message{ID: id})
	if err != nil {
		return err
	}
	p := uint8(priorityLow)
	if priority == domain.PriorityHigh {
		p = priorityHigh
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	confirm, err := b.pub.PublishWithDeferredConfirmWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Priority:     p,
		Expiration:   expiration,
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for confirm: %w", err)
	}
	if !acked {
		return errors.New("broker rejected the message")
	}
	return nil
}

// Consume handles messages of one channel one at a time until ctx is done. A
// message is acknowledged when handle succeeds and requeued when it fails.
func (b *Broker) Consume(ctx context.Context, channel string, handle func(ctx context.Context, id int64) error) error {
	ch, err := b.conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set prefetch: %w", err)
	}
	deliveries, err := ch.Consume(QueueName(channel), "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			var m message
			if err := json.Unmarshal(d.Body, &m); err != nil {
				log.Printf("dropping malformed message: %v", err)
				_ = d.Reject(false)
				continue
			}
			if err := handle(ctx, m.ID); err != nil {
				log.Printf("notification %d: %v", m.ID, err)
				_ = d.Nack(false, true)
				time.Sleep(time.Second) // avoid a hot loop on a persistent failure
				continue
			}
			_ = d.Ack(false)
		}
	}
}
