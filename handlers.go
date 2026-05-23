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
	fmt.Printf(
		"sse request received: method=%s path=%q remote_addr=%q user_agent=%q\n",
		req.Method,
		req.URL.Path,
		requestClientAddress(req),
		req.UserAgent(),
	)

	if req.Method != http.MethodGet {
		fmt.Printf("sse request rejected: method=%s reason=%q remote_addr=%q\n", req.Method, "method not allowed", requestClientAddress(req))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		fmt.Printf("sse request rejected: reason=%q remote_addr=%q\n", "streaming unsupported", requestClientAddress(req))
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := sseHub.add(req)
	removeReason := "handler returned"
	defer func() {
		sseHub.remove(client, removeReason)
	}()

	if _, err := fmt.Fprintf(w, "event: ready\ndata: {}\n\n"); err != nil {
		removeReason = "ready write failed"
		fmt.Printf("sse ready write failed: client_id=%d error=%q\n", client.id, err)
		return
	}
	flusher.Flush()
	fmt.Printf("sse ready sent: client_id=%d remote_addr=%q\n", client.id, client.remoteAddr)

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-req.Context().Done():
			removeReason = req.Context().Err().Error()
			fmt.Printf("sse client context done: client_id=%d reason=%q remote_addr=%q\n", client.id, removeReason, client.remoteAddr)
			return
		case message, ok := <-client.send:
			if !ok {
				removeReason = "send channel closed"
				fmt.Printf("sse send channel closed: client_id=%d remote_addr=%q\n", client.id, client.remoteAddr)
				return
			}
			if _, err := fmt.Fprintf(w, "event: incident\ndata: %s\n\n", message); err != nil {
				removeReason = "incident write failed"
				fmt.Printf(
					"sse incident write failed: client_id=%d payload_bytes=%d error=%q remote_addr=%q\n",
					client.id,
					len(message),
					err,
					client.remoteAddr,
				)
				return
			}
			flusher.Flush()
			fmt.Printf("sse incident sent: client_id=%d payload_bytes=%d remote_addr=%q\n", client.id, len(message), client.remoteAddr)
		case <-keepalive.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				removeReason = "keepalive write failed"
				fmt.Printf("sse keepalive write failed: client_id=%d error=%q remote_addr=%q\n", client.id, err, client.remoteAddr)
				return
			}
			flusher.Flush()
			fmt.Printf("sse keepalive sent: client_id=%d remote_addr=%q\n", client.id, client.remoteAddr)
		}
	}
}

func requestClientAddress(req *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"} {
		if value := req.Header.Get(header); value != "" {
			return value
		}
	}
	return req.RemoteAddr
}

type sseClient struct {
	id          int64
	send        chan []byte
	remoteAddr  string
	userAgent   string
	connectedAt time.Time
}

type sseClientHub struct {
	mu      sync.Mutex
	nextID  int64
	clients map[*sseClient]bool
}

func (hub *sseClientHub) add(req *http.Request) *sseClient {
	now := time.Now()

	hub.mu.Lock()
	defer hub.mu.Unlock()

	hub.nextID++
	client := &sseClient{
		id:          hub.nextID,
		send:        make(chan []byte, clientSendBuffer),
		remoteAddr:  requestClientAddress(req),
		userAgent:   req.UserAgent(),
		connectedAt: now,
	}

	hub.clients[client] = true
	fmt.Printf(
		"sse client registered: id=%d active_clients=%d remote_addr=%q user_agent=%q\n",
		client.id,
		len(hub.clients),
		client.remoteAddr,
		client.userAgent,
	)
	return client
}

func (hub *sseClientHub) remove(client *sseClient, reason string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	if _, ok := hub.clients[client]; !ok {
		fmt.Printf("sse client already removed: id=%d reason=%q\n", client.id, reason)
		return
	}

	delete(hub.clients, client)
	close(client.send)
	fmt.Printf(
		"sse client removed: id=%d reason=%q active_clients=%d connected_for=%s remote_addr=%q\n",
		client.id,
		reason,
		len(hub.clients),
		time.Since(client.connectedAt).Round(time.Millisecond),
		client.remoteAddr,
	)
}

func (hub *sseClientHub) activeCount() int {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	return len(hub.clients)
}

func broadcastIncidentSSE(incident Incident) error {
	message, err := json.Marshal(incident)
	if err != nil {
		return err
	}

	fmt.Printf("sse incident broadcast requested: incident_id=%q type=%q payload_bytes=%d\n", incident.ID, incident.Type, len(message))
	return sseHub.broadcast(incident.ID, message)
}

func (hub *sseClientHub) broadcast(incidentID string, message []byte) error {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	var writeErrors []error
	activeClients := len(hub.clients)
	queued := 0
	dropped := 0
	fmt.Printf("sse broadcast fanout started: incident_id=%q active_clients=%d\n", incidentID, activeClients)
	for client := range hub.clients {
		select {
		case client.send <- message:
			queued++
			fmt.Printf(
				"sse broadcast queued: incident_id=%q client_id=%d queue_depth=%d queue_capacity=%d remote_addr=%q\n",
				incidentID,
				client.id,
				len(client.send),
				cap(client.send),
				client.remoteAddr,
			)
		default:
			dropped++
			writeErrors = append(writeErrors, fmt.Errorf("sse client send buffer is full: client_id=%d", client.id))
			fmt.Printf(
				"sse client dropped: incident_id=%q client_id=%d reason=%q queue_depth=%d queue_capacity=%d remote_addr=%q\n",
				incidentID,
				client.id,
				"send buffer full",
				len(client.send),
				cap(client.send),
				client.remoteAddr,
			)
			delete(hub.clients, client)
			close(client.send)
		}
	}
	fmt.Printf(
		"sse broadcast fanout finished: incident_id=%q active_clients=%d queued=%d dropped=%d remaining_clients=%d\n",
		incidentID,
		activeClients,
		queued,
		dropped,
		len(hub.clients),
	)

	return errors.Join(writeErrors...)
}
