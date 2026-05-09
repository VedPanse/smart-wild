package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/healthz", healthHandler)
	http.HandleFunc("/alert", alertHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	addr := "0.0.0.0:" + port
	fmt.Printf("orchestrator listening on http://%s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Printf("server stopped: %v\n", err)
	}

	// TODO Push update to client app via websocket (along with details)
	// TODO Push incident data to storage (database)
	// TODO Notify client website (it will pull itself)
}
