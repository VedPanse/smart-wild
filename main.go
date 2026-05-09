package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func main() {
	http.HandleFunc("/alert", alertHandler)

	fmt.Println("orchestrator listening on http://localhost:8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		fmt.Printf("server stopped: %v\n", err)
	}

	// TODO Push update to client app via websocket (along with details)
	// TODO Push incident data to storage (database)
	// TODO Notify client website (it will pull itself)
}

func alertHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var incident Incident
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&incident); err != nil {
		http.Error(w, fmt.Sprintf("invalid incident payload: %v", err), http.StatusBadRequest)
		return
	}

	fmt.Printf("alert received: %s (%s)\n", incident.ID, incident.Type)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":      "accepted",
		"incident_id": incident.ID,
	}); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
}

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
