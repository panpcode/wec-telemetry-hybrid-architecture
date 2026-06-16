package main

import (
	"log"
	"net/http"
	"os"

	"github.com/nats-io/nats.go"
)

func main() {
	natsURL := getenv("NATS_URL", "nats://localhost:4222")
	httpAddr := getenv("HTTP_ADDR", ":3000")

	// Connect to NATS — dashboard only reads, it never publishes.
	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		log.Fatalf("NATS connect: %v", err)
	}
	defer nc.Close()
	log.Printf("Connected to NATS at %s", natsURL)

	// Hub manages WebSocket clients and fans out telemetry broadcasts.
	hub := newHub()
	go hub.run()             // ticker that broadcasts car state every 200ms
	go hub.subscribeNATS(nc) // NATS subscriber that updates car state map

	http.HandleFunc("/ws", hub.serveWS)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	// All other paths serve the single-page pit wall HTML.
	http.HandleFunc("/", serveDashboard)

	log.Printf("Pit wall dashboard at http://localhost%s", httpAddr)
	log.Fatal(http.ListenAndServe(httpAddr, nil))
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
