package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

type ESClient struct {
	client *elasticsearch.Client
}

func NewESClient() (*ESClient, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{"http://localhost:9200"},
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
