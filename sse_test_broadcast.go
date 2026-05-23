package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	sseTestBroadcastDuration = 10 * time.Second
	sseTestBroadcastInterval = 1 * time.Second
)

var sseTestBroadcastMu sync.Mutex
var sseTestBroadcastRunning bool

func sseTestBroadcastHandler(w http.ResponseWriter, req *http.Request) {
	if os.Getenv("SSE_TEST_BROADCAST_ENABLED") != "true" {
		http.NotFound(w, req)
		return
	}

	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	started := startSSETestBroadcast("manual")
	status := "started"
	if !started {
		status = "already_running"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":   status,
		"duration": sseTestBroadcastDuration.String(),
		"interval": sseTestBroadcastInterval.String(),
	}); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
}

func startSSETestBroadcast(source string) bool {
	sseTestBroadcastMu.Lock()
	if sseTestBroadcastRunning {
		sseTestBroadcastMu.Unlock()
		return false
	}
	sseTestBroadcastRunning = true
	sseTestBroadcastMu.Unlock()

	go runSSETestBroadcast(source)
	return true
}

func runSSETestBroadcast(source string) {
	defer func() {
		sseTestBroadcastMu.Lock()
		sseTestBroadcastRunning = false
		sseTestBroadcastMu.Unlock()
		fmt.Printf("SSE test broadcast stopped: source=%s\n", source)
	}()

	fmt.Printf("SSE test broadcast started: source=%s duration=%s interval=%s\n", source, sseTestBroadcastDuration, sseTestBroadcastInterval)

	ticker := time.NewTicker(sseTestBroadcastInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(sseTestBroadcastDuration)
	for sequence := 1; ; sequence++ {
		if !time.Now().Before(deadline) {
			return
		}

		now := time.Now().UTC()
		incident := testIncident(sequence, now)
		if err := broadcastIncidentSSE(incident); err != nil {
			fmt.Printf("failed to broadcast SSE test incident: %v\n", err)
		}

		nextBroadcast := time.Now().Add(sseTestBroadcastInterval)
		if !nextBroadcast.Before(deadline) {
			time.Sleep(time.Until(deadline))
			return
		}

		<-ticker.C
	}
}

func testIncident(sequence int, now time.Time) Incident {
	roadName := "SSE Test Route"
	direction := "northbound"
	mileMarker := "test"

	return Incident{
		ID:                 fmt.Sprintf("sse_test_%s_%02d", now.Format("20060102T150405Z"), sequence),
		Type:               IncidentTypeAnimalOnRoad,
		OccurredAt:         now,
		ReportedAt:         now,
		SpeakerFrequencyHz: 20000,
		Location: Location{
			Latitude:   37.7749,
			Longitude:  -122.4194,
			RoadName:   &roadName,
			Direction:  &direction,
			MileMarker: &mileMarker,
			CameraID:   "rpi-sse-test",
		},
		RecommendedAction: RecommendedAction{
			Priority: PriorityHigh,
			Message:  "SSE connectivity test incident.",
		},
		Evidence: Evidence{},
	}
}
