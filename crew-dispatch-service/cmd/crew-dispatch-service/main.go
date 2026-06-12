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

const durableName = "crew-dispatch-service-flight-landed"

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

	addr := envOr("HTTP_ADDR", ":8085")
	logger.Info("crew-dispatch-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Mux()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handleFlightLanded assigns a crew to the landed aircraft and publishes
// CrewAssignedEvent.
func handleFlightLanded(logger *slog.Logger, client *messaging.Client) messaging.Handler {
	return func(ctx context.Context, data []byte) error {
		var event models.FlightEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode flight event: %w", err)
		}

		now := time.Now().UTC()
		assignment := models.CrewAssignment{
			FlightNumber: event.Flight.Number,
			Crew:         dispatchCrew(event.Flight.Number),
			AssignedAt:   now,
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		evt := models.CrewAssignedEvent{
			Assignment:    assignment,
			CorrelationID: event.CorrelationID,
			OccurredAt:    now,
		}
		if err := client.Publish(publishCtx, messaging.SubjectCrewAssigned, evt); err != nil {
			return fmt.Errorf("publish crew assigned: %w", err)
		}

		logger.Info("crew dispatched",
			"flight", event.Flight.Number,
			"crew_count", len(assignment.Crew),
			"correlation_id", event.CorrelationID,
		)
		return nil
	}
}

// dispatchCrew builds a deterministic crew roster from the flight number.
// Real allocation would check availability and qualifications.
func dispatchCrew(flightNumber string) []models.CrewMember {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flightNumber))
	seed := h.Sum32()

	captains := []string{"Capt. Sharma", "Capt. O'Brien", "Capt. Nakamura"}
	captain := captains[seed%uint32(len(captains))]

	return []models.CrewMember{
		{ID: fmt.Sprintf("crew-%d-1", seed), Name: captain, Role: models.CrewRoleCaptain},
		{ID: fmt.Sprintf("crew-%d-2", seed), Name: "F/O Williams", Role: models.CrewRoleCabin},
		{ID: fmt.Sprintf("crew-%d-3", seed), Name: "Ground Agent Lee", Role: models.CrewRoleGroundOps},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
