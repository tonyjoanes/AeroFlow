// simulator runs a closed-loop airport simulation: a fixed fleet of aircraft
// cycle endlessly through SCHEDULED → BOARDING → DEPARTED → IN_FLIGHT →
// LANDED → (turnaround) → SCHEDULED. Every state transition is published to
// NATS so the full downstream event chain fires on every landing.
//
// Usage:
//
//	simulator --flights 20 --speed 60
//	simulator --flights 10 --speed 3600   # 1 second per simulated hour
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tonyjoanes/aeroflow/internal/messaging"
	"github.com/tonyjoanes/aeroflow/internal/models"
)

// route describes a scheduled service between two airports.
type route struct {
	origin      string
	destination string
	minHours    float64 // minimum flight time in simulated hours
	maxHours    float64
}

// routes is the fixed timetable. All real airport pairs with realistic durations.
var routes = []route{
	{origin: "LHR", destination: "JFK", minHours: 7.0, maxHours: 8.0},
	{origin: "JFK", destination: "LHR", minHours: 6.5, maxHours: 7.5},
	{origin: "LHR", destination: "DXB", minHours: 6.5, maxHours: 7.5},
	{origin: "DXB", destination: "LHR", minHours: 7.0, maxHours: 8.0},
	{origin: "LHR", destination: "CDG", minHours: 1.25, maxHours: 1.75},
	{origin: "CDG", destination: "LHR", minHours: 1.25, maxHours: 1.75},
	{origin: "LHR", destination: "SIN", minHours: 12.5, maxHours: 13.5},
	{origin: "SIN", destination: "LHR", minHours: 13.0, maxHours: 14.0},
	{origin: "LHR", destination: "SYD", minHours: 21.0, maxHours: 23.0},
	{origin: "JFK", destination: "CDG", minHours: 7.0, maxHours: 8.0},
	{origin: "CDG", destination: "JFK", minHours: 8.0, maxHours: 9.0},
	{origin: "JFK", destination: "NRT", minHours: 13.0, maxHours: 14.0},
	{origin: "LHR", destination: "HKG", minHours: 11.5, maxHours: 13.0},
	{origin: "LHR", destination: "ORD", minHours: 8.5, maxHours: 9.5},
	{origin: "DXB", destination: "SIN", minHours: 7.0, maxHours: 8.0},
	{origin: "FRA", destination: "JFK", minHours: 8.5, maxHours: 9.5},
	{origin: "AMS", destination: "JFK", minHours: 7.5, maxHours: 8.5},
	{origin: "MAD", destination: "JFK", minHours: 8.0, maxHours: 9.0},
	{origin: "LHR", destination: "YYZ", minHours: 8.0, maxHours: 9.0},
	{origin: "DOH", destination: "LHR", minHours: 6.5, maxHours: 7.5},
}

// aircraft is a registration paired with a friendly airline prefix so flight
// numbers look like real callsigns.
type aircraft struct {
	registration string
	prefix       string // e.g. "BA", "EK"
	number       int    // base flight number; incremented on each cycle
}

// fleet is the fixed set of aircraft in the simulation.
var fleet = []aircraft{
	{registration: "G-BOAC", prefix: "BA", number: 100},
	{registration: "G-CIVB", prefix: "BA", number: 200},
	{registration: "G-VIIA", prefix: "BA", number: 300},
	{registration: "N172UA", prefix: "UA", number: 900},
	{registration: "N77066", prefix: "UA", number: 910},
	{registration: "A6-ENA", prefix: "EK", number: 201},
	{registration: "A6-ENB", prefix: "EK", number: 202},
	{registration: "F-GZNA", prefix: "AF", number: 7},
	{registration: "F-GZNB", prefix: "AF", number: 8},
	{registration: "D-AIMA", prefix: "LH", number: 400},
	{registration: "D-AIMB", prefix: "LH", number: 401},
	{registration: "PH-BFA", prefix: "KL", number: 641},
	{registration: "PH-BFB", prefix: "KL", number: 642},
	{registration: "9V-SKA", prefix: "SQ", number: 317},
	{registration: "9V-SKB", prefix: "SQ", number: 318},
	{registration: "B-HNK", prefix: "CX", number: 250},
	{registration: "TC-JJA", prefix: "TK", number: 1},
	{registration: "C-FITU", prefix: "AC", number: 849},
	{registration: "A7-BAA", prefix: "QR", number: 7},
	{registration: "VH-OEA", prefix: "QF", number: 1},
}

