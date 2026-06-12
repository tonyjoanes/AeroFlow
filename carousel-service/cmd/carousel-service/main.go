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
	serviceName = "carousel-service"
	durableName = "carousel-service-baggage-started"
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

	consumeCtx, err := client.Consume(ctx, messaging.StreamName, durableName, messaging.SubjectBaggageStarted, handleBaggageStarted(logger, client, svc))
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	addr := envOr("HTTP_ADDR", ":8083")
	logger.Info("carousel-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, svc.InstrumentMux(server.Mux())); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func handleBaggageStarted(logger *slog.Logger, client *messaging.Client, svc *metrics.ServiceMetrics) messaging.Handler {
	tracer := tracing.Tracer(serviceName)

	return func(ctx context.Context, data []byte) error {
		ctx, span := tracer.Start(ctx, "carousel.assign")
		defer span.End()
		done := svc.ObserveConsume(messaging.SubjectBaggageStarted)

		var event models.BaggageStartedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			err = fmt.Errorf("decode baggage started event: %w", err)
			done(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.SetAttributes(attribute.String("flight.number", event.Job.FlightNumber))

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
			err = fmt.Errorf("publish carousel assigned: %w", err)
			done(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		svc.RecordPublish(messaging.SubjectCarouselAssigned)
		done(nil)
		logger.Info("carousel assigned",
			"flight", event.Job.FlightNumber,
			"carousel", assignment.Carousel.ID,
			"correlation_id", event.CorrelationID,
		)
		return nil
	}
}

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
