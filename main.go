package main

import (
	"fmt"
	"net/http"
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
