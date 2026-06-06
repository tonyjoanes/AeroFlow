package models

import "time"

// FlightStatus represents the lifecycle state of a flight.
type FlightStatus string

const (
	FlightScheduled FlightStatus = "SCHEDULED"
	FlightBoarding  FlightStatus = "BOARDING"
	FlightDeparted  FlightStatus = "DEPARTED"
	FlightLanded    FlightStatus = "LANDED"
)

// Flight is the core domain entity tracked through the event chain.
type Flight struct {
	Number      string       `json:"number"`
	Origin      string       `json:"origin"`
	Destination string       `json:"destination"`
	Status      FlightStatus `json:"status"`
	ScheduledAt time.Time    `json:"scheduled_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// FlightEvent is published to NATS whenever a flight changes state.
type FlightEvent struct {
	Flight        Flight    `json:"flight"`
	CorrelationID string    `json:"correlation_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}
