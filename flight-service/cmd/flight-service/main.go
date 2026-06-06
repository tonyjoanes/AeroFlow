package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/models"
	"github.com/tonyjoanes/aeroflow/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	natsURL := envOr("NATS_URL", "nats://localhost:4222")

	client, err := messaging.Connect(natsURL)
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	mux := server.Mux()
	mux.HandleFunc("POST /flights/land", landHandler(logger, client))

	addr := envOr("HTTP_ADDR", ":8080")
	logger.Info("flight-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

type landRequest struct {
	Number      string `json:"number"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

// landHandler accepts a flight landing notification and publishes a
// FlightEvent with status LANDED, kicking off the downstream event chain.
func landHandler(logger *slog.Logger, client *messaging.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req landRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Number == "" {
			http.Error(w, "number is required", http.StatusBadRequest)
			return
		}

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

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := client.Publish(ctx, messaging.SubjectFlightLanded, event); err != nil {
			logger.Error("failed to publish flight landed event", "error", err, "flight", req.Number)
			http.Error(w, "failed to publish event", http.StatusInternalServerError)
			return
		}

		logger.Info("published flight landed event", "flight", req.Number, "correlation_id", event.CorrelationID)
		w.WriteHeader(http.StatusAccepted)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
