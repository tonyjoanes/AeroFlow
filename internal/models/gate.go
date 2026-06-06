package models

import "time"

// Gate represents a physical gate that flights are assigned to.
type Gate struct {
	ID       string `json:"id"`
	Terminal string `json:"terminal"`
}

// GateAssignment links a flight to a gate at a point in time.
type GateAssignment struct {
	FlightNumber string    `json:"flight_number"`
	Gate         Gate      `json:"gate"`
	AssignedAt   time.Time `json:"assigned_at"`
}

// GateAssignedEvent is published to NATS once a flight has a gate.
type GateAssignedEvent struct {
	Assignment    GateAssignment `json:"assignment"`
	CorrelationID string         `json:"correlation_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
}
