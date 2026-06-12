package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/metrics"
	"github.com/tonyjoanes/aeroflow/internal/models"
	"github.com/tonyjoanes/aeroflow/internal/server"
	"github.com/tonyjoanes/aeroflow/internal/tracing"
)

const (
	serviceName = "baggage-service"
	durableName = "baggage-service-flight-landed"
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

	addr := envOr("HTTP_ADDR", ":8082")
	logger.Info("baggage-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, svc.InstrumentMux(server.Mux())); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func handleFlightLanded(logger *slog.Logger, client *messaging.Client, svc *metrics.ServiceMetrics) messaging.Handler {
	tracer := tracing.Tracer(serviceName)

	return func(ctx context.Context, data []byte) error {
		ctx, span := tracer.Start(ctx, "baggage.create_job")
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

		span.SetAttributes(attribute.String("flight.number", event.Flight.Number))

		now := time.Now().UTC()
		job := models.BaggageJob{
			ID:           uuid.NewString(),
			FlightNumber: event.Flight.Number,
			Status:       models.BaggageStarted,
			CreatedAt:    now,
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		evt := models.BaggageStartedEvent{
			Job:           job,
			CorrelationID: event.CorrelationID,
			OccurredAt:    now,
		}
		if err := client.Publish(publishCtx, messaging.SubjectBaggageStarted, evt); err != nil {
			err = fmt.Errorf("publish baggage started: %w", err)
			done(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		svc.RecordPublish(messaging.SubjectBaggageStarted)
		done(nil)
		logger.Info("baggage job created",
			"flight", event.Flight.Number,
			"job_id", job.ID,
			"correlation_id", event.CorrelationID,
		)
		return nil
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
