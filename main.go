package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	esClient, err := NewESClient()
	if err != nil {
		log.Fatal("Failed to create Elasticsearch client:", err)
	}

	http.HandleFunc("/ws/monitor", func(w http.ResponseWriter, r *http.Request) {
		monitorWSHandler(w, r, esClient)
	})

	http.HandleFunc("/ws/track", func(w http.ResponseWriter, r *http.Request) {
		trackingWSHandler(w, r, esClient)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":        "ok",
			"elasticsearch": "connected",
		})
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "Coding Tracker Server",
			"endpoints": map[string]string{
				"monitor": "ws://localhost:8081/ws/monitor",
				"track":   "ws://localhost:8081/ws/track",
				"health":  "http://localhost:8081/health",
			},
			"elasticsearch": "http://localhost:9200",
			"kibana":        "http://localhost:5601",
		})
	})

	log.Println("Server running on :8081")
	log.Println("Elasticsearch: http://localhost:9200")
	log.Println("Kibana: http://localhost:5601")
	log.Println("")
	log.Println("WebSocket Endpoints:")
	log.Println("  - System Monitor: ws://localhost:8081/ws/monitor")
	log.Println("  - Coding Tracker: ws://localhost:8081/ws/track")
	log.Println("")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}
