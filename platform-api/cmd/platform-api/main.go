package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/metrics"
	"github.com/tonyjoanes/aeroflow/internal/server"
	"github.com/tonyjoanes/aeroflow/internal/tracing"
	k8sclient "github.com/tonyjoanes/aeroflow/platform-api/internal/k8s"
)

//go:embed web/templates/*.html
var templateFS embed.FS

const serviceName = "platform-api"

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

	k8s, err := k8sclient.New()
	if err != nil {
		logger.Warn("kubernetes client unavailable — service catalogue will be empty", "error", err)
	}

	natsURL := envOr("NATS_URL", "nats://localhost:4222")
	nats, err := messaging.Connect(natsURL)
	if err != nil {
		logger.Warn("nats unavailable — live event feed will be empty", "error", err)
	}

	tmpl := template.Must(
		template.New("").Funcs(templateFuncs()).ParseFS(templateFS, "web/templates/*.html"),
	)

	store := newFlightStore()

	if nats != nil {
		go subscribeEvents(ctx, logger, nats, store)
	}

	mux := server.Mux()
	mux.HandleFunc("GET /", servicesPage(logger, k8s, tmpl))
	mux.HandleFunc("GET /flights", flightsPage(logger, store, tmpl))
	mux.HandleFunc("GET /events", eventsPage(tmpl))
	mux.HandleFunc("GET /events/stream", eventsStream(store))
	mux.HandleFunc("GET /api/services", apiServices(logger, k8s))
	mux.HandleFunc("GET /api/health", apiHealth(logger, k8s))
	mux.HandleFunc("GET /api/flights", apiFlights(store))
	mux.HandleFunc("POST /api/rollout", apiRollout(logger, k8s))

	addr := envOr("HTTP_ADDR", ":9000")
	logger.Info("platform-api listening", "addr", addr)
	if err := http.ListenAndServe(addr, svc.InstrumentMux(mux)); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// ── API handlers ─────────────────────────────────────────────────────────────

func apiServices(logger *slog.Logger, k8s *k8sclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if k8s == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "kubernetes unavailable"})
			return
		}
		summaries, err := k8s.ListServices(r.Context())
		if err != nil {
			logger.Error("list services", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, summaries)
	}
}

func apiHealth(logger *slog.Logger, k8s *k8sclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if k8s == nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "unknown", "reason": "kubernetes unavailable"})
			return
		}
		status, err := k8s.AggregateHealth(r.Context())
		if err != nil {
			logger.Error("aggregate health", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
	}
}

func apiFlights(store *flightStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.list())
	}
}

func apiRollout(logger *slog.Logger, k8s *k8sclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if k8s == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "kubernetes unavailable"})
			return
		}
		var req k8sclient.RolloutRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Namespace == "" || req.Deployment == "" || req.Image == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace, deployment, and image are required"})
			return
		}
		if err := k8s.PatchImage(r.Context(), req); err != nil {
			logger.Error("patch image", "error", err, "deployment", req.Deployment)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		logger.Info("rollout triggered", "namespace", req.Namespace, "deployment", req.Deployment, "image", req.Image)
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "rollout triggered"})
	}
}

// ── UI handlers ───────────────────────────────────────────────────────────────

func servicesPage(logger *slog.Logger, k8s *k8sclient.Client, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var summaries []k8sclient.ServiceSummary
		health := "unknown"

		if k8s != nil {
			var err error
			summaries, err = k8s.ListServices(r.Context())
			if err != nil {
				logger.Error("list services for UI", "error", err)
			}
			health, _ = k8s.AggregateHealth(r.Context())
		}

		total, ready, degraded := 0, 0, 0
		for _, s := range summaries {
			for _, d := range s.Deployments {
				total++
				if d.Available {
					ready++
				} else {
					degraded++
				}
			}
		}

		tmpl.ExecuteTemplate(w, "services.html", map[string]any{
			"Summaries":        summaries,
			"Health":           health,
			"TotalDeployments": total,
			"ReadyCount":       ready,
			"DegradedCount":    degraded,
		})
	}
}

