package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)



type ESClient struct {
	client *elasticsearch.Client
}

func NewESClient() (*ESClient, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://localhost:9200",
		},
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	log.Println("Connected to Elasticsearch")
	return &ESClient{client: es}, nil
}

func (es *ESClient) IndexDocument(indexName string, data interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	res, err := es.client.Index(
		indexName,
		bytes.NewReader(jsonData),
		es.client.Index.WithContext(ctx),
		es.client.Index.WithRefresh("true"),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Printf("Error indexing to %s: %s", indexName, res.String())
	}

	return nil
}

func (es *ESClient) IndexSession(session CodingSession) error {
	sessionData := map[string]interface{}{
		"duration_seconds": session.DurationSeconds,
		"editor":           session.Editor,
		"project":          session.Project,
		"language":         session.Language,
		"file_path":        session.FilePath,
		"client_timestamp": session.Timestamp,
		"server_timestamp": time.Now().Format(time.RFC3339),
	}

	if session.LinesOfCode != nil {
		sessionData["lines_of_code"] = *session.LinesOfCode
	}

	return es.IndexDocument("coding-sessions", sessionData)
}

func (es *ESClient) IndexMetrics(metrics SystemMetrics) error {
	return es.IndexDocument("system-metrics", metrics)
}

func main() {
	// Initialize Elasticsearch
	esClient, err := NewESClient()
	if err != nil {
		log.Fatal("Failed to create Elasticsearch client:", err)
	}

	hub := newHub()
	go hub.run()

	http.HandleFunc("/ws/monitor", func(w http.ResponseWriter, r *http.Request) {
		monitorWSHandler(w, r, esClient, hub)
	})

	http.HandleFunc("/ws/external", func(w http.ResponseWriter, r *http.Request) {
		externalWSHandler(w, r, hub)
	})

	http.HandleFunc("/ws/track", func(w http.ResponseWriter, r *http.Request) {
		trackingWSHandler(w, r, esClient, hub)
	})

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "ok",
			"elasticsearch":    "connected",
			"connected_clients": len(hub.clients),
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
			"connected_clients": len(hub.clients),
		})
	})

	log.Println("Server running on :8081")
	log.Println("Elasticsearch: http://localhost:9200")
	log.Println("Kibana: http://localhost:5601")
	log.Println("")
	log.Println("🔌 WebSocket Endpoints:")
	log.Println("   - System Monitor: ws://localhost:8081/ws/monitor (receive broadcasts)")
	log.Println("   - Coding Tracker: ws://localhost:8081/ws/track (send sessions)")
	log.Println("")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}
