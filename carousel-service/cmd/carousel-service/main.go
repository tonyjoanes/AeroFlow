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

const durableName = "carousel-service-baggage-started"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client, err := messaging.Connect(envOr("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()
	consumeCtx, err := client.Consume(ctx, messaging.StreamName, durableName, messaging.SubjectBaggageStarted, handleBaggageStarted(logger, client))
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	addr := envOr("HTTP_ADDR", ":8083")
	logger.Info("carousel-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Mux()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// handleBaggageStarted assigns a carousel to the baggage job and publishes
// CarouselAssignedEvent.
func handleBaggageStarted(logger *slog.Logger, client *messaging.Client) messaging.Handler {
	return func(ctx context.Context, data []byte) error {
		var event models.BaggageStartedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode baggage started event: %w", err)
		}

		now := time.Now().UTC()
		assignment := models.CarouselAssignment{
			JobID:        event.Job.ID,
			FlightNumber: event.Job.FlightNumber,
			Carousel:     assignCarousel(event.Job.FlightNumber),
			AssignedAt:   now,
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		evt := models.CarouselAssignedEvent{
			Assignment:    assignment,
			CorrelationID: event.CorrelationID,
			OccurredAt:    now,
		}
		if err := client.Publish(publishCtx, messaging.SubjectCarouselAssigned, evt); err != nil {
			return fmt.Errorf("publish carousel assigned: %w", err)
		}

		logger.Info("carousel assigned",
			"flight", event.Job.FlightNumber,
			"carousel", assignment.Carousel.ID,
			"correlation_id", event.CorrelationID,
		)
		return nil
	}
}

// assignCarousel deterministically picks a carousel from the flight number.
func assignCarousel(flightNumber string) models.Carousel {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flightNumber))

	terminals := []string{"T1", "T2", "T3"}
	terminal := terminals[h.Sum32()%uint32(len(terminals))]
	belt := (h.Sum32() % 6) + 1

	return models.Carousel{
		ID:       fmt.Sprintf("%s-C%02d", terminal, belt),
		Terminal: terminal,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