func flightsPage(logger *slog.Logger, store *flightStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "flights.html", map[string]any{
			"Flights": store.list(),
		})
	}
}

func eventsPage(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "events.html", nil)
	}
}

// eventsStream pushes every new NATS event to the browser via Server-Sent Events.
func eventsStream(store *flightStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := store.subscribe()
		defer store.unsubscribe(ch)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case evt := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", evt)
				flusher.Flush()
			}
		}
	}
}

// ── Flight state store ────────────────────────────────────────────────────────

// flightRecord is an in-memory snapshot of a flight's progress through the chain.
type flightRecord struct {
	Number      string    `json:"number"`
	Origin      string    `json:"origin"`
	Destination string    `json:"destination"`
	Status      string    `json:"status"`
	Gate        string    `json:"gate"`
	Carousel    string    `json:"carousel"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type flightStore struct {
	mu          sync.RWMutex
	flights     map[string]*flightRecord
	subscribers map[chan []byte]struct{}
}

func newFlightStore() *flightStore {
	return &flightStore{
		flights:     make(map[string]*flightRecord),
		subscribers: make(map[chan []byte]struct{}),
	}
}

func (s *flightStore) list() []flightRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]flightRecord, 0, len(s.flights))
	for _, f := range s.flights {
		out = append(out, *f)
	}
	return out
}

func (s *flightStore) upsert(fn string, update func(*flightRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flights[fn]
	if !ok {
		f = &flightRecord{Number: fn}
		s.flights[fn] = f
	}
	f.UpdatedAt = time.Now().UTC()
	update(f)
}

func (s *flightStore) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *flightStore) unsubscribe(ch chan []byte) {
	s.mu.Lock()
	delete(s.subscribers, ch)
	s.mu.Unlock()
}

func (s *flightStore) broadcast(data []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}

// ── NATS subscription ─────────────────────────────────────────────────────────

func subscribeEvents(ctx context.Context, logger *slog.Logger, nats *messaging.Client, store *flightStore) {
	nats.Consume(ctx, messaging.StreamName, "platform-api-all-events", "aeroflow.>",
		func(ctx context.Context, data []byte) error {
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(data, &envelope); err != nil {
				return nil
			}

			// Update the in-memory flight board from whatever event arrived.
			updateFlightStore(store, envelope)

			// Broadcast raw bytes to SSE subscribers.
			store.broadcast(data)
			return nil
		},
	)
}

func updateFlightStore(store *flightStore, envelope map[string]json.RawMessage) {
	// Try to extract a flight number from wherever it appears in the event.
	fn := extractString(envelope, "flight", "number")
	if fn == "" {
		fn = extractString(envelope, "assignment", "flight_number")
	}
	if fn == "" {
		fn = extractString(envelope, "job", "flight_number")
	}
	if fn == "" {
		return
	}

	store.upsert(fn, func(f *flightRecord) {
		if origin := extractString(envelope, "flight", "origin"); origin != "" {
			f.Origin = origin
		}
		if dest := extractString(envelope, "flight", "destination"); dest != "" {
			f.Destination = dest
		}
		if status := extractString(envelope, "flight", "status"); status != "" {
			f.Status = status
		}
		if gate := extractString(envelope, "assignment", "gate", "id"); gate != "" {
			f.Gate = gate
		}
		if carousel := extractString(envelope, "assignment", "carousel", "id"); carousel != "" {
			f.Carousel = carousel
		}
	})
}

// extractString navigates a nested JSON envelope by key path.
func extractString(envelope map[string]json.RawMessage, keys ...string) string {
	current := envelope
	for i, k := range keys {
		raw, ok := current[k]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return ""
			}
			return s
		}
		var next map[string]json.RawMessage
		if err := json.Unmarshal(raw, &next); err != nil {
			return ""
		}
		current = next
	}
	return ""
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"allAvailable": func(deps []k8sclient.DeploymentInfo) bool {
			for _, d := range deps {
				if !d.Available {
					return false
				}
			}
			return true
		},
		"statusBadge": func(status string) string {
			switch status {
			case "LANDED":
				return "badge-green"
			case "BOARDING":
				return "badge-yellow"
			default:
				return "badge-yellow"
			}
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
