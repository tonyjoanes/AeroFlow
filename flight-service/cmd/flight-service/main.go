package main

import (
	"context"
	"encoding/json"
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

const serviceName = "flight-service"

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

	mux := server.Mux()
	mux.HandleFunc("POST /flights/land", landHandler(logger, client, svc))

	addr := envOr("HTTP_ADDR", ":8080")
	logger.Info("flight-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, svc.InstrumentMux(mux)); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

type landRequest struct {
	Number      string `json:"number"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

func landHandler(logger *slog.Logger, client *messaging.Client, svc *metrics.ServiceMetrics) http.HandlerFunc {
	tracer := tracing.Tracer(serviceName)

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "flights.land")
		defer span.End()

		var req landRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Number == "" {
			http.Error(w, "number is required", http.StatusBadRequest)
			return
		}

		span.SetAttributes(
			attribute.String("flight.number", req.Number),
			attribute.String("flight.origin", req.Origin),
			attribute.String("flight.destination", req.Destination),
		)

		now := time.Now().UTC()
		event := models.FlightEvent{
			Flight: models.Flight{
				Number:      req.Number,
				Origin:      req.Origin,
				Destination: req.Destination,
				Status:      models.FlightLanded,
				UpdatedAt:   now,
			},
			CorrelationID: uuid.NewString(),
			OccurredAt:    now,
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := client.Publish(publishCtx, messaging.SubjectFlightLanded, event); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error("failed to publish flight landed event", "error", err, "flight", req.Number)
			http.Error(w, "failed to publish event", http.StatusInternalServerError)
			return
		}

		svc.RecordPublish(messaging.SubjectFlightLanded)
		logger.Info("published flight landed event",
			"flight", req.Number,
			"correlation_id", event.CorrelationID,
		)
		w.WriteHeader(http.StatusAccepted)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
