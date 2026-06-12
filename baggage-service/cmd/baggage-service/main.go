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

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/models"
	"github.com/tonyjoanes/aeroflow/internal/server"
)

const durableName = "baggage-service-flight-landed"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client, err := messaging.Connect(envOr("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	consumeCtx, err := client.Consume(ctx, messaging.StreamName, durableName, messaging.SubjectFlightLanded, handleFlightLanded(logger, client))
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	addr := envOr("HTTP_ADDR", ":8082")
	logger.Info("baggage-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Mux()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handleFlightLanded creates a baggage job for each landed flight and
// publishes BaggageStartedEvent to continue the event chain.
func handleFlightLanded(logger *slog.Logger, client *messaging.Client) messaging.Handler {
	return func(ctx context.Context, data []byte) error {
		var event models.FlightEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode flight event: %w", err)
		}

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
			return fmt.Errorf("publish baggage started: %w", err)
		}

		logger.Info("baggage job created", "flight", event.Flight.Number, "job_id", job.ID, "correlation_id", event.CorrelationID)
		return nil
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