// flightState holds the mutable state of one aircraft's current cycle.
type flightState struct {
	mu           sync.Mutex
	ac           aircraft
	route        route
	flightNumber string
	status       models.FlightStatus
	correlID     string
	cycleCount   int
}

func (fs *flightState) nextFlightNumber() string {
	fs.cycleCount++
	return fmt.Sprintf("%s%d", fs.ac.prefix, fs.ac.number+fs.cycleCount)
}

func main() {
	natsURL := flag.String("nats", envOr("NATS_URL", "nats://localhost:4222"), "NATS URL")
	speed := flag.Float64("speed", 60, "simulation speed multiplier (60 = 1 real minute per simulated hour)")
	flightCount := flag.Int("flights", len(fleet), "number of aircraft to simulate (max "+fmt.Sprintf("%d", len(fleet))+")")
	flag.Parse()

	if *flightCount > len(fleet) {
		*flightCount = len(fleet)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client, err := messaging.Connect(*natsURL)
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	logger.Info("simulator starting",
		"aircraft", *flightCount,
		"speed", *speed,
		"nats", *natsURL,
	)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range *flightCount {
		ac := fleet[i]
		// Stagger initial start times so not all aircraft land simultaneously.
		initialDelay := time.Duration(r.Float64()*60) * time.Second

		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(initialDelay)
			runAircraft(ctx, logger, client, ac, *speed, r)
		}()
	}

	wg.Wait()
}

// runAircraft drives one aircraft through the state machine forever.
func runAircraft(ctx context.Context, logger *slog.Logger, client *messaging.Client, ac aircraft, speed float64, r *rand.Rand) {
	fs := &flightState{ac: ac}

	for {
		// Pick a random route for this cycle.
		fs.route = routes[r.Intn(len(routes))]
		fs.correlID = uuid.NewString()
		fs.flightNumber = fs.nextFlightNumber()

		transitions := []struct {
			status    models.FlightStatus
			subject   string
			waitHours float64 // simulated hours to wait in this state
		}{
			{models.FlightScheduled, messaging.SubjectFlightScheduled, 0.5},
			{models.FlightBoarding, messaging.SubjectFlightBoarding, 0.5},
			{models.FlightDeparted, messaging.SubjectFlightDeparted, 0.1},
			{models.FlightLanded, messaging.SubjectFlightLanded, // IN_FLIGHT duration is the flight time
				fs.route.minHours + r.Float64()*(fs.route.maxHours-fs.route.minHours)},
		}

		// IN_FLIGHT is a special intermediate state — publish it, then wait
		// for the flight duration before publishing LANDED.
		if err := publishEvent(ctx, client, messaging.SubjectFlightInFlight, models.FlightEvent{
			Flight: models.Flight{
				Number:      fs.flightNumber,
				Origin:      fs.route.origin,
				Destination: fs.route.destination,
				Status:      models.FlightDeparted,
				UpdatedAt:   time.Now().UTC(),
			},
			CorrelationID: fs.correlID,
			OccurredAt:    time.Now().UTC(),
		}); err != nil {
			logger.Warn("publish failed", "error", err)
		}

		for _, t := range transitions {
			wait := hoursToReal(t.waitHours, speed)

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}

			evt := models.FlightEvent{
				Flight: models.Flight{
					Number:      fs.flightNumber,
					Origin:      fs.route.origin,
					Destination: fs.route.destination,
					Status:      t.status,
					UpdatedAt:   time.Now().UTC(),
				},
				CorrelationID: fs.correlID,
				OccurredAt:    time.Now().UTC(),
			}

			if err := publishEvent(ctx, client, t.subject, evt); err != nil {
				logger.Warn("publish failed", "subject", t.subject, "error", err)
				continue
			}

			logger.Info("transition",
				"flight", fs.flightNumber,
				"route", fs.route.origin+"→"+fs.route.destination,
				"status", t.status,
				"correlation_id", fs.correlID,
			)
		}

		// Turnaround: swap origin/destination for the return leg, wait a bit.
		fs.route.origin, fs.route.destination = fs.route.destination, fs.route.origin
		select {
		case <-ctx.Done():
			return
		case <-time.After(hoursToReal(0.75, speed)): // ~45 min turnaround
		}
	}
}

func publishEvent(ctx context.Context, client *messaging.Client, subject string, evt models.FlightEvent) error {
	pCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.Publish(pCtx, subject, evt)
}

// hoursToReal converts simulated hours to a real-time duration using the
// speed multiplier (speed=60 → 1 real minute per simulated hour).
func hoursToReal(hours, speed float64) time.Duration {
	realSeconds := (hours * 3600) / speed
	return time.Duration(realSeconds * float64(time.Second))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
