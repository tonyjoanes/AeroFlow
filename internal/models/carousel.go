package models

import "time"

// Carousel represents a baggage reclaim belt in a terminal.
type Carousel struct {
	ID       string `json:"id"`
	Terminal string `json:"terminal"`
}

// CarouselAssignment links a baggage job to a carousel.
type CarouselAssignment struct {
	JobID        string    `json:"job_id"`
	FlightNumber string    `json:"flight_number"`
	Carousel     Carousel  `json:"carousel"`
	AssignedAt   time.Time `json:"assigned_at"`
}

// CarouselAssignedEvent is published once a carousel is assigned to a flight.
type CarouselAssignedEvent struct {
	Assignment    CarouselAssignment `json:"assignment"`
	CorrelationID string             `json:"correlation_id"`
	OccurredAt    time.Time          `json:"occurred_at"`
}
