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

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/models"
	"github.com/tonyjoanes/aeroflow/internal/server"
)

const durableName = "gate-service-flight-landed"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	natsURL := envOr("NATS_URL", "nats://localhost:4222")

	client, err := messaging.Connect(natsURL)
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

	mux := server.Mux()
	addr := envOr("HTTP_ADDR", ":8081")
	logger.Info("gate-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handleFlightLanded assigns a gate to a newly landed flight and publishes a
// GateAssignedEvent, continuing the event chain.
func handleFlightLanded(logger *slog.Logger, client *messaging.Client) messaging.Handler {
	return func(ctx context.Context, data []byte) error {
		var event models.FlightEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode flight event: %w", err)
		}

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
			return fmt.Errorf("publish gate assigned event: %w", err)
		}

		logger.Info("assigned gate",
			"flight", event.Flight.Number,
			"gate", assigned.Assignment.Gate.ID,
			"correlation_id", event.CorrelationID,
		)
		return nil
	}
}

// assignGate deterministically picks a gate for a flight number. It's a
// placeholder allocation strategy — good enough to drive the event chain
// until real gate-availability logic exists.
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
