package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	routes := map[string]http.HandlerFunc{
		"/":          rootHandler,
		"/healthz":   healthHandler,
		"/alert":     alertHandler,
		"/handshake": handshakeHandler,
	}
	for route, handler := range routes {
		http.HandleFunc(route, handler)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	addr := "0.0.0.0:" + port
	fmt.Printf("orchestrator listening on http://%s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Printf("server stopped: %v\n", err)
	}
}
