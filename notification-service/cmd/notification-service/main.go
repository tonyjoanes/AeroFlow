package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/server"
)

// notification-service is a fan-out subscriber that listens to every event on
// the AEROFLOW stream and logs it. In a real system this would dispatch emails,
// push notifications, or SMS via a downstream provider.

const durableName = "notification-service-all-events"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client, err := messaging.Connect(envOr("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx := context.Background()

	// Subscribe to every subject on the stream using the wildcard.
	consumeCtx, err := client.Consume(ctx, messaging.StreamName, durableName, "aeroflow.>", notifyHandler(logger))
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	addr := envOr("HTTP_ADDR", ":8086")
	logger.Info("notification-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, server.Mux()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// notifyHandler fans out every event to the notification channel. Currently
// it just logs the raw payload; extend this to call real providers.
func notifyHandler(logger *slog.Logger) messaging.Handler {
	return func(ctx context.Context, data []byte) error {
		// Extract correlation_id and any top-level fields for structured logging
		// without needing to know the exact event type.
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(data, &envelope); err != nil {
			logger.Warn("received unparseable event", "raw", string(data))
			return nil
		}

		correlationID := ""
		if raw, ok := envelope["correlation_id"]; ok {
			_ = json.Unmarshal(raw, &correlationID)
		}

		logger.Info("event received",
			"correlation_id", correlationID,
			"fields", keys(envelope),
		)
		return nil
	}
}

func keys(m map[string]json.RawMessage) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
