package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel/attribute"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/metrics"
	"github.com/tonyjoanes/aeroflow/internal/server"
	"github.com/tonyjoanes/aeroflow/internal/tracing"
)

// notification-service is a fan-out subscriber that listens to every event on
// the AEROFLOW stream and logs it. In a real system this would dispatch emails,
// push notifications, or SMS via a downstream provider.

const (
	serviceName = "notification-service"
	durableName = "notification-service-all-events"
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

	consumeCtx, err := client.Consume(ctx, messaging.StreamName, durableName, "aeroflow.>", notifyHandler(logger, svc))
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	addr := envOr("HTTP_ADDR", ":8086")
	logger.Info("notification-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, svc.InstrumentMux(server.Mux())); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func notifyHandler(logger *slog.Logger, svc *metrics.ServiceMetrics) messaging.Handler {
	tracer := tracing.Tracer(serviceName)

	return func(ctx context.Context, data []byte) error {
		ctx, span := tracer.Start(ctx, "notification.fan_out")
		defer span.End()
		done := svc.ObserveConsume("aeroflow.>")

		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(data, &envelope); err != nil {
			logger.Warn("received unparseable event", "raw", string(data))
			done(nil)
			return nil
		}

		correlationID := ""
		if raw, ok := envelope["correlation_id"]; ok {
			_ = json.Unmarshal(raw, &correlationID)
		}

		span.SetAttributes(attribute.String("correlation.id", correlationID))
		logger.Info("event received",
			"correlation_id", correlationID,
			"fields", keys(envelope),
		)
		done(nil)
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
