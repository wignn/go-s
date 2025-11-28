package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan interface{}
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.RWMutex
	// weekly session records keyed by client identifier (e.g., client IP)
	weeklyRecords map[string][]SessionRecord
}

func newHub() *Hub {
	return &Hub{
		clients:       make(map[*websocket.Conn]bool),
		broadcast:     make(chan interface{}, 256),
		register:      make(chan *websocket.Conn),
		unregister:    make(chan *websocket.Conn),
		weeklyRecords: make(map[string][]SessionRecord),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Printf("Client registered. Total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
			h.mutex.Unlock()
			log.Printf("Client unregistered. Total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mutex.RLock()
			jsonData, _ := json.Marshal(message)
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, jsonData)
				if err != nil {
					log.Printf("❌ Broadcast error: %v", err)
					client.Close()
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

func eventID() string {
	return time.Now().Format("20060102150405")
}

// AddSessionRecord records a session duration for a client key and returns
// the total seconds recorded for the last 7 days for that client.
func (h *Hub) AddSessionRecord(clientKey string, duration int64) int64 {
	now := time.Now()
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	recs := h.weeklyRecords[clientKey]
	recs = append(recs, SessionRecord{Timestamp: now, Duration: duration})

	var pruned []SessionRecord
	var total int64
	for _, r := range recs {
		if r.Timestamp.After(sevenDaysAgo) {
			pruned = append(pruned, r)
			total += r.Duration
		}
	}

	h.weeklyRecords[clientKey] = pruned
	return total
}
