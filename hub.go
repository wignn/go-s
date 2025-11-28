package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	// clients maps connection -> filter ("" == all)
	clients   map[*websocket.Conn]string
	broadcast chan interface{}
	// register accepts Subscription objects (Conn + Filter)
	register   chan Subscription
	unregister chan *websocket.Conn
	mutex      sync.RWMutex
	// weekly session records keyed by client identifier (e.g., client IP)
	weeklyRecords map[string][]SessionRecord
}

func newHub() *Hub {
	return &Hub{
		clients:       make(map[*websocket.Conn]string),
		broadcast:     make(chan interface{}, 256),
		register:      make(chan Subscription),
		unregister:    make(chan *websocket.Conn),
		weeklyRecords: make(map[string][]SessionRecord),
	}
}

func (h *Hub) run() {
	for {
		select {
		case sub := <-h.register:
			h.mutex.Lock()
			h.clients[sub.Conn] = sub.Filter
			h.mutex.Unlock()
			log.Printf("Client registered. Total: %d (filter=%s)", len(h.clients), sub.Filter)

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
			for client, filter := range h.clients {
				// if client has a filter, only send matching types
				if filter != "" {
					if bm, ok := message.(BroadcastMessage); ok {
						if bm.Type != filter {
							continue
						}
					} else {
						// message not a BroadcastMessage, skip
						continue
					}
				}

				err := client.WriteMessage(websocket.TextMessage, jsonData)
				if err != nil {
					log.Printf("Broadcast error: %v", err)
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
