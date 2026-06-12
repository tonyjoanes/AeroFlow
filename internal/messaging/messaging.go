// Package messaging wraps the NATS JetStream client with the small set of
// pub/sub helpers AeroFlow services need.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
)

// Subject prefixes used across the AeroFlow event chain (aeroflow.>).
const (
	SubjectFlightLanded      = "aeroflow.flights.landed"
	SubjectGateAssigned      = "aeroflow.gates.assigned"
	SubjectBaggageStarted    = "aeroflow.baggage.started"
	SubjectCarouselAssigned  = "aeroflow.carousel.assigned"
	SubjectTurnaroundStarted = "aeroflow.turnaround.started"
	SubjectCrewAssigned      = "aeroflow.crew.assigned"
)

// StreamName is the JetStream stream every AeroFlow service publishes to and
// consumes from.
const StreamName = "AEROFLOW"

// streamSubjects is the wildcard the AEROFLOW stream captures.
const streamSubjects = "aeroflow.>"

// natsCarrier adapts nats.Header (map[string][]string) to the
// propagation.TextMapCarrier interface, which works with map[string]string.
type natsCarrier nats.Header

func (c natsCarrier) Get(key string) string {
	vals := nats.Header(c).Values(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c natsCarrier) Set(key, value string) {
	nats.Header(c).Set(key, value)
}

func (c natsCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

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

	if _, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: []string{streamSubjects},
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("create or update stream %s: %w", StreamName, err)
	}

	return &Client{conn: conn, js: js}, nil
}

// Close drains and closes the underlying NATS connection.
func (c *Client) Close() {
	c.conn.Close()
}

// Publish marshals payload as JSON and publishes it to subject, injecting
// the current trace context into NATS message headers so downstream
// consumers can continue the trace.
func (c *Client) Publish(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload for %s: %w", subject, err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  make(nats.Header),
	}
	otel.GetTextMapPropagator().Inject(ctx, natsCarrier(msg.Header))

	if _, err := c.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}

	return nil
}

// Handler processes a single decoded message. Returning an error leaves the
// message unacknowledged so JetStream redelivers it.
type Handler func(ctx context.Context, data []byte) error

// Consume creates (or reuses) a durable consumer on stream for subject and
// invokes handler for every message, extracting trace context from headers
// and acking on success.
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
		// Extract trace context from NATS headers so this span is a child
		// of the publisher's span.
		msgCtx := otel.GetTextMapPropagator().Extract(ctx, natsCarrier(msg.Headers()))
		if err := handler(msgCtx, msg.Data()); err != nil {
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	})
}
