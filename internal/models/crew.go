package models

import "time"

// CrewRole classifies the type of crew member assigned.
type CrewRole string

const (
	CrewRoleCabin     CrewRole = "CABIN"
	CrewRoleCaptain   CrewRole = "CAPTAIN"
	CrewRoleGroundOps CrewRole = "GROUND_OPS"
)

// CrewMember represents a single crew assignment.
type CrewMember struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Role CrewRole `json:"role"`
}

// CrewAssignment links a crew to an aircraft/flight.
type CrewAssignment struct {
	FlightNumber string       `json:"flight_number"`
	Crew         []CrewMember `json:"crew"`
	AssignedAt   time.Time    `json:"assigned_at"`
}

// CrewAssignedEvent is published once a crew has been dispatched to a flight.
type CrewAssignedEvent struct {
	Assignment    CrewAssignment `json:"assignment"`
	CorrelationID string         `json:"correlation_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
}
