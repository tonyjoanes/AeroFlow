package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/metrics"
	"github.com/tonyjoanes/aeroflow/internal/models"
	"github.com/tonyjoanes/aeroflow/internal/server"
	"github.com/tonyjoanes/aeroflow/internal/tracing"
)

const (
	serviceName = "gate-service"
	durableName = "gate-service-flight-landed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx := context.Background()
	shutdown, err := tracing.Init(ctx, serviceName)
	if err != nil {
		logger.Error("failed to init tracing", "error", err)
		os.Exit(1)
	}
	defer shutdown()

	svc := metrics.New(serviceName)

	client, err := messaging.Connect(envOr("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	consumeCtx, err := client.Consume(ctx, messaging.StreamName, durableName, messaging.SubjectFlightLanded, handleFlightLanded(logger, client, svc))
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	addr := envOr("HTTP_ADDR", ":8081")
	logger.Info("gate-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, svc.InstrumentMux(server.Mux())); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func handleFlightLanded(logger *slog.Logger, client *messaging.Client, svc *metrics.ServiceMetrics) messaging.Handler {
	tracer := tracing.Tracer(serviceName)

	return func(ctx context.Context, data []byte) error {
		ctx, span := tracer.Start(ctx, "gate.assign")
		defer span.End()
		done := svc.ObserveConsume(messaging.SubjectFlightLanded)

		var event models.FlightEvent
		if err := json.Unmarshal(data, &event); err != nil {
			err = fmt.Errorf("decode flight event: %w", err)
			done(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.SetAttributes(
			attribute.String("flight.number", event.Flight.Number),
			attribute.String("correlation.id", event.CorrelationID),
		)

		now := time.Now().UTC()
		assigned := models.GateAssignedEvent{
			Assignment: models.GateAssignment{
				FlightNumber: event.Flight.Number,
				Gate:         assignGate(event.Flight.Number),
				AssignedAt:   now,
			},
			CorrelationID: event.CorrelationID,
			OccurredAt:    now,
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := client.Publish(publishCtx, messaging.SubjectGateAssigned, assigned); err != nil {
			err = fmt.Errorf("publish gate assigned event: %w", err)
			done(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		svc.RecordPublish(messaging.SubjectGateAssigned)
		done(nil)
		logger.Info("assigned gate",
			"flight", event.Flight.Number,
			"gate", assigned.Assignment.Gate.ID,
			"correlation_id", event.CorrelationID,
		)
		return nil
	}
}

func assignGate(flightNumber string) models.Gate {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flightNumber))

	terminals := []string{"T1", "T2", "T3"}
	terminal := terminals[h.Sum32()%uint32(len(terminals))]
	gateNumber := (h.Sum32() % 20) + 1

	return models.Gate{
		ID:       fmt.Sprintf("%s-G%02d", terminal, gateNumber),
		Terminal: terminal,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
