package queue

import (
	"context"
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MaxPriority is the highest priority the channel queues support.
const MaxPriority = 10

// Channels lists the delivery channels; each one gets its own queue.
var Channels = []string{"email", "push"}

// QueueName returns the name of the queue serving the given channel.
func QueueName(channel string) string {
	return "notifications." + channel
}

// Broker holds a RabbitMQ connection and a channel with the queues declared.
type Broker struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Connect dials RabbitMQ and declares one durable priority queue per channel.
// Declaring is idempotent, so the API and every worker can call it on start.
func Connect(url string) (*Broker, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	for _, c := range Channels {
		_, err := ch.QueueDeclare(QueueName(c), true, false, false, false,
			amqp.Table{"x-max-priority": int32(MaxPriority)})
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("declare queue %s: %w", QueueName(c), err)
		}
	}
	return &Broker{conn: conn, ch: ch}, nil
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
