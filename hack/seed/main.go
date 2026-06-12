// seed generates a realistic flight schedule and POSTs landing events to
// flight-service. Use --burst to fire all flights as fast as possible for
// stress testing and making Grafana dashboards interesting.
//
// Usage:
//
//	seed --addr http://localhost:8080 --flights 20
//	seed --addr http://localhost:8080 --flights 50 --burst
//	seed --addr http://localhost:8080 --flights 10 --interval 2s
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"
)

// schedule is a curated set of real-world-ish flight numbers, origins and
// destinations that make logs and dashboards feel authentic.
var schedule = []landRequest{
	{Number: "BA442", Origin: "LHR", Destination: "JFK"},
	{Number: "EK201", Origin: "DXB", Destination: "LHR"},
	{Number: "QF001", Origin: "SYD", Destination: "DXB"},
	{Number: "AA100", Origin: "JFK", Destination: "LHR"},
	{Number: "LH400", Origin: "FRA", Destination: "JFK"},
	{Number: "AF007", Origin: "CDG", Destination: "JFK"},
	{Number: "SQ317", Origin: "SIN", Destination: "LHR"},
	{Number: "CX250", Origin: "HKG", Destination: "LHR"},
	{Number: "TK001", Origin: "IST", Destination: "JFK"},
	{Number: "UA901", Origin: "ORD", Destination: "LHR"},
	{Number: "DL401", Origin: "ATL", Destination: "CDG"},
	{Number: "NH211", Origin: "NRT", Destination: "LHR"},
	{Number: "IB6251", Origin: "MAD", Destination: "JFK"},
	{Number: "VS003", Origin: "LHR", Destination: "JFK"},
	{Number: "KL641", Origin: "AMS", Destination: "JFK"},
	{Number: "MH001", Origin: "KUL", Destination: "LHR"},
	{Number: "ET500", Origin: "ADD", Destination: "LHR"},
	{Number: "AC849", Origin: "YYZ", Destination: "LHR"},
	{Number: "QR007", Origin: "DOH", Destination: "LHR"},
	{Number: "EY019", Origin: "AUH", Destination: "LHR"},
}

type landRequest struct {
	Number      string `json:"number"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

func main() {
	addr := flag.String("addr", "http://localhost:8080", "flight-service base URL")
	count := flag.Int("flights", 10, "number of flights to land")
	burst := flag.Bool("burst", false, "fire all flights immediately with no delay")
	interval := flag.Duration("interval", 500*time.Millisecond, "delay between flights in normal mode")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := &http.Client{Timeout: 10 * time.Second}

	flights := buildFlightList(*count)

	logger.Info("starting seed",
		"addr", *addr,
		"flights", *count,
		"burst", *burst,
		"interval", interval.String(),
	)

	start := time.Now()
	ok, failed := 0, 0

	for i, f := range flights {
		if err := land(client, *addr, f); err != nil {
			logger.Error("failed to land flight", "flight", f.Number, "error", err)
			failed++
		} else {
			logger.Info("landed", "flight", f.Number, "origin", f.Origin, "destination", f.Destination)
			ok++
		}

		if !*burst && i < len(flights)-1 {
			time.Sleep(*interval)
		}
	}

	logger.Info("seed complete",
		"ok", ok,
		"failed", failed,
		"elapsed", time.Since(start).Round(time.Millisecond).String(),
	)

	if failed > 0 {
		os.Exit(1)
	}
}

// buildFlightList picks count flights from the schedule, cycling and
// randomising order so runs look different each time.
func buildFlightList(count int) []landRequest {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	perm := r.Perm(len(schedule))

	flights := make([]landRequest, 0, count)
	for i := range count {
		flights = append(flights, schedule[perm[i%len(schedule)]])
	}
	return flights
}

func land(client *http.Client, addr string, f landRequest) error {
	body, err := json.Marshal(f)
	if err != nil {
		return err
	}

	resp, err := client.Post(addr+"/flights/land", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
