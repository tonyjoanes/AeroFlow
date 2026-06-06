// Package messaging wraps the NATS JetStream client with the small set of
// pub/sub helpers AeroFlow services need.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Subject prefixes used across the AeroFlow event chain (aeroflow.>).
const (
	SubjectFlightLanded = "aeroflow.flights.landed"
	SubjectGateAssigned = "aeroflow.gates.assigned"
)

// Client wraps a NATS connection and JetStream context for publish/subscribe.
type Client struct {
	conn *nats.Conn
	js   jetstream.JetStream
}

// Connect dials NATS at url and initialises a JetStream context.
func Connect(url string) (*Client, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	return &Client{conn: conn, js: js}, nil
}

// Close drains and closes the underlying NATS connection.
func (c *Client) Close() {
	c.conn.Close()
}

// Publish marshals payload as JSON and publishes it to subject.
func (c *Client) Publish(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload for %s: %w", subject, err)
	}

	if _, err := c.js.Publish(ctx, subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}

	return nil
}

// Handler processes a single decoded message. Returning an error leaves the
// message unacknowledged so JetStream redelivers it.
type Handler func(ctx context.Context, data []byte) error

// Consume creates (or reuses) a durable consumer on stream for subject and
// invokes handler for every message, acking on success.
func (c *Client) Consume(ctx context.Context, stream, durable, subject string, handler Handler) (jetstream.ConsumeContext, error) {
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Durable:       durable,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("create consumer %s/%s: %w", stream, durable, err)
	}

	return consumer.Consume(func(msg jetstream.Msg) {
		if err := handler(ctx, msg.Data()); err != nil {
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	})
}
