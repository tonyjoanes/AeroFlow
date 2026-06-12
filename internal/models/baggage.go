package models

import "time"

// BaggageStatus tracks a baggage job through its lifecycle.
type BaggageStatus string

const (
	BaggageStarted    BaggageStatus = "STARTED"
	BaggageOnCarousel BaggageStatus = "ON_CAROUSEL"
	BaggageCollected  BaggageStatus = "COLLECTED"
)

// BaggageJob is created for each flight that lands and tracks all bags
// through to the carousel.
type BaggageJob struct {
	ID           string        `json:"id"`
	FlightNumber string        `json:"flight_number"`
	Status       BaggageStatus `json:"status"`
	CreatedAt    time.Time     `json:"created_at"`
}

// BaggageStartedEvent is published when baggage handling begins for a flight.
type BaggageStartedEvent struct {
	Job           BaggageJob `json:"job"`
	CorrelationID string     `json:"correlation_id"`
	OccurredAt    time.Time  `json:"occurred_at"`
}
