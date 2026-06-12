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

const durableName = "turnaround-service-flight-landed"

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

	addr := envOr("HTTP_ADDR", ":8084")
	logger.Info("turnaround-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Mux()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handleFlightLanded starts ground ops coordination for the landed aircraft
// and publishes TurnaroundStartedEvent.
func handleFlightLanded(logger *slog.Logger, client *messaging.Client) messaging.Handler {
	return func(ctx context.Context, data []byte) error {
		var event models.FlightEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode flight event: %w", err)
		}

		now := time.Now().UTC()
		turnaround := models.Turnaround{
			ID:           uuid.NewString(),
			FlightNumber: event.Flight.Number,
			Status:       models.TurnaroundStarted,
			StartedAt:    now,
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		evt := models.TurnaroundStartedEvent{
			Turnaround:    turnaround,
			CorrelationID: event.CorrelationID,
			OccurredAt:    now,
		}
		if err := client.Publish(publishCtx, messaging.SubjectTurnaroundStarted, evt); err != nil {
			return fmt.Errorf("publish turnaround started: %w", err)
		}

		logger.Info("turnaround started",
			"flight", event.Flight.Number,
			"turnaround_id", turnaround.ID,
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
