package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const clientSendBuffer = 16

var upgrader = websocket.Upgrader{}
var clientHub = websocketClientHub{
	clients: make(map[*websocketClient]bool),
}
var sseHub = sseClientHub{
	clients: make(map[*sseClient]bool),
}

func rootHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}

	http.ServeFile(w, req, "index.html")
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
	// Make sure method is post
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// JSON -> Incident
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":      "accepted",
		"incident_id": incident.ID,
	}); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}

	go func() {
		if err := broadcastIncident(incident); err != nil {
			fmt.Printf("failed to broadcast incident: %v\n", err)
		}
	}()

	go func() {
		if err := broadcastIncidentSSE(incident); err != nil {
			fmt.Printf("failed to broadcast incident over SSE: %v\n", err)
		}
	}()

	go func() {
		sendToDatabase(incident)
	}()
}

func handshakeHandler(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)

	if err != nil {
		fmt.Println(err)
		return
	}

	client := clientHub.add(conn)
	defer clientHub.remove(client)
	go client.writePump()

	fmt.Println("Client connected")
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			fmt.Printf("client disconnected: %v\n", err)
			return
		}
	}
}

type websocketClient struct {
	conn *websocket.Conn
	send chan []byte
}

type websocketClientHub struct {
	mu      sync.Mutex
	clients map[*websocketClient]bool
}

func (hub *websocketClientHub) add(conn *websocket.Conn) *websocketClient {
	client := &websocketClient{
		conn: conn,
		send: make(chan []byte, clientSendBuffer),
	}

	hub.mu.Lock()
	defer hub.mu.Unlock()

	hub.clients[client] = true
	return client
}

func (hub *websocketClientHub) remove(client *websocketClient) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	if _, ok := hub.clients[client]; !ok {
		return
	}

	delete(hub.clients, client)
	close(client.send)
	client.conn.Close()
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
		select {
		case client.send <- message:
		default:
			writeErrors = append(writeErrors, errors.New("websocket client send buffer is full"))
			delete(hub.clients, client)
			close(client.send)
			client.conn.Close()
		}
	}

	return errors.Join(writeErrors...)
}

func (client *websocketClient) writePump() {
	defer clientHub.remove(client)

	for message := range client.send {
		if err := client.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			fmt.Printf("failed to set websocket write deadline: %v\n", err)
			return
		}
		if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			fmt.Printf("failed to write websocket message: %v\n", err)
			return
		}
	}
}

func sseHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := sseHub.add()
	defer sseHub.remove(client)

	fmt.Fprintf(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-req.Context().Done():
			return
		case message, ok := <-client.send:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: incident\ndata: %s\n\n", message)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

type sseClient struct {
	send chan []byte
}

type sseClientHub struct {
	mu      sync.Mutex
	clients map[*sseClient]bool
}

func (hub *sseClientHub) add() *sseClient {
	client := &sseClient{
		send: make(chan []byte, clientSendBuffer),
	}

	hub.mu.Lock()
	defer hub.mu.Unlock()

	hub.clients[client] = true
	return client
}

func (hub *sseClientHub) remove(client *sseClient) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	if _, ok := hub.clients[client]; !ok {
		return
	}

	delete(hub.clients, client)
	close(client.send)
}

func broadcastIncidentSSE(incident Incident) error {
	message, err := json.Marshal(incident)
	if err != nil {
		return err
	}

	return sseHub.broadcast(message)
}

func (hub *sseClientHub) broadcast(message []byte) error {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	var writeErrors []error
	for client := range hub.clients {
		select {
		case client.send <- message:
		default:
			writeErrors = append(writeErrors, errors.New("sse client send buffer is full"))
			delete(hub.clients, client)
			close(client.send)
		}
	}

	return errors.Join(writeErrors...)
}
