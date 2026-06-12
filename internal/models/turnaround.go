package models

import "time"

// TurnaroundStatus tracks ground ops for an aircraft between landing and
// next departure.
type TurnaroundStatus string

const (
	TurnaroundStarted  TurnaroundStatus = "STARTED"
	TurnaroundComplete TurnaroundStatus = "COMPLETE"
)

// Turnaround coordinates all ground operations for a landed aircraft.
type Turnaround struct {
	ID           string           `json:"id"`
	FlightNumber string           `json:"flight_number"`
	Status       TurnaroundStatus `json:"status"`
	StartedAt    time.Time        `json:"started_at"`
}

// TurnaroundStartedEvent is published when ground ops coordination begins.
type TurnaroundStartedEvent struct {
	Turnaround    Turnaround `json:"turnaround"`
	CorrelationID string     `json:"correlation_id"`
	OccurredAt    time.Time  `json:"occurred_at"`
}
