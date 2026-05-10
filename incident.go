package main

import (
	"errors"
	"fmt"
	"time"
)

type IncidentType string

const (
	IncidentTypeAnimalOnRoad    IncidentType = "animal_on_road"
	IncidentTypePersonOnRoad    IncidentType = "person_on_road"
	IncidentTypeStoppedVehicle  IncidentType = "stopped_vehicle"
	IncidentTypeRoadObstruction IncidentType = "road_obstruction"
	IncidentTypeUnknown         IncidentType = "unknown"
)

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

type Incident struct {
	ID                string            `json:"incident_id"`
	Type              IncidentType      `json:"type"`
	OccurredAt        time.Time         `json:"occurred_at"`
	ReportedAt        time.Time         `json:"reported_at"`
	Location          Location          `json:"location"`
	RecommendedAction RecommendedAction `json:"recommended_action"`
	Evidence          Evidence          `json:"evidence"`
}

type Location struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	RoadName   *string `json:"road_name"`
	Direction  *string `json:"direction"`
	MileMarker *string `json:"mile_marker"`
	CameraID   string  `json:"camera_id"`
}

type RecommendedAction struct {
	Priority Priority `json:"priority"`
	Message  string   `json:"message"`
}

type Evidence struct {
	SnapshotURL  *string `json:"snapshot_url"`
	VideoClipURL *string `json:"video_clip_url"`
}

func (incident Incident) Validate() error {
	if incident.ID == "" {
		return errors.New("incident_id is required")
	}
	if !incident.Type.Valid() {
		return fmt.Errorf("type must be one of: %s, %s, %s, %s, %s",
			IncidentTypeAnimalOnRoad,
			IncidentTypePersonOnRoad,
			IncidentTypeStoppedVehicle,
			IncidentTypeRoadObstruction,
			IncidentTypeUnknown,
		)
	}
	if incident.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if incident.ReportedAt.IsZero() {
		return errors.New("reported_at is required")
	}
	if incident.Location.CameraID == "" {
		return errors.New("location.camera_id is required")
	}
	if !incident.RecommendedAction.Priority.Valid() {
		return fmt.Errorf("recommended_action.priority must be one of: %s, %s, %s, %s",
			PriorityLow,
			PriorityMedium,
			PriorityHigh,
			PriorityCritical,
		)
	}
	if incident.RecommendedAction.Message == "" {
		return errors.New("recommended_action.message is required")
	}

	return nil
}

func (incidentType IncidentType) Valid() bool {
	switch incidentType {
	case IncidentTypeAnimalOnRoad,
		IncidentTypePersonOnRoad,
		IncidentTypeStoppedVehicle,
		IncidentTypeRoadObstruction,
		IncidentTypeUnknown:
		return true
	default:
		return false
	}
}

func (priority Priority) Valid() bool {
	switch priority {
	case PriorityLow,
		PriorityMedium,
		PriorityHigh,
		PriorityCritical:
		return true
	default:
		return false
	}
}
