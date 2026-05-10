package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
)

var upgrader = websocket.Upgrader{}
var clientHub = websocketClientHub{
	clients: make(map[*websocket.Conn]bool),
}

func healthHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
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
	if err := incident.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid incident payload: %v", err), http.StatusBadRequest)
		return
	}

	fmt.Printf("alert received: %s (%s)\n", incident.ID, incident.Type)

	if err := broadcastIncident(incident); err != nil {
		http.Error(w, fmt.Sprintf("failed to broadcast incident: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":      "accepted",
		"incident_id": incident.ID,
	}); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
}

func handshakeHandler(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)

	if err != nil {
		fmt.Println(err)
		return
	}

	clientHub.add(conn)
	defer clientHub.remove(conn)

	fmt.Println("Client connected")
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			fmt.Printf("client disconnected: %v\n", err)
			return
		}
	}
}

type websocketClientHub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

func (hub *websocketClientHub) add(conn *websocket.Conn) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	hub.clients[conn] = true
}

func (hub *websocketClientHub) remove(conn *websocket.Conn) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	delete(hub.clients, conn)
	conn.Close()
}

func broadcastIncident(incident Incident) error {
	message, err := json.Marshal(incident)
	if err != nil {
		return err
	}

	return clientHub.broadcast(message)
}

func (hub *websocketClientHub) broadcast(message []byte) error {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	var writeErrors []error
	for client := range hub.clients {
		err := client.WriteMessage(
			websocket.TextMessage,
			message,
		)

		if nil != err {
			writeErrors = append(writeErrors, err)
			client.Close()
			delete(hub.clients, client)
		}
	}

	return errors.Join(writeErrors...)
}
