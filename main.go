package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

type ESClient struct {
	client *elasticsearch.Client
}

func NewESClient() (*ESClient, error) {
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}

	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	// Test connection
	res, err := es.Info()
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Printf("Elasticsearch connection error: %s", res.String())
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
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	esClient, err := NewESClient()
	if err != nil {
		log.Printf("Warning: Failed to create Elasticsearch client: %v", err)
		log.Println("Server will continue without Elasticsearch indexing")
		esClient = nil
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

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		esStatus := "disconnected"
		if esClient != nil {
			esStatus = "connected"
		}

		hub.mutex.RLock()
		clientCount := len(hub.clients)
		hub.mutex.RUnlock()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":            "ok",
			"elasticsearch":     esStatus,
			"connected_clients": clientCount,
			"timestamp":         time.Now().Format(time.RFC3339),
		})
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		hub.mutex.RLock()
		clientsInfo := make(map[string]string)
		for conn, filter := range hub.clients {
			clientsInfo[conn.RemoteAddr().String()] = filter
		}
		
		weeklyStats := make(map[string]int64)
		for client, records := range hub.weeklyRecords {
			var total int64
			for _, rec := range records {
				total += rec.Duration
			}
			weeklyStats[client] = total
		}
		hub.mutex.RUnlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"clients":       clientsInfo,
			"weekly_totals": weeklyStats,
			"timestamp":     time.Now().Format(time.RFC3339),
		})
	})

	// Root endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		
		hub.mutex.RLock()
		clientCount := len(hub.clients)
		hub.mutex.RUnlock()

		port := os.Getenv("PORT")
		if port == "" {
			port = "8081"
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "Coding Tracker Server",
			"version": "1.0.0",
			"endpoints": map[string]string{
				"monitor":  "ws://localhost:" + port + "/ws/monitor",
				"external": "ws://localhost:" + port + "/ws/external",
				"track":    "ws://localhost:" + port + "/ws/track",
				"health":   "http://localhost:" + port + "/health",
				"stats":    "http://localhost:" + port + "/stats",
			},
			"connected_clients": clientCount,
			"timestamp":         time.Now().Format(time.RFC3339),
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Coding Tracker Server Started")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Server running on port %s", port)
	log.Println("")
	log.Println("WebSocket Endpoints:")
	log.Printf("   • Monitor (metrics):     ws://localhost:%s/ws/monitor", port)
	log.Printf("   • External (sessions):   ws://localhost:%s/ws/external", port)
	log.Printf("   • Track (send data):     ws://localhost:%s/ws/track", port)
	log.Println("")
	log.Println("HTTP Endpoints:")
	log.Printf("   • Health Check:          http://localhost:%s/health", port)
	log.Printf("   • Statistics:            http://localhost:%s/stats", port)
	log.Printf("   • API Info:              http://localhost:%s/", port)
	log.Println("")
	if esClient != nil {
		esURL := os.Getenv("ELASTICSEARCH_URL")
		if esURL == "" {
			esURL = "http://localhost:9200"
		}
		log.Printf("Elasticsearch:            %s", esURL)
	} else {
		log.Println("Elasticsearch: Not connected (data will not be persisted)")
	}
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}